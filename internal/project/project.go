// Package project finds the local checkout that matches a bundle's origin project.
package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnvDirs overrides the directories searched for a matching git remote
// (os.PathListSeparator separated).
const EnvDirs = "SPOOLS_PROJECT_DIRS"

// Resolve picks the local project root for a thread that lived at originRoot
// on a machine whose home was originHome. Tried in order: the override, the
// original path, the original path rebased onto the local home, then any repo
// in the search dirs whose origin remote matches gitRemote.
func Resolve(override, originRoot, originHome, gitRemote string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if !isDir(abs) {
			return "", fmt.Errorf("--project %s: not a directory", abs)
		}
		return abs, nil
	}
	if originRoot == "" {
		return "", nil
	}
	if isDir(originRoot) {
		return originRoot, nil
	}

	home, _ := os.UserHomeDir()
	if p, ok := Rebase(originRoot, originHome, home); ok && isDir(p) {
		return p, nil
	}

	if want := normalizeRemote(gitRemote); want != "" {
		for _, dir := range searchDirs(home) {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				p := filepath.Join(dir, e.Name())
				// Skip non-repos without shelling out; git -C would walk up to a parent repo.
				if _, err := os.Stat(filepath.Join(p, ".git")); err != nil {
					continue
				}
				if normalizeRemote(Remote(p)) == want {
					return p, nil
				}
			}
		}
	}

	return "", fmt.Errorf("can't find project %s locally (remote %q); pass --project <path> or set %s", originRoot, gitRemote, EnvDirs)
}

func searchDirs(home string) []string {
	if v := os.Getenv(EnvDirs); v != "" {
		return filepath.SplitList(v)
	}
	if home == "" {
		return nil
	}
	var out []string
	for _, d := range []string{"personal", "code", "src", "projects"} {
		out = append(out, filepath.Join(home, d))
	}
	return out
}

// Rebase moves path from under fromHome to under toHome.
func Rebase(path, fromHome, toHome string) (string, bool) {
	if fromHome == "" || toHome == "" {
		return "", false
	}
	rel, ok := strings.CutPrefix(path, fromHome)
	if !ok || rel != "" && rel[0] != '/' && rel[0] != '\\' {
		return "", false
	}
	return filepath.Join(toHome, filepath.FromSlash(rel)), true
}

// Remote returns dir's origin remote URL, or "" if there isn't one.
func Remote(dir string) string { return git(dir, "remote", "get-url", "origin") }

// Branch returns dir's current branch, or "" if it isn't a repo.
func Branch(dir string) string { return git(dir, "rev-parse", "--abbrev-ref", "HEAD") }

func git(dir string, args ...string) string {
	if dir == "" {
		return ""
	}
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// normalizeRemote makes ssh and https forms of the same remote compare equal:
// git@github.com:a/b.git, ssh://git@github.com/a/b, https://github.com/a/b -> github.com/a/b
func normalizeRemote(r string) string {
	r = strings.TrimSpace(r)
	if r == "" {
		return ""
	}
	if i := strings.Index(r, "://"); i >= 0 {
		r = r[i+3:]
	} else {
		r = strings.Replace(r, ":", "/", 1)
	}
	if i := strings.Index(r, "@"); i >= 0 {
		r = r[i+1:]
	}
	r = strings.TrimSuffix(strings.TrimSuffix(r, "/"), ".git")
	return strings.ToLower(r)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
