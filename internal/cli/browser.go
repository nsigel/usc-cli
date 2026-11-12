package cli

import (
	"github.com/nsigel/usc-cli/browser"
	"github.com/spf13/cobra"
)

func browserCommand() *cobra.Command {
	return group("browser", "Browser session helpers", browserSyncCommand())
}

func browserSyncCommand() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Copy saved USC session cookies into Chrome via CDP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			result, err := browser.SyncCookies(cmd.Context(), options.SessionFile, target)
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&target, "cdp", browser.DefaultTarget, "Chrome DevTools target: port, host:port, http(s) URL, or ws URL")
	return cmd
}
