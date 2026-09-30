// Package zed reads Zed's agent thread store.
package zed

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cachebag/spools/internal/adapter"
	"github.com/cachebag/spools/internal/bundle"
	"github.com/cachebag/spools/internal/project"
	"github.com/klauspost/compress/zstd"
	_ "modernc.org/sqlite"
)

// Zed keeps agent threads in <dir>/threads/threads.db and workspace history
// in <dir>/db/<channel>/db.sqlite.
type Zed struct {
	dbPath string
	dir    string
}

func New() *Zed {
	dir := defaultDir()
	return &Zed{dbPath: filepath.Join(dir, "threads", "threads.db"), dir: dir}
}

func defaultDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Zed")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Zed")
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, "zed")
		}
		return filepath.Join(home, ".local", "share", "zed")
	}
}

func (z *Zed) Name() string { return "zed" }

func (z *Zed) Detect() bool {
	_, err := os.Stat(z.dbPath)
	return err == nil
}

// blegh...
func (z *Zed) Running() bool {
	name := "zed"
	if runtime.GOOS == "darwin" {
		name = "Zed"
	}
	return exec.Command("pgrep", "-x", name).Run() == nil
}

func (z *Zed) open() (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+z.dbPath+"?mode=ro")
}

func (z *Zed) List() ([]adapter.Session, error) {
	db, err := z.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, summary, updated_at, folder_paths FROM threads ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []adapter.Session
	for rows.Next() {
		var s adapter.Session
		var updated string
		var folders sql.NullString
		if err := rows.Scan(&s.ID, &s.Title, &updated, &folders); err != nil {
			return nil, err
		}
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		s.Project = folders.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// KnownProjects reads local workspaces from every release channel's db
// (stable, preview, ...), newest first. Remote (ssh) workspaces are skipped
// since their paths live on another machine.
func (z *Zed) KnownProjects() ([]string, error) {
	dbs, err := filepath.Glob(filepath.Join(z.dir, "db", "*", "db.sqlite"))
	if err != nil {
		return nil, err
	}

	type workspace struct{ path, ts string }
	var all []workspace
	for _, path := range dbs {
		db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
		if err != nil {
			return nil, err
		}
		rows, err := db.Query(`SELECT paths, timestamp FROM workspaces
			WHERE remote_connection_id IS NULL AND paths IS NOT NULL AND paths != ''`)
		if err != nil {
			// 0-global and friends have no workspaces table.
			db.Close()
			continue
		}
		for rows.Next() {
			var paths, ts string
			if err := rows.Scan(&paths, &ts); err != nil {
				rows.Close()
				db.Close()
				return nil, err
			}
			for _, p := range strings.Split(paths, "\n") {
				all = append(all, workspace{p, ts})
			}
		}
		err = rows.Err()
		rows.Close()
		db.Close()
		if err != nil {
			return nil, err
		}
	}

	// Zed's timestamps (RFC 3339) sort as strings.
	sort.SliceStable(all, func(i, j int) bool { return all[i].ts > all[j].ts })
	seen := map[string]bool{}
	var out []string
	for _, w := range all {
		if !seen[w.path] {
			seen[w.path] = true
			out = append(out, w.path)
		}
	}
	return out, nil
}

func decode(dataType string, data []byte) ([]byte, error) {
	switch dataType {
	case "zstd":
		r, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	case "json":
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported data_type %q", dataType)
	}
}

func (z *Zed) Export(id string) (*bundle.Bundle, error) {
	db, err := z.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var summary, updated, dataType string
	var data []byte
	var parentID, folders, foldersOrder, created sql.NullString
	err = db.QueryRow(
		`SELECT summary, updated_at, data_type, data, parent_id, folder_paths, folder_paths_order, created_at FROM threads WHERE id = ?`, id,
	).Scan(&summary, &updated, &dataType, &data, &parentID, &folders, &foldersOrder, &created)
	if err != nil {
		return nil, err
	}

	payload, err := decode(dataType, data)
	if err != nil {
		return nil, err
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("thread %s: decoded payload is not valid JSON", id)
	}

	projectRoot := strings.Split(folders.String, "\n")[0]
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()

	updatedAt, _ := time.Parse(time.RFC3339, updated)

	return &bundle.Bundle{
		Version:   bundle.Version,
		Tool:      z.Name(),
		SessionID: id,
		Title:     summary,
		UpdatedAt: updatedAt,
		GitRemote: project.Remote(projectRoot),
		GitBranch: project.Branch(projectRoot),
		Origin:    bundle.Origin{Machine: host, ProjectRoot: projectRoot, Home: home},
		Payload:   bundle.Templatize(payload, projectRoot, home),
		Meta: map[string]string{
			"data_type":          dataType,
			"parent_id":          parentID.String,
			"folder_paths":       folders.String,
			"folder_paths_order": foldersOrder.String,
			"created_at":         created.String,
		},
	}, nil
}

// zedTime matches how Zed writes timestamps (RFC 3339, +00:00 rather than Z).
const zedTime = "2006-01-02T15:04:05.999999999-07:00"

func encode(payload []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	out := enc.EncodeAll(payload, nil)
	return out, enc.Close()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (z *Zed) Import(b *bundle.Bundle, opts adapter.ImportOptions) (*adapter.ImportResult, error) {
	root := opts.ProjectRoot
	home, _ := os.UserHomeDir()

	payload := bundle.Expand(b.Payload, root, home)
	if !json.Valid(payload) {
		return nil, fmt.Errorf("thread %s: payload is not valid JSON after path rewrite", b.SessionID)
	}

	// First folder is the project root; any others get rebased onto the local home.
	var folders []string
	if fp := b.Meta["folder_paths"]; fp != "" {
		folders = strings.Split(fp, "\n")
		for i := 1; i < len(folders); i++ {
			if p, ok := project.Rebase(folders[i], b.Origin.Home, home); ok {
				folders[i] = p
			}
		}
		folders[0] = root
	} else if root != "" {
		folders = []string{root}
	}

	updated := time.Now().UTC()
	if !b.UpdatedAt.IsZero() {
		updated = b.UpdatedAt.UTC()
	}

	data, err := encode(payload)
	if err != nil {
		return nil, err
	}

	mode := "rw"
	if opts.DryRun {
		mode = "ro"
	}
	db, err := sql.Open("sqlite", "file:"+z.dbPath+"?mode="+mode+"&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	res := &adapter.ImportResult{ID: b.SessionID, Title: b.Title, ProjectRoot: root}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads WHERE id = ?`, b.SessionID).Scan(&n); err != nil {
		return nil, err
	}
	res.Replaced = n > 0
	if opts.DryRun {
		return res, nil
	}

	_, err = db.Exec(
		`INSERT OR REPLACE INTO threads (id, summary, updated_at, data_type, data, parent_id, folder_paths, folder_paths_order, created_at)
		 VALUES (?, ?, ?, 'zstd', ?, ?, ?, ?, ?)`,
		b.SessionID, b.Title, updated.Format(zedTime), data,
		nullable(b.Meta["parent_id"]), nullable(strings.Join(folders, "\n")),
		nullable(b.Meta["folder_paths_order"]), nullable(b.Meta["created_at"]),
	)
	if err != nil {
		return nil, err
	}
	return res, nil
}
