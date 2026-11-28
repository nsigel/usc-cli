package cli

import (
	"errors"

	"github.com/nsigel/usc-cli/auth"
	"github.com/spf13/cobra"
)

func duoCommand() *cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "List Duo devices enrolled on your USC account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			devices, err := auth.ListDuoDevices(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeJSON(cmd, devices)
		},
	}
	create := &cobra.Command{
		Use:   "create",
		Short: "Create and save a Duo TOTP device using existing authentication",
		Args:  cobra.NoArgs,
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
	del := &cobra.Command{
		Use:   "delete [DEVICE_ID]",
		Short: "Delete a Duo phone/TOTP device (defaults to the saved CLI device)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			} else {
				credentials, err := options.ResolveCredentials()
				if err != nil {
					return err
				}
				id = credentials.DuoDeviceID
				if id == "" {
					return errors.New("no saved Duo device ID; specify DEVICE_ID (run usc duo device list to find it)")
				}
			}
			result, err := auth.DeleteDuoDevice(cmd.Context(), options, id)
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	}
	return group("duo", "Manage Duo enrollment", group("device", "Manage Duo devices", list, create, del))
}
