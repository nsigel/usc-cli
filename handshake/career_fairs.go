package handshake

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nsigel/usc-cli/internal/dom"
	"golang.org/x/net/html"
)

type careerFairWire struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"student_description"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	StartDate         string `json:"start_date"`
	EndDate           string `json:"end_date"`
	TimeZone          string `json:"time_zone"`
	MomentTimeZone    string `json:"moment_time_zone"`
	Medium            string `json:"career_fair_medium"`
	InstitutionID     int    `json:"institution_id"`
	InstitutionName   string `json:"institution_name"`
	InstitutionLogo   string `json:"institution_logo_url"`
	ContactName       string `json:"contact_name"`
	ContactEmail      string `json:"contact_email"`
	ContactTitle      string `json:"contact_title"`
	RegistrationStart string `json:"student_registration_start"`
	RegistrationEnd   string `json:"student_registration_end"`
	RegistrationOpen  bool   `json:"in_student_registration_period?"`
	RegisteredCount   int    `json:"registered_count"`
	CheckedInCount    int    `json:"checked_in_count"`
	EmployerCount     int    `json:"employer_registrations_count"`
	AvailableSpace    *int   `json:"available_space"`
	WaitlistEnabled   bool   `json:"waitlist_enabled"`
	InviteOnly        bool   `json:"invite_only"`
	Public            bool   `json:"public"`
	Status            string `json:"status"`
	ExternalLink      string `json:"external_link"`
	Logo              string `json:"smart_logo"`
	Location          struct {
		Name      string   `json:"name"`
		City      string   `json:"city"`
		State     string   `json:"state"`
		Country   string   `json:"country"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"location"`
}

type careerFairSessionWire struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	StartDate          string `json:"start_date_time"`
	EndDate            string `json:"end_date_time"`
	TimeZone           string `json:"moment_time_zone"`
	Type               string `json:"session_type"`
	LocationType       string `json:"location_type"`
	AvailableSpace     *int   `json:"available_space"`
	ExternalLink       string `json:"external_link"`
	RegistrationPeriod bool   `json:"in_student_registration_period?"`
	WaitlistEnabled    bool   `json:"waitlist_enabled?"`
}

// careerFairPageWire covers both layouts of the career-fair page's embedded
// data: in-person fairs nest it under "page", virtual fairs under "entities".
type careerFairPageWire struct {
	Page struct {
		CareerFair careerFairWire `json:"career_fair"`
		Sessions   []struct {
			Session careerFairSessionWire `json:"career_fair_session"`
		} `json:"career_fair_sessions"`
	} `json:"page"`
	Entities struct {
		CareerFair careerFairWire          `json:"career_fairs"`
		Sessions   []careerFairSessionWire `json:"career_fair_sessions"`
	} `json:"entities"`
}

func (p careerFairPageWire) fair() (careerFairWire, []careerFairSessionWire) {
	if p.Page.CareerFair.ID == 0 {
		return p.Entities.CareerFair, p.Entities.Sessions
	}
	sessions := make([]careerFairSessionWire, len(p.Page.Sessions))
	for index, wrapped := range p.Page.Sessions {
		sessions[index] = wrapped.Session
	}
	return p.Page.CareerFair, sessions
}

// CareerFairs returns one page of career fairs.
func (c *Client) CareerFairs(ctx context.Context, search Search) (Page[CareerFairSummary], error) {
	page, err := c.search(ctx, search, "CareerFair")
	return convertPage(page, normalizeCareerFairSummary), err
}

// CareerFair returns the complete student-facing career-fair page by ID.
func (c *Client) CareerFair(ctx context.Context, id int) (CareerFairDetail, error) {
	if id < 1 {
		return CareerFairDetail{}, errors.New("career fair ID must be a positive integer")
	}
	body, err := c.getHTML(ctx, "/stu/career_fairs/"+strconv.Itoa(id), "career fair "+strconv.Itoa(id))
	if err != nil {
		return CareerFairDetail{}, err
	}
	encoded := careerFairProps(body)
	if encoded == "" {
		return CareerFairDetail{}, fmt.Errorf("career fair %d: page has no career-fair data", id)
	}
	var props careerFairPageWire
	if err := json.Unmarshal([]byte(encoded), &props); err != nil {
		return CareerFairDetail{}, fmt.Errorf("career fair %d: decode page data: %w", id, err)
	}
	fair, sessions := props.fair()
	if fair.ID == 0 {
		return CareerFairDetail{}, fmt.Errorf("career fair %d was not found", id)
	}
	detail := CareerFairDetail{
		ID: fair.ID, Name: fair.Name, Description: plainText(fair.Description), DescriptionHTML: fair.Description,
		CreatedAt: fair.CreatedAt, UpdatedAt: fair.UpdatedAt, StartDate: fair.StartDate, EndDate: fair.EndDate,
		TimeZone: cmp.Or(fair.TimeZone, fair.MomentTimeZone), Medium: strings.ToLower(fair.Medium),
		Location: Location{Name: fair.Location.Name, City: fair.Location.City, State: fair.Location.State,
			Country: fair.Location.Country, Latitude: fair.Location.Latitude, Longitude: fair.Location.Longitude},
		Institution:       Organization{ID: strconv.Itoa(fair.InstitutionID), Name: fair.InstitutionName, Type: "school", Logo: fair.InstitutionLogo},
		Contact:           Contact{Name: fair.ContactName, Title: fair.ContactTitle, Email: fair.ContactEmail},
		RegistrationStart: fair.RegistrationStart, RegistrationEnd: fair.RegistrationEnd,
		RegistrationOpen: fair.RegistrationOpen, RegisteredCount: fair.RegisteredCount,
		CheckedInCount: fair.CheckedInCount, EmployerCount: fair.EmployerCount,
		AvailableSpace: fair.AvailableSpace, WaitlistEnabled: fair.WaitlistEnabled, InviteOnly: fair.InviteOnly,
		Public: fair.Public, Status: fair.Status, ExternalLink: fair.ExternalLink, Logo: fair.Logo,
		Sessions: make([]CareerFairSession, len(sessions)), URL: baseURL + "/stu/career_fairs/" + strconv.Itoa(fair.ID),
	}
	for index, session := range sessions {
		detail.Sessions[index] = CareerFairSession{
			ID: session.ID, Name: session.Name, StartDate: session.StartDate, EndDate: session.EndDate,
			TimeZone: session.TimeZone, Type: session.Type, LocationType: session.LocationType,
			AvailableSpace: session.AvailableSpace, ExternalLink: session.ExternalLink,
			RegistrationOpen: session.RegistrationPeriod, WaitlistEnabled: session.WaitlistEnabled,
		}
	}
	return detail, nil
}

// careerFairProps returns the JSON the page embeds for its React root, or ""
// when the page has none.
func careerFairProps(document []byte) string {
	root := dom.FindFirst(dom.Parse(document), func(node *html.Node) bool {
		class := dom.Attr(node, "data-react-class")
		return class == "CareerFairsShowRoot" || class == "StudentCareerFairsShowRoot"
	})
	return dom.Attr(root, "data-react-props")
}
