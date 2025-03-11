package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newOllamaCommitCommand(out io.Writer) *cobra.Command {
	var commitDryOpt *bool
	cmd := &cobra.Command{ // very experimental proposal 😇
		Use:   "commit",
		Short: "ollama-commit then commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			{
				status, err := stat()
				if err != nil {
					return fmt.Errorf("yagstat: %w", err)
				}
				var coll [][]string
				for _, fs := range status {
					if fs.isStaged() {
						coll = append(coll, strings.Split(fs.path, string(os.PathSeparator)))
					}
				}
				p := make(map[string]string)
				for _, path := range coll {
					for i := 1; i < len(path); i++ {
						p[path[i]] = path[i-1]
					}
				}
				fmt.Println(p)
			}

			// DEPTODO requires PATH setup for ollama-commit, ts and vim

			var buf bytes.Buffer
			w := io.MultiWriter(out, &buf)
			runCommit := func() error {
				commit := exec.Command("ollama-commit")
				commit.Stdout = w
				commit.Stderr = w
				if err := commit.Run(); err != nil {
					return err
				}
				return nil
			}
			var err error
			if !*commitDryOpt {
				if err = runCommit(); err != nil {
					return err
				}
			}
			tsOut, err := exec.Command("yag", "timestamp").Output()
			if err != nil {
				return err
			}
			if *commitDryOpt {
				fmt.Println("tag:", tsOut)
				// DEPFIXME only mimics the behavior of ollama-commit default diff
				//
				// Interoperability with it ollama-commit is also limit and is being a problem
				// Considering to opt for a server/client architecture for instance.
				var buf bytes.Buffer
				cmd := exec.Command("git", "diff", "--cached")
				cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
				cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
				if err = cmd.Run(); err != nil {
					return fmt.Errorf("run git diff command: %w", err)
				}
				cmd = exec.Command("pbcopy")
				cmd.Stdin = strings.NewReader(buf.String())
				if err = cmd.Run(); err != nil {
					return fmt.Errorf("copy to os clipboard: pbpaste: %w", err)
				}
				fmt.Println("📋 pasted into the os clipboard")
				return nil
			}
			const stashName = ".commit-stash"
			stash, err := os.Create(stashName)
			if err != nil {
				return err
			}
			_, err = stash.Write(tsOut)
			if err != nil {
				return err
			}
			_, err = stash.WriteString("\n\n")
			if err != nil {
				return err
			}
			_, err = stash.Write(buf.Bytes())
			if err != nil {
				return err
			}
			err = stash.Close()
			if err != nil {
				return err
			}
			edit := exec.Command("vim", stashName)
			edit.Stdin = os.Stdin
			edit.Stdout = os.Stdout
			edit.Stderr = os.Stderr
			err = edit.Run()
			if err != nil {
				return err
			}
			stash, err = os.Open(stashName)
			if err != nil {
				return err
			}
			var stashCpy bytes.Buffer

			{
				t := io.TeeReader(stash, &stashCpy)
				scan := bufio.NewScanner(t)
				fmt.Println()
				fmt.Println()
				fmt.Println("[EDIT]")
				fmt.Println()
				for scan.Scan() {
					fmt.Println(scan.Text())
				}
			}

			err = stash.Close()
			if err != nil {
				return err
			}

			return gitCli{
				infoOut: out,
				in:      &stashCpy,
			}.run("commit", "-F", "-")
		},
	}
	commitDryOpt = cmd.Flags().Bool("dry", false, "disable generation of commit message")
	return cmd
}
