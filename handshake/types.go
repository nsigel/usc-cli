package handshake

// Category is a Handshake event-search category.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Organization identifies an event host or owner.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Logo string `json:"logo,omitempty"`
}

// Employer is an employer attached to an event or career fair.
type Employer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Logo     string `json:"logo,omitempty"`
	Industry string `json:"industry,omitempty"`
	Location string `json:"location,omitempty"`
}

// Contact is an event organizer or contact.
type Contact struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	Email string `json:"email,omitempty"`
	Photo string `json:"photo,omitempty"`
}

// EventSummary is one event-search result.
type EventSummary struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	StartDate           string       `json:"start_date"`
	EndDate             string       `json:"end_date"`
	Medium              string       `json:"medium"`
	EventType           string       `json:"event_type,omitempty"`
	Categories          []Category   `json:"categories"`
	Host                Organization `json:"host"`
	Employers           []Employer   `json:"employers"`
	Contacts            []Contact    `json:"contacts"`
	Favorited           bool         `json:"favorited"`
	Registered          bool         `json:"registered"`
	SameSchoolHost      bool         `json:"same_school_host"`
	SameSchoolEvent     bool         `json:"same_school_event"`
	RegistrationStart   *string      `json:"registration_start"`
	RegistrationEnd     *string      `json:"registration_end"`
	RegisteredAttendees int          `json:"registered_attendees"`
	URL                 string       `json:"url"`
}

// CareerFairSummary is one career-fair search result.
type CareerFairSummary struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	StartDate           string       `json:"start_date"`
	EndDate             string       `json:"end_date"`
	Medium              string       `json:"medium"`
	Categories          []Category   `json:"categories"`
	Host                Organization `json:"host"`
	Employers           []Employer   `json:"employers"`
	Favorited           bool         `json:"favorited"`
	Registered          bool         `json:"registered"`
	SameSchoolHost      bool         `json:"same_school_host"`
	RegistrationStart   *string      `json:"registration_start"`
	RegistrationEnd     *string      `json:"registration_end"`
	RegisteredAttendees int          `json:"registered_attendees"`
	URL                 string       `json:"url"`
}

// Registration summarizes the current user's event registration state.
type Registration struct {
	Registered  bool `json:"registered"`
	Registering bool `json:"registering"`
	Waitlisted  bool `json:"waitlisted"`
	CheckedIn   bool `json:"checked_in"`
}

// Location is a normalized event or career-fair location.
type Location struct {
	Name      string   `json:"name"`
	Building  string   `json:"building,omitempty"`
	Room      string   `json:"room,omitempty"`
	City      string   `json:"city,omitempty"`
	State     string   `json:"state,omitempty"`
	Country   string   `json:"country,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// EventDetail is the complete student-facing event record.
type EventDetail struct {
	ID                   string        `json:"id"`
	Name                 string        `json:"name"`
	Description          string        `json:"description"`
	DescriptionHTML      string        `json:"description_html"`
	StartDate            string        `json:"start_date"`
	EndDate              string        `json:"end_date"`
	TimeZone             string        `json:"time_zone"`
	EventType            string        `json:"event_type"`
	Medium               string        `json:"medium"`
	Location             Location      `json:"location"`
	Owner                Organization  `json:"owner"`
	Host                 Organization  `json:"host"`
	Categories           []Category    `json:"categories"`
	Contacts             []Contact     `json:"contacts"`
	Employers            []Employer    `json:"employers"`
	Registration         *Registration `json:"registration,omitempty"`
	RegistrationStart    *string       `json:"registration_start"`
	RegistrationEnd      *string       `json:"registration_end"`
	AvailableSpace       *int          `json:"available_space"`
	WaitlistEnabled      bool          `json:"waitlist_enabled"`
	WaitlistedCount      int           `json:"waitlisted_count"`
	RegisteredAttendees  int           `json:"registered_attendees"`
	VirtualLink          *string       `json:"virtual_link"`
	ExternalLink         *string       `json:"external_link"`
	ExternalRegistration bool          `json:"external_registration"`
	LegalDisclaimer      string        `json:"legal_disclaimer,omitempty"`
	Image                string        `json:"image,omitempty"`
	URL                  string        `json:"url"`
}

// CareerFairSession is one component session of a career fair.
type CareerFairSession struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date"`
	TimeZone         string `json:"time_zone"`
	Type             string `json:"type,omitempty"`
	LocationType     string `json:"location_type,omitempty"`
	AvailableSpace   *int   `json:"available_space"`
	ExternalLink     string `json:"external_link,omitempty"`
	RegistrationOpen bool   `json:"registration_open"`
	WaitlistEnabled  bool   `json:"waitlist_enabled"`
}

// CareerFairDetail is the complete student-facing career-fair record.
type CareerFairDetail struct {
	ID                int                 `json:"id"`
	Name              string              `json:"name"`
	Description       string              `json:"description"`
	DescriptionHTML   string              `json:"description_html"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
	StartDate         string              `json:"start_date"`
	EndDate           string              `json:"end_date"`
	TimeZone          string              `json:"time_zone"`
	Medium            string              `json:"medium"`
	Location          Location            `json:"location"`
	Institution       Organization        `json:"institution"`
	Contact           Contact             `json:"contact"`
	RegistrationStart string              `json:"registration_start"`
	RegistrationEnd   string              `json:"registration_end"`
	RegistrationOpen  bool                `json:"registration_open"`
	RegisteredCount   int                 `json:"registered_count"`
	CheckedInCount    int                 `json:"checked_in_count"`
	EmployerCount     int                 `json:"employer_count"`
	AvailableSpace    *int                `json:"available_space"`
	WaitlistEnabled   bool                `json:"waitlist_enabled"`
	InviteOnly        bool                `json:"invite_only"`
	Public            bool                `json:"public"`
	Status            string              `json:"status"`
	ExternalLink      string              `json:"external_link,omitempty"`
	Logo              string              `json:"logo,omitempty"`
	Sessions          []CareerFairSession `json:"sessions"`
	URL               string              `json:"url"`
}

// Search is shared by event and career-fair list operations.
type Search struct {
	Categories     []string
	Organizer      string
	Keyword        string
	Medium         string
	Date           string
	Sort           string
	PostedBySchool bool
	Limit          int
	After          string
}

// Page is one cursor-based result page.
type Page[T any] struct {
	Items      []T    `json:"items"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	CursorKind string `json:"cursor_kind"`
	Sort       string `json:"sort"`
}
