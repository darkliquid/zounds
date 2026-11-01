package main

import (
	"context"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), interruptSignals()...)
	defer stop()
	cobra.CheckErr(commands.NewRootCommand().ExecuteContext(ctx))
}
