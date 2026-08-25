package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/fhttp/cookiejar"
)

const sessionVersion = 1

type storedCookie struct {
	SetBy  string      `json:"set_by"`
	Cookie http.Cookie `json:"cookie"`
}

type savedSession struct {
	Version int            `json:"version"`
	Cookies []storedCookie `json:"cookies"`
}

// sessionJar keeps the response URL that set each cookie. The standard jar
// intentionally hides that data, but it is required to restore host-only and
// path-scoped cookies correctly.
type sessionJar struct {
	inner   *cookiejar.Jar
	entries map[string]storedCookie
}

func newSessionJar() (*sessionJar, error) {
	inner, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &sessionJar{inner: inner, entries: make(map[string]storedCookie)}, nil
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
		if cookie.MaxAge < 0 {
			delete(j.entries, key)
			continue
		}
		j.entries[key] = storedCookie{
			SetBy:  cookieSource(source),
			Cookie: cloneCookie(cookie),
		}
	}
}

func (j *sessionJar) snapshot() []storedCookie {
	cookies := make([]storedCookie, 0, len(j.entries))
	for _, cookie := range j.entries {
		cookies = append(cookies, cookie)
	}
	sort.Slice(cookies, func(i, j int) bool {
		left := cookies[i].SetBy + "\x00" + cookies[i].Cookie.Name
		right := cookies[j].SetBy + "\x00" + cookies[j].Cookie.Name
		return left < right
	})
	return cookies
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

func loadSession(name string, jar *sessionJar) error {
	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var session savedSession
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("read session: %w", err)
	}
	if session.Version != sessionVersion {
		return fmt.Errorf("unsupported session version %d", session.Version)
	}
	for _, entry := range session.Cookies {
		source, err := url.ParseRequestURI(entry.SetBy)
		if err != nil || source.Scheme != "https" || source.Host == "" {
			return errors.New("invalid cookie source in session")
		}
		jar.SetCookies(source, []*http.Cookie{&entry.Cookie})
	}
	return nil
}

func saveSession(name string, jar *sessionJar) error {
	data, err := json.MarshalIndent(savedSession{Version: sessionVersion, Cookies: jar.snapshot()}, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(name)
	if directory != "." {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	temporary, err := os.CreateTemp(directory, ".session-*")
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
