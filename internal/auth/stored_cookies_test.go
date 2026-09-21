package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStoredCookiesSynthetic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	payload := []byte(`{
  "version": 1,
  "cookies": [
    {
      "set_by": "https://example.edu/",
      "cookie": {
        "Name": "demo",
        "Value": "synthetic-only",
        "Path": "/",
        "Domain": "",
        "Expires": "0001-01-01T00:00:00Z",
        "RawExpires": "",
        "MaxAge": 0,
        "Secure": true,
        "HttpOnly": true,
        "SameSite": 2,
        "Raw": "",
        "Unparsed": null
      }
    }
  ]
}
`)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	cookies, err := LoadStoredCookies(path)
	if err != nil {
		t.Fatalf("LoadStoredCookies: %v", err)
	}
	if len(cookies) != 1 {
		t.Fatalf("len = %d", len(cookies))
	}
	if cookies[0].Cookie.Name != "demo" {
		t.Fatalf("name = %q", cookies[0].Cookie.Name)
	}

	_, err = LoadStoredCookies(filepath.Join(dir, "missing.json"))
	if err != ErrSessionNotFound {
		t.Fatalf("missing error = %v, want ErrSessionNotFound", err)
	}
}
