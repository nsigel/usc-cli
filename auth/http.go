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

type navigation struct {
	method        string
	url           string
	body          []byte
	contentType   string
	referer       *url.URL
	duoTraceGroup string
}

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

func neverFollowRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func requireHTTPS(target *url.URL) error {
	if target.Scheme != "https" || target.Host == "" {
		return fmt.Errorf("refusing non-HTTPS authentication URL %q", target.Redacted())
	}
	return nil
}

// pageTimeout bounds one page of the sign-in, so a site that stops answering
// fails while the user is still watching.
const pageTimeout = 15 * time.Second

func (a *authenticator) submit(ctx context.Context, current *page, form form) (*page, error) {
	ctx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
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

		a.rememberMicrosoft(data)
	}
	return &page{URL: request.URL, Status: response.StatusCode, Header: response.Header, Body: data, request: nav}, nil
}

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
