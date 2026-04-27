package intake

import (
	"strings"
	"testing"

	"github.com/Joncik91/inflate/internal/config"
)

func TestRunFromReader(t *testing.T) {
	in := strings.NewReader("senior backend engineer\nAPI services\nterse\n")
	var out strings.Builder
	p, err := RunFromReader(in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Identity != "senior backend engineer" {
		t.Errorf("identity = %q", p.Identity)
	}
	if p.Style != "terse" {
		t.Errorf("style = %q", p.Style)
	}
}

func TestRunFromReaderNormalisesStyle(t *testing.T) {
	in := strings.NewReader("dev\nstuff\nGRANDMASTER\n")
	var out strings.Builder
	p, _ := RunFromReader(in, &out)
	if p.Style != "verbose" {
		t.Errorf("expected GRANDMASTER -> verbose, got %q", p.Style)
	}
	_ = config.Profile{} // import sanity
}

func TestRunFullSetupAnthropic(t *testing.T) {
	in := strings.NewReader("dev\nstuff\nstandard\na\nclaude-haiku-4-5\n")
	keys := &stubKeyReader{key: "sk-ant-test"}
	var out strings.Builder
	r, err := RunFullSetup(in, &out, keys)
	if err != nil {
		t.Fatal(err)
	}
	if r.Profile.Identity != "dev" {
		t.Errorf("identity = %q", r.Profile.Identity)
	}
	if r.Provider.Kind != "anthropic" {
		t.Errorf("kind = %q, want anthropic", r.Provider.Kind)
	}
	if r.Provider.Model != "claude-haiku-4-5" {
		t.Errorf("model = %q", r.Provider.Model)
	}
	if r.Provider.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("api_key_env = %q", r.Provider.APIKeyEnv)
	}
	if r.APIKeyName != "ANTHROPIC_API_KEY" {
		t.Errorf("APIKeyName = %q", r.APIKeyName)
	}
	if r.APIKeyValue != "sk-ant-test" {
		t.Errorf("APIKeyValue = %q", r.APIKeyValue)
	}
}

func TestRunFullSetupDeepseek(t *testing.T) {
	in := strings.NewReader("dev\nstuff\nstandard\nd\ndeepseek-chat\n")
	keys := &stubKeyReader{key: "sk-ds"}
	var out strings.Builder
	r, err := RunFullSetup(in, &out, keys)
	if err != nil {
		t.Fatal(err)
	}
	if r.Provider.Kind != "openai_compat" {
		t.Errorf("kind = %q, want openai_compat", r.Provider.Kind)
	}
	if r.Provider.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("base_url = %q", r.Provider.BaseURL)
	}
	if r.APIKeyName != "DEEPSEEK_API_KEY" {
		t.Errorf("APIKeyName = %q", r.APIKeyName)
	}
}

func TestRunFullSetupCustom(t *testing.T) {
	in := strings.NewReader("dev\nstuff\nstandard\nc\nhttps://my.local:8000/v1\nllama-3\nMY_LOCAL_KEY\n")
	keys := &stubKeyReader{key: "anything"}
	var out strings.Builder
	r, err := RunFullSetup(in, &out, keys)
	if err != nil {
		t.Fatal(err)
	}
	if r.Provider.Kind != "openai_compat" {
		t.Errorf("kind = %q", r.Provider.Kind)
	}
	if r.Provider.BaseURL != "https://my.local:8000/v1" {
		t.Errorf("base_url = %q", r.Provider.BaseURL)
	}
	if r.APIKeyName != "MY_LOCAL_KEY" {
		t.Errorf("APIKeyName = %q", r.APIKeyName)
	}
}

type stubKeyReader struct {
	key string
	err error
}

func (s *stubKeyReader) ReadKey(prompt string) (string, error) {
	return s.key, s.err
}
