package tui

import (
	"github.com/Joncik91/inflate/internal/harvester"
)

// inflateStartedMsg signals the streaming inflation goroutine kicked off.
// The model uses this to start the spinner if no chunks have already arrived.
type inflateStartedMsg struct{ ReqID int }

// inflateChunkMsg is sent for every chunk streamed back from the inflater.
type inflateChunkMsg struct {
	Text  string
	ReqID int
}

// inflateDoneMsg signals the inflater channel was closed cleanly.
type inflateDoneMsg struct{ ReqID int }

// inflateFailMsg signals an inflater error (the streaming channel never
// opened, or closed without producing any content).
type inflateFailMsg struct {
	Err   string
	ReqID int
}

// spinnerTickMsg fires every spinnerInterval while inflating.
type spinnerTickMsg struct{}

// idleFiredMsg signals the per-keystroke idle timer expired.
type idleFiredMsg struct{}

// bundleUpdatedMsg is sent when the harvester publishes a new ContextBundle.
type bundleUpdatedMsg struct{ Bundle harvester.ContextBundle }

// toastMsg shows an ephemeral status flash (e.g. "copied ✓").
type toastMsg struct{ Text string }

// toastClearMsg clears the toast after its TTL.
type toastClearMsg struct{}
