package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nsigel/usc-cli/internal/cli"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.New(version)
	if err := command.ExecuteContext(ctx); err != nil {
		_ = cli.WriteError(command, err)
		os.Exit(1)
	}
}
