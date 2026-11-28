// Package page holds the sign-in page, which the CLI serves on loopback and
// usc.sh serves for a sign-in on another machine.
package page

import (
	"embed"
	"net/http"
)

//go:embed *.html *.js *.css
var files embed.FS

// Path is where a site serves the page for a relayed sign-in. A page that
// stops understanding older CLIs gets a new path beside this one.
const Path = "/login/v1/"

func Handler() http.Handler {
	served := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Secure(w.Header())
		served.ServeHTTP(w, r)
	})
}

// Secure sets the headers every response around the page carries.
func Secure(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
}
