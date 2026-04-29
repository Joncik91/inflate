package tui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Joncik91/inflate/internal/config"
	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/intake"
)

// configHomeOverride redirects ConfigDir() to a temp path so SaveConfig
// in cycleProvider doesn't clobber the user's real ~/.config/inflate.
func configHomeOverride(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "inflate")
}

func stubTUIProbe(t *testing.T, models []intake.OllamaModel, ok bool) {
	t.Helper()
	prev := probeForTest
	probeForTest = func() ([]intake.OllamaModel, bool) { return models, ok }
	t.Cleanup(func() { probeForTest = prev })
}

// fakeOllamaServer returns a server that satisfies Ollama.Validate (ie
// answers /api/tags listing the named models). Used so the cycle's
// Validate roundtrip succeeds in tests.
func fakeOllamaServer(t *testing.T, modelNames ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		sb.WriteString(`{"models":[`)
		for i, n := range modelNames {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`{"name":"` + n + `"}`)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCycleProviderCloudToOllama(t *testing.T) {
	configHomeOverride(t)
	srv := fakeOllamaServer(t, "gemma4:26b")
	t.Cleanup(func() { ollamaURLForTest = "" })
	ollamaURLForTest = srv.URL
	stubTUIProbe(t, []intake.OllamaModel{
		{Name: "gemma4:26b", Family: "gemma4", ParameterSize: "26B"},
	}, true)

	cfg := config.Config{
		AutoPaste: false,
		Provider: config.ProviderConfig{
			Kind:      "openai_compat",
			BaseURL:   "https://api.deepseek.com/v1",
			Model:     "deepseek-chat",
			APIKeyEnv: "DEEPSEEK_API_KEY",
		},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)

	got, _ := m.cycleProvider()
	gm := got.(Model)
	if gm.cfg.Provider.Kind != "ollama" {
		t.Errorf("expected kind=ollama after first cycle, got %q", gm.cfg.Provider.Kind)
	}
	if gm.cfg.Provider.Model != "gemma4:26b" {
		t.Errorf("expected model=gemma4:26b, got %q", gm.cfg.Provider.Model)
	}
	if gm.originalProviderCfg.Kind != "openai_compat" {
		t.Errorf("original config not preserved, got kind=%q", gm.originalProviderCfg.Kind)
	}
	if !strings.Contains(gm.toast, "switched to ollama:gemma4:26b") {
		t.Errorf("expected toast about switch, got: %q", gm.toast)
	}
	if gm.helpOpen {
		t.Errorf("help should auto-close after switch")
	}
}

func TestCycleProviderWrapsBackToOriginal(t *testing.T) {
	configHomeOverride(t)
	srv := fakeOllamaServer(t, "gemma4:26b")
	t.Cleanup(func() { ollamaURLForTest = "" })
	ollamaURLForTest = srv.URL
	stubTUIProbe(t, []intake.OllamaModel{
		{Name: "gemma4:26b", Family: "gemma4", ParameterSize: "26B"},
	}, true)

	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:    "openai_compat",
			BaseURL: "https://api.deepseek.com/v1",
			Model:   "deepseek-chat",
		},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)

	// First press: cloud → ollama
	got, _ := m.cycleProvider()
	gm := got.(Model)
	if gm.cfg.Provider.Kind != "ollama" {
		t.Fatalf("step 1: expected ollama, got %q", gm.cfg.Provider.Kind)
	}

	// Second press: ollama → cloud (wrap-around)
	got2, _ := gm.cycleProvider()
	gm2 := got2.(Model)
	if gm2.cfg.Provider.Kind != "openai_compat" {
		t.Errorf("step 2: expected back to openai_compat, got %q", gm2.cfg.Provider.Kind)
	}
	if gm2.cfg.Provider.Model != "deepseek-chat" {
		t.Errorf("step 2: expected deepseek-chat, got %q", gm2.cfg.Provider.Model)
	}
}

func TestCycleProviderWalksMultipleOllamaModels(t *testing.T) {
	configHomeOverride(t)
	srv := fakeOllamaServer(t, "gemma4:26b", "qwen3.6:35b")
	t.Cleanup(func() { ollamaURLForTest = "" })
	ollamaURLForTest = srv.URL
	stubTUIProbe(t, []intake.OllamaModel{
		{Name: "qwen3.6:35b", Family: "qwen35moe", ParameterSize: "36B"},
		{Name: "gemma4:26b", Family: "gemma4", ParameterSize: "26B"},
	}, true)

	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:    "openai_compat",
			BaseURL: "https://api.deepseek.com/v1",
			Model:   "deepseek-chat",
		},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)

	// Press 1: original → smallest ollama (gemma4:26b)
	got, _ := m.cycleProvider()
	gm := got.(Model)
	if gm.cfg.Provider.Model != "gemma4:26b" {
		t.Fatalf("step 1: expected gemma4:26b (smallest), got %q", gm.cfg.Provider.Model)
	}

	// Press 2: gemma → next ollama (qwen3.6:35b)
	got2, _ := gm.cycleProvider()
	gm2 := got2.(Model)
	if gm2.cfg.Provider.Model != "qwen3.6:35b" {
		t.Errorf("step 2: expected qwen3.6:35b, got %q", gm2.cfg.Provider.Model)
	}

	// Press 3: qwen → wrap to original (deepseek)
	got3, _ := gm2.cycleProvider()
	gm3 := got3.(Model)
	if gm3.cfg.Provider.Model != "deepseek-chat" {
		t.Errorf("step 3: expected wrap to deepseek-chat, got %q", gm3.cfg.Provider.Model)
	}
}

func TestCycleProviderOriginalIsOllamaAndOnlyOneModel(t *testing.T) {
	configHomeOverride(t)
	stubTUIProbe(t, []intake.OllamaModel{
		{Name: "gemma4:26b", Family: "gemma4", ParameterSize: "26B"},
	}, true)

	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:    "ollama",
			Model:   "gemma4:26b",
			BaseURL: "http://localhost:11434",
		},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)

	got, _ := m.cycleProvider()
	gm := got.(Model)
	if !strings.Contains(gm.toast, "no other providers") {
		t.Errorf("expected 'no other providers' toast, got: %q", gm.toast)
	}
}

func TestCycleProviderOllamaUnreachable(t *testing.T) {
	configHomeOverride(t)
	stubTUIProbe(t, nil, false)

	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:      "anthropic",
			Model:     "claude-haiku-4-5",
			APIKeyEnv: "ANTHROPIC_API_KEY",
		},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)

	got, _ := m.cycleProvider()
	gm := got.(Model)
	if !strings.Contains(gm.toast, "no other providers") {
		t.Errorf("expected 'no other providers' toast, got: %q", gm.toast)
	}
	if gm.cfg.Provider.Kind != "anthropic" {
		t.Errorf("provider should not have changed, got %q", gm.cfg.Provider.Kind)
	}
}

func TestPKeyCyclesProviderInHelpOverlay(t *testing.T) {
	configHomeOverride(t)
	stubTUIProbe(t, nil, false) // Ollama not detected — toast only

	cfg := config.Config{
		Provider: config.ProviderConfig{Kind: "anthropic", Model: "haiku"},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)
	m.helpOpen = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	gm := model.(Model)
	if gm.toast == "" {
		t.Error("expected toast after pressing p in help overlay")
	}

	// Without help open, `p` should NOT trigger cycle — it appends to seed.
	m2 := New(stubProvider{}, h, cfg, 0)
	model2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	gm2 := model2.(Model)
	if gm2.seed != "p" {
		t.Errorf("p outside help should append to seed, got seed=%q", gm2.seed)
	}
}

func TestReachableOllamaIsExported(t *testing.T) {
	if reachableOllama("http://127.0.0.1:1") {
		t.Error("expected unreachable to return false")
	}
}

func TestPickSmallestModel(t *testing.T) {
	got := pickSmallestModel([]intake.OllamaModel{
		{Name: "qwen3.6:35b", ParameterSize: "36.0B"},
		{Name: "gemma4:26b", ParameterSize: "25.8B"},
		{Name: "llama3:8b", ParameterSize: "7B"},
	})
	if got != "llama3:8b" {
		t.Errorf("smallest = %q, want llama3:8b", got)
	}
}

func TestPickSmallestModelHandlesMissingSizes(t *testing.T) {
	got := pickSmallestModel([]intake.OllamaModel{
		{Name: "fallback:latest", ParameterSize: ""},
		{Name: "real:7b", ParameterSize: "7B"},
	})
	if got != "real:7b" {
		t.Errorf("should prefer model with parseable size, got %q", got)
	}
}

func TestSortBySize(t *testing.T) {
	models := []intake.OllamaModel{
		{Name: "qwen3.6:35b", ParameterSize: "36.0B"},
		{Name: "gemma4:26b", ParameterSize: "25.8B"},
		{Name: "llama3:8b", ParameterSize: "7B"},
		{Name: "mystery:latest", ParameterSize: ""},
	}
	sortBySize(models)
	if models[0].Name != "llama3:8b" {
		t.Errorf("smallest first; got %q at index 0", models[0].Name)
	}
	if models[1].Name != "gemma4:26b" {
		t.Errorf("got %q at index 1", models[1].Name)
	}
	if models[2].Name != "qwen3.6:35b" {
		t.Errorf("got %q at index 2", models[2].Name)
	}
	if models[3].Name != "mystery:latest" {
		t.Errorf("unparseable size should sort last; got %q at index 3", models[3].Name)
	}
}

func TestParseParamSize(t *testing.T) {
	cases := map[string]int{
		"7B":    7000,
		"36.0B": 36000,
		"26B":   26000,
		"137M":  137,
		"":      0,
		"junk":  0,
	}
	for in, want := range cases {
		if got := parseParamSize(in); got != want {
			t.Errorf("parseParamSize(%q) = %d, want %d", in, got, want)
		}
	}
}
