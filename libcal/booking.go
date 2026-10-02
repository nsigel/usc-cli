package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
	"golang.org/x/net/html"
)

var ErrNoAvailability = &Error{Code: "libcal_no_availability", Message: "no matching Leavey spaces are available"}

const bookingMethodID = "11"

// ReservationDetails supplies the identity and any custom fields LibCal asks
// for. Terms are only accepted when AcceptTerms is true.
type ReservationDetails struct {
	Name        string
	Email       string
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

// BookEarliest finds and books the earliest matching Leavey slot. Group rooms
// are searched by default; set IncludePods to allow one-person pods.
func (c *Client) BookEarliest(ctx context.Context, options AvailabilityOptions, details ReservationDetails) (reservation Reservation, resultErr error) {
	if !details.AcceptTerms {
		return Reservation{}, errors.New("booking requires acceptance of the LibCal reservation terms")
	}
	tx, err := c.beginCheckout(ctx, false)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.finish(&resultErr)
	available, err := c.Availability(ctx, options)
	if err != nil {
		return Reservation{}, err
	}
	if len(available) == 0 {
		return Reservation{}, ErrNoAvailability
	}
	return c.book(ctx, available[0], details, tx)
}

// Book reserves one slot returned by Availability. It creates a temporary
// LibCal hold, completes the booking form, and removes the hold if submission
// fails.
func (c *Client) Book(ctx context.Context, slot AvailableSlot, details ReservationDetails) (reservation Reservation, resultErr error) {
	tx, err := c.beginCheckout(ctx, false)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.finish(&resultErr)
	return c.book(ctx, slot, details, tx)
}

func (c *Client) book(ctx context.Context, slot AvailableSlot, details ReservationDetails, tx *checkout) (Reservation, error) {
	if !details.AcceptTerms {
		return Reservation{}, errors.New("booking requires acceptance of the LibCal reservation terms")
	}
	if slot.Space.ID < 1 || slot.Space.CategoryID < 1 || slot.Checksum == "" {
		return Reservation{}, errors.New("booking requires an AvailableSlot returned by Availability")
	}
	if slot.End.Sub(slot.Start) < 30*time.Minute || slot.End.Sub(slot.Start) > 2*time.Hour || slot.End.Sub(slot.Start)%(30*time.Minute) != 0 {
		return Reservation{}, errors.New("reservation duration must be 30 minutes to 2 hours in 30-minute increments")
	}
	form, err := c.prepareCheckout(ctx, slot, tx)
	if err != nil {
		return Reservation{}, err
	}
	values, err := fillBookingForm(form, details, slot, *tx.pending)
	if err != nil {
		return Reservation{}, err
	}
	// Persist before the POST: a killed process must never silently retry a
	// reservation whose confirmation was lost.
	tx.state.Submitting = true
	if err := tx.save(); err != nil {
		tx.state.Submitting = false
		return Reservation{}, err
	}
	response, err := c.submitBookingForm(ctx, form, values)
	if err != nil {
		var rejected *ResponseError
		if errors.As(err, &rejected) && rejected.Status >= 400 && rejected.Status < 500 {
			tx.state.Submitting = false
			return Reservation{}, err
		}
		return Reservation{}, fmt.Errorf("%w: %v", ErrBookingUnknown, err)
	}
	if err := checkBookingResponse(response); err != nil {
		var rejected *Error
		if errors.As(err, &rejected) {
			tx.state.Submitting = false
			return Reservation{}, err
		}
		return Reservation{}, fmt.Errorf("%w: %v", ErrBookingUnknown, err)
	}
	tx.state = checkoutState{}
	tx.pending = nil
	if err := tx.save(); err != nil {
		return Reservation{}, fmt.Errorf("booking confirmed, but could not clear recovery state; do not retry: %w", err)
	}
	reservation := Reservation{
		Space: slot.Space, Start: slot.Start, End: slot.End,
		State: "confirmed", ConfirmedAt: time.Now(),
	}
	if err := c.recordReservation(reservation); err != nil {
		return Reservation{}, fmt.Errorf(
			"booking confirmed for %s from %s to %s, but could not record it locally; do not retry: %w",
			reservation.Space.Name, reservation.Start.Format(time.RFC3339), reservation.End.Format(time.RFC3339), err,
		)
	}
	return reservation, nil
}

type pendingBooking struct {
	ID              int      `json:"id"`
	EID             int      `json:"eid"`
	SeatID          int      `json:"seat_id"`
	GroupID         int      `json:"gid"`
	LocationID      int      `json:"lid"`
	Start           string   `json:"start"`
	End             string   `json:"end"`
	Checksum        string   `json:"checksum"`
	Options         []string `json:"options"`
	OptionChecksums []string `json:"optionChecksums"`
	OptionSelected  int      `json:"optionSelected"`
}

type addBookingResponse struct {
	Bookings []pendingBooking `json:"bookings"`
	Error    string           `json:"error"`
}

func (c *Client) addPendingBooking(ctx context.Context, slot AvailableSlot) (pendingBooking, error) {
	date := slot.Start.Format("2006-01-02")
	endDate := slot.Start.AddDate(0, 0, 1).Format("2006-01-02")
	values := url.Values{
		"add[eid]":      {strconv.Itoa(slot.Space.ID)},
		"add[seat_id]":  {"0"},
		"add[gid]":      {strconv.Itoa(slot.Space.CategoryID)},
		"add[lid]":      {strconv.Itoa(leaveyLocationID)},
		"add[start]":    {slot.Start.Format("2006-01-02 15:04")},
		"add[checksum]": {slot.Checksum},
		"lid":           {strconv.Itoa(leaveyLocationID)},
		"gid":           {strconv.Itoa(slot.Space.CategoryID)},
		"start":         {date},
		"end":           {endDate},
	}
	data, err := c.requestFrom(ctx, http.MethodPost, "/spaces/availability/booking/add", values, "application/json, text/javascript, */*;q=0.01", refererForSpace(slot.Space))
	if err != nil {
		return pendingBooking{}, err
	}
	var response addBookingResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return pendingBooking{}, fmt.Errorf("decode LibCal booking hold: %w", err)
	}
	if response.Error != "" {
		return pendingBooking{}, libcalError(response.Error)
	}
	for _, booking := range response.Bookings {
		if booking.EID != slot.Space.ID {
			continue
		}
		return booking, nil
	}
	return pendingBooking{}, errors.New("LibCal did not return the selected room hold")
}

func (c *Client) setPendingDuration(ctx context.Context, slot AvailableSlot, booking pendingBooking) (pendingBooking, error) {
	selected := -1
	for i, option := range booking.Options {
		end, err := parseLibCalTime(option, slot.Start.Location())
		if err == nil && end.Equal(slot.End) {
			selected = i
			break
		}
	}
	if selected < 0 || selected >= len(booking.OptionChecksums) {
		return pendingBooking{}, errors.New("LibCal no longer offers the requested reservation duration")
	}
	if currentEnd, err := parseLibCalTime(booking.End, slot.Start.Location()); err == nil && currentEnd.Equal(slot.End) {
		return booking, nil
	}

	date := slot.Start.Format("2006-01-02")
	values := url.Values{
		"update[id]":       {strconv.Itoa(booking.ID)},
		"update[checksum]": {booking.OptionChecksums[selected]},
		"update[end]":      {booking.Options[selected]},
		"lid":              {strconv.Itoa(leaveyLocationID)},
		"gid":              {strconv.Itoa(slot.Space.CategoryID)},
		"start":            {date},
		"end":              {slot.Start.AddDate(0, 0, 1).Format("2006-01-02")},
	}
	addPendingBookingFields(values, "bookings[0]", booking)
	data, err := c.requestFrom(ctx, http.MethodPost, "/spaces/availability/booking/add", values, "application/json, text/javascript, */*;q=0.01", refererForSpace(slot.Space))
	if err != nil {
		return pendingBooking{}, err
	}
	var response addBookingResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return pendingBooking{}, fmt.Errorf("decode LibCal reservation duration update: %w", err)
	}
	if response.Error != "" {
		return pendingBooking{}, libcalError(response.Error)
	}
	for _, updated := range response.Bookings {
		if updated.ID == booking.ID && updated.EID == slot.Space.ID {
			updatedEnd, parseErr := parseLibCalTime(updated.End, slot.Start.Location())
			if parseErr != nil || !updatedEnd.Equal(slot.End) {
				return pendingBooking{}, errors.New("LibCal did not apply the requested reservation duration")
			}
			return updated, nil
		}
	}
	return pendingBooking{}, errors.New("LibCal did not return the updated room hold")
}

func (c *Client) removePendingBooking(ctx context.Context, slot AvailableSlot, booking pendingBooking) error {
	values := url.Values{
		"removeId": {strconv.Itoa(booking.ID)},
		"lid":      {strconv.Itoa(leaveyLocationID)},
		"gid":      {strconv.Itoa(slot.Space.CategoryID)},
		"start":    {slot.Start.Format("2006-01-02")},
		"end":      {slot.Start.AddDate(0, 0, 1).Format("2006-01-02")},
	}
	addPendingBookingFields(values, "bookings[0]", booking)
	data, err := c.requestFrom(ctx, http.MethodPost, "/spaces/availability/booking/add", values, "application/json, text/javascript, */*;q=0.01", refererForSpace(slot.Space))
	if err != nil {
		return err
	}
	var response addBookingResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("decode LibCal room hold release: %w", err)
	}
	if response.Error != "" {
		return libcalError(response.Error)
	}
	for _, remaining := range response.Bookings {
		if remaining.ID == booking.ID {
			return errors.New("LibCal still reports the temporary room hold as staged")
		}
	}
	return nil
}

func addPendingBookingFields(values url.Values, prefix string, booking pendingBooking) {
	values.Set(prefix+"[id]", strconv.Itoa(booking.ID))
	values.Set(prefix+"[eid]", strconv.Itoa(booking.EID))
	values.Set(prefix+"[seat_id]", strconv.Itoa(booking.SeatID))
	values.Set(prefix+"[gid]", strconv.Itoa(booking.GroupID))
	values.Set(prefix+"[lid]", strconv.Itoa(booking.LocationID))
	values.Set(prefix+"[start]", booking.Start)
	values.Set(prefix+"[end]", booking.End)
	values.Set(prefix+"[checksum]", booking.Checksum)
}

type bookingPageResponse struct {
	Redirect string `json:"redirect"`
	HTML     string `json:"html"`
	Error    string `json:"error"`
}

func (c *Client) bookingForm(ctx context.Context, slot AvailableSlot, booking pendingBooking) (bookingPageResponse, error) {
	category := categoryBySlug(slot.Space.CategorySlug)
	if category.ID == 0 {
		return bookingPageResponse{}, errors.New("unsupported Leavey reservation category")
	}
	values := url.Values{
		"patron":     {""},
		"patronHash": {""},
		"returnUrl":  {categoryReturnURL(category)},
		"method":     {bookingMethodID},
	}
	addPendingBookingFields(values, "bookings[0]", booking)
	data, err := c.requestFrom(ctx, http.MethodPost, "/ajax/space/times", values, "application/json, text/javascript, */*;q=0.01", refererForSpace(slot.Space))
	if err != nil {
		return bookingPageResponse{}, err
	}
	var response bookingPageResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return bookingPageResponse{}, fmt.Errorf("decode LibCal booking form: %w", err)
	}
	if response.Redirect != "" {
		response.Redirect, err = normalizeAuthRedirect(response.Redirect)
		if err != nil {
			return bookingPageResponse{}, err
		}
		return response, nil
	}
	if response.Error != "" {
		return bookingPageResponse{}, libcalError(response.Error)
	}
	if response.HTML == "" {
		return bookingPageResponse{}, errors.New("LibCal returned no reservation form")
	}
	return response, nil
}

func normalizeAuthRedirect(value string) (string, error) {
	target, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse LibCal authentication redirect: %w", err)
	}
	if !target.IsAbs() {
		base, _ := url.Parse(baseURL)
		target = base.ResolveReference(target)
	}
	if target.Scheme != "https" || !strings.EqualFold(target.Hostname(), "libcal.usc.edu") || target.Path != "/spaces/auth" {
		return "", errors.New("LibCal returned an unsupported authentication redirect")
	}
	return target.String(), nil
}

type bookingForm struct {
	Action  string
	Method  string
	Referer string
	Fields  []bookingFormField
}

type bookingFormField struct {
	Name     string
	Type     string
	Label    string
	Value    string
	Required bool
	Checked  bool
	Options  []bookingOption
}

type bookingOption struct {
	Value    string
	Disabled bool
	Selected bool
}

func parseBookingForm(raw string) (bookingForm, error) {
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return bookingForm{}, fmt.Errorf("parse LibCal reservation form: %w", err)
	}
	var formNode *html.Node
	walk(document, func(node *html.Node) bool {
		if node.Type == html.ElementNode && node.Data == "form" {
			if attr(node, "id") == "s-lc-eq-bform" {
				formNode = node
				return true
			}
			if formNode == nil {
				formNode = node
			}
		}
		return false
	})
	if formNode == nil {
		return bookingForm{}, errors.New("LibCal returned a page without a reservation form")
	}
	result := bookingForm{Action: attr(formNode, "action"), Method: strings.ToUpper(attr(formNode, "method"))}
	if result.Method == "" {
		result.Method = http.MethodPost
	}
	labels := make(map[string]string)
	walk(formNode, func(node *html.Node) bool {
		if node.Type == html.ElementNode && node.Data == "label" {
			if id := attr(node, "for"); id != "" {
				labels[id] = strings.TrimSpace(textContent(node))
			}
		}
		return false
	})
	walk(formNode, func(node *html.Node) bool {
		if node.Type != html.ElementNode {
			return false
		}
		switch node.Data {
		case "input":
			typeName := strings.ToLower(attr(node, "type"))
			if typeName == "" {
				typeName = "text"
			}
			if typeName == "submit" || typeName == "button" || typeName == "reset" || attr(node, "name") == "" {
				return false
			}
			class := " " + attr(node, "class") + " "
			result.Fields = append(result.Fields, bookingFormField{
				Name: attr(node, "name"), Type: typeName, Label: labels[attr(node, "id")], Value: attr(node, "value"),
				Required: hasAttr(node, "required") || strings.Contains(class, " notempty ") || strings.Contains(class, " requirechecked "),
				Checked:  hasAttr(node, "checked"),
			})
		case "textarea":
			name := attr(node, "name")
			if name != "" {
				class := " " + attr(node, "class") + " "
				result.Fields = append(result.Fields, bookingFormField{Name: name, Type: "textarea", Label: labels[attr(node, "id")], Value: textContent(node), Required: hasAttr(node, "required") || strings.Contains(class, " notempty ")})
			}
		case "select":
			name := attr(node, "name")
			if name == "" {
				return false
			}
			field := bookingFormField{Name: name, Type: "select", Label: labels[attr(node, "id")], Required: hasAttr(node, "required")}
			walk(node, func(option *html.Node) bool {
				if option.Type == html.ElementNode && option.Data == "option" {
					field.Options = append(field.Options, bookingOption{Value: attr(option, "value"), Disabled: hasAttr(option, "disabled"), Selected: hasAttr(option, "selected")})
				}
				return false
			})
			result.Fields = append(result.Fields, field)
		}
		return false
	})
	if result.Action == "" {
		return bookingForm{}, errors.New("LibCal reservation form has no submission URL")
	}
	return result, nil
}

func fillBookingForm(form bookingForm, details ReservationDetails, slot AvailableSlot, booking pendingBooking) (url.Values, error) {
	values := make(url.Values)
	firstName, lastName := splitName(details.Name)
	for _, field := range form.Fields {
		if field.Type == "hidden" {
			values.Set(field.Name, field.Value)
			continue
		}
		value, provided := details.Fields[field.Name]
		if !provided {
			value = field.Value
		}
		label := strings.ToLower(field.Label)
		name := strings.ToLower(field.Name)
		switch field.Type {
		case "email":
			if details.Email != "" {
				value = details.Email
			}
		case "checkbox":
			if isTermsField(field) {
				if !details.AcceptTerms {
					return nil, errors.New("LibCal requires terms acceptance; rerun with --accept-terms")
				}
				if value == "" {
					value = "1"
				}
			} else if !field.Checked && !provided {
				continue
			}
		case "radio":
			if !field.Checked && !provided {
				continue
			}
		case "select":
			if !provided {
				for _, option := range field.Options {
					if option.Selected && !option.Disabled {
						value = option.Value
						break
					}
				}
				if value == "" {
					for _, option := range field.Options {
						if !option.Disabled && option.Value != "" {
							value = option.Value
							break
						}
					}
				}
			}
		default:
			if value == "" {
				if strings.Contains(name+" "+label, "email") && details.Email != "" {
					value = details.Email
				} else if strings.Contains(name+" "+label, "first name") || strings.Contains(name, "firstname") {
					value = firstName
				} else if strings.Contains(name+" "+label, "last name") || strings.Contains(name, "lastname") {
					value = lastName
				} else if strings.Contains(name+" "+label, "name") && details.Name != "" {
					value = details.Name
				}
			}
		}
		if value == "" && field.Required {
			return nil, fmt.Errorf("LibCal requires the %s field; pass it with --field %s=VALUE", field.Name, field.Name)
		}
		if value != "" {
			values.Add(field.Name, value)
		}
	}
	if form.Action == "/ajax/equipment/checkout" {
		return values, nil
	}
	serializedBookings, err := json.Marshal([]map[string]any{{
		"id": booking.ID, "eid": booking.EID, "seat_id": booking.SeatID,
		"gid": booking.GroupID, "lid": booking.LocationID,
		"start": booking.Start, "end": booking.End, "checksum": booking.Checksum,
	}})
	if err != nil {
		return nil, fmt.Errorf("encode LibCal booking details: %w", err)
	}
	category := categoryBySlug(slot.Space.CategorySlug)
	if category.ID == 0 {
		return nil, errors.New("unsupported Leavey reservation category")
	}
	// LibCal appends these fields in its browser submit handler immediately
	// before posting the reservation form.
	values.Set("bookings", string(serializedBookings))
	values.Set("returnUrl", categoryReturnURL(category))
	values.Set("pickupHolds", "")
	values.Set("method", bookingMethodID)
	return values, nil
}

func isTermsField(field bookingFormField) bool {
	name := strings.ToLower(field.Name + " " + field.Label)
	return strings.Contains(name, "terms") || strings.Contains(name, "condition") || strings.Contains(name, "agree")
}

func splitName(value string) (string, string) {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func (c *Client) submitBookingForm(ctx context.Context, form bookingForm, values url.Values) ([]byte, error) {
	base, _ := url.Parse(baseURL)
	target, err := base.Parse(form.Action)
	if err != nil || target.Scheme != "https" || target.Host != base.Host {
		return nil, errors.New("LibCal reservation form points to an unexpected host")
	}
	method := form.Method
	if method != http.MethodPost && method != http.MethodGet {
		return nil, errors.New("LibCal reservation form uses an unsupported method")
	}
	path := target.EscapedPath()
	if target.RawQuery != "" {
		path += "?" + target.RawQuery
	}
	if method == http.MethodGet {
		query := target.Query()
		for name, entries := range values {
			for _, value := range entries {
				query.Add(name, value)
			}
		}
		target.RawQuery = query.Encode()
		path = target.EscapedPath() + "?" + target.RawQuery
	}
	return c.requestFrom(ctx, method, path, valuesIf(method == http.MethodPost, values), "application/json, text/html, */*;q=0.8", form.Referer)
}

func refererForSpace(space Space) string {
	category := categoryBySlug(space.CategorySlug)
	if category.ID == 0 {
		return baseURL + "/reserve/lvl2"
	}
	return baseURL + categoryReturnURL(category)
}

func valuesIf(condition bool, values url.Values) url.Values {
	if !condition {
		return nil
	}
	return values
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

func safeMessage(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func textFromHTML(data []byte) string {
	document, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return ""
	}
	return textContent(document)
}

func textContent(node *html.Node) string {
	var text strings.Builder
	walk(node, func(child *html.Node) bool {
		if child.Type == html.TextNode {
			text.WriteString(child.Data)
			text.WriteByte(' ')
		}
		return false
	})
	return strings.TrimSpace(text.String())
}

func walk(node *html.Node, visit func(*html.Node) bool) bool {
	if visit(node) {
		return true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if walk(child, visit) {
			return true
		}
	}
	return false
}

func attr(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func hasAttr(node *html.Node, name string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return true
		}
	}
	return false
}
