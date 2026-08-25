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
	profile, profileErr := a.Store.LoadProfile(profileName)
	if profileErr != nil && !errors.Is(profileErr, os.ErrNotExist) {
		return profileErr
	}
	saved, savedErr := a.Store.LoadCredentials(profileName)
	if savedErr != nil && !errors.Is(savedErr, os.ErrNotExist) {
		return savedErr
	}

	credentials := auth.Credentials{
		Username:   firstNonempty(options.username, os.Getenv("USC_USERNAME"), saved.Username, profile.Username),
		Password:   firstNonempty(options.password, os.Getenv("USC_PASSWORD"), saved.Password),
		BypassCode: firstNonempty(options.bypassCode, os.Getenv("USC_DUO_BYPASS"), saved.BypassCode),
	}
	if err := a.completeCredentials(&credentials, options.nonInteractive); err != nil {
		return err
	}
	profile, err = a.Store.EnsureProfile(profileName, credentials.Username)
	if err != nil {
		return err
	}
	if _, err := a.Store.LoadConfig(); errors.Is(err, os.ErrNotExist) {
		if err := a.Store.SetCurrent(profileName); err != nil {
			return err
		}
	}

	result, err := a.Login(cmd.Context(), auth.DefaultTarget, a.Store.SessionPath(profileName), credentials)
	if err != nil {
		return authenticationError(profileName, fmt.Errorf("authenticate profile %q: %w", profileName, err))
	}
	if !options.noRemember {
		if err := a.Store.SaveCredentials(profileName, config.Credentials{
			Username: credentials.Username, Password: credentials.Password, BypassCode: credentials.BypassCode,
		}); err != nil {
			return fmt.Errorf("save credentials: %w", err)
		}
	}
	if a.format == "human" {
		_, err = fmt.Fprintf(a.Out, "Logged in as %s (profile %s).\nSession: %s\n", profile.Username, profile.Name, a.Store.SessionPath(profileName))
		return err
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
	missing := func() []string {
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
		return names
	}
	if nonInteractive {
		if names := missing(); len(names) != 0 {
			return fmt.Errorf("missing %v; set USC_USERNAME, USC_PASSWORD, and USC_DUO_BYPASS or remove --non-interactive", names)
		}
		return nil
	}
	var err error
	if credentials.Username == "" {
		credentials.Username, err = a.ask("USC NetID: ", false)
		if err != nil {
			return err
		}
	}
	if credentials.Password == "" {
		credentials.Password, err = a.ask("USC password: ", true)
		if err != nil {
			return err
		}
	}
	if credentials.BypassCode == "" {
		credentials.BypassCode, err = a.ask("Duo bypass code: ", true)
		if err != nil {
			return err
		}
	}
	if names := missing(); len(names) != 0 {
		return fmt.Errorf("missing %v", names)
	}
	return nil
}

func (a *App) automaticCredentials(profileName string) (auth.Credentials, error) {
	profile, err := a.Store.LoadProfile(profileName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return auth.Credentials{}, err
	}
	saved, err := a.Store.LoadCredentials(profileName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return auth.Credentials{}, err
	}
	return auth.Credentials{
		Username:   firstNonempty(os.Getenv("USC_USERNAME"), saved.Username, profile.Username),
		Password:   firstNonempty(os.Getenv("USC_PASSWORD"), saved.Password),
		BypassCode: firstNonempty(os.Getenv("USC_DUO_BYPASS"), saved.BypassCode),
	}, nil
}

func credentialsComplete(credentials auth.Credentials) bool {
	return credentials.Username != "" && credentials.Password != "" && credentials.BypassCode != ""
}

// ensureSession is the single entry point authenticated commands use. Login
// first tries the saved cookies and only runs SSO when the target redirects to
// USC login. Environment values override remembered credentials and are saved
// after success, which makes Duo bypass rotation self-healing as well.
func (a *App) ensureSession(ctx context.Context, profileName string) (auth.Result, error) {
	credentials, err := a.automaticCredentials(profileName)
	if err != nil {
		return auth.Result{}, err
	}
	if !fileExists(a.Store.SessionPath(profileName)) && !credentialsComplete(credentials) {
		return auth.Result{}, authenticationError(profileName, auth.ErrCredentialsRequired)
	}
	result, err := a.Login(ctx, auth.DefaultTarget, a.Store.SessionPath(profileName), credentials)
	if err != nil {
		return auth.Result{}, authenticationError(profileName, fmt.Errorf("authenticate profile %q: %w", profileName, err))
	}
	if credentialsComplete(credentials) {
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
			if a.format == "human" {
				verb := "authenticated"
				if result.Reauthenticated {
					verb = "reauthenticated"
				}
				_, err = fmt.Fprintf(a.Out, "Profile %s is %s.\n", profile, verb)
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
			credentials, err := a.automaticCredentials(profileName)
			if err != nil {
				return err
			}
			code := os.Getenv("USC_DUO_BYPASS")
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
				return authenticationError(profileName, auth.ErrCredentialsRequired)
			}
			if code == "" {
				return errors.New("Duo bypass code cannot be empty")
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
			if a.format == "human" {
				_, err = fmt.Fprintf(a.Out, "Updated Duo bypass code for profile %s.\n", profileName)
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
			if a.format == "human" {
				_, err = fmt.Fprintf(a.Out, "Logged out profile %s.\n", profile)
				return err
			}
			return a.writeJSON(map[string]any{"logged_out": true, "profile": profile, "credentials_forgotten": forget})
		},
	}
	cmd.Flags().BoolVar(&forget, "forget", false, "also remove remembered credentials")
	return cmd
}
