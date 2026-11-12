package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
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
		j.entries[key] = StoredCookie{SetBy: sourceWithoutQuery(source), Cookie: stored}
	}
}

func (j *sessionJar) snapshot() []StoredCookie {
	cookies := make([]StoredCookie, 0, len(j.entries))
	for _, cookie := range j.entries {
		if !expired(cookie.Cookie) {
			cookies = append(cookies, cookie)
		}
	}
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

// sourceWithoutQuery keeps signed SAML state in query strings off disk.
func sourceWithoutQuery(source *url.URL) string {
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

// SessionStore persists one user's cookies. Load returns ErrSessionNotFound
// when no session exists. Implementations must not log cookie values.
// Separate users must have separate stores; concurrent writers need external coordination.
type SessionStore interface {
	Load() ([]StoredCookie, error)
	Save([]StoredCookie) error
}

// FileSessionStore uses the same private, versioned file as SessionFile.
type FileSessionStore struct{ Path string }

func (s FileSessionStore) Load() ([]StoredCookie, error) { return LoadStoredCookies(s.Path) }
func (s FileSessionStore) Save(cookies []StoredCookie) error {
	data, err := json.MarshalIndent(savedSession{Version: sessionVersion, Cookies: cookies}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(s.Path, append(data, '\n'))
}

func loadStoredSession(store SessionStore, jar *sessionJar, ignoredDomains []string) error {
	cookies, err := store.Load()
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range cookies {
		if cookieMatchesDomains(entry, ignoredDomains) {
			continue
		}
		source, err := url.ParseRequestURI(entry.SetBy)
		if err != nil || source.Scheme != "https" || source.Host == "" {
			return errors.New("invalid cookie source in session")
		}
		cookie := cloneCookie(&entry.Cookie)
		jar.SetCookies(source, []*http.Cookie{&cookie})
	}
	return nil
}
