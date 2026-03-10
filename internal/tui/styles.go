package tui

import "charm.land/lipgloss/v2"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7C3AED") // purple
	colorSecondary = lipgloss.Color("#06B6D4") // cyan
	colorMuted     = lipgloss.Color("#6B7280") // gray
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

	// Todos
	todoDoneStyle = lipgloss.NewStyle().
			Foreground(colorAccent) // green for [x]

	todoOpenStyle = lipgloss.NewStyle().
			Foreground(colorWarning) // yellow for [ ]

	todoSourceStyle = lipgloss.NewStyle().
			Foreground(colorMuted) // gray for source note name

	// Search highlighting
	searchMatchStyle = lipgloss.NewStyle().
				Background(colorWarning).
				Foreground(lipgloss.Color("#1E1E2E"))

	// Editor
	editorLabelStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)

	editorSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	editorDirtyStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)

	editorSavedStyle = lipgloss.NewStyle().
				Foreground(colorAccent)

	// Markdown heading styles
	mdH1Style = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	mdH2Style = lipgloss.NewStyle().Bold(true).Foreground(colorSecondary)
	mdH3Style = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	mdH4Style = lipgloss.NewStyle().Bold(true)

	// Dimmed markers (**, *, `, #)
	mdMarkerStyle = lipgloss.NewStyle().Foreground(colorMuted).Faint(true)

	// Inline styles
	mdBoldStyle       = lipgloss.NewStyle().Bold(true)
	mdItalicStyle     = lipgloss.NewStyle().Italic(true)
	mdBoldItalicStyle = lipgloss.NewStyle().Bold(true).Italic(true)

	// Code styles
	mdCodeStyle      = lipgloss.NewStyle().Background(lipgloss.Color("#313244")).Foreground(colorWarning)
	mdCodeBlockStyle = lipgloss.NewStyle().Background(lipgloss.Color("#1E1E2E")).Foreground(colorFg)

	// List and structure
	mdBulletStyle     = lipgloss.NewStyle().Foreground(colorAccent)
	mdOrderedStyle    = lipgloss.NewStyle().Foreground(colorAccent)
	mdBlockquoteStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	mdHRuleStyle      = lipgloss.NewStyle().Foreground(colorMuted)

	// Cursor
	mdCursorStyle = lipgloss.NewStyle().Reverse(true)
)
