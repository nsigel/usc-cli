package skill

import (
	_ "embed"
	"io"
)

//go:embed usc-cli/SKILL.md
var usc string

func WriteUSC(writer io.Writer) error {
	_, err := io.WriteString(writer, usc)
	return err
}
