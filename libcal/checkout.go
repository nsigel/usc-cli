package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
)

// ErrBookingUnknown means submission may have succeeded. Never retry blindly.
var ErrBookingUnknown = &Error{Code: "libcal_booking_unknown", Message: "LibCal booking outcome is unknown; check your confirmation email before retrying; run usc libcal release only after checking"}

type checkoutState struct {
	Session    string `json:"session,omitempty"`
	Submitting bool   `json:"submitting,omitempty"`
}

// Keep the lock file separate from the atomically replaced recovery record.
// Never unlink the lock file: another process may be using the same inode.
type checkout struct {
	client  *Client
	file    *os.File
	state   checkoutState
	slot    availableSlot
	pending *pendingBooking
}

func (c *Client) beginCheckout(ctx context.Context, release bool) (*checkout, error) {
	tx := &checkout{client: c}
	if c.checkoutPath == "" {
		return tx, nil
	}
	if err := os.MkdirAll(filepath.Dir(c.checkoutPath), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(c.checkoutPath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockCheckout(f); err != nil {
		f.Close()
		return nil, &Error{Code: "libcal_checkout_busy", Message: "another LibCal checkout is running; wait for it to finish"}
	}
	tx.file = f
	fail := func(err error) (*checkout, error) { f.Close(); return nil, err }
	data, err := os.ReadFile(c.checkoutPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &tx.state); err != nil {
			return fail(errors.New("cannot read unfinished LibCal checkout state; do not start another booking"))
		}
	}
	if tx.state.Submitting && !release {
		return fail(ErrBookingUnknown)
	}
	if tx.state.Session != "" {
		if err := c.endCheckout(ctx, tx.state.Session); err != nil {
			return fail(&Error{Code: "libcal_cleanup_failed", Message: "could not release unfinished LibCal checkout; retry usc libcal release: " + err.Error(), Cause: err})
		}
	}
	tx.state = checkoutState{}
	if err := tx.save(); err != nil {
		return fail(err)
	}
	return tx, nil
}

func (tx *checkout) save() error {
	if tx.file == nil {
		return nil
	}
	data, err := json.Marshal(tx.state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(tx.client.checkoutPath), ".libcal-checkout-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), tx.client.checkoutPath)
}

func (tx *checkout) finish(resultErr *error) {
	if tx.file != nil {
		defer tx.file.Close()
	}
	// A request whose response was lost may still be executing on the server.
	// Keep its marker and let the user reconcile the confirmation first.
	if tx.state.Submitting {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cleanupErr error
	if tx.state.Session != "" {
		cleanupErr = tx.client.endCheckout(ctx, tx.state.Session)
	} else if tx.pending != nil {
		cleanupErr = tx.client.removePendingBooking(ctx, tx.slot, *tx.pending)
	}
	if cleanupErr != nil {
		*resultErr = errors.Join(*resultErr, &Error{Code: "libcal_cleanup_failed", Message: "could not release LibCal checkout; retry usc libcal release: " + cleanupErr.Error(), Cause: cleanupErr})
		return
	}
	tx.state = checkoutState{}
	*resultErr = errors.Join(*resultErr, tx.save())
}

// Release ends only an unfinished CLI checkout; it never cancels a confirmed
// reservation. It also acknowledges a previously unknown submission outcome.
func (c *Client) Release(ctx context.Context) (resultErr error) {
	tx, err := c.beginCheckout(ctx, true)
	if err != nil {
		return err
	}
	defer tx.finish(&resultErr)
	return nil
}

func (c *Client) endCheckout(ctx context.Context, session string) error {
	data, err := c.request(ctx, http.MethodPost, "/ajax/space/session/end", url.Values{"session": {session}}, "application/json")
	if err != nil {
		return err
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return errors.New("LibCal returned an invalid checkout release response")
	}
	if result.Error != "" {
		return libcalError(result.Error)
	}
	return nil
}

// These fields are appended by LibCal's checkout submit handler, not present
// as input elements in the HTML. Parse only the known literals, never execute JS.
var checkoutInputPattern = regexp.MustCompile(`appendHiddenInput\('(?P<name>returnUrl|logoutUrl|session)',\s*("(?:\\.|[^"\\])*"|[0-9]+),`)

func (c *Client) checkoutForm(ctx context.Context, slot availableSlot, tx *checkout) (bookingForm, error) {
	booking, err := c.addPendingBooking(ctx, slot)
	if err != nil {
		return bookingForm{}, err
	}
	tx.slot, tx.pending = slot, &booking
	updated, err := c.setPendingDuration(ctx, slot, booking)
	if err != nil {
		return bookingForm{}, err
	}
	tx.pending = &updated
	booking = updated
	page, err := c.bookingForm(ctx, slot, booking)
	if err != nil {
		return bookingForm{}, err // HTTP 400 is not an authentication challenge.
	}
	referer := refererForSpace(slot.Space)
	if page.Redirect != "" {
		if c.authenticate == nil {
			return bookingForm{}, ErrAuthenticationRequired
		}
		response, authErr := c.authenticate(ctx, page.Redirect, referer)
		if response != nil {
			defer response.Body.Close()
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 10<<20))
			if readErr != nil {
				return bookingForm{}, errors.Join(authErr, readErr)
			}
			page.HTML = string(data)
			referer = response.Request.URL.String()
		}
		// Capture the checkout ID even when saving authenticated cookies failed,
		// so the server-side hold can still be released.
		if response == nil {
			return bookingForm{}, authErr
		}
		if err := tx.capture(page.HTML); err != nil {
			return bookingForm{}, errors.Join(authErr, err)
		}
		if authErr != nil {
			return bookingForm{}, authErr
		}
	} else if err := tx.capture(page.HTML); err != nil {
		return bookingForm{}, err
	}
	form, err := parseBookingForm(page.HTML)
	if err != nil {
		return bookingForm{}, err
	}
	form.Referer = referer
	if form.Action == "/ajax/equipment/checkout" {
		if tx.state.Session == "" {
			return bookingForm{}, errors.New("LibCal checkout is missing its session identifier")
		}
		fields := make(map[string]bool)
		for _, match := range checkoutInputPattern.FindAllStringSubmatch(page.HTML, -1) {
			value := match[2]
			if strings.HasPrefix(value, `"`) {
				value, err = strconv.Unquote(value)
				if err != nil {
					return bookingForm{}, errors.New("LibCal checkout has an invalid form value")
				}
			}
			form.Fields = append(form.Fields, bookingFormField{Name: match[1], Type: "hidden", Value: value})
			fields[match[1]] = value != ""
		}
		for _, name := range []string{"session", "returnUrl", "logoutUrl"} {
			if !fields[name] {
				return bookingForm{}, &Error{Code: "libcal_invalid_form", Message: "LibCal checkout is missing the required " + name + " field"}
			}
		}
	}
	return form, nil
}

func (tx *checkout) capture(body string) error {
	for _, match := range checkoutInputPattern.FindAllStringSubmatch(body, -1) {
		if match[1] == "session" {
			tx.state.Session = strings.Trim(match[2], `"`)
			return tx.save()
		}
	}
	return nil
}
