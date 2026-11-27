package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	"github.com/nsigel/usc-cli/weblogin"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func authCommand() *cobra.Command {
	return group("auth", "Manage the saved USC sign-in", loginCommand(), statusCommand(), logoutCommand())
}

const signInProbe = site.Brightspace

func loginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to USC and save the credentials for later commands",
		Long: "Sign in to USC and save the credentials for later commands.\n\n" +
			"With --web, print the URL of a local sign-in page, then run\n" +
			"usc auth status --wait URL.\n\n" +
			"Without --web, the first login lists Duo methods. Rerun with --push-device,\n" +
			"--mobile-code, or --sms-device to add a Duo device, or --bypass to sign in\n" +
			"with a bypass code instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if serve, _ := cmd.Flags().GetBool("web-serve"); serve {
				return serveWebLogin(cmd)
			}
			if web, _ := cmd.Flags().GetBool("web"); web {
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
	cmd.Flags().Bool("web-serve", false, "Serve the sign-in page in the foreground")
	_ = cmd.Flags().MarkHidden("web-serve")
	cmd.MarkFlagsMutuallyExclusive("mobile-code", "push-device", "sms-device", "bypass", "web", "web-serve")
	return cmd
}

func statusCommand() *cobra.Command {
	cmd := &cobra.Command{
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
			if page, _ := cmd.Flags().GetString("wait"); page != "" {
				if err := weblogin.Wait(cmd.Context(), page); err != nil {
					return err
				}
				if credentials, err = options.ResolveCredentials(); err != nil {
					return err
				}
			}
			_, err = auth.Open(cmd.Context(), signInProbe, auth.Options{SessionFile: options.SessionFile})
			if err != nil && !errors.Is(err, auth.ErrCredentialsRequired) {
				return err
			}
			return writeJSON(cmd, map[string]bool{"authenticated": err == nil, "credentials_saved": credentials.Complete()})
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
