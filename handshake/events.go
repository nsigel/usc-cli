package handshake

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Categories returns Handshake's live event category vocabulary.
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var response struct {
		Categories []categoryWire `json:"eventAbstractionCategories"`
	}
	if err := c.graphQL(ctx, searchGraphQL, "GetEventAbstractionCategories", categoriesQuery, map[string]any{}, &response); err != nil {
		return nil, err
	}
	return normalizeCategories(response.Categories), nil
}

// Events returns one page of student events.
func (c *Client) Events(ctx context.Context, search Search) (Page[EventSummary], error) {
	page, err := c.search(ctx, search, "Event")
	return convertPage(page, normalizeEventSummary), err
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
	if err := c.graphQL(ctx, detailGraphQL, "GetEvent", eventQuery, map[string]any{"id": strconv.Itoa(id)}, &response); err != nil {
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
