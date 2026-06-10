package main

import "github.com/charmbracelet/lipgloss"

var (
	stGreen   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	stYellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	stRed     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stMagenta = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	stCyan    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stBold    = lipgloss.NewStyle().Bold(true)
	stSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	stCursor  = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	stRedBold = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func actionGlyph(action string) (string, lipgloss.Style) {
	switch action {
	case actCreate:
		return "+", stGreen
	case actUpdate:
		return "~", stYellow
	case actDelete:
		return "-", stRed
	case actReplace:
		return "±", stMagenta
	case actRead:
		return "↻", stCyan
	default:
		return "·", stDim
	}
}

func hookGlyph(hookAction string) (string, lipgloss.Style) {
	switch hookAction {
	case "create":
		return "+", stGreen
	case "update":
		return "~", stYellow
	case "delete":
		return "-", stRed
	case "replace":
		return "±", stMagenta
	case "read", "refresh":
		return "↻", stCyan
	default:
		return "·", stDim
	}
}
