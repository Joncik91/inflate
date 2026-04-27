package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/provider"
)

type fixedProvider struct{ text string }

func (fixedProvider) Name() string                    { return "fixed" }
func (fixedProvider) Validate(_ context.Context) error { return nil }
func (f fixedProvider) Stream(_ context.Context, _ provider.Request) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- f.text
	close(ch)
	return ch, nil
}

func TestTypingTriggersInflationAndUpdatesPreview(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(fixedProvider{text: "Role: dev\nTask: fix bug\n"}, h, false, 0)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 30))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix bug")})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Role:")
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}
