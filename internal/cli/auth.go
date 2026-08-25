package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/config"
	"github.com/spf13/cobra"
)

type loginOptions struct {
	username       string
	password       string
	bypassCode     string
	noRemember     bool
	nonInteractive bool
}

func (a *App) loginCommand() *cobra.Command {
	options := loginOptions{}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and save a session for the selected profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runLogin(cmd, options)
		},
	}
	cmd.Flags().StringVarP(&options.username, "username", "u", "", "USC NetID (or USC_USERNAME)")
	cmd.Flags().StringVar(&options.password, "password", "", "USC password (prefer USC_PASSWORD)")
	cmd.Flags().StringVar(&options.bypassCode, "bypass-code", "", "Duo bypass code (or USC_DUO_BYPASS)")
	cmd.Flags().BoolVar(&options.noRemember, "no-remember", false, "do not save credentials after a successful login")
	cmd.Flags().BoolVar(&options.nonInteractive, "non-interactive", false, "fail instead of prompting for missing credentials")
	return cmd
}

func (a *App) runLogin(cmd *cobra.Command, options loginOptions) error {
	profileName, err := a.selectedProfile()
	if err != nil {
		return err
	}
	credentials, err := a.resolveCredentials(profileName, auth.Credentials{
		Username:   firstNonempty(options.username, os.Getenv("USC_USERNAME")),
		Password:   firstNonempty(options.password, os.Getenv("USC_PASSWORD")),
		BypassCode: firstNonempty(options.bypassCode, os.Getenv("USC_DUO_BYPASS")),
	})
	if err != nil {
		return err
	}
	if err := a.completeCredentials(&credentials, options.nonInteractive); err != nil {
		return err
	}
	if _, err := a.Store.EnsureProfile(profileName, credentials.Username); err != nil {
		return err
	}
	if _, err := a.Store.LoadConfig(); errors.Is(err, os.ErrNotExist) {
		if err := a.Store.SetCurrent(profileName); err != nil {
			return err
		}
	}

	result, err := a.Login(cmd.Context(), auth.DefaultTarget, a.Store.SessionPath(profileName), credentials)
	if err != nil {
		return fmt.Errorf("authenticate profile %q: %w", profileName, err)
	}
	if !options.noRemember {
		if err := a.Store.SaveCredentials(profileName, config.Credentials{
			Username: credentials.Username, Password: credentials.Password, BypassCode: credentials.BypassCode,
		}); err != nil {
			return fmt.Errorf("save credentials: %w", err)
		}
	}
	return a.writeJSON(map[string]any{
		"authenticated":   true,
		"profile":         profileName,
		"session_path":    a.Store.SessionPath(profileName),
		"status":          result.Status,
		"url":             result.URL,
		"reauthenticated": result.Reauthenticated,
	})
}

func (a *App) completeCredentials(credentials *auth.Credentials, nonInteractive bool) error {
	if nonInteractive {
		if names := missingCredentials(*credentials); len(names) != 0 {
			return fmt.Errorf("missing %v; set USC_USERNAME, USC_PASSWORD, and USC_DUO_BYPASS or remove --non-interactive", names)
		}
		return nil
	}
	for _, prompt := range []struct {
		label  string
		secret bool
		value  *string
	}{
		{label: "USC NetID: ", value: &credentials.Username},
		{label: "USC password: ", secret: true, value: &credentials.Password},
		{label: "Duo bypass code: ", secret: true, value: &credentials.BypassCode},
	} {
		if *prompt.value == "" {
			value, err := a.ask(prompt.label, prompt.secret)
			if err != nil {
				return err
			}
			*prompt.value = value
		}
	}
	if names := missingCredentials(*credentials); len(names) != 0 {
		return fmt.Errorf("missing %v", names)
	}
	return nil
}

func (a *App) resolveCredentials(profileName string, overrides auth.Credentials) (auth.Credentials, error) {
	profile, err := a.Store.LoadProfile(profileName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return auth.Credentials{}, err
	}
	saved, err := a.Store.LoadCredentials(profileName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return auth.Credentials{}, err
	}
	return auth.Credentials{
		Username:   firstNonempty(overrides.Username, saved.Username, profile.Username),
		Password:   firstNonempty(overrides.Password, saved.Password),
		BypassCode: firstNonempty(overrides.BypassCode, saved.BypassCode),
	}, nil
}

func missingCredentials(credentials auth.Credentials) []string {
	var missing []string
	if credentials.Username == "" {
		missing = append(missing, "username")
	}
	if credentials.Password == "" {
		missing = append(missing, "password")
	}
	if credentials.BypassCode == "" {
		missing = append(missing, "bypass code")
	}
	return missing
}

// ensureSession is the single entry point authenticated commands use. Login
// first tries the saved cookies and only runs SSO when the target redirects to
// USC login. If reauthentication is needed, it uses the profile's remembered
// credentials.
func (a *App) ensureSession(ctx context.Context, profileName string) (auth.Result, error) {
	credentials, err := a.resolveCredentials(profileName, auth.Credentials{})
	if err != nil {
		return auth.Result{}, err
	}
	if !fileExists(a.Store.SessionPath(profileName)) && !credentials.Complete() {
		return auth.Result{}, fmt.Errorf("profile %q: %w", profileName, auth.ErrCredentialsRequired)
	}
	result, err := a.Login(ctx, auth.DefaultTarget, a.Store.SessionPath(profileName), credentials)
	if err != nil {
		return auth.Result{}, fmt.Errorf("authenticate profile %q: %w", profileName, err)
	}
	if credentials.Complete() {
		if _, err := a.Store.EnsureProfile(profileName, credentials.Username); err != nil {
			return auth.Result{}, err
		}
		if err := a.Store.SaveCredentials(profileName, config.Credentials{
			Username: credentials.Username, Password: credentials.Password, BypassCode: credentials.BypassCode,
		}); err != nil {
			return auth.Result{}, err
		}
	}
	return result, nil
}

func (a *App) statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Verify the saved session for the selected profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, err := a.selectedProfile()
			if err != nil {
				return err
			}
			sessionPath := a.Store.SessionPath(profile)
			result, err := a.ensureSession(cmd.Context(), profile)
			if err != nil {
				return err
			}
			return a.writeJSON(map[string]any{
				"authenticated": true, "profile": profile, "reauthenticated": result.Reauthenticated,
				"status": result.Status, "url": result.URL, "session_path": sessionPath,
			})
		},
	}
}

func (a *App) bypassCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "bypass [CODE]",
		Short: "Replace the selected profile's Duo bypass code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profileName, err := a.selectedProfile()
			if err != nil {
				return err
			}
			credentials, err := a.resolveCredentials(profileName, auth.Credentials{})
			if err != nil {
				return err
			}
			code := ""
			if len(args) == 1 {
				code = args[0]
			}
			if code == "" {
				code, err = a.ask("New Duo bypass code: ", true)
				if err != nil {
					return err
				}
			}
			if credentials.Username == "" || credentials.Password == "" {
				return fmt.Errorf("profile %q: %w", profileName, auth.ErrCredentialsRequired)
			}
			if code == "" {
				return errors.New("duo bypass code cannot be empty")
			}
			credentials.BypassCode = code
			if _, err := a.Store.EnsureProfile(profileName, credentials.Username); err != nil {
				return err
			}
			if err := a.Store.SaveCredentials(profileName, config.Credentials{
				Username: credentials.Username, Password: credentials.Password, BypassCode: credentials.BypassCode,
			}); err != nil {
				return err
			}
			return a.writeJSON(map[string]any{"updated": true, "profile": profileName})
		},
	}
}

func (a *App) logoutCommand() *cobra.Command {
	var forget bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved session for the selected profile",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			profile, err := a.selectedProfile()
			if err != nil {
				return err
			}
			if err := a.Store.ClearSession(profile); err != nil {
				return err
			}
			if forget {
				if err := a.Store.ForgetCredentials(profile); err != nil {
					return err
				}
			}
			return a.writeJSON(map[string]any{"logged_out": true, "profile": profile, "credentials_forgotten": forget})
		},
	}
	cmd.Flags().BoolVar(&forget, "forget", false, "also remove remembered credentials")
	return cmd
}
