package cli

import (
	"github.com/nsigel/usc-cli/internal/classes"
	"github.com/spf13/cobra"
)

func classesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "classes TERM_CODE COURSE_CODE",
		Short: "Show a public USC course and its sections",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			course, err := classes.New().Course(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return writeJSON(cmd, course)
		},
	}
}
