package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	// Lipgloss border-width handling: Width() on a style with Padding
	// represents the inner area + padding (NOT counting border). So the
	// total rendered width is Width + 2 (one column per side for the
	// border). To make the rounded box fit inside the terminal we set
	// paneWidth = m.width - 2. The contentWidth available for body
	// word-wrap is paneWidth - 4 (horizontal padding 2 left + 2 right).
	// Floor at 24 so very narrow terminals still produce *some* output.
	paneWidth := m.width - 2
	if paneWidth < 24 {
		paneWidth = 24
	}
	contentWidth := paneWidth - 4

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

	pane := previewStyle.Width(paneWidth)
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
		bodyStyle := lipgloss.NewStyle().Width(width)
		for i, s := range sections {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(sectionLabelStyle.Render(s.Label))
			b.WriteString("\n")
			// Body text wraps at full pane width and starts at column
			// zero on its own line. Soft line-wraps from the LLM
			// (e.g. when it formatted to ~70 cols itself) get
			// collapsed first so lipgloss can re-wrap to the actual
			// pane width without producing awkward sub-sentence breaks.
			b.WriteString(bodyStyle.Render(reflowBody(s.Body)))
		}
		return b.String()
	}
	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return text
	}
	return strings.TrimRight(rendered, "\n")
}

// reflowBody collapses soft line-wraps inside a section body while keeping
// structural breaks. Heuristic: a single \n joining two non-empty lines
// where the next line starts with a lowercase letter (or doesn't look
// like a list marker) is treated as a soft wrap from the LLM and
// replaced with a space. Anything else (blank lines, "- " bullets,
// "1. " numbers, mid-line punctuation) stays as-is so deliberate
// multi-line formatting survives.
func reflowBody(body string) string {
	lines := strings.Split(body, "\n")
	if len(lines) <= 1 {
		return body
	}
	var out strings.Builder
	for i, line := range lines {
		if i == 0 {
			out.WriteString(line)
			continue
		}
		prev := lines[i-1]
		trimmed := strings.TrimSpace(line)
		if isStructuralBreak(prev, line, trimmed) {
			out.WriteString("\n")
			out.WriteString(line)
		} else {
			// Soft wrap — fold into previous line with a space.
			out.WriteString(" ")
			out.WriteString(trimmed)
		}
	}
	return out.String()
}

// isStructuralBreak returns true if the boundary between prev and curr
// looks intentional (a paragraph break, list item, or numbered item),
// false if it looks like an LLM soft-wrap to fit ~70 cols.
func isStructuralBreak(prev, curr, currTrimmed string) bool {
	// Blank line = paragraph break.
	if currTrimmed == "" || strings.TrimSpace(prev) == "" {
		return true
	}
	// List markers at the start of curr signal intentional list rendering.
	if strings.HasPrefix(currTrimmed, "- ") ||
		strings.HasPrefix(currTrimmed, "* ") ||
		strings.HasPrefix(currTrimmed, "• ") {
		return true
	}
	// Numbered list: "1. foo", "10) bar".
	if len(currTrimmed) >= 2 {
		c0 := currTrimmed[0]
		if c0 >= '0' && c0 <= '9' {
			for j := 1; j < len(currTrimmed); j++ {
				cj := currTrimmed[j]
				if cj >= '0' && cj <= '9' {
					continue
				}
				if (cj == '.' || cj == ')') && j+1 < len(currTrimmed) && currTrimmed[j+1] == ' ' {
					return true
				}
				break
			}
		}
	}
	return false
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
		// Strip leading markdown decoration that smaller local models
		// sometimes emit around section labels: "**Output:**", "## Output:",
		// "- **Output**:". Normalize before prefix-matching.
		stripped := strings.TrimSpace(line)
		stripped = strings.TrimLeft(stripped, "*#-> ")
		stripped = strings.TrimSpace(stripped)
		for _, label := range known {
			prefix := label + ":"
			altPrefix := label + "**:" // matches "**Output**:" → "Output**:"
			if strings.HasPrefix(stripped, prefix) || strings.HasPrefix(stripped, altPrefix) {
				rest := strings.TrimPrefix(stripped, label)
				rest = strings.TrimLeft(rest, "*: ")
				body := strings.TrimRight(rest, "* ")
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
			// Preserve the LLM's line breaks. Lists (Constraints, multi-line
			// Context) read better with explicit newlines; lipgloss will
			// wrap each line at the pane width independently.
			if bodies[current].Len() > 0 {
				bodies[current].WriteString("\n")
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
	// Strip the same markdown decoration we accept in body matching.
	firstLine := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	firstLine = strings.TrimLeft(firstLine, "*#-> ")
	firstLine = strings.TrimSpace(firstLine)
	firstOK := false
	for _, label := range known {
		if strings.HasPrefix(firstLine, label+":") || strings.HasPrefix(firstLine, label+"**:") {
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

	// Neighbor-repo hint: if we're in a parent dir with N child repos
	// underneath us, suggest re-launching from one of them so git/file
	// context is grounded. Two-line layout so a long hint doesn't get
	// clipped on the right side of the terminal.
	if len(m.bundle.NeighborRepos) > 0 && !m.bundle.GitOK {
		repos := m.bundle.NeighborRepos
		if len(repos) > 5 {
			repos = append(repos[:5:5], "…")
		}
		count := fmt.Sprintf("Hint: %d git repos in subdirs — relaunch from one for full context",
			len(m.bundle.NeighborRepos))
		names := "  → " + strings.Join(repos, ", ")
		out = out + "\n" + statusStyleWarn.Render(count) + "\n" + statusStyleWarn.Render(names)
	}

	if m.toast != "" {
		out = out + "   " + toastStyle.Render(m.toast)
	}
	return out
}
