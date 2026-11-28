package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/internal/weblogin"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func authCommand() *cobra.Command {
	return group("auth", "Manage the saved USC sign-in", loginCommand(), statusCommand(), logoutCommand())
}

const signInProbe = auth.SiteBrightspace

func loginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to USC and save the credentials for later commands",
		Long: "Sign in to USC and save credentials for later commands.\n\n" +
			"Browser sign-in (and terminal sign-in unless --bypass is used) enrolls a\n" +
			"Duo TOTP device (named with --device-name) so later commands authenticate\n" +
			"automatically. Subsequent sign-ins for the same NetID reuse the saved device.\n\n" +
			"Browser sign-in (recommended for humans and required for chat agents):\n" +
			"  --web     Start a local sign-in page on 127.0.0.1 (valid 10 minutes).\n" +
			"            Use on your own machine or when the user can control a local browser.\n" +
			"  --remote  Start an end-to-end encrypted sign-in page on usc.sh (valid 10 minutes).\n" +
			"            Use when the user's browser cannot reach this machine.\n" +
			"Then wait for completion with: usc auth status --wait URL\n\n" +
			"Terminal and CI sign-in (without --web or --remote):\n" +
			"  Prompts for NetID and password in an interactive terminal, or reads USC_*\n" +
			"  environment variables. The first run lists available Duo methods; rerun\n" +
			"  with --push-device, --mobile-code, or --sms-device to enroll a Duo device,\n" +
			"  or --bypass to sign in with a bypass code instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if serve, _ := cmd.Flags().GetBool("web-serve"); serve {
				return serveWebLogin(cmd)
			}
			web, _ := cmd.Flags().GetBool("web")
			if remote, _ := cmd.Flags().GetBool("remote"); web || remote {
				return startWebLogin(cmd)
			}
			options, err := signIn()
			if err != nil {
				return err
			}
			if options.Credentials, err = options.ResolveCredentials(); err != nil {
				return err
			}
			options.DuoDeviceName, _ = cmd.Flags().GetString("device-name")
			options.DuoSMSDevice, _ = cmd.Flags().GetString("sms-device")
			options.DuoPushDevice, _ = cmd.Flags().GetString("push-device")
			options.DuoPushStarted = func(code string) {
				_ = encodeJSON(cmd.ErrOrStderr(), map[string]string{"event": "duo_push_sent", "verification_code": code}, false)
			}
			mobile, _ := cmd.Flags().GetBool("mobile-code")
			if mobile || options.DuoSMSDevice != "" {
				input, ok := cmd.InOrStdin().(*os.File)
				if !ok || !term.IsTerminal(int(input.Fd())) {
					return errors.New("code login requires a terminal for secure input")
				}
				options.RequestDuoPasscode = func() (string, error) {
					label := "Fresh Duo Mobile code (wait for a new code after each approval): "
					if options.DuoSMSDevice != "" {
						label = "Enter the code just sent by SMS: "
					}
					fmt.Fprint(cmd.ErrOrStderr(), label)
					value, err := term.ReadPassword(int(input.Fd()))
					fmt.Fprintln(cmd.ErrOrStderr())
					return strings.TrimSpace(string(value)), err
				}
			}
			bypass, _ := cmd.Flags().GetBool("bypass")
			if bypass {
				options.Credentials.DuoTOTPSecret = ""
			}
			if options.Credentials.Username == "" || options.Credentials.Password == "" || (bypass && options.Credentials.BypassCode == "") {
				if err := promptForMissing(cmd, &options.Credentials, bypass); err != nil {
					return err
				}

				if err := os.Remove(options.SessionFile); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			if !bypass && options.Credentials.DuoTOTPSecret == "" {
				result, err := auth.CreateDuoDevice(cmd.Context(), options)
				if err != nil {
					return err
				}
				return writeJSON(cmd, result)
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
	cmd.Flags().String("device-name", "", "Name for the new Duo authenticator")
	cmd.Flags().String("sms-device", "", "Add a Duo device using codes texted to this phone ID")
	cmd.Flags().Bool("mobile-code", false, "Add a Duo device using Duo Mobile codes")
	cmd.Flags().String("push-device", "", "Add a Duo device using Duo Push on this device ID")
	cmd.Flags().Bool("bypass", false, "Sign in with a bypass code instead of adding a Duo device")
	cmd.Flags().Bool("web", false, "Print the URL of a local sign-in page and return")
	cmd.Flags().Bool("remote", false, "Print a usc.sh sign-in link that opens on any device and return")
	cmd.Flags().Bool("web-serve", false, "Serve the sign-in page in the foreground")
	_ = cmd.Flags().MarkHidden("web-serve")
	cmd.MarkFlagsMutuallyExclusive("mobile-code", "push-device", "sms-device", "bypass", "web", "web-serve")
	cmd.MarkFlagsMutuallyExclusive("mobile-code", "push-device", "sms-device", "bypass", "web", "remote")
	return cmd
}

type authStatus struct {
	Authenticated    bool   `json:"authenticated"`
	CredentialsSaved bool   `json:"credentials_saved"`
	DuoDeviceID      string `json:"duo_device_id,omitempty"`
}

func statusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check the saved USC session and credentials",
		Long: "Check whether the saved USC session is active and whether credentials are saved.\n\n" +
			"If the session cookies have expired but a Duo TOTP device is saved, status\n" +
			"refreshes the session automatically without prompting, and exits with the\n" +
			"sign-in error if USC or Duo rejects the saved credentials. With --wait URL, blocks\n" +
			"until the sign-in page finishes and returns an error if the page closes or\n" +
			"expires without completing sign-in.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			credentials, err := options.ResolveCredentials()
			if err != nil {
				return err
			}
			var waitErr error
			if page, _ := cmd.Flags().GetString("wait"); page != "" {
				waitErr = weblogin.Wait(cmd.Context(), page)
				if waitErr != nil && !errors.Is(waitErr, weblogin.ErrPageClosed) {
					return waitErr
				}
				if credentials, err = options.ResolveCredentials(); err != nil {
					return err
				}
			}
			_, err = auth.Open(cmd.Context(), signInProbe, auth.Options{SessionFile: options.SessionFile})
			// When session cookies have expired and a reusable Duo TOTP device is saved,
			// verify and refresh the session so status reflects whether commands will work.
			// Never spend a bypass code during a status check, and skip refresh if --wait
			// saw the sign-in page close without reporting completion.
			if errors.Is(err, auth.ErrCredentialsRequired) &&
				!errors.Is(waitErr, weblogin.ErrPageClosed) &&
				credentials.Username != "" && credentials.Password != "" && credentials.DuoTOTPSecret != "" {
				totp := credentials
				totp.BypassCode = ""
				_, err = auth.Open(cmd.Context(), signInProbe, auth.Options{
					Credentials:     totp,
					CredentialsFile: options.CredentialsFile,
					SessionFile:     options.SessionFile,
				})
			}
			if errors.Is(waitErr, weblogin.ErrPageClosed) {
				if err != nil {
					if errors.Is(err, auth.ErrCredentialsRequired) {
						return waitErr
					}
					return err
				}
			} else if err != nil && !errors.Is(err, auth.ErrCredentialsRequired) {
				return err
			}
			status := authStatus{
				Authenticated:    err == nil,
				CredentialsSaved: credentials.Complete(),
			}
			if options.Credentials.DuoTOTPSecret == "" {
				status.DuoDeviceID = credentials.DuoDeviceID
			}
			return writeJSON(cmd, status)
		},
	}
	cmd.Flags().String("wait", "", "Wait for the sign-in page at this URL to finish")
	return cmd
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

func promptForMissing(cmd *cobra.Command, credentials *auth.Credentials, bypass bool) error {
	fields := []struct {
		name, label string
		secret      bool
		value       *string
	}{
		{"USC_USERNAME", "USC NetID: ", false, &credentials.Username},
		{"USC_PASSWORD", "USC password: ", true, &credentials.Password},
		{"USC_DUO_BYPASS", "One-time Duo bypass code: ", true, &credentials.BypassCode},
	}
	if !bypass || credentials.DuoTOTPSecret != "" || credentials.BypassCode != "" {
		fields = fields[:2]
	}
	input, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		var missing []string
		for _, field := range fields {
			if *field.value == "" {
				missing = append(missing, field.name)
			}
		}
		return auth.ForSite(signInProbe, &auth.SignInRequired{Missing: missing})
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
