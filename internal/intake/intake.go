// Package intake runs the one-time first-run wizard.
package intake

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Joncik91/inflate/internal/config"
)

// RunFromReader reads three lines from r, writes prompts to w, returns the Profile.
func RunFromReader(r io.Reader, w io.Writer) (config.Profile, error) {
	scanner := bufio.NewScanner(r)
	ask := func(q string) string {
		fmt.Fprintln(w, q)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		return ""
	}
	identity := ask("Who are you? (e.g., senior backend engineer, mostly Go and Python)")
	work := ask("What kind of work? (e.g., API services, CLI tools)")
	style := normalizeStyle(ask("Prompt style preference? terse / standard / verbose"))
	if identity == "" {
		identity = "developer"
	}
	if work == "" {
		work = "general software engineering"
	}
	return config.Profile{Identity: identity, Work: work, Style: style}, nil
}

func normalizeStyle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "terse", "acolyte":
		return "terse"
	case "verbose", "grandmaster":
		return "verbose"
	default:
		return "standard"
	}
}
