package tui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Joncik91/inflate/internal/config"
	"github.com/Joncik91/inflate/internal/intake"
	"github.com/Joncik91/inflate/internal/provider"
)

// probeForTest and ollamaURLForTest are seams for unit tests. Production
// goes through intake.ProbeOllama against localhost:11434.
var (
	probeForTest     func() ([]intake.OllamaModel, bool)
	ollamaURLForTest string
)

// cycleEntry is one stop on the `p`-key carousel.
type cycleEntry struct {
	cfg      config.ProviderConfig
	provider provider.Provider // pre-built when known; nil for ollama models we build on demand
}

// providerCycle returns the ordered list of providers `p` rotates through.
// The first entry is whatever was originally in config.toml at boot
// (anchor for "go back to cloud"). Subsequent entries are each Ollama
// chat-capable model the probe finds, sorted smallest → largest by
// parameter count so the lighter model comes first.
//
// If the original was Ollama and only one model is pulled, the list has
// just that one entry and pressing `p` is a no-op (toast says so).
func (m Model) providerCycle() []cycleEntry {
	out := []cycleEntry{
		{cfg: m.originalProviderCfg, provider: m.originalProvider},
	}

	var (
		models []intake.OllamaModel
		ok     bool
	)
	if probeForTest != nil {
		models, ok = probeForTest()
	} else {
		models, ok = intake.ProbeOllama("")
	}
	if !ok {
		return out
	}

	// Sort by parameter size, smallest first.
	sortBySize(models)

	baseURL := ollamaURLForTest
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	for _, mdl := range models {
		oc := config.ProviderConfig{
			Kind:    "ollama",
			Model:   mdl.Name,
			BaseURL: baseURL,
		}
		// If the original config matches this exact ollama model, skip —
		// we already have it as entry 0 and we don't want a duplicate stop.
		if m.originalProviderCfg.Kind == "ollama" && m.originalProviderCfg.Model == mdl.Name {
			continue
		}
		out = append(out, cycleEntry{cfg: oc})
	}
	return out
}

// cycleProvider advances to the next entry on the cycle. Pressing `p`
// repeatedly walks: original (e.g. DeepSeek) → ollama:gemma4:26b →
// ollama:qwen3.6:35b → original → ... Saves to disk and toasts the result.
//
// Cancels any in-flight inflation before swapping.
func (m Model) cycleProvider() (tea.Model, tea.Cmd) {
	if m.cancelInflight != nil {
		m.cancelInflight()
		m.cancelInflight = nil
	}
	m.inflating = false

	cycle := m.providerCycle()
	if len(cycle) <= 1 {
		m.toast = "no other providers detected — start `ollama serve` or use `inflate config provider`"
		return m, clearToastAfter(toastDuration)
	}

	// Find the current entry's position. Match by Kind+Model+BaseURL —
	// good enough since we built the cycle ourselves.
	curIdx := 0
	for i, e := range cycle {
		if e.cfg == m.cfg.Provider {
			curIdx = i
			break
		}
	}
	nextIdx := (curIdx + 1) % len(cycle)
	next := cycle[nextIdx]

	// Build the new provider if it's an Ollama entry (we don't pre-build
	// these to avoid hitting /api/tags for every model on startup).
	newProv := next.provider
	if newProv == nil {
		newProv = provider.NewOllama(next.cfg.Model, next.cfg.BaseURL)
		// Verify the daemon answers /api/tags before swapping. Probe was
		// best-effort; this catches the daemon-just-died race.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := newProv.Validate(ctx); err != nil {
			cancel()
			m.toast = "ollama: " + err.Error()
			return m, clearToastAfter(toastDuration)
		}
		cancel()
	}

	m.cfg.Provider = next.cfg
	if err := config.SaveConfig(m.cfg); err != nil {
		m.errBanner = "save config: " + err.Error()
		return m, nil
	}
	m.provider = newProv
	m.helpOpen = false

	suffix := ""
	if next.cfg.Kind == "ollama" {
		suffix = " — first inflate may take 30-60s to load model"
	}
	m.toast = fmt.Sprintf("switched to %s ✓ (%d/%d) %s",
		newProv.Name(), nextIdx+1, len(cycle), suffix)
	return m, clearToastAfter(toastDuration)
}

// sortBySize sorts Ollama models in place, smallest parameter count first.
// Models with unparseable sizes go last (treated as "unknown / probably big").
func sortBySize(models []intake.OllamaModel) {
	for i := 1; i < len(models); i++ {
		for j := i; j > 0; j-- {
			a := paramRank(models[j-1])
			b := paramRank(models[j])
			if a > b {
				models[j-1], models[j] = models[j], models[j-1]
			}
		}
	}
}

// paramRank returns the model's parameter count for sorting; 0/unparseable
// is mapped to a large sentinel so those models sort last.
func paramRank(m intake.OllamaModel) int {
	n := parseParamSize(m.ParameterSize)
	if n == 0 {
		return 1 << 30
	}
	return n
}

// pickSmallestModel returns the chat-capable model with the smallest
// parameter count. Smaller models load faster (less VRAM transfer) and
// generate faster, which matters a lot on iGPUs. Falls back to the first
// model when sizes are unparseable.
//
// Retained for tests that exercise the legacy single-pick path; the
// `p` cycle now uses sortBySize + the full ordered list.
func pickSmallestModel(models []intake.OllamaModel) string {
	if len(models) == 0 {
		return ""
	}
	best := 0
	bestSize := parseParamSize(models[0].ParameterSize)
	for i := 1; i < len(models); i++ {
		s := parseParamSize(models[i].ParameterSize)
		if s > 0 && (bestSize == 0 || s < bestSize) {
			best = i
			bestSize = s
		}
	}
	return models[best].Name
}

// parseParamSize converts an Ollama parameter_size string ("26B", "36.0B",
// "137M", "7B") into millions of params. Returns 0 if the string is empty
// or unparseable.
func parseParamSize(s string) int {
	if s == "" {
		return 0
	}
	mult := 1
	suffix := s[len(s)-1]
	num := s[:len(s)-1]
	switch suffix {
	case 'B', 'b':
		mult = 1000
	case 'M', 'm':
		mult = 1
	default:
		num = s // no suffix; assume raw millions
	}
	var f float64
	if _, err := fmt.Sscanf(num, "%f", &f); err != nil {
		return 0
	}
	return int(f * float64(mult))
}

// reachableOllama is a thin wrapper around the probe that also confirms
// the daemon truly responds (probe alone doesn't fully exercise it).
// Currently unused — kept here for the model-picker overlay (future).
func reachableOllama(baseURL string) bool {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
