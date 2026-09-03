package cli

import (
	"github.com/nsigel/usc-cli/internal/skill"
	"github.com/spf13/cobra"
)

func skillCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill definition",
		Long:  "Print the bundled usc-cli SKILL.md for use with agent skill systems.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return skill.WriteUSC(cmd.OutOrStdout())
		},
	}
}
