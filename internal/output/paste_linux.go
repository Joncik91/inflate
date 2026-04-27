//go:build linux

package output

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Paste types text into the X11 window with the given ID using xdotool.
// windowID may be 0, in which case xdotool targets the focused window.
func Paste(text string, windowID int) error {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return fmt.Errorf("xdotool not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"type", "--delay", "4"}
	if windowID > 0 {
		args = append(args, "--window", strconv.Itoa(windowID))
	}
	args = append(args, text)
	cmd := exec.CommandContext(ctx, "xdotool", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool: %w (%s)", err, string(out))
	}
	return nil
}
