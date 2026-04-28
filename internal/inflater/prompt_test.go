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
	want := "<jsonl> contains proposals, decisions, AND rejections"
	if !strings.Contains(s, want) {
		t.Errorf("system prompt missing the JSONL-as-exploration rule: %q", s)
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
