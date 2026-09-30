package adapter

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cachebag/spools/internal/bundle"
)

type fake struct {
	detect, running bool
	got             *ImportOptions
}

func (f *fake) Name() string                          { return "fake" }
func (f *fake) Detect() bool                          { return f.detect }
func (f *fake) Running() bool                         { return f.running }
func (f *fake) List() ([]Session, error)              { return nil, nil }
func (f *fake) KnownProjects() ([]string, error)      { return nil, nil }
func (f *fake) Export(string) (*bundle.Bundle, error) { return nil, nil }
func (f *fake) Import(b *bundle.Bundle, opts ImportOptions) (*ImportResult, error) {
	f.got = &opts
	return &ImportResult{ID: b.SessionID, ProjectRoot: opts.ProjectRoot}, nil
}

func bundleAt(root string) *bundle.Bundle {
	return &bundle.Bundle{Version: bundle.Version, Tool: "fake", SessionID: "t1", Origin: bundle.Origin{ProjectRoot: root}}
}

func TestImportResolvesProject(t *testing.T) {
	root := t.TempDir()
	f := &fake{detect: true}
	if _, err := Import(f, bundleAt(root), ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	if f.got.ProjectRoot != root {
		t.Fatalf("adapter got root %q, want %q", f.got.ProjectRoot, root)
	}
}

func TestImportRefusals(t *testing.T) {
	gone := bundleAt(filepath.Join(t.TempDir(), "gone"))
	gone.GitRemote = "git@github.com:cachebag/spools.git"

	wrongTool := bundleAt(t.TempDir())
	wrongTool.Tool = "other"

	for _, tc := range []struct {
		name string
		f    *fake
		b    *bundle.Bundle
		opts ImportOptions
		want string
	}{
		{"wrong tool", &fake{detect: true}, wrongTool, ImportOptions{}, `not "fake"`},
		{"not installed", &fake{}, bundleAt(t.TempDir()), ImportOptions{}, "not found"},
		{"running", &fake{detect: true, running: true}, bundleAt(t.TempDir()), ImportOptions{}, "running"},
		{"missing project", &fake{detect: true}, gone, ImportOptions{}, "open it in fake once, or pass --project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Import(tc.f, tc.b, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
			if tc.f.got != nil {
				t.Fatal("adapter Import should not have been called")
			}
		})
	}
}

func TestImportDryRunWhileRunning(t *testing.T) {
	f := &fake{detect: true, running: true}
	if _, err := Import(f, bundleAt(t.TempDir()), ImportOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if f.got == nil || !f.got.DryRun {
		t.Fatal("dry run should reach the adapter even while running")
	}
}
