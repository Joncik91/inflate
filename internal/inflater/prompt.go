// Package inflater turns a harvested context + a short user fragment into
// a context-loaded prompt suitable for Claude Code.
package inflater

import (
	"fmt"
	"strings"

	"github.com/Joncik91/inflate/internal/harvester"
)

const skeletonRules = `When inflating a fragment, output exactly these sections in this order:

Role: <one line — who is being asked>
Context: <2-6 lines — what situation/system/state is relevant; cite files, branches, errors from <git>/<jsonl>/<file> when present>
Task: <one line — the imperative ask, derived from the user's <seed>>
Constraints: <1-3 short lines — preserve invariants, scope limits, taboo to break>
Output: <one line — the expected response shape>

How to read the context blocks:
- <cwd>: the AUTHORITATIVE current working directory. The "repository" / "project" the user is in is whatever <cwd> says — never derive it from <shell> or <jsonl>.
- <git>: AUTHORITATIVE for branch + recent commits + modified files in the CURRENT repo at <cwd>.
- <file>: open editor file inside <cwd>. Authoritative.
- <jsonl>: the active Claude Code session's recent transcript at <cwd>. Useful for what the user just discussed, but it may reference OTHER directories or projects the user navigated through earlier — do NOT infer the current project from it.
- <shell>: ambient command history from the user's account. NOT scoped to <cwd>. The commands are mostly historical and may reference projects, tools, env vars, and hosts the user is not currently working on. Treat as low-signal background noise.
- <profile>: who the user is and their style preference.

Rules:
- Use only what's in the context blocks. Never invent files, errors, or facts.
- The current project is always <cwd> + <git>. If <shell>/<jsonl> mention a different repo, ignore it for the "Context" section.
- **Special case — when both <git> and <jsonl> are absent (the user is in a non-repo dir or hasn't started a Claude Code session yet):**
  - <shell> is NOT a substitute for project context. Do not promote shell history to "Context."
  - In that case, write "Context: working in <cwd> outside any git repo. No active Claude Code session." and stop. Do not list shell-mentioned projects, tools, or env vars.
  - Treat the seed as a generic question; ask the user for clarifying details if the task isn't a generic shell/admin query.
- If a section can't be filled from context, write "ask for X if not provided".
- Match the user's style preference from <profile>.
- Return ONLY the inflated prompt. No preamble, no explanation, no markdown fences.`

// SystemPrompt returns the system message for the inflater LLM call.
// When the bundle has no usable context (everything ✗), the system prompt
// switches to "pure-structure" mode.
func SystemPrompt(b harvester.ContextBundle) string {
	mode := "rich-context"
	if !b.GitOK && !b.ShellOK && !b.FileOK && !b.JSONLOK {
		mode = "pure-structure (only profile available — keep inflation skeletal, no fabricated specifics)"
	}
	return fmt.Sprintf("You are Inflate, a prompt expander operating in %s mode.\n\n%s", mode, skeletonRules)
}

// UserPrompt assembles the XML-tagged context block + the user's seed.
func UserPrompt(b harvester.ContextBundle, seed string) string {
	var sb strings.Builder
	if b.Cwd != "" {
		fmt.Fprintf(&sb, "<cwd>%s</cwd>\n", b.Cwd)
	}
	if b.ProfileOK {
		fmt.Fprintf(&sb, "<profile>\n%s\n</profile>\n", b.Profile)
	}
	if b.GitOK {
		fmt.Fprintf(&sb, "<git>\n%s\n</git>\n", b.Git)
	}
	if b.FileOK {
		fmt.Fprintf(&sb, "<file>\n%s\n</file>\n", b.File)
	}
	if b.JSONLOK {
		fmt.Fprintf(&sb, "<jsonl>\n%s\n</jsonl>\n", b.JSONL)
	}
	if b.ShellOK {
		fmt.Fprintf(&sb, "<shell>\n%s\n</shell>\n", b.Shell)
	}
	fmt.Fprintf(&sb, "<seed>%s</seed>\n", seed)
	return sb.String()
}
