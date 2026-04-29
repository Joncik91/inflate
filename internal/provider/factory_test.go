package provider

import (
	"testing"

	"github.com/Joncik91/inflate/internal/config"
)

func TestNewFromConfigEnvKey(t *testing.T) {
	t.Setenv("MY_KEY", "secret")
	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:      "anthropic",
			Model:     "claude-haiku",
			APIKeyEnv: "MY_KEY",
		},
	}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "anthropic:claude-haiku" {
		t.Errorf("got %s", p.Name())
	}
}

func TestNewFromConfigMissingKey(t *testing.T) {
	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:      "anthropic",
			Model:     "claude-haiku",
			APIKeyEnv: "DOES_NOT_EXIST_XYZ",
		},
	}
	_, err := NewFromConfig(cfg)
	if err == nil {
		t.Error("expected error when env var missing")
	}
}

func TestNewFromConfigUnknownKind(t *testing.T) {
	cfg := config.Config{
		Provider: config.ProviderConfig{Kind: "mystery", APIKey: "x"},
	}
	_, err := NewFromConfig(cfg)
	if err == nil {
		t.Error("expected error for unknown provider kind")
	}
}

func TestNewFromConfigOllamaNoKey(t *testing.T) {
	// Ollama is local and doesn't need an API key — the factory must skip
	// the empty-key check for kind=ollama.
	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:    "ollama",
			Model:   "gemma4:26b",
			BaseURL: "http://localhost:11434",
		},
	}
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("Ollama config without key should succeed, got: %v", err)
	}
	if p.Name() != "ollama:gemma4:26b" {
		t.Errorf("Name = %q, want ollama:gemma4:26b", p.Name())
	}
}
