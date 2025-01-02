package main

import (
	"context"

	"github.com/lafourgale/fx/yag/cmd"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(cmd.NewCLI().ExecuteContext(context.Background()))
}
