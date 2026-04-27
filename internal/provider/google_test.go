package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGoogleStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		send := func(s string) {
			w.Write([]byte("data: " + s + "\n\n"))
			flusher.Flush()
		}
		send(`{"candidates":[{"content":{"parts":[{"text":"Hello "}]}}]}`)
		send(`{"candidates":[{"content":{"parts":[{"text":"world"}]}}]}`)
	}))
	defer srv.Close()

	p := NewGoogle("test-key", "gemini-flash", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := p.Stream(ctx, Request{System: "s", User: "u", MaxTokens: 50})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for c := range ch {
		sb.WriteString(c)
	}
	if got := sb.String(); got != "Hello world" {
		t.Errorf("stream = %q, want %q", got, "Hello world")
	}
}
