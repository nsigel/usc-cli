package auth

import "fmt"

type Site string

const (
	SiteAccount     Site = "account"
	SiteMobileOrder Site = "mobileorder"
	SiteBrightspace Site = "brightspace"
	SiteHandshake   Site = "handshake"
	SiteLibCal      Site = "libcal"
	SiteWebReg      Site = "webreg"
	SiteDirectory   Site = "directory"
)

var loginURLs = map[Site]string{
	SiteBrightspace: "https://brightspace.usc.edu/",
	SiteDirectory:   "https://uscdirectory.usc.edu/web/directory/student/",
	SiteHandshake:   "https://usc.joinhandshake.com/auth/saml/70/session/new?redirect_to_idp=true",

	SiteWebReg: "https://webreg.usc.edu/Terms",
}

func loginURL(name Site) (string, error) {
	target, ok := loginURLs[name]
	if !ok {
		return "", fmt.Errorf("site %q has no sign-in page", name)
	}
	return target, nil
}
