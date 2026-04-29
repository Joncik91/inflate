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

// configHomeOverride redirects ConfigDir() to a temp path so the toggle's
// SaveConfig call doesn't clobber the user's real ~/.config/inflate. The
// XDG_CONFIG_HOME env var is honored by config.ConfigDir.
func configHomeOverride(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "inflate")
}

func stubTUIProbe(t *testing.T, models []intake.OllamaModel, ok bool) {
	t.Helper()
	// We need to override the *intake* package's probe used by our toggle.
	// Since intake exposes ProbeOllama as a function var indirectly via
	// probeOllama (intake-internal), we override by spinning up a real
	// httptest server and pointing toggleProvider's ProbeOllama there.
	// But ProbeOllama is called with "" → localhost:11434, not configurable.
	// Easier: set up a fake Ollama on localhost dynamically isn't safe in tests.
	// So we expose a per-test override on the function variable.
	prev := probeForTest
	probeForTest = func() ([]intake.OllamaModel, bool) { return models, ok }
	t.Cleanup(func() { probeForTest = prev })
}

func TestToggleProviderCloudToOllama(t *testing.T) {
	configHomeOverride(t)

	// Mock Ollama daemon for the Validate roundtrip.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:26b","details":{"family":"gemma4","parameter_size":"26B","quantization_level":"Q4_K_M"}}]}`))
	}))
	defer srv.Close()
	t.Setenv("OLLAMA_TEST_URL", srv.URL)

	stubTUIProbe(t, []intake.OllamaModel{
		{Name: "gemma4:26b", Family: "gemma4"},
	}, true)
	t.Cleanup(func() { ollamaURLForTest = "" })
	ollamaURLForTest = srv.URL

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

	got, _ := m.toggleProvider()
	gm := got.(Model)
	if gm.cfg.Provider.Kind != "ollama" {
		t.Errorf("expected kind=ollama, got %q", gm.cfg.Provider.Kind)
	}
	if gm.cfg.Provider.Model != "gemma4:26b" {
		t.Errorf("expected model=gemma4:26b, got %q", gm.cfg.Provider.Model)
	}
	if gm.previousProviderCfg.Kind != "openai_compat" {
		t.Errorf("previous provider not remembered, got kind=%q", gm.previousProviderCfg.Kind)
	}
	if !strings.Contains(gm.toast, "switched to ollama:gemma4:26b") {
		t.Errorf("expected toast about switch, got: %q", gm.toast)
	}
	if gm.helpOpen {
		t.Errorf("help should auto-close after switch")
	}

	// Verify config was saved to disk.
	saved, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if saved.Provider.Kind != "ollama" {
		t.Errorf("saved config kind=%q, want ollama", saved.Provider.Kind)
	}
}

func TestToggleProviderOllamaWithoutPrevious(t *testing.T) {
	configHomeOverride(t)
	cfg := config.Config{
		Provider: config.ProviderConfig{
			Kind:    "ollama",
			Model:   "gemma4:26b",
			BaseURL: "http://localhost:11434",
		},
	}
	h, _ := harvester.New(harvester.Options{ProjectDir: t.TempDir()})
	m := New(stubProvider{}, h, cfg, 0)
	// previousProvider is nil — booted with Ollama already configured.

	got, _ := m.toggleProvider()
	gm := got.(Model)
	if !strings.Contains(gm.toast, "no previous provider") {
		t.Errorf("expected hint about config provider, got: %q", gm.toast)
	}
	if gm.cfg.Provider.Kind != "ollama" {
		t.Errorf("provider should not have changed, got %q", gm.cfg.Provider.Kind)
	}
}

func TestToggleProviderOllamaUnreachable(t *testing.T) {
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

	got, _ := m.toggleProvider()
	gm := got.(Model)
	if !strings.Contains(gm.toast, "Ollama not detected") {
		t.Errorf("expected 'not detected' toast, got: %q", gm.toast)
	}
	if gm.cfg.Provider.Kind != "anthropic" {
		t.Errorf("provider should not have changed, got %q", gm.cfg.Provider.Kind)
	}
}

func TestPKeyTogglesProviderInHelpOverlay(t *testing.T) {
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

	// Without help open, `p` should NOT trigger toggle — it appends to seed.
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
