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
	if originHome != "" && home != "" {
		if rel, ok := strings.CutPrefix(originRoot, originHome); ok {
			if p := filepath.Join(home, filepath.FromSlash(rel)); isDir(p) {
				return p, nil
			}
		}
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
				if normalizeRemote(remoteOf(p)) == want {
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

func remoteOf(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
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
