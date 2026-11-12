package auth

import (
	"bytes"
	"cmp"
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

// navigation is one browser request in the sign-in flow.
type navigation struct {
	method        string // GET when empty
	url           string
	body          []byte
	contentType   string
	referer       *url.URL
	duoTraceGroup string
}

// page is a response in the sign-in flow and the navigation that fetched it.
type page struct {
	URL     *url.URL
	Status  int
	Header  http.Header
	Body    []byte
	request navigation
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

// submit sends a form the way a browser would from the page that holds it.
func (a *authenticator) submit(ctx context.Context, current *page, form form) (*page, error) {
	destination, err := current.URL.Parse(form.Action)
	if err != nil {
		return nil, err
	}
	fields := encodeFields(form.Fields)
	if method := strings.ToUpper(form.Method); method != "" && method != http.MethodGet {
		return a.do(ctx, navigation{
			method: method, url: destination.String(), referer: current.URL,
			body: []byte(fields), contentType: "application/x-www-form-urlencoded",
		})
	}
	if fields != "" {
		if destination.RawQuery != "" {
			destination.RawQuery += "&"
		}
		destination.RawQuery += fields
	}
	return a.get(ctx, destination.String(), current.URL)
}

func (a *authenticator) get(ctx context.Context, target string, referer *url.URL) (*page, error) {
	return a.do(ctx, navigation{url: target, referer: referer})
}

func (a *authenticator) do(ctx context.Context, nav navigation) (*page, error) {
	request, err := newRequest(ctx, nav)
	if err != nil {
		return nil, err
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
	if isMicrosoftHost(request.URL) {
		// Microsoft spreads continuation tokens over multiple pages, and later
		// pages frequently omit values that are still required.
		a.rememberMicrosoft(data)
	}
	return &page{URL: request.URL, Status: response.StatusCode, Header: response.Header, Body: data, request: nav}, nil
}

// newRequest builds a navigation with the headers a browser sends. The
// identity providers reject requests that do not look like the page that
// produced them.
func newRequest(ctx context.Context, nav navigation) (*http.Request, error) {
	method := cmp.Or(nav.method, http.MethodGet)
	request, err := http.NewRequestWithContext(ctx, method, nav.url, bytes.NewReader(nav.body))
	if err != nil {
		return nil, err
	}
	if err := requireHTTPS(request.URL); err != nil {
		return nil, err
	}
	request.Header.Set("Accept", documentAccept)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if nav.contentType != "" {
		request.Header.Set("Content-Type", nav.contentType)
	}
	if nav.duoTraceGroup != "" {
		request.Header.Set("X-Duo-Req-Trace-Group", nav.duoTraceGroup)
	}
	origin := request.URL.Scheme + "://" + request.URL.Host
	if nav.referer != nil {
		sameOrigin := origin == nav.referer.Scheme+"://"+nav.referer.Host
		origin = nav.referer.Scheme + "://" + nav.referer.Host
		// Browsers apply strict-origin-when-cross-origin by default. Several SAML
		// consumers reject a cross-site request whose Referer carries the
		// identity provider's full state-bearing URL instead of only its origin.
		if sameOrigin {
			request.Header.Set("Referer", nav.referer.String())
		} else {
			request.Header.Set("Referer", origin+"/")
		}
		if method == http.MethodGet {
			fetchSite := "cross-site"
			if sameOrigin {
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
		request.Header.Set("Origin", origin)
	}
	return request, nil
}
