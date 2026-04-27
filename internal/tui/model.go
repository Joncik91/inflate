package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/inflater"
	"github.com/Joncik91/inflate/internal/output"
	"github.com/Joncik91/inflate/internal/provider"
)

const (
	idleDelay     = 600 * time.Millisecond
	toastDuration = 1500 * time.Millisecond
)

// Model is the TUI state.
type Model struct {
	provider   provider.Provider
	harvester  *harvester.Harvester
	autoPaste  bool
	pasteWinID int

	seed           string
	preview        string
	bundle         harvester.ContextBundle
	stale          bool // preview no longer matches current seed
	inflightID     int
	cancelInflight context.CancelFunc
	toast          string
	width          int
	height         int
}

// New constructs the bubbletea Model. Caller starts the program with tea.NewProgram(m).
func New(p provider.Provider, h *harvester.Harvester, autoPaste bool, pasteWinID int) Model {
	return Model{
		provider:   p,
		harvester:  h,
		autoPaste:  autoPaste,
		pasteWinID: pasteWinID,
		bundle:     h.Latest(),
	}
}

func (m Model) Init() tea.Cmd { return waitForBundle(m.harvester.Bundles()) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case idleFiredMsg:
		return m.startInflation()

	case inflateChunkMsg:
		m.preview += msg.Text
		return m, nil

	case inflateDoneMsg:
		if msg.ReqID == m.inflightID {
			m.stale = false
			m.cancelInflight = nil
		}
		return m, nil

	case inflateFailMsg:
		m.toast = "inflate failed: " + msg.Err
		return m, clearToastAfter(toastDuration)

	case bundleUpdatedMsg:
		m.bundle = msg.Bundle
		return m, waitForBundle(m.harvester.Bundles())

	case inflateBatchMsg:
		return m.applyBatch(msg)

	case toastClearMsg:
		m.toast = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		if m.cancelInflight != nil {
			m.cancelInflight()
		}
		return m, tea.Quit
	case "enter":
		return m.send()
	case "esc":
		m.seed = ""
		m.preview = ""
		m.stale = false
		return m, nil
	case "tab":
		return m.startInflation()
	case "backspace":
		if len(m.seed) > 0 {
			m.seed = m.seed[:len(m.seed)-1]
		}
	default:
		if k.Type == tea.KeyRunes {
			m.seed += string(k.Runes)
		} else if k.String() == "space" {
			m.seed += " "
		}
	}
	if m.preview != "" {
		m.stale = true
	}
	return m, idleAfter(idleDelay)
}

func (m Model) send() (tea.Model, tea.Cmd) {
	if m.preview == "" {
		m.toast = "(empty — type something)"
		return m, clearToastAfter(toastDuration)
	}
	if err := output.WriteClipboard(m.preview); err != nil {
		m.toast = "clipboard error"
		return m, clearToastAfter(toastDuration)
	}
	m.toast = "copied ✓"
	if m.autoPaste {
		if err := output.Paste(m.preview, m.pasteWinID); err == nil {
			m.toast = "pasted ✓"
		}
	}
	return m, clearToastAfter(toastDuration)
}

func (m Model) startInflation() (tea.Model, tea.Cmd) {
	if m.seed == "" {
		return m, nil
	}
	if m.cancelInflight != nil {
		m.cancelInflight()
	}
	m.inflightID++
	m.preview = ""
	m.stale = false

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelInflight = cancel

	id := m.inflightID
	bundle := m.harvester.Latest()
	seed := m.seed
	prov := m.provider

	return m, func() tea.Msg {
		ch := inflater.Inflate(ctx, prov, bundle, seed)
		var collected string
		for c := range ch {
			collected += c
		}
		return inflateBatchMsg{Text: collected, ReqID: id}
	}
}

// inflateBatchMsg is the simplified single-shot delivery used in v0; the
// stream is consumed inside the Cmd and delivered as one message. v1 will
// switch to per-chunk delivery via a tea.Program.Send loop.
type inflateBatchMsg struct {
	Text  string
	ReqID int
}

func idleAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return idleFiredMsg{} })
}

func clearToastAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return toastClearMsg{} })
}

func waitForBundle(ch <-chan harvester.ContextBundle) tea.Cmd {
	return func() tea.Msg {
		b, ok := <-ch
		if !ok {
			return nil
		}
		return bundleUpdatedMsg{Bundle: b}
	}
}

// applyBatch populates preview from the single-shot inflation result.
func (m Model) applyBatch(b inflateBatchMsg) (tea.Model, tea.Cmd) {
	if b.ReqID == m.inflightID {
		m.preview = b.Text
		m.stale = false
	}
	return m, nil
}
