package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Ollama speaks to a local Ollama daemon via the native /api/chat NDJSON
// stream. We deliberately avoid /v1/chat/completions (the OpenAI-compat
// shim) because it doesn't expose Ollama's `think` toggle — and reasoning
// models like gemma4 burn the entire output budget on hidden reasoning
// tokens otherwise. Validate hits /api/tags so the configured model is
// verified to be pulled without warming it up.
type Ollama struct {
	model   string
	baseURL string // e.g. "http://localhost:11434"
	http    *http.Client
}

// NewOllama constructs a provider against a local Ollama daemon.
// baseURL is the daemon root ("http://localhost:11434"); leave empty for the
// default.
func NewOllama(model, baseURL string) *Ollama {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Ollama{
		model:   model,
		baseURL: baseURL,
		http:    &http.Client{},
	}
}

func (o *Ollama) Name() string { return "ollama:" + o.model }

type ollamaChatReq struct {
	Model    string              `json:"model"`
	Stream   bool                `json:"stream"`
	Think    bool                `json:"think"`
	Messages []ollamaChatMessage `json:"messages"`
	Options  ollamaChatOptions   `json:"options"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type ollamaChatChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// Stream issues a /api/chat request and emits each NDJSON line's content
// chunk on the returned channel. Closes when `done: true` arrives or ctx
// is cancelled. Setup errors return immediately; mid-stream parse errors
// are skipped silently to match the other providers' contract.
func (o *Ollama) Stream(ctx context.Context, req Request) (<-chan string, error) {
	body := ollamaChatReq{
		Model:  o.model,
		Stream: true,
		// Disable reasoning. Inflate wants a direct prompt expansion; thinking
		// tokens drain the output budget without producing visible content on
		// reasoning models like gemma4.
		Think: false,
		Messages: []ollamaChatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Options: ollamaChatOptions{NumPredict: req.MaxTokens},
	}
	buf, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out := make(chan string)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk ollamaChatChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			if chunk.Message.Content != "" {
				select {
				case <-ctx.Done():
					return
				case out <- chunk.Message.Content:
				}
			}
			if chunk.Done {
				return
			}
		}
	}()
	return out, nil
}

// Validate hits /api/tags (instant) and confirms the configured model is pulled.
// Faster than a 1-token chat (no model load) and surfaces the two real
// failure modes — daemon down vs. model not pulled — with actionable hints.
func (o *Ollama) Validate(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	cli := &http.Client{Timeout: 5 * time.Second}
	resp, err := cli.Do(httpReq)
	if err != nil {
		return fmt.Errorf("Ollama not reachable on %s — is it running? (`ollama serve`)", o.baseURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Ollama %d on /api/tags", resp.StatusCode)
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("Ollama /api/tags: %v", err)
	}
	for _, m := range tags.Models {
		if m.Name == o.model {
			return nil
		}
	}
	return fmt.Errorf("model %q not pulled — run: ollama pull %s", o.model, o.model)
}
