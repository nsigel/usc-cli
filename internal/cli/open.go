package cli

import (
	"os"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
	"github.com/nsigel/usc-cli/config"
	"github.com/nsigel/usc-cli/handshake"
	"github.com/nsigel/usc-cli/libcal"
	"github.com/nsigel/usc-cli/webreg"
	"github.com/spf13/cobra"
)

func signIn() (auth.Options, error) {
	credentialsFile, err := config.CredentialsPath()
	if err != nil {
		return auth.Options{}, err
	}
	sessionFile, err := config.SessionPath()
	if err != nil {
		return auth.Options{}, err
	}
	return auth.Options{
		CredentialsFile: credentialsFile,
		SessionFile:     sessionFile,
		Credentials: auth.Credentials{
			Username:   os.Getenv("USC_USERNAME"),
			Password:   os.Getenv("USC_PASSWORD"),
			BypassCode: os.Getenv("USC_DUO_BYPASS"),
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
