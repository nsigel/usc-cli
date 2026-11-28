package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nsigel/usc-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.New()
	if err := command.ExecuteContext(ctx); err != nil {
		_ = cli.WriteError(command, err)
		os.Exit(1)
	}
}
