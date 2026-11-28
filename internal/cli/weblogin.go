package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/internal/weblogin"
	"github.com/spf13/cobra"
)

// startWebLogin leaves the sign-in page running in a detached process so the
// command can return its URL at once.
func startWebLogin(cmd *cobra.Command) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("device-name")
	arguments := []string{"auth", "login", "--web-serve", "--compact", "--device-name", name}
	if remote, _ := cmd.Flags().GetBool("remote"); remote {
		arguments = append(arguments, "--remote")
	}
	server := exec.Command(executable, arguments...)
	server.SysProcAttr = detached()
	var problem bytes.Buffer
	server.Stderr = &problem
	output, err := server.StdoutPipe()
	if err != nil {
		return err
	}
	if err := server.Start(); err != nil {
		return err
	}
	var started weblogin.Started
	if err := json.NewDecoder(output).Decode(&started); err != nil {
		_ = server.Wait()
		var reported struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(problem.Bytes(), &reported) == nil && reported.Error != "" {
			return errors.New(reported.Error)
		}
		return errors.New("could not start the sign-in page")
	}
	if err := server.Process.Release(); err != nil {
		return err
	}
	return writeJSON(cmd, started)
}

func serveWebLogin(cmd *cobra.Command) error {
	files, err := signIn()
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("device-name")
	options := weblogin.Options{
		Auth:  auth.Options{CredentialsFile: files.CredentialsFile, SessionFile: files.SessionFile, DuoDeviceName: name},
		Probe: signInProbe,
	}
	if remote, _ := cmd.Flags().GetBool("remote"); remote {
		options.Relay = "https://usc.sh"
		// A link on another site would be a phishing page, so the override
		// only reaches a relay under test on this machine.
		if site, err := url.Parse(os.Getenv("USC_RELAY_URL")); err == nil && (site.Hostname() == "localhost" || site.Hostname() == "127.0.0.1") {
			options.Relay = site.String()
		}
	}
	server, err := weblogin.Listen(options)
	if err != nil {
		return err
	}
	if err := writeJSON(cmd, server.Started()); err != nil {
		return err
	}
	return server.Serve(cmd.Context())
}
