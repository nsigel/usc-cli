package cli

import (
	"github.com/nsigel/usc-cli/directory"
	"github.com/spf13/cobra"
)

func directoryCommand() *cobra.Command {
	return group("directory", "Search the USC Student Directory", directoryStudentsCommand(), directoryStudentCommand())
}

func directoryStudentsCommand() *cobra.Command {
	var query directory.StudentQuery
	cmd := &cobra.Command{
		Use: "students", Short: "Search students by name, major, school, or email", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openDirectory(cmd)
			if err != nil {
				return err
			}
			results, err := client.Students(cmd.Context(), query)
			if err != nil {
				return err
			}
			return writeJSON(cmd, results)
		},
	}
	cmd.Flags().StringVar(&query.Query, "query", "", "name, major, or email address; not combined with the other flags")
	cmd.Flags().StringVar(&query.FirstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&query.LastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&query.Major, "major", "", "major")
	cmd.Flags().StringVar(&query.School, "school", "", "school, e.g. Viterbi")
	cmd.Flags().StringVar(&query.Email, "email", "", "email address")
	cmd.Flags().StringVar(&query.Phone, "phone", "", "published USC campus/office phone number")
	cmd.MarkFlagsOneRequired("query", "first-name", "last-name", "major", "school", "email", "phone")
	return cmd
}

func directoryStudentCommand() *cobra.Command {
	return &cobra.Command{
		Use: "student STUDENT_ID", Short: "Show one student from a search", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := openDirectory(cmd)
			if err != nil {
				return err
			}
			student, err := client.Student(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(cmd, student)
		},
	}
}
