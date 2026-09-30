package libcal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nsigel/usc-cli/internal/auth"
)

// Authenticate performs LibCal's booking-specific USC SSO handoff without
// submitting a reservation. LibCal only creates a valid auth redirect after a
// room is staged, so this briefly holds an available 2nd-floor room and always
// removes that hold before returning.
func Authenticate(ctx context.Context, sessionFile string, credentials auth.Credentials, fresh bool) (resultErr error) {
	open := func() (*auth.Session, error) {
		return auth.OpenSessionWithDomainPolicy(sessionFile, []string{"libcal.usc.edu", "libauth.com"}, []string{"lc_ebcart"})
	}
	if fresh {
		open = func() (*auth.Session, error) { return auth.OpenFreshSession(sessionFile) }
	}
	session, err := open()
	if err != nil {
		return fmt.Errorf("open USC session: %w", err)
	}
	client := New(session)
	slot, err := authProbeSlot(ctx, client)
	if err != nil {
		return err
	}
	booking, err := client.addPendingBooking(ctx, slot)
	if err != nil {
		return fmt.Errorf("stage temporary LibCal room hold: %w", err)
	}
	defer func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.removePendingBooking(cleanupContext, slot, booking); err != nil {
			cleanupErr := fmt.Errorf("release temporary LibCal room hold: %w", err)
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()

	updated, err := client.setPendingDuration(ctx, slot, booking)
	if err != nil {
		return fmt.Errorf("prepare temporary LibCal room hold: %w", err)
	}
	booking = updated
	form, err := client.bookingForm(ctx, slot, booking)
	if err != nil {
		if !isBadRequest(err) {
			return err
		}
		return session.Authenticate(ctx, authURLForSpace(slot.Space), credentials, refererForSpace(slot.Space))
	}
	if form.Redirect != "" {
		return session.Authenticate(ctx, form.Redirect, credentials, refererForSpace(slot.Space))
	}
	if form.HTML == "" {
		return errors.New("LibCal did not complete its USC sign-in handoff")
	}
	return nil
}

func authProbeSlot(ctx context.Context, client *Client) (AvailableSlot, error) {
	location, err := pacific()
	if err != nil {
		return AvailableSlot{}, err
	}
	now := time.Now().In(location).Add(10 * time.Minute)
	today := time.Now().In(location)
	for offset := 0; offset <= 7; offset++ {
		date := today.AddDate(0, 0, offset)
		slots, err := client.Availability(ctx, AvailabilityOptions{
			Date:     date,
			Duration: time.Hour,
			Category: CategorySecond,
		})
		if err != nil {
			return AvailableSlot{}, err
		}
		for _, slot := range slots {
			if slot.Start.After(now) {
				return slot, nil
			}
		}
	}
	return AvailableSlot{}, errors.New("no Leavey 2nd-floor room is available in the next week to start LibCal sign-in")
}
