package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
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
			"  In an interactive terminal, prompts for NetID, password, and Duo method to\n" +
			"  enroll a Duo device in one pass (or pass --push-device, --mobile-code,\n" +
			"  --sms-device, or --bypass directly). Non-interactively, reads USC_*\n" +
			"  environment variables and supports --push-device or --bypass.",
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
			input, isFile := cmd.InOrStdin().(*os.File)
			interactive := isFile && term.IsTerminal(int(input.Fd()))
			var reader *bufio.Reader
			if interactive {
				reader = bufio.NewReader(input)
			}
			options.DuoDeviceName, _ = cmd.Flags().GetString("device-name")
			options.DuoSMSDevice, _ = cmd.Flags().GetString("sms-device")
			options.DuoPushDevice, _ = cmd.Flags().GetString("push-device")
			pushRound := 0
			humanPush := interactive && isTerminal(cmd.ErrOrStderr())
			options.DuoPushStarted = func(code string) {
				if !humanPush {
					_ = encodeJSON(cmd.ErrOrStderr(), map[string]string{"event": "duo_push_sent", "verification_code": code}, false)
					return
				}
				pushRound++
				message := "Approve Duo Push in Duo Mobile"
				if pushRound > 1 {
					message = "Approve once more in Duo Mobile to finish enrolling usc"
				}
				if code != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s (code: %s)...\n", message, code)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s...\n", message)
				}
			}
			mobile, _ := cmd.Flags().GetBool("mobile-code")
			if mobile || options.DuoSMSDevice != "" {
				if !interactive {
					return errors.New("code login requires a terminal for secure input")
				}
				options.RequestDuoPasscode = promptPasscode(cmd, input, options.DuoSMSDevice != "")
			}
			bypass, _ := cmd.Flags().GetBool("bypass")
			if bypass {
				options.Credentials.DuoTOTPSecret = ""
			}
			if options.Credentials.Username == "" || options.Credentials.Password == "" || (bypass && options.Credentials.BypassCode == "") {
				if err := promptForMissing(cmd, input, reader, interactive, &options.Credentials, bypass); err != nil {
					return err
				}
				if err := os.Remove(options.SessionFile); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			if !bypass && options.Credentials.DuoTOTPSecret == "" {
				explicitMethod := mobile || options.DuoPushDevice != "" || options.DuoSMSDevice != ""
				result, err := auth.CreateDuoDevice(cmd.Context(), options)
				var required *auth.DuoMethodsRequired
				if interactive && !explicitMethod && errors.As(err, &required) {
					if err := selectDuoMethod(cmd, input, reader, required.Methods, &options); err != nil {
						return err
					}
					result, err = auth.CreateDuoDevice(cmd.Context(), options)
				}
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
			"refreshes the session automatically without prompting and exits with an error\n" +
			"if USC or Duo rejects the saved credentials. With --wait URL, blocks until\n" +
			"the sign-in page finishes and returns an error if the page closes or expires\n" +
			"without completing sign-in.",
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

type logoutResult struct {
	LoggedOut     bool   `json:"logged_out"`
	DeviceDeleted bool   `json:"device_deleted,omitempty"`
	DuoDeviceID   string `json:"duo_device_id,omitempty"`
}

func logoutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Delete the saved USC session and credentials",
		Long: "Delete the saved local USC session and credentials.\n\n" +
			"With --delete-device, also deletes the saved CLI Duo device from your USC\n" +
			"account before removing local files.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			result := logoutResult{LoggedOut: true}
			if deleteDevice, _ := cmd.Flags().GetBool("delete-device"); deleteDevice {
				credentials, err := options.ResolveCredentials()
				if err != nil {
					return err
				}
				if credentials.DuoDeviceID == "" {
					return errors.New("no saved Duo device ID to delete")
				}
				if _, err := auth.DeleteDuoDevice(cmd.Context(), options, credentials.DuoDeviceID); err != nil {
					return fmt.Errorf("could not delete Duo device %s (%v); run usc auth logout without --delete-device to remove local files only", credentials.DuoDeviceID, err)
				}
				result.DeviceDeleted = true
				result.DuoDeviceID = credentials.DuoDeviceID
			}
			for _, file := range []string{options.SessionFile, options.CredentialsFile} {
				if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			return writeJSON(cmd, result)
		},
	}
	cmd.Flags().Bool("delete-device", false, "Delete the saved CLI Duo device from your USC account before logging out")
	return cmd
}

func promptPasscode(cmd *cobra.Command, input *os.File, sms bool) func() (string, error) {
	round := 0
	return func() (string, error) {
		round++
		label := "Duo Mobile code: "
		if sms {
			label = "Enter the code sent by SMS: "
			if round > 1 {
				label = "Enter the new code sent by SMS: "
			}
		} else if round > 1 {
			label = "New Duo Mobile code (wait for a new code): "
		}
		return readSecret(cmd, input, label)
	}
}

func readSecret(cmd *cobra.Command, input *os.File, label string) (string, error) {
	fmt.Fprint(cmd.ErrOrStderr(), label)
	value, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func selectDuoMethod(cmd *cobra.Command, input *os.File, reader *bufio.Reader, methods []auth.DuoMethod, options *auth.Options) error {
	type choice struct {
		kind     string
		deviceID string
		label    string
	}
	var choices []choice
	for _, kind := range []string{"push", "sms_otp"} {
		for _, method := range methods {
			if method.Type != kind || method.DeviceID == "" {
				continue
			}
			base := "Duo Push"
			if kind == "sms_otp" {
				base = "Text message"
			}
			label := base
			if name := strings.TrimSpace(method.Name); name != "" {
				label += " (" + name + ")"
			}
			choices = append(choices, choice{kind: kind, deviceID: method.DeviceID, label: label})
		}
	}
	choices = append(choices,
		choice{kind: "mobile_otp", label: "Duo Mobile code"},
		choice{kind: "bypass_code", label: "Bypass code"},
	)

	errOut := cmd.ErrOrStderr()
	fmt.Fprintln(errOut, "Choose a Duo method to enroll usc:")
	for i, c := range choices {
		fmt.Fprintf(errOut, "  %d. %s\n", i+1, c.label)
	}
	var picked choice
	for {
		fmt.Fprint(errOut, "Select [1]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		index := 1
		if line != "" {
			parsed, err := strconv.Atoi(line)
			if err != nil || parsed < 1 || parsed > len(choices) {
				fmt.Fprintf(errOut, "Enter a number from 1 to %d.\n", len(choices))
				continue
			}
			index = parsed
		}
		picked = choices[index-1]
		break
	}
	switch picked.kind {
	case "push":
		options.DuoPushDevice = picked.deviceID
	case "sms_otp":
		options.DuoSMSDevice = picked.deviceID
		options.RequestDuoPasscode = promptPasscode(cmd, input, true)
	case "mobile_otp":
		options.RequestDuoPasscode = promptPasscode(cmd, input, false)
	case "bypass_code":
		for options.Credentials.BypassCode == "" {
			code, err := readSecret(cmd, input, "Duo bypass code: ")
			if err != nil {
				return err
			}
			options.Credentials.BypassCode = code
		}
	}
	return nil
}

func promptForMissing(cmd *cobra.Command, input *os.File, reader *bufio.Reader, interactive bool, credentials *auth.Credentials, bypass bool) error {
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
	if !interactive {
		var missing []string
		for _, field := range fields {
			if *field.value == "" {
				missing = append(missing, field.name)
			}
		}
		return auth.ForSite(signInProbe, &auth.SignInRequired{Missing: missing})
	}
	for _, field := range fields {
		for *field.value == "" {
			if field.secret {
				secret, err := readSecret(cmd, input, field.label)
				if err != nil {
					return err
				}
				*field.value = secret
				continue
			}
			fmt.Fprint(cmd.ErrOrStderr(), field.label)
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			*field.value = strings.TrimSpace(line)
		}
	}
	return nil
}
