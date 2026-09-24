package zed

import (
	"database/sql"
	"encoding/json"
	"os"
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

func workspaceDB(t *testing.T, dir, channel string, rows ...[3]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "db", channel), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "db", channel, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE workspaces (workspace_id INTEGER PRIMARY KEY, paths TEXT, remote_connection_id INTEGER, timestamp TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO workspaces (paths, remote_connection_id, timestamp) VALUES (?, ?, ?)`, r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKnownProjects(t *testing.T) {
	dir := t.TempDir()
	workspaceDB(t, dir, "0-stable",
		[3]any{"/code/old", nil, "2026-01-01 00:00:00"},
		[3]any{"/code/new", nil, "2026-09-01 00:00:00"},
		[3]any{"/home/me/remote", 3, "2026-09-02 00:00:00"},
		[3]any{"", nil, "2026-09-03 00:00:00"},
	)
	workspaceDB(t, dir, "0-preview",
		[3]any{"/code/a\n/code/b", nil, "2026-05-01 00:00:00"},
		[3]any{"/code/old", nil, "2026-08-01 00:00:00"},
	)
	// 0-global has no workspaces table and should be skipped.
	if err := os.MkdirAll(filepath.Join(dir, "db", "0-global"), 0o755); err != nil {
		t.Fatal(err)
	}
	global, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "db", "0-global", "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := global.Exec(`CREATE TABLE kv_store (key TEXT)`); err != nil {
		t.Fatal(err)
	}
	global.Close()

	got, err := (&Zed{dir: dir}).KnownProjects()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/code/new", "/code/old", "/code/a", "/code/b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}
