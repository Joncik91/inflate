package intake

import (
	"strings"
	"testing"

	"github.com/Joncik91/inflate/internal/config"
)

// stubProbe pins probeOllama to a known result so the existing tests don't
// depend on whether a real Ollama happens to be running on the test machine.
func stubProbe(t *testing.T, models []OllamaModel, ok bool) {
	t.Helper()
	prev := probeOllama
	probeOllama = func(string) ([]OllamaModel, bool) { return models, ok }
	t.Cleanup(func() { probeOllama = prev })
}

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
	stubProbe(t, nil, false)
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
	stubProbe(t, nil, false)
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
	stubProbe(t, nil, false)
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

func TestRunFullSetupOllamaByIndex(t *testing.T) {
	stubProbe(t, []OllamaModel{
		{Name: "gemma4:26b", ParameterSize: "26B", Quantization: "Q4_K_M", Family: "gemma4"},
		{Name: "qwen3.6:35b", ParameterSize: "36B", Quantization: "Q4_K_M", Family: "qwen35moe"},
	}, true)

	in := strings.NewReader("dev\nstuff\nstandard\nl\n1\n")
	keys := &stubKeyReader{key: "should-not-be-asked"}
	var out strings.Builder
	r, err := RunFullSetup(in, &out, keys)
	if err != nil {
		t.Fatal(err)
	}
	if r.Provider.Kind != "ollama" {
		t.Errorf("kind = %q, want ollama", r.Provider.Kind)
	}
	if r.Provider.Model != "gemma4:26b" {
		t.Errorf("model = %q, want gemma4:26b", r.Provider.Model)
	}
	if r.APIKeyName != "" {
		t.Errorf("Ollama path must skip API key, got APIKeyName = %q", r.APIKeyName)
	}
	if r.APIKeyValue != "" {
		t.Errorf("Ollama path must skip API key, got APIKeyValue = %q", r.APIKeyValue)
	}
	if !strings.Contains(out.String(), "[l]ocal Ollama (2 models found)") {
		t.Errorf("expected menu to advertise Ollama; got: %s", out.String())
	}
}

func TestRunFullSetupOllamaByName(t *testing.T) {
	stubProbe(t, []OllamaModel{
		{Name: "gemma4:26b", ParameterSize: "26B", Quantization: "Q4_K_M", Family: "gemma4"},
	}, true)

	in := strings.NewReader("dev\nstuff\nstandard\nl\ngemma4:26b\n")
	var out strings.Builder
	r, err := RunFullSetup(in, &out, &stubKeyReader{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Provider.Model != "gemma4:26b" {
		t.Errorf("model = %q", r.Provider.Model)
	}
}

func TestRunFullSetupOllamaNotDetected(t *testing.T) {
	stubProbe(t, nil, false)
	in := strings.NewReader("dev\nstuff\nstandard\nl\n")
	var out strings.Builder
	_, err := RunFullSetup(in, &out, &stubKeyReader{})
	if err == nil {
		t.Fatal("expected error when user picks Ollama but probe failed")
	}
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("error should mention `ollama serve`, got: %v", err)
	}
}

func TestRunProviderOnlyOllama(t *testing.T) {
	stubProbe(t, []OllamaModel{
		{Name: "gemma4:26b", ParameterSize: "26B", Quantization: "Q4_K_M", Family: "gemma4"},
	}, true)

	in := strings.NewReader("l\n1\n")
	var out strings.Builder
	prov, keyName, keyVal, err := RunProviderOnly(in, &out, &stubKeyReader{})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Kind != "ollama" || prov.Model != "gemma4:26b" {
		t.Errorf("got %+v", prov)
	}
	if keyName != "" || keyVal != "" {
		t.Errorf("ollama path must skip key, got name=%q value=%q", keyName, keyVal)
	}
}

func TestRunProviderOnlyAnthropic(t *testing.T) {
	stubProbe(t, nil, false)

	in := strings.NewReader("a\nclaude-haiku-4-5\n")
	var out strings.Builder
	prov, keyName, keyVal, err := RunProviderOnly(in, &out, &stubKeyReader{key: "sk-ant"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Kind != "anthropic" {
		t.Errorf("kind = %q", prov.Kind)
	}
	if keyName != "ANTHROPIC_API_KEY" || keyVal != "sk-ant" {
		t.Errorf("key info wrong: name=%q value=%q", keyName, keyVal)
	}
}

func TestResolveOllamaPickInvalid(t *testing.T) {
	models := []OllamaModel{{Name: "gemma4:26b"}}
	if _, err := resolveOllamaPick("99", models); err == nil {
		t.Error("expected error for out-of-range index")
	}
	if _, err := resolveOllamaPick("notamodel", models); err == nil {
		t.Error("expected error for unknown name")
	}
	if _, err := resolveOllamaPick("", models); err == nil {
		t.Error("expected error for empty input")
	}
}
