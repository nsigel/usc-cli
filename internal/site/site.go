// Package site describes the USC systems supported by the CLI.
package site

import "fmt"

// Name is the stable command-line name of a USC site.
type Name string

const (
	Brightspace Name = "brightspace"
	Classes     Name = "classes"
	Handshake   Name = "handshake"
	WebReg      Name = "webreg"
)

// Login identifies how a site enters USC authentication. It does not imply
// that two sites share cookies or an application protocol.
type Login string

const (
	EntraOIDC      Login = "entra-oidc"
	EntraSAML      Login = "entra-saml"
	Legacy         Login = "legacy"
	ShibbolethSAML Login = "shibboleth-saml"
)

// Site is the small amount of information shared by every integration.
// Site-specific clients own everything else. LoginURL and Login are empty for
// public sites.
type Site struct {
	Name     Name   `json:"name"`
	URL      string `json:"url"`
	LoginURL string `json:"login_url,omitempty"`
	Login    Login  `json:"login,omitempty"`
}

var catalog = []Site{
	{Name: Brightspace, URL: "https://brightspace.usc.edu/", LoginURL: "https://brightspace.usc.edu/", Login: EntraSAML},
	{Name: Classes, URL: "https://classes.usc.edu/"},
	{Name: Handshake, URL: "https://usc.joinhandshake.com/", LoginURL: "https://usc.joinhandshake.com/auth/saml/70/session/new?redirect_to_idp=true", Login: ShibbolethSAML},
	{Name: WebReg, URL: "https://webreg.usc.edu/", LoginURL: "https://webreg.usc.edu/auth/login?returnUrl=%2FTerms", Login: EntraOIDC},
}

// All returns the known sites in command-line order.
func All() []Site {
	return append([]Site(nil), catalog...)
}

// Find returns the site with name.
func Find(name Name) (Site, error) {
	for _, candidate := range catalog {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return Site{}, fmt.Errorf("unknown site %q", name)
}
