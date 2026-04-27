// Package output handles delivering the inflated prompt to the user's
// destination terminal.
package output

import (
	"fmt"

	"golang.design/x/clipboard"
)

var clipboardReady bool

// Init must be called once before any clipboard call.
func Init() error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}
	clipboardReady = true
	return nil
}

// WriteClipboard puts text on the system clipboard.
func WriteClipboard(text string) error {
	if !clipboardReady {
		return fmt.Errorf("clipboard not initialized")
	}
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}
