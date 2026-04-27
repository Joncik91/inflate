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

Rules:
- Use only what's in the context blocks. Never invent files, errors, or facts.
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
	if b.ProfileOK {
		fmt.Fprintf(&sb, "<profile>\n%s\n</profile>\n", b.Profile)
	}
	if b.GitOK {
		fmt.Fprintf(&sb, "<git>\n%s\n</git>\n", b.Git)
	}
	if b.ShellOK {
		fmt.Fprintf(&sb, "<shell>\n%s\n</shell>\n", b.Shell)
	}
	if b.FileOK {
		fmt.Fprintf(&sb, "<file>\n%s\n</file>\n", b.File)
	}
	if b.JSONLOK {
		fmt.Fprintf(&sb, "<jsonl>\n%s\n</jsonl>\n", b.JSONL)
	}
	fmt.Fprintf(&sb, "<seed>%s</seed>\n", seed)
	return sb.String()
}
