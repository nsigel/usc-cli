package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/weblogin"
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
	server := exec.Command(executable, "auth", "login", "--web-serve", "--compact", "--device-name", name)
	server.SysProcAttr = detached()
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
	server, err := weblogin.Listen(weblogin.Options{
		Auth:  auth.Options{CredentialsFile: files.CredentialsFile, SessionFile: files.SessionFile, DuoDeviceName: name},
		Probe: signInProbe,
	})
	if err != nil {
		return err
	}
	if err := writeJSON(cmd, server.Started()); err != nil {
		return err
	}
	return server.Serve(cmd.Context())
}
