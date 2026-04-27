// Package output handles delivering the inflated prompt to the user's
// destination terminal.
package output

import (
	"fmt"

	"github.com/atotto/clipboard"
)

// Init is a no-op for atotto/clipboard but kept for API symmetry. It returns
// nil unless clipboard.Unsupported is true (no clipboard backend available).
func Init() error {
	if clipboard.Unsupported {
		return fmt.Errorf("no clipboard backend available (install xclip or xsel on Linux)")
	}
	return nil
}

// WriteClipboard puts text on the system clipboard.
func WriteClipboard(text string) error {
	return clipboard.WriteAll(text)
}
