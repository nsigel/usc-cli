package libcal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nsigel/usc-cli/internal/auth"
	http "github.com/saucesteals/fhttp"
)

// Authenticate performs LibCal's booking-specific USC SSO handoff without
// submitting a reservation. LibCal only creates a valid auth redirect after a
// room is staged, so this briefly holds an available 2nd-floor room and always
// removes that hold before returning.
func Authenticate(ctx context.Context, sessionFile string, credentials auth.Credentials, fresh bool) (resultErr error) {
	session, err := auth.OpenSession(sessionFile, auth.SessionOptions{
		Fresh:           fresh,
		AllowMissing:    true,
		ResetDomains:    []string{"libcal.usc.edu", "libauth.com"},
		PreserveCookies: []string{"lc_ebcart"},
	})
	if err != nil {
		return fmt.Errorf("open USC session: %w", err)
	}
	client := New(session)
	client.checkoutPath = filepath.Join(filepath.Dir(sessionFile), "libcal-checkout.json")
	client.authenticate = func(ctx context.Context, target, referer string) (*http.Response, error) {
		return session.Authenticate(ctx, target, credentials, referer)
	}
	tx, err := client.beginCheckout(ctx, false)
	if err != nil {
		return err
	}
	defer tx.finish(&resultErr)
	slot, err := authProbeSlot(ctx, client)
	if err != nil {
		return err
	}
	_, err = client.prepareCheckout(ctx, slot, tx)
	return err
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
