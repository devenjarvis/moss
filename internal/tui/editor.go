package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
}

// NewEditor creates a new editor for the given note.
func NewEditor(n *note.Note, database *db.DB, width, height int) Editor {
	ti := textinput.New()
	ti.Placeholder = "Title..."
	ti.CharLimit = 200
	ti.SetValue(n.Title)
	ti.Focus()

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

	e := Editor{
		note:       n,
		database:   database,
		titleInput: ti,
		tagsInput:  tags,
		dateInput:  date,
		body:       ta,
		focus:      editorFocusTitle,
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
			e.cycleFocus(1)
			return e, nil, false

		case "shift+tab":
			e.cycleFocus(-1)
			return e, nil, false

		case "enter":
			// In frontmatter fields, enter moves to body
			if e.focus != editorFocusBody {
				e.setFocus(editorFocusBody)
				return e, nil, false
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

		// Mark dirty and schedule auto-save
		e.dirty = true
		e.saved = false
		e.lastEdit = time.Now()
		e.tickID++
		tickID := e.tickID
		tickCmd := tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return editorAutoSaveTickMsg{id: tickID}
		})

		return e, tea.Batch(cmd, tickCmd), false

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
func (e Editor) View(width, height int) string {
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
		status = editorDirtyStyle.Render("● modified")
	} else if e.saved {
		status = editorSavedStyle.Render("✓ saved")
	} else {
		status = helpStyle.Render("Tab: next field  Esc: save & close  Ctrl+S: save")
	}

	// Body takes remaining space
	header := lipgloss.JoinVertical(lipgloss.Left, row1, row2, separator)
	headerHeight := lipgloss.Height(header)
	statusHeight := 1

	bodyHeight := height - headerHeight - statusHeight - 1
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	e.body.SetHeight(bodyHeight)
	e.body.SetWidth(width - 4)

	return lipgloss.JoinVertical(lipgloss.Left, header, e.body.View(), status)
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
	next := (e.focus + direction + 4) % 4
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
