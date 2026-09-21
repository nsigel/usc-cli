package cli

import (
	"errors"
	"os"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/browser"
	"github.com/spf13/cobra"
)

func browserCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser",
		Short: "Browser session helpers",
	}
	cmd.AddCommand(browserSyncCommand())
	return cmd
}

func browserSyncCommand() *cobra.Command {
	var cdpTarget string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Copy saved USC session cookies into Chrome via CDP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := sessionPath()
			if err != nil {
				return err
			}
			target := first(cdpTarget, os.Getenv("USC_CDP"), browser.DefaultTarget())
			result, err := browser.SyncCookies(cmd.Context(), path, target)
			if err != nil {
				if errors.Is(err, auth.ErrSessionNotFound) {
					return withAction(err, "usc auth login")
				}
				if errors.Is(err, browser.ErrCDPUnavailable) {
					return withAction(err, "start Chrome with --remote-debugging-port=9222 (or 9224)")
				}
				return err
			}
			return writeJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&cdpTarget, "cdp", "", "Chrome DevTools target: port, host:port, http(s) URL, or ws URL (or USC_CDP; default 127.0.0.1:9222)")
	return cmd
}
