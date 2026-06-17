package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// component.go holds moss's reusable UI components. Screens (model.go,
// editor.go) compose these; they never draw borders, pad, gap-fill, or
// truncate by hand. Each component owns its styling via the named styles in
// styles.go. See docs/DESIGN.md §8.

// --- Pane -----------------------------------------------------------------

// pane is a bordered region with a title and a body, sized to exactly
// width × height cells. Both the list and the preview/editor regions are
// panes; this is the only way moss draws a bordered region.
type pane struct {
	title  string
	width  int
	height int
	active bool
}

// contentHeight returns the number of body rows available inside the pane,
// after accounting for the border and title.
func (p pane) contentHeight() int {
	h := p.height - paneBorderWidth - paneTitleHeight
	if h < 1 {
		return 1
	}
	return h
}

// render wraps body in the pane's border, title, and padding.
func (p pane) render(body string) string {
	style := paneStyle
	if p.active {
		style = activePaneStyle
	}
	title := titleStyle.Render(p.title)
	inner := lipgloss.JoinVertical(lipgloss.Left, title, body)
	return style.
		Width(p.width - paneBorderWidth).
		Height(p.height - paneBorderWidth).
		Render(inner)
}

// padToHeight appends blank lines so content occupies at least n rows. Used to
// push a pane's body to fill its content area.
func padToHeight(content string, n int) string {
	lines := strings.Count(content, "\n") + 1
	for lines < n {
		content += "\n"
		lines++
	}
	return content
}

// --- List item ------------------------------------------------------------

// renderListItem renders one selectable row: the cursor glyph prefix plus the
// pre-formatted label, styled by selection state. Both the notes list and the
// todos list use this — neither re-implements cursor prefixing.
func renderListItem(label string, selected bool) string {
	if selected {
		return selectedItemStyle.Render(glyphCursor + label)
	}
	return normalItemStyle.Render(glyphNoCursor + label)
}

// --- Status bar -----------------------------------------------------------

// statusBar is the bottom line: left-aligned segments and optional
// right-aligned segments, gap-filled to the full width. All modes build one of
// these so the join/gap-fill logic lives in exactly one place.
type statusBar struct {
	width int
	left  []string
	right []string
}

func (s statusBar) render() string {
	left := strings.Join(s.left, glyphStatusSep)
	right := strings.Join(s.right, glyphStatusSep)

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return statusBarStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// metaChip renders a secondary-colored key:value metadata segment for the
// status bar (e.g. "tag:go", "project:x").
func metaChip(text string) string {
	return metaChipStyle.Render(text)
}

// --- Modal ----------------------------------------------------------------

// modal center-places content in a single rounded overlay box over a
// width × height screen. Used for the help overlay.
func modal(width, height int, content string) string {
	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		dialogStyle.Render(content),
	)
}

// --- Key hints ------------------------------------------------------------

// keyHint renders advertised-shortcut text (status bar, editor footer) in the
// one muted hint style used everywhere.
func keyHint(text string) string {
	return helpStyle.Render(text)
}

// clampWidth truncates a (possibly styled) string to at most width cells.
func clampWidth(s string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
