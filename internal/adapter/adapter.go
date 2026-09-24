// Package adapter defines the contract every tool integration implements.
// Keep this small: contributors should be able to add a tool in one file.
package adapter

import (
	"errors"
	"fmt"
	"time"

	"github.com/cachebag/spools/internal/bundle"
	"github.com/cachebag/spools/internal/project"
)

type Session struct {
	ID        string
	Title     string
	Project   string // git remote URL if resolvable, else worktree path
	UpdatedAt time.Time
}

type Adapter interface {
	Name() string

	Detect() bool

	Running() bool

	List() ([]Session, error)

	// KnownProjects returns local folders the tool has opened, most recent
	// first. Import matches a thread's git remote against these.
	KnownProjects() ([]string, error)

	Export(id string) (*bundle.Bundle, error)

	// Import writes b into the local store. Callers go through the package
	// level Import, so opts.ProjectRoot is already resolved and the tool is
	// known to be installed and not running.
	Import(b *bundle.Bundle, opts ImportOptions) (*ImportResult, error)
}

type ImportOptions struct {
	DryRun bool
	// ProjectRoot is the --project override going into Import, and the
	// resolved local root (or "" if the thread had no project) coming out.
	ProjectRoot string
}

type ImportResult struct {
	ID          string
	Title       string
	ProjectRoot string // local path the thread was rebased onto
	Replaced    bool   // a thread with this id already existed
}

// Import runs the checks every tool needs, resolves where the thread's
// project lives on this machine, then hands off to a.Import.
func Import(a Adapter, b *bundle.Bundle, opts ImportOptions) (*ImportResult, error) {
	if b.Tool != a.Name() {
		return nil, fmt.Errorf("bundle is for %q, not %q", b.Tool, a.Name())
	}
	if !a.Detect() {
		return nil, fmt.Errorf("%s not found on this machine", a.Name())
	}
	// The tool holds its store open; writing underneath it risks corrupting it.
	if !opts.DryRun && a.Running() {
		return nil, fmt.Errorf("%s is running on this machine; quit it before importing", a.Name())
	}

	root, err := project.Resolve(opts.ProjectRoot, b.Origin.ProjectRoot, b.GitRemote, a.KnownProjects)
	if errors.Is(err, project.ErrNotFound) {
		return nil, fmt.Errorf("%w; open it in %s once, or pass --project <path>", err, a.Name())
	}
	if err != nil {
		return nil, err
	}
	opts.ProjectRoot = root
	return a.Import(b, opts)
}
