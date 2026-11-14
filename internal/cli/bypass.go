package cli

import (
	"github.com/nsigel/usc-cli/auth"
	"github.com/spf13/cobra"
)

func bypassCommand() *cobra.Command {
	return group("bypass", "Manage USC Duo bypass codes", &cobra.Command{
		Use:   "create",
		Short: "Create and save a Duo bypass code",
		Long: "Create a Duo bypass code using the saved USC sign-in and save it for later commands.\n\n" +
			"May invalidate previous codes. JSON output contains the secret code; do not log it.\n" +
			"Does not prompt or schedule automatic renewal.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := signIn()
			if err != nil {
				return err
			}
			result, err := auth.CreateBypassCode(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	})
}
