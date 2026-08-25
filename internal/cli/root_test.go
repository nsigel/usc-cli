package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestJSONFormattingFlags(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		pretty bool
	}{
		{name: "piped output is compact", args: []string{"sites", "webreg"}},
		{name: "json forces compact output", args: []string{"--json", "sites", "webreg"}},
		{name: "pretty forces indented output", args: []string{"--pretty", "sites", "webreg"}, pretty: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := New("test")
			var stdout bytes.Buffer
			command.SetOut(&stdout)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			got := stdout.String()
			if test.pretty != strings.Contains(got, "\n  ") {
				t.Fatalf("output = %q, pretty = %t", got, test.pretty)
			}
		})
	}
}

func TestWriteErrorUsesOutputFormat(t *testing.T) {
	command := New("test")
	if err := command.PersistentFlags().Set("pretty", "true"); err != nil {
		t.Fatalf("set pretty flag: %v", err)
	}

	var stderr bytes.Buffer
	command.SetErr(&stderr)
	if err := WriteError(command, errors.New("no session")); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "\n  ") {
		t.Fatalf("error output = %q, want pretty JSON", stderr.String())
	}
}
