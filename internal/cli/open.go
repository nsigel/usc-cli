package cli

import (
	"os"

	usc "github.com/nsigel/usc-cli"
	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
	"github.com/nsigel/usc-cli/directory"
	"github.com/nsigel/usc-cli/handshake"
	"github.com/nsigel/usc-cli/libcal"
	"github.com/nsigel/usc-cli/webreg"
	"github.com/spf13/cobra"
)

func signIn() (auth.Options, error) {
	credentialsFile, err := usc.CredentialsPath()
	if err != nil {
		return auth.Options{}, err
	}
	sessionFile, err := usc.SessionPath()
	if err != nil {
		return auth.Options{}, err
	}
	return auth.Options{
		DuoPasscode:     os.Getenv("USC_DUO_PASSCODE"),
		CredentialsFile: credentialsFile,
		SessionFile:     sessionFile,
		Credentials: auth.Credentials{
			Username:      os.Getenv("USC_USERNAME"),
			Password:      os.Getenv("USC_PASSWORD"),
			BypassCode:    os.Getenv("USC_DUO_BYPASS"),
			DuoTOTPSecret: os.Getenv("USC_DUO_TOTP_SECRET"),
		},
	}, nil
}

func openBrightspace(cmd *cobra.Command) (*brightspace.Client, error) {
	options, err := signIn()
	if err != nil {
		return nil, err
	}
	return brightspace.Open(cmd.Context(), options)
}

func openHandshake(cmd *cobra.Command) (*handshake.Client, error) {
	options, err := signIn()
	if err != nil {
		return nil, err
	}
	return handshake.Open(cmd.Context(), options)
}

func openWebReg(cmd *cobra.Command) (*webreg.Client, error) {
	options, err := signIn()
	if err != nil {
		return nil, err
	}
	return webreg.Open(cmd.Context(), options)
}

func openLibCal() (*libcal.Client, error) {
	options, err := signIn()
	if err != nil {
		return nil, err
	}
	return libcal.Open(options)
}

func openDirectory(cmd *cobra.Command) (*directory.Client, error) {
	options, err := signIn()
	if err != nil {
		return nil, err
	}
	return directory.Open(cmd.Context(), options)
}
