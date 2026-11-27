package weblogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// Wait blocks until the sign-in page at pageURL finishes or stops serving.
// It returns an error only when the page reports a failed sign-in.
func Wait(ctx context.Context, pageURL string) error {
	page, err := url.Parse(pageURL)
	if err != nil || page.Scheme != "http" || page.Hostname() != "127.0.0.1" || page.Fragment == "" {
		return errors.New("not a sign-in page URL")
	}
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+page.Host+"/api/wait", nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+page.Fragment)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return ctx.Err()
		}
		var current state
		err = json.NewDecoder(response.Body).Decode(&current)
		response.Body.Close()
		switch {
		case response.StatusCode != http.StatusOK || err != nil:
			return errors.New("not a sign-in page URL")
		case current.Step == stepDone:
			return nil
		case current.Step == stepFailed:
			return errors.New(current.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
