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
- <processes>: dev tools currently running in the user's session right now (e.g. "claude, go, vim"). High signal for "what is the user actively doing" — if cargo is in the list it's likely a Rust project, if pytest then Python, etc.
- <profile>: who the user is and their style preference.

Rules:
- Use only what's in the context blocks. Never invent files, errors, or facts.
- The current project is always <cwd> + <git>. If <shell>/<jsonl> mention a different repo, ignore it for the "Context" section.
- The presence of a <jsonl> block IS a fact: it means a Claude Code session is currently active in this directory. The CONTENTS of <jsonl> (recent assistant replies, file references) are exploration, not authoritative — proposals discussed there may have been rejected. So: feel free to mention "an active Claude Code session is open" when <jsonl> is present, but do not assert that any specific artifact mentioned inside it exists or is in use unless <git> or <file> confirms it.
- File-name corollary: a filename appearing ONLY inside <jsonl> is NOT a real file. Do not cite it in Constraints, Output, or Context as if it exists ("items in BACKLOG.md", "the FOO.md spec"). If you need to reference a file, the path must appear in <git> or <file>. If neither has it, drop the reference entirely or phrase it as "the file the user mentioned earlier (if any)".
- **Special case — when <jsonl> is absent (no active Claude Code session for this directory):**
  - If <git> is also absent, the user is in a non-repo dir without a session: write "Context: working in <cwd> outside any git repo. No active Claude Code session." and stop. Do not promote shell history to "Context."
  - If <git> IS present (repo exists, just no active Claude Code chat): use git+file as the Context source as normal.
  - Treat the seed as a generic question if neither <git> nor <jsonl> is present; ask the user for clarifying details if the task isn't a generic shell/admin query.
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
	if b.ProcessesOK {
		fmt.Fprintf(&sb, "<processes>%s</processes>\n", b.Processes)
	}
	fmt.Fprintf(&sb, "<seed>%s</seed>\n", seed)
	return sb.String()
}
