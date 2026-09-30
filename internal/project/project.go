// Package project finds the local checkout that matches a bundle's origin project.
package project

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNotFound = errors.New("project not found locally")

// Resolve picks the local project root for a thread that lived at originRoot.
// Tried in order: the override, the original path, then the first of the
// tool's recently opened projects (most recent first) with a matching remote.
// known is only called if the first two miss.
func Resolve(override, originRoot, gitRemote string, known func() ([]string, error)) (string, error) {
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

	want := normalizeRemote(gitRemote)
	if want == "" {
		return "", fmt.Errorf("%s isn't here and has no git remote to match on; pass --project <path>", originRoot)
	}
	candidates, err := known()
	if err != nil {
		return "", fmt.Errorf("listing recent projects: %w", err)
	}
	for _, p := range candidates {
		// Only repos themselves; an opened parent dir like ~/code doesn't count.
		if _, err := os.Stat(filepath.Join(p, ".git")); err != nil {
			continue
		}
		for _, r := range remotes(p) {
			if normalizeRemote(r) == want {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("%w: %s (remote %s)", ErrNotFound, originRoot, gitRemote)
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

// remotes returns every remote URL in dir, so forks match on upstream too.
func remotes(dir string) []string {
	var out []string
	for _, line := range strings.Split(git(dir, "config", "--get-regexp", `^remote\..*\.url$`), "\n") {
		if _, url, ok := strings.Cut(line, " "); ok {
			out = append(out, url)
		}
	}
	return out
}

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
