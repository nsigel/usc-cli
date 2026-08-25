package main

import (
	"os"

	"github.com/nsigel/usc-cli/internal/cli"
)

var version = "dev"

func main() {
	command := cli.New(version)
	if err := command.Execute(); err != nil {
		_ = cli.WriteError(command, err)
		os.Exit(1)
	}
}
