package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/brightspace"
	"github.com/nsigel/usc-cli/internal/config"
	"github.com/nsigel/usc-cli/internal/handshake"
	"github.com/nsigel/usc-cli/internal/site"
	libcalclient "github.com/nsigel/usc-cli/libcal"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage the shared USC session"}
	cmd.AddCommand(loginCommand(), statusCommand(), logoutCommand(), marshallAuthCommand())
	return cmd
}

func loginCommand() *cobra.Command {
	var username string
	var nonInteractive bool
	var fresh bool
	cmd := &cobra.Command{
		Use:   "login [site]",
		Short: "Sign in through USC SSO",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Password and MFA values intentionally have no flags: process listings
			// and shell histories routinely expose command-line arguments.
			savedCredentials, err := loadCredentials()
			if err != nil {
				return err
			}
			credentials := auth.Credentials{
				Username:   first(username, os.Getenv("USC_USERNAME"), savedCredentials.Username),
				Password:   first(os.Getenv("USC_PASSWORD"), savedCredentials.Password),
				BypassCode: first(os.Getenv("USC_DUO_BYPASS"), savedCredentials.BypassCode),
			}
			usingSavedBypassCode := credentials.BypassCode != "" &&
				os.Getenv("USC_DUO_BYPASS") == "" && savedCredentials.BypassCode != ""
			path, err := config.SessionPath()
			if err != nil {
				return err
			}
			target, err := authenticationTarget(args)
			if err != nil {
				return err
			}
			err = loginTarget(cmd.Context(), target, path, credentials, fresh)
			if errors.Is(err, auth.ErrCredentialsRequired) {
				if err := completeCredentials(cmd, &credentials, nonInteractive); err != nil {
					return err
				}
				err = loginTarget(cmd.Context(), target, path, credentials, fresh)
			}
			if errors.Is(err, auth.ErrBypassRejected) && usingSavedBypassCode && !nonInteractive {
				// A bypass code can expire. Keep the stored value until a replacement
				// succeeds, but let an interactive login replace it immediately.
				credentials.BypassCode = ""
				if err := completeCredentials(cmd, &credentials, false); err != nil {
					return err
				}
				err = loginTarget(cmd.Context(), target, path, credentials, fresh)
			}
			if err != nil {
				return err
			}
			if credentials.Complete() {
				if err := saveCredentials(credentials); err != nil {
					return err
				}
			}
			return writeJSON(cmd, map[string]bool{"authenticated": true})
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "USC NetID (or USC_USERNAME)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "fail instead of prompting")
	cmd.Flags().BoolVar(&fresh, "fresh", false, "ignore the saved session and run a new login")
	return cmd
}

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [site]",
		Short: "Check the saved USC session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.SessionPath()
			if err != nil {
				return err
			}
			target, err := authenticationTarget(args)
			if err != nil {
				return err
			}
			// Empty credentials may complete a silent SSO handshake, but they can
			// never submit secrets or perform an interactive login.
			err = loginTarget(cmd.Context(), target, path, auth.Credentials{}, false)
			if errors.Is(err, auth.ErrCredentialsRequired) {
				// Being signed out is a valid status result, not a command failure.
				return writeJSON(cmd, map[string]bool{"authenticated": false})
			}
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]bool{"authenticated": true})
		},
	}
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the saved USC session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.SessionPath()
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

func authenticationTarget(args []string) (string, error) {
	name := site.Brightspace
	if len(args) == 1 {
		name = site.Name(args[0])
	}
	selected, err := site.Find(name)
	if err != nil {
		return "", err
	}
	if selected.LoginURL == "" {
		return "", fmt.Errorf("site %q does not use USC authentication", name)
	}
	return selected.LoginURL, nil
}

func loginTarget(ctx context.Context, target, sessionFile string, credentials auth.Credentials, fresh bool) error {
	if target == site.LibCalLoginURL {
		return libcalclient.Authenticate(ctx, sessionFile, credentials, fresh)
	}
	login := auth.Login
	if fresh {
		login = auth.LoginFresh
	}
	return login(ctx, target, sessionFile, credentials)
}

type savedCredentials struct {
	Username   string                    `json:"username"`
	Password   string                    `json:"password"`
	BypassCode string                    `json:"bypass_code"`
	Marshall   *savedMarshallCredentials `json:"marshall,omitempty"`
}

type savedMarshallCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func loadCredentials() (savedCredentials, error) {
	path, err := config.CredentialsPath()
	if err != nil {
		return savedCredentials{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return savedCredentials{}, nil
	}
	if err != nil {
		return savedCredentials{}, fmt.Errorf("read saved credentials: %w", err)
	}
	var credentials savedCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return savedCredentials{}, fmt.Errorf("read saved credentials: %w", err)
	}
	return credentials, nil
}

func saveCredentials(credentials auth.Credentials) error {
	saved, err := loadCredentials()
	if err != nil {
		return err
	}
	saved.Username = credentials.Username
	saved.Password = credentials.Password
	saved.BypassCode = credentials.BypassCode
	return writeCredentials(saved)
}

func saveMarshallCredentials(email, password string) error {
	saved, err := loadCredentials()
	if err != nil {
		return err
	}
	saved.Marshall = &savedMarshallCredentials{Email: email, Password: password}
	return writeCredentials(saved)
}

func writeCredentials(credentials savedCredentials) error {
	path, err := config.CredentialsPath()
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".credentials-*")
	if err != nil {
		return fmt.Errorf("create saved credentials: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure saved credentials: %w", err)
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return fmt.Errorf("write saved credentials: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close saved credentials: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	return nil
}

func marshallAuthCommand() *cobra.Command {
	var email string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "marshall [EMAIL]",
		Short: "Save Marshall EMS credentials",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			saved, err := loadCredentials()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				email = first(email, args[0])
			}
			savedEmail, savedPassword := "", ""
			if saved.Marshall != nil {
				savedEmail = saved.Marshall.Email
				savedPassword = saved.Marshall.Password
			}
			email = first(email, os.Getenv("USC_MARSHALL_EMAIL"), savedEmail)
			password := first(os.Getenv("USC_MARSHALL_PASSWORD"), savedPassword, os.Getenv("USC_PASSWORD"), saved.Password)

			if email == "" {
				if nonInteractive {
					return errors.New("Marshall email is required; set USC_MARSHALL_EMAIL or pass the email address")
				}
				email, err = prompt(cmd, bufio.NewReader(cmd.InOrStdin()), "Marshall email (user@marshall.usc.edu): ", false)
				if err != nil {
					return err
				}
			}
			email = strings.TrimSpace(email)
			if !strings.HasSuffix(strings.ToLower(email), "@marshall.usc.edu") || len(email) == len("@marshall.usc.edu") {
				return errors.New("Marshall email must end in @marshall.usc.edu")
			}
			if password == "" {
				if nonInteractive {
					return errors.New("Marshall password is required; set USC_MARSHALL_PASSWORD or run usc auth marshall interactively")
				}
				password, err = prompt(cmd, bufio.NewReader(cmd.InOrStdin()), "Marshall password: ", true)
				if err != nil {
					return err
				}
			}
			if err := saveMarshallCredentials(email, password); err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"credentials_saved": true, "email": email})
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "Marshall email address (or USC_MARSHALL_EMAIL)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "fail instead of prompting")
	return cmd
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

type errorPayload struct {
	Error  string `json:"error"`
	Action string `json:"action,omitempty"`
}

// ErrorPayload returns the stable JSON representation of a command error.
func ErrorPayload(err error) errorPayload {
	payload := errorPayload{Error: err.Error()}
	if action := errorAction(err); action != "" {
		payload.Action = action
	} else if errors.Is(err, handshake.ErrSessionInvalid) {
		payload.Action = "usc auth login handshake"
	} else if errors.Is(err, libcalclient.ErrAuthenticationRequired) {
		payload.Action = "usc auth login libcal"
	} else if errors.Is(err, auth.ErrBypassRejected) ||
		errors.Is(err, auth.ErrCredentialsRequired) ||
		errors.Is(err, brightspace.ErrSessionInvalid) {
		payload.Action = "usc auth login"
	}
	return payload
}
