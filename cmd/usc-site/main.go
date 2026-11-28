// Command usc-site serves usc.sh: the built website, its docs search, the
// sign-in page, and the relay that page uses to reach a usc process on
// another machine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nsigel/usc-cli/internal/relay"
	"github.com/nsigel/usc-cli/internal/weblogin/page"
)

func main() {
	address := flag.String("listen", ":80", "address to listen on")
	directory := flag.String("site", "/srv", "directory holding the built website")
	origin := flag.String("origin", "https://usc.sh", "public origin of the website")
	flag.Parse()

	files := os.DirFS(*directory)
	mux := http.NewServeMux()
	mux.Handle("/", site(files))
	mux.Handle("GET /.well-known/search", search(files, *origin))
	mux.Handle("/relay/", relay.Handler())
	mux.Handle(page.Path, http.StripPrefix(strings.TrimSuffix(page.Path, "/"), page.Handler()))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := &http.Server{
		Addr:              *address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(closing)
	}()
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-drained
}

// site serves the built website, answering a missing page with 404.html.
// A page with a Markdown version at index.md is served as Markdown to
// requests that prefer text/markdown over text/html.
func site(files fs.FS) http.Handler {
	served := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "."
		}
		info, err := fs.Stat(files, name)
		if err == nil && info.IsDir() {
			_, err = fs.Stat(files, path.Join(name, "index.html"))
		}
		if err == nil && info.IsDir() {
			markdown := path.Join(name, "index.md")
			if _, err := fs.Stat(files, markdown); err == nil {
				w.Header().Add("Vary", "Accept")
				links := []string{fmt.Sprintf(`<%s>; rel="alternate"; type="text/markdown"`, path.Join("/", markdown))}
				if name == "." {
					links = append(links, `</docs/>; rel="service-doc"`)
				}
				w.Header().Set("Link", strings.Join(links, ", "))
				if prefersMarkdown(r.Header.Get("Accept")) {
					w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
					http.ServeFileFS(w, r, files, markdown)
					return
				}
			}
		}
		if err != nil {
			missing, _ := fs.ReadFile(files, "404.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(missing)
			return
		}
		switch {
		case name == "install":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		case strings.HasSuffix(name, ".md"):
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		case strings.HasPrefix(name, "_astro/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		served.ServeHTTP(w, r)
	})
}

// prefersMarkdown reports whether an Accept header accepts text/markdown at
// least as much as text/html.
func prefersMarkdown(accept string) bool {
	quality := map[string]float64{}
	for _, item := range strings.Split(accept, ",") {
		mediaType, params, _ := strings.Cut(strings.TrimSpace(item), ";")
		q := 1.0
		for param := range strings.SplitSeq(params, ";") {
			if value, ok := strings.CutPrefix(strings.TrimSpace(param), "q="); ok {
				q, _ = strconv.ParseFloat(value, 64)
			}
		}
		quality[strings.ToLower(strings.TrimSpace(mediaType))] = q
	}
	return quality["text/markdown"] > 0 && quality["text/markdown"] >= quality["text/html"]
}
