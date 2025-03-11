package cmd

import "github.com/spf13/cobra"

func newTimestampCodeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "timestamp",
		Aliases: []string{"ts"},
		Short:   "make a timestamped tag for current location",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tstampFormat{}.print()
		},
	}
	return cmd
}

func newTimestampLitterateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "litt",
		Short: "litterate version of timestamp command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tstampFormat{
				litt: true,
			}.print()
		},
	}
	return cmd
}
