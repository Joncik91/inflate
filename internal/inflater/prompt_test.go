package inflater

import (
	"strings"
	"testing"

	"github.com/Joncik91/inflate/internal/harvester"
)

func TestSystemPromptIncludesSkeleton(t *testing.T) {
	s := SystemPrompt(harvester.ContextBundle{ProfileOK: true})
	for _, want := range []string{"Role:", "Task:", "Constraints:", "Output:"} {
		if !strings.Contains(s, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestSystemPromptHasJSONLExplorationRule(t *testing.T) {
	s := SystemPrompt(harvester.ContextBundle{ProfileOK: true})
	// Three-part rule: presence of jsonl is a fact (session is active),
	// contents are exploration not authoritative, AND a filename appearing
	// only inside jsonl must not be cited as if it exists.
	wants := []string{
		"presence of a <jsonl> block IS a fact",
		"are exploration, not authoritative",
		"a filename appearing ONLY inside <jsonl> is NOT a real file",
	}
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("system prompt missing %q\nsystem prompt:\n%s", w, s)
		}
	}
}

func TestSystemPromptPureStructureWhenEmpty(t *testing.T) {
	s := SystemPrompt(harvester.ContextBundle{})
	if !strings.Contains(s, "pure-structure") {
		t.Errorf("expected pure-structure mode in system prompt: %s", s)
	}
}

func TestUserPromptIncludesAvailableContext(t *testing.T) {
	b := harvester.ContextBundle{
		Profile:   "Identity: tester",
		Git:       "branch: main",
		ProfileOK: true,
		GitOK:     true,
	}
	u := UserPrompt(b, "fix the bug")
	for _, want := range []string{"<profile>", "Identity: tester", "<git>", "branch: main", "<seed>", "fix the bug"} {
		if !strings.Contains(u, want) {
			t.Errorf("user prompt missing %q\n%s", want, u)
		}
	}
	if strings.Contains(u, "<jsonl>") {
		t.Errorf("expected no jsonl section when JSONLOK=false")
	}
}
