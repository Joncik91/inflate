package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/provider"
)

type stubProvider struct{}

func (stubProvider) Name() string                                              { return "stub" }
func (stubProvider) Validate(_ context.Context) error                          { return nil }
func (stubProvider) Stream(_ context.Context, _ provider.Request) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func TestModelInitialView(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: "/tmp"})
	m := New(stubProvider{}, h, false, 0)
	v := tea.Model(m).View()
	if !strings.Contains(v, "type a fragment") {
		t.Errorf("expected hint in initial view, got:\n%s", v)
	}
}

func TestQuestionMarkTogglesHelp(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: "/tmp"})
	m := New(stubProvider{}, h, false, 0)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	v1 := model.(Model).View()
	if !strings.Contains(v1, "Keys") || !strings.Contains(v1, "Ctrl-C") {
		t.Errorf("expected help overlay after `?`, got:\n%s", v1)
	}

	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	v2 := model.(Model).View()
	if strings.Contains(v2, "Ctrl-C") {
		t.Errorf("expected help overlay closed after second `?`, got:\n%s", v2)
	}
}

func TestEscDismissesErrorBanner(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: "/tmp"})
	m := New(stubProvider{}, h, false, 0)
	m.errBanner = "inflate failed: 401"
	m.inflightID = 1

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := model.(Model).errBanner; got != "" {
		t.Errorf("expected errBanner cleared by Esc, got %q", got)
	}
}

func TestTypingClearsErrorBanner(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: "/tmp"})
	m := New(stubProvider{}, h, false, 0)
	m.errBanner = "clipboard error"

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if got := model.(Model).errBanner; got != "" {
		t.Errorf("expected typing to clear errBanner, got %q", got)
	}
}

func TestEscClearsSeedAfterErrorAlreadyDismissed(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: "/tmp"})
	m := New(stubProvider{}, h, false, 0)
	m.seed = "hello"

	// No error banner present — Esc should clear seed (preserves v0.1.2 behavior).
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := model.(Model).seed; got != "" {
		t.Errorf("expected Esc to clear seed when no error banner, got %q", got)
	}
}

func TestParseSectionsHappy(t *testing.T) {
	in := "Role: dev\nContext: working in repo X\nTask: fix bug\nConstraints: keep it simple\nOutput: a diff"
	got := parseSections(in)
	if len(got) != 5 {
		t.Fatalf("expected 5 sections, got %d (%+v)", len(got), got)
	}
	want := []string{"Role", "Context", "Task", "Constraints", "Output"}
	for i, w := range want {
		if got[i].Label != w {
			t.Errorf("section %d Label = %q, want %q", i, got[i].Label, w)
		}
	}
	if got[0].Body != "dev" {
		t.Errorf("Role body = %q, want %q", got[0].Body, "dev")
	}
}

func TestParseSectionsRejectsFreeformPreamble(t *testing.T) {
	in := "Here is your prompt:\nRole: dev\nTask: fix bug"
	if got := parseSections(in); got != nil {
		t.Errorf("preamble should reject section parse, got %+v", got)
	}
}

func TestParseSectionsRejectsRandomText(t *testing.T) {
	in := "just some explanatory text without any labels"
	if got := parseSections(in); got != nil {
		t.Errorf("freeform text should not parse as sections, got %+v", got)
	}
}
