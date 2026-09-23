// Package zed reads Zed's agent thread store.
//
// IDK IF THIS IS CORRECT
package zed

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/CHANGEME/spools/internal/adapter"
	"github.com/CHANGEME/spools/internal/bundle"
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

	rows, err := db.Query(`SELECT id, summary, updated_at FROM threads ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []adapter.Session
	for rows.Next() {
		var s adapter.Session
		var updated string
		if err := rows.Scan(&s.ID, &s.Title, &updated); err != nil {
			return nil, err
		}
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (z *Zed) Export(id string) (*bundle.Bundle, error) {
	panic("not implemented")
}

func (z *Zed) Import(b *bundle.Bundle, opts adapter.ImportOptions) error {
	panic("not implemented")
}
