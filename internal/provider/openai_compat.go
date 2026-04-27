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

// OpenAICompat speaks the Chat Completions streaming protocol used by OpenAI,
// DeepSeek, Groq, Together, OpenRouter, vLLM, llama.cpp's server, etc.
type OpenAICompat struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func NewOpenAICompat(apiKey, model, baseURL string) *OpenAICompat {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAICompat{apiKey: apiKey, model: model, baseURL: baseURL, http: &http.Client{}}
}

func (o *OpenAICompat) Name() string { return "openai_compat:" + o.model }

func (o *OpenAICompat) Validate(ctx context.Context) error {
	ch, err := o.Stream(ctx, Request{System: "ping", User: "hi", MaxTokens: 1})
	if err != nil {
		return err
	}
	for range ch {
	}
	return nil
}

type openAIReq struct {
	Model     string              `json:"model"`
	Stream    bool                `json:"stream"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
	Messages  []openAIChatMessage `json:"messages"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (o *OpenAICompat) Stream(ctx context.Context, req Request) (<-chan string, error) {
	body := openAIReq{
		Model:     o.model,
		Stream:    true,
		MaxTokens: req.MaxTokens,
		Messages: []openAIChatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
	}
	buf, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai_compat %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
			if payload == "[DONE]" {
				return
			}
			var ev struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			for _, c := range ev.Choices {
				if c.Delta.Content == "" {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- c.Delta.Content:
				}
			}
		}
	}()
	return out, nil
}
