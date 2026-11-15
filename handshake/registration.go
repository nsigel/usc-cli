package handshake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/nsigel/usc-cli/internal/dom"
	http "github.com/saucesteals/fhttp"
	"golang.org/x/net/html"
)

type attendeeWire struct {
	ID             int    `json:"id"`
	AttendableID   int    `json:"attendable_id"`
	AttendableName string `json:"attendable_name"`
	Registered     bool   `json:"registered"`
	Waitlisted     bool   `json:"waitlisted"`
}

type attendeeResultWire struct {
	Success      bool         `json:"success"`
	Errors       any          `json:"errors"`
	RedirectPath string       `json:"redirect_path"`
	Attendee     attendeeWire `json:"attendee"`
}

func (r attendeeResultWire) registration() CareerFairRegistration {
	return CareerFairRegistration{AttendeeID: r.Attendee.ID, SessionID: r.Attendee.AttendableID,
		SessionName: r.Attendee.AttendableName, Registered: r.Attendee.Registered, Waitlisted: r.Attendee.Waitlisted}
}

// RegisterCareerFairSession registers the current user for a career-fair
// session, or puts them on its waitlist. Session IDs come from CareerFair.
func (c *Client) RegisterCareerFairSession(ctx context.Context, sessionID int) (CareerFairRegistration, error) {
	label := "register for career fair session " + strconv.Itoa(sessionID)
	var result attendeeResultWire
	path := "/stu/career_fair_sessions/" + strconv.Itoa(sessionID) + "/attendees.json"
	if err := c.changeAttendance(ctx, http.MethodPost, path, []byte(`{"attendee":{}}`), label, &result); err != nil {
		return CareerFairRegistration{}, err
	}
	if !result.Attendee.Registered && !result.Attendee.Waitlisted {
		message := label + ": Handshake did not register you"
		if result.Errors != nil {
			message += fmt.Sprintf(": %v", result.Errors)
		}
		if result.RedirectPath != "" {
			message += "; finish at " + baseURL + result.RedirectPath
		}
		return CareerFairRegistration{}, errors.New(message)
	}
	return result.registration(), nil
}

// CancelCareerFairSession cancels the current user's registration or waitlist
// spot for a career-fair session. Cancelling a session the user is not
// registered for succeeds and returns the unregistered record.
func (c *Client) CancelCareerFairSession(ctx context.Context, sessionID int) (CareerFairRegistration, error) {
	label := "cancel career fair session " + strconv.Itoa(sessionID)
	var results map[string]attendeeResultWire
	path := "/stu/career_fair_sessions/attendees?" + url.Values{"attendable_ids[]": {strconv.Itoa(sessionID)}}.Encode()
	if err := c.changeAttendance(ctx, http.MethodDelete, path, nil, label, &results); err != nil {
		return CareerFairRegistration{}, err
	}
	for _, result := range results {
		if result.Success && result.Attendee.AttendableID == sessionID {
			return result.registration(), nil
		}
	}
	return CareerFairRegistration{}, errors.New(label + ": Handshake did not confirm the cancellation")
}

// Handshake rejects attendee changes without the CSRF token in its HTML pages.
func (c *Client) changeAttendance(ctx context.Context, method, path string, payload []byte, label string, destination any) error {
	page, err := c.getHTML(ctx, "/stu/events", label)
	if err != nil {
		return err
	}
	token := dom.Attr(dom.FindFirst(dom.Parse(page), func(node *html.Node) bool {
		return node.Data == "meta" && dom.Attr(node, "name") == "csrf-token"
	}), "content")
	if token == "" {
		return errors.New(label + ": Handshake page has no CSRF token")
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", baseURL)
	request.Header.Set("Referer", baseURL+"/stu/events")
	request.Header.Set("X-CSRF-Token", token)
	body, err := c.send(request, label)
	var failure *Error
	if errors.As(err, &failure) && failure.Status == http.StatusNotFound {
		return errors.New(label + ": no such session")
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("%s: decode response: %w", label, err)
	}
	return nil
}
