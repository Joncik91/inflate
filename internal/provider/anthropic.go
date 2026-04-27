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
)

// Anthropic is a streaming client for the Messages API.
type Anthropic struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// NewAnthropic builds a provider. baseURL defaults to https://api.anthropic.com.
func NewAnthropic(apiKey, model, baseURL string) *Anthropic {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Anthropic{apiKey: apiKey, model: model, baseURL: baseURL, http: &http.Client{}}
}

func (a *Anthropic) Name() string { return "anthropic:" + a.model }

func (a *Anthropic) Validate(ctx context.Context) error {
	ch, err := a.Stream(ctx, Request{System: "ping", User: "hi", MaxTokens: 1})
	if err != nil {
		return err
	}
	for range ch { // drain
	}
	return nil
}

type anthropicReq struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system,omitempty"`
	Stream    bool   `json:"stream"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func (a *Anthropic) Stream(ctx context.Context, req Request) (<-chan string, error) {
	body := anthropicReq{
		Model:     a.model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Stream:    true,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: req.User}},
	}
	buf, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out := make(chan string)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			var ev struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" {
				select {
				case <-ctx.Done():
					return
				case out <- ev.Delta.Text:
				}
			}
		}
	}()
	return out, nil
}
