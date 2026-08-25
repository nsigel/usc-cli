// Package auth implements USC's shared SSO flow without exposing
// secret-bearing values.
package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/fhttp/cookiejar"
	"github.com/saucesteals/mimic"
	"golang.org/x/net/html"
)

const (
	chromeVersion  = "151.0.0.0"
	documentAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
)

// Credentials are used only when an existing session cannot reach the target.
type Credentials struct{ Username, Password, BypassCode string }
type field struct{ Name, Value string }
type form struct {
	Method, Action string
	Fields         []field
}
type page struct {
	URL      *url.URL
	Status   int
	Header   http.Header
	Body     []byte
	LoadedAt time.Time
}

type storedCookie struct {
	SetBy  string      `json:"set_by"`
	Cookie http.Cookie `json:"cookie"`
}

type session struct {
	Version int            `json:"version"`
	Cookies []storedCookie `json:"cookies"`
}

// sessionJar records each Set-Cookie response along with its source URL. The
// standard jar hides its internal entries, so replaying SetCookies is the
// only way to persist host-only and path-scoped cookies faithfully.
type sessionJar struct {
	inner   *cookiejar.Jar
	entries map[string]storedCookie
}

func newSessionJar() (*sessionJar, error) {
	inner, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &sessionJar{inner: inner, entries: map[string]storedCookie{}}, nil
}

func (j *sessionJar) Cookies(u *url.URL) []*http.Cookie { return j.inner.Cookies(u) }

func (j *sessionJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.inner.SetCookies(u, cookies)
	source := cookieSource(u)
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		key := cookieKey(u, cookie)
		if cookie.MaxAge < 0 {
			delete(j.entries, key)
			continue
		}
		j.entries[key] = storedCookie{SetBy: source, Cookie: copyCookie(cookie)}
	}
}

func cookieKey(source *url.URL, cookie *http.Cookie) string {
	domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
	if domain == "" {
		domain = strings.ToLower(source.Hostname())
	}
	path := cookie.Path
	if path == "" {
		path = defaultCookiePath(source.Path)
	}
	return domain + "\x00" + path + "\x00" + cookie.Name
}

func defaultCookiePath(requestPath string) string {
	if requestPath == "" || requestPath[0] != '/' || strings.Count(requestPath, "/") <= 1 {
		return "/"
	}
	return requestPath[:strings.LastIndex(requestPath, "/")]
}

func (j *sessionJar) snapshot() []storedCookie {
	cookies := make([]storedCookie, 0, len(j.entries))
	for _, cookie := range j.entries {
		cookies = append(cookies, cookie)
	}
	sort.Slice(cookies, func(i, k int) bool {
		return cookies[i].SetBy+"\x00"+cookies[i].Cookie.Name < cookies[k].SetBy+"\x00"+cookies[k].Cookie.Name
	})
	return cookies
}

func cookieSource(u *url.URL) string {
	copy := *u
	copy.RawQuery = ""
	copy.Fragment = ""
	return copy.String()
}

func copyCookie(cookie *http.Cookie) http.Cookie {
	copy := *cookie
	copy.Unparsed = append([]string(nil), cookie.Unparsed...)
	return copy
}

type sso struct {
	client          *http.Client
	jar             *sessionJar
	saml2Request    string            // SAML state carried across the USC redirect flow
	microsoft       map[string]string // values JavaScript keeps across Microsoft pages
	usedCredentials bool
}

const DefaultTarget = "https://webreg.usc.edu/Terms"

var (
	ErrCredentialsRequired = errors.New("USC credentials are required")
	ErrBypassRejected      = errors.New("Duo bypass code was rejected")
)

// Result describes the authenticated page reached by Login.
type Result struct {
	URL             string
	Status          int
	Reauthenticated bool
}

// Login restores a saved session, authenticates when necessary, and persists
// the resulting cookies.
func Login(ctx context.Context, target, sessionFile string, creds Credentials) (Result, error) {
	b, err := newSSO()
	if err != nil {
		return Result{}, err
	}
	if err := loadSession(sessionFile, b.jar); err != nil {
		return Result{}, err
	}
	p, err := b.open(ctx, target, creds)
	if err != nil {
		return Result{}, err
	}
	if err := saveSession(sessionFile, b.jar); err != nil {
		return Result{}, err
	}
	return Result{URL: p.URL.String(), Status: p.Status, Reauthenticated: b.usedCredentials}, nil
}

func newSSO() (*sso, error) {
	jar, err := newSessionJar()
	if err != nil {
		return nil, err
	}
	transport, err := mimic.NewTransport(mimic.TransportOptions{
		Version: chromeVersion, Brand: mimic.BrandChrome, Platform: mimic.PlatformMac,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	})
	if err != nil {
		return nil, err
	}
	return &sso{jar: jar, microsoft: map[string]string{}, client: &http.Client{Transport: transport, Jar: jar, Timeout: 45 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (b *sso) open(ctx context.Context, target string, c Credentials) (*page, error) {
	targetURL, err := url.ParseRequestURI(target)
	if err != nil || targetURL.Scheme != "https" || targetURL.Host == "" {
		return nil, fmt.Errorf("target must be an absolute HTTPS URL")
	}
	p, err := b.do(ctx, http.MethodGet, targetURL.String(), nil, "", nil)
	if err != nil {
		return nil, err
	}
	for steps := 0; steps < 40; steps++ {
		if p.Status >= 300 && p.Status < 400 {
			p, err = b.follow(ctx, p)
			if err != nil {
				return nil, err
			}
			continue
		}
		if p.URL.Hostname() == "login.usc.edu" && p.URL.Path == "/login/login" {
			if err := c.require(); err != nil {
				return nil, err
			}
			b.usedCredentials = true
			f, err := firstForm(p)
			if err != nil {
				return nil, fmt.Errorf("USC login form: %w", err)
			}
			setField(&f, "j_username", c.Username)
			setField(&f, "j_password", c.Password)
			p, err = b.submit(ctx, p, f)
			if err != nil {
				return nil, err
			}
			continue
		}
		if p.URL.Hostname() == "login.usc.edu" && p.URL.Path == "/sso/SSORedirect/metaAlias/USCRealm/idp" {
			loginURL := elementValue(p.Body, "loginUrl")
			if loginURL != "" { // saml2-write.js bootstrap page
				b.saml2Request = elementValue(p.Body, "saml2Request")
				if b.saml2Request == "" {
					return nil, errors.New("USC SAML redirect page is missing saml2Request")
				}
				p, err = b.do(ctx, http.MethodGet, loginURL, nil, "", p.URL)
				if err != nil {
					return nil, err
				}
				continue
			}
		}
		if p.URL.Hostname() == "login.usc.edu" && p.URL.Path == "/sso/saml2/continue/metaAlias/USCRealm/idp" {
			f, err := firstForm(p)
			if err != nil {
				return nil, fmt.Errorf("USC SAML continuation form: %w", err)
			}
			setField(&f, "saml2Request", b.saml2Request)
			if secondVisitURL := p.URL.Query().Get("secondVisitUrl"); secondVisitURL != "" {
				f.Action = secondVisitURL
			}
			p, err = b.submit(ctx, p, f)
			if err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(p.URL.Host, "api-") && strings.Contains(p.URL.Host, ".duosecurity.com") && strings.HasPrefix(p.URL.Path, "/prompt/") {
			p, err = b.duo(ctx, p, c.BypassCode)
			if err != nil {
				return nil, err
			}
			continue
		}
		if p.URL.Host == targetURL.Host && p.Status >= 200 && p.Status < 300 {
			return p, nil
		}
		if p.URL.Host == "login.microsoftonline.com" && strings.HasSuffix(p.URL.Path, "/oauth2/v2.0/authorize") && p.URL.Query().Get("sso_reload") != "true" {
			u := *p.URL
			u.RawQuery += "&sso_reload=true"
			p, err = b.do(ctx, http.MethodGet, u.String(), nil, "", p.URL)
			if err != nil {
				return nil, err
			}
			continue
		}
		if p.URL.Host == "login.microsoftonline.com" {
			if f, ok := microsoftRedirectForm(p); ok {
				p, err = b.submit(ctx, p, f)
				if err != nil {
					return nil, err
				}
				continue
			}
		}
		if p.URL.Host == "login.microsoftonline.com" {
			if p.URL.Path == "/login.srf" {
				me, getErr := b.do(ctx, http.MethodGet, "https://login.live.com/Me.htm", nil, "", p.URL)
				if getErr != nil {
					return nil, getErr
				}
				if me.Status != 200 {
					return nil, fmt.Errorf("Microsoft account bootstrap returned %d", me.Status)
				}
				b.rememberMicrosoft(me.Body)
			}
			f, ok := microsoftForm(p, b.microsoft)
			if !ok {
				// Some Microsoft pages are navigations, not postbacks.  Let the
				// generic HTML navigation handler process those below.
			} else {
				if missing := missingMicrosoftFormValues(p, b.microsoft); len(missing) != 0 {
					return nil, fmt.Errorf("Microsoft %s did not expose required SSO state: %s", p.URL.Path, strings.Join(missing, ","))
				}
				p, err = b.submit(ctx, p, f)
				if err != nil {
					return nil, err
				}
				continue
			}
		}
		if f, ok := autoForm(p); ok {
			p, err = b.submit(ctx, p, f)
			if err != nil {
				return nil, err
			}
			continue
		}
		if target, ok := htmlNavigation(p); ok {
			p, err = b.do(ctx, http.MethodGet, target.String(), nil, "", p.URL)
			if err != nil {
				return nil, err
			}
			continue
		}
		if p.URL.Host == "login.microsoftonline.com" {
			return nil, fmt.Errorf("Microsoft page has no form or supported redirect (title: %q)", pageTitle(p.Body))
		}
		return nil, fmt.Errorf("no safe continuation for %s%s (HTTP %d)", p.URL.Host, p.URL.Path, p.Status)
	}
	return nil, errors.New("authentication exceeded 40 protocol steps")
}

func (b *sso) follow(ctx context.Context, p *page) (*page, error) {
	loc := p.Header.Get("Location")
	if loc == "" {
		return nil, fmt.Errorf("redirect from %s%s has no Location", p.URL.Host, p.URL.Path)
	}
	u, err := p.URL.Parse(loc)
	if err != nil {
		return nil, err
	}
	return b.do(ctx, http.MethodGet, u.String(), nil, "", p.URL)
}

func (b *sso) duo(ctx context.Context, prompt *page, bypassCode string) (*page, error) {
	authkey := prompt.URL.Query().Get("authkey")
	traceGroup := prompt.URL.Query().Get("req_trace_group")
	if authkey == "" || traceGroup == "" {
		return nil, errors.New("Duo prompt is missing authkey or req_trace_group")
	}
	base := "https://" + prompt.URL.Host + prompt.URL.Path
	features, hints := duoClientDetails()
	payloadURL := base + "/auth/payload?" + orderedQuery([]field{{"authkey", authkey}, {"browser_features", features}, {"is_ipad", "false"}, {"client_hints", hints}})
	payload, err := b.do(ctx, http.MethodGet, payloadURL, nil, "", prompt.URL)
	if err != nil {
		return nil, err
	}
	if payload.Status != 200 {
		return nil, fmt.Errorf("Duo auth/payload returned %d", payload.Status)
	}
	ids := jsonStrings(payload.Body)
	for _, event := range []string{"index", "device_health", "pre_authn_eval"} {
		if err := b.duoEvent(ctx, base, authkey, traceGroup, prompt.URL, event, ids); err != nil {
			return nil, err
		}
	}
	evalURL := base + "/pre_authn/evaluation?" + orderedQuery([]field{{"authkey", authkey}, {"browser_features", features}, {"local_trust_choice", "trusted"}})
	eval, err := b.do(ctx, http.MethodGet, evalURL, nil, "", prompt.URL, duoTrace(traceGroup))
	if err != nil {
		return nil, err
	}
	if eval.Status != 200 {
		return nil, fmt.Errorf("Duo pre_authn/evaluation returned %d", eval.Status)
	}

	body, _ := json.Marshal(map[string]string{"authkey": authkey, "bypass_code": bypassCode})
	factor, err := b.do(ctx, http.MethodPost, base+"/auth/factors/bypass_code", body, "application/json", prompt.URL, duoTrace(traceGroup))
	if err != nil {
		return nil, err
	}
	if err := validateDuoFactor(factor.Status, factor.Body); err != nil {
		return nil, err
	}
	if err := b.duoEvent(ctx, base, authkey, traceGroup, prompt.URL, "auth_success", ids); err != nil {
		return nil, err
	}

	params := []field{{"authkey", authkey}}
	if code := jsonStrings(factor.Body)["oidc_code"]; code != "" {
		params = append(params, field{"oidc_code", code})
	}
	final, err := b.do(ctx, http.MethodGet, base+"/auth/finalize_auth?"+orderedQuery(params), nil, "", prompt.URL, duoTrace(traceGroup))
	if err != nil {
		return nil, err
	}
	if final.Status != 200 {
		return nil, fmt.Errorf("Duo finalize_auth returned %d", final.Status)
	}
	exitURL := jsonStrings(final.Body)["url"]
	if exitURL == "" {
		return nil, errors.New("Duo finalization did not return an exit URL")
	}
	return b.do(ctx, http.MethodGet, exitURL, nil, "", prompt.URL)
}

func (b *sso) duoEvent(ctx context.Context, base, authkey, traceGroup string, referer *url.URL, view string, ids map[string]string) error {
	context := map[string]any{"current_view": view, "view_history": "", "message": "page loaded", "platform_authenticator_status": "available", "platform_id": "macos", "req-trace-group": traceGroup}
	if view != "index" {
		context["card_name"] = map[string]string{"device_health": "DeviceHealthCard", "pre_authn_eval": "PreAuthnEvaluationCard", "auth_success": "SuccessCard"}[view]
		context["auth_flow"] = "mfa"
		context["authn_result"] = map[string]string{"status": "not_started"}
		for _, k := range []string{"akey", "ikey", "ukey"} {
			if ids[k] != "" {
				context[k] = ids[k]
			}
		}
	}
	name := "card_visit"
	if view == "index" {
		name = "platform info"
	}
	body, _ := json.Marshal(map[string]any{"context": context, "name": name, "level": "info"})
	p, err := b.do(ctx, http.MethodPost, base+"/auth/browser_events?"+orderedQuery([]field{{"authkey", authkey}}), body, "application/json", referer, duoTrace(traceGroup))
	if err != nil {
		return err
	}
	if p.Status != 200 {
		return fmt.Errorf("Duo event endpoint returned %d", p.Status)
	}
	return nil
}

func duoTrace(v string) http.Header { h := http.Header{}; h.Set("X-Duo-Req-Trace-Group", v); return h }

func (b *sso) submit(ctx context.Context, p *page, f form) (*page, error) {
	u, err := p.URL.Parse(f.Action)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(f.Method)
	if method == "" {
		method = http.MethodGet
	}
	if method == http.MethodGet {
		if q := orderedQuery(f.Fields); q != "" {
			if u.RawQuery != "" {
				u.RawQuery += "&"
			}
			u.RawQuery += q
		}
		return b.do(ctx, method, u.String(), nil, "", p.URL)
	}
	return b.do(ctx, method, u.String(), []byte(orderedQuery(f.Fields)), "application/x-www-form-urlencoded", p.URL)
}

func (b *sso) do(ctx context.Context, method, rawURL string, body []byte, contentType string, referer *url.URL, extra ...http.Header) (*page, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	if contentType == "application/json" {
		req.Header.Set("Accept", "*/*")
	} else {
		req.Header.Set("Accept", documentAccept)
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	req.Header.Set("Sec-Fetch-Dest", fetchDest(contentType))
	req.Header.Set("Sec-Fetch-Mode", fetchMode(contentType))
	req.Header.Set("Sec-Fetch-Site", fetchSite(referer, req.URL))
	if referer != nil {
		req.Header.Set("Referer", navigationReferer(referer, req.URL))
	}
	if method == http.MethodPost {
		origin := req.URL.Scheme + "://" + req.URL.Host
		if referer != nil {
			origin = referer.Scheme + "://" + referer.Host
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", contentType)
	}
	for _, h := range extra {
		for k, v := range h {
			req.Header[k] = v
		}
	}
	// The HAR uses these two brands on macOS Chrome 151. Mimic still supplies
	// the matching Chrome TLS and HTTP/2 fingerprints.
	req.Header.Set("sec-ch-ua", `"Chromium";v="151", "Not=A?Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header[http.HeaderOrderKey] = []string{"accept", "accept-encoding", "accept-language", "cache-control", "content-type", "origin", "pragma", "priority", "referer", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site", "upgrade-insecure-requests", "user-agent", "x-duo-req-trace-group"}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if req.URL.Hostname() == "login.microsoftonline.com" {
		b.rememberMicrosoft(data)
	}
	return &page{URL: req.URL, Status: resp.StatusCode, Header: resp.Header, Body: data, LoadedAt: time.Now()}, nil
}

func fetchDest(contentType string) string {
	if contentType == "application/json" {
		return "empty"
	}
	return "document"
}
func fetchMode(contentType string) string {
	if contentType == "application/json" {
		return "cors"
	}
	return "navigate"
}
func fetchSite(from, to *url.URL) string {
	if from == nil {
		return "none"
	}
	if from.Host == to.Host {
		return "same-origin"
	}
	return "cross-site"
}
func navigationReferer(from, to *url.URL) string {
	if from.Hostname() == to.Hostname() {
		return from.String()
	}
	return from.Scheme + "://" + from.Host + "/"
}

func firstForm(p *page) (form, error) {
	f, ok := autoForm(p)
	if !ok {
		return form{}, errors.New("no form found")
	}
	return f, nil
}
func autoForm(p *page) (form, bool) {
	z := html.NewTokenizer(bytes.NewReader(p.Body))
	var f form
	inForm := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return f, inForm && len(f.Fields) > 0
		}
		t := z.Token()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			if t.Data == "form" && !inForm {
				inForm = true
				f.Method = "GET"
				for _, a := range t.Attr {
					if a.Key == "method" {
						f.Method = a.Val
					}
					if a.Key == "action" {
						f.Action = a.Val
					}
				}
			} else if inForm && (t.Data == "input" || t.Data == "button") {
				var name, value string
				for _, a := range t.Attr {
					if a.Key == "name" {
						name = a.Val
					}
					if a.Key == "value" {
						value = a.Val
					}
				}
				if name != "" {
					f.Fields = append(f.Fields, field{name, value})
				}
			}
		case html.EndTagToken:
			if t.Data == "form" && inForm {
				return f, len(f.Fields) > 0
			}
		}
	}
}

var navRE = regexp.MustCompile(`(?is)(?:url\s*=|(?:window\.)?location(?:\.href)?\s*=|location\.replace\()\s*["']?([^"'<>\s)]+)`)

func htmlNavigation(p *page) (*url.URL, bool) {
	m := navRE.FindSubmatch(p.Body)
	if len(m) < 2 {
		return nil, false
	}
	u, err := p.URL.Parse(string(m[1]))
	return u, err == nil
}
func pageTitle(body []byte) string {
	z := html.NewTokenizer(bytes.NewReader(body))
	inTitle := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken:
			if z.Token().Data == "title" {
				inTitle = true
			}
		case html.TextToken:
			if inTitle {
				return strings.TrimSpace(z.Token().Data)
			}
		case html.EndTagToken:
			if z.Token().Data == "title" {
				return ""
			}
		}
	}
}
func elementValue(body []byte, id string) string {
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			var gotID, value string
			for _, a := range t.Attr {
				if a.Key == "id" {
					gotID = a.Val
				}
				if a.Key == "value" {
					value = a.Val
				}
			}
			if gotID == id {
				return value
			}
		}
	}
}
func (b *sso) rememberMicrosoft(body []byte) {
	for k, v := range microsoftValues(body) {
		if v != "" {
			b.microsoft[k] = v
		}
	}
}
func microsoftForm(p *page, stored map[string]string) (form, bool) {
	values := map[string]string{}
	for k, v := range stored {
		values[k] = v
	}
	current := microsoftValues(p.Body)
	for k, v := range current {
		if v != "" {
			values[k] = v
		}
	}
	// urlPost identifies the current page's transition.  Retaining it from
	// a prior page turns a terminal SAML auto-post page into a duplicate
	// Microsoft postback, so only take the action from this response.
	action := firstValue(current, "urlPost", "urlPostMsa")
	if action == "" {
		return form{}, false
	}
	pageTime := firstValue(values, "i19")
	if pageTime == "" {
		// ConvergedCmsi's instrumentation-control sets i19 to
		// Date.now() - performance.timing.loadEventEnd immediately before
		// form submission.  The response-read timestamp is our equivalent
		// load-complete marker in the raw client.
		pageTime = strconv.FormatInt(max(0, time.Since(p.LoadedAt).Milliseconds()), 10)
	}
	common := []field{{"ctx", firstValue(values, "sCtx", "ctx")}, {"hpgrequestid", firstValue(values, "sessionId", "hpgrequestid")}, {"flowToken", firstValue(values, "sFT", "flowToken")}, {"canary", firstValue(values, "canary")}, {"i19", pageTime}}
	postPath, err := p.URL.Parse(action)
	if err != nil {
		return form{}, false
	}
	switch postPath.Path {
	case "/appverify":
		return form{Method: http.MethodPost, Action: action, Fields: []field{{"ContinueAuth", "true"}, common[0], common[1], common[2], {"iscsrfspeedbump", "true"}, common[3], common[4]}}, true
	case "/kmsi":
		return form{Method: http.MethodPost, Action: action, Fields: []field{{"LoginOptions", "1"}, {"type", "28"}, common[0], common[1], common[2], common[3], common[4]}}, true
	default:
		return form{}, false
	}
}
func microsoftRedirectForm(p *page) (form, bool) {
	cfg := microsoftConfig(p.Body)
	if cfg == nil {
		return form{}, false
	}
	action, _ := cfg["urlPost"].(string)
	params, _ := cfg["oPostParams"].(map[string]any)
	if action == "" || len(params) == 0 {
		return form{}, false
	}
	fields := make([]field, 0, len(params))
	for name, raw := range params {
		value, ok := raw.(string)
		if !ok {
			return form{}, false
		}
		// BssoInterrupt_Core decodes the JSON values before placing them in
		// hidden inputs (Helper.htmlUnescape).  The config deliberately
		// encodes characters such as '&' and '+'; submitting its wire form
		// literally changes the signed SAML payload.
		fields = append(fields, field{Name: stdhtml.UnescapeString(name), Value: stdhtml.UnescapeString(value)})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return form{Method: http.MethodPost, Action: action, Fields: fields}, true
}
func microsoftValues(body []byte) map[string]string {
	values := map[string]string{}
	for _, cfg := range []map[string]any{microsoftConfig(body), microsoftServerData(body)} {
		if cfg != nil {
			for k, v := range jsonStrings(mustJSON(cfg)) {
				values[k] = v
			}
		}
	}
	for k, v := range microsoftAssignments(body) {
		values[k] = v
	}
	for _, k := range []string{"sCtx", "sFT", "i19", "ctx", "flowToken"} {
		if value := configString(body, k); value != "" {
			values[k] = value
		}
		if values[k] == "" {
			values[k] = scriptString(body, k)
		}
	}
	return values
}
func configString(body []byte, key string) string {
	re := regexp.MustCompile(`(?s)"` + regexp.QuoteMeta(key) + `"\s*:\s*"((?:\\.|[^"\\])*)"`)
	all := re.FindAllSubmatch(body, -1)
	if len(all) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(append(append([]byte{'"'}, all[len(all)-1][1]...), '"'), &value) == nil {
		return value
	}
	return ""
}
func missingMicrosoftFormValues(p *page, stored map[string]string) []string {
	values := map[string]string{}
	for k, v := range stored {
		values[k] = v
	}
	for k, v := range microsoftValues(p.Body) {
		values[k] = v
	}
	var out []string
	for label, names := range map[string][]string{"ctx": {"sCtx", "ctx"}, "flowToken": {"sFT", "flowToken"}, "canary": {"canary"}, "hpgrequestid": {"sessionId", "hpgrequestid"}} {
		if firstValue(values, names...) == "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}
func firstValue(values map[string]string, names ...string) string {
	for _, n := range names {
		if values[n] != "" {
			return values[n]
		}
	}
	return ""
}
func mustJSON(v any) []byte { data, _ := json.Marshal(v); return data }
func microsoftConfig(body []byte) map[string]any {
	if at := bytes.Index(body, []byte("$Config={")); at >= 0 {
		return objectAfter(body[at+len("$Config="):], "")
	}
	return configAssignment(body, "$Config")
}
func microsoftServerData(body []byte) map[string]any { return configAssignment(body, "ServerData") }
func configAssignment(body []byte, marker string) map[string]any {
	var best map[string]any
	bestScore := -1
	for offset := 0; offset < len(body); {
		at := bytes.Index(body[offset:], []byte(marker))
		if at < 0 {
			break
		}
		at += offset
		limit := at + 96
		if limit > len(body) {
			limit = len(body)
		}
		eq := bytes.IndexByte(body[at:limit], '=')
		if eq >= 0 {
			if candidate := objectAfter(body[at+eq+1:], ""); candidate != nil {
				score := len(candidate)
				if _, ok := candidate["urlPost"]; ok {
					score += 10000
				}
				if _, ok := candidate["sCtx"]; ok {
					score += 10000
				}
				if _, ok := candidate["sFT"]; ok {
					score += 10000
				}
				if score > bestScore {
					best, bestScore = candidate, score
				}
			}
		}
		offset = at + len(marker)
	}
	return best
}
func objectAfter(body []byte, marker string) map[string]any {
	i := 0
	if marker != "" {
		i = bytes.Index(body, []byte(marker))
		if i < 0 {
			return nil
		}
		eq := bytes.IndexByte(body[i:], '=')
		if eq < 0 {
			return nil
		}
		i += eq + 1
	}
	open := bytes.IndexByte(body[i:], '{')
	if open < 0 {
		return nil
	}
	i += open
	depth, quote, escaped := 0, byte(0), false
	for j := i; j < len(body); j++ {
		c := body[j]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				var out map[string]any
				if json.Unmarshal(body[i:j+1], &out) == nil {
					return out
				}
				return nil
			}
		}
	}
	return nil
}

var microsoftAssignmentRE = regexp.MustCompile(`(?s)\$Config\.([A-Za-z0-9_]+)\s*=\s*["']([^"']*)["']`)

func microsoftAssignments(body []byte) map[string]string {
	values := map[string]string{}
	for _, m := range microsoftAssignmentRE.FindAllSubmatch(body, -1) {
		values[string(m[1])] = string(m[2])
	}
	return values
}
func scriptString(body []byte, key string) string {
	re := regexp.MustCompile(`(?s)(?:["']?` + regexp.QuoteMeta(key) + `["']?\s*[:=]\s*["'])([^"']*)`)
	m := re.FindSubmatch(body)
	if len(m) == 2 {
		return string(m[1])
	}
	return ""
}
func setField(f *form, name, value string) {
	for i := range f.Fields {
		if f.Fields[i].Name == name {
			f.Fields[i].Value = value
			return
		}
	}
	f.Fields = append(f.Fields, field{name, value})
}
func orderedQuery(fields []field) string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, url.QueryEscape(f.Name)+"="+url.QueryEscape(f.Value))
	}
	return strings.Join(out, "&")
}
func duoClientDetails() (string, string) {
	features := `{"touch_supported":false,"platform_authenticator_status":"available","webauthn_supported":true,"screen_resolution_height":956,"screen_resolution_width":1470,"screen_color_depth":30,"is_uvpa_available":true,"client_capabilities_uvpa":true}`
	hints := `{"brands":[{"brand":"Chromium","version":"151"},{"brand":"Not=A?Brand","version":"99"}],"fullVersionList":[{"brand":"Chromium","version":"151.0.7922.76"},{"brand":"Not=A?Brand","version":"99.0.0.0"}],"mobile":false,"platform":"macOS","platformVersion":"14.8.4","uaFullVersion":"151.0.7922.76"}`
	return features, base64.StdEncoding.EncodeToString([]byte(hints))
}

func jsonStrings(data []byte) map[string]string {
	var root any
	_ = json.Unmarshal(data, &root)
	out := map[string]string{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				if s, ok := y.(string); ok && out[k] == "" {
					out[k] = s
					var embedded any
					if json.Unmarshal([]byte(s), &embedded) == nil {
						walk(embedded)
					}
				}
				walk(y)
			}
		case []any:
			for _, y := range x {
				walk(y)
			}
		}
	}
	walk(root)
	return out
}

func validateDuoFactor(status int, body []byte) error {
	if status == 400 || status == 401 || status == 403 {
		return ErrBypassRejected
	}
	if status != 200 {
		return fmt.Errorf("Duo bypass factor returned %d", status)
	}
	var payload struct {
		Stat     string `json:"stat"`
		Response struct {
			AuthnEvaluation struct {
				IsAllowed bool `json:"is_allowed"`
			} `json:"authn_evaluation"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Stat != "OK" || !payload.Response.AuthnEvaluation.IsAllowed {
			return ErrBypassRejected
		}
		return nil
	}
	return errors.New("Duo bypass factor returned an invalid response")
}

func (c Credentials) require() error {
	if c.Username == "" || c.Password == "" || c.BypassCode == "" {
		return ErrCredentialsRequired
	}
	return nil
}

func loadSession(name string, jar *sessionJar) error {
	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved session
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("read session: %w", err)
	}
	if saved.Version != 1 {
		return fmt.Errorf("unsupported session version %d", saved.Version)
	}
	for _, entry := range saved.Cookies {
		source, err := url.ParseRequestURI(entry.SetBy)
		if err != nil || source.Scheme != "https" || source.Host == "" {
			return fmt.Errorf("invalid cookie source in session")
		}
		jar.SetCookies(source, []*http.Cookie{&entry.Cookie})
	}
	return nil
}

func saveSession(name string, jar *sessionJar) error {
	data, err := json.MarshalIndent(session{Version: 1, Cookies: jar.snapshot()}, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(name); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	dir := filepath.Dir(name)
	temporary, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, name)
}
