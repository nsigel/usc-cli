// Package cli defines the command-line boundary for usc.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"

	"github.com/nsigel/usc-cli/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// New returns a configured usc root command.
func New() *cobra.Command {
	root := &cobra.Command{
		Use:               "usc",
		Short:             "USC services from your terminal",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.PersistentFlags().Bool("compact", false, "emit single-line JSON")
	root.PersistentFlags().Bool("pretty", false, "pretty-print JSON")
	root.MarkFlagsMutuallyExclusive("compact", "pretty")
	root.AddCommand(authCommand(), browserCommand(), brightspaceCommand(), handshakeCommand(), libcalCommand(), webregCommand(), classesCommand(), skillCommand(), versionCommand())
	return root
}

// group is a command that only holds subcommands. Cobra would otherwise print
// help and exit 0 for a mistyped subcommand.
func group(use, short string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use: use, Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(children...)
	return cmd
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version this binary was built from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeJSON(cmd, buildVersion())
		},
	}
}

// buildVersion reads what the Go toolchain recorded at build time: the module
// version for go install, and the commit for a build from a checkout.
func buildVersion() map[string]any {
	version := map[string]any{"version": "unknown"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	version["version"] = info.Main.Version
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			version["commit"] = setting.Value
		case "vcs.modified":
			version["modified"] = setting.Value == "true"
		}
	}
	return version
}

func writeJSON(cmd *cobra.Command, value any) error {
	writer := cmd.OutOrStdout()
	return encodeJSON(writer, value, prettyOutput(cmd, writer))
}

// errorInfo is the JSON written to stderr. Action is the usc command that
// resolves the error, when there is one.
type errorInfo struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Action string `json:"action,omitempty"`
}

// WriteError writes a command error as JSON in the command's output format.
func WriteError(cmd *cobra.Command, err error) error {
	info := errorInfo{Error: err.Error()}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		info.Code = coded.ErrorCode()
	}
	// Every sign-in problem has the same fix, whichever site reported it.
	var siteErr *auth.SiteError
	if errors.As(err, &siteErr) || errors.Is(err, auth.ErrSessionNotFound) {
		info.Action = "usc auth login"
	}
	writer := cmd.ErrOrStderr()
	return encodeJSON(writer, info, prettyOutput(cmd, writer))
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
	compact, _ := cmd.Flags().GetBool("compact")
	if compact {
		return false
	}
	return isTerminal(writer)
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// positiveID parses a numeric identifier argument; label names it in errors.
func positiveID(value, label string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", label)
	}
	return id, nil
}
