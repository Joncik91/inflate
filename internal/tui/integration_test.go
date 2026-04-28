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

// teatestSender adapts *teatest.TestModel.Send to the tui.Sender interface.
type teatestSender struct{ tm interface{ Send(tea.Msg) } }

func (s teatestSender) Send(msg tea.Msg) { s.tm.Send(msg) }

func TestTypingTriggersInflationAndUpdatesPreview(t *testing.T) {
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(fixedProvider{text: "Role: dev\nTask: fix bug\nContext: ctx\nConstraints: c\nOutput: o\n"}, h, false, 0)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 30))
	// The streaming path needs a sender; in production main.go injects
	// the running tea.Program. In tests, the TestModel is itself the program.
	tm.Send(ProgramInjectMsg{Program: teatestSender{tm: tm}})

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix bug")})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		// Section labels render in bold cyan via lipgloss; the literal
		// "Role" word still appears in the byte stream.
		return strings.Contains(string(b), "Role")
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}
