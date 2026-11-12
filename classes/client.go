// Package classes reads the public USC Schedule of Classes (classes.usc.edu).
// It needs no sign-in.
package classes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
)

const apiURL = "https://classes.usc.edu/api/"

// ErrNotFound means the Schedule of Classes has nothing for the request, such
// as a course code that is not offered in the term.
var ErrNotFound = errors.New("not found in the Schedule of Classes")

// Client reads the public Schedule of Classes API. It is safe for concurrent
// use.
type Client struct {
	http *http.Client
}

// New returns a Schedule of Classes client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNoContent {
		return fmt.Errorf("classes %s: %w", path, ErrNotFound)
	}
	if response.StatusCode != http.StatusOK {
		excerpt, _ := io.ReadAll(io.LimitReader(response.Body, 200))
		return fmt.Errorf("classes %s: HTTP %d: %s", path, response.StatusCode, strings.TrimSpace(string(excerpt)))
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("classes %s: decode response: %w", path, err)
	}
	return nil
}

func validateTermCode(termCode string) error {
	if len(termCode) != 5 || strings.Trim(termCode, "0123456789") != "" {
		return fmt.Errorf("term code must be five digits such as 20271, got %q", termCode)
	}
	return nil
}
