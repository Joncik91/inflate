package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
)

func (m Model) View() string {
	var preview string
	if m.preview == "" {
		preview = "(type a fragment below)"
	} else {
		rendered, err := glamour.Render(m.preview, "dark")
		if err != nil {
			preview = m.preview
		} else {
			preview = strings.TrimRight(rendered, "\n")
		}
	}
	previewBlock := previewStyle.Render(preview)
	if m.stale {
		previewBlock = previewDimStyle.Render(preview)
	}

	status := fmt.Sprintf("ctx: %s  |  redactions: %d  |  provider: %s",
		m.bundle.FlagsString(), m.bundle.Redacted, m.provider.Name())
	statusLine := statusStyleOK.Render(status)
	if m.toast != "" {
		statusLine = statusLine + "   " + toastStyle.Render(m.toast)
	}

	input := inputStyle.Render("> " + m.seed + "▌")

	return previewBlock + "\n" + statusLine + "\n" + input
}
