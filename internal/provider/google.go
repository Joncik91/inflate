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

// Google calls Gemini via the streamGenerateContent endpoint.
type Google struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func NewGoogle(apiKey, model, baseURL string) *Google {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &Google{apiKey: apiKey, model: model, baseURL: baseURL, http: &http.Client{}}
}

func (g *Google) Name() string { return "google:" + g.model }

func (g *Google) Validate(ctx context.Context) error {
	ch, err := g.Stream(ctx, Request{System: "ping", User: "hi", MaxTokens: 1})
	if err != nil {
		return err
	}
	for range ch {
	}
	return nil
}

type gemReq struct {
	SystemInstruction *gemContent      `json:"systemInstruction,omitempty"`
	Contents          []gemContent     `json:"contents"`
	GenerationConfig  *gemGenConfig    `json:"generationConfig,omitempty"`
}
type gemContent struct {
	Parts []gemPart `json:"parts"`
	Role  string    `json:"role,omitempty"`
}
type gemPart struct {
	Text string `json:"text"`
}
type gemGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

func (g *Google) Stream(ctx context.Context, req Request) (<-chan string, error) {
	body := gemReq{
		SystemInstruction: &gemContent{Parts: []gemPart{{Text: req.System}}},
		Contents:          []gemContent{{Role: "user", Parts: []gemPart{{Text: req.User}}}},
		GenerationConfig:  &gemGenConfig{MaxOutputTokens: req.MaxTokens},
	}
	buf, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", g.baseURL, g.model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("google %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			for _, c := range ev.Candidates {
				for _, p := range c.Content.Parts {
					if p.Text == "" {
						continue
					}
					select {
					case <-ctx.Done():
						return
					case out <- p.Text:
					}
				}
			}
		}
	}()
	return out, nil
}
