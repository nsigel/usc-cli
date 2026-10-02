// Package skill exposes agent skills bundled with the usc executable.
package skill

import (
	_ "embed"
	"io"
)

//go:embed usc-cli/SKILL.md
var usc string

// WriteUSC writes the bundled usc-cli skill to writer.
func WriteUSC(writer io.Writer) error {
	_, err := io.WriteString(writer, usc)
	return err
}
