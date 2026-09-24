package zed

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cachebag/spools/internal/adapter"
)

const schema = `CREATE TABLE threads (
	id TEXT PRIMARY KEY,
	summary TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	data_type TEXT NOT NULL,
	data BLOB NOT NULL,
	parent_id TEXT, folder_paths TEXT, folder_paths_order TEXT, created_at TEXT)`

func newStore(t *testing.T) *Zed {
	t.Helper()
	path := filepath.Join(t.TempDir(), "threads.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return &Zed{dbPath: path, running: func() bool { return false }}
}

func insert(t *testing.T, z *Zed, id, folder, payload string) {
	t.Helper()
	data, err := encode([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+z.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO threads VALUES (?, 'hello', '2026-06-03T22:00:06.279733696+00:00', 'zstd', ?, NULL, ?, '0', '2026-06-03T21:00:00+00:00')`,
		id, data, folder)
	if err != nil {
		t.Fatal(err)
	}
}

func payloadOf(t *testing.T, z *Zed, id string) (payload, folders string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+z.dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var dataType string
	var data []byte
	if err := db.QueryRow(`SELECT data_type, data, folder_paths FROM threads WHERE id = ?`, id).Scan(&dataType, &data, &folders); err != nil {
		t.Fatal(err)
	}
	out, err := decode(dataType, data)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), folders
}

func TestExportImportRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("export doesn't templatize JSON-escaped windows paths yet")
	}
	src, dst := newStore(t), newStore(t)
	origin := t.TempDir()
	insert(t, src, "t1", origin, `{"cwd":"`+origin+`/main.go"}`)

	b, err := src.Export("t1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b.Payload), "{{PROJECT}}/main.go") {
		t.Fatalf("export didn't templatize project: %s", b.Payload)
	}

	target := t.TempDir()
	res, err := dst.Import(b, adapter.ImportOptions{ProjectRoot: target})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced || res.ProjectRoot != target {
		t.Fatalf("unexpected result %+v", res)
	}
	payload, folders := payloadOf(t, dst, "t1")
	if want := `{"cwd":"` + target + `/main.go"}`; payload != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
	if folders != target {
		t.Fatalf("folder_paths = %q, want %q", folders, target)
	}

	res, err = dst.Import(b, adapter.ImportOptions{ProjectRoot: target})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replaced {
		t.Fatal("second import should report replaced")
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	src, dst := newStore(t), newStore(t)
	origin := t.TempDir()
	insert(t, src, "t1", origin, `{}`)
	b, err := src.Export("t1")
	if err != nil {
		t.Fatal(err)
	}
	// Running only blocks real writes, not dry runs.
	dst.running = func() bool { return true }
	if _, err := dst.Import(b, adapter.ImportOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", "file:"+dst.dbPath+"?mode=ro")
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote %d rows (err %v)", n, err)
	}
}

func TestImportRefusesWhileRunning(t *testing.T) {
	src, dst := newStore(t), newStore(t)
	insert(t, src, "t1", t.TempDir(), `{}`)
	b, err := src.Export("t1")
	if err != nil {
		t.Fatal(err)
	}
	dst.running = func() bool { return true }
	if _, err := dst.Import(b, adapter.ImportOptions{}); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("want running error, got %v", err)
	}
}

func TestImportMissingProject(t *testing.T) {
	src, dst := newStore(t), newStore(t)
	origin := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	insert(t, src, "t1", origin, `{}`)
	b, err := src.Export("t1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(origin); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPOOLS_PROJECT_DIRS", t.TempDir())
	if _, err := dst.Import(b, adapter.ImportOptions{}); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("want --project hint, got %v", err)
	}
}
