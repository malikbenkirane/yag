package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newTestSubCommand(class string, isStaged bool, isClass func(fstat) bool) *cobra.Command {
	staged := "unstaged"
	if isStaged {
		staged = "staged"
	}
	name := fmt.Sprintf("list_%s_%s", class, staged)
	return &cobra.Command{
		Use: name,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := stat()
			if err != nil {
				return fmt.Errorf("stat(): %w", err)
			}
			for _, s := range s {
				if isStaged && s.isStaged() && isClass(s) {
					fmt.Println(s.path)
				}
				if !isStaged && (!s.isStaged() || s.staged == '?') && isClass(s) {
					fmt.Println(s.path)
				}
			}
			return nil
		},
	}
}

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	listChangedStaged := newTestSubCommand("changed", true, func(f fstat) bool { return f.modified() })
	listUntrackedStaged := newTestSubCommand("untracked", true, func(f fstat) bool { return f.untrackedNewFile() })
	listChangedUnstaged := newTestSubCommand("changed", false, func(f fstat) bool { return f.modified() })
	listUntrackedUnstaged := newTestSubCommand("untracked", false, func(f fstat) bool { return f.untracked() })
	cmd.AddCommand(
		listChangedStaged,
		listUntrackedStaged,
		listChangedUnstaged,
		listUntrackedUnstaged,
	)
	return cmd
}
