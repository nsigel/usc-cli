package cli

import (
	"fmt"

	"github.com/nsigel/usc-cli/brightspace"
	"github.com/spf13/cobra"
)

func brightspaceCommand() *cobra.Command {
	return group("brightspace", "Read Brightspace course data",
		brightspaceWhoAmICommand(),
		coursesCommand(),
		contentCommand(),
		gradesCommand(),
		announcementsCommand(),
		assignmentsCommand(),
		submissionsCommand(),
		downloadCommand(),
	)
}

func brightspaceWhoAmICommand() *cobra.Command {
	return &cobra.Command{
		Use: "whoami", Short: "Show the authenticated Brightspace user", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			user, err := client.WhoAmI(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd, user)
		},
	}
}

func coursesCommand() *cobra.Command {
	var includeAll bool
	cmd := &cobra.Command{
		Use: "courses", Short: "List Brightspace enrollments", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			courses, err := client.Courses(cmd.Context(), includeAll)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"courses": courses})
		},
	}
	cmd.Flags().BoolVar(&includeAll, "all", false, "include non-course enrollments")
	return cmd
}

func contentCommand() *cobra.Command {
	var flat bool
	cmd := &cobra.Command{
		Use: "content COURSE_ID", Short: "Show a course's content table of contents", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := positiveID(args[0], "COURSE_ID")
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			toc, err := client.Content(cmd.Context(), courseID)
			if err != nil {
				return err
			}
			if flat {
				return writeJSON(cmd, map[string]any{"course_id": courseID, "topics": toc.Flatten()})
			}
			return writeJSON(cmd, toc)
		},
	}
	cmd.Flags().BoolVar(&flat, "flat", false, "list topics in one array, each with its module path")
	return cmd
}

func gradesCommand() *cobra.Command {
	var gradedOnly bool
	cmd := &cobra.Command{
		Use: "grades COURSE_ID", Short: "Show grades for a course", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := positiveID(args[0], "COURSE_ID")
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			grades, err := client.Grades(cmd.Context(), courseID)
			if err != nil {
				return err
			}
			if gradedOnly {
				grades = grades.Graded()
			}
			return writeJSON(cmd, grades)
		},
	}
	cmd.Flags().BoolVar(&gradedOnly, "graded-only", false, "only include items with a score")
	return cmd
}

func announcementsCommand() *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use: "announcements [COURSE_ID]", Short: "Show announcements for one course or every enrolled course", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				feed, err := client.AllAnnouncements(cmd.Context(), since)
				if err != nil {
					return err
				}
				return writeJSON(cmd, feed)
			}
			courseID, err := positiveID(args[0], "COURSE_ID")
			if err != nil {
				return err
			}
			announcements, err := client.Announcements(cmd.Context(), courseID, since)
			if err != nil {
				return err
			}
			return writeJSON(cmd, brightspace.AnnouncementFeed{Announcements: announcements})
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only include announcements on or after this ISO-8601 time")
	return cmd
}

func assignmentsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "assignments COURSE_ID", Short: "List assignment folders for a course", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := positiveID(args[0], "COURSE_ID")
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			assignments, err := client.Assignments(cmd.Context(), courseID)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"assignments": assignments})
		},
	}
}

func submissionsCommand() *cobra.Command {
	var download bool
	var output string
	cmd := &cobra.Command{
		Use: "submissions COURSE_ID FOLDER_ID", Short: "List or download your submissions to an assignment folder", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := positiveID(args[0], "COURSE_ID")
			if err != nil {
				return err
			}
			folderID, err := positiveID(args[1], "FOLDER_ID")
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			submissions, err := client.Submissions(cmd.Context(), courseID, folderID)
			if err != nil {
				return err
			}
			result := map[string]any{"course_id": courseID, "folder_id": folderID}
			if !download {
				result["submissions"] = submissions
				return writeJSON(cmd, result)
			}
			result["files"], err = downloadSubmissions(cmd, client, courseID, folderID, submissions, output)
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	}
	cmd.Flags().BoolVar(&download, "download", false, "download every submitted file")
	cmd.Flags().StringVarP(&output, "output", "o", ".", "directory for downloaded files")
	return cmd
}

type downloadedFile struct {
	SubmissionID int     `json:"submission_id"`
	FileID       int     `json:"file_id"`
	Date         *string `json:"date"`
	Path         string  `json:"path"`
}

func downloadSubmissions(cmd *cobra.Command, client *brightspace.Client, courseID, folderID int, submissions []brightspace.Submission, directory string) ([]downloadedFile, error) {
	files := []downloadedFile{}
	used := make(map[string]bool)
	for _, submission := range submissions {
		for _, file := range submission.Files {
			name := safeFileName(file.Name)
			if used[name] {
				name = fmt.Sprintf("%d_%s", submission.ID, name)
			}
			used[name] = true
			body, err := client.DownloadSubmissionFile(cmd.Context(), courseID, folderID, submission.ID, file.ID)
			if err != nil {
				return nil, err
			}
			path, err := saveDownload(directory, name, body)
			body.Close()
			if err != nil {
				return nil, err
			}
			files = append(files, downloadedFile{SubmissionID: submission.ID, FileID: file.ID, Date: submission.Date, Path: path})
		}
	}
	return files, nil
}
