package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropicStreamHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(event, data string) {
			w.Write([]byte("event: " + event + "\n"))
			w.Write([]byte("data: " + data + "\n\n"))
			flusher.Flush()
		}
		writeSSE("message_start", `{"type":"message_start","message":{"id":"x","type":"message","role":"assistant","content":[],"model":"haiku","usage":{"input_tokens":1,"output_tokens":0}}}`)
		writeSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}`)
		writeSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}`)
		writeSSE("content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`)
		writeSSE("message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", "claude-haiku", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := p.Stream(ctx, Request{System: "s", User: "u", MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk)
	}
	if got := sb.String(); got != "Hello world" {
		t.Errorf("stream output = %q, want %q", got, "Hello world")
	}
}

func TestAnthropicAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid"}}`))
	}))
	defer srv.Close()

	p := NewAnthropic("bad-key", "claude-haiku", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Validate(ctx); err == nil {
		t.Error("expected Validate to fail with 401")
	}
}
