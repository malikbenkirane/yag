package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install",
		Aliases: []string{"i"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			{
				if err = os.Chdir(srcDir); err != nil {
					return err
				}
				c := exec.Command("go", "install", ".")
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				err = c.Run()
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	return cmd
}
