// Copyright (c) 2025 Arc Engineering
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/arc-git/internal/prompt"
	"github.com/yourorg/arc-sdk/ai"
	"github.com/yourorg/arc-sdk/errors"
	"github.com/yourorg/arc-sdk/output"
)

// newAnnotateCmd creates the annotate subcommand.
func newAnnotateCmd(aiCfg *ai.Config) *cobra.Command {
	var (
		since      int
		from       string
		to         string
		provider   string
		model      string
		apiKey     string
		dryRun     bool
		force      bool
		outputOpts output.OutputOptions
	)

	cmd := &cobra.Command{
		Use:   "annotate",
		Short: "Annotate git commits with AI-generated notes",
		Long: `Annotate git commits with AI-generated notes explaining the changes.

This command iterates through commits and generates technical notes using AI,
storing them in git notes. The notes provide context and explanations that
make git history more understandable months or years later.

The annotation is stored under the "ai" notes ref, viewable with:
  git log --show-notes=ai
  git log --notes=ai --grep="refactor"

For each commit, the AI analyzes:
- The diff and file changes
- The commit message
- Surrounding context from adjacent commits
- Code patterns and implications

This creates a searchable, AI-enriched git history.`,
		Example: `  # Annotate the last 10 commits
  arc-git annotate --since 10

  # Limit the scan to a specific commit range
  arc-git annotate --from HEAD~5 --to HEAD

  # Switch providers when experimenting with different AI stacks
  arc-git annotate --since 20 --provider openrouter

  # Preview what would be written without touching git notes
  arc-git annotate --since 5 --dry-run

  # Overwrite existing annotations when regenerating
  arc-git annotate --since 10 --force

  # Emit structured JSON for downstream tooling
  arc-git annotate --since 20 --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := outputOpts.Resolve(); err != nil {
				return err
			}

			// Build effective config with flag overrides
			cfg := *aiCfg
			if provider != "" {
				cfg.Provider = provider
			}
			if apiKey != "" {
				cfg.APIKey = apiKey
			}
			if model != "" {
				cfg.DefaultModel = model
			}

			return runAnnotate(&cfg, since, from, to, dryRun, force, outputOpts)
		},
	}

	cmd.Flags().IntVar(&since, "since", 10, "Annotate last N commits")
	cmd.Flags().StringVar(&from, "from", "", "Start commit (e.g., HEAD~20)")
	cmd.Flags().StringVar(&to, "to", "HEAD", "End commit (default: HEAD)")
	cmd.Flags().StringVar(&provider, "provider", "", "AI provider (claude, anthropic, openrouter)")
	cmd.Flags().StringVar(&model, "model", "", "Model to use")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview annotations without saving")
	cmd.Flags().BoolVar(&force, "force", false, "Re-annotate existing commits")
	outputOpts.AddOutputFlags(cmd, output.OutputTable)

	return cmd
}

// runAnnotate implements the git annotation workflow.
func runAnnotate(cfg *ai.Config, since int, from, to string, dryRun, force bool, out output.OutputOptions) error {
	// Helper for conditional logging (quiet mode suppresses progress)
	logProgress := func(format string, args ...interface{}) {
		if !out.Is(output.OutputQuiet) && !out.Is(output.OutputJSON) && !out.Is(output.OutputYAML) {
			fmt.Printf(format, args...)
		}
	}

	logProgress("Starting git history annotation...\n")

	// Get commits to annotate
	commits, err := getCommits(since, from, to)
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	if len(commits) == 0 {
		logProgress("No commits to annotate.\n")
		return nil
	}

	logProgress("Found %d commits to annotate\n", len(commits))

	// Validate config
	if err := ai.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("invalid AI configuration: %w", err)
	}

	// Create AI client and service
	client, err := ai.NewClient(*cfg)
	if err != nil {
		return fmt.Errorf("failed to create AI client: %w", err)
	}
	service := ai.NewService(client, *cfg)

	// Process commits
	annotated := 0
	skipped := 0
	failed := 0

	// Track results for JSON output
	type AnnotationResult struct {
		Hash       string `json:"hash"`
		Status     string `json:"status"`
		Message    string `json:"message,omitempty"`
		Annotation string `json:"annotation,omitempty"`
	}
	var results []AnnotationResult

	for i, commit := range commits {
		logProgress("\n[%d/%d] Processing %s\n", i+1, len(commits), commit.Hash[:7])

		// Check if already annotated (unless --force)
		if !force && hasNote(commit.Hash, "ai") {
			logProgress("  Already annotated (use --force to re-annotate)\n")
			skipped++
			results = append(results, AnnotationResult{
				Hash:    commit.Hash[:7],
				Status:  "skipped",
				Message: "already annotated",
			})
			continue
		}

		// Get commit diff
		diff, err := getCommitDiff(commit.Hash)
		if err != nil {
			logProgress("  Failed to get diff: %v\n", err)
			failed++
			results = append(results, AnnotationResult{
				Hash:    commit.Hash[:7],
				Status:  "failed",
				Message: fmt.Sprintf("failed to get diff: %v", err),
			})
			continue
		}

		if len(diff) == 0 {
			logProgress("  No diff (merge commit?), skipping\n")
			skipped++
			results = append(results, AnnotationResult{
				Hash:    commit.Hash[:7],
				Status:  "skipped",
				Message: "no diff (merge commit?)",
			})
			continue
		}

		// Generate annotation
		logProgress("  Generating AI annotation...\n")
		annotation, err := generateAnnotation(service, commit, diff)
		if err != nil {
			logProgress("  Failed to generate annotation: %v\n", err)
			failed++
			results = append(results, AnnotationResult{
				Hash:    commit.Hash[:7],
				Status:  "failed",
				Message: fmt.Sprintf("failed to generate annotation: %v", err),
			})
			continue
		}

		// Preview or save
		if dryRun {
			logProgress("\n--- Annotation for %s ---\n%s\n", commit.Hash[:7], annotation)
			results = append(results, AnnotationResult{
				Hash:       commit.Hash[:7],
				Status:     "preview",
				Annotation: annotation,
			})
		} else {
			if err := addNote(commit.Hash, "ai", annotation); err != nil {
				logProgress("  Failed to add note: %v\n", err)
				failed++
				results = append(results, AnnotationResult{
					Hash:    commit.Hash[:7],
					Status:  "failed",
					Message: fmt.Sprintf("failed to add note: %v", err),
				})
				continue
			}
			logProgress("  Annotated successfully\n")
			results = append(results, AnnotationResult{
				Hash:       commit.Hash[:7],
				Status:     "success",
				Annotation: annotation,
			})
		}

		annotated++
	}

	// Output results
	switch {
	case out.Is(output.OutputJSON):
		result := map[string]interface{}{
			"total":     len(commits),
			"annotated": annotated,
			"skipped":   skipped,
			"failed":    failed,
			"dry_run":   dryRun,
			"results":   results,
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return errors.NewCLIError(fmt.Sprintf("failed to encode JSON: %v", err)).
				WithHint("Try --output table instead")
		}
	case out.Is(output.OutputQuiet):
		// Quiet mode: suppress summary
	default:
		fmt.Printf("\n=== Annotation Complete ===\n")
		fmt.Printf("Annotated: %d\n", annotated)
		fmt.Printf("Skipped: %d\n", skipped)
		fmt.Printf("Failed: %d\n", failed)

		if dryRun {
			fmt.Println("\n(Dry run - no notes were added)")
			fmt.Println("Run without --dry-run to save annotations")
		} else {
			fmt.Println("\nView annotations with: git log --show-notes=ai")
		}
	}

	return nil
}

// Commit represents a git commit for annotation.
type Commit struct {
	Hash    string
	Message string
	Author  string
	Date    string
}

// getCommits gets the list of commits to annotate.
func getCommits(since int, from, to string) ([]Commit, error) {
	var commits []Commit

	// Build git log command
	args := []string{"log", "--format=%H%n%an <%ae>%n%ad%n%s", "--no-merges"}

	if from != "" {
		args = append(args, fmt.Sprintf("%s..%s", from, to))
	} else {
		args = append(args, fmt.Sprintf("-n%d", since))
	}

	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	// Parse output
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := 0; i+3 <= len(lines); i += 4 {
		if lines[i] == "" {
			break
		}
		commits = append(commits, Commit{
			Hash:    lines[i],
			Author:  lines[i+1],
			Date:    lines[i+2],
			Message: lines[i+3],
		})
	}

	return commits, nil
}

// getCommitDiff gets the diff for a specific commit.
func getCommitDiff(hash string) (string, error) {
	cmd := exec.Command("git", "show", "--format=", hash)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show failed: %w", err)
	}
	return string(out), nil
}

// hasNote checks if a commit has a note under the given ref.
func hasNote(hash, ref string) bool {
	cmd := exec.Command("git", "notes", "--ref", ref, "show", hash)
	return cmd.Run() == nil
}

// addNote adds a note to a commit under the given ref.
func addNote(hash, ref, note string) error {
	// Write note to temp file
	tmpFile, err := os.CreateTemp("", "arc-git-note-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(note); err != nil {
		return fmt.Errorf("failed to write note: %w", err)
	}
	tmpFile.Close()

	// Add note using git notes
	cmd := exec.Command("git", "notes", "--ref", ref, "add", "-F", tmpFile.Name(), hash)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git notes failed: %w\nOutput: %s", err, out)
	}

	return nil
}

// generateAnnotation generates an AI annotation for a commit.
func generateAnnotation(service *ai.Service, commit Commit, diff string) (string, error) {
	systemPrompt, userPrompt := prompt.AnnotateCommit(commit.Hash[:7], commit.Message, commit.Author, commit.Date, diff)

	ctx := context.Background()
	resp, err := service.Run(ctx, ai.RunOptions{
		System: systemPrompt,
		Prompt: userPrompt,
		Model:  prompt.AnnotateCommitModel,
	})
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}

	return strings.TrimSpace(resp.Text), nil
}
