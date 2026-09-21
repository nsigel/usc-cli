package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/brightspace"
	"github.com/nsigel/usc-cli/internal/site"
	"github.com/spf13/cobra"
)

func brightspaceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "brightspace", Short: "Read Brightspace course data"}
	cmd.AddCommand(
		brightspaceWhoAmICommand(),
		coursesCommand(),
		contentCommand(),
		gradesCommand(),
		announcementsCommand(),
		assignmentsCommand(),
		downloadCommand(),
	)
	return cmd
}

func brightspaceWhoAmICommand() *cobra.Command {
	return &cobra.Command{
		Use: "whoami", Short: "Show the authenticated Brightspace user", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openBrightspace(cmd.Context())
			if err != nil {
				return err
			}
			result, err := client.WhoAmI(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	}
}

func coursesCommand() *cobra.Command {
	var includeAll bool
	cmd := &cobra.Command{
		Use: "courses", Short: "List Brightspace enrollments", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openBrightspace(cmd.Context())
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
			courseID, err := courseID(args[0])
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd.Context())
			if err != nil {
				return err
			}
			toc, err := client.Content(cmd.Context(), courseID)
			if err != nil {
				return err
			}
			if !flat {
				return writeJSON(cmd, toc)
			}
			return writeJSON(cmd, map[string]any{"course_id": courseID, "topics": flattenTopics(toc.Modules, "")})
		},
	}
	cmd.Flags().BoolVar(&flat, "flat", false, "flatten topics into a single list")
	return cmd
}

func gradesCommand() *cobra.Command {
	var gradedOnly bool
	cmd := &cobra.Command{
		Use: "grades COURSE_ID", Short: "Show grades for a course", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := courseID(args[0])
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd.Context())
			if err != nil {
				return err
			}
			result, err := client.Grades(cmd.Context(), courseID)
			if err != nil {
				return err
			}
			if gradedOnly {
				filtered := make([]brightspace.Grade, 0, len(result.Grades))
				for _, grade := range result.Grades {
					if grade.Score != nil {
						filtered = append(filtered, grade)
					}
				}
				result.Grades = filtered
			}
			return writeJSON(cmd, result)
		},
	}
	cmd.Flags().BoolVar(&gradedOnly, "graded-only", false, "only include items with a score")
	return cmd
}

func announcementsCommand() *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use: "announcements [COURSE_ID]", Short: "Show course announcements", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := openBrightspace(cmd.Context())
			if err != nil {
				return err
			}
			if len(args) == 1 {
				courseID, err := courseID(args[0])
				if err != nil {
					return err
				}
				items, err := client.Announcements(cmd.Context(), courseID, since)
				if err != nil {
					return err
				}
				return writeJSON(cmd, map[string]any{"announcements": announcementsForCourse(courseID, nil, items)})
			}
			courses, err := client.Courses(cmd.Context(), false)
			if err != nil {
				return err
			}
			items := make([]any, 0)
			errors := make([]map[string]any, 0)
			for _, course := range courses {
				announcements, err := client.Announcements(cmd.Context(), course.ID, since)
				if err != nil {
					errors = append(errors, map[string]any{"course_id": course.ID, "error": err.Error()})
					continue
				}
				for _, announcement := range announcementsForCourse(course.ID, course.Code, announcements) {
					items = append(items, announcement)
				}
			}
			result := map[string]any{"announcements": items}
			if len(errors) > 0 {
				result["errors"] = errors
			}
			return writeJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only include announcements on or after this ISO-8601 time")
	return cmd
}

func assignmentsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "assignments COURSE_ID", Aliases: []string{"dropbox"}, Short: "List assignment folders for a course", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := courseID(args[0])
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd.Context())
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

func openBrightspace(ctx context.Context) (*brightspace.Client, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	selected, err := site.Find(site.Brightspace)
	if err != nil {
		return nil, err
	}
	if err := auth.Login(ctx, selected.LoginURL, path, auth.Credentials{}); err != nil {
		if errors.Is(err, auth.ErrCredentialsRequired) {
			return nil, withAction(errors.New("authentication required"), "usc auth login")
		}
		return nil, err
	}
	session, err := auth.OpenSession(path)
	if err != nil {
		return nil, err
	}
	return brightspace.New(session), nil
}

func courseID(value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("COURSE_ID must be a positive integer")
	}
	return id, nil
}

func flattenTopics(modules []brightspace.Module, parent string) []map[string]any {
	topics := make([]map[string]any, 0)
	for _, module := range modules {
		path := module.Title
		if parent != "" {
			path = parent + " > " + path
		}
		for _, topic := range module.Topics {
			topics = append(topics, map[string]any{"id": topic.ID, "title": topic.Title, "type": topic.Type, "url": topic.URL, "due_date": topic.DueDate, "last_modified": topic.LastModified, "module": path})
		}
		topics = append(topics, flattenTopics(module.Modules, path)...)
	}
	return topics
}

func announcementsForCourse(courseID int, code *string, announcements []brightspace.Announcement) []map[string]any {
	items := make([]map[string]any, len(announcements))
	for index, announcement := range announcements {
		items[index] = map[string]any{"course_id": courseID, "course_code": code, "id": announcement.ID, "title": announcement.Title, "body": announcement.Body, "start_date": announcement.StartDate, "end_date": announcement.EndDate, "created_date": announcement.CreatedDate, "last_modified_date": announcement.LastModifiedDate, "is_pinned": announcement.IsPinned, "is_hidden": announcement.IsHidden, "attachments": announcement.Attachments}
	}
	return items
}
