package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/devenjarvis/moss/internal/ai"
	"github.com/devenjarvis/moss/internal/autocorrect"
	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
)

// Editor focus states
const (
	editorFocusTitle = iota
	editorFocusTags
	editorFocusDate
	editorFocusBody
)

// Editor messages
type editorAutoSaveTickMsg struct {
	id int // tick ID to match against current
}

type editorSavedMsg struct {
	err     error
	newPath string // non-empty if file was renamed
}

type noteCreatedMsg struct {
	path string
}

// Editor messages for AI enhancement

// editorEnhanceChunkMsg delivers a streaming chunk of AI thoughts text.
type editorEnhanceChunkMsg struct {
	delta string            // new text to append to thoughts
	ch    <-chan ai.StreamEvent // channel for next chunk
}

// editorEnhanceCompleteMsg signals streaming is done with the corrected body.
type editorEnhanceCompleteMsg struct {
	correctedBody string
	err           error
}

// editorEnhanceTickMsg fires after a short typing pause to trigger AI enhancement.
type editorEnhanceTickMsg struct {
	id int // tick ID to match against current
}

// editorSpinnerTickMsg drives the spinner animation while AI is thinking.
type editorSpinnerTickMsg struct{}

// Editor is the in-app note editor component.
type Editor struct {
	note     *note.Note
	database *db.DB

	titleInput textinput.Model
	tagsInput  textinput.Model
	dateInput  textinput.Model
	body       textarea.Model

	focus    int
	dirty    bool
	saving   bool
	saved    bool
	lastEdit time.Time
	tickID   int
	topLine  int // first visible logical line for markdown renderer

	corrector      *autocorrect.Corrector
	lastCorrection *lastCorrectionState // tracks last autocorrect for undo-on-backspace

	// AI enhancement state
	lastReviewedBody string // body at last AI review (to detect changes)
	preCorrectBody   string // body before last auto-correction (for undo)
	aiThoughts       string // current AI thoughts/questions
	enhancePending   bool   // waiting for AI enhance response
	canUndoEnhance   bool   // true after auto-correction applied
	bodyAtRequest    string // body snapshot when enhance request was sent

	// Enhance debounce (separate from auto-save)
	enhanceTickID int // tick ID for enhance debounce

	// Spinner animation
	spinnerFrame int       // current frame index
	spinnerTick  time.Time // last spinner update

	// Streaming state
	streamingThoughts bool // true while thoughts are streaming in

	// Redo state
	redoCorrectedBody string // stashed corrected body for redo after undo
	canRedoEnhance    bool   // true after undo, cleared on next edit

	// Enhance debounce readiness flag (checked by model.go)
	enhanceReady bool
}

// lastCorrectionState stores enough info to undo the most recent autocorrect.
type lastCorrectionState struct {
	original  string // the word before correction
	corrected string // the word after correction
	focus     int    // which field was corrected
	row       int    // body line (only for body focus)
}

// NewEditor creates a new editor for the given note.
func NewEditor(n *note.Note, database *db.DB, width, height int, corrector *autocorrect.Corrector) Editor {
	ti := textinput.New()
	ti.Placeholder = "Title..."
	ti.CharLimit = 200
	ti.SetValue(n.Title)

	tags := textinput.New()
	tags.Placeholder = "tag1, tag2, ..."
	tags.CharLimit = 300
	tags.SetValue(strings.Join(n.Tags, ", "))

	date := textinput.New()
	date.Placeholder = "YYYY-MM-DD"
	date.CharLimit = 10
	date.SetValue(n.Date)

	ta := textarea.New()
	ta.Placeholder = "Start writing..."
	ta.SetValue(n.Body)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // unlimited
	ta.Focus()

	e := Editor{
		note:       n,
		database:   database,
		titleInput: ti,
		tagsInput:  tags,
		dateInput:  date,
		body:       ta,
		focus:      editorFocusBody,
		corrector:  corrector,
	}
	e.SetSize(width, height)
	return e
}

// Update handles key events for the editor. Returns the updated editor, a command, and whether the editor should close.
func (e Editor) Update(msg tea.Msg) (Editor, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()

		switch key {
		case "esc":
			// Save and close
			cmd := e.saveNow()
			return e, cmd, true

		case "ctrl+s":
			e.saving = true
			return e, e.saveNow(), false

		case "tab":
			if e.focus == editorFocusBody {
				e.indentLine(2)
				return e, nil, false
			}
			e.cycleFocus(1)
			return e, nil, false

		case "shift+tab":
			if e.focus == editorFocusBody {
				e.outdentLine(2)
				return e, nil, false
			}
			e.cycleFocus(-1)
			return e, nil, false

		case "ctrl+y":
			// Redo AI enhancement corrections
			if e.canRedoEnhance && e.redoCorrectedBody != "" {
				row := e.body.Line()
				col := e.body.Column()
				e.preCorrectBody = e.body.Value()
				e.body.SetValue(e.redoCorrectedBody)
				repositionCursor(&e.body, row, col)
				e.canUndoEnhance = true
				e.canRedoEnhance = false
				e.dirty = true
				e.saved = false
				e.lastEdit = time.Now()
				e.tickID++
				tickID := e.tickID
				tickCmd := tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
					return editorAutoSaveTickMsg{id: tickID}
				})
				return e, tickCmd, false
			}
			return e, nil, false

		case "ctrl+z":
			// Undo AI enhancement corrections
			if e.canUndoEnhance && e.preCorrectBody != "" {
				row := e.body.Line()
				col := e.body.Column()
				// Stash for redo
				e.redoCorrectedBody = e.body.Value()
				e.body.SetValue(e.preCorrectBody)
				repositionCursor(&e.body, row, col)
				e.canUndoEnhance = false
				e.canRedoEnhance = true
				e.dirty = true
				e.saved = false
				e.lastEdit = time.Now()
				e.tickID++
				tickID := e.tickID
				tickCmd := tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
					return editorAutoSaveTickMsg{id: tickID}
				})
				return e, tickCmd, false
			}
			return e, nil, false

		case "ctrl+m":
			if e.focus == editorFocusBody {
				e.setFocus(editorFocusTitle)
			} else {
				e.setFocus(editorFocusBody)
			}
			return e, nil, false

		case "enter":
			// In frontmatter fields, enter moves to body
			if e.focus != editorFocusBody {
				e.setFocus(editorFocusBody)
				return e, nil, false
			}
			// Smart list continuation in body
			if handled, cmd := e.handleSmartEnter(); handled {
				return e, cmd, false
			}

		case "super+b", "super+i", "super+1", "super+2", "super+3":
			if e.focus == editorFocusBody {
				switch key {
				case "super+b":
					e.body = toggleInlineMarker(e.body, "**")
				case "super+i":
					e.body = toggleInlineMarker(e.body, "*")
				case "super+1":
					e.body = toggleHeading(e.body, 1)
				case "super+2":
					e.body = toggleHeading(e.body, 2)
				case "super+3":
					e.body = toggleHeading(e.body, 3)
				}
				e.dirty = true
				e.saved = false
				e.lastEdit = time.Now()
				e.tickID++
				tickID := e.tickID
				tickCmd := tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
					return editorAutoSaveTickMsg{id: tickID}
				})
				return e, tickCmd, false
			}
		}

		// Filter unhandled super/hyper key events — don't pass to widgets
		// where they could insert garbage text on terminals with partial
		// Kitty keyboard protocol support.
		if msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModHyper) {
			return e, nil, false
		}

		// Undo last autocorrect on backspace
		if e.lastCorrection != nil && (msg.Code == tea.KeyBackspace || key == "backspace") {
			if e.undoLastCorrection() {
				e.dirty = true
				e.saved = false
				e.lastEdit = time.Now()
				e.tickID++
				tickID := e.tickID
				tickCmd := tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
					return editorAutoSaveTickMsg{id: tickID}
				})
				return e, tickCmd, false
			}
		}

		// Delegate to focused widget
		var cmd tea.Cmd
		switch e.focus {
		case editorFocusTitle:
			e.titleInput, cmd = e.titleInput.Update(msg)
		case editorFocusTags:
			e.tagsInput, cmd = e.tagsInput.Update(msg)
		case editorFocusDate:
			e.dateInput, cmd = e.dateInput.Update(msg)
		case editorFocusBody:
			e.body, cmd = e.body.Update(msg)
		}

		// Autocorrect after word boundary
		if e.corrector != nil && isWordBoundary(msg) {
			switch e.focus {
			case editorFocusTitle:
				e.autocorrectTextInput(&e.titleInput)
			case editorFocusBody:
				e.autocorrectTextArea()
			}
		} else {
			// Any non-boundary key clears the undo state
			e.lastCorrection = nil
		}

		// Clear redo state on new edits
		e.canRedoEnhance = false

		// Mark dirty and schedule auto-save + enhance debounce
		e.dirty = true
		e.saved = false
		e.lastEdit = time.Now()
		e.tickID++
		tickID := e.tickID
		tickCmd := tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
			return editorAutoSaveTickMsg{id: tickID}
		})

		// Schedule enhance debounce (800ms after last keystroke)
		e.enhanceTickID++
		enhanceID := e.enhanceTickID
		enhanceCmd := tea.Tick(800*time.Millisecond, func(_ time.Time) tea.Msg {
			return editorEnhanceTickMsg{id: enhanceID}
		})

		return e, tea.Batch(cmd, tickCmd, enhanceCmd), false

	case editorAutoSaveTickMsg:
		// Only save if this tick matches the latest and we're dirty
		if msg.id == e.tickID && e.dirty {
			e.saving = true
			return e, e.saveNow(), false
		}
		return e, nil, false

	case editorSavedMsg:
		e.saving = false
		if msg.err == nil {
			e.dirty = false
			e.saved = true
			if msg.newPath != "" {
				e.note.FilePath = msg.newPath
			}
		}
		return e, nil, false

	case editorEnhanceChunkMsg:
		// Streaming chunk of AI thoughts — append to display
		e.aiThoughts += msg.delta
		e.streamingThoughts = true
		// Schedule read of the next chunk from the stream
		return e, waitForStreamChunk(msg.ch), false

	case editorEnhanceCompleteMsg:
		e.enhancePending = false
		e.streamingThoughts = false
		if msg.err != nil {
			// Silently ignore AI errors — don't disrupt editing
			return e, nil, false
		}

		// Only apply corrections if body hasn't changed since we sent the request
		currentBody := e.body.Value()
		if currentBody == e.bodyAtRequest && msg.correctedBody != "" && msg.correctedBody != currentBody {
			row := e.body.Line()
			col := e.body.Column()
			e.preCorrectBody = currentBody
			e.body.SetValue(msg.correctedBody)
			repositionCursor(&e.body, row, col)
			e.canUndoEnhance = true
			e.lastReviewedBody = msg.correctedBody
			// Mark dirty so corrections get saved
			e.dirty = true
			e.saved = false
			e.lastEdit = time.Now()
			e.tickID++
			tickID := e.tickID
			return e, tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
				return editorAutoSaveTickMsg{id: tickID}
			}), false
		}
		// Body changed during AI processing — update reviewed marker to current
		e.lastReviewedBody = currentBody
		return e, nil, false

	case editorEnhanceTickMsg:
		// Only trigger if this is the latest enhance tick (debounce)
		if msg.id == e.enhanceTickID {
			// Signal to model.go that it should try maybeEnhance
			// We set a flag that model.go checks
			e.enhanceReady = true
		}
		return e, nil, false

	case editorSpinnerTickMsg:
		if e.enhancePending {
			e.spinnerFrame = (e.spinnerFrame + 1) % len(spinnerFrames)
			return e, tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
				return editorSpinnerTickMsg{}
			}), false
		}
		return e, nil, false
	}

	// Handle non-key messages (blink, etc.)
	var cmd tea.Cmd
	switch e.focus {
	case editorFocusTitle:
		e.titleInput, cmd = e.titleInput.Update(msg)
	case editorFocusTags:
		e.tagsInput, cmd = e.tagsInput.Update(msg)
	case editorFocusDate:
		e.dateInput, cmd = e.dateInput.Update(msg)
	case editorFocusBody:
		e.body, cmd = e.body.Update(msg)
	}
	return e, cmd, false
}

// View renders the editor.
func (e *Editor) View(width, height int) string {
	// Header: frontmatter fields (2 rows)
	titleLabel := editorLabelStyle.Render("title: ")
	tagsLabel := editorLabelStyle.Render("tags: ")
	dateLabel := editorLabelStyle.Render("date: ")

	row1 := titleLabel + e.titleInput.View()
	row2 := tagsLabel + e.tagsInput.View() + "  " + dateLabel + e.dateInput.View()

	separator := editorSeparatorStyle.Render(strings.Repeat("─", width-4))

	// Status line
	var status string
	if e.saving {
		status = editorDirtyStyle.Render("saving...")
	} else if e.dirty {
		statusText := "● modified"
		if e.enhancePending {
			statusText = e.SpinnerText() + " AI reviewing...  " + statusText
		}
		status = editorDirtyStyle.Render(statusText)
	} else if e.saved {
		savedText := "✓ saved"
		if e.enhancePending {
			savedText = e.SpinnerText() + " AI reviewing...  " + savedText
		}
		status = editorSavedStyle.Render(savedText)
	} else {
		helpText := "Tab: indent  Shift+Tab: outdent  Ctrl+M: frontmatter  Esc: save & close  Ctrl+S: save  ⌘B/I: bold/italic  ⌘1-3: heading"
		if e.enhancePending {
			helpText = e.SpinnerText() + " AI reviewing...  " + helpText
		}
		status = helpStyle.Render(helpText)
	}

	// Thoughts section (if available)
	var thoughtsStr string
	thoughtsHeight := 0
	if e.aiThoughts != "" {
		thoughtsStr = e.renderThoughts(width - 4)
		thoughtsHeight = lipgloss.Height(thoughtsStr)
	}

	// Body takes remaining space
	header := lipgloss.JoinVertical(lipgloss.Left, row1, row2, separator)
	headerHeight := lipgloss.Height(header)
	statusHeight := 1

	bodyHeight := height - headerHeight - statusHeight - thoughtsHeight - 1
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	e.body.SetWidth(width - 4)

	// Scroll tracking: keep cursor in view
	curLine := e.body.Line()
	if curLine < e.topLine {
		e.topLine = curLine
	}
	if curLine >= e.topLine+bodyHeight {
		e.topLine = curLine - bodyHeight + 1
	}

	bodyStr := renderMarkdownBody(
		e.body.Value(),
		e.body.Line(),
		e.body.Column(),
		e.focus == editorFocusBody,
		width-4,
		bodyHeight,
		e.topLine,
	)

	if thoughtsStr != "" {
		return lipgloss.JoinVertical(lipgloss.Left, header, bodyStr, thoughtsStr, status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, bodyStr, status)
}

// SetSize updates the editor dimensions.
func (e *Editor) SetSize(width, height int) {
	inputWidth := width - 14
	if inputWidth < 10 {
		inputWidth = 10
	}
	e.titleInput.SetWidth(inputWidth)
	e.tagsInput.SetWidth(inputWidth / 2)
	e.dateInput.SetWidth(12)

	bodyHeight := height - 6
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	e.body.SetHeight(bodyHeight)
	e.body.SetWidth(width - 4)
}

// FilePath returns the path of the note being edited.
func (e Editor) FilePath() string {
	return e.note.FilePath
}

func (e *Editor) cycleFocus(direction int) {
	// Tab only cycles within frontmatter fields (title, tags, date); body is reached via Enter or Ctrl+M
	const frontmatterCount = 3 // editorFocusTitle, editorFocusTags, editorFocusDate
	next := (e.focus + direction + frontmatterCount) % frontmatterCount
	e.setFocus(next)
}

func (e *Editor) setFocus(focus int) {
	// Blur current
	switch e.focus {
	case editorFocusTitle:
		e.titleInput.Blur()
	case editorFocusTags:
		e.tagsInput.Blur()
	case editorFocusDate:
		e.dateInput.Blur()
	case editorFocusBody:
		e.body.Blur()
	}

	e.focus = focus

	// Focus new
	switch e.focus {
	case editorFocusTitle:
		e.titleInput.Focus()
	case editorFocusTags:
		e.tagsInput.Focus()
	case editorFocusDate:
		e.dateInput.Focus()
	case editorFocusBody:
		e.body.Focus()
	}
}

func (e *Editor) indentLine(spaces int) {
	val := e.body.Value()
	lines := strings.Split(val, "\n")
	lineIdx := e.body.Line()
	col := e.body.Column()
	prefix := strings.Repeat(" ", spaces)
	lines[lineIdx] = prefix + lines[lineIdx]
	e.body.SetValue(strings.Join(lines, "\n"))
	repositionCursor(&e.body, lineIdx, col+spaces)
}

func (e *Editor) outdentLine(spaces int) {
	val := e.body.Value()
	lines := strings.Split(val, "\n")
	lineIdx := e.body.Line()
	col := e.body.Column()
	line := lines[lineIdx]
	removed := 0
	for removed < spaces && removed < len(line) && line[removed] == ' ' {
		removed++
	}
	lines[lineIdx] = line[removed:]
	e.body.SetValue(strings.Join(lines, "\n"))
	newCol := col - removed
	if newCol < 0 {
		newCol = 0
	}
	repositionCursor(&e.body, lineIdx, newCol)
}

// listPrefixRe matches list markers: "- ", "* ", "1. ", "- [ ] ", "- [x] ", etc.
// It captures: (1) leading whitespace, (2) the marker itself.
var listPrefixRe = regexp.MustCompile(`^(\s*)([-*]\s\[[ xX]\]\s|[-*]\s|\d+\.\s)`)

// parseListPrefix returns the indentation and list marker for a line, or empty strings if none.
func parseListPrefix(line string) (indent, marker string) {
	m := listPrefixRe.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}

// nextListMarker returns the marker to use on the next line.
// Ordered list numbers are incremented; checkboxes become unchecked; bullets stay the same.
func nextListMarker(marker string) string {
	// Checkbox: always produce unchecked
	if strings.Contains(marker, "[x]") || strings.Contains(marker, "[X]") || strings.Contains(marker, "[ ]") {
		bullet := marker[0:1] // '-' or '*'
		return bullet + " [ ] "
	}
	// Ordered list: increment number
	if i := strings.Index(marker, "."); i > 0 {
		if num, err := strconv.Atoi(marker[:i]); err == nil {
			return strconv.Itoa(num+1) + ". "
		}
	}
	// Unordered bullet: keep as-is
	return marker
}

// handleSmartEnter processes Enter in the body textarea with smart list continuation.
// Returns true if it handled the keypress (caller should not delegate to textarea).
func (e *Editor) handleSmartEnter() (bool, tea.Cmd) {
	val := e.body.Value()
	lines := strings.Split(val, "\n")
	lineIdx := e.body.Line()
	col := e.body.Column()

	if lineIdx >= len(lines) {
		return false, nil
	}

	currentLine := lines[lineIdx]
	indent, marker := parseListPrefix(currentLine)
	if marker == "" {
		return false, nil
	}

	prefix := indent + marker
	contentAfterPrefix := currentLine[len(prefix):]

	// Check if the line is "empty" — only the prefix with no real content after it
	if strings.TrimSpace(contentAfterPrefix) == "" {
		// Remove the prefix, leaving a blank line (or just indentation removed)
		lines[lineIdx] = ""
		e.body.SetValue(strings.Join(lines, "\n"))
		repositionCursor(&e.body, lineIdx, 0)
		e.markDirty()
		return true, e.scheduleAutoSave()
	}

	// There's content: split the line at cursor and continue the list
	beforeCursor := currentLine[:col]
	afterCursor := currentLine[col:]

	next := nextListMarker(marker)
	newLine := indent + next + strings.TrimLeft(afterCursor, "")

	lines[lineIdx] = beforeCursor
	// Insert the new line after the current one
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:lineIdx+1]...)
	newLines = append(newLines, newLine)
	if lineIdx+1 < len(lines) {
		newLines = append(newLines, lines[lineIdx+1:]...)
	}

	e.body.SetValue(strings.Join(newLines, "\n"))
	repositionCursor(&e.body, lineIdx+1, len(indent+next))

	// If the list is ordered, renumber subsequent items at the same indent level
	e.renumberOrderedList(newLines, lineIdx+2, indent)

	e.markDirty()
	return true, e.scheduleAutoSave()
}

// renumberOrderedList updates numbering for consecutive ordered list items starting
// at startIdx, at the given indent level. Stops at the first non-matching line.
func (e *Editor) renumberOrderedList(lines []string, startIdx int, indent string) {
	if startIdx >= len(lines) {
		return
	}
	// Check if the previous line was ordered
	prevIndent, prevMarker := parseListPrefix(lines[startIdx-1])
	if prevIndent != indent || !isOrderedMarker(prevMarker) {
		return
	}
	prevNum := orderedMarkerNum(prevMarker)
	if prevNum < 0 {
		return
	}

	changed := false
	num := prevNum + 1
	for i := startIdx; i < len(lines); i++ {
		lineIndent, lineMarker := parseListPrefix(lines[i])
		if lineIndent != indent || !isOrderedMarker(lineMarker) {
			break
		}
		expected := strconv.Itoa(num) + ". "
		if lineMarker != expected {
			lines[i] = indent + expected + lines[i][len(lineIndent+lineMarker):]
			changed = true
		}
		num++
	}

	if changed {
		curRow := e.body.Line()
		curCol := e.body.Column()
		e.body.SetValue(strings.Join(lines, "\n"))
		repositionCursor(&e.body, curRow, curCol)
	}
}

func isOrderedMarker(marker string) bool {
	return len(marker) > 0 && marker[0] >= '0' && marker[0] <= '9'
}

func orderedMarkerNum(marker string) int {
	i := strings.Index(marker, ".")
	if i <= 0 {
		return -1
	}
	n, err := strconv.Atoi(marker[:i])
	if err != nil {
		return -1
	}
	return n
}

// markDirty marks the editor as modified.
func (e *Editor) markDirty() {
	e.dirty = true
	e.saved = false
	e.lastEdit = time.Now()
}

// scheduleAutoSave returns a command that schedules an auto-save tick.
func (e *Editor) scheduleAutoSave() tea.Cmd {
	e.tickID++
	tickID := e.tickID
	return tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
		return editorAutoSaveTickMsg{id: tickID}
	})
}

// repositionCursor moves the cursor to the given row and column after a SetValue call
// (which resets cursor to the beginning). We loop until Line() reaches the target
// rather than calling CursorDown a fixed number of times, because CursorDown moves
// by visual (soft-wrapped) rows, not logical lines.
func repositionCursor(ta *textarea.Model, row, col int) {
	ta.MoveToBegin()
	target := min(row, ta.LineCount()-1)
	for ta.Line() < target {
		ta.CursorDown()
	}
	ta.SetCursorColumn(col)
}

// toggleInlineMarker inserts a markdown marker pair (e.g. ** for bold, * for italic)
// at the cursor, or removes it if the cursor is between matching markers.
func toggleInlineMarker(ta textarea.Model, marker string) textarea.Model {
	line := ta.Line()
	col := ta.Column()
	value := ta.Value()
	lines := strings.Split(value, "\n")
	currentLine := lines[line]
	ml := len(marker)

	// Check if cursor is between matching marker pairs (e.g. **|**)
	if col >= ml && col+ml <= len(currentLine) &&
		currentLine[col-ml:col] == marker && currentLine[col:col+ml] == marker {
		// For single-char marker (*), ensure we're not inside bold markers (**)
		if ml == 1 && col >= 2 && col+2 <= len(currentLine) &&
			currentLine[col-2:col] == "**" && currentLine[col:col+2] == "**" {
			// Inside bold markers, not italic — insert italic instead
			ta.InsertString(marker + marker)
			ta.SetCursorColumn(col + ml)
			return ta
		}
		// Remove markers (toggle off)
		lines[line] = currentLine[:col-ml] + currentLine[col+ml:]
		ta.SetValue(strings.Join(lines, "\n"))
		repositionCursor(&ta, line, col-ml)
		return ta
	}

	// Insert markers (toggle on)
	ta.InsertString(marker + marker)
	ta.SetCursorColumn(col + ml)
	return ta
}

// toggleHeading toggles a heading prefix (# , ## , ### ) on the current line.
func toggleHeading(ta textarea.Model, level int) textarea.Model {
	line := ta.Line()
	col := ta.Column()
	value := ta.Value()
	lines := strings.Split(value, "\n")
	currentLine := lines[line]
	prefix := strings.Repeat("#", level) + " "

	// Strip any existing heading prefix (up to 6 levels)
	stripped := currentLine
	oldPrefixLen := 0
	for i := 6; i >= 1; i-- {
		p := strings.Repeat("#", i) + " "
		if strings.HasPrefix(currentLine, p) {
			stripped = strings.TrimPrefix(currentLine, p)
			oldPrefixLen = len(p)
			break
		}
	}

	// Toggle: if same level, remove; otherwise set new level
	var newCol int
	if strings.HasPrefix(currentLine, prefix) {
		lines[line] = stripped
		newCol = col - oldPrefixLen
	} else {
		lines[line] = prefix + stripped
		newCol = col - oldPrefixLen + len(prefix)
	}
	if newCol < 0 {
		newCol = 0
	}

	ta.SetValue(strings.Join(lines, "\n"))
	repositionCursor(&ta, line, newCol)
	return ta
}

func (e *Editor) saveNow() tea.Cmd {
	// Capture current values
	n := e.note
	title := e.titleInput.Value()
	tagsStr := e.tagsInput.Value()
	dateStr := e.dateInput.Value()
	bodyText := e.body.Value()
	database := e.database
	oldTitle := n.Title

	return func() tea.Msg {
		// Update note fields
		n.Title = title
		n.Date = dateStr
		n.Body = bodyText

		// Parse tags
		n.Tags = nil
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				n.Tags = append(n.Tags, t)
			}
		}

		// Update computed fields
		n.WordCount = len(strings.Fields(bodyText))
		n.HasTodos = strings.Contains(strings.ToLower(bodyText), "- [ ]") ||
			strings.Contains(strings.ToLower(bodyText), "- [x]") ||
			strings.Contains(strings.ToLower(bodyText), "todo")

		// Write to disk
		if err := n.WriteFrontmatter(); err != nil {
			return editorSavedMsg{err: err}
		}

		// Rename file if title changed
		var newPath string
		if title != oldTitle && title != "" {
			renamed, err := n.RenameToTitle()
			if err != nil {
				return editorSavedMsg{err: err}
			}
			if renamed != n.FilePath {
				// Delete old DB entry
				_ = database.DeleteNote(n.FilePath)
				n.FilePath = renamed
				newPath = renamed
			}
		}

		// Update database
		if err := database.UpsertNote(n); err != nil {
			return editorSavedMsg{err: err}
		}
		todos := n.ParseTodos()
		if err := database.UpsertTodos(n.FilePath, todos); err != nil {
			return editorSavedMsg{err: fmt.Errorf("todo sync: %w", err)}
		}

		return editorSavedMsg{newPath: newPath}
	}
}

// isWordBoundary returns true if the key event represents a word boundary character.
func isWordBoundary(msg tea.KeyPressMsg) bool {
	if msg.Code == tea.KeyEnter || msg.Code == tea.KeySpace {
		return true
	}
	if len(msg.Text) == 1 {
		switch msg.Text[0] {
		case '.', ',', '!', '?', ';', ':', ')', ']', '}', '-':
			return true
		}
	}
	return false
}

// isWordChar returns true if r is a character that can be part of a word.
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '\''
}

// autocorrectTextArea checks and corrects the word just completed in the body textarea.
func (e *Editor) autocorrectTextArea() {
	row := e.body.Line()
	col := e.body.Column()
	value := e.body.Value()
	lines := strings.Split(value, "\n")

	if row >= len(lines) {
		return
	}

	line := lines[row]

	// The cursor is after the boundary char. The word ended just before it.
	wordEnd := col - 1
	if wordEnd <= 0 {
		return
	}

	// Scan backwards to find word start
	wordStart := wordEnd
	for wordStart > 0 && isWordChar(rune(line[wordStart-1])) {
		wordStart--
	}

	if wordStart == wordEnd {
		return
	}

	word := line[wordStart:wordEnd]
	correction := e.corrector.CorrectWord(word)
	if correction == nil {
		return
	}

	// Replace word in line
	lines[row] = line[:wordStart] + correction.Corrected + line[wordEnd:]
	e.body.SetValue(strings.Join(lines, "\n"))

	// Restore cursor position (corrected word may differ in length)
	newCol := col + (len(correction.Corrected) - len(word))
	repositionCursor(&e.body, row, newCol)

	// Store for undo
	e.lastCorrection = &lastCorrectionState{
		original:  correction.Original,
		corrected: correction.Corrected,
		focus:     editorFocusBody,
		row:       row,
	}
}

// autocorrectTextInput checks and corrects the word just completed in a text input field.
func (e *Editor) autocorrectTextInput(ti *textinput.Model) {
	val := ti.Value()
	pos := ti.Position()

	if pos <= 1 {
		return
	}

	// Word ended just before the boundary char at pos-1
	wordEnd := pos - 1
	if wordEnd <= 0 {
		return
	}

	wordStart := wordEnd
	for wordStart > 0 && isWordChar(rune(val[wordStart-1])) {
		wordStart--
	}

	if wordStart == wordEnd {
		return
	}

	word := val[wordStart:wordEnd]
	correction := e.corrector.CorrectWord(word)
	if correction == nil {
		return
	}

	newVal := val[:wordStart] + correction.Corrected + val[wordEnd:]
	ti.SetValue(newVal)
	ti.SetCursor(pos + (len(correction.Corrected) - len(word)))

	// Store for undo
	e.lastCorrection = &lastCorrectionState{
		original:  correction.Original,
		corrected: correction.Corrected,
		focus:     e.focus,
	}
}

// undoLastCorrection reverses the most recent autocorrect. Returns true if an undo was performed.
func (e *Editor) undoLastCorrection() bool {
	lc := e.lastCorrection
	if lc == nil || lc.focus != e.focus {
		return false
	}

	switch lc.focus {
	case editorFocusBody:
		value := e.body.Value()
		lines := strings.Split(value, "\n")
		if lc.row >= len(lines) {
			return false
		}
		line := lines[lc.row]
		col := e.body.Column()

		// The boundary char is at col-1 (cursor is after it), corrected word ends at col-2
		// Find the corrected word in the line near the cursor
		searchEnd := col - 1 // position of boundary char
		searchStart := searchEnd - len(lc.corrected)
		if searchStart < 0 || searchEnd > len(line) {
			return false
		}
		if line[searchStart:searchEnd] != lc.corrected {
			return false
		}

		// Replace corrected word with original
		lines[lc.row] = line[:searchStart] + lc.original + line[searchEnd:]
		e.body.SetValue(strings.Join(lines, "\n"))
		newCol := col + (len(lc.original) - len(lc.corrected))
		repositionCursor(&e.body, lc.row, newCol)

	case editorFocusTitle:
		val := e.titleInput.Value()
		pos := e.titleInput.Position()
		searchEnd := pos - 1
		searchStart := searchEnd - len(lc.corrected)
		if searchStart < 0 || searchEnd > len(val) {
			return false
		}
		if val[searchStart:searchEnd] != lc.corrected {
			return false
		}
		newVal := val[:searchStart] + lc.original + val[searchEnd:]
		e.titleInput.SetValue(newVal)
		e.titleInput.SetCursor(pos + (len(lc.original) - len(lc.corrected)))

	default:
		return false
	}

	e.lastCorrection = nil
	return true
}

// renderThoughts renders the AI thoughts/questions section in a bordered box.
func (e *Editor) renderThoughts(width int) string {
	if e.aiThoughts == "" {
		return ""
	}

	thoughtsText := e.aiThoughts
	if e.streamingThoughts {
		thoughtsText += "▌" // cursor while streaming
	}

	var sections []string

	// Undo/redo hints
	if e.canUndoEnhance {
		sections = append(sections, helpStyle.Render("Ctrl+Z to undo"))
	} else if e.canRedoEnhance {
		sections = append(sections, helpStyle.Render("Ctrl+Y to redo AI corrections"))
	}

	sections = append(sections, aiThoughtsStyle.Render(thoughtsText))

	innerContent := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Render in a bordered box
	boxWidth := width
	if boxWidth < 10 {
		boxWidth = 10
	}
	box := aiThoughtsBoxStyle.Width(boxWidth - 2).Render(innerContent)

	return box
}

// BodyValue returns the current body text for external access.
func (e *Editor) BodyValue() string {
	return e.body.Value()
}

// LastReviewedBody returns the body text at the last AI review.
func (e *Editor) LastReviewedBody() string {
	return e.lastReviewedBody
}

// EnhancePending returns whether an AI enhance request is in flight.
func (e *Editor) EnhancePending() bool {
	return e.enhancePending
}

// SetEnhancePending marks that an enhance request has been submitted.
func (e *Editor) SetEnhancePending(pending bool) {
	e.enhancePending = pending
}

// SetBodyAtRequest stores the body snapshot when an enhance request was sent.
func (e *Editor) SetBodyAtRequest(body string) {
	e.bodyAtRequest = body
}

// ClearThoughts resets the AI thoughts display for a new streaming session.
func (e *Editor) ClearThoughts() {
	e.aiThoughts = ""
	e.streamingThoughts = false
}

// EnhanceReady returns and clears the enhance-ready flag set by debounce tick.
func (e *Editor) EnhanceReady() bool {
	if e.enhanceReady {
		e.enhanceReady = false
		return true
	}
	return false
}

// spinnerFrames are the animation frames for the AI thinking spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerText returns the current spinner frame text, or empty if not pending.
func (e *Editor) SpinnerText() string {
	if !e.enhancePending {
		return ""
	}
	return spinnerFrames[e.spinnerFrame%len(spinnerFrames)]
}

// StartSpinner kicks off the spinner animation and returns the first tick command.
func (e *Editor) StartSpinner() tea.Cmd {
	e.spinnerFrame = 0
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
		return editorSpinnerTickMsg{}
	})
}

// waitForStreamChunk returns a tea.Cmd that reads the next event from a stream channel.
func waitForStreamChunk(ch <-chan ai.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			// Channel closed unexpectedly
			return editorEnhanceCompleteMsg{}
		}
		if event.Err != nil {
			return editorEnhanceCompleteMsg{err: event.Err}
		}
		if event.Done {
			return editorEnhanceCompleteMsg{correctedBody: event.CorrectedBody}
		}
		return editorEnhanceChunkMsg{delta: event.ThoughtsDelta, ch: ch}
	}
}
