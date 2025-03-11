package cmd

import (
	"bytes"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newTagCommand(git func(args ...string) error, out io.Writer) *cobra.Command {
	var tagRemoteOpt *string
	cmd := &cobra.Command{ //
		Use:   "tag",
		Short: "tag and push with last commit tag title",
		RunE: func(cmd *cobra.Command, args []string) error {
			var logOut bytes.Buffer
			if err := (gitCli{
				infoOut: out,
				cmdOut:  io.MultiWriter(&logOut),
			}.run("log", "-1", "--oneline")); err != nil {
				return err
			}
			logParts := strings.Split(logOut.String(), " ")
			tag := strings.TrimSpace(logParts[len(logParts)-1])
			if err := git("tag", tag); err != nil {
				return err
			}
			if err := git("push", *tagRemoteOpt, tag); err != nil {
				return err
			}
			return nil
		},
	}
	tagRemoteOpt = cmd.Flags().String("remote", "github", "git remote parameter")
	return cmd
}
