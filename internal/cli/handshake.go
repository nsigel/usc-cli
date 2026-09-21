package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/handshake"
	"github.com/nsigel/usc-cli/internal/site"
	"github.com/spf13/cobra"
)

func handshakeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "handshake", Short: "Read Handshake events and career fairs"}
	cmd.AddCommand(
		handshakeCategoriesCommand(),
		handshakeEventsCommand(),
		handshakeEventCommand(),
		handshakeCareerFairsCommand(),
		handshakeCareerFairCommand(),
	)
	return cmd
}

func handshakeCategoriesCommand() *cobra.Command {
	return &cobra.Command{
		Use: "categories", Short: "List event filter categories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openHandshake(cmd.Context())
			if err != nil {
				return err
			}
			categories, err := client.Categories(cmd.Context())
			if err != nil {
				return handshakeError(err)
			}
			return writeJSON(cmd, map[string]any{"categories": categories})
		},
	}
}

func handshakeEventsCommand() *cobra.Command {
	search := handshake.Search{}
	cmd := &cobra.Command{
		Use: "events", Short: "Search student events", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openHandshake(cmd.Context())
			if err != nil {
				return err
			}
			page, err := client.Events(cmd.Context(), search)
			if err != nil {
				return handshakeError(err)
			}
			return writeJSON(cmd, handshakePage("events", page.Items, page))
		},
	}
	addHandshakeSearchFlags(cmd, &search)
	return cmd
}

func handshakeEventCommand() *cobra.Command {
	return &cobra.Command{
		Use: "event EVENT_ID", Short: "Show event details", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "EVENT_ID")
			if err != nil {
				return err
			}
			client, err := openHandshake(cmd.Context())
			if err != nil {
				return err
			}
			event, err := client.Event(cmd.Context(), id)
			if err != nil {
				return handshakeError(err)
			}
			return writeJSON(cmd, event)
		},
	}
}

func handshakeCareerFairsCommand() *cobra.Command {
	search := handshake.Search{}
	cmd := &cobra.Command{
		Use: "career-fairs", Aliases: []string{"fairs"}, Short: "Search career fairs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openHandshake(cmd.Context())
			if err != nil {
				return err
			}
			page, err := client.CareerFairs(cmd.Context(), search)
			if err != nil {
				return handshakeError(err)
			}
			return writeJSON(cmd, handshakePage("career_fairs", page.Items, page))
		},
	}
	addHandshakeSearchFlags(cmd, &search)
	return cmd
}

func handshakeCareerFairCommand() *cobra.Command {
	return &cobra.Command{
		Use: "career-fair CAREER_FAIR_ID", Aliases: []string{"fair"}, Short: "Show career-fair details", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "CAREER_FAIR_ID")
			if err != nil {
				return err
			}
			client, err := openHandshake(cmd.Context())
			if err != nil {
				return err
			}
			fair, err := client.CareerFair(cmd.Context(), id)
			if err != nil {
				return handshakeError(err)
			}
			return writeJSON(cmd, fair)
		},
	}
}

func addHandshakeSearchFlags(cmd *cobra.Command, search *handshake.Search) {
	cmd.Flags().StringSliceVarP(&search.Categories, "category", "c", nil, "category name, slug, or ID (repeatable)")
	cmd.Flags().StringVar(&search.Organizer, "organizer", "", "organizer email or name")
	cmd.Flags().StringVarP(&search.Keyword, "keyword", "q", "", "title or keyword search")
	cmd.Flags().StringVar(&search.Medium, "medium", "all", "all, virtual, or in-person")
	cmd.Flags().StringVar(&search.Date, "date", "all", "all, today, next-10, next-30, or past-year")
	cmd.Flags().StringVar(&search.Sort, "sort", "relevance", "relevance, date, or posted-desc")
	cmd.Flags().BoolVar(&search.PostedBySchool, "posted-by-school", false, "only show events posted by USC")
	cmd.Flags().IntVarP(&search.Limit, "limit", "n", 30, "results per page (1-100)")
	cmd.Flags().StringVar(&search.After, "after", "", "cursor returned by the previous page")
}

func handshakePage[T any](key string, items []T, page handshake.Page[T]) map[string]any {
	return map[string]any{
		key: items,
		"pagination": map[string]any{
			"has_more": page.HasMore, "next_cursor": page.NextCursor, "cursor_kind": page.CursorKind,
		},
		"sort": page.Sort,
	}
}

func openHandshake(ctx context.Context) (*handshake.Client, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	selected, err := site.Find(site.Handshake)
	if err != nil {
		return nil, err
	}
	if err := auth.Login(ctx, selected.LoginURL, path, auth.Credentials{}); err != nil {
		if errors.Is(err, auth.ErrCredentialsRequired) {
			return nil, withAction(errors.New("authentication required"), "usc auth login handshake")
		}
		return nil, err
	}
	session, err := auth.OpenSession(path)
	if err != nil {
		return nil, err
	}
	return handshake.New(session), nil
}

func handshakeError(err error) error {
	if errors.Is(err, handshake.ErrSessionInvalid) {
		return withAction(err, "usc auth login handshake")
	}
	return err
}

func positiveID(value, label string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", label)
	}
	return id, nil
}
