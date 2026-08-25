package auth

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"time"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/mimic"
)

const (
	chromeVersion  = "151.0.0.0"
	documentAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
)

type page struct {
	URL    *url.URL
	Status int
	Header http.Header
	Body   []byte
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
		jar:       jar,
		microsoft: make(map[string]string),
		client: &http.Client{
			Transport: transport,
			Jar:       jar,
			Timeout:   45 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (a *authenticator) submit(ctx context.Context, current *page, form form) (*page, error) {
	destination, err := current.URL.Parse(form.Action)
	if err != nil {
		return nil, err
	}
	method := form.Method
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
	request.Header.Set("Accept", documentAccept)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != nil {
		request.Header.Set("Referer", referer.String())
	}
	if method == http.MethodPost {
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
		a.rememberMicrosoft(data)
	}
	return &page{
		URL:    request.URL,
		Status: response.StatusCode,
		Header: response.Header,
		Body:   data,
	}, nil
}
