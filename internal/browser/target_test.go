package browser

import "testing"

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults", input: "", want: "http://127.0.0.1:9222"},
		{name: "port only", input: "9224", want: "http://127.0.0.1:9224"},
		{name: "host port", input: "127.0.0.1:9224", want: "http://127.0.0.1:9224"},
		{name: "http url", input: "http://127.0.0.1:9224", want: "http://127.0.0.1:9224"},
		{name: "http url trailing slash", input: "http://127.0.0.1:9224/", want: "http://127.0.0.1:9224"},
		{name: "websocket url", input: "ws://127.0.0.1:9224/devtools/browser/abc", want: "ws://127.0.0.1:9224/devtools/browser/abc"},
		{name: "wss url", input: "wss://example.com/devtools/browser/abc", want: "wss://example.com/devtools/browser/abc"},
		{name: "invalid port", input: "99999", wantErr: true},
		{name: "garbage", input: "not-a-target", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveTarget(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("ResolveTarget(%q) error = nil, want error", test.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTarget(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("ResolveTarget(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestDefaultTarget(t *testing.T) {
	if got := DefaultTarget(); got != "127.0.0.1:9222" {
		t.Fatalf("DefaultTarget() = %q, want 127.0.0.1:9222", got)
	}
}
