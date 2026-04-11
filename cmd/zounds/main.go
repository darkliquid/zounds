package main

import (
	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func main() {
	cobra.CheckErr(commands.NewRootCommand().Execute())
}
