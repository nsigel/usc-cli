package cli

import (
	"github.com/nsigel/usc-cli/classes"
	"github.com/spf13/cobra"
)

func classesCommand() *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.AddCommand(&cobra.Command{Use: "terms", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		value, err := classes.New().Terms(cmd.Context())
		if err != nil {
			return err
		}
		return writeJSON(cmd, value)
	}})
	cmd.AddCommand(&cobra.Command{Use: "schools TERM_CODE", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		value, err := classes.New().Schools(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return writeJSON(cmd, value)
	}})
	cmd.AddCommand(&cobra.Command{Use: "courses TERM_CODE SCHOOL_PREFIX PROGRAM_PREFIX", Args: cobra.ExactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		value, err := classes.New().Courses(cmd.Context(), args[0], args[1], args[2])
		if err != nil {
			return err
		}
		return writeJSON(cmd, value)
	}})
	cmd.AddCommand(&cobra.Command{Use: "search TERM_CODE SEARCH_TERM", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		value, err := classes.New().Search(cmd.Context(), args[0], args[1])
		if err != nil {
			return err
		}
		return writeJSON(cmd, value)
	}})
	return cmd

}
