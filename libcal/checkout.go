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
var ErrBookingUnknown = &Error{Code: "libcal_booking_unknown", Message: "LibCal booking outcome is unknown; check the confirmation email, then release the checkout before booking again"}

// checkoutState is the recovery record. Session names a server-side checkout
// to release. OutcomeUnknown is set before a reservation is submitted and
// cleared only by a definite answer, so a lost response blocks retries.
type checkoutState struct {
	Session        string `json:"session,omitempty"`
	OutcomeUnknown bool   `json:"outcome_unknown,omitempty"`
}

// The lock file is separate from the atomically replaced record and is never
// unlinked: another process may hold the same inode.
type checkout struct {
	client  *Client
	file    *os.File
	state   checkoutState
	slot    availableSlot
	pending *pendingBooking
}

// beginCheckout starts a booking attempt. It refuses while an earlier
// submission's outcome is unknown, and otherwise releases whatever an earlier
// attempt left held.
func (c *Client) beginCheckout(ctx context.Context) (*checkout, error) {
	tx, err := c.openCheckout()
	if err != nil {
		return nil, err
	}
	if tx.state.OutcomeUnknown {
		tx.close()
		return nil, ErrBookingUnknown
	}
	if err := tx.releaseEarlierAttempt(ctx); err != nil {
		tx.close()
		return nil, err
	}
	return tx, nil
}

// openCheckout takes the checkout lock and reads the recovery record. A
// client without a checkout path keeps no record and takes no lock.
func (c *Client) openCheckout() (*checkout, error) {
	tx := &checkout{client: c}
	if c.checkoutPath == "" {
		return tx, nil
	}
	if err := os.MkdirAll(filepath.Dir(c.checkoutPath), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(c.checkoutPath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockCheckout(lock); err != nil {
		lock.Close()
		return nil, &Error{Code: "libcal_checkout_busy", Message: "another LibCal checkout is running; wait for it to finish"}
	}
	tx.file = lock
	data, err := os.ReadFile(c.checkoutPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		tx.close()
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &tx.state); err != nil {
			tx.close()
			return nil, errors.New("cannot read unfinished LibCal checkout state; do not start another booking")
		}
	}
	return tx, nil
}

// releaseEarlierAttempt ends the server-side checkout a previous attempt
// recorded and clears the record, including an unknown-outcome mark.
func (tx *checkout) releaseEarlierAttempt(ctx context.Context) error {
	if tx.state.Session != "" {
		if err := tx.client.endCheckout(ctx, tx.state.Session); err != nil {
			return &Error{Code: "libcal_cleanup_failed", Message: "could not release the unfinished LibCal checkout: " + err.Error(), Cause: err}
		}
	}
	tx.state = checkoutState{}
	return tx.save()
}

func (tx *checkout) close() {
	if tx.file != nil {
		tx.file.Close()
	}
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
	defer tx.close()
	if tx.state.OutcomeUnknown {
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
		*resultErr = errors.Join(*resultErr, &Error{Code: "libcal_cleanup_failed", Message: "could not release the LibCal checkout: " + cleanupErr.Error(), Cause: cleanupErr})
		return
	}
	tx.state = checkoutState{}
	*resultErr = errors.Join(*resultErr, tx.save())
}

// Release ends only an unfinished CLI checkout; it never cancels a confirmed
// reservation. It also acknowledges a previously unknown submission outcome.
func (c *Client) Release(ctx context.Context) error {
	tx, err := c.openCheckout()
	if err != nil {
		return err
	}
	defer tx.close()
	return tx.releaseEarlierAttempt(ctx)
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
// of rendering them as inputs.
var checkoutInputPattern = regexp.MustCompile(`appendHiddenInput\('(?P<name>returnUrl|logoutUrl|session)',\s*("(?:\\.|[^"\\])*"|[0-9]+),`)

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
	// Record the checkout ID before reporting a sign-in failure, so the hold
	// it names is still released.
	if err := tx.capture(html); err != nil || authErr != nil {
		return bookingForm{}, errors.Join(authErr, err)
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
// on and that page's URL. When signing in worked but saving the session did
// not, it returns the page together with the error.
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

func (tx *checkout) capture(html string) error {
	for _, match := range checkoutInputPattern.FindAllStringSubmatch(html, -1) {
		if match[1] == "session" {
			tx.state.Session = strings.Trim(match[2], `"`)
			return tx.save()
		}
	}
	return nil
}
