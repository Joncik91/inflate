package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	// Compute the inner content width of the rounded preview pane.
	// Total visible width = m.width. Subtract 2 chars of border + 4 chars
	// of horizontal padding (Padding(1, 2) = 2 left + 2 right) = 6.
	// Floor at 20 so very narrow terminals still produce *some* output.
	contentWidth := m.width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	var paneBody string
	switch {
	case m.helpOpen:
		paneBody = helpText
	case m.preview == "":
		if m.inflating {
			paneBody = "(inflating…)"
		} else {
			paneBody = "(type a fragment below)"
		}
	default:
		paneBody = renderPreview(m.preview, contentWidth)
	}

	pane := previewStyle.Width(contentWidth)
	previewBlock := pane.Render(paneBody)
	if m.stale && !m.helpOpen {
		previewBlock = pane.Faint(true).Render(paneBody)
	}

	statusLine := renderStatus(m)
	input := inputStyle.Render("> " + m.seed + "▌")

	parts := []string{previewBlock}
	if m.inflating {
		parts = append(parts, fmt.Sprintf("  %s Inflating…", spinnerFrames[m.spinnerFrame]))
	}
	parts = append(parts, statusLine)
	if m.errBanner != "" {
		parts = append(parts, statusStyleErr.Render(m.errBanner))
	}
	parts = append(parts, input)
	return strings.Join(parts, "\n")
}

// renderPreview prefers the named-section layout when the inflated text
// follows the Promptism skeleton (Role:/Context:/Task:/Constraints:/Output:).
// Falls back to glamour when the parse fails. width is the inner content
// width of the surrounding pane; long bodies are word-wrapped to that width
// and indented under their label so the layout stays readable on narrow
// and wide terminals.
func renderPreview(text string, width int) string {
	if sections := parseSections(text); len(sections) > 0 {
		var b strings.Builder
		// Indent body lines under the label by the visible label width
		// plus the "  ·  " separator. Use lipgloss to wrap; it counts
		// printable runes correctly even for styled strings.
		for i, s := range sections {
			if i > 0 {
				b.WriteString("\n")
			}
			label := sectionLabelStyle.Render(s.Label)
			sep := "  ·  "
			indent := lipgloss.NewStyle().PaddingLeft(len(s.Label) + len(sep)).Render
			body := lipgloss.NewStyle().Width(width - len(s.Label) - len(sep)).Render(s.Body)
			lines := strings.Split(body, "\n")
			b.WriteString(label)
			b.WriteString(sep)
			b.WriteString(lines[0])
			for _, rest := range lines[1:] {
				b.WriteString("\n")
				b.WriteString(indent(rest))
			}
		}
		return b.String()
	}
	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return text
	}
	return strings.TrimRight(rendered, "\n")
}

type promptSection struct {
	Label string
	Body  string
}

// parseSections returns Role/Context/Task/Constraints/Output sections
// from a Promptism-formatted prompt. Returns nil if the format isn't
// recognized (e.g. the LLM produced freeform output, or the response
// is mid-stream and incomplete).
func parseSections(text string) []promptSection {
	known := []string{"Role", "Context", "Task", "Constraints", "Output"}
	var sections []promptSection
	current := -1
	var bodies []strings.Builder
	for _, line := range strings.Split(text, "\n") {
		matched := false
		for _, label := range known {
			prefix := label + ":"
			if strings.HasPrefix(strings.TrimSpace(line), prefix) {
				body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix))
				sections = append(sections, promptSection{Label: label})
				bodies = append(bodies, strings.Builder{})
				bodies[len(bodies)-1].WriteString(body)
				current = len(sections) - 1
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if current >= 0 {
			t := strings.TrimSpace(line)
			if t == "" {
				continue
			}
			if bodies[current].Len() > 0 {
				bodies[current].WriteString(" ")
			}
			bodies[current].WriteString(t)
		}
	}
	if len(sections) == 0 {
		return nil
	}
	for i := range sections {
		sections[i].Body = bodies[i].String()
	}
	// Reject parse if the *first* line of the input wasn't a recognized
	// section label — this catches cases where the LLM wrote a preamble.
	firstLine := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	firstOK := false
	for _, label := range known {
		if strings.HasPrefix(firstLine, label+":") {
			firstOK = true
			break
		}
	}
	if !firstOK {
		return nil
	}
	return sections
}

// renderStatus builds the human-readable status line. Layout:
//
//	Using profile, git, shell · DeepSeek
//	Missing: open editor file
//
// Color comes from bundle.Severity(): green / yellow / red.
func renderStatus(m Model) string {
	style := statusStyleOK
	switch m.bundle.Severity() {
	case "warn":
		style = statusStyleWarn
	case "err":
		style = statusStyleErr
	}

	present := m.bundle.PresentSources()
	if present == "" {
		present = "no context yet"
	}
	primary := fmt.Sprintf("Using %s  ·  %s", present, m.provider.Name())
	if m.bundle.Redacted > 0 {
		primary = fmt.Sprintf("%s  ·  %d redactions", primary, m.bundle.Redacted)
	}
	out := style.Render(primary)

	if missing := m.bundle.MissingSources(); missing != "" && m.bundle.Severity() != "ok" {
		out = out + "\n" + style.Render("Missing: "+missing)
	}

	if m.toast != "" {
		out = out + "   " + toastStyle.Render(m.toast)
	}
	return out
}
