package project

import (
	"errors"
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

func gitRepo(t *testing.T, dir string, remotes ...string) {
	t.Helper()
	cmds := [][]string{{"init", "-q", dir}}
	for i := 0; i+1 < len(remotes); i += 2 {
		cmds = append(cmds, []string{"-C", dir, "remote", "add", remotes[i], remotes[i+1]})
	}
	for _, args := range cmds {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func known(paths ...string) func() ([]string, error) {
	return func() ([]string, error) { return paths, nil }
}

func TestResolveKnownProjects(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	parent := dir // opened as a workspace but not a repo itself
	fork := filepath.Join(dir, "fork")
	clone := filepath.Join(dir, "clone")
	gitRepo(t, fork, "origin", "git@github.com:someone/spools.git", "upstream", "https://github.com/cachebag/spools")
	gitRepo(t, clone, "origin", "git@github.com:cachebag/spools.git")

	for _, tc := range []struct {
		name  string
		known []string
		want  string
	}{
		{"most recent match wins", []string{parent, clone, fork}, clone},
		{"matches non-origin remotes", []string{parent, fork, clone}, fork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve("", "/nope/spools", "https://github.com/cachebag/spools", known(tc.known...))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	_, err := Resolve("", "/nope/other", "git@github.com:cachebag/other.git", known(parent, fork, clone))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestResolveOriginPathSkipsKnown(t *testing.T) {
	root := t.TempDir()
	got, err := Resolve("", root, "", func() ([]string, error) {
		t.Fatal("known projects shouldn't be read when the original path exists")
		return nil, nil
	})
	if err != nil || got != root {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestResolveNoRemote(t *testing.T) {
	_, err := Resolve("", "/nope/spools", "", known())
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("want a --project error that isn't ErrNotFound, got %v", err)
	}
}
