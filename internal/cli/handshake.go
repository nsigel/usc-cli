package cli

import (
	"github.com/nsigel/usc-cli/handshake"
	"github.com/spf13/cobra"
)

func handshakeCommand() *cobra.Command {
	return group("handshake", "Read Handshake events and career fairs",
		handshakeCategoriesCommand(),
		handshakeEventsCommand(),
		handshakeEventCommand(),
		handshakeCareerFairsCommand(),
		handshakeCareerFairCommand(),
	)
}

func handshakeCategoriesCommand() *cobra.Command {
	return &cobra.Command{
		Use: "categories", Short: "List event filter categories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openHandshake(cmd)
			if err != nil {
				return err
			}
			categories, err := client.Categories(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd, categories)
		},
	}
}

func handshakeEventsCommand() *cobra.Command {
	var search handshake.Search
	cmd := &cobra.Command{
		Use: "events", Short: "Search student events", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openHandshake(cmd)
			if err != nil {
				return err
			}
			page, err := client.Events(cmd.Context(), search)
			if err != nil {
				return err
			}
			return writeJSON(cmd, page)
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
			client, err := openHandshake(cmd)
			if err != nil {
				return err
			}
			event, err := client.Event(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(cmd, event)
		},
	}
}

func handshakeCareerFairsCommand() *cobra.Command {
	var search handshake.Search
	cmd := &cobra.Command{
		Use: "career-fairs", Short: "Search career fairs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openHandshake(cmd)
			if err != nil {
				return err
			}
			page, err := client.CareerFairs(cmd.Context(), search)
			if err != nil {
				return err
			}
			return writeJSON(cmd, page)
		},
	}
	addHandshakeSearchFlags(cmd, &search)
	return cmd
}

func handshakeCareerFairCommand() *cobra.Command {
	return &cobra.Command{
		Use: "career-fair CAREER_FAIR_ID", Short: "Show career-fair details", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "CAREER_FAIR_ID")
			if err != nil {
				return err
			}
			client, err := openHandshake(cmd)
			if err != nil {
				return err
			}
			fair, err := client.CareerFair(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(cmd, fair)
		},
	}
}

func addHandshakeSearchFlags(cmd *cobra.Command, search *handshake.Search) {
	cmd.Flags().StringSliceVar(&search.Categories, "category", nil, "category name, slug, or ID (repeatable)")
	cmd.Flags().StringVar(&search.Organizer, "organizer", "", "host, employer, or contact name or email")
	cmd.Flags().StringVar(&search.Keyword, "keyword", "", "title or keyword search")
	cmd.Flags().StringVar(&search.Medium, "medium", "all", "all, virtual, or in-person")
	cmd.Flags().StringVar(&search.Date, "date", "all", "all, today, next-10, next-30, or past-year")
	cmd.Flags().StringVar(&search.Sort, "sort", "relevance", "relevance, date, or posted-desc")
	cmd.Flags().BoolVar(&search.PostedBySchool, "posted-by-school", false, "only show events posted by USC")
	cmd.Flags().IntVar(&search.Limit, "limit", 30, "results per page (1-100)")
	cmd.Flags().StringVar(&search.After, "after", "", "next_cursor of the previous page")
}
