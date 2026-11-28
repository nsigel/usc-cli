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
	"github.com/nsigel/usc-cli/internal/weblogin"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
	root.AddCommand(duoCommand(), authCommand(), bypassCommand(), brightspaceCommand(), handshakeCommand(), libcalCommand(), webregCommand(), directoryCommand(), classesCommand(), mobileorderCommand(), skillCommand(), versionCommand())
	return root
}

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

type errorInfo struct {
	Methods []auth.DuoMethod `json:"methods,omitempty"`
	Error   string           `json:"error"`
	Code    string           `json:"code,omitempty"`
	Action  string           `json:"action,omitempty"`
	Missing []string         `json:"missing,omitempty"`
	URL     string           `json:"url,omitempty"`
}

func WriteError(cmd *cobra.Command, err error) error {
	info := errorInfo{Error: err.Error()}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		info.Code = coded.ErrorCode()
	}
	var siteErr *auth.SiteError
	switch {
	case errors.As(err, &siteErr):
		info.Action = "usc auth login"
	case errors.Is(err, weblogin.ErrPageClosed):
		info.Code = "sign_in_page_closed"
		info.Action = "usc auth login"
	case info.Code == "libcal_booking_unknown", info.Code == "libcal_cleanup_failed":
		info.Action = "usc libcal release"
	case info.Code == "libcal_invalid_details":
		info.Action = "usc libcal book --field NAME=VALUE"
	}
	var methods *auth.DuoMethodsRequired
	if errors.As(err, &methods) {
		info.Methods = methods.Methods
	}
	var required *auth.SignInRequired
	if errors.As(err, &required) {
		info.Missing = required.Missing
		for _, field := range required.Missing {
			if field == "USC_DUO_BYPASS" {
				info.URL = "https://account.usc.edu/2fa/duo-bypass-code"
			}
		}
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

func positiveID(value, label string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", label)
	}
	return id, nil
}
