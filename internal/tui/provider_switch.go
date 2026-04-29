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

// toggleProvider switches between the configured cloud provider and a
// local Ollama, in either direction:
//
//   - cloud → ollama: probe localhost:11434, build a fresh Ollama provider
//     against the first chat-capable model, save the new config to disk,
//     remember the previous provider so we can switch back.
//   - ollama → cloud: restore the in-memory previous provider/config.
//     If we don't have one (e.g. inflate booted with Ollama already
//     configured), tell the user to use `inflate config provider`.
//
// Cancels any in-flight inflation before swapping. Toasts the result.
func (m Model) toggleProvider() (tea.Model, tea.Cmd) {
	if m.cancelInflight != nil {
		m.cancelInflight()
		m.cancelInflight = nil
	}
	m.inflating = false

	if m.cfg.Provider.Kind == "ollama" {
		// Switch back to the previous provider, if we have one in memory.
		if m.previousProvider == nil {
			m.toast = "no previous provider — use `inflate config provider`"
			return m, clearToastAfter(toastDuration)
		}
		m.cfg.Provider = m.previousProviderCfg
		m.provider = m.previousProvider
		m.previousProvider = nil
		m.previousProviderCfg = config.ProviderConfig{}
		if err := config.SaveConfig(m.cfg); err != nil {
			m.errBanner = "save config: " + err.Error()
			return m, nil
		}
		m.helpOpen = false
		m.toast = "switched to " + m.provider.Name() + " ✓"
		return m, clearToastAfter(toastDuration)
	}

	// cloud → ollama path. Probe with a 500ms timeout so the UI doesn't
	// freeze if Ollama isn't running.
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
		m.toast = "Ollama not detected on localhost:11434"
		return m, clearToastAfter(toastDuration)
	}
	picked := models[0].Name

	// Build the Ollama provider and verify the daemon really answers
	// `/api/tags` before swapping (probe was best-effort).
	ollama := provider.NewOllama(picked, ollamaURLForTest)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ollama.Validate(ctx); err != nil {
		m.toast = "ollama: " + err.Error()
		return m, clearToastAfter(toastDuration)
	}

	// Remember the previous provider so `p` can flip back.
	m.previousProvider = m.provider
	m.previousProviderCfg = m.cfg.Provider

	baseURL := ollamaURLForTest
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	m.cfg.Provider = config.ProviderConfig{
		Kind:    "ollama",
		Model:   picked,
		BaseURL: baseURL,
	}
	if err := config.SaveConfig(m.cfg); err != nil {
		m.errBanner = "save config: " + err.Error()
		return m, nil
	}
	m.provider = ollama
	m.helpOpen = false

	suffix := ""
	if len(models) > 1 {
		suffix = fmt.Sprintf(" (1 of %d, use `inflate config provider` to pick others)", len(models))
	}
	m.toast = "switched to " + ollama.Name() + " ✓" + suffix
	return m, clearToastAfter(toastDuration)
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
