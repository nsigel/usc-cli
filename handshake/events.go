package handshake

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const categoriesQuery = `query GetEventAbstractionCategories {
  eventAbstractionCategories { id name behaviorIdentifier }
}`

const eventAbstractionsQuery = `query GetEventAbstractions($params: EventAbstractionSearchInput, $first: Int, $after: String, $maxEmployersCount: Int) {
  eventAbstractions(params: $params, first: $first, after: $after) {
    pageInfo { endCursor hasNextPage }
    edges {
      cursor
      node {
        id
        name
        type
        favorited
        startDate
        endDate
        registered
        medium
        employers(limit: $maxEmployersCount) {
          id
          name
          logo { url(size: "small") }
          industry { name }
          location { name }
        }
        categories { id name behaviorIdentifier }
        ... on Event {
          host { logoUrl name }
          sameSchoolHost
          sameSchoolEvent
          studentRegistrationEnd
          studentRegistrationStart
          registeredAttendeesCount
        }
        ... on CareerFair {
          host { logoUrl name }
          careerFairMedium
          sameSchoolHost
          studentRegistrationEnd
          studentRegistrationStart
          registeredAttendeesCount
        }
      }
    }
  }
}`

const eventQuery = `query GetEvent($id: ID!) {
  event(id: $id) {
    id
    name
    description
    startDate
    endDate
    eventType { name behaviorIdentifier }
    eventMediumType
    externalLink
    externalRegistration
    legalDisclaimer
    studentRegistrationEnd
    studentRegistrationStart
    virtualLink
    availableSpace
    waitlistEnabled
    waitlistedCount
    registeredAttendeesCount
    timeZone
    image { url }
    room { name building { name location { name } } }
    location { name }
    owner {
      ... on Employer { id name logo { url } }
      ... on School { id name logo { url } }
      __typename
    }
    host { id type name logo { url } }
    attendee { registered registering waitlisted checkedIn }
    categories { id name behaviorIdentifier }
    contacts { id firstName lastName title emailAddress user { userPhotoUrl } }
    employers {
      id
      name
      logo { url(size: "small") }
      industry { name }
      location { name }
    }
  }
}`

type categoryWire struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	BehaviorIdentifier string `json:"behaviorIdentifier"`
}

type imageWire struct {
	URL string `json:"url"`
}

type employerWire struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Logo     imageWire `json:"logo"`
	Industry struct {
		Name string `json:"name"`
	} `json:"industry"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
}

type contactWire struct {
	ID           string `json:"id"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Title        string `json:"title"`
	EmailAddress string `json:"emailAddress"`
	User         struct {
		Photo string `json:"userPhotoUrl"`
	} `json:"user"`
}

type hostWire struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Typename string    `json:"__typename"`
	Name     string    `json:"name"`
	Logo     imageWire `json:"logo"`
	LogoURL  string    `json:"logoUrl"`
}

type abstractionWire struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	Favorited        bool           `json:"favorited"`
	StartDate        string         `json:"startDate"`
	EndDate          string         `json:"endDate"`
	Registered       bool           `json:"registered"`
	Medium           string         `json:"medium"`
	CareerFairMedium string         `json:"careerFairMedium"`
	Employers        []employerWire `json:"employers"`
	Categories       []categoryWire `json:"categories"`
	EventType        struct {
		Name string `json:"name"`
	} `json:"eventType"`
	Host                hostWire      `json:"host"`
	Contacts            []contactWire `json:"contacts"`
	SameSchoolHost      bool          `json:"sameSchoolHost"`
	SameSchoolEvent     bool          `json:"sameSchoolEvent"`
	RegistrationEnd     *string       `json:"studentRegistrationEnd"`
	RegistrationStart   *string       `json:"studentRegistrationStart"`
	RegisteredAttendees int           `json:"registeredAttendeesCount"`
}

type abstractionEdgeWire struct {
	Cursor string          `json:"cursor"`
	Node   abstractionWire `json:"node"`
}

type abstractionConnectionWire struct {
	PageInfo struct {
		EndCursor   string `json:"endCursor"`
		HasNextPage bool   `json:"hasNextPage"`
	} `json:"pageInfo"`
	Edges []abstractionEdgeWire `json:"edges"`
}

// Categories returns Handshake's live event category vocabulary.
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var response struct {
		Categories []categoryWire `json:"eventAbstractionCategories"`
	}
	if err := c.graphQL(ctx, "GetEventAbstractionCategories", categoriesQuery, map[string]any{}, &response); err != nil {
		return nil, err
	}
	result := make([]Category, len(response.Categories))
	for index, category := range response.Categories {
		result[index] = normalizeCategory(category)
	}
	return result, nil
}

// Events returns one page of student events.
func (c *Client) Events(ctx context.Context, search Search) (Page[EventSummary], error) {
	page, err := c.search(ctx, search, "Event")
	if err != nil {
		return Page[EventSummary]{}, err
	}
	result := Page[EventSummary]{HasMore: page.HasMore, NextCursor: page.NextCursor, CursorKind: page.CursorKind, Sort: page.Sort}
	result.Items = make([]EventSummary, len(page.Items))
	for index, event := range page.Items {
		result.Items[index] = normalizeEventSummary(event)
	}
	return result, nil
}

// CareerFairs returns one page of career fairs from Handshake's unified event search.
func (c *Client) CareerFairs(ctx context.Context, search Search) (Page[CareerFairSummary], error) {
	page, err := c.search(ctx, search, "CareerFair")
	if err != nil {
		return Page[CareerFairSummary]{}, err
	}
	result := Page[CareerFairSummary]{HasMore: page.HasMore, NextCursor: page.NextCursor, CursorKind: page.CursorKind, Sort: page.Sort}
	result.Items = make([]CareerFairSummary, len(page.Items))
	for index, fair := range page.Items {
		result.Items[index] = normalizeCareerFairSummary(fair)
	}
	return result, nil
}

func (c *Client) search(ctx context.Context, search Search, model string) (Page[abstractionWire], error) {
	if search.Limit == 0 {
		search.Limit = 30
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
	sortValue, postedOrder, err := searchSort(search.Sort)
	if err != nil {
		return Page[abstractionWire]{}, err
	}
	params := map[string]any{
		"categories":     categoryIDs,
		"collection":     "ALL",
		"date":           date,
		"keyword":        strings.TrimSpace(search.Keyword),
		"medium":         medium,
		"postedBySchool": search.PostedBySchool,
		"searchModels":   []string{model},
		"sort":           sortValue,
	}
	if postedOrder {
		return c.searchPostedOrder(ctx, search, params)
	}
	return c.searchCursorOrder(ctx, search, params)
}

func (c *Client) searchCursorOrder(ctx context.Context, search Search, params map[string]any) (Page[abstractionWire], error) {
	after := search.After
	result := Page[abstractionWire]{Items: []abstractionWire{}, CursorKind: "graphql", Sort: normalizedSort(search.Sort)}
	for {
		fetchSize := search.Limit
		if search.Organizer != "" && fetchSize < 30 {
			fetchSize = 30
		}
		connection, err := c.fetchAbstractions(ctx, params, fetchSize, after)
		if err != nil {
			return Page[abstractionWire]{}, err
		}
		for edgeIndex, edge := range connection.Edges {
			if search.Organizer != "" {
				matched, err := c.matchesOrganizer(ctx, &edge.Node, modelForParams(params), search.Organizer)
				if err != nil {
					return Page[abstractionWire]{}, err
				}
				if !matched {
					continue
				}
			}
			result.Items = append(result.Items, edge.Node)
			if len(result.Items) == search.Limit {
				result.HasMore = edgeIndex < len(connection.Edges)-1 || connection.PageInfo.HasNextPage
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

func (c *Client) searchPostedOrder(ctx context.Context, search Search, params map[string]any) (Page[abstractionWire], error) {
	var items []abstractionWire
	after := ""
	for {
		// The schema's complexity limit rejects this projection at 100 records.
		// Fifty is comfortably below the live tenant cap while avoiding tiny scans.
		connection, err := c.fetchAbstractions(ctx, params, 50, after)
		if err != nil {
			return Page[abstractionWire]{}, err
		}
		for _, edge := range connection.Edges {
			if search.Organizer != "" {
				matched, err := c.matchesOrganizer(ctx, &edge.Node, modelForParams(params), search.Organizer)
				if err != nil {
					return Page[abstractionWire]{}, err
				}
				if !matched {
					continue
				}
			}
			items = append(items, edge.Node)
		}
		if !connection.PageInfo.HasNextPage {
			break
		}
		after = strings.TrimSpace(connection.PageInfo.EndCursor)
	}
	sort.SliceStable(items, func(i, j int) bool { return numericID(items[i].ID) > numericID(items[j].ID) })
	if search.After != "" {
		cursor, err := strconv.ParseInt(search.After, 10, 64)
		if err != nil || cursor < 1 {
			return Page[abstractionWire]{}, errors.New("after must be an event ID when sort is posted-desc")
		}
		filtered := items[:0]
		for _, item := range items {
			if numericID(item.ID) < cursor {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	page := Page[abstractionWire]{Items: items, CursorKind: "id", Sort: "posted-desc"}
	if len(page.Items) > search.Limit {
		page.Items = page.Items[:search.Limit]
		page.HasMore = true
	}
	if page.HasMore && len(page.Items) > 0 {
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (c *Client) fetchAbstractions(ctx context.Context, params map[string]any, first int, after string) (abstractionConnectionWire, error) {
	// The student site requests three employer previews per search card. Matching
	// that limit also keeps large monitoring pages under GraphQL's complexity cap.
	variables := map[string]any{"params": params, "first": first, "maxEmployersCount": 3}
	if after != "" {
		variables["after"] = after
	}
	var response struct {
		Connection abstractionConnectionWire `json:"eventAbstractions"`
	}
	err := c.graphQL(ctx, "GetEventAbstractions", eventAbstractionsQuery, variables, &response)
	return response.Connection, err
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

func searchSort(value string) (string, bool, error) {
	switch normalizedSort(value) {
	case "relevance":
		return "RELEVANCE", false, nil
	case "date":
		return "DATE", false, nil
	case "posted-desc":
		// Handshake exposes no event posting timestamp and rejects every posting
		// sort enum. Event IDs are monotonic, so descending IDs are the useful
		// monitoring order while the server still supplies the complete result set.
		return "DATE", true, nil
	default:
		return "", false, errors.New("sort must be relevance, date, or posted-desc")
	}
}

func normalizedSort(value string) string {
	if value == "" {
		return "relevance"
	}
	return slug(value)
}

func matchesOrganizer(event abstractionWire, value string) bool {
	needle := strings.ToLower(strings.TrimSpace(value))
	if needle == "" {
		return true
	}
	if strings.Contains(strings.ToLower(event.Host.Name), needle) {
		return true
	}
	for _, contact := range event.Contacts {
		if strings.Contains(strings.ToLower(contact.EmailAddress), needle) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(contact.FirstName+" "+contact.LastName)), needle) {
			return true
		}
	}
	for _, employer := range event.Employers {
		if strings.Contains(strings.ToLower(employer.Name), needle) {
			return true
		}
	}
	return false
}

func (c *Client) matchesOrganizer(ctx context.Context, event *abstractionWire, model, value string) (bool, error) {
	if matchesOrganizer(*event, value) {
		return true, nil
	}
	id, err := strconv.Atoi(event.ID)
	if err != nil || id < 1 {
		return false, nil
	}
	needle := strings.ToLower(strings.TrimSpace(value))
	if model == "CareerFair" {
		fair, err := c.CareerFair(ctx, id)
		if err != nil {
			return false, err
		}
		return strings.Contains(strings.ToLower(fair.Contact.Name), needle) ||
			strings.Contains(strings.ToLower(fair.Contact.Email), needle), nil
	}
	detail, err := c.Event(ctx, id)
	if err != nil {
		return false, err
	}
	event.EventType.Name = detail.EventType
	event.Contacts = make([]contactWire, len(detail.Contacts))
	for index, contact := range detail.Contacts {
		event.Contacts[index] = contactWire{ID: contact.ID, Title: contact.Title, EmailAddress: contact.Email}
		parts := strings.Fields(contact.Name)
		if len(parts) > 0 {
			event.Contacts[index].FirstName = parts[0]
			event.Contacts[index].LastName = strings.Join(parts[1:], " ")
		}
		if strings.Contains(strings.ToLower(contact.Name), needle) || strings.Contains(strings.ToLower(contact.Email), needle) {
			return true, nil
		}
	}
	return false, nil
}

func modelForParams(params map[string]any) string {
	models, _ := params["searchModels"].([]string)
	if len(models) == 1 {
		return models[0]
	}
	return "Event"
}

func numericID(value string) int64 {
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

func normalizeCategory(category categoryWire) Category {
	return Category{ID: category.ID, Name: category.Name, Slug: slug(category.Name)}
}

func normalizeCategories(categories []categoryWire) []Category {
	result := make([]Category, len(categories))
	for index, category := range categories {
		result[index] = normalizeCategory(category)
	}
	return result
}

func normalizeEmployers(employers []employerWire) []Employer {
	result := make([]Employer, len(employers))
	for index, employer := range employers {
		result[index] = Employer{ID: employer.ID, Name: employer.Name, Logo: employer.Logo.URL, Industry: employer.Industry.Name, Location: employer.Location.Name}
	}
	return result
}

func normalizeContacts(contacts []contactWire) []Contact {
	result := make([]Contact, len(contacts))
	for index, contact := range contacts {
		result[index] = Contact{ID: contact.ID, Name: strings.TrimSpace(contact.FirstName + " " + contact.LastName), Title: contact.Title, Email: contact.EmailAddress, Photo: contact.User.Photo}
	}
	return result
}

func normalizeHost(host hostWire) Organization {
	logo := host.LogoURL
	if logo == "" {
		logo = host.Logo.URL
	}
	hostType := host.Type
	if hostType == "" {
		hostType = host.Typename
	}
	return Organization{ID: host.ID, Name: host.Name, Type: strings.ToLower(hostType), Logo: logo}
}

func normalizeEventSummary(event abstractionWire) EventSummary {
	return EventSummary{
		ID: event.ID, Name: event.Name, StartDate: event.StartDate, EndDate: event.EndDate,
		Medium: strings.ToLower(event.Medium), EventType: event.EventType.Name,
		Categories: normalizeCategories(event.Categories), Host: normalizeHost(event.Host),
		Employers: normalizeEmployers(event.Employers), Contacts: normalizeContacts(event.Contacts),
		Favorited: event.Favorited, Registered: event.Registered, SameSchoolHost: event.SameSchoolHost,
		SameSchoolEvent: event.SameSchoolEvent, RegistrationStart: event.RegistrationStart,
		RegistrationEnd: event.RegistrationEnd, RegisteredAttendees: event.RegisteredAttendees,
		URL: baseURL + "/stu/events/" + event.ID,
	}
}

func normalizeCareerFairSummary(fair abstractionWire) CareerFairSummary {
	medium := fair.CareerFairMedium
	if medium == "" {
		medium = fair.Medium
	}
	return CareerFairSummary{
		ID: fair.ID, Name: fair.Name, StartDate: fair.StartDate, EndDate: fair.EndDate,
		Medium: strings.ToLower(medium), Categories: normalizeCategories(fair.Categories),
		Host: normalizeHost(fair.Host), Employers: normalizeEmployers(fair.Employers),
		Favorited: fair.Favorited, Registered: fair.Registered, SameSchoolHost: fair.SameSchoolHost,
		RegistrationStart: fair.RegistrationStart, RegistrationEnd: fair.RegistrationEnd,
		RegisteredAttendees: fair.RegisteredAttendees, URL: baseURL + "/stu/career_fairs/" + fair.ID,
	}
}

type eventDetailWire struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Description          string    `json:"description"`
	StartDate            string    `json:"startDate"`
	EndDate              string    `json:"endDate"`
	EventMediumType      string    `json:"eventMediumType"`
	ExternalLink         *string   `json:"externalLink"`
	ExternalRegistration bool      `json:"externalRegistration"`
	LegalDisclaimer      string    `json:"legalDisclaimer"`
	RegistrationEnd      *string   `json:"studentRegistrationEnd"`
	RegistrationStart    *string   `json:"studentRegistrationStart"`
	VirtualLink          *string   `json:"virtualLink"`
	AvailableSpace       *int      `json:"availableSpace"`
	WaitlistEnabled      bool      `json:"waitlistEnabled"`
	WaitlistedCount      int       `json:"waitlistedCount"`
	RegisteredAttendees  int       `json:"registeredAttendeesCount"`
	TimeZone             string    `json:"timeZone"`
	Image                imageWire `json:"image"`
	EventType            struct {
		Name string `json:"name"`
	} `json:"eventType"`
	Room struct {
		Name     string `json:"name"`
		Building struct {
			Name     string `json:"name"`
			Location struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"building"`
	} `json:"room"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	Owner    hostWire `json:"owner"`
	Host     hostWire `json:"host"`
	Attendee *struct {
		Registered  bool `json:"registered"`
		Registering bool `json:"registering"`
		Waitlisted  bool `json:"waitlisted"`
		CheckedIn   bool `json:"checkedIn"`
	} `json:"attendee"`
	Categories []categoryWire `json:"categories"`
	Contacts   []contactWire  `json:"contacts"`
	Employers  []employerWire `json:"employers"`
}

// Event returns one complete event record by ID.
func (c *Client) Event(ctx context.Context, id int) (EventDetail, error) {
	if id < 1 {
		return EventDetail{}, errors.New("event ID must be a positive integer")
	}
	var response struct {
		Event *eventDetailWire `json:"event"`
	}
	if err := c.graphQLAt(ctx, "/hs/graphql", "GetEvent", eventQuery, map[string]any{"id": strconv.Itoa(id)}, &response); err != nil {
		return EventDetail{}, err
	}
	if response.Event == nil {
		return EventDetail{}, fmt.Errorf("event %d was not found", id)
	}
	event := response.Event
	detail := EventDetail{
		ID: event.ID, Name: event.Name, Description: plainText(event.Description), DescriptionHTML: event.Description,
		StartDate: event.StartDate, EndDate: event.EndDate, TimeZone: event.TimeZone,
		EventType: event.EventType.Name, Medium: strings.ToLower(event.EventMediumType),
		Location: Location{Name: event.Location.Name, Building: event.Room.Building.Name, Room: event.Room.Name},
		Owner:    normalizeHost(event.Owner), Host: normalizeHost(event.Host), Categories: normalizeCategories(event.Categories),
		Contacts: normalizeContacts(event.Contacts), Employers: normalizeEmployers(event.Employers),
		RegistrationStart: event.RegistrationStart, RegistrationEnd: event.RegistrationEnd,
		AvailableSpace: event.AvailableSpace, WaitlistEnabled: event.WaitlistEnabled,
		WaitlistedCount: event.WaitlistedCount, RegisteredAttendees: event.RegisteredAttendees,
		VirtualLink: event.VirtualLink, ExternalLink: event.ExternalLink,
		ExternalRegistration: event.ExternalRegistration, LegalDisclaimer: event.LegalDisclaimer,
		Image: event.Image.URL, URL: baseURL + "/stu/events/" + event.ID,
	}
	if detail.Location.Name == "" {
		detail.Location.Name = event.Room.Building.Location.Name
	}
	if event.Attendee != nil {
		detail.Registration = &Registration{Registered: event.Attendee.Registered, Registering: event.Attendee.Registering, Waitlisted: event.Attendee.Waitlisted, CheckedIn: event.Attendee.CheckedIn}
	}
	return detail, nil
}
