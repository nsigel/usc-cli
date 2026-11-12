package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	http "github.com/saucesteals/fhttp"
)

// A hold is LibCal's temporary claim on a slot while its checkout form is
// open. The browser keeps the hold in page state and resends it on every
// request; pendingBooking is that state.
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

func (b pendingBooking) fields() map[string]string {
	return map[string]string{
		"id": strconv.Itoa(b.ID), "eid": strconv.Itoa(b.EID), "seat_id": strconv.Itoa(b.SeatID),
		"gid": strconv.Itoa(b.GroupID), "lid": strconv.Itoa(b.LocationID),
		"start": b.Start, "end": b.End, "checksum": b.Checksum,
	}
}

func (b pendingBooking) echo(values url.Values) {
	for name, value := range b.fields() {
		values.Set("bookings[0]["+name+"]", value)
	}
}

func (c *Client) changeHolds(ctx context.Context, slot availableSlot, values url.Values, action string) ([]pendingBooking, error) {
	values.Set("lid", strconv.Itoa(leaveyLocationID))
	values.Set("gid", strconv.Itoa(slot.Space.CategoryID))
	values.Set("start", slot.Start.Format(time.DateOnly))
	values.Set("end", slot.Start.AddDate(0, 0, 1).Format(time.DateOnly))
	data, err := c.request(ctx, http.MethodPost, "/spaces/availability/booking/add", values, ajaxAccept, refererFor(slot.Space))
	if err != nil {
		return nil, err
	}
	var response struct {
		Bookings []pendingBooking `json:"bookings"`
		Error    string           `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode LibCal %s: %w", action, err)
	}
	if response.Error != "" {
		return nil, libcalError(response.Error)
	}
	return response.Bookings, nil
}

func (c *Client) addHold(ctx context.Context, slot availableSlot) (pendingBooking, error) {
	holds, err := c.changeHolds(ctx, slot, url.Values{
		"add[eid]":      {strconv.Itoa(slot.Space.ID)},
		"add[seat_id]":  {"0"},
		"add[gid]":      {strconv.Itoa(slot.Space.CategoryID)},
		"add[lid]":      {strconv.Itoa(leaveyLocationID)},
		"add[start]":    {slot.Start.Format("2006-01-02 15:04")},
		"add[checksum]": {slot.Checksum},
	}, "booking hold")
	if err != nil {
		return pendingBooking{}, err
	}
	for _, hold := range holds {
		if hold.EID == slot.Space.ID {
			return hold, nil
		}
	}
	return pendingBooking{}, errors.New("LibCal did not return the selected room hold")
}

func (c *Client) extendHold(ctx context.Context, slot availableSlot, hold pendingBooking) (pendingBooking, error) {
	endsAtSlotEnd := func(value string) bool {
		end, err := parseLibCalTime(value, slot.Start.Location())
		return err == nil && end.Equal(slot.End)
	}
	selected := -1
	for index, option := range hold.Options {
		if endsAtSlotEnd(option) {
			selected = index
			break
		}
	}
	if selected < 0 || selected >= len(hold.OptionChecksums) {
		return pendingBooking{}, errors.New("LibCal no longer offers the requested reservation duration")
	}
	if endsAtSlotEnd(hold.End) {
		return hold, nil
	}
	values := url.Values{
		"update[id]":       {strconv.Itoa(hold.ID)},
		"update[checksum]": {hold.OptionChecksums[selected]},
		"update[end]":      {hold.Options[selected]},
	}
	hold.echo(values)
	holds, err := c.changeHolds(ctx, slot, values, "reservation duration update")
	if err != nil {
		return pendingBooking{}, err
	}
	for _, updated := range holds {
		if updated.ID == hold.ID && updated.EID == slot.Space.ID {
			if !endsAtSlotEnd(updated.End) {
				return pendingBooking{}, errors.New("LibCal did not apply the requested reservation duration")
			}
			return updated, nil
		}
	}
	return pendingBooking{}, errors.New("LibCal did not return the updated room hold")
}

func (c *Client) removeHold(ctx context.Context, slot availableSlot, hold pendingBooking) error {
	values := url.Values{"removeId": {strconv.Itoa(hold.ID)}}
	hold.echo(values)
	holds, err := c.changeHolds(ctx, slot, values, "room hold release")
	if err != nil {
		return err
	}
	for _, remaining := range holds {
		if remaining.ID == hold.ID {
			return errors.New("LibCal still reports the temporary room hold as staged")
		}
	}
	return nil
}
