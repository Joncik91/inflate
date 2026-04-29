package provider

import (
	"fmt"
	"os"

	"github.com/Joncik91/inflate/internal/config"
)

// providerNeedsKey reports whether a provider kind requires an API key.
// Local providers like Ollama don't authenticate.
func providerNeedsKey(kind string) bool {
	switch kind {
	case "ollama":
		return false
	default:
		return true
	}
}

// NewFromConfig builds a Provider from a config. It resolves the API key
// from APIKey (inline) or APIKeyEnv (env var), erroring if neither is set —
// unless the chosen provider doesn't need one.
func NewFromConfig(c config.Config) (Provider, error) {
	var key string
	if providerNeedsKey(c.Provider.Kind) {
		key = c.Provider.APIKey
		if key == "" && c.Provider.APIKeyEnv != "" {
			key = os.Getenv(c.Provider.APIKeyEnv)
			if key == "" {
				return nil, fmt.Errorf("env var %q is empty (set it or use api_key)", c.Provider.APIKeyEnv)
			}
		}
		if key == "" {
			return nil, fmt.Errorf("no API key: set provider.api_key or provider.api_key_env")
		}
	}
	switch c.Provider.Kind {
	case "anthropic":
		return NewAnthropic(key, c.Provider.Model, ""), nil
	case "openai_compat":
		return NewOpenAICompat(key, c.Provider.Model, c.Provider.BaseURL), nil
	case "google":
		return NewGoogle(key, c.Provider.Model, ""), nil
	case "ollama":
		return NewOllama(c.Provider.Model, c.Provider.BaseURL), nil
	default:
		return nil, fmt.Errorf("unknown provider kind %q (try `inflate doctor` or `inflate config edit`)", c.Provider.Kind)
	}
}
