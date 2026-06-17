package tui

import "charm.land/lipgloss/v2"

// theme.go holds moss's design tokens: the raw color palette, the semantic
// color roles derived from it, spacing, border, and glyph constants. This is
// the ONLY file that may contain raw hex colors or literal UI glyphs — every
// other file references the named tokens below. See docs/DESIGN.md.

// --- Raw palette (private) ------------------------------------------------
//
// A dark, Catppuccin-Mocha-adjacent palette. These hex values exist here and
// nowhere else; reference the semantic tokens instead so the palette can be
// re-themed without touching screens.
var (
	hexBase    = lipgloss.Color("#1E1E2E") // darkest — backgrounds
	hexSurface = lipgloss.Color("#313244") // raised surface
	hexText    = lipgloss.Color("#CDD6F4") // light foreground text
	hexSubtle  = lipgloss.Color("#6B7280") // muted gray

	hexPurple = lipgloss.Color("#7C3AED") // brand primary
	hexCyan   = lipgloss.Color("#06B6D4") // brand secondary
	hexGreen  = lipgloss.Color("#A6E3A1") // success
	hexYellow = lipgloss.Color("#F9E2AF") // warning
)

// --- Semantic color tokens ------------------------------------------------
//
// These describe a role, not a hue. Use these everywhere.
var (
	// Neutrals
	colorBg      = hexBase    // app / editor / code-block background
	colorSurface = hexSurface // raised surface (inline code, chips)
	colorText    = hexText    // default foreground
	colorMuted   = hexSubtle  // secondary text, chrome, hints, inactive
	colorInverse = hexBase    // text drawn on a colored background

	// Brand
	colorPrimary   = hexPurple // focus, H1, blockquote, active border
	colorSecondary = hexCyan   // metadata chips, H2, field labels

	// Status
	colorSuccess = hexGreen  // done / saved / positive, list markers
	colorWarning = hexYellow // pending / dirty / attention, search match

	// Chrome roles
	colorBorder      = colorMuted   // inactive pane border
	colorBorderFocus = colorPrimary // focused pane border
)

// --- Layout tokens --------------------------------------------------------

const (
	// listWidthRatio is the list pane width as a fraction of terminal width.
	listWidthRatio = 0.22

	// paneBorderWidth is the number of cells a pane border consumes on each
	// axis (left+right, or top+bottom).
	paneBorderWidth = 2

	// paneTitleHeight is the number of rows a pane title occupies.
	paneTitleHeight = 1

	// statusBarHeight is the number of rows the bottom status bar occupies.
	statusBarHeight = 1

	// spacePanePadX is the horizontal padding inside panes and the status bar.
	spacePanePadX = 1
)

// --- Border tokens --------------------------------------------------------

// paneBorder is the border used for every bordered region in moss.
var paneBorder = lipgloss.RoundedBorder()

// --- Glyph tokens ---------------------------------------------------------
//
// A meaning has exactly one glyph. Defined once, referenced everywhere.
const (
	glyphCursor    = "▸ "  // selected list-row prefix
	glyphNoCursor  = "  "  // unselected list-row prefix (keeps alignment)
	glyphStatusSep = " │ " // separator between status-bar segments
	glyphRule      = "─"   // horizontal rule / separator fill
	glyphDirty     = "●"   // unsaved changes
	glyphSaved     = "✓"   // saved
)
