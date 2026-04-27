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
