package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Joncik91/inflate/internal/config"
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
	program    Sender // tea.Program — used to push streamed chunks
	autoPaste  bool
	pasteWinID int

	// cfg is the loaded config snapshot; mutated when the user toggles
	// providers via `p` so the change persists to disk and across launches.
	cfg config.Config
	// previousProvider holds the provider we'll restore when toggling back
	// from Ollama. Set when switching INTO Ollama. nil if there's nothing
	// to restore (cold start with Ollama already configured).
	previousProvider     provider.Provider
	previousProviderCfg  config.ProviderConfig

	seed           string
	preview        string
	bundle         harvester.ContextBundle
	stale          bool // preview no longer matches current seed
	inflightID     int
	cancelInflight context.CancelFunc

	// Feedback split into two channels:
	// - toast      transient (1.5s) confirmations like "copied ✓"
	// - errBanner  persistent error message; cleared by user typing or Esc
	toast     string
	errBanner string

	// Inflation feedback. inflating=true while the spinner should run.
	// First chunk arriving sets inflating=false and the preview pane
	// becomes the implicit progress indicator.
	inflating    bool
	spinnerFrame int

	// Help overlay toggled by `?`. When true, the preview pane is
	// replaced with the keybinding cheat sheet.
	helpOpen bool

	width  int
	height int
}


// New constructs the bubbletea Model. After tea.NewProgram(m) is
// called, the caller MUST call SetProgram on the model captured by
// the program (in practice: pass program in via a setter, see main.go)
// so the streaming inflation Cmd can push chunks through Program.Send.
func New(p provider.Provider, h *harvester.Harvester, cfg config.Config, pasteWinID int) Model {
	return Model{
		provider:   p,
		harvester:  h,
		cfg:        cfg,
		autoPaste:  cfg.AutoPaste,
		pasteWinID: pasteWinID,
		bundle:     h.Latest(),
	}
}

// ProgramInjectMsg carries a reference to the running tea.Program.
// main.go dispatches this via p.Send right after p.Run starts so the
// streaming inflation Cmd can push chunks through Program.Send.
// Exported because main lives outside this package.
type ProgramInjectMsg struct{ Program Sender }

// Sender is the subset of tea.Program the streaming inflation needs.
// Exported so main.go's tea.Program (which satisfies the interface)
// can be passed in. Tests inject a no-op sender.
type Sender interface {
	Send(msg tea.Msg)
}

func (m Model) Init() tea.Cmd { return waitForBundle(m.harvester.Bundles()) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case ProgramInjectMsg:
		m.program = msg.Program
		return m, nil

	case idleFiredMsg:
		return m.startInflation()

	case inflateStartedMsg:
		// Spinner kicks off only if no chunks have already arrived
		// (very fast first-token responses skip the spinner entirely).
		if msg.ReqID == m.inflightID && m.preview == "" {
			m.inflating = true
			return m, spinnerTick()
		}
		return m, nil

	case spinnerTickMsg:
		if !m.inflating {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, spinnerTick()

	case inflateChunkMsg:
		if msg.ReqID != m.inflightID {
			return m, nil // stale chunk from a cancelled run
		}
		m.preview += msg.Text
		m.inflating = false // first chunk: preview itself is the indicator
		return m, nil

	case inflateDoneMsg:
		if msg.ReqID == m.inflightID {
			m.stale = false
			m.inflating = false
			m.cancelInflight = nil
		}
		return m, nil

	case inflateFailMsg:
		if msg.ReqID == m.inflightID {
			m.errBanner = "inflate failed: " + msg.Err
			m.inflating = false
		}
		return m, nil

	case bundleUpdatedMsg:
		m.bundle = msg.Bundle
		return m, waitForBundle(m.harvester.Bundles())

	case toastClearMsg:
		m.toast = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()

	// Help overlay handling. `?` toggles help only when:
	//   - input is empty (no seed typed yet), OR
	//   - help is already open (so the same key dismisses it).
	// Otherwise `?` is a normal character (so questions can include "?").
	// Esc closes help when open.
	if key == "?" && (m.seed == "" || m.helpOpen) {
		m.helpOpen = !m.helpOpen
		return m, nil
	}
	if m.helpOpen && key == "esc" {
		m.helpOpen = false
		return m, nil
	}
	// `p` from the help overlay toggles between the configured provider
	// and a local Ollama. Only handled while help is open so it doesn't
	// shadow the letter "p" in normal typing.
	if m.helpOpen && key == "p" {
		return m.toggleProvider()
	}

	switch key {
	case "ctrl+c":
		if m.cancelInflight != nil {
			m.cancelInflight()
		}
		return m, tea.Quit
	case "enter":
		return m.send()
	case "esc":
		// Esc has a priority cascade:
		//   1. dismiss persistent error banner if present
		//   2. otherwise clear seed + preview
		if m.errBanner != "" {
			m.errBanner = ""
			return m, nil
		}
		m.seed = ""
		m.preview = ""
		m.stale = false
		return m, nil
	case "tab":
		return m.startInflation()
	case "backspace":
		if len(m.seed) > 0 {
			m.seed = m.seed[:len(m.seed)-1]
			// Typing (or backspacing) clears any persistent error.
			m.errBanner = ""
		}
	case " ", "space":
		m.seed += " "
		m.errBanner = ""
	default:
		if k.Type == tea.KeyRunes {
			m.seed += string(k.Runes)
			m.errBanner = ""
		} else if k.Type == tea.KeySpace {
			m.seed += " "
			m.errBanner = ""
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
	m.errBanner = ""

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelInflight = cancel

	id := m.inflightID
	bundle := m.harvester.Latest()
	seed := m.seed
	prov := m.provider
	prog := m.program

	// Stream chunks via Program.Send so View renders incrementally.
	// The synchronous Cmd just signals inflation has begun (kicks off
	// the spinner if the first chunk hasn't arrived yet).
	if prog != nil {
		go func() {
			ch := inflater.Inflate(ctx, prov, bundle, seed)
			gotAny := false
			for c := range ch {
				gotAny = true
				prog.Send(inflateChunkMsg{Text: c, ReqID: id})
			}
			if !gotAny {
				prog.Send(inflateFailMsg{Err: "no response (provider returned empty stream)", ReqID: id})
				return
			}
			prog.Send(inflateDoneMsg{ReqID: id})
		}()
	} else {
		// Test/no-program path: fall back to single batch (preserves
		// the single-shot path the old TUI used).
		go func() {
			ch := inflater.Inflate(ctx, prov, bundle, seed)
			var collected string
			for c := range ch {
				collected += c
			}
			// Without a program ref we can only return one message.
			// This branch should never run in production main.go.
			_ = collected
		}()
	}
	return m, func() tea.Msg { return inflateStartedMsg{ReqID: id} }
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

