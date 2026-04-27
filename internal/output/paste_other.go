//go:build !linux

package output

import "fmt"

// Paste is unsupported outside Linux in v0; always returns an error so the
// caller falls back to clipboard-only.
func Paste(text string, windowID int) error {
	return fmt.Errorf("paste not supported on this platform (clipboard only)")
}
