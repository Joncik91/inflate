package intake

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// OllamaModel describes a chat-capable model installed in a local Ollama.
type OllamaModel struct {
	Name          string // e.g. "gemma4:26b"
	ParameterSize string // e.g. "26B"
	Quantization  string // e.g. "Q4_K_M"
	Family        string // e.g. "gemma4"
}

type ollamaTagsResp struct {
	Models []ollamaTagsEntry `json:"models"`
}

type ollamaTagsEntry struct {
	Name    string `json:"name"`
	Details struct {
		Family           string `json:"family"`
		ParameterSize    string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	} `json:"details"`
}

// embeddingFamilies are model families that only produce embeddings, not
// chat completions. Filtered out of the wizard's model picker.
var embeddingFamilies = map[string]bool{
	"nomic-bert": true,
	"bge-m3":     true,
	"all-minilm": true,
	"mxbai":      true,
}

// probeOllama is the seam tests can override. Real callers go through
// ProbeOllama (the public API).
var probeOllama = ProbeOllama

// ProbeOllama queries a local Ollama daemon and returns chat-capable models.
// Returns (nil, false) if the daemon is unreachable, returns 0 chat-capable
// models, or fails to respond within 500ms — the wizard must stay snappy.
// Pass an empty baseURL for the default localhost:11434.
func ProbeOllama(baseURL string) ([]OllamaModel, bool) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, false
	}

	var tags ollamaTagsResp
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, false
	}

	out := make([]OllamaModel, 0, len(tags.Models))
	for _, m := range tags.Models {
		if embeddingFamilies[m.Details.Family] {
			continue
		}
		out = append(out, OllamaModel{
			Name:          m.Name,
			ParameterSize: m.Details.ParameterSize,
			Quantization:  m.Details.QuantizationLevel,
			Family:        m.Details.Family,
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
