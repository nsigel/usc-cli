package auth

import (
	"net/url"
	"testing"

	http "github.com/saucesteals/fhttp"
)

func TestSessionJarRecordsAndReplaysScopedCookies(t *testing.T) {
	jar, err := newSessionJar()
	if err != nil {
		t.Fatal(err)
	}
	loginURL, err := url.Parse("https://login.usc.edu/login/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(loginURL, []*http.Cookie{{Name: "sso", Value: "first", Secure: true, HttpOnly: true}})
	updatedURL, err := url.Parse("https://login.usc.edu/login/continue")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(updatedURL, []*http.Cookie{{Name: "sso", Value: "second", Secure: true, HttpOnly: true}})

	snapshot := jar.snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("recorded %d cookies, want 1", len(snapshot))
	}
	if snapshot[0].Cookie.Value != "second" {
		t.Fatalf("recorded value %q, want updated value", snapshot[0].Cookie.Value)
	}

	restored, err := newSessionJar()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range snapshot {
		source, err := url.Parse(entry.SetBy)
		if err != nil {
			t.Fatal(err)
		}
		restored.SetCookies(source, []*http.Cookie{&entry.Cookie})
	}
	matchingURL, err := url.Parse("https://login.usc.edu/login/next")
	if err != nil {
		t.Fatal(err)
	}
	if cookies := restored.Cookies(matchingURL); len(cookies) != 1 || cookies[0].Value != "second" {
		t.Fatalf("restored matching cookies = %#v, want updated cookie", cookies)
	}
	nonMatchingURL, err := url.Parse("https://login.usc.edu/other")
	if err != nil {
		t.Fatal(err)
	}
	if cookies := restored.Cookies(nonMatchingURL); len(cookies) != 0 {
		t.Fatalf("restored non-matching cookies = %#v, want none", cookies)
	}
}
