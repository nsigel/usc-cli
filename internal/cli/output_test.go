package cli

import (
	"bytes"
	"testing"
)

func TestWriteJSONIsCompactWhenOutputIsNotTerminal(t *testing.T) {
	output := &bytes.Buffer{}
	app := &App{Out: output}

	if err := app.writeJSON(map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "{\"ok\":true}\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriteJSONCanForcePrettyOutput(t *testing.T) {
	output := &bytes.Buffer{}
	app := &App{Out: output, prettyJSON: true}

	if err := app.writeJSON(map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "{\n  \"ok\": true\n}\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
