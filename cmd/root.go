package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lafourgale/fx/yag/internal/help"
	ollama "github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var srcDir = os.Getenv("YAG_SRCDIR")

func NewCLI() *cobra.Command {
	if srcDir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			panic(err)

		}
		srcDir = filepath.Join(h, "i", "fx", "yag")
	}
	out := os.Stdout
	git := gitCli{
		infoOut: os.Stdout,
	}.run
	rootCmd := &cobra.Command{
		Use:   "yag -- [file]*",
		Short: "Yet Another [Git]",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) >= 1 {
				// Check if args are existing files names.
				for _, fname := range args {
					info, err := os.Stat(fname)
					if os.IsNotExist(err) {
						return fmt.Errorf("%q does not exist.", fname)
					}
					if info.IsDir() {
						entries, err := os.ReadDir(fname)
						if err != nil {
							return err
						}
						fmt.Fprintf(out, "%q is a directory, please cherry-pick or use (-d or --directory)\n\n", fname)
						// Loop throug the directory entries for further review
						for _, e := range entries {
							var note = "  "
							if e.IsDir() {
								note = "D "
							}
							fmt.Fprintf(out, "\033[33m%s\033[0m \033[32m%q\033[0m\n", note, filepath.Join(fname, e.Name()))
						}
						fmt.Fprintln(out)
						return git("status", fname)
					}
				}
				// Prepare git command args.
				gitArgs := append([]string{"add"}, args...)
				return git(gitArgs...)
			}
			stats, err := stat()
			if err != nil {
				return fmt.Errorf("git stat(): %w", err)
			}
			{
				u := make([]fstat, 0, len(stats))
				m := make([]fstat, 0, len(stats))
				for _, f := range stats {
					switch {
					case f.untrackedNewFile():
						u = append(u, f)
					case f.modified():
						m = append(m, f)
					}
				}
				if len(u) > 0 {
					fmt.Println()
					fmt.Print("💣 ")
					printUtil{
						out:       out,
						cut:       "staging untracked",
						noNewLine: true,
					}.yellowOnBlack()
					fmt.Println(" files:")
					for _, f := range u {
						fmt.Println(f.path)
					}
				}
				if len(m) > 0 {
					fmt.Println()
					fmt.Print("🧨 unstaged ")
					printUtil{
						out:       out,
						cut:       "modified",
						noNewLine: true,
					}.greenOnBlack()
					fmt.Println(" files:")
					fmt.Println()
					for _, f := range m {
						printUtil{out: out, cut: f.path}.greenOnBlack()
					}

					fmt.Println("\n💥💥💥💥💥")
					xc := exec.Command("git", "status", "-uno")
					xc.Stdout = os.Stdout
					xc.Stderr = os.Stderr
					if err = xc.Run(); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	unstageCmd := &cobra.Command{
		Use:   "unstage [file]...",
		Short: "git restore --staged <file>...",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return git(append([]string{"restore", "--staged"}, args...)...)
		},
	}
	uCmd := &cobra.Command{
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
	tsCmd := &cobra.Command{
		Use:     "timestamp",
		Aliases: []string{"ts"},
		Short:   "make a timestamped tag for current location",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tstampFormat{}.print()
		},
	}
	tsLittCmd := &cobra.Command{
		Use:   "litt",
		Short: "litterate version of timestamp command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tstampFormat{
				litt: true,
			}.print()
		},
	}
	tfCmd := &cobra.Command{
		Use:   "tf",
		Short: "terraform template commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	rootCmd.AddCommand(tfCmd)
	var (
		githubTfOptVisibilityIsPrivate *bool
		githubTfOptName                *string
		githubTfOptDescription         *string
		githubTfOptOwner               *string
	)
	tfGithubCmd := &cobra.Command{
		Use:   "github",
		Short: "populate files for defining github tf resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			mainFilename := "main.tf"
			// First check that all files to populate are not already there
			if help.NewStat(mainFilename).IsExist() {
				return fmt.Errorf("main.tf already exists")
			}
			githubModuleDir := filepath.Join("modules", "github")
			if help.NewStat(githubModuleDir).IsDir() {
				return fmt.Errorf("modules/github already exists")
			}
			githubModuleFile := filepath.Join(githubModuleDir, "repo.tf")
			if help.NewStat(githubModuleFile).IsExist() {
				return fmt.Errorf("modules/github/repo.tf already exists")
			}
			const repoTf = `
resource "github_repository" "%[1]s" {
  name        = "%[1]s"
  description = "%[2]s"
  visibility  = "%[3]s"
}

output "url" {
  value = github_repository.yag.ssh_clone_url
}`
			const mainTf = `
terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "6.2.2"
    }
  }
}

provider "github" {
  owner = "%s"
}

module "repo" {
  source = "./modules/github"
}

output "repo_url" {
  value = module.repo.url
}`
			if err = os.MkdirAll(githubModuleDir, 0600); err != nil {
				return fmt.Errorf("unable to create %q: %w", githubModuleDir, err)
			}
			var f *os.File
			if f, err = os.Create(githubModuleFile); err != nil {
				return fmt.Errorf("unable to create %q: %w", githubModuleFile, err)
			}
			var (
				visibility  = "public"
				description = *githubTfOptDescription
				name        = *githubTfOptName
			)
			if *githubTfOptVisibilityIsPrivate {
				visibility = "private"
			}
			if *githubTfOptOwner == "" || *githubTfOptName == "" {
				return fmt.Errorf("must provide non-zero --name and --owner flags")
			}
			if _, err = fmt.Fprintf(f, repoTf, name, description, visibility); err != nil {
				return fmt.Errorf("unable to write %q: %w", githubModuleFile, err)
			}
			if err = f.Close(); err != nil {
				return fmt.Errorf("unable to close %q: %w", f.Name(), err)
			}
			if f, err = os.Create(mainFilename); err != nil {
				return fmt.Errorf("create %q: %w", mainFilename, err)
			}
			if _, err = fmt.Fprintf(f, mainTf, *githubTfOptOwner); err != nil {
				return fmt.Errorf("unable to populate main.tf: %w", err)
			}
			if err = f.Close(); err != nil {
				return fmt.Errorf("unable to close %q: %w", f.Name(), err)
			}
			return nil
		},
	}
	githubTfOptVisibilityIsPrivate = tfGithubCmd.Flags().Bool("private", false, "set private visibility for the repository")
	githubTfOptDescription = tfGithubCmd.Flags().String("description", "", "repository description")
	githubTfOptName = tfGithubCmd.Flags().String("name", "", "repository name (flag is required)")
	githubTfOptOwner = tfGithubCmd.Flags().String("owner", "", "repository owner (flag is required)")
	tfCmd.AddCommand(tfGithubCmd)
	installCmd := &cobra.Command{
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
	var commitDryOpt *bool
	commitCmd := &cobra.Command{ // very experimental proposal 😇
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
	commitDryOpt = commitCmd.Flags().Bool("dry", false, "disable generation of commit message")
	var tagRemoteOpt *string
	tagCmd := &cobra.Command{ //
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
	tagRemoteOpt = tagCmd.Flags().String("remote", "github", "git remote parameter")
	unoCmd := &cobra.Command{
		Use:   "uno",
		Short: "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return git("status", "-uno", ".")
		},
	}
	var listUntrackedOpt *bool
	skCmd := &cobra.Command{
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
	listUntrackedOpt = skCmd.Flags().Bool("list-untracked", false, "include untracked files in skim list")
	claudeCmd := &cobra.Command{
		Use:   "claude",
		Short: "invoke claude",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	var noCommitOpt, clearOpt, noLlamaOpt *bool
	claudeCommitCmd := &cobra.Command{
		Use:   "commit",
		Short: "ask claude for a good commit message (vertexai)",
		RunE: func(cmd *cobra.Command, args []string) error {

			debug, err := zap.NewDevelopment()
			if err != nil {
				return err
			}

			debug = debug.Named("claude_commit")

			debug.Debug("--no-commit flag valued", zap.Bool("no-commit", *noCommitOpt))
			debug.Debug("--no-llama flag valued", zap.Bool("no-llaama", *noLlamaOpt))

			var token string
			{
				out, err := exec.Command("gcloud", "auth", "print-access-token").Output()
				if err != nil {
					return err
				}
				token = string(out[:len(out)-1])
				debug.Debug("googlcloud aiplatform token retrieved")
			}

			projectId := "upbeat-task-298823"
			model := "claude-3-5-sonnet-v2@20241022"
			location := "europe-west1"
			cli := http.DefaultClient
			var url = fmt.Sprintf("https://%[2]s-aiplatform.googleapis.com/v1/projects/%[3]s/locations/%[2]s/publishers/anthropic/models/%[1]s:streamRawPredict", model, location, projectId)
			fmt.Println("POST", url)
			var msg claudeMsgContent
			{
				var diff string
				{
					comb, err := exec.Command("git", "diff", "--cached").CombinedOutput()
					if err != nil {
						return err
					}
					diff = string(comb)
					debug.Debug("diff --cached pass", zap.String("diff", diff), zap.Int("len(diff)", len(diff)))
					debug.Info("hey")
					if len(diff) == 0 {
						fmt.Println("🤔 nothing to commit")
						return nil
					}
				}
				msg = newClaudeMsgTxt(fmt.Sprintf("Provide a good commit message for the following diff:\n```diff\n%s\n```\n", diff))
			}

			payload := struct {
				Version   string      `json:"anthropic_version"`
				Messges   []claudeMsg `json:"messages"`
				Stream    bool        `json:"stream"`
				MaxTokens int         `json:"max_tokens"`
			}{
				Version: "vertex-2023-10-16",
				Messges: []claudeMsg{
					{
						Role:    "user",
						Content: []claudeMsgContent{msg},
					},
				},
				MaxTokens: 256,
				Stream:    false,
			}
			var buf bytes.Buffer
			w := io.MultiWriter(os.Stderr, &buf)
			fmt.Println("payload:")
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				return err
			}
			fmt.Println()
			req, err := http.NewRequest(http.MethodPost, url, &buf)
			if err != nil {
				return err
			}
			debug.Info("new request for vertexai api",
				zap.String("anthropic_version", payload.Version),
				zap.String("msg", msg.String()),
				zap.Int("max_tokens", payload.MaxTokens),
				zap.String("model", model),
				zap.String("location", location),
				zap.String("project_id", projectId),
			)
			req.Header.Add("Authorization", "Bearer "+token)
			// sensitive: fmt.Println("auth", req.Header.Get("Authorization"))
			req.Header.Add("Content-Type", "application/json; charset=utf-8")
			res, err := cli.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			// fmt.Println(res.Status)
			var out bytes.Buffer
			{
				var tmp bytes.Buffer
				_, err = io.Copy(io.MultiWriter(&tmp, &out), res.Body)
				if err != nil {
					return err
				}
				debug.Debug("Vertex AI API response returned",
					zap.String("status", res.Status), zap.String("response", tmp.String()))
			}
			var finalCommit bytes.Buffer
			{
				fmt.Println()
				fmt.Println("out", out.String())
				valid := strings.ToValidUTF8(out.String(), "?")
				r := strings.NewReader(valid)
				fmt.Println(valid)
				var commitMsgBody string
				{
					var buf bytes.Buffer
					xc := exec.Command("jq", "-r", ".content[0].text")
					xc.Stdin = r
					xc.Stdout = &buf
					xc.Stderr = &buf
					if err = xc.Run(); err != nil {
						return err
					}
					commitMsgBody = buf.String()
					debug.Debug("claude response extracted", zap.String("response", commitMsgBody))

					if *noLlamaOpt {
						debug.Debug("skipping commitMsgBody extraction with ollama3.2")
					} else {
						client, err := ollama.ClientFromEnvironment()
						if err != nil {
							return err
						}
						messages := []ollama.Message{
							{
								Role: "system",
								Content: `You will extract with no editing from
								the given paragraph the commit message.

								We need to keep a good level of details and to
								stay technical. Bullet points and syntetic
								process are encouraged but the level of details
								must match or increase what was initially
								provided.

								We absolutely need the commit message to be passed
								to [git commit] command cli as if passed with
								[-f] or [-m] with no extra characters`,
							},
							{
								Role:    "user",
								Content: commitMsgBody,
							},
						}
						ctx := cmd.Context()
						req := &ollama.ChatRequest{
							Model:    "llama3.2:3b",
							Messages: messages,
						}

						var buf bytes.Buffer
						respFunc := func(resp ollama.ChatResponse) error {
							fmt.Fprint(&buf, resp.Message.Content)
							return nil
						}

						debug.Debug("starting llama chat")

						if err = client.Chat(ctx, req, respFunc); err != nil {
							return err
						}

						clearAndDisplay := func(buf *bytes.Buffer) (string, error) {
							if *clearOpt {
								if err = func() error {
									// Try ANSI first
									if _, err := fmt.Fprint(os.Stdout, "\033[H\033[2J"); err == nil {
										_, err := buf.WriteTo(os.Stdout)
										return err
									}

									// Fallback to OS specific clear
									var cmd *exec.Cmd
									if runtime.GOOS == "windows" {
										cmd = exec.Command("cmd", "/c", "cls")
									} else {

										cmd = exec.Command("clear")
									}

									cmd.Stdout = os.Stdout
									if err := cmd.Run(); err != nil {
										return err
									}
									return nil
								}(); err != nil {
									return "", err
								}
							} else {
								debug.Debug("clear opt disabled")
							}

							var b bytes.Buffer
							_, err := buf.WriteTo(io.MultiWriter(os.Stdout, &b))
							return b.String(), err
						}

						commitMsgBody, err = clearAndDisplay(&buf)
						if err != nil {
							return err
						}

					}

				}
				debug.Debug("yag timestamp")
				ts, err := exec.Command("yag", "timestamp").CombinedOutput()
				if err != nil {
					return err
				}
				debug.Debug("create commit-stash")
				f, err := os.Create(".commit-stash")
				if err != nil {
					return err
				}
				debug.Info(".commit-stash opened")
				defer func() {
					err = f.Close()
					if err != nil {
						debug.Error("unable to close .commit-stash", zap.Error(err))
					}
				}()
				w := io.MultiWriter(os.Stderr, f, &finalCommit)
				fmt.Fprintln(w, string(ts))
				fmt.Fprintln(w, commitMsgBody)
				debug.Debug("write final commit", zap.String("body", commitMsgBody), zap.String("tag", string(ts)))
			}
			if *noCommitOpt {
				red("\n\nnothing to commit\n")
				xc := exec.Command("pbcopy")
				xc.Stdout = os.Stdout
				xc.Stdin = strings.NewReader(finalCommit.String())
				xc.Stderr = os.Stderr
				debug.Debug("copy to pastebin", zap.String("final_commit_msg", finalCommit.String()))
				if err = xc.Run(); err != nil {
					return fmt.Errorf("unable to pbcopy: %w", err)
				}
				return nil
			}
			{
				cmd := exec.Command("git", "commit", "--file", ".commit-stash")
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				if err = cmd.Run(); err != nil {
					return err
				}
			}
			{
				os.Setenv("EDITOR", "vi")
				cmd := exec.Command("git", "commit", "--verbose", "--amend", "--allow-empty", "--allow-empty-message")
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stderr
				if err = cmd.Run(); err != nil {
					return err
				}
			}
			return nil
		},
	}
	noLlamaOpt = claudeCommitCmd.Flags().Bool("no-llama", true, "disable ollama llama3.2 commit message extraction")
	noCommitOpt = claudeCommitCmd.Flags().Bool("no-commit", false, "disable git commit ultimate step")
	clearOpt = claudeCommitCmd.Flags().Bool("clear", false, "clear screen")
	rootCmd.AddCommand(unstageCmd, commitCmd, tagCmd, unoCmd, tsCmd, installCmd, skCmd, claudeCmd, uCmd)
	claudeCmd.AddCommand(claudeCommitCmd)
	tsCmd.AddCommand(tsLittCmd)
	yagRootCmd := &cobra.Command{
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
	rootCmd.AddCommand(yagRootCmd)
	return rootCmd
}

type fstat struct {
	staged   byte
	unstaged byte
	path     string
}

func (fs fstat) untrackedNewFile() bool {
	return fs.staged == 'A' && fs.unstaged == ' '
}

func (fs fstat) modified() bool {
	return fs.unstaged == 'M'
}

func (fs fstat) untracked() bool {
	return fs.unstaged == '?' && fs.staged == '?'
}

func (fs fstat) isStaged() bool {
	return fs.staged != ' '
}

type gitCli struct {
	infoOut io.Writer
	cmdOut  io.Writer
	in      io.Reader
}

func (gc gitCli) run(args ...string) error {
	if len(args) == 0 {
		fmt.Fprintln(gc.infoOut, "We are looking for changes in current directory:")
		fmt.Fprintln(gc.infoOut)
		args = []string{"status", "-uno", "."}
	}
	if gc.in == nil {
		gc.in = os.Stdin
	}
	if gc.cmdOut == nil {
		gc.cmdOut = os.Stdout
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = gc.cmdOut
	cmd.Stdin = gc.in
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func stat() ([]fstat, error) {
	c := exec.Command("git", "status", "-s")
	c.Stderr = os.Stderr
	var buf bytes.Buffer
	c.Stdout = &buf
	err := c.Run()
	if err != nil {
		return nil, err
	}
	var status []fstat
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		t := scanner.Text()
		status = append(status, fstat{
			staged:   t[0],
			unstaged: t[1],
			path:     t[3:],
		})
	}
	return status, nil
}

var gitRoot = func() (gitroot, cd string, err error) {
	cd, err = os.Getwd()
	if err != nil {
		return
	}
	gitroot = cd
	{
		for {
			_, err = os.Stat(filepath.Join(gitroot, ".git"))
			if os.IsNotExist(err) {
				gitroot = filepath.Join(gitroot, "..")
				continue
			}
			break
		}
	}
	return
}

func timestamp(litt bool) error {
	tsfmt := "200601021504.05"
	if litt {
		tsfmt = "Mon.Jan.2.34PM"
	}
	tstr := time.Now().Format(tsfmt)
	var (
		gitroot, cd string
		err         error
	)

	gitroot, cd, err = gitRoot()
	if err != nil {
		return fmt.Errorf("gitroot: %w", err)
	}
	{
		var part1, part2 string
		cdpath := strings.Split(cd, string(os.PathSeparator))
		rootpath := strings.Split(gitroot, string(os.PathSeparator))
		delta := len(cdpath) - len(rootpath)
		if delta == 0 { // we are in git root directory
			part1 = "root"
		}
		if delta >= 1 { // only one depth: root of sub directory of the root
			part1 = cdpath[len(rootpath)]
		}
		if delta >= 2 {
			l := 2
			if delta == 2 {
				l = 1
			}
			part2 = strings.Join(cdpath[len(cdpath)-l:], ".")
		}
		tag := fmt.Sprintf("%s.dev-%s.%s", part1, tstr, part2)

		// Remove extra dot character
		if tag[len(tag)-1] == '.' {
			tag = tag[:len(tag)-1]
		}

		fmt.Println(tag)
	}
	return nil
}

type tstampFormat struct{ litt bool }

func (tsf tstampFormat) print() error {
	if err := timestamp(tsf.litt); err != nil {
		return err
	}
	return nil
}

type claudeMsg struct {
	Role    string             `json:"role"`
	Content []claudeMsgContent `json:"content"`
}

type claudeMsgContent interface {
	fmt.Stringer
	json.Marshaler
}

func newClaudeMsgTxt(text string) claudeMsgContent {
	return claudeMsgTxt{
		Type: "text",
		Text: text,
	}
}

func (c claudeMsgTxt) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		Type: c.Type,
		Text: c.Text,
	})
}

func (cmc claudeMsgTxt) String() string {
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(cmc); err != nil {
		panic(err)
	}
	return b.String()
}

type claudeMsgTxt struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var red = func(s string) string {
	return fmt.Sprintf("\033[31m%s\033[0m", s)
}

type printUtil struct {
	out       io.Writer
	cut       string
	noNewLine bool
}

const (
	printBlack  = "\033[40m"
	printYellow = "\033[33m"
	printGreen  = "\033[32m"
	printReset  = "\033[0m"
)

func (u printUtil) seq(steps ...string) {
	for _, lex := range steps {
		fmt.Fprint(u.out, lex)
	}
	nl := "\n"
	if u.noNewLine {
		nl = ""
	}
	fmt.Fprint(u.out, nl)
}

func (u printUtil) yellowOnBlack() {
	u.seq(
		printBlack,  // background
		printYellow, // foreground
		u.cut,       // cut
		printReset,  // reset colors
	)
}

func (u printUtil) greenOnBlack() {
	u.seq(
		printBlack,
		printGreen,
		u.cut,
		printReset,
	)
}
