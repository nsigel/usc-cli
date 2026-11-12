package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
)

// bookingMethodID is LibCal's code for a booking made from the public page.
const bookingMethodID = "11"

// Selection identifies the exact space, start, and end to reserve.
type Selection struct {
	SpaceID int       `json:"space_id"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
}

type availableSlot struct {
	Space    Space
	Start    time.Time
	End      time.Time
	Checksum string
}

// Validate rejects invalid inputs and durations over two hours without requests.
func (s Selection) Validate() error {
	if s.SpaceID < 1 || s.Start.IsZero() || s.End.IsZero() {
		return errors.New("an explicit space, start, and end are required")
	}
	if !s.End.After(s.Start) {
		return errors.New("end must be after start")
	}
	if s.End.Sub(s.Start) > 2*time.Hour {
		return &Error{Code: "libcal_duration_limit", Message: "a LibCal reservation cannot exceed 2 hours; choose a shorter interval from the schedule"}
	}
	if s.Start.Second() != 0 || s.Start.Nanosecond() != 0 ||
		(s.End.Second() != 0 || s.End.Nanosecond() != 0) {
		return errors.New("start and end must use whole minutes")
	}
	return nil
}

// ReservationDetails are the caller's explicit answers to LibCal's checkout
// form, keyed by field name. Book refuses to submit unless AcceptTerms is set.
type ReservationDetails struct {
	Fields      map[string]string
	AcceptTerms bool
}

// Reservation is the confirmed LibCal booking returned after form submission.
type Reservation struct {
	Space       Space     `json:"space"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	State       string    `json:"state"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// Book reserves exactly the selected space, start, and end; it never
// substitutes another room or time. A failed or interrupted booking releases
// its temporary hold. ErrBookingUnknown means the outcome must be checked by
// hand before booking again.
func (c *Client) Book(ctx context.Context, selection Selection, details ReservationDetails) (reservation Reservation, resultErr error) {
	if err := selection.Validate(); err != nil {
		return Reservation{}, err
	}
	if !details.AcceptTerms {
		return Reservation{}, errors.New("booking requires acceptance of the LibCal reservation terms")
	}
	tx, err := c.beginCheckout(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.finish(&resultErr)
	slot, err := c.slotStartingAt(ctx, selection)
	if err != nil {
		return Reservation{}, err
	}
	form, err := c.checkoutForm(ctx, slot, tx)
	if err != nil {
		return Reservation{}, err
	}
	values, err := form.submission(details, slot, *tx.pending)
	if err != nil {
		return Reservation{}, err
	}
	if err := c.submit(ctx, form, values, tx); err != nil {
		return Reservation{}, err
	}
	return Reservation{Space: slot.Space, Start: slot.Start, End: slot.End, State: "confirmed", ConfirmedAt: time.Now()}, nil
}

// slotStartingAt finds the selection's start in the live schedule. The end is
// not checked here: LibCal validates it when the hold is extended.
func (c *Client) slotStartingAt(ctx context.Context, selection Selection) (availableSlot, error) {
	space, err := c.Space(ctx, selection.SpaceID)
	if err != nil {
		return availableSlot{}, err
	}
	schedule, err := c.Schedule(ctx, ScheduleOptions{Date: selection.Start, Category: space.CategorySlug})
	if err != nil {
		return availableSlot{}, err
	}
	for _, category := range schedule.Categories {
		for _, room := range category.Spaces {
			if room.Space.ID != selection.SpaceID {
				continue
			}
			for _, interval := range room.Intervals {
				if interval.Start.Equal(selection.Start) && interval.Available && interval.checksum != "" {
					end := selection.End.In(interval.Start.Location())
					return availableSlot{Space: room.Space, Start: interval.Start, End: end, Checksum: interval.checksum}, nil
				}
			}
		}
	}
	return availableSlot{}, &Error{Code: "libcal_selected_slot_unavailable", Message: "the selected space and start are unavailable; refresh the schedule"}
}

// LibCal's own booking form gets the fields its browser submit handler
// appends; the equipment checkout form already carries them.
func (form bookingForm) submission(details ReservationDetails, slot availableSlot, hold pendingBooking) (url.Values, error) {
	values, err := form.answers(details)
	if err != nil || form.Action == equipmentCheckoutPath {
		return values, err
	}
	category, err := categoryOf(slot.Space)
	if err != nil {
		return nil, err
	}
	bookings, err := json.Marshal([]map[string]any{{
		"id": hold.ID, "eid": hold.EID, "seat_id": hold.SeatID, "gid": hold.GroupID, "lid": hold.LocationID,
		"start": hold.Start, "end": hold.End, "checksum": hold.Checksum,
	}})
	if err != nil {
		return nil, fmt.Errorf("encode LibCal booking details: %w", err)
	}
	values.Set("bookings", string(bookings))
	values.Set("returnUrl", category.returnPath())
	values.Set("pickupHolds", "")
	values.Set("method", bookingMethodID)
	return values, nil
}

func (c *Client) submit(ctx context.Context, form bookingForm, values url.Values, tx *checkout) error {
	tx.state.OutcomeUnknown = true
	if err := tx.save(); err != nil {
		tx.state.OutcomeUnknown = false
		return err
	}
	response, err := c.sendForm(ctx, form, values)
	var httpRejection *ResponseError
	if errors.As(err, &httpRejection) && httpRejection.Status >= 400 && httpRejection.Status < 500 {
		tx.state.OutcomeUnknown = false
		return err
	}
	if err == nil {
		err = checkBookingResponse(response)
		var rejection *Error
		if errors.As(err, &rejection) {
			tx.state.OutcomeUnknown = false
			return err
		}
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBookingUnknown, err)
	}
	tx.state = checkoutState{}
	tx.pending = nil
	if err := tx.save(); err != nil {
		return fmt.Errorf("booking confirmed, but could not clear recovery state; do not retry: %w", err)
	}
	return nil
}

func (c *Client) sendForm(ctx context.Context, form bookingForm, values url.Values) ([]byte, error) {
	base, _ := url.Parse(baseURL)
	target, err := base.Parse(form.Action)
	if err != nil || target.Scheme != "https" || target.Host != base.Host {
		return nil, errors.New("LibCal reservation form points to an unexpected host")
	}
	const accept = "application/json, text/html, */*;q=0.8"
	switch form.Method {
	case http.MethodPost:
		return c.request(ctx, http.MethodPost, target.RequestURI(), values, accept, form.Referer)
	case http.MethodGet:
		query := target.Query()
		for name, entries := range values {
			query[name] = append(query[name], entries...)
		}
		target.RawQuery = query.Encode()
		return c.request(ctx, http.MethodGet, target.RequestURI(), nil, accept, form.Referer)
	}
	return nil, errors.New("LibCal reservation form uses an unsupported method")
}

func checkBookingResponse(data []byte) error {
	var wire map[string]any
	if json.Unmarshal(data, &wire) == nil {
		if message, ok := wire["error"].(string); ok && message != "" {
			return libcalError(message)
		}
		// LibCal's bookingSuccessCallback treats nonempty HTML without an
		// error as success; confirmation wording varies by category.
		if body, ok := wire["html"].(string); ok && strings.TrimSpace(body) != "" {
			return nil
		}
	}
	var checkoutHTML string
	if json.Unmarshal(data, &checkoutHTML) == nil {
		data = []byte(checkoutHTML)
	}
	text := strings.ToLower(strings.TrimSpace(textFromHTML(data)))
	if strings.Contains(text, "booking confirmed") || strings.Contains(text, "reservation confirmed") || strings.Contains(text, "reservation was confirmed") || strings.Contains(text, "successfully booked") {
		return nil
	}
	return errors.New("LibCal did not return a reservation confirmation")
}
