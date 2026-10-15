// Package claude reads Claude Code's session store.
//
// Sessions live in ~/.claude/projects/<encoded cwd>/<session id>.jsonl, where
// the cwd is encoded by swapping every non-alphanumeric for '-'. A sibling
// dir named after the session holds tool-results. Every record in the jsonl
// carries cwd, sessionId and timestamp; ai-title records carry the title.
package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cachebag/spools/internal/adapter"
	"github.com/cachebag/spools/internal/bundle"
	"github.com/cachebag/spools/internal/project"
)

type Claude struct{ dir string } // ~/.claude/projects

func New() *Claude {
	home, _ := os.UserHomeDir()
	return &Claude{dir: filepath.Join(home, ".claude", "projects")}
}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) Detect() bool {
	fi, err := os.Stat(c.dir)
	return err == nil && fi.IsDir()
}

// Sessions are one file each and Claude only touches its own, so importing
// while it runs is safe.
func (c *Claude) Running() bool { return false }

func encodeDir(root string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, root)
}

type record struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId"`
	Cwd       string    `json:"cwd"`
	GitBranch string    `json:"gitBranch"`
	Timestamp time.Time `json:"timestamp"`
	AITitle   string    `json:"aiTitle"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type info struct {
	cwd, branch, title string
	first, last        time.Time
}

// The last ai-title record wins. Without one, the first user prompt is the title.
func scan(path string) (info, error) {
	f, err := os.Open(path)
	if err != nil {
		return info{}, err
	}
	defer func() { _ = f.Close() }()

	var in info
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 64<<20)
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if in.cwd == "" && r.Cwd != "" {
			in.cwd, in.branch = r.Cwd, r.GitBranch
		}
		if !r.Timestamp.IsZero() {
			if in.first.IsZero() {
				in.first = r.Timestamp
			}
			in.last = r.Timestamp
		}
		switch r.Type {
		case "ai-title":
			if r.AITitle != "" {
				in.title = r.AITitle
			}
		case "user":
			if in.title == "" {
				in.title = firstText(r.Message.Content)
			}
		}
	}
	return in, sc.Err()
}

func firstText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return trimTitle(s)
	}
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return trimTitle(b.Text)
			}
		}
	}
	return ""
}

func trimTitle(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

// Session ids are UUIDs. Anything else is rejected before it reaches the
// filesystem, since the id ends up in a glob and in a path that gets deleted.
func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func (c *Claude) sessionPath(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	matches, err := filepath.Glob(filepath.Join(c.dir, "*", id+".jsonl"))
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("session %s not found under %s", id, c.dir)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session %s exists under more than one project: %s", id, strings.Join(matches, ", "))
	}
}

func (c *Claude) sessions() ([]string, error) {
	return filepath.Glob(filepath.Join(c.dir, "*", "*.jsonl"))
}

func (c *Claude) List() ([]adapter.Session, error) {
	paths, err := c.sessions()
	if err != nil {
		return nil, err
	}
	var out []adapter.Session
	for _, p := range paths {
		in, err := scan(p)
		if err != nil {
			return nil, err
		}
		if in.cwd == "" {
			continue // empty or not a session
		}
		out = append(out, adapter.Session{
			ID:        strings.TrimSuffix(filepath.Base(p), ".jsonl"),
			Title:     in.title,
			Project:   in.cwd,
			UpdatedAt: in.last,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Directory names are lossy (every non-alphanumeric becomes '-'), so the cwd
// is read from a session file instead.
func (c *Claude) KnownProjects() ([]string, error) {
	sessions, err := c.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range sessions {
		if !seen[s.Project] {
			seen[s.Project] = true
			out = append(out, s.Project)
		}
	}
	return out, nil
}

func (c *Claude) Export(id string) (*bundle.Bundle, error) {
	path, err := c.sessionPath(id)
	if err != nil {
		return nil, err
	}
	in, err := scan(path)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()

	var files []bundle.File
	side := strings.TrimSuffix(path, ".jsonl")
	err = filepath.WalkDir(side, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(side, p)
		files = append(files, bundle.File{Path: filepath.ToSlash(rel), Data: bundle.Templatize(data, in.cwd, home)})
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	return &bundle.Bundle{
		Version:   bundle.Version,
		Tool:      c.Name(),
		SessionID: id,
		Title:     in.title,
		UpdatedAt: in.last,
		GitRemote: project.Remote(in.cwd),
		GitBranch: in.branch,
		Origin:    bundle.Origin{Machine: host, ProjectRoot: in.cwd, Home: home},
		Payload:   bundle.Templatize(payload, in.cwd, home),
		Files:     files,
		Meta:      map[string]string{"created_at": in.first.Format(time.RFC3339Nano)},
	}, nil
}

func (c *Claude) Import(b *bundle.Bundle, opts adapter.ImportOptions) (*adapter.ImportResult, error) {
	root := opts.ProjectRoot
	if root == "" {
		return nil, fmt.Errorf("claude sessions need a project; pass --project <path>")
	}
	home, _ := os.UserHomeDir()

	dir := filepath.Join(c.dir, encodeDir(root))
	path := filepath.Join(dir, b.SessionID+".jsonl")
	res := &adapter.ImportResult{ID: b.SessionID, Title: b.Title, ProjectRoot: root}
	_, err := os.Stat(path)
	res.Replaced = err == nil
	if opts.DryRun {
		return res, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, bundle.Expand(b.Payload, root, home), 0o600); err != nil {
		return nil, err
	}
	side := strings.TrimSuffix(path, ".jsonl")
	for _, f := range b.Files {
		p := filepath.Join(side, filepath.FromSlash(f.Path))
		if !strings.HasPrefix(p, side+string(filepath.Separator)) {
			return nil, fmt.Errorf("bundle file %q escapes the session dir", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, bundle.Expand(f.Data, root, home), 0o600); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (c *Claude) Remove(id string) error {
	path, err := c.sessionPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.RemoveAll(strings.TrimSuffix(path, ".jsonl"))
}
