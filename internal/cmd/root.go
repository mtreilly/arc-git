// Copyright (c) 2025 Arc Engineering
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yourorg/arc-sdk/ai"
)

// NewRootCmd creates the root command for arc-git.
func NewRootCmd(aiCfg *ai.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "arc-git",
		Short: "Git integration with AI",
		Long: `Git integration with AI-powered features for history analysis,
commit annotation, and repository intelligence.

These commands enhance git workflows with AI-generated insights and
automated documentation of code changes.`,
		Example: `  # Annotate recent commits with AI assistance
  arc-git annotate --since 10

  # Focus on a specific commit range
  arc-git annotate --from HEAD~5 --to HEAD

  # Inspect annotations alongside regular history
  git log --show-notes=ai

  # Search only the AI-generated notes for key terms
  git log --grep "refactor" --notes=ai`,
	}

	root.AddCommand(
		newAnnotateCmd(aiCfg),
	)

	return root
}
