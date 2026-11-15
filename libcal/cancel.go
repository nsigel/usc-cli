package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/internal/dom"
	http "github.com/saucesteals/fhttp"
	"golang.org/x/net/html"
)

// Booking is one reservation listed under a cancellation link.
type Booking struct {
	ID         int       `json:"id"`
	Space      string    `json:"space"`
	Category   string    `json:"category"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Cancelable bool      `json:"cancelable"`
}

// Cancellation is LibCal's answer to a canceled booking.
type Cancellation struct {
	BookingID     int  `json:"booking_id"`
	Canceled      bool `json:"canceled"`
	MoreRemaining bool `json:"more_remaining"`
}

var errInvalidCancelLink = &Error{Code: "libcal_cancel_link_invalid", Message: "LibCal does not recognize the cancellation link"}

var cancelCodePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// CancelCode returns the code in a LibCal cancellation link, such as
// https://libcal.usc.edu/equipment/cancel?id=ab4448afc8ed from a
// confirmation email. The code alone is the only key to its bookings.
func CancelCode(link string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(link))
	invalid := errors.New("expected a LibCal cancellation link such as https://libcal.usc.edu/equipment/cancel?id=CODE")
	if err != nil || parsed.Scheme != "https" || parsed.Host != "libcal.usc.edu" || !strings.HasSuffix(parsed.Path, "/cancel") {
		return "", invalid
	}
	code := parsed.Query().Get("id")
	if !cancelCodePattern.MatchString(code) {
		return "", invalid
	}
	return code, nil
}

// Bookings lists the reservations a cancellation code covers. Canceled and
// past bookings stay listed but are not cancelable.
func (c *Client) Bookings(ctx context.Context, code string) ([]Booking, error) {
	if !cancelCodePattern.MatchString(code) {
		return nil, errors.New("invalid LibCal cancellation code")
	}
	data, err := c.request(ctx, http.MethodGet, "/equipment/cancel?id="+code, nil, "text/html", "")
	var httpError *ResponseError
	if errors.As(err, &httpError) && httpError.Status == http.StatusNotFound {
		return nil, errInvalidCancelLink
	}
	if err != nil {
		return nil, err
	}
	return parseBookings(data)
}

func parseBookings(data []byte) ([]Booking, error) {
	root := dom.Parse(data)
	if dom.FindFirst(root, dom.ID("s-lc-eq-cancel-content")) == nil {
		return nil, errInvalidCancelLink
	}
	bookings := []Booking{}
	for _, node := range dom.FindAll(root, dom.Class("div", "s-lc-eq-cancel-booking")) {
		id, err := strconv.Atoi(strings.TrimPrefix(dom.Attr(node, "id"), "booking_"))
		if err != nil {
			return nil, errors.New("LibCal listed a booking without an ID")
		}
		start, end, err := parseBookingTimes(textOf(node, dom.Class("div", "s-lc-eq-cancel-booking-times")))
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, Booking{
			ID:         id,
			Space:      textOf(node, dom.Tag("h2")),
			Category:   textOf(node, dom.Class("div", "s-lc-eq-cancel-booking-category")),
			Start:      start,
			End:        end,
			Cancelable: dom.FindFirst(node, dom.Class("button", "s-lc-cancel-single")) != nil,
		})
	}
	return bookings, nil
}

func textOf(node *html.Node, match func(*html.Node) bool) string {
	if found := dom.FindFirst(node, match); found != nil {
		return dom.Collapse(dom.Text(found))
	}
	return ""
}

// parseBookingTimes reads LibCal's "11:00am - 12:00pm, Monday, November 16,
// 2026". An end at or before the start falls on the next day.
func parseBookingTimes(value string) (time.Time, time.Time, error) {
	invalid := errors.New("LibCal listed a booking with unreadable times")
	span, day, ok := strings.Cut(value, ", ")
	from, to, found := strings.Cut(span, " - ")
	if !ok || !found {
		return time.Time{}, time.Time{}, invalid
	}
	location, err := pacific()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	const layout = "3:04pm, Monday, January 2, 2006"
	start, err := time.ParseInLocation(layout, strings.TrimSpace(from)+", "+day, location)
	if err != nil {
		return time.Time{}, time.Time{}, invalid
	}
	end, err := time.ParseInLocation(layout, strings.TrimSpace(to)+", "+day, location)
	if err != nil {
		return time.Time{}, time.Time{}, invalid
	}
	if !end.After(start) {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, nil
}

// Cancel cancels one booking listed under the cancellation code.
func (c *Client) Cancel(ctx context.Context, code string, bookingID int) (Cancellation, error) {
	if !cancelCodePattern.MatchString(code) {
		return Cancellation{}, errors.New("invalid LibCal cancellation code")
	}
	if bookingID < 1 {
		return Cancellation{}, errors.New("an explicit booking ID is required")
	}
	referer := baseURL + "/equipment/cancel?id=" + code
	data, err := c.request(ctx, http.MethodPost, "/ajax/equipment/cancel/"+code+"/"+strconv.Itoa(bookingID), url.Values{}, ajaxAccept, referer)
	if err != nil {
		return Cancellation{}, err
	}
	var wire struct {
		Success       bool   `json:"success"`
		Error         string `json:"error"`
		MoreRemaining bool   `json:"moreRemaining"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return Cancellation{}, errors.New("LibCal returned an unreadable cancellation response")
	}
	if !wire.Success {
		if wire.Error == "" {
			wire.Error = "LibCal could not cancel the booking"
		}
		return Cancellation{}, libcalError(wire.Error)
	}
	return Cancellation{BookingID: bookingID, Canceled: true, MoreRemaining: wire.MoreRemaining}, nil
}
