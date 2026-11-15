package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func authCommand() *cobra.Command {
	return group("auth", "Manage the saved USC sign-in", loginCommand(), statusCommand(), logoutCommand())
}

// signInProbe is the site auth commands open to run or check the USC sign-in.
// Any site would do: one sign-in covers them all.
const signInProbe = site.Brightspace

func loginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in to USC and save the credentials for later commands",
		Long: "Sign in to USC and save the credentials for later commands.\n\n" +
			"Prompts for whatever is missing among NetID, password, and Duo bypass code.\n" +
			"Without a terminal it reads USC_USERNAME, USC_PASSWORD, and USC_DUO_BYPASS instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			if options.Credentials, err = options.ResolveCredentials(); err != nil {
				return err
			}
			if !options.Credentials.Complete() {
				if err := promptForMissing(cmd, &options.Credentials); err != nil {
					return err
				}
				// Only a real sign-in tests new credentials, and a session that is
				// still live would skip it.
				if err := os.Remove(options.SessionFile); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			if _, err := auth.Open(cmd.Context(), signInProbe, options); err != nil {
				return err
			}
			if err := auth.SaveCredentials(options.CredentialsFile, options.Credentials); err != nil {
				return err
			}
			return writeJSON(cmd, map[string]bool{"authenticated": true})
		},
	}
}

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check the saved USC session and credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			credentials, err := options.ResolveCredentials()
			if err != nil {
				return err
			}
			_, err = auth.Open(cmd.Context(), signInProbe, auth.Options{SessionFile: options.SessionFile})
			if err != nil && !errors.Is(err, auth.ErrCredentialsRequired) {
				return err
			}
			return writeJSON(cmd, map[string]bool{"authenticated": err == nil, "credentials_saved": credentials.Complete()})
		},
	}
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the saved USC session and credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			for _, file := range []string{options.SessionFile, options.CredentialsFile} {
				if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			return writeJSON(cmd, map[string]bool{"logged_out": true})
		},
	}
}

func promptForMissing(cmd *cobra.Command, credentials *auth.Credentials) error {
	fields := []struct {
		name, label string
		secret      bool
		value       *string
	}{
		{"USC_USERNAME", "USC NetID: ", false, &credentials.Username},
		{"USC_PASSWORD", "USC password: ", true, &credentials.Password},
		{"USC_DUO_BYPASS", "Duo bypass code: ", false, &credentials.BypassCode},
	}
	input, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		var missing []string
		for _, field := range fields {
			if *field.value == "" {
				missing = append(missing, field.name)
			}
		}
		return auth.ForSite(signInProbe, fmt.Errorf("%w: run usc auth login in a terminal, or set %s", auth.ErrCredentialsRequired, strings.Join(missing, ", ")))
	}
	reader := bufio.NewReader(input)
	for _, field := range fields {
		for *field.value == "" {
			fmt.Fprint(cmd.ErrOrStderr(), field.label)
			if field.secret {
				secret, err := term.ReadPassword(int(input.Fd()))
				fmt.Fprintln(cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				*field.value = strings.TrimSpace(string(secret))
				continue
			}
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			*field.value = strings.TrimSpace(line)
		}
	}
	return nil
}
