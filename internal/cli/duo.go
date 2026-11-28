package cli

import (
	"github.com/nsigel/usc-cli/auth"
	"github.com/spf13/cobra"
)

func duoCommand() *cobra.Command {
	create := &cobra.Command{
		Use: "create", Short: "Create and save a Duo TOTP device using existing authentication",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			options.DuoDeviceName, _ = cmd.Flags().GetString("device-name")
			result, err := auth.CreateDuoDevice(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	}
	create.Flags().String("device-name", "", "Name for the new Duo authenticator")
	delete := &cobra.Command{Use: "delete DEVICE_ID", Short: "Delete the specified Duo enrollment", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		options, err := signIn()
		if err != nil {
			return err
		}
		result, err := auth.DeleteDuoDevice(cmd.Context(), options, args[0])
		if err != nil {
			return err
		}
		return writeJSON(cmd, result)
	}}
	return group("duo", "Manage Duo enrollment", group("device", "Manage Duo devices", create, delete))
}
