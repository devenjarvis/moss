package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7C3AED") // purple
	colorSecondary = lipgloss.Color("#06B6D4") // cyan
	colorMuted     = lipgloss.Color("#6B7280") // gray
	colorBg        = lipgloss.Color("#1E1E2E") // dark bg
	colorBgAlt     = lipgloss.Color("#313244") // alt bg
	colorFg        = lipgloss.Color("#CDD6F4") // light text
	colorAccent    = lipgloss.Color("#A6E3A1") // green
	colorWarning   = lipgloss.Color("#F9E2AF") // yellow

	// Pane styles
	paneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	activePaneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	// Title bar
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	// List items
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	statusActiveStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	// Help
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Chat
	chatInputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorSecondary).
			Padding(0, 1)

	chatResponseStyle = lipgloss.NewStyle().
				Foreground(colorFg).
				Padding(0, 1)

	// Tags
	tagStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Background(colorBgAlt).
			Padding(0, 1)

	// Todos
	todoDoneStyle = lipgloss.NewStyle().
			Foreground(colorAccent) // green for [x]

	todoOpenStyle = lipgloss.NewStyle().
			Foreground(colorWarning) // yellow for [ ]

	todoSourceStyle = lipgloss.NewStyle().
			Foreground(colorMuted) // gray for source note name
)
