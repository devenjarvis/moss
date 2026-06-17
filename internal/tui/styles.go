package tui

import "charm.land/lipgloss/v2"

// styles.go holds named lipgloss styles composed from the design tokens in
// theme.go. This is the only place (besides component.go) that calls
// lipgloss.NewStyle(); screens reference these names. See docs/DESIGN.md.

var (
	// --- Panes ---
	paneStyle = lipgloss.NewStyle().
			BorderStyle(paneBorder).
			BorderForeground(colorBorder).
			Padding(0, spacePanePadX)

	activePaneStyle = lipgloss.NewStyle().
			BorderStyle(paneBorder).
			BorderForeground(colorBorderFocus).
			Padding(0, spacePanePadX)

	// Pane title bar
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, spacePanePadX)

	// --- List items ---
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(colorText)

	// --- Status bar ---
	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, spacePanePadX)

	statusActiveStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	// Metadata chips in the status bar (tag:, project:, vN available).
	metaChipStyle = lipgloss.NewStyle().Foreground(colorSecondary)
	// De-emphasized chip (sort: indicator).
	metaSortStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// --- Help / key hints ---
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Help modal rows
	helpSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	helpKeyStyle     = lipgloss.NewStyle().Foreground(colorSecondary)
	helpDescStyle    = lipgloss.NewStyle().Foreground(colorText)

	// --- Input prompts (list pane) ---
	newNoteLabelStyle   = lipgloss.NewStyle().Foreground(colorSuccess)
	tagFilterLabelStyle = lipgloss.NewStyle().Foreground(colorSecondary)
	confirmPromptStyle  = lipgloss.NewStyle().Foreground(colorWarning).Bold(true)

	// --- Todos ---
	todoDoneStyle = lipgloss.NewStyle().
			Foreground(colorSuccess) // green for [x]

	todoOpenStyle = lipgloss.NewStyle().
			Foreground(colorWarning) // yellow for [ ]

	todoSourceStyle = lipgloss.NewStyle().
			Foreground(colorMuted) // gray for source note name

	// --- Search highlighting ---
	searchMatchStyle = lipgloss.NewStyle().
				Background(colorWarning).
				Foreground(colorInverse)

	// --- Modal / dialog ---
	dialogStyle = lipgloss.NewStyle().
			Foreground(colorText).
			BorderStyle(paneBorder).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	// --- Editor ---
	editorLabelStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)

	editorSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	editorDirtyStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)

	editorSavedStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	// --- Markdown heading styles ---
	mdH1Style = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	mdH2Style = lipgloss.NewStyle().Bold(true).Foreground(colorSecondary)
	mdH3Style = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	mdH4Style = lipgloss.NewStyle().Bold(true)

	// Dimmed markers (**, *, `, #)
	mdMarkerStyle = lipgloss.NewStyle().Foreground(colorMuted).Faint(true)

	// Inline styles
	mdBoldStyle       = lipgloss.NewStyle().Bold(true)
	mdItalicStyle     = lipgloss.NewStyle().Italic(true)
	mdBoldItalicStyle = lipgloss.NewStyle().Bold(true).Italic(true)

	// Code styles
	mdCodeStyle      = lipgloss.NewStyle().Background(colorSurface).Foreground(colorWarning)
	mdCodeBlockStyle = lipgloss.NewStyle().Background(colorBg).Foreground(colorText)

	// List and structure
	mdBulletStyle         = lipgloss.NewStyle().Foreground(colorSuccess)
	mdOrderedStyle        = lipgloss.NewStyle().Foreground(colorSuccess)
	mdBlockquoteStyle     = lipgloss.NewStyle().Foreground(colorPrimary)
	mdBlockquoteTextStyle = lipgloss.NewStyle().Foreground(colorMuted)
	mdHRuleStyle          = lipgloss.NewStyle().Foreground(colorMuted)

	// Cursor
	mdCursorStyle = lipgloss.NewStyle().Reverse(true)

	// Checkbox styles
	mdCheckboxOpenStyle        = lipgloss.NewStyle().Foreground(colorWarning)
	mdCheckboxDoneStyle        = lipgloss.NewStyle().Foreground(colorSuccess)
	mdCheckboxOpenContentStyle = lipgloss.NewStyle().Foreground(colorWarning)
	mdCheckboxDoneContentStyle = lipgloss.NewStyle().Foreground(colorSuccess).Faint(true)
)
