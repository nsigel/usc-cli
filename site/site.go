// Package site names the USC systems the clients sign in to.
package site

import "fmt"

// Name identifies a USC site in errors and sign-in calls.
type Name string

const (
	Brightspace Name = "brightspace"
	Handshake   Name = "handshake"
	LibCal      Name = "libcal"
	WebReg      Name = "webreg"
)

// loginURLs are the pages that enter USC SSO for each site. LibCal has none:
// its SSO entry only exists once a booking is staged.
var loginURLs = map[Name]string{
	Brightspace: "https://brightspace.usc.edu/",
	Handshake:   "https://usc.joinhandshake.com/auth/saml/70/session/new?redirect_to_idp=true",
	// WebReg's /auth/login forces a new OIDC round trip on every visit, and
	// Microsoft rejects rapid repeats as a request loop. An app page reaches
	// Microsoft only when the WebReg session has expired.
	WebReg: "https://webreg.usc.edu/Terms",
}

// LoginURL returns the page that signs in to a site.
func LoginURL(name Name) (string, error) {
	target, ok := loginURLs[name]
	if !ok {
		return "", fmt.Errorf("site %q has no sign-in page", name)
	}
	return target, nil
}
