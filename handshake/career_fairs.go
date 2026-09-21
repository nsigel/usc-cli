package handshake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

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

type careerFairPageWire struct {
	Page struct {
		CareerFair careerFairWire `json:"career_fair"`
		Sessions   []struct {
			CareerFairSession careerFairSessionWire `json:"career_fair_session"`
		} `json:"career_fair_sessions"`
	} `json:"page"`
}

// CareerFair returns the complete student-facing career-fair page by ID.
func (c *Client) CareerFair(ctx context.Context, id int) (CareerFairDetail, error) {
	if id < 1 {
		return CareerFairDetail{}, errors.New("career fair ID must be a positive integer")
	}
	body, err := c.getHTML(ctx, "/stu/career_fairs/"+strconv.Itoa(id), "GetCareerFair")
	if err != nil {
		return CareerFairDetail{}, err
	}
	encoded, err := reactProps(body, "CareerFairsShowRoot")
	if err != nil {
		return CareerFairDetail{}, fmt.Errorf("career fair %d: %w", id, err)
	}
	var props careerFairPageWire
	if err := json.Unmarshal([]byte(encoded), &props); err != nil {
		return CareerFairDetail{}, fmt.Errorf("career fair %d: decode page data: %w", id, err)
	}
	fair := props.Page.CareerFair
	if fair.ID == 0 {
		return CareerFairDetail{}, fmt.Errorf("career fair %d was not found", id)
	}
	detail := CareerFairDetail{
		ID: fair.ID, Name: fair.Name, Description: plainText(fair.Description), DescriptionHTML: fair.Description,
		CreatedAt: fair.CreatedAt, UpdatedAt: fair.UpdatedAt, StartDate: fair.StartDate, EndDate: fair.EndDate,
		TimeZone: firstNonempty(fair.TimeZone, fair.MomentTimeZone), Medium: strings.ToLower(fair.Medium),
		Location: Location{Name: fair.Location.Name, City: fair.Location.City, State: fair.Location.State,
			Country: fair.Location.Country, Latitude: fair.Location.Latitude, Longitude: fair.Location.Longitude},
		Institution:       Organization{ID: strconv.Itoa(fair.InstitutionID), Name: fair.InstitutionName, Type: "school", Logo: fair.InstitutionLogo},
		Contact:           Contact{Name: fair.ContactName, Title: fair.ContactTitle, Email: fair.ContactEmail},
		RegistrationStart: fair.RegistrationStart, RegistrationEnd: fair.RegistrationEnd,
		RegistrationOpen: fair.RegistrationOpen, RegisteredCount: fair.RegisteredCount,
		CheckedInCount: fair.CheckedInCount, EmployerCount: fair.EmployerCount,
		AvailableSpace: fair.AvailableSpace, WaitlistEnabled: fair.WaitlistEnabled, InviteOnly: fair.InviteOnly,
		Public: fair.Public, Status: fair.Status, ExternalLink: fair.ExternalLink, Logo: fair.Logo,
		Sessions: make([]CareerFairSession, len(props.Page.Sessions)), URL: baseURL + "/stu/career_fairs/" + strconv.Itoa(fair.ID),
	}
	for index, wrapped := range props.Page.Sessions {
		session := wrapped.CareerFairSession
		detail.Sessions[index] = CareerFairSession{
			ID: session.ID, Name: session.Name, StartDate: session.StartDate, EndDate: session.EndDate,
			TimeZone: session.TimeZone, Type: session.Type, LocationType: session.LocationType,
			AvailableSpace: session.AvailableSpace, ExternalLink: session.ExternalLink,
			RegistrationOpen: session.RegistrationPeriod, WaitlistEnabled: session.WaitlistEnabled,
		}
	}
	return detail, nil
}

func reactProps(document []byte, reactClass string) (string, error) {
	root, err := html.Parse(bytes.NewReader(document))
	if err != nil {
		return "", err
	}
	var visit func(*html.Node) string
	visit = func(node *html.Node) string {
		if node.Type == html.ElementNode {
			matched := false
			props := ""
			for _, attribute := range node.Attr {
				switch attribute.Key {
				case "data-react-class":
					matched = attribute.Val == reactClass
				case "data-react-props":
					props = attribute.Val
				}
			}
			if matched && props != "" {
				return props
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if value := visit(child); value != "" {
				return value
			}
		}
		return ""
	}
	if props := visit(root); props != "" {
		return props, nil
	}
	return "", errors.New("page did not contain CareerFairsShowRoot data")
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
