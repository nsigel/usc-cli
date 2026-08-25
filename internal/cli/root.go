// Package cli defines the command-line boundary for usc.
package cli

import (
	"encoding/json"

	"github.com/nsigel/usc-cli/internal/site"
	"github.com/spf13/cobra"
)

// New returns a configured usc root command.
func New(version string) *cobra.Command {
	root := &cobra.Command{
		Use:               "usc",
		Short:             "USC services from your terminal",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.AddCommand(authCommand(), brightspaceCommand(), sitesCommand(), versionCommand(version))
	return root
}

func sitesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sites [name]",
		Short: "List USC sites and their authentication entry points",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return writeJSON(cmd, site.All())
			}
			selected, err := site.Find(site.Name(args[0]))
			if err != nil {
				return err
			}
			return writeJSON(cmd, selected)
		},
	}
}

func versionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeJSON(cmd, map[string]string{"version": version})
		},
	}
}

func writeJSON(cmd *cobra.Command, value any) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
}
