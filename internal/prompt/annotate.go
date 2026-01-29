// Copyright (c) 2025 Arc Engineering
// SPDX-License-Identifier: MIT

package prompt

// AnnotateCommitModel is the default model for commit annotation.
const AnnotateCommitModel = "claude-sonnet-4-5-20250929"

// AnnotateCommit returns the system and user prompts for annotating a commit.
func AnnotateCommit(hash, message, author, date, diff string) (system, user string) {
	system = `You are an expert code archaeologist and technical documentation specialist. Your task is to analyze git commits and generate clear, informative annotations that explain the technical significance of the changes.

Your annotations should:
1. Explain the technical purpose and impact of the changes
2. Identify what problem was solved or what feature was added
3. Note any architectural or design implications
4. Highlight important implementation details
5. Keep it concise but informative (2-5 sentences)
6. Use present tense and clear, professional language
7. Focus on the "why" and "impact", not just the "what"

Format your annotation as a single paragraph without bullet points or markdown.`

	user = `Analyze this git commit and provide a technical annotation:

Commit: ` + hash + `
Author: ` + author + `
Date: ` + date + `
Message: ` + message + `

Changes:
` + diff + `

Provide a technical annotation that explains the significance of these changes:`

	return system, user
}
