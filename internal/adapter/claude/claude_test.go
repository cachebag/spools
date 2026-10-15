package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cachebag/spools/internal/adapter"
)

func session(t *testing.T, store *Claude, id, cwd string) {
	t.Helper()
	dir := filepath.Join(store.dir, encodeDir(cwd))
	if err := os.MkdirAll(filepath.Join(dir, id, "tool-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"queue-operation","sessionId":"` + id + `"}`,
		`{"type":"user","cwd":"` + cwd + `","gitBranch":"main","timestamp":"2026-10-03T15:57:35.297Z","sessionId":"` + id + `","message":{"role":"user","content":[{"type":"text","text":"fix the thing in ` + cwd + `/main.go\nplease"}]}}`,
		`{"type":"ai-title","aiTitle":"Fix the thing","sessionId":"` + id + `"}`,
		`{"type":"assistant","cwd":"` + cwd + `","timestamp":"2026-10-03T16:00:00Z","sessionId":"` + id + `","message":{"role":"assistant","content":[]}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id, "tool-results", "x.txt"), []byte("saw "+cwd+"/a"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEncodeDir(t *testing.T) {
	if got := encodeDir("/Users/me/Desktop/fall_2026/cap.stone"); got != "-Users-me-Desktop-fall-2026-cap-stone" {
		t.Fatal(got)
	}
}

func TestListAndKnownProjects(t *testing.T) {
	c := &Claude{dir: t.TempDir()}
	session(t, c, "11111111-1111-1111-1111-111111111111", "/home/me/relay")
	session(t, c, "22222222-2222-2222-2222-222222222222", "/home/me/relay")
	session(t, c, "33333333-3333-3333-3333-333333333333", "/home/me/other")

	list, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Title != "Fix the thing" || list[0].Project == "" {
		t.Fatalf("%+v", list)
	}
	known, err := c.KnownProjects()
	if err != nil || len(known) != 2 {
		t.Fatalf("%v %v", known, err)
	}
}

func TestRoundTrip(t *testing.T) {
	src, dst := &Claude{dir: t.TempDir()}, &Claude{dir: t.TempDir()}
	session(t, src, "11111111-1111-1111-1111-111111111111", "/home/me/relay")

	b, err := src.Export("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Fix the thing" || b.GitBranch != "main" || len(b.Files) != 1 {
		t.Fatalf("%+v", b)
	}
	if !strings.Contains(string(b.Payload), "{{PROJECT}}/main.go") || !strings.Contains(string(b.Files[0].Data), "{{PROJECT}}/a") {
		t.Fatal("paths not templatized")
	}

	root := "/Users/me/code/relay"
	res, err := dst.Import(b, adapter.ImportOptions{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced {
		t.Fatal("fresh import reported replaced")
	}
	got, err := os.ReadFile(filepath.Join(dst.dir, "-Users-me-code-relay", "11111111-1111-1111-1111-111111111111.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"cwd":"`+root+`"`) || strings.Contains(string(got), "/home/me") {
		t.Fatalf("cwd not rewritten:\n%s", got)
	}
	side, err := os.ReadFile(filepath.Join(dst.dir, "-Users-me-code-relay", "11111111-1111-1111-1111-111111111111", "tool-results", "x.txt"))
	if err != nil || string(side) != "saw "+root+"/a" {
		t.Fatalf("%q %v", side, err)
	}

	res, err = dst.Import(b, adapter.ImportOptions{ProjectRoot: root, DryRun: true})
	if err != nil || !res.Replaced {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestImportNeedsProject(t *testing.T) {
	c := &Claude{dir: t.TempDir()}
	session(t, c, "11111111-1111-1111-1111-111111111111", "/home/me/relay")
	b, _ := c.Export("11111111-1111-1111-1111-111111111111")
	if _, err := c.Import(b, adapter.ImportOptions{}); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatal(err)
	}
}

func TestRemove(t *testing.T) {
	c := &Claude{dir: t.TempDir()}
	session(t, c, "11111111-1111-1111-1111-111111111111", "/home/me/relay")
	session(t, c, "22222222-2222-2222-2222-222222222222", "/home/me/relay")
	if err := c.Remove("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(c.dir, encodeDir("/home/me/relay"))
	if _, err := os.Stat(filepath.Join(dir, "11111111-1111-1111-1111-111111111111.jsonl")); !os.IsNotExist(err) {
		t.Fatal("jsonl still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "11111111-1111-1111-1111-111111111111")); !os.IsNotExist(err) {
		t.Fatal("sidecar dir still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "22222222-2222-2222-2222-222222222222.jsonl")); err != nil {
		t.Fatal("s2 should be untouched")
	}
	if err := c.Remove("11111111-1111-1111-1111-111111111111"); err == nil {
		t.Fatal("removing twice should error")
	}
}

func TestSessionPathRejectsBadIDs(t *testing.T) {
	c := &Claude{dir: t.TempDir()}
	session(t, c, "11111111-1111-1111-1111-111111111111", "/home/me/relay")
	for _, id := range []string{"", "*", "../../x", "11111111-1111-1111-1111-111111111111/../x", "not-a-uuid"} {
		if _, err := c.sessionPath(id); err == nil || !strings.Contains(err.Error(), "invalid session id") {
			t.Fatalf("%q: got %v", id, err)
		}
		if err := c.Remove(id); err == nil {
			t.Fatalf("%q: remove should fail", id)
		}
	}
}

func TestSessionPathAmbiguous(t *testing.T) {
	c := &Claude{dir: t.TempDir()}
	id := "11111111-1111-1111-1111-111111111111"
	session(t, c, id, "/home/me/relay")
	session(t, c, id, "/home/me/other")
	if _, err := c.sessionPath(id); err == nil || !strings.Contains(err.Error(), "more than one project") {
		t.Fatalf("got %v", err)
	}
}
