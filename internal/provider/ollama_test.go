package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeOllama returns a server that mocks /api/tags with the given model names
// and emits a deterministic two-chunk NDJSON response on /api/chat.
func fakeOllama(t *testing.T, models ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var sb strings.Builder
		sb.WriteString(`{"models":[`)
		for i, m := range models {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`{"name":"` + m + `","model":"` + m + `","details":{"family":"gemma4","parameter_size":"26B","quantization_level":"Q4_K_M"}}`)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		emit := func(content string, done bool) {
			line, _ := json.Marshal(map[string]any{
				"model":   "gemma4:26b",
				"message": map[string]string{"role": "assistant", "content": content},
				"done":    done,
			})
			_, _ = w.Write(append(line, '\n'))
			flusher.Flush()
		}
		emit("hello", false)
		emit(" world", false)
		emit("", true)
	})
	return httptest.NewServer(mux)
}

func TestOllamaValidateHappy(t *testing.T) {
	srv := fakeOllama(t, "gemma4:26b", "qwen3.6:35b")
	defer srv.Close()

	p := NewOllama("gemma4:26b", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Validate(ctx); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestOllamaValidateModelNotPulled(t *testing.T) {
	srv := fakeOllama(t, "gemma4:26b")
	defer srv.Close()

	p := NewOllama("llama3:99b", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.Validate(ctx)
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "ollama pull llama3:99b") {
		t.Errorf("error should suggest the pull command, got: %v", err)
	}
}

func TestOllamaValidateDaemonDown(t *testing.T) {
	// Use a port nothing listens on — Validate should report it as unreachable.
	p := NewOllama("gemma4:26b", "http://127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.Validate(ctx)
	if err == nil {
		t.Fatal("expected error when daemon unreachable")
	}
	if !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("error should mention reachability, got: %v", err)
	}
}

func TestOllamaStream(t *testing.T) {
	srv := fakeOllama(t, "gemma4:26b")
	defer srv.Close()

	p := NewOllama("gemma4:26b", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := p.Stream(ctx, Request{System: "s", User: "u", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for c := range ch {
		sb.WriteString(c)
	}
	if got := sb.String(); got != "hello world" {
		t.Errorf("stream = %q, want %q", got, "hello world")
	}
}

// TestOllamaStreamSendsThinkFalse confirms the request body disables Ollama's
// reasoning mode. Without this, gemma4 burns the entire output budget on
// hidden reasoning tokens and returns an empty content stream — which is
// how this bug was originally caught against a real local daemon.
func TestOllamaStreamSendsThinkFalse(t *testing.T) {
	var captured map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"message":{"content":"x"},"done":true}` + "\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewOllama("gemma4:26b", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, _ := p.Stream(ctx, Request{System: "s", User: "u", MaxTokens: 8})
	for range ch {
	}

	if think, ok := captured["think"].(bool); !ok || think {
		t.Errorf("expected think=false in payload, got: %v", captured["think"])
	}
}

func TestOllamaName(t *testing.T) {
	p := NewOllama("gemma4:26b", "")
	if p.Name() != "ollama:gemma4:26b" {
		t.Errorf("Name = %q, want %q", p.Name(), "ollama:gemma4:26b")
	}
}
