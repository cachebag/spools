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
	"strings"
	"time"

	"github.com/cachebag/spools/internal/adapter"
	"github.com/cachebag/spools/internal/bundle"
	"github.com/klauspost/compress/zstd"
	_ "modernc.org/sqlite"
)

type Zed struct{ dbPath string }

func New() *Zed { return &Zed{dbPath: defaultDBPath()} }

func defaultDBPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Zed", "threads", "threads.db")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Zed", "threads", "threads.db")
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, "zed", "threads", "threads.db")
		}
		return filepath.Join(home, ".local", "share", "zed", "threads", "threads.db")
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

func gitOutput(root string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (z *Zed) Export(id string) (*bundle.Bundle, error) {
	db, err := z.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var summary, updated, dataType string
	var data []byte
	var parentID, folders, created sql.NullString
	err = db.QueryRow(
		`SELECT summary, updated_at, data_type, data, parent_id, folder_paths, created_at FROM threads WHERE id = ?`, id,
	).Scan(&summary, &updated, &dataType, &data, &parentID, &folders, &created)
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

	s := string(payload)
	if projectRoot != "" {
		s = strings.ReplaceAll(s, projectRoot, "{{PROJECT}}")
	}
	if home != "" {
		s = strings.ReplaceAll(s, home, "{{HOME}}")
	}

	updatedAt, _ := time.Parse(time.RFC3339, updated)

	return &bundle.Bundle{
		Version:   bundle.Version,
		Tool:      z.Name(),
		SessionID: id,
		Title:     summary,
		UpdatedAt: updatedAt,
		GitRemote: gitOutput(projectRoot, "remote", "get-url", "origin"),
		GitBranch: gitOutput(projectRoot, "rev-parse", "--abbrev-ref", "HEAD"),
		Origin:    bundle.Origin{Machine: host, ProjectRoot: projectRoot, Home: home},
		Payload:   []byte(s),
		Meta: map[string]string{
			"data_type":    dataType,
			"parent_id":    parentID.String,
			"folder_paths": folders.String,
			"created_at":   created.String,
		},
	}, nil
}

func (z *Zed) Import(b *bundle.Bundle, opts adapter.ImportOptions) error {
	panic("not implemented")
}
