// Package adapter defines the contract every tool integration implements.
// Keep this small: contributors should be able to add a tool in one file.
package adapter

import (
	"time"

	"github.com/cachebag/spools/internal/bundle"
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

	Export(id string) (*bundle.Bundle, error)

	Import(b *bundle.Bundle, opts ImportOptions) error
}

type ImportOptions struct {
	DryRun      bool
	ProjectRoot string
}
