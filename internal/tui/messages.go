package tui

import (
	"github.com/Joncik91/inflate/internal/harvester"
)

// inflateChunkMsg is sent for every chunk streamed back from the inflater.
type inflateChunkMsg struct{ Text string }

// inflateDoneMsg signals the inflater channel was closed.
type inflateDoneMsg struct{ ReqID int }

// inflateFailMsg signals an inflater error (the streaming channel never opened).
type inflateFailMsg struct{ Err string }

// idleFiredMsg signals the per-keystroke idle timer expired.
type idleFiredMsg struct{}

// bundleUpdatedMsg is sent when the harvester publishes a new ContextBundle.
type bundleUpdatedMsg struct{ Bundle harvester.ContextBundle }

// toastMsg shows an ephemeral status flash (e.g. "copied ✓").
type toastMsg struct{ Text string }

// toastClearMsg clears the toast after its TTL.
type toastClearMsg struct{}
