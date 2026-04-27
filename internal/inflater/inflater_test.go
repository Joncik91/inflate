package inflater

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/provider"
)

type fakeProvider struct {
	chunks []string
	delay  time.Duration
}

func (f *fakeProvider) Name() string                          { return "fake" }
func (f *fakeProvider) Validate(ctx context.Context) error    { return nil }
func (f *fakeProvider) Stream(ctx context.Context, _ provider.Request) (<-chan string, error) {
	out := make(chan string)
	go func() {
		defer close(out)
		for _, c := range f.chunks {
			if f.delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(f.delay):
				}
			}
			select {
			case <-ctx.Done():
				return
			case out <- c:
			}
		}
	}()
	return out, nil
}

func TestInflateStreams(t *testing.T) {
	p := &fakeProvider{chunks: []string{"Role: x", " Task: y"}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ch := Inflate(ctx, p, harvester.ContextBundle{ProfileOK: true}, "fix bug")
	var sb strings.Builder
	for c := range ch {
		sb.WriteString(c)
	}
	if got := sb.String(); !strings.Contains(got, "Role:") {
		t.Errorf("got %q", got)
	}
}

func TestInflateCancellation(t *testing.T) {
	p := &fakeProvider{chunks: []string{"a", "b", "c"}, delay: 100 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	ch := Inflate(ctx, p, harvester.ContextBundle{}, "x")
	got := <-ch
	cancel()
	// drain — should close quickly after cancel
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				if got != "a" {
					t.Errorf("first chunk = %q, want 'a'", got)
				}
				return
			}
		case <-deadline:
			t.Fatal("channel not closed within 500ms after cancel")
		}
	}
}
