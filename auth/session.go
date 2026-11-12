package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/fhttp/cookiejar"
)

const sessionVersion = 1

// StoredCookie is a session cookie plus the URL that set it, which is needed
// to restore host-only and path-scoped cookies. Never log Cookie.Value.
type StoredCookie struct {
	SetBy  string      `json:"set_by"`
	Cookie http.Cookie `json:"cookie"`
}

type savedSession struct {
	Version int            `json:"version"`
	Cookies []StoredCookie `json:"cookies"`
}

// LoadStoredCookies reads the cookies in a saved session file. A missing file
// returns ErrSessionNotFound.
func LoadStoredCookies(sessionFile string) ([]StoredCookie, error) {
	session, err := readSession(sessionFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrSessionNotFound
	}
	return session.Cookies, err
}

func readSession(name string) (savedSession, error) {
	var session savedSession
	data, err := os.ReadFile(name)
	if err != nil {
		return session, err
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return session, fmt.Errorf("read session: %w", err)
	}
	if session.Version != sessionVersion {
		return session, fmt.Errorf("unsupported session version %d", session.Version)
	}
	return session, nil
}

// sessionJar keeps the response URL that set each cookie, which the standard
// jar hides.
type sessionJar struct {
	inner   *cookiejar.Jar
	entries map[string]StoredCookie
}

func newSessionJar() (*sessionJar, error) {
	inner, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &sessionJar{inner: inner, entries: make(map[string]StoredCookie)}, nil
}

func (j *sessionJar) Cookies(url *url.URL) []*http.Cookie {
	return j.inner.Cookies(url)
}

func (j *sessionJar) SetCookies(source *url.URL, cookies []*http.Cookie) {
	j.inner.SetCookies(source, cookies)
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		key := cookieKey(source, cookie)
		stored := cloneCookie(cookie)
		// Max-Age is relative to the response, so store an absolute expiry;
		// otherwise a reloaded cookie would start its lifetime over.
		if stored.MaxAge > 0 {
			stored.Expires = time.Now().Add(time.Duration(stored.MaxAge) * time.Second).UTC()
			stored.MaxAge = 0
		}
		if stored.MaxAge < 0 || expired(stored) {
			delete(j.entries, key)
			continue
		}
		j.entries[key] = StoredCookie{SetBy: cookieSource(source), Cookie: stored}
	}
}

func (j *sessionJar) snapshot() []StoredCookie {
	cookies := make([]StoredCookie, 0, len(j.entries))
	for _, cookie := range j.entries {
		if !expired(cookie.Cookie) {
			cookies = append(cookies, cookie)
		}
	}
	// Stable ordering prevents meaningless session-file churn between commands.
	sort.Slice(cookies, func(i, j int) bool {
		left := cookies[i].SetBy + "\x00" + cookies[i].Cookie.Name
		right := cookies[j].SetBy + "\x00" + cookies[j].Cookie.Name
		return left < right
	})
	return cookies
}

func expired(cookie http.Cookie) bool {
	return !cookie.Expires.IsZero() && !cookie.Expires.After(time.Now())
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

func cookieSource(source *url.URL) string {
	// Queries can contain signed SAML state. Cookies only need the origin and
	// path to recover host-only and default-path behavior.
	clean := *source
	clean.RawQuery = ""
	clean.Fragment = ""
	return clean.String()
}

func cloneCookie(cookie *http.Cookie) http.Cookie {
	clone := *cookie
	clone.Unparsed = append([]string(nil), cookie.Unparsed...)
	return clone
}

// loadSession restores a saved session into jar, skipping ignoredDomains. A
// missing file is an empty session.
func loadSession(name string, jar *sessionJar, ignoredDomains []string) error {
	session, err := readSession(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range session.Cookies {
		if cookieMatchesDomains(entry, ignoredDomains) {
			continue
		}
		source, err := url.ParseRequestURI(entry.SetBy)
		if err != nil || source.Scheme != "https" || source.Host == "" {
			return errors.New("invalid cookie source in session")
		}
		jar.SetCookies(source, []*http.Cookie{&entry.Cookie})
	}
	return nil
}

func saveSessionReplacingDomains(name string, jar *sessionJar, domains, preserve []string) error {
	existing, err := newSessionJar()
	if err != nil {
		return err
	}
	if err := loadSession(name, existing, nil); err != nil {
		return err
	}
	// Keep only the explicitly preserved cookies from disk. Re-merging all
	// saved cookies would resurrect cookies deleted during authentication.
	for key, entry := range existing.entries {
		if !cookieMatchesDomains(entry, domains) || !slices.Contains(preserve, entry.Cookie.Name) {
			delete(existing.entries, key)
		}
	}
	for key, entry := range jar.entries {
		if cookieMatchesDomains(entry, domains) && slices.Contains(preserve, entry.Cookie.Name) {
			continue
		}
		existing.entries[key] = entry
	}
	return saveSession(name, existing)
}

func cookieMatchesDomains(entry StoredCookie, domains []string) bool {
	if len(domains) == 0 {
		return false
	}
	source, err := url.ParseRequestURI(entry.SetBy)
	if err == nil && hostMatchesDomains(source.Hostname(), domains) {
		return true
	}
	return hostMatchesDomains(entry.Cookie.Domain, domains)
}

func hostMatchesDomains(host string, domains []string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), ".")
	for _, domain := range domains {
		domain = strings.TrimPrefix(strings.ToLower(domain), ".")
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func saveSession(name string, jar *sessionJar) error {
	data, err := json.MarshalIndent(savedSession{Version: sessionVersion, Cookies: jar.snapshot()}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(name, append(data, '\n'))
}
