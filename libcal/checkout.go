package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// checkout is one booking attempt and its crash-recovery record. The record
// (libcal-checkout.json) names the server-side checkout to release if this
// process dies, and marks a submission whose outcome was never seen. Its lock
// file is separate from the atomically replaced record and is never
// unlinked: another process may hold the same inode.
type checkout struct {
	client  *Client
	file    *os.File
	state   checkoutState
	slot    availableSlot
	pending *pendingBooking
}

// beginCheckout takes the checkout lock and releases whatever a previous
// attempt left behind. An unknown submission outcome blocks new bookings
// until Release acknowledges it.
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

// finish releases the attempt's hold unless the booking was confirmed or its
// outcome is unknown, and adds any cleanup failure to resultErr.
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
		cleanupErr = tx.client.removeHold(ctx, tx.slot, *tx.pending)
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
	data, err := c.request(ctx, http.MethodPost, "/ajax/space/session/end", url.Values{"session": {session}}, "application/json", "")
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

const equipmentCheckoutPath = "/ajax/equipment/checkout"

// LibCal's checkout page appends these fields in its submit handler instead
// of rendering them as inputs. Only the known literals are read; no
// JavaScript is evaluated.
var checkoutInputPattern = regexp.MustCompile(`appendHiddenInput\('(?P<name>returnUrl|logoutUrl|session)',\s*("(?:\\.|[^"\\])*"|[0-9]+),`)

// checkoutForm holds the slot for its full duration and returns LibCal's
// checkout form, signing in through USC SSO when LibCal asks for it.
func (c *Client) checkoutForm(ctx context.Context, slot availableSlot, tx *checkout) (bookingForm, error) {
	hold, err := c.addHold(ctx, slot)
	if err != nil {
		return bookingForm{}, err
	}
	tx.slot, tx.pending = slot, &hold
	extended, err := c.extendHold(ctx, slot, hold)
	if err != nil {
		return bookingForm{}, err
	}
	tx.pending = &extended
	page, err := c.checkoutPage(ctx, slot, extended)
	if err != nil {
		return bookingForm{}, err
	}
	html, referer := page.HTML, refererFor(slot.Space)
	var authErr error
	if page.Redirect != "" {
		html, referer, authErr = c.signIn(ctx, page.Redirect, referer)
	}
	// Record the checkout ID even when sign-in failed after reaching the
	// page, so the server-side hold can still be released.
	if err := errors.Join(authErr, tx.capture(html)); err != nil {
		return bookingForm{}, err
	}
	form, err := parseBookingForm(html)
	if err != nil {
		return bookingForm{}, err
	}
	form.Referer = referer
	if form.Action == equipmentCheckoutPath {
		if tx.state.Session == "" {
			return bookingForm{}, errors.New("LibCal checkout is missing its session identifier")
		}
		if err := form.addScriptedFields(html); err != nil {
			return bookingForm{}, err
		}
	}
	return form, nil
}

type checkoutPage struct {
	Redirect string `json:"redirect"`
	HTML     string `json:"html"`
	Error    string `json:"error"`
}

// checkoutPage asks LibCal for the held slot's checkout form. LibCal answers
// with the form's HTML, or with a redirect to its SSO entry.
func (c *Client) checkoutPage(ctx context.Context, slot availableSlot, hold pendingBooking) (checkoutPage, error) {
	category, err := categoryOf(slot.Space)
	if err != nil {
		return checkoutPage{}, err
	}
	values := url.Values{
		"patron":     {""},
		"patronHash": {""},
		"returnUrl":  {category.returnPath()},
		"method":     {bookingMethodID},
	}
	hold.echo(values)
	data, err := c.request(ctx, http.MethodPost, "/ajax/space/times", values, ajaxAccept, refererFor(slot.Space))
	if err != nil {
		return checkoutPage{}, err
	}
	var page checkoutPage
	if err := json.Unmarshal(data, &page); err != nil {
		return checkoutPage{}, fmt.Errorf("decode LibCal booking form: %w", err)
	}
	switch {
	case page.Redirect != "":
		page.Redirect, err = authRedirect(page.Redirect)
		return page, err
	case page.Error != "":
		return checkoutPage{}, libcalError(page.Error)
	case page.HTML == "":
		return checkoutPage{}, errors.New("LibCal returned no reservation form")
	}
	return page, nil
}

// authRedirect accepts only LibCal's own SSO entry as a sign-in target.
func authRedirect(value string) (string, error) {
	base, _ := url.Parse(baseURL)
	target, err := base.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse LibCal authentication redirect: %w", err)
	}
	if target.Scheme != "https" || !strings.EqualFold(target.Hostname(), base.Hostname()) || target.Path != "/spaces/auth" {
		return "", errors.New("LibCal returned an unsupported authentication redirect")
	}
	return target.String(), nil
}

// signIn follows LibCal's SSO redirect and returns the checkout page it lands
// on and that page's URL. The page is returned even when saving the session
// failed.
func (c *Client) signIn(ctx context.Context, target, referer string) (html, landed string, err error) {
	if c.authenticate == nil {
		return "", "", ErrAuthenticationRequired
	}
	response, err := c.authenticate(ctx, target, referer)
	if response == nil {
		return "", "", err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if readErr != nil {
		return "", "", errors.Join(err, readErr)
	}
	return string(data), response.Request.URL.String(), err
}

// addScriptedFields adds the hidden fields the checkout page's script would
// append, all of which the submission requires.
func (form *bookingForm) addScriptedFields(html string) error {
	found := make(map[string]bool)
	for _, match := range checkoutInputPattern.FindAllStringSubmatch(html, -1) {
		name, value := match[1], match[2]
		if strings.HasPrefix(value, `"`) {
			var err error
			if value, err = strconv.Unquote(value); err != nil {
				return errors.New("LibCal checkout has an invalid form value")
			}
		}
		form.Fields = append(form.Fields, bookingFormField{Name: name, Type: "hidden", Value: value})
		found[name] = value != ""
	}
	for _, name := range []string{"session", "returnUrl", "logoutUrl"} {
		if !found[name] {
			return &Error{Code: "libcal_invalid_form", Message: "LibCal checkout is missing the required " + name + " field"}
		}
	}
	return nil
}

// capture records the checkout session ID from a checkout page, if it has one.
func (tx *checkout) capture(html string) error {
	for _, match := range checkoutInputPattern.FindAllStringSubmatch(html, -1) {
		if match[1] == "session" {
			tx.state.Session = strings.Trim(match[2], `"`)
			return tx.save()
		}
	}
	return nil
}
