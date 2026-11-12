package handshake

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultLimit = 30
	// organizerFetchSize is the smallest page fetched under an organizer
	// filter, which discards most results.
	organizerFetchSize = 30
)

type query struct {
	model     string
	params    map[string]any
	limit     int
	after     string
	organizer string
	sort      string
}

func (c *Client) search(ctx context.Context, search Search, model string) (Page[abstractionWire], error) {
	if search.Limit == 0 {
		search.Limit = defaultLimit
	}
	if search.Limit < 1 || search.Limit > 100 {
		return Page[abstractionWire]{}, errors.New("limit must be between 1 and 100")
	}
	categoryIDs, err := c.categoryIDs(ctx, search.Categories)
	if err != nil {
		return Page[abstractionWire]{}, err
	}
	medium, err := searchMedium(search.Medium)
	if err != nil {
		return Page[abstractionWire]{}, err
	}
	date, err := searchDate(search.Date)
	if err != nil {
		return Page[abstractionWire]{}, err
	}
	order, err := serverSort(search.Sort)
	if err != nil {
		return Page[abstractionWire]{}, err
	}
	q := query{
		model: model, limit: search.Limit, after: search.After,
		organizer: strings.TrimSpace(search.Organizer), sort: normalizedSort(search.Sort),
		params: map[string]any{
			"categories":     categoryIDs,
			"collection":     "ALL",
			"date":           date,
			"keyword":        strings.TrimSpace(search.Keyword),
			"medium":         medium,
			"postedBySchool": search.PostedBySchool,
			"searchModels":   []string{model},
			"sort":           order,
		},
	}
	if q.sort == sortPosted {
		return c.searchByNewestID(ctx, q)
	}
	return c.searchByCursor(ctx, q)
}

func (c *Client) searchByCursor(ctx context.Context, q query) (Page[abstractionWire], error) {
	result := Page[abstractionWire]{Items: []abstractionWire{}, CursorKind: "graphql", Sort: q.sort}
	fetchSize := q.limit
	if q.organizer != "" {
		fetchSize = max(fetchSize, organizerFetchSize)
	}
	after := q.after
	for {
		connection, err := c.fetchAbstractions(ctx, q.params, fetchSize, after)
		if err != nil {
			return Page[abstractionWire]{}, err
		}
		for index, edge := range connection.Edges {
			matched, err := c.matchesOrganizer(ctx, edge.Node, q)
			if err != nil {
				return Page[abstractionWire]{}, err
			}
			if !matched {
				continue
			}
			result.Items = append(result.Items, edge.Node)
			if len(result.Items) == q.limit {
				result.HasMore = index < len(connection.Edges)-1 || connection.PageInfo.HasNextPage
				if result.HasMore {
					result.NextCursor = strings.TrimSpace(edge.Cursor)
				}
				return result, nil
			}
		}
		if !connection.PageInfo.HasNextPage {
			return result, nil
		}
		after = strings.TrimSpace(connection.PageInfo.EndCursor)
	}
}

// Handshake exposes no posting timestamp and rejects every posting sort,
// but IDs are monotonic, so this is the order in which results were posted.
func (c *Client) searchByNewestID(ctx context.Context, q query) (Page[abstractionWire], error) {
	before := int64(0)
	if q.after != "" {
		var err error
		if before, err = strconv.ParseInt(q.after, 10, 64); err != nil || before < 1 {
			return Page[abstractionWire]{}, errors.New("after must be an event ID when sort is posted-desc")
		}
	}
	items := []abstractionWire{}
	after := ""
	for {
		// The schema's complexity limit rejects this projection at 100 records.
		connection, err := c.fetchAbstractions(ctx, q.params, 50, after)
		if err != nil {
			return Page[abstractionWire]{}, err
		}
		for _, edge := range connection.Edges {
			if before > 0 && numericID(edge.Node.ID) >= before {
				continue
			}
			matched, err := c.matchesOrganizer(ctx, edge.Node, q)
			if err != nil {
				return Page[abstractionWire]{}, err
			}
			if matched {
				items = append(items, edge.Node)
			}
		}
		if !connection.PageInfo.HasNextPage {
			break
		}
		after = strings.TrimSpace(connection.PageInfo.EndCursor)
	}
	sort.SliceStable(items, func(i, j int) bool { return numericID(items[i].ID) > numericID(items[j].ID) })
	page := Page[abstractionWire]{Items: items, CursorKind: "id", Sort: q.sort}
	if len(items) > q.limit {
		page.Items = items[:q.limit]
		page.HasMore = true
		page.NextCursor = page.Items[q.limit-1].ID
	}
	return page, nil
}

func (c *Client) fetchAbstractions(ctx context.Context, params map[string]any, first int, after string) (abstractionConnectionWire, error) {
	// The student site requests three employer previews per search card. Matching
	// that limit also keeps large pages under GraphQL's complexity cap.
	variables := map[string]any{"params": params, "first": first, "maxEmployersCount": 3}
	if after != "" {
		variables["after"] = after
	}
	var response struct {
		Connection abstractionConnectionWire `json:"eventAbstractions"`
	}
	err := c.graphQL(ctx, searchGraphQL, "GetEventAbstractions", eventAbstractionsQuery, variables, &response)
	return response.Connection, err
}

// matchesOrganizer reports whether the organizer filter accepts a result.
func (c *Client) matchesOrganizer(ctx context.Context, node abstractionWire, q query) (bool, error) {
	if q.organizer == "" || cardNamesOrganizer(node, q.organizer) {
		return true, nil
	}
	contacts, err := c.fetchContacts(ctx, node.ID, q.model)
	for _, contact := range contacts {
		if containsFold(contact.Name, q.organizer) || containsFold(contact.Email, q.organizer) {
			return true, nil
		}
	}
	return false, err
}

// cardNamesOrganizer checks what a search result already carries: its host
// and employers.
func cardNamesOrganizer(node abstractionWire, organizer string) bool {
	if containsFold(node.Host.Name, organizer) {
		return true
	}
	for _, employer := range node.Employers {
		if containsFold(employer.Name, organizer) {
			return true
		}
	}
	return false
}

// fetchContacts requests a result's detail record, the only place its
// contacts are listed.
func (c *Client) fetchContacts(ctx context.Context, id, model string) ([]Contact, error) {
	number, err := strconv.Atoi(id)
	if err != nil {
		return nil, nil
	}
	if model == "CareerFair" {
		fair, err := c.CareerFair(ctx, number)
		return []Contact{fair.Contact}, err
	}
	event, err := c.Event(ctx, number)
	return event.Contacts, err
}

func containsFold(value, substring string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(substring))
}

func numericID(value string) int64 {
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

func convertPage[T any](page Page[abstractionWire], convert func(abstractionWire) T) Page[T] {
	result := Page[T]{Items: make([]T, len(page.Items)), HasMore: page.HasMore, NextCursor: page.NextCursor, CursorKind: page.CursorKind, Sort: page.Sort}
	for index, item := range page.Items {
		result.Items[index] = convert(item)
	}
	return result
}

func (c *Client) categoryIDs(ctx context.Context, values []string) ([]string, error) {
	if len(values) == 0 {
		return []string{}, nil
	}
	categories, err := c.Categories(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]string, len(categories)*2)
	for _, category := range categories {
		byName[category.Slug] = category.ID
		byName[strings.ToLower(category.Name)] = category.ID
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		key := slug(value)
		id := byName[key]
		if id == "" {
			if _, err := strconv.Atoi(value); err == nil {
				id = value
			}
		}
		if id == "" {
			return nil, fmt.Errorf("unknown category %q", value)
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result, nil
}

func searchMedium(value string) (string, error) {
	switch slug(value) {
	case "", "all", "hybrid":
		return "HYBRID", nil
	case "virtual":
		return "VIRTUAL", nil
	case "in-person", "inperson":
		return "IN_PERSON", nil
	default:
		return "", errors.New("medium must be all, virtual, or in-person")
	}
}

func searchDate(value string) (string, error) {
	switch slug(value) {
	case "", "all":
		return "ALL", nil
	case "today":
		return "TODAY", nil
	case "next-10":
		return "NEXT_10", nil
	case "next-30":
		return "NEXT_30", nil
	case "past-year":
		return "PAST_YEAR", nil
	default:
		return "", errors.New("date must be all, today, next-10, next-30, or past-year")
	}
}

const sortPosted = "posted-desc"

// serverSort is the order requested from Handshake. posted-desc has no
// server equivalent: everything is read by date and reordered by ID here.
func serverSort(value string) (string, error) {
	switch normalizedSort(value) {
	case "relevance":
		return "RELEVANCE", nil
	case "date", sortPosted:
		return "DATE", nil
	default:
		return "", errors.New("sort must be relevance, date, or posted-desc")
	}
}

func normalizedSort(value string) string {
	if value == "" {
		return "relevance"
	}
	return slug(value)
}
