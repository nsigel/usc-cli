package main

import (
	"encoding/json"
	"os"

	"github.com/nsigel/usc-cli/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.New(version).Execute(); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(cli.ErrorPayload(err))
		os.Exit(1)
	}
}
