package main

import (
	"fmt"
	"os"

	"github.com/CHANGEME/spools/internal/adapter"
	"github.com/CHANGEME/spools/internal/adapter/zed"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "spools",
		Short: "Move agent threads/sessions between machines",
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

	return cmd
}
