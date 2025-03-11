package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newSkimCommand() *cobra.Command {

	var listUntrackedOpt *bool

	cmd := &cobra.Command{
		Use:   "sk",
		Short: "skim through git status",
		RunE: func(cmd *cobra.Command, args []string) error {
			var buf bytes.Buffer
		INSTR_LOOP:
			for {
				buf.Reset()
				status, err := stat()
				if err != nil {
					return fmt.Errorf("yag_stat: %w", err)
				}
				for _, fs := range status {
					if fs.modified() || (*listUntrackedOpt && fs.untracked()) {
						buf.WriteString(fs.path + "\n")
					}
				}
				buf.WriteString("help\ndone\ntag-last-commit\nclaude-commit\nclaude-commit-llamax\n")
				c := exec.Command("sk")
				c.Stdin = &buf
				var outBuf bytes.Buffer
				c.Stdout = &outBuf
				c.Stderr = os.Stderr
				err = c.Run()
				if err != nil {
					if exiterr, ok := err.(*exec.ExitError); ok {
						switch exiterr.ExitCode() {
						case 1:
							// no match
							fmt.Println("exit no match")
							return nil
						case 2:
							// error
							return err
						case 130:
							// interrupted (C-C or ESC)
							fmt.Println("exit interrupted")
							return nil
						}
					}
				}

				switch skInstr := strings.TrimSpace(outBuf.String()); skInstr {
				case "claude-commit-llamax":
					x := exec.Command("yag", "claude", "commit")
					x.Stdout = os.Stdout
					x.Stderr = os.Stderr
					x.Stdin = os.Stdin
					if err = x.Run(); err != nil {
						return fmt.Errorf("yag claude commit: %w", err)
					}
				case "claude-commit":
					x := exec.Command("yag", "claude", "commit", "--no-llama")
					x.Stdout = os.Stdout
					x.Stderr = os.Stderr
					x.Stdin = os.Stdin
					if err = x.Run(); err != nil {
						return fmt.Errorf("yag claude commit: %w", err)
					}
				case "help":
					fmt.Println("Nothing yet to help you here")
					scanner := bufio.NewScanner(os.Stdin)
					_ = scanner.Scan()
					if scanner.Err() != nil {
						return fmt.Errorf("scanner: %w", err)
					}
				case "done":
					break INSTR_LOOP
				case "tag-last-commit":
					x := exec.Command("yag", "tag")
					x.Stdout = os.Stdout
					x.Stderr = os.Stderr
					x.Stdin = os.Stdin
					if err = x.Run(); err != nil {
						return fmt.Errorf("yag claude commit: %w", err)
					}

				default:
					c = exec.Command("git", "add", skInstr)
					c.Stdout = os.Stdout
					c.Stderr = os.Stderr
					err = c.Run()
					if err != nil {
						return err
					}
				}
			}
			return nil
		},
	}

	listUntrackedOpt = cmd.Flags().Bool("list-untracked", false, "include untracked files in skim list")

	return cmd

}
