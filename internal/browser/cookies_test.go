package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/nsigel/usc-cli/internal/auth"
	http "github.com/saucesteals/fhttp"
)

func TestCookiesToParamsSynthetic(t *testing.T) {
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	stored := []auth.StoredCookie{
		{
			SetBy: "https://example.edu/path/login",
			Cookie: http.Cookie{
				Name:     "host_only",
				Value:    "secret-host",
				Path:     "/",
				Secure:   true,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Expires:  expires,
			},
		},
		{
			SetBy: "https://sso.example.edu/",
			Cookie: http.Cookie{
				Name:     "scoped",
				Value:    "secret-scoped",
				Domain:   ".example.edu",
				Path:     "/sso",
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			},
		},
		{
			SetBy: "https://app.example.edu/",
			Cookie: http.Cookie{
				Name:     "__Host-id",
				Value:    "secret-host-prefix",
				Domain:   "should-be-ignored.example.edu",
				Path:     "/ignored",
				SameSite: http.SameSiteNoneMode,
			},
		},
		{
			SetBy:  "https://example.edu/",
			Cookie: http.Cookie{Name: "", Value: "x"},
		},
		{
			SetBy:  "https://example.edu/",
			Cookie: http.Cookie{Name: "empty_value", Value: ""},
		},
		{
			SetBy: "https://example.edu/",
			Cookie: http.Cookie{
				Name:     "default_samesite",
				Value:    "keep",
				SameSite: http.SameSiteDefaultMode,
			},
		},
	}

	got := CookiesToParams(stored)
	if got.Skipped != 2 {
		t.Fatalf("Skipped = %d, want 2", got.Skipped)
	}
	if len(got.Cookies) != 4 {
		t.Fatalf("len(Cookies) = %d, want 4", len(got.Cookies))
	}

	hostOnly := got.Cookies[0]
	if hostOnly.URL != "https://example.edu" {
		t.Fatalf("host_only URL = %q", hostOnly.URL)
	}
	if hostOnly.Domain != "example.edu" {
		t.Fatalf("host_only Domain = %q, want example.edu", hostOnly.Domain)
	}
	if hostOnly.SameSite != "Lax" {
		t.Fatalf("host_only SameSite = %q", hostOnly.SameSite)
	}
	if hostOnly.Expires == nil || *hostOnly.Expires != float64(expires.Unix()) {
		t.Fatalf("host_only Expires = %v", hostOnly.Expires)
	}

	scoped := got.Cookies[1]
	if scoped.Domain != ".example.edu" {
		t.Fatalf("scoped Domain = %q", scoped.Domain)
	}
	if scoped.Path != "/sso" || scoped.SameSite != "Strict" {
		t.Fatalf("scoped Path/SameSite = %q %q", scoped.Path, scoped.SameSite)
	}

	hostPrefix := got.Cookies[2]
	if hostPrefix.Domain != "" {
		t.Fatalf("__Host- Domain = %q, want empty", hostPrefix.Domain)
	}
	if !hostPrefix.Secure || hostPrefix.Path != "/" {
		t.Fatalf("__Host- Secure/Path = %v %q", hostPrefix.Secure, hostPrefix.Path)
	}
	if hostPrefix.SameSite != "None" {
		t.Fatalf("__Host- SameSite = %q", hostPrefix.SameSite)
	}

	defaultSame := got.Cookies[3]
	if defaultSame.SameSite != "" {
		t.Fatalf("default SameSite = %q, want omitted", defaultSame.SameSite)
	}

	// Ensure test failures never dump cookie values in assertion messages above.
	for _, cookie := range got.Cookies {
		if cookie.Value == "" {
			t.Fatalf("unexpected empty value for %q", cookie.Name)
		}
		if strings.Contains(cookie.Name, "Value") {
			t.Fatal("unexpected")
		}
	}
}
