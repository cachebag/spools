package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeRemote(t *testing.T) {
	want := "github.com/cachebag/spools"
	for _, r := range []string{
		"git@github.com:cachebag/spools.git",
		"ssh://git@github.com/cachebag/spools",
		"https://github.com/cachebag/spools.git",
		"https://github.com/cachebag/spools/",
	} {
		if got := normalizeRemote(r); got != want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", r, got, want)
		}
	}
}

func TestResolveRebasesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p := filepath.Join(home, "personal", "spools")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve("", "/nope/olduser/personal/spools", "/nope/olduser", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("got %q, want %q", got, p)
	}
}

func TestResolveByRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "renamed")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", "git@github.com:cachebag/spools.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Setenv(EnvDirs, dir)
	got, err := Resolve("", "/nope/spools", "", "https://github.com/cachebag/spools")
	if err != nil {
		t.Fatal(err)
	}
	if got != repo {
		t.Fatalf("got %q, want %q", got, repo)
	}
}
