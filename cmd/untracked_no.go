package cmd

import "github.com/spf13/cobra"

func newUntrackedNoCommand(git func(args ...string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uno",
		Short: "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return git("status", "-uno", ".")
		},
	}
	return cmd
}
