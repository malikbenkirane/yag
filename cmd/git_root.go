package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGitRootCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "root",
		Short: "git root command",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cd, err := gitRoot()
			if err != nil {
				return err
			}
			fmt.Println("root:", root)
			fmt.Println("cdir:", cd)
			return nil
		},
	}

	return cmd
}
