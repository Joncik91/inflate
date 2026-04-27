// Package provider abstracts the LLM backends Inflate can call.
package provider

import "context"

// Request is the model-agnostic prompt envelope.
type Request struct {
	System    string
	User      string
	MaxTokens int
}

// Provider is the contract every backend implements.
type Provider interface {
	// Stream returns a channel of text chunks, closed when the response ends
	// or ctx is cancelled. err is non-nil only for setup failures (auth,
	// network unreachable). Mid-stream errors close the channel silently.
	Stream(ctx context.Context, req Request) (<-chan string, error)

	// Validate makes a 1-token ping to confirm credentials work.
	Validate(ctx context.Context) error

	// Name returns a human label for the status line.
	Name() string
}
