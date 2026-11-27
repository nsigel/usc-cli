package browser

import (
	"net/url"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	http "github.com/saucesteals/fhttp"
)

type CookieParam struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	URL      string   `json:"url,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	Path     string   `json:"path,omitempty"`
	Secure   bool     `json:"secure,omitempty"`
	HTTPOnly bool     `json:"httpOnly,omitempty"`
	Expires  *float64 `json:"expires,omitempty"`
	SameSite string   `json:"sameSite,omitempty"`
}

type CookieConversion struct {
	Cookies []CookieParam
	Skipped int
}

func CookiesToParams(stored []auth.StoredCookie) CookieConversion {
	out := make([]CookieParam, 0, len(stored))
	skipped := 0
	for _, entry := range stored {
		param, ok := cookieToParam(entry)
		if !ok {
			skipped++
			continue
		}
		out = append(out, param)
	}
	return CookieConversion{Cookies: out, Skipped: skipped}
}

func cookieToParam(entry auth.StoredCookie) (CookieParam, bool) {
	c := entry.Cookie
	if c.Name == "" || c.Value == "" {
		return CookieParam{}, false
	}

	setBy, err := url.Parse(entry.SetBy)
	if err != nil || setBy.Scheme == "" || setBy.Host == "" {
		return CookieParam{}, false
	}
	origin := setBy.Scheme + "://" + setBy.Host

	param := CookieParam{
		Name:     c.Name,
		Value:    c.Value,
		URL:      origin,
		Path:     c.Path,
		Secure:   c.Secure,
		HTTPOnly: c.HttpOnly,
	}

	hostPrefixed := strings.HasPrefix(c.Name, "__Host-")
	if hostPrefixed {

		param.Secure = true
		param.Path = "/"
	} else if domain := strings.TrimSpace(c.Domain); domain != "" {
		param.Domain = domain
	} else {

		param.Domain = setBy.Hostname()
	}

	if sameSite := sameSiteString(c.SameSite); sameSite != "" {
		param.SameSite = sameSite
		if sameSite == "None" {
			param.Secure = true
		}
	}

	if expires := cookieExpiresUnix(c); expires != nil {
		param.Expires = expires
	}

	return param, true
}

func sameSiteString(value http.SameSite) string {
	switch value {
	case 0, http.SameSiteDefaultMode:
		return ""
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteNoneMode:
		return "None"
	default:
		return ""
	}
}

func cookieExpiresUnix(c http.Cookie) *float64 {
	if c.Expires.IsZero() || c.Expires.Year() < 1601 {
		return nil
	}
	seconds := float64(c.Expires.Unix())
	return &seconds
}
