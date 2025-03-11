package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newTerraformCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tf",
		Short: "terraform template commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	return cmd
}

func newGithubTerraformCommand() *cobra.Command {
	var private *bool
	var name, owner, description *string
	cmd := &cobra.Command{
		Use:   "github",
		Short: "populate files for defining github tf resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			mainFilename := "main.tf"
			// First check that all files to populate are not already there
			{
				_, err := os.Stat(mainFilename)
				if err == nil {
					return fmt.Errorf("main.tf already exists")
				}
			}

			githubModuleDir := filepath.Join("modules", "github")
			{
				info, _ := os.Stat(githubModuleDir)
				if info.IsDir() {
					return fmt.Errorf("modules/github already exists and is already a directory")
				}
			}

			githubModuleFile := filepath.Join(githubModuleDir, "repo.tf")
			{
				_, err := os.Stat(githubModuleFile)
				if err == nil {
					return fmt.Errorf("modules/github/repo.tf already exists")
				}
			}
			const repoTf = `
resource "github_repository" "%[1]s" {
  name        = "%[1]s"
  description = "%[2]s"
  visibility  = "%[3]s"
}

output "url" {
  value = github_repository.%[1]s.ssh_clone_url
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
			if err = os.MkdirAll(githubModuleDir, 0700); err != nil {
				return fmt.Errorf("unable to create %q: %w", githubModuleDir, err)
			}
			var f *os.File
			if f, err = os.Create(githubModuleFile); err != nil {
				return fmt.Errorf("unable to create %q: %w", githubModuleFile, err)
			}
			var (
				visibility = "public"
			)
			if *private {
				visibility = "private"
			}
			if *owner == "" || *name == "" {
				return fmt.Errorf("must provide non-zero --name and --owner flags")
			}
			if _, err = fmt.Fprintf(f, repoTf, *name, *description, visibility); err != nil {
				return fmt.Errorf("unable to write %q: %w", githubModuleFile, err)
			}
			if err = f.Close(); err != nil {
				return fmt.Errorf("unable to close %q: %w", f.Name(), err)
			}
			if f, err = os.Create(mainFilename); err != nil {
				return fmt.Errorf("create %q: %w", mainFilename, err)
			}
			if _, err = fmt.Fprintf(f, mainTf, *owner); err != nil {
				return fmt.Errorf("unable to populate main.tf: %w", err)
			}
			if err = f.Close(); err != nil {
				return fmt.Errorf("unable to close %q: %w", f.Name(), err)
			}
			// init then apply tofu configuration
			{
				x := exec.CommandContext(cmd.Context(), "tofu", "init")
				x.Stdin = os.Stdin
				x.Stdout = os.Stdout
				x.Stderr = os.Stderr
				if err = x.Run(); err != nil {
					return fmt.Errorf("tofu init: %w", err)
				}
			}
			{
				x := exec.CommandContext(cmd.Context(), "tofu", "apply")
				x.Stdin = os.Stdin
				x.Stdout = os.Stdout
				x.Stderr = os.Stderr
				if err = x.Run(); err != nil {
					return fmt.Errorf("tofu apply: %w", err)
				}
			}
			return nil
		},
	}
	private = cmd.Flags().Bool("private", false, "set private visibility for the repository")
	description = cmd.Flags().String("description", "", "repository description")
	name = cmd.Flags().String("name", "", "repository name (flag is required)")
	owner = cmd.Flags().String("owner", "", "repository owner (flag is required)")

	return cmd
}
