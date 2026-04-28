package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 80 * time.Millisecond

// spinnerTick returns a Cmd that fires one spinnerTickMsg after the
// interval. The Update handler re-issues the tick only while inflating,
// so the spinner stops repainting as soon as the first chunk arrives.
func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}
