package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nsigel/usc-cli/skill"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func skillCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill definition",
		Long:  "Print the bundled usc-cli SKILL.md, followed by every command generated from this CLI.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			writer := cmd.OutOrStdout()
			if err := skill.WriteUSC(writer); err != nil {
				return err
			}
			_, err := io.WriteString(writer, "\n## All commands\n\nGenerated from the CLI; run `usc <command> --help` for flag details.\n\n"+commandIndex(cmd.Root()))
			return err
		},
	}
}

// commandIndex lists each leaf command as "path args --flag : short".
func commandIndex(cmd *cobra.Command) string {
	if cmd.Hidden || cmd.Name() == "help" || cmd.Name() == "completion" {
		return ""
	}
	if cmd.HasSubCommands() {
		var index strings.Builder
		for _, child := range cmd.Commands() {
			index.WriteString(commandIndex(child))
		}
		return index.String()
	}
	line := strings.TrimSuffix(cmd.CommandPath(), cmd.Name()) + cmd.Use
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name != "help" && !flag.Hidden {
			line += " --" + flag.Name
		}
	})
	return fmt.Sprintf("- %s : %s\n", line, cmd.Short)
}
