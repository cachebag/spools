package zed

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
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
	return &Zed{dbPath: path}
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

// cwdPayload builds a payload the way Zed would, so windows paths get their
// backslashes JSON-escaped.
func cwdPayload(t *testing.T, root string) string {
	t.Helper()
	out, err := json.Marshal(map[string]string{"cwd": root + "/main.go"})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestExportImportRoundTrip(t *testing.T) {
	src, dst := newStore(t), newStore(t)
	origin := t.TempDir()
	insert(t, src, "t1", origin, cwdPayload(t, origin))

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
	if want := cwdPayload(t, target); payload != want {
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
	insert(t, src, "t1", t.TempDir(), `{}`)
	b, err := src.Export("t1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Import(b, adapter.ImportOptions{DryRun: true, ProjectRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", "file:"+dst.dbPath+"?mode=ro")
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote %d rows (err %v)", n, err)
	}
}
