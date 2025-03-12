package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	ollama "github.com/ollama/ollama/api"
)

func newClaudeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "claude",
		Short: "invoke claude",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

func newClaudeCommitCommand() *cobra.Command {

	var noCommitOpt, clearOpt, noLlamaOpt *bool
	var vertexProjectId, vertexModel, vertexLocation *string

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "ask claude for a good commit message (vertexai)",
		RunE: func(cmd *cobra.Command, args []string) error {

			debug, err := zap.NewProduction()
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

			projectId := *vertexProjectId
			model := *vertexModel
			location := *vertexLocation
			cli := http.DefaultClient
			var url = fmt.Sprintf("https://%[2]s-aiplatform.googleapis.com/v1/projects/%[3]s/locations/%[2]s/publishers/anthropic/models/%[1]s:streamRawPredict", model, location, projectId)
			// fmt.Println("POST", url)
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
					// debug.Info("hey")
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
			// w := io.MultiWriter(os.Stderr, &buf)
			// fmt.Println("payload:")
			if err := json.NewEncoder(&buf).Encode(payload); err != nil {
				return err
			}
			// fmt.Println()
			req, err := http.NewRequest(http.MethodPost, url, &buf)
			if err != nil {
				return err
			}
			debug.Debug("new request for vertexai api",
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
				// fmt.Println()
				// fmt.Println("out", out.String())
				valid := strings.ToValidUTF8(out.String(), "?")
				r := strings.NewReader(valid)
				// fmt.Println(valid)
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

	noLlamaOpt = cmd.Flags().Bool("no-llama", true, "disable ollama llama3.2 commit post processing")
	noCommitOpt = cmd.Flags().Bool("no-commit", false, "disable git commit ultimate step")
	clearOpt = cmd.Flags().Bool("clear", false, "clear screen")

	vertexLocation = cmd.Flags().String("vx-location", "europe-west1", "vertex ai project location")
	vertexModel = cmd.Flags().String("vx-model", "claude-3-5-sonnet-v2@20241022", "vertex ai claude sonnet model id")
	vertexProjectId = cmd.Flags().String("vx-project", "upbeat-task-298823", "vertex ai project id")

	return cmd

}
