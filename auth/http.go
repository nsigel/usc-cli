package auth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/mimic"
)

const (
	chromeVersion  = "151.0.7922.76"
	documentAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
)

type page struct {
	URL                 *url.URL
	Status              int
	Header              http.Header
	Body                []byte
	RedirectMethod      string
	RedirectBody        []byte
	RedirectContentType string
	RedirectReferer     *url.URL
}

func newAuthenticator() (*authenticator, error) {
	jar, err := newSessionJar()
	if err != nil {
		return nil, err
	}
	// Microsoft and Duo select different flows from the TLS fingerprint. A
	// generic Go client can be sent to browser-only challenges it cannot run.
	transport, err := mimic.NewTransport(mimic.TransportOptions{
		Version:  chromeVersion,
		Brand:    mimic.BrandChrome,
		Platform: mimic.PlatformMac,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	})
	if err != nil {
		return nil, err
	}
	return &authenticator{
		jar:             jar,
		duoClientHintUA: transport.DefaultHeaders.Get("sec-ch-ua"),
		microsoft:       make(map[string]string),
		client: &http.Client{
			Transport:     transport,
			Jar:           jar,
			Timeout:       45 * time.Second,
			CheckRedirect: neverFollowRedirects,
		},
	}, nil
}

// neverFollowRedirects hands every redirect back to the SSO state machine,
// which must see each identity-provider boundary to choose the next step.
func neverFollowRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// requireHTTPS is checked on every hop, since each can carry cookies or
// signed SSO state.
func requireHTTPS(target *url.URL) error {
	if target.Scheme != "https" || target.Host == "" {
		return fmt.Errorf("refusing non-HTTPS authentication URL %q", target.Redacted())
	}
	return nil
}

func (a *authenticator) submit(ctx context.Context, current *page, form form) (*page, error) {
	destination, err := current.URL.Parse(form.Action)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(form.Method)
	if method == "" {
		method = http.MethodGet
	}
	encodedFields := encodeFields(form.Fields)
	if method == http.MethodGet {
		if encodedFields != "" {
			if destination.RawQuery != "" {
				destination.RawQuery += "&"
			}
			destination.RawQuery += encodedFields
		}
		return a.do(ctx, method, destination.String(), nil, "", current.URL, "")
	}
	return a.do(ctx, method, destination.String(), []byte(encodedFields), "application/x-www-form-urlencoded", current.URL, "")
}

func (a *authenticator) do(ctx context.Context, method, rawURL string, body []byte, contentType string, referer *url.URL, duoTraceGroup string) (*page, error) {
	request, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := requireHTTPS(request.URL); err != nil {
		return nil, err
	}
	request.Header.Set("Accept", documentAccept)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != nil {
		// Browsers apply strict-origin-when-cross-origin by default. Several SAML
		// consumers reject a cross-site form POST whose Referer contains the IdP's
		// full state-bearing URL instead of only its origin.
		refererValue := referer.String()
		if request.URL.Scheme != referer.Scheme || request.URL.Host != referer.Host {
			refererValue = referer.Scheme + "://" + referer.Host + "/"
		}
		request.Header.Set("Referer", refererValue)
		if method == http.MethodGet {
			fetchSite := "cross-site"
			if request.URL.Scheme == referer.Scheme && request.URL.Host == referer.Host {
				fetchSite = "same-origin"
			}
			request.Header.Set("Sec-Fetch-Dest", "document")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Site", fetchSite)
			request.Header.Set("Sec-Fetch-User", "?1")
			request.Header.Set("Upgrade-Insecure-Requests", "1")
		}
	}
	if method == http.MethodPost {
		// USC's identity providers reject cross-site form posts that do not look
		// like the browser page that produced them.
		origin := request.URL.Scheme + "://" + request.URL.Host
		if referer != nil {
			origin = referer.Scheme + "://" + referer.Host
		}
		request.Header.Set("Origin", origin)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if duoTraceGroup != "" {
		request.Header.Set("X-Duo-Req-Trace-Group", duoTraceGroup)
	}

	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if request.URL.Hostname() == "login.microsoftonline.com" {
		// Microsoft spreads continuation tokens over multiple pages, and later
		// pages frequently omit values that are still required.
		a.rememberMicrosoft(data)
	}
	result := &page{
		URL:    request.URL,
		Status: response.StatusCode,
		Header: response.Header,
		Body:   data,
	}
	if response.StatusCode == http.StatusTemporaryRedirect || response.StatusCode == http.StatusPermanentRedirect {
		result.RedirectMethod = method
		result.RedirectBody = append([]byte(nil), body...)
		result.RedirectContentType = contentType
		if referer != nil {
			copy := *referer
			result.RedirectReferer = &copy
		}
	}
	return result, nil
}
