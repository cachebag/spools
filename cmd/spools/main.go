package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/cachebag/spools/internal/adapter"
	"github.com/cachebag/spools/internal/adapter/zed"
	"github.com/cachebag/spools/internal/bundle"
	"github.com/cachebag/spools/internal/remote"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "spools",
		Short: "Move agent threads/sessions between machines",

		SilenceUsage:  true,
		SilenceErrors: true,
	}

	adapters := []adapter.Adapter{zed.New()}
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

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List local sessions",
		RunE: func(c *cobra.Command, args []string) error {
			if !a.Detect() {
				return fmt.Errorf("%s not found on this machine", a.Name())
			}
			sessions, err := a.List()
			if err != nil {
				return err
			}
			for _, s := range sessions {
				fmt.Printf("%-40s  %-30s  %s\n", s.ID, s.Title, s.UpdatedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	})

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
		Use:   "pull <host> <session-id>",
		Short: "Export a session on host over ssh and import it locally",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			var buf bytes.Buffer
			if err := remote.Spools(args[0], nil, &buf, a.Name(), "export", args[1]); err != nil {
				return err
			}
			b, err := bundle.Read(&buf)
			if err != nil {
				return err
			}
			return runImport(a, b, opts)
		},
	}
	addImportFlags(pullCmd)
	cmd.AddCommand(pullCmd)

	return cmd
}

func runImport(a adapter.Adapter, b *bundle.Bundle, opts adapter.ImportOptions) error {
	res, err := a.Import(b, opts)
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
	return nil
}
