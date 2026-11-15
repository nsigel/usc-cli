package handshake

import (
	"cmp"
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
	Success  bool            `json:"success"`
	Errors   json.RawMessage `json:"errors"`
	Attendee struct {
		ID             int    `json:"id"`
		AttendableID   int    `json:"attendable_id"`
		AttendableName string `json:"attendable_name"`
		Registered     bool   `json:"registered"`
		Waitlisted     bool   `json:"waitlisted"`
	} `json:"attendee"`
}

func (w attendeeWire) registration() CareerFairRegistration {
	return CareerFairRegistration{AttendeeID: w.Attendee.ID, SessionID: w.Attendee.AttendableID,
		SessionName: w.Attendee.AttendableName, Registered: w.Attendee.Registered, Waitlisted: w.Attendee.Waitlisted}
}

// RegisterCareerFairSession registers the current user for a career-fair
// session, or waitlists them. Session IDs come from CareerFair.
func (c *Client) RegisterCareerFairSession(ctx context.Context, sessionID int) (CareerFairRegistration, error) {
	label := "register for career fair session " + strconv.Itoa(sessionID)
	var result attendeeWire
	path := "/stu/career_fair_sessions/" + strconv.Itoa(sessionID) + "/attendees.json"
	if err := c.changeAttendance(ctx, http.MethodPost, path, []byte(`{"attendee":{}}`), label, &result); err != nil {
		return CareerFairRegistration{}, err
	}
	if !result.Attendee.Registered && !result.Attendee.Waitlisted {
		return CareerFairRegistration{}, fmt.Errorf("%s: Handshake did not register you: %s", label, cmp.Or(string(result.Errors), "no reason given"))
	}
	return result.registration(), nil
}

// CancelCareerFairSession cancels the current user's registration or waitlist
// spot for a career-fair session. Cancelling an unregistered session succeeds.
func (c *Client) CancelCareerFairSession(ctx context.Context, sessionID int) (CareerFairRegistration, error) {
	label := "cancel career fair session " + strconv.Itoa(sessionID)
	var results map[string]attendeeWire // keyed by attendee ID
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

func (c *Client) changeAttendance(ctx context.Context, method, path string, payload []byte, label string, destination any) error {
	token, err := c.csrfToken(ctx)
	if err != nil {
		return err
	}
	body, err := c.sendJSON(ctx, method, path, payload, label, token)
	if failure := (*Error)(nil); errors.As(err, &failure) && failure.Status == http.StatusNotFound {
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

// csrfToken reads Rails' token from a student page; /stu/events is one that has it.
func (c *Client) csrfToken(ctx context.Context) (string, error) {
	page, err := c.getHTML(ctx, "/stu/events", "CSRF token")
	if err != nil {
		return "", err
	}
	token := dom.Attr(dom.FindFirst(dom.Parse(page), func(node *html.Node) bool {
		return node.Data == "meta" && dom.Attr(node, "name") == "csrf-token"
	}), "content")
	if token == "" {
		return "", errors.New("handshake page has no CSRF token")
	}
	return token, nil
}
