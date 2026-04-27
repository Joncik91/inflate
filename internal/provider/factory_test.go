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
