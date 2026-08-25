package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/site"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage the shared USC session"}
	cmd.AddCommand(loginCommand(), statusCommand(), logoutCommand())
	return cmd
}

func loginCommand() *cobra.Command {
	var username string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "login [site]",
		Short: "Sign in through USC SSO",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := authSite(args)
			if err != nil {
				return err
			}
			// Password and MFA values intentionally have no flags: process listings
			// and shell histories routinely expose command-line arguments.
			credentials := auth.Credentials{
				Username:   first(username, os.Getenv("USC_USERNAME")),
				Password:   os.Getenv("USC_PASSWORD"),
				BypassCode: os.Getenv("USC_DUO_BYPASS"),
			}
			if err := completeCredentials(cmd, &credentials, nonInteractive); err != nil {
				return err
			}
			path, err := sessionPath()
			if err != nil {
				return err
			}
			result, err := auth.Login(cmd.Context(), selected.LoginURL, path, credentials)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{
				"authenticated":   true,
				"reauthenticated": result.Reauthenticated,
				"site":            selected.Name,
				"status":          result.Status,
				"url":             result.URL,
			})
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "USC NetID (or USC_USERNAME)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "fail instead of prompting")
	return cmd
}

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [site]",
		Short: "Check the saved USC session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := authSite(args)
			if err != nil {
				return err
			}
			path, err := sessionPath()
			if err != nil {
				return err
			}
			// Empty credentials make status observational: it may validate cookies,
			// but it can never silently perform a fresh login.
			result, err := auth.Login(cmd.Context(), selected.LoginURL, path, auth.Credentials{})
			if errors.Is(err, auth.ErrCredentialsRequired) {
				// Being signed out is a valid status result, not a command failure.
				return writeJSON(cmd, map[string]any{"authenticated": false, "site": selected.Name})
			}
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{
				"authenticated": true,
				"site":          selected.Name,
				"status":        result.Status,
				"url":           result.URL,
			})
		},
	}
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the saved USC session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := sessionPath()
			if err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return writeJSON(cmd, map[string]bool{"logged_out": true})
		},
	}
}

func authSite(args []string) (site.Site, error) {
	// WebReg is the default because its Entra OIDC route exercises USC's primary
	// SSO path without tying authentication to a course or advising role.
	name := site.WebReg
	if len(args) == 1 {
		name = site.Name(args[0])
	}
	selected, err := site.Find(name)
	if err != nil {
		return site.Site{}, err
	}
	if selected.Login == site.Legacy {
		return site.Site{}, fmt.Errorf("%s does not use USC SSO", selected.Name)
	}
	return selected, nil
}

func sessionPath() (string, error) {
	// A single cross-domain jar mirrors browser SSO and avoids fake per-site
	// sessions that would duplicate identity-provider cookies.
	root := os.Getenv("USC_CONFIG_DIR")
	if root == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("find config directory: %w", err)
		}
		root = filepath.Join(config, "usc")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(absolute, "session.json"), nil
}

func completeCredentials(cmd *cobra.Command, credentials *auth.Credentials, nonInteractive bool) error {
	missing := func() string {
		var names []string
		if credentials.Username == "" {
			names = append(names, "username")
		}
		if credentials.Password == "" {
			names = append(names, "password")
		}
		if credentials.BypassCode == "" {
			names = append(names, "bypass code")
		}
		return strings.Join(names, ", ")
	}
	if nonInteractive {
		if names := missing(); names != "" {
			return fmt.Errorf("%w: missing %s", auth.ErrCredentialsRequired, names)
		}
		return nil
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	for _, item := range []struct {
		label  string
		secret bool
		value  *string
	}{
		{label: "USC NetID: ", value: &credentials.Username},
		{label: "USC password: ", secret: true, value: &credentials.Password},
		{label: "Duo bypass code: ", secret: true, value: &credentials.BypassCode},
	} {
		if *item.value == "" {
			value, err := prompt(cmd, reader, item.label, item.secret)
			if err != nil {
				return err
			}
			*item.value = value
		}
	}
	if names := missing(); names != "" {
		return fmt.Errorf("%w: missing %s", auth.ErrCredentialsRequired, names)
	}
	return nil
}

func prompt(cmd *cobra.Command, reader *bufio.Reader, label string, secret bool) (string, error) {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), label); err != nil {
		return "", err
	}
	if input, ok := cmd.InOrStdin().(*os.File); secret && ok && term.IsTerminal(int(input.Fd())) {
		value, err := term.ReadPassword(int(input.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		return strings.TrimSpace(string(value)), err
	}
	value, err := reader.ReadString('\n')
	if err != nil && value == "" {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ErrorPayload returns the stable JSON representation of a command error.
func ErrorPayload(err error) map[string]any {
	payload := map[string]any{"error": err.Error()}
	if errors.Is(err, auth.ErrBypassRejected) {
		payload["code"] = "duo_bypass_invalid"
		payload["action"] = "usc auth login"
	} else if errors.Is(err, auth.ErrCredentialsRequired) {
		payload["code"] = "credentials_required"
		payload["action"] = "usc auth login"
	}
	return payload
}
