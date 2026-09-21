// Package cli defines the command-line boundary for usc.
package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/nsigel/usc-cli/internal/site"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type actionableError struct {
	err    error
	action string
}

func (e *actionableError) Error() string { return e.err.Error() }
func (e *actionableError) Unwrap() error { return e.err }

func withAction(err error, action string) error {
	return &actionableError{err: err, action: action}
}

func errorAction(err error) string {
	var actionable *actionableError
	if errors.As(err, &actionable) {
		return actionable.action
	}
	return ""
}

// New returns a configured usc root command.
func New(version string) *cobra.Command {
	root := &cobra.Command{
		Use:               "usc",
		Short:             "USC services from your terminal",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.PersistentFlags().Bool("json", false, "emit compact JSON")
	root.PersistentFlags().Bool("pretty", false, "pretty-print JSON")
	root.MarkFlagsMutuallyExclusive("json", "pretty")
	root.AddCommand(authCommand(), browserCommand(), brightspaceCommand(), handshakeCommand(), classesCommand(), sitesCommand(), skillCommand(), versionCommand(version))
	return root
}

func sitesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sites [name]",
		Short: "List supported USC sites",
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
	writer := cmd.OutOrStdout()
	return encodeJSON(writer, value, prettyOutput(cmd, writer))
}

// WriteError writes a command error using the command's configured JSON format.
func WriteError(cmd *cobra.Command, err error) error {
	writer := cmd.ErrOrStderr()
	return encodeJSON(writer, ErrorPayload(err), prettyOutput(cmd, writer))
}

func encodeJSON(writer io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(writer)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

func prettyOutput(cmd *cobra.Command, writer io.Writer) bool {
	pretty, _ := cmd.Flags().GetBool("pretty")
	if pretty {
		return true
	}
	compact, _ := cmd.Flags().GetBool("json")
	if compact {
		return false
	}
	return isTerminal(writer)
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
