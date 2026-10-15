package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cachebag/spools/internal/adapter"
	"github.com/cachebag/spools/internal/adapter/claude"
	"github.com/cachebag/spools/internal/adapter/zed"
	"github.com/cachebag/spools/internal/bundle"
	"github.com/cachebag/spools/internal/remote"
	"github.com/spf13/cobra"
)

// Set by the release build with -ldflags "-X main.version=...". A go install
// build gets the module version from the build info instead.
var version = "dev"

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	root := &cobra.Command{
		Use:     "spools",
		Short:   "Move agent threads/sessions between machines",
		Version: buildVersion(),

		SilenceUsage:  true,
		SilenceErrors: true,
	}

	adapters := []adapter.Adapter{zed.New(), claude.New()}
	for _, a := range adapters {
		root.AddCommand(toolCommand(a))
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func toolCommand(a adapter.Adapter) *cobra.Command {
	cmd := &cobra.Command{Use: a.Name(), Short: "Manage " + a.Name() + " sessions"}

	var projectFilter string
	var asJSON bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List local sessions",
		RunE: func(c *cobra.Command, args []string) error {
			if !a.Detect() {
				return fmt.Errorf("%s not found on this machine", a.Name())
			}
			sessions, err := listSessions(a, projectFilter)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(sessions)
			}
			for _, s := range sessions {
				title := s.Title
				if title == "" {
					title = "(untitled)"
				} else if len(title) > 40 {
					title = title[:37] + "..."
				}
				fmt.Printf("%-36s  %-40s  %-16s  %s\n", s.ID, title, s.UpdatedAt.Format("2006-01-02 15:04"), s.Project)
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&projectFilter, "project", "", "only sessions whose project path contains this")
	listCmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	cmd.AddCommand(listCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "export <session-id>",
		Short: "Export one session as a bundle (JSON on stdout)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			b, err := a.Export(args[0])
			if err != nil {
				return err
			}
			return bundle.Write(os.Stdout, b)
		},
	})

	var opts adapter.ImportOptions
	addImportFlags := func(c *cobra.Command) {
		c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be imported without writing")
		c.Flags().StringVar(&opts.ProjectRoot, "project", "", "local project root to attach the thread to")
	}

	importCmd := &cobra.Command{
		Use:   "import <file|->",
		Short: "Import a bundle from a file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var r io.Reader = os.Stdin
			if args[0] != "-" {
				data, err := os.ReadFile(args[0])
				if err != nil {
					return err
				}
				r = bytes.NewReader(data)
			}
			b, err := bundle.Read(r)
			if err != nil {
				return err
			}
			return runImport(a, b, opts)
		},
	}
	addImportFlags(importCmd)
	cmd.AddCommand(importCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "rm <session-id>",
		Short: "Delete a local session",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runRemove(a, zed.New(), args[0], os.Stdout)
		},
	})

	pushCmd := &cobra.Command{
		Use:   "push <host> <session-id>",
		Short: "Export a local session and import it on host over ssh",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			b, err := a.Export(args[1])
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := bundle.Write(&buf, b); err != nil {
				return err
			}
			remoteArgs := []string{a.Name(), "import", "-"}
			if opts.DryRun {
				remoteArgs = append(remoteArgs, "--dry-run")
			}
			if opts.ProjectRoot != "" {
				remoteArgs = append(remoteArgs, "--project", opts.ProjectRoot)
			}
			return remote.Spools(args[0], &buf, os.Stdout, remoteArgs...)
		},
	}
	addImportFlags(pushCmd)
	cmd.AddCommand(pushCmd)

	pullCmd := &cobra.Command{
		Use:   "pull <host> [session-id]",
		Short: "Export a session on host over ssh and import it locally",
		Long: `Export a session on host over ssh and import it locally.

With a session id, --project is the local directory to import into.
Without one, --project is required and selects every session on host whose
project path contains it; each is imported into the matching local project.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			host := args[0]
			if len(args) == 2 {
				return pull(a, host, args[1], opts)
			}
			if opts.ProjectRoot == "" {
				return fmt.Errorf("pass a session id or --project <name> to pull a whole project")
			}
			var buf bytes.Buffer
			if err := remote.Spools(host, nil, &buf, a.Name(), "list", "--json", "--project", opts.ProjectRoot); err != nil {
				return err
			}
			var sessions []adapter.Session
			if err := json.NewDecoder(&buf).Decode(&sessions); err != nil {
				return fmt.Errorf("reading session list from %s (is spools up to date there?): %w", host, err)
			}
			if len(sessions) == 0 {
				return fmt.Errorf("no %s sessions on %s match %q", a.Name(), host, opts.ProjectRoot)
			}
			opts.ProjectRoot = ""
			return pullAll(sessions, os.Stderr, func(id string) error { return pull(a, host, id, opts) })
		},
	}
	addImportFlags(pullCmd)
	pullCmd.Flags().Lookup("project").Usage = "local project root with a session id, otherwise a filter on the remote project path"
	cmd.AddCommand(pullCmd)

	return cmd
}

// sidebar is the part of the zed adapter that tracks other agents' threads.
type sidebar interface {
	Detect() bool
	Running() bool
	Unregister(tool, sessionID string) error
}

// runRemove checks that the sidebar can be cleaned before deleting anything,
// since the session can't be found again once it is gone.
func runRemove(a adapter.Adapter, z sidebar, id string, out io.Writer) error {
	external := a.Name() != "zed" && z.Detect()
	if external && z.Running() {
		return fmt.Errorf("zed is running on this machine; quit it before removing")
	}
	if err := adapter.Remove(a, id); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "removed %s\n", id)
	if !external {
		return nil
	}
	if err := z.Unregister(a.Name(), id); err != nil {
		return fmt.Errorf("session removed, but its zed sidebar row could not be: %w", err)
	}
	return nil
}

func listSessions(a adapter.Adapter, projectFilter string) ([]adapter.Session, error) {
	sessions, err := a.List()
	if err != nil {
		return nil, err
	}
	out := []adapter.Session{}
	if projectFilter == "" {
		return append(out, sessions...), nil
	}
	for _, s := range sessions {
		if strings.Contains(s.Project, projectFilter) {
			out = append(out, s)
		}
	}
	return out, nil
}

func pullAll(sessions []adapter.Session, errOut io.Writer, pull func(id string) error) error {
	var failed int
	for _, s := range sessions {
		if err := pull(s.ID); err != nil {
			_, _ = fmt.Fprintf(errOut, "%s: %v\n", s.ID, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d sessions failed", failed, len(sessions))
	}
	return nil
}

func pull(a adapter.Adapter, host, id string, opts adapter.ImportOptions) error {
	var buf bytes.Buffer
	if err := remote.Spools(host, nil, &buf, a.Name(), "export", id); err != nil {
		return err
	}
	b, err := bundle.Read(&buf)
	if err != nil {
		return err
	}
	return runImport(a, b, opts)
}

func runImport(a adapter.Adapter, b *bundle.Bundle, opts adapter.ImportOptions) error {
	res, err := adapter.Import(a, b, opts)
	if err != nil {
		return err
	}
	verb := "imported"
	if res.Replaced {
		verb = "replaced"
	}
	if opts.DryRun {
		verb = "would import"
		if res.Replaced {
			verb = "would replace"
		}
	}
	fmt.Printf("%s %s (%s) -> %s\n", verb, res.ID, res.Title, res.ProjectRoot)
	if opts.DryRun || a.Name() == "zed" {
		return nil
	}

	// External agent threads show up in zed's sidebar only via an index row.
	z := zed.New()
	if !z.Detect() {
		return nil
	}
	if z.Running() {
		fmt.Fprintln(os.Stderr, "zed is running; thread won't show in its sidebar until you quit zed and re-import")
		return nil
	}
	var created time.Time
	if c := b.Meta["created_at"]; c != "" {
		created, _ = time.Parse(time.RFC3339Nano, c)
	}
	err = z.Register(zed.External{
		Tool: a.Name(), SessionID: res.ID, Title: res.Title, Root: res.ProjectRoot,
		Created: created, Updated: b.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("registering with zed: %w", err)
	}
	fmt.Println("added to zed sidebar")
	return nil
}
