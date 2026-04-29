package inflater

import (
	"context"
	"time"

	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/provider"
)

const (
	maxOutputTokens = 800
	// totalDeadline is generous on purpose. Cloud providers usually finish
	// in 3-10s, so this only matters for local Ollama models. A 36B MoE on
	// integrated GPU runs at ~17 tok/s — 800 output tokens takes ~47s, and
	// a longer Promptism expansion can hit ~90s. Anything that runs longer
	// than 3 minutes is genuinely hung and worth cancelling.
	totalDeadline = 180 * time.Second
)

// Inflate calls the provider with the system + user prompts assembled from
// the bundle and seed, returning a channel of streamed text chunks. The
// channel is closed when the response ends, ctx is cancelled, or the
// 30-second total deadline elapses.
func Inflate(ctx context.Context, p provider.Provider, b harvester.ContextBundle, seed string) <-chan string {
	out := make(chan string)
	if seed == "" {
		close(out)
		return out
	}

	go func() {
		defer close(out)
		callCtx, cancel := context.WithTimeout(ctx, totalDeadline)
		defer cancel()

		req := provider.Request{
			System:    SystemPrompt(b),
			User:      UserPrompt(b, seed),
			MaxTokens: maxOutputTokens,
		}
		ch, err := p.Stream(callCtx, req)
		if err != nil {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-ch:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- chunk:
				}
			}
		}
	}()
	return out
}
