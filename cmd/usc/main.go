package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nsigel/usc-cli/internal/cli"
)

func main() {
	command, err := cli.New()
	if err == nil {
		err = command.Execute()
	}
	if err != nil {
		if encodeErr := json.NewEncoder(os.Stderr).Encode(map[string]string{"error": err.Error()}); encodeErr != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
