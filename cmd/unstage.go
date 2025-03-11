package cmd

import "github.com/spf13/cobra"

func newUnstageCommand(git func(args ...string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unstage [file]...",
		Short: "git restore --staged <file>...",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return git(append([]string{"restore", "--staged"}, args...)...)
		},
	}
	return cmd
}
