package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newOnlyUntrackedFilesCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "u",
		Short: "only 🎶 untracked files",
		RunE: func(cmd *cobra.Command, args []string) error {
			var b bytes.Buffer
			gitRun := gitCli{
				infoOut: out,
				cmdOut:  &b,
			}.run
			if err := gitRun("status", "--short", "--show-stash"); err != nil {
				return err
			}
			scanner := bufio.NewScanner(&b)
			var untracked []string
			for scanner.Scan() {
				line := scanner.Text()
				if cut, found := strings.CutPrefix(line, "?? "); found {
					untracked = append(untracked, cut)
				}
			}
			var err error
			if err = scanner.Err(); err != nil {
				return fmt.Errorf("scanner error: %w", err)
			}
			isEven := false
			sort.Strings(untracked)
			for _, cut := range untracked {
				root, cd, err := gitRoot()
				if err != nil {
					return err
				}
				cut = filepath.Join(cd, cut)
				cut, found := strings.CutPrefix(cut, root)
				if !found {
					return fmt.Errorf("%q not found prefix=%q", cut, root)
				}
				if isEven {
					printUtil{out: out, cut: cut}.seq(printYellow, printYellow, cut, printReset)
				} else {
					printUtil{out: out, cut: cut}.greenOnBlack()
				}
				isEven = !isEven
			}
			return nil
		},
	}
	return cmd
}
