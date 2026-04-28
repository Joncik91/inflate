package tui

import "github.com/charmbracelet/lipgloss"

var (
	previewStyle      = lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.RoundedBorder())
	previewDimStyle   = previewStyle.Faint(true)
	statusStyleOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusStyleWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	statusStyleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	inputStyle        = lipgloss.NewStyle().Padding(0, 2).Border(lipgloss.NormalBorder(), true, false, false, false)
	toastStyle        = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236"))
	// Section labels (Role · Context · Task · …) in the named-section preview.
	sectionLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)
