package cli

import (
	"github.com/nsigel/usc-cli/classes"
	"github.com/spf13/cobra"
)

func classesCommand() *cobra.Command {
	return group("classes", "Read the public USC Schedule of Classes",
		classesTermsCommand(), classesSchoolsCommand(), classesProgramCommand(),
		classesCourseCommand(), classesSearchCommand(),
	)
}

func classesTermsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "terms", Short: "List the terms the Schedule of Classes publishes", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			terms, err := classes.New().Terms(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd, terms)
		},
	}
}

func classesSchoolsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "schools TERM_CODE", Short: "List a term's schools and their programs", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			schools, err := classes.New().Schools(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(cmd, schools)
		},
	}
}

func classesProgramCommand() *cobra.Command {
	return &cobra.Command{
		Use: "program TERM_CODE SCHOOL_PREFIX PROGRAM_PREFIX", Short: "List every course and section a program offers", Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			courses, err := classes.New().ProgramCourses(cmd.Context(), args[0], args[1], args[2])
			if err != nil {
				return err
			}
			return writeJSON(cmd, courses)
		},
	}
}

func classesCourseCommand() *cobra.Command {
	return &cobra.Command{
		Use: "course TERM_CODE COURSE_CODE", Short: "Show one course and its sections", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			code, err := classes.ParseCourseCode(args[1])
			if err != nil {
				return err
			}
			course, err := classes.New().Course(cmd.Context(), args[0], code)
			if err != nil {
				return err
			}
			return writeJSON(cmd, course)
		},
	}
}

func classesSearchCommand() *cobra.Command {
	return &cobra.Command{
		Use: "search TERM_CODE TEXT", Short: "Search a term's courses by code, title, description, or instructor", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			courses, err := classes.New().Search(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return writeJSON(cmd, courses)
		},
	}
}
