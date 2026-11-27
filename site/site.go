package site

import "fmt"

type Name string

const (
	Account     Name = "account"
	MobileOrder Name = "mobileorder"
	Brightspace Name = "brightspace"
	Handshake   Name = "handshake"
	LibCal      Name = "libcal"
	WebReg      Name = "webreg"
	Directory   Name = "directory"
)

var loginURLs = map[Name]string{
	Brightspace: "https://brightspace.usc.edu/",
	Directory:   "https://uscdirectory.usc.edu/web/directory/student/",
	Handshake:   "https://usc.joinhandshake.com/auth/saml/70/session/new?redirect_to_idp=true",

	WebReg: "https://webreg.usc.edu/Terms",
}

func LoginURL(name Name) (string, error) {
	target, ok := loginURLs[name]
	if !ok {
		return "", fmt.Errorf("site %q has no sign-in page", name)
	}
	return target, nil
}
