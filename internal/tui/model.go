package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/devenjarvis/moss/internal/autocorrect"
	"github.com/devenjarvis/moss/internal/config"
	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
	msync "github.com/devenjarvis/moss/internal/sync"
	"github.com/devenjarvis/moss/internal/version"
)

// Pane focus constants
const (
	paneList    = 0
	panePreview = 1
)

// Mode constants
const (
	modeNormal = iota
	modeSearch
	modeHelp
	modeConfirm
	modeNewNote
	modeTagFilter
	modeTodos
	modeEdit
)

// Sort modes
const (
	sortDate     = "date"
	sortTitle    = "title"
	sortModified = "modified"
	sortWords    = "words"
)

// Messages
type notesLoadedMsg struct {
	notes []*note.Note
}

type notePreviewMsg struct {
	content string
}

type syncCompleteMsg struct {
	notes []*note.Note
	err   error
}

type errMsg struct {
	err error
}

type clearStatusMsg struct{}

type deleteNoteMsg struct {
	err error
}

type tagsLoadedMsg struct {
	tags []string
}

type todosLoadedMsg struct {
	todos []note.TodoItem
}

type tagFilterMsg struct {
	notes []*note.Note
}

type searchResultsMsg struct {
	query string
	notes []*note.Note
	err   error
}

type todoToggledMsg struct {
	err error
}

type updateAvailableMsg struct {
	version string
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// Model is the main TUI model.
type Model struct {
	cfg      config.Config
	database *db.DB
	watcher  *msync.Watcher

	// Layout
	width      int
	height     int
	activePane int
	mode       int

	// Note list
	notes         []*note.Note
	filteredNotes []*note.Note
	listCursor    int
	listOffset    int

	// Preview
	preview        viewport.Model
	previewContent string

	// Search
	searchInput textinput.Model
	searchQuery string   // active search query text
	searchTerms []string // terms to highlight in results

	// New note title input
	newNoteInput textinput.Model

	// Tag filter input
	tagInput  textinput.Model
	allTags   []string
	activeTag string

	// Sort
	sortMode string

	// Confirm dialog
	confirmMsg    string
	confirmAction func() tea.Cmd

	// Status
	statusMsg string
	syncing   bool

	// Help overlay
	showHelp bool

	// Todos view
	todos         []note.TodoItem
	filteredTodos []note.TodoItem
	todoCursor    int
	todoOffset    int
	todoFilter    string // "open", "done", "all"

	// Editor
	editor      Editor
	editingPath string

	// Update notification
	updateVersion string

	// Input suppression: ignore key events until this time.
	// Prevents misparsed terminal query responses (OSC, CSI) from being
	// dispatched as individual key events at startup and after mode transitions.
	suppressInputUntil time.Time
}

// New creates a new TUI model.
func New(cfg config.Config, database *db.DB) Model {
	si := textinput.New()
	si.Placeholder = "Search notes..."
	si.CharLimit = 200

	ni := textinput.New()
	ni.Placeholder = "Note title (enter for untitled)..."
	ni.CharLimit = 200

	tagi := textinput.New()
	tagi.Placeholder = "Tag name..."
	tagi.CharLimit = 100

	preview := viewport.New()

	return Model{
		cfg:          cfg,
		database:     database,
		searchInput:  si,
		newNoteInput: ni,
		tagInput:     tagi,
		preview:      preview,
		sortMode:     sortDate,
		todoFilter:   "open",
		// Suppress key input briefly at startup to absorb terminal query
		// responses (OSC color, CSI cursor position) that Bubble Tea's input
		// parser may dispatch as individual key events.
		suppressInputUntil: time.Now().Add(500 * time.Millisecond),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadNotes(m.database),
		textinput.Blink,
		checkForUpdate(),
	)
}

func checkForUpdate() tea.Cmd {
	return func() tea.Msg {
		if latest := version.CheckLatest(); latest != "" {
			return updateAvailableMsg{version: latest}
		}
		return nil
	}
}

func loadNotes(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		notes, err := database.AllNotes()
		if err != nil {
			return errMsg{err}
		}
		return notesLoadedMsg{notes}
	}
}

func loadNotesSorted(database *db.DB, sortBy string) tea.Cmd {
	return func() tea.Msg {
		notes, err := database.AllNotesSorted(sortBy)
		if err != nil {
			return errMsg{err}
		}
		return notesLoadedMsg{notes}
	}
}

func syncNotes(notesDir string, database *db.DB) tea.Cmd {
	return func() tea.Msg {
		notes, err := msync.SyncNotes(notesDir, database)
		return syncCompleteMsg{notes, err}
	}
}

func renderPreview(n *note.Note) tea.Cmd {
	return func() tea.Msg {
		// Build display content as raw markdown — rendering happens in Update
		// when we have access to the viewport width.
		var sb strings.Builder

		// Show frontmatter summary at top
		sb.WriteString(fmt.Sprintf("# %s\n\n", n.Title))
		if n.Date != "" {
			sb.WriteString(fmt.Sprintf("**Date:** %s\n", n.Date))
		}
		if len(n.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(n.Tags, ", ")))
		}
		if n.Project != "" {
			sb.WriteString(fmt.Sprintf("**Project:** %s\n", n.Project))
		}
		if n.Summary != "" {
			sb.WriteString(fmt.Sprintf("\n> %s\n", n.Summary))
		}
		sb.WriteString("\n---\n\n")
		sb.WriteString(n.Body)

		return notePreviewMsg{content: sb.String()}
	}
}

func deleteNoteFile(filePath string, database *db.DB) tea.Cmd {
	return func() tea.Msg {
		if err := os.Remove(filePath); err != nil {
			return deleteNoteMsg{err: err}
		}
		if err := database.DeleteNote(filePath); err != nil {
			return deleteNoteMsg{err: err}
		}
		return deleteNoteMsg{}
	}
}

func loadTags(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		tags, err := database.AllTags()
		if err != nil {
			return tagsLoadedMsg{}
		}
		return tagsLoadedMsg{tags: tags}
	}
}

func loadTodos(database *db.DB, filter string) tea.Cmd {
	return func() tea.Msg {
		todos, err := database.AllTodos(filter)
		if err != nil {
			return errMsg{err}
		}
		return todosLoadedMsg{todos: todos}
	}
}

func searchNotes(database *db.DB, query, activeTag string) tea.Cmd {
	return func() tea.Msg {
		pq := db.ParseSearchQuery(query)
		notes, err := database.SearchAdvanced(pq, activeTag)
		return searchResultsMsg{query: query, notes: notes, err: err}
	}
}

func toggleTodo(item note.TodoItem, database *db.DB) tea.Cmd {
	return func() tea.Msg {
		err := note.ToggleTodo(item.FilePath, item.LineNumber)
		if err != nil {
			return todoToggledMsg{err: err}
		}
		// Re-parse and re-index the modified file so the DB reflects the change
		n, err := note.ParseFile(item.FilePath)
		if err != nil {
			return todoToggledMsg{err: err}
		}
		if err := database.UpsertNote(n); err != nil {
			return todoToggledMsg{err: err}
		}
		todos := n.ParseTodos()
		if err := database.UpsertTodos(n.FilePath, todos); err != nil {
			return todoToggledMsg{err: err}
		}
		return todoToggledMsg{}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case notesLoadedMsg:
		m.notes = msg.notes
		m.filteredNotes = msg.notes
		// Re-apply tag filter if active
		if m.activeTag != "" {
			return m, m.filterByTag(m.activeTag)
		}
		if len(m.notes) > 0 {
			return m, renderPreview(m.notes[0])
		}
		return m, nil

	case syncCompleteMsg:
		m.syncing = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Sync error: %v", msg.err)
			return m, clearStatusAfter(5 * time.Second)
		}
		m.notes = msg.notes
		m.filteredNotes = msg.notes
		// Re-apply tag filter if active
		if m.activeTag != "" {
			return m, m.filterByTag(m.activeTag)
		}
		m.statusMsg = fmt.Sprintf("Synced %d notes", len(msg.notes))
		clearCmd := clearStatusAfter(3 * time.Second)
		// Reload todos if currently in todos view
		if m.mode == modeTodos {
			return m, tea.Batch(loadTodos(m.database, m.todoFilter), clearCmd)
		}
		// Clamp cursor to valid range
		if len(m.filteredNotes) == 0 {
			m.listCursor = 0
			m.listOffset = 0
			m.previewContent = ""
			m.preview.SetContent("")
			return m, clearCmd
		}
		if m.listCursor >= len(m.filteredNotes) {
			m.listCursor = len(m.filteredNotes) - 1
		}
		if m.listOffset > m.listCursor {
			m.listOffset = m.listCursor
		}
		return m, tea.Batch(renderPreview(m.filteredNotes[m.listCursor]), clearCmd)

	case notePreviewMsg:
		m.previewContent = msg.content
		width := m.preview.Width()
		if width <= 0 {
			width = 80
		}
		m.preview.SetContent(renderMarkdownPreview(msg.content, width))
		m.preview.GotoTop()
		return m, nil

	case deleteNoteMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Delete error: %v", msg.err)
			return m, clearStatusAfter(5 * time.Second)
		}
		m.statusMsg = "Note deleted"
		m.syncing = true
		return m, tea.Batch(
			syncNotes(m.cfg.NotesDir, m.database),
			clearStatusAfter(3*time.Second),
		)

	case tagsLoadedMsg:
		m.allTags = msg.tags
		return m, nil

	case tagFilterMsg:
		m.filteredNotes = msg.notes
		m.listCursor = 0
		m.listOffset = 0
		if len(m.filteredNotes) > 0 {
			return m, renderPreview(m.filteredNotes[0])
		}
		m.previewContent = ""
		m.preview.SetContent("")
		return m, nil

	case todosLoadedMsg:
		m.todos = msg.todos
		m.filteredTodos = msg.todos
		m.todoCursor = 0
		m.todoOffset = 0
		// Update preview if we have todos
		if len(m.filteredTodos) > 0 {
			return m, m.previewForTodo(m.filteredTodos[0])
		}
		m.previewContent = ""
		m.preview.SetContent("")
		return m, nil

	case todoToggledMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Toggle error: %v", msg.err)
			return m, clearStatusAfter(3 * time.Second)
		}
		return m, loadTodos(m.database, m.todoFilter)

	case editorAutoSaveTickMsg:
		if m.mode == modeEdit {
			var cmd tea.Cmd
			var shouldClose bool
			m.editor, cmd, shouldClose = m.editor.Update(msg)
			if shouldClose {
				return m, cmd
			}
			return m, cmd
		}
		return m, nil

	case editorSavedMsg:
		if m.mode == modeEdit {
			var cmd tea.Cmd
			m.editor, cmd, _ = m.editor.Update(msg)
			if msg.err != nil {
				m.statusMsg = fmt.Sprintf("Save error: %v", msg.err)
				return m, tea.Batch(cmd, clearStatusAfter(5*time.Second))
			}
			if msg.newPath != "" {
				// File was renamed - update watcher pause
				if m.watcher != nil {
					m.watcher.ResumeFile(m.editingPath)
					m.watcher.PauseFile(msg.newPath)
				}
				m.editingPath = msg.newPath
			}
			return m, cmd
		}
		return m, nil

	case noteCreatedMsg:
		parsed, err := note.ParseFile(msg.path)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, clearStatusAfter(5 * time.Second)
		}
		editorW := m.width
		contentH := m.height - 3
		m.editor = NewEditor(parsed, m.database, editorW-4, contentH-1, autocorrect.New(m.cfg.AutocorrectEnabled()))
		m.editingPath = msg.path
		m.mode = modeEdit
		m.activePane = panePreview
		if m.watcher != nil {
			m.watcher.PauseFile(msg.path)
		}
		return m, nil

	case updateAvailableMsg:
		m.updateVersion = msg.version
		return m, nil

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case errMsg:
		m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		return m, clearStatusAfter(5 * time.Second)

	case searchResultsMsg:
		// Discard stale results from a previous keystroke
		if m.mode != modeSearch || msg.query != m.searchInput.Value() {
			return m, nil
		}
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Search error: %v", msg.err)
			return m, clearStatusAfter(3 * time.Second)
		}
		m.filteredNotes = msg.notes
		m.searchQuery = msg.query
		m.searchTerms = extractSearchTerms(msg.query)
		m.listCursor = 0
		m.listOffset = 0
		if len(m.filteredNotes) > 0 {
			return m, renderPreview(m.filteredNotes[0])
		}
		m.previewContent = ""
		m.preview.SetContent("")
		return m, nil

	case tea.KeyPressMsg:
		// Drop key events during suppression window. Terminal query responses
		// (OSC background color, CSI cursor position) can be misparsed by
		// Bubble Tea's input parser as individual character key events.
		if !m.suppressInputUntil.IsZero() && time.Now().Before(m.suppressInputUntil) {
			return m, nil
		}
		return m.handleKey(msg)
	}

	// Update focused input
	var cmd tea.Cmd
	switch {
	case m.mode == modeEdit:
		m.editor, cmd, _ = m.editor.Update(msg)
		return m, cmd
	case m.mode == modeSearch:
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	case m.mode == modeNewNote:
		m.newNoteInput, cmd = m.newNoteInput.Update(msg)
		return m, cmd
	case m.mode == modeTagFilter:
		m.tagInput, cmd = m.tagInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Confirm mode: only y/n/esc are valid
	if m.mode == modeConfirm {
		switch key {
		case "y", "Y":
			m.mode = modeNormal
			if m.confirmAction != nil {
				return m, m.confirmAction()
			}
			return m, nil
		case "n", "N", "esc":
			m.mode = modeNormal
			m.confirmMsg = ""
			m.confirmAction = nil
			return m, nil
		}
		return m, nil
	}

	// Edit mode: delegate all keys to editor
	if m.mode == modeEdit {
		var cmd tea.Cmd
		var shouldClose bool
		m.editor, cmd, shouldClose = m.editor.Update(msg)
		if shouldClose {
			if m.watcher != nil {
				m.watcher.ResumeFile(m.editingPath)
			}
			m.mode = modeNormal
			m.activePane = paneList
			m.editingPath = ""
			m.syncing = true
			// Suppress input briefly after closing editor — the renderer
			// transition can trigger terminal responses (cursor position
			// reports) that arrive as spurious key events.
			m.suppressInputUntil = time.Now().Add(300 * time.Millisecond)
			return m, tea.Batch(cmd, syncNotes(m.cfg.NotesDir, m.database))
		}
		return m, cmd
	}

	// Filter key events with non-text-producing modifiers (Super/Cmd, Hyper).
	// These should never insert text or trigger mode changes. Terminals with
	// partial Kitty keyboard protocol support may generate spurious key events
	// with these modifiers.
	if msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModHyper) {
		return m, nil
	}

	// Global escape
	if key == "esc" {
		if m.mode == modeSearch {
			m.mode = modeNormal
			m.searchInput.Blur()
			m.searchQuery = ""
			m.searchTerms = nil
			// Preserve active tag filter if set
			if m.activeTag != "" {
				m.listCursor = 0
				m.listOffset = 0
				return m, m.filterByTag(m.activeTag)
			}
			m.filteredNotes = m.notes
			if m.listCursor >= len(m.filteredNotes) {
				m.listCursor = max(0, len(m.filteredNotes)-1)
			}
			m.listOffset = 0
			if len(m.filteredNotes) > 0 {
				return m, renderPreview(m.filteredNotes[m.listCursor])
			}
			return m, nil
		}
		if m.mode == modeNewNote {
			m.mode = modeNormal
			m.newNoteInput.Blur()
			return m, nil
		}
		if m.mode == modeTagFilter {
			m.mode = modeNormal
			m.tagInput.Blur()
			// Clear tag filter
			m.activeTag = ""
			m.filteredNotes = m.notes
			m.listCursor = 0
			m.listOffset = 0
			if len(m.filteredNotes) > 0 {
				return m, renderPreview(m.filteredNotes[0])
			}
			return m, nil
		}
		if m.mode == modeTodos {
			m.mode = modeNormal
			m.activePane = paneList
			if len(m.filteredNotes) > 0 && m.listCursor < len(m.filteredNotes) {
				return m, renderPreview(m.filteredNotes[m.listCursor])
			}
			return m, nil
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}

	// Help overlay toggle
	if key == "?" && m.mode == modeNormal {
		m.showHelp = !m.showHelp
		return m, nil
	}

	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Search mode input
	if m.mode == modeSearch {
		switch key {
		case "enter":
			// Exit search mode, keep current results
			m.mode = modeNormal
			m.searchInput.Blur()
			if len(m.filteredNotes) > 0 {
				m.statusMsg = fmt.Sprintf("Found %d notes", len(m.filteredNotes))
			} else if m.searchInput.Value() != "" {
				m.statusMsg = "No notes found"
			}
			return m, clearStatusAfter(3 * time.Second)
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			// Live search: fire search on every keystroke
			query := strings.TrimSpace(m.searchInput.Value())
			if query == "" {
				// Empty query: restore full list (respecting active tag)
				if m.activeTag != "" {
					return m, tea.Batch(cmd, m.filterByTag(m.activeTag))
				}
				m.filteredNotes = m.notes
				m.searchQuery = ""
				m.searchTerms = nil
				m.listCursor = 0
				m.listOffset = 0
				if len(m.filteredNotes) > 0 {
					return m, tea.Batch(cmd, renderPreview(m.filteredNotes[0]))
				}
				return m, cmd
			}
			return m, tea.Batch(cmd, searchNotes(m.database, query, m.activeTag))
		}
	}

	// New note title input
	if m.mode == modeNewNote {
		switch key {
		case "enter":
			title := m.newNoteInput.Value()
			m.mode = modeNormal
			m.newNoteInput.Blur()
			m.newNoteInput.SetValue("")
			if title == "" {
				title = "untitled"
			}
			return m, m.createNewNote(title)
		default:
			var cmd tea.Cmd
			m.newNoteInput, cmd = m.newNoteInput.Update(msg)
			return m, cmd
		}
	}

	// Tag filter input
	if m.mode == modeTagFilter {
		switch key {
		case "enter":
			tag := m.tagInput.Value()
			m.mode = modeNormal
			m.tagInput.Blur()
			m.tagInput.SetValue("")
			if tag == "" {
				// Clear filter
				m.activeTag = ""
				m.filteredNotes = m.notes
				m.listCursor = 0
				m.listOffset = 0
				if len(m.filteredNotes) > 0 {
					return m, renderPreview(m.filteredNotes[0])
				}
				return m, nil
			}
			m.activeTag = tag
			return m, m.filterByTag(tag)
		default:
			var cmd tea.Cmd
			m.tagInput, cmd = m.tagInput.Update(msg)
			return m, cmd
		}
	}

	// Todos mode
	if m.mode == modeTodos {
		switch key {
		case "j", "down":
			if m.todoCursor < len(m.filteredTodos)-1 {
				m.todoCursor++
				m.ensureTodoVisible()
				return m, m.previewForTodo(m.filteredTodos[m.todoCursor])
			}
			return m, nil
		case "k", "up":
			if m.todoCursor > 0 {
				m.todoCursor--
				m.ensureTodoVisible()
				return m, m.previewForTodo(m.filteredTodos[m.todoCursor])
			}
			return m, nil
		case "enter":
			// Jump to source note
			if len(m.filteredTodos) > 0 && m.todoCursor < len(m.filteredTodos) {
				todo := m.filteredTodos[m.todoCursor]
				for i, n := range m.filteredNotes {
					if n.FilePath == todo.FilePath {
						m.listCursor = i
						m.ensureListVisible()
						break
					}
				}
				m.mode = modeNormal
				n := m.findNoteByPath(todo.FilePath)
				if n != nil {
					return m, renderPreview(n)
				}
				return m, nil
			}
			return m, nil
		case "space", "x":
			// Toggle todo
			if len(m.filteredTodos) > 0 && m.todoCursor < len(m.filteredTodos) {
				return m, toggleTodo(m.filteredTodos[m.todoCursor], m.database)
			}
			return m, nil
		case "f":
			// Cycle filter
			switch m.todoFilter {
			case "open":
				m.todoFilter = "done"
			case "done":
				m.todoFilter = "all"
			default:
				m.todoFilter = "open"
			}
			return m, loadTodos(m.database, m.todoFilter)
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// Normal mode
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.activePane = (m.activePane + 1) % 2
		return m, nil

	case "h", "left":
		if m.activePane > 0 {
			m.activePane--
		}
		return m, nil

	case "l", "right":
		if m.activePane < panePreview {
			m.activePane++
		}
		return m, nil

	case "j", "down":
		switch m.activePane {
		case paneList:
			if m.listCursor < len(m.filteredNotes)-1 {
				m.listCursor++
				m.ensureListVisible()
				return m, renderPreview(m.filteredNotes[m.listCursor])
			}
		case panePreview:
			m.preview.ScrollDown(3)
		}
		return m, nil

	case "k", "up":
		switch m.activePane {
		case paneList:
			if m.listCursor > 0 {
				m.listCursor--
				m.ensureListVisible()
				return m, renderPreview(m.filteredNotes[m.listCursor])
			}
		case panePreview:
			m.preview.ScrollUp(3)
		}
		return m, nil

	case "ctrl+d":
		if m.activePane == panePreview {
			m.preview.HalfPageDown()
		}
		return m, nil

	case "ctrl+u":
		if m.activePane == panePreview {
			m.preview.HalfPageUp()
		}
		return m, nil

	case "enter":
		if m.activePane == paneList && len(m.filteredNotes) > 0 && m.listCursor < len(m.filteredNotes) {
			n := m.filteredNotes[m.listCursor]
			parsed, err := note.ParseFile(n.FilePath)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, clearStatusAfter(5 * time.Second)
			}
			previewW := m.previewWidth()
			contentH := m.height - 3
			m.editor = NewEditor(parsed, m.database, previewW-4, contentH-1, autocorrect.New(m.cfg.AutocorrectEnabled()))
			m.editingPath = n.FilePath
			m.mode = modeEdit
			m.activePane = panePreview
			if m.watcher != nil {
				m.watcher.PauseFile(n.FilePath)
			}
			return m, nil
		}
		return m, nil

	case "/":
		m.mode = modeSearch
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		m.searchQuery = ""
		m.searchTerms = nil
		return m, textinput.Blink

	case "n":
		m.mode = modeNewNote
		m.newNoteInput.SetValue("")
		m.newNoteInput.Focus()
		return m, textinput.Blink

	case "d":
		if len(m.filteredNotes) > 0 && m.listCursor < len(m.filteredNotes) {
			n := m.filteredNotes[m.listCursor]
			m.mode = modeConfirm
			m.confirmMsg = fmt.Sprintf("Delete \"%s\"? (y/n)", n.Title)
			m.confirmAction = func() tea.Cmd {
				return deleteNoteFile(n.FilePath, m.database)
			}
		}
		return m, nil

	case "t":
		m.mode = modeTagFilter
		m.tagInput.SetValue("")
		m.tagInput.Focus()
		// Load available tags
		return m, tea.Batch(textinput.Blink, loadTags(m.database))

	case "o":
		// Cycle sort mode
		switch m.sortMode {
		case sortDate:
			m.sortMode = sortTitle
		case sortTitle:
			m.sortMode = sortModified
		case sortModified:
			m.sortMode = sortWords
		default:
			m.sortMode = sortDate
		}
		m.statusMsg = fmt.Sprintf("Sort: %s", m.sortMode)
		m.listCursor = 0
		m.listOffset = 0
		return m, tea.Batch(
			loadNotesSorted(m.database, m.sortMode),
			clearStatusAfter(3*time.Second),
		)

	case "s":
		m.syncing = true
		m.statusMsg = "Syncing..."
		return m, syncNotes(m.cfg.NotesDir, m.database)

	case "T":
		m.mode = modeTodos
		m.activePane = paneList
		m.todoCursor = 0
		m.todoOffset = 0
		return m, loadTodos(m.database, m.todoFilter)
	}

	return m, nil
}

func (m *Model) createNewNote(title string) tea.Cmd {
	return func() tea.Msg {
		path, err := note.CreateNew(m.cfg.NotesDir, title)
		if err != nil {
			return errMsg{err}
		}
		return noteCreatedMsg{path: path}
	}
}

func (m *Model) filterByTag(tag string) tea.Cmd {
	return func() tea.Msg {
		results, err := m.database.FilterByTag(tag)
		if err != nil {
			return errMsg{err}
		}
		return tagFilterMsg{notes: results}
	}
}

func (m *Model) ensureListVisible() {
	listHeight := m.listHeight()
	if m.listCursor < m.listOffset {
		m.listOffset = m.listCursor
	}
	if m.listCursor >= m.listOffset+listHeight {
		m.listOffset = m.listCursor - listHeight + 1
	}
}

func (m *Model) listHeight() int {
	// Account for status bar (1), borders (2), title (1), input bar (1)
	h := m.height - 5
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) ensureTodoVisible() {
	listHeight := m.listHeight()
	if m.todoCursor < m.todoOffset {
		m.todoOffset = m.todoCursor
	}
	if m.todoCursor >= m.todoOffset+listHeight {
		m.todoOffset = m.todoCursor - listHeight + 1
	}
}

func (m Model) findNoteByPath(filePath string) *note.Note {
	for _, n := range m.notes {
		if n.FilePath == filePath {
			return n
		}
	}
	return nil
}

func (m Model) previewForTodo(todo note.TodoItem) tea.Cmd {
	n := m.findNoteByPath(todo.FilePath)
	if n != nil {
		return renderPreview(n)
	}
	return nil
}

func (m *Model) updateLayout() {
	previewWidth := m.previewWidth()
	contentHeight := m.height - 3 // status bar (1) + pane borders (2)

	m.preview.SetWidth(previewWidth - 4)
	m.preview.SetHeight(contentHeight - 1) // subtract title line

	if m.previewContent != "" {
		width := m.preview.Width()
		if width <= 0 {
			width = 80
		}
		m.preview.SetContent(renderMarkdownPreview(m.previewContent, width))
	}

	if m.mode == modeEdit {
		m.editor.SetSize(m.width-4, contentHeight-1)
	}
}

func (m Model) listWidth() int {
	return int(float64(m.width) * listWidthRatio)
}

func (m Model) previewWidth() int {
	return m.width - m.listWidth()
}

func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if m.width == 0 {
		v.SetContent("Loading...")
		return v
	}

	if m.showHelp {
		v.SetContent(m.helpView())
		return v
	}

	contentHeight := m.height - 1 // status bar (1 line)

	var body string
	if m.mode == modeEdit {
		// Full-screen editor: hide the list pane
		editorPane := m.renderPreviewPane(m.width, contentHeight)
		body = editorPane
	} else {
		listW := m.listWidth()
		previewW := m.previewWidth()

		// Render panes
		listPane := m.renderListPane(listW, contentHeight)
		previewPane := m.renderPreviewPane(previewW, contentHeight)

		body = lipgloss.JoinHorizontal(lipgloss.Top, listPane, previewPane)
	}

	// Status bar
	status := m.renderStatusBar()

	v.SetContent(lipgloss.JoinVertical(lipgloss.Left, body, status))
	return v
}

func (m Model) renderListPane(width, height int) string {
	if m.mode == modeTodos {
		p := pane{title: fmt.Sprintf("Todos (%s)", m.todoFilter), width: width, height: height, active: m.activePane == paneList}
		return p.render(m.renderTodoList(width, p.contentHeight()))
	}

	p := pane{title: "Notes", width: width, height: height, active: m.activePane == paneList}
	return p.render(m.renderNoteList(width, p.contentHeight()))
}

// renderTodoList renders the todos list body (no pane chrome), padded to rows.
func (m Model) renderTodoList(width, rows int) string {
	if len(m.filteredTodos) == 0 {
		return padToHeight(helpStyle.Render("\n  No todos found."), rows)
	}

	listH := m.listHeight()
	var items []string
	for i := m.todoOffset; i < len(m.filteredTodos) && i < m.todoOffset+listH; i++ {
		t := m.filteredTodos[i]

		// Checkbox
		var checkbox string
		if t.Done {
			checkbox = todoDoneStyle.Render("[x]")
		} else {
			checkbox = todoOpenStyle.Render("[ ]")
		}

		// Todo text
		todoText := t.Text
		if todoText == "" {
			todoText = "(empty)"
		}

		// Source note name (truncated)
		source := t.NoteTitle
		maxTextLen := width - 12 - len(source)
		if maxTextLen < 8 {
			maxTextLen = width - 10
			source = ""
		}
		if maxTextLen < 0 {
			maxTextLen = 0
		}
		if maxTextLen > 3 && len(todoText) > maxTextLen {
			todoText = todoText[:maxTextLen-3] + "..."
		} else if maxTextLen == 0 {
			todoText = ""
		}

		var display string
		if source != "" {
			display = checkbox + " " + todoText + " " + todoSourceStyle.Render(source)
		} else {
			display = checkbox + " " + todoText
		}

		items = append(items, renderListItem(display, i == m.todoCursor))
	}

	return padToHeight(strings.Join(items, "\n"), rows)
}

// renderNoteList renders the notes list body (with any input prompt), padded to rows.
func (m Model) renderNoteList(width, rows int) string {
	listH := m.listHeight()
	var items []string
	for i := m.listOffset; i < len(m.filteredNotes) && i < m.listOffset+listH; i++ {
		n := m.filteredNotes[i]

		// Build display with date prefix and indicators
		var display string
		datePrefix := ""
		if n.Date != "" {
			// Show short date (MM-DD)
			parts := strings.Split(n.Date, "-")
			if len(parts) == 3 {
				datePrefix = parts[1] + "-" + parts[2] + " "
			}
		}

		titleText := n.Title
		// Add indicators
		indicators := ""
		if n.HasTodos {
			indicators += "+"
		}
		if indicators != "" {
			indicators = " " + indicators
		}

		maxTitleLen := width - 6 - len(datePrefix) - len(indicators)
		if maxTitleLen < 0 {
			maxTitleLen = 0
		}
		if maxTitleLen > 3 && len(titleText) > maxTitleLen {
			titleText = titleText[:maxTitleLen-3] + "..."
		} else if maxTitleLen > 0 && maxTitleLen <= 3 && len(titleText) > maxTitleLen {
			titleText = titleText[:maxTitleLen]
		} else if maxTitleLen == 0 {
			titleText = ""
		}

		display = datePrefix + titleText + indicators

		// Apply search highlighting to display text
		if len(m.searchTerms) > 0 {
			display = highlightMatches(display, m.searchTerms)
		}

		items = append(items, renderListItem(display, i == m.listCursor))
	}

	content := strings.Join(items, "\n")
	if inputBar := m.renderListInputBar(width); inputBar != "" {
		content = inputBar + "\n" + content
	}

	return padToHeight(content, rows)
}

// renderListInputBar renders the prompt shown above the note list for input
// modes (search, new note, tag filter, confirm). Empty in normal mode.
func (m Model) renderListInputBar(width int) string {
	switch m.mode {
	case modeSearch:
		return m.searchInput.View()
	case modeNewNote:
		return newNoteLabelStyle.Render("Title: ") + m.newNoteInput.View()
	case modeTagFilter:
		label := "Tag: "
		if len(m.allTags) > 0 {
			label = fmt.Sprintf("Tag (%s): ", strings.Join(m.allTags, ", "))
			// Truncate if too long for pane
			maxLen := width - 6
			if maxLen > 0 && len(label) > maxLen {
				label = label[:maxLen-3] + "..."
			}
		}
		return tagFilterLabelStyle.Render(label) + m.tagInput.View()
	case modeConfirm:
		return confirmPromptStyle.Render(m.confirmMsg)
	}
	return ""
}

func (m Model) renderPreviewPane(width, height int) string {
	active := m.activePane == panePreview

	if m.mode == modeEdit {
		p := pane{title: "Editor", width: width, height: height, active: active}
		return p.render(m.editor.View(width-4, height-4))
	}

	p := pane{title: "Preview", width: width, height: height, active: active}

	var content string
	if m.mode == modeTodos && len(m.filteredTodos) == 0 {
		content = helpStyle.Render("\n  No todos found.\n  Press 'f' to change filter.")
	} else if m.mode != modeTodos && len(m.filteredNotes) == 0 {
		content = helpStyle.Render("\n  No notes found.\n  Press 'n' to create one.")
	} else {
		content = m.preview.View()
	}

	return p.render(content)
}

func (m Model) renderStatusBar() string {
	if m.mode == modeEdit {
		return statusBar{
			width: m.width,
			left: []string{
				"Editing: " + m.editor.note.Title,
				keyHint("Tab: fields  Esc: save & close  Ctrl+S: save"),
			},
		}.render()
	}

	if m.mode == modeTodos {
		left := []string{fmt.Sprintf("%d todos (%s)", len(m.filteredTodos), m.todoFilter)}
		if len(m.filteredTodos) > 0 && m.todoCursor < len(m.filteredTodos) {
			if t := m.filteredTodos[m.todoCursor]; t.NoteProject != "" {
				left = append(left, metaChip("project:"+t.NoteProject))
			}
		}
		left = append(left, keyHint("f:filter  space:toggle  enter:go to note  Esc:back"))
		return statusBar{width: m.width, left: left}.render()
	}

	// Note count
	left := []string{fmt.Sprintf("%d notes", len(m.filteredNotes))}

	// Current note info
	if len(m.filteredNotes) > 0 && m.listCursor < len(m.filteredNotes) {
		n := m.filteredNotes[m.listCursor]
		if n.Date != "" {
			left = append(left, n.Date)
		}
		left = append(left, fmt.Sprintf("%d words", n.WordCount))
	}

	// Active tag filter
	if m.activeTag != "" {
		left = append(left, metaChip("tag:"+m.activeTag))
	}

	// Sort mode (show if not default)
	if m.sortMode != sortDate {
		left = append(left, metaSortStyle.Render("sort:"+m.sortMode))
	}

	// Sync status
	if m.syncing {
		left = append(left, statusActiveStyle.Render("syncing..."))
	}

	// Status message
	if m.statusMsg != "" {
		left = append(left, m.statusMsg)
	}

	// Update notification
	if m.updateVersion != "" {
		left = append(left, metaChip("v"+m.updateVersion+" available"))
	}

	return statusBar{
		width: m.width,
		left:  left,
		right: []string{keyHint("? help │ q quit")},
	}.render()
}

// extractSearchTerms extracts the value portions of a search query for highlighting.
// Field-prefixed terms like "title:foo" extract "foo", regular terms are kept as-is.
func extractSearchTerms(query string) []string {
	tokens := strings.Fields(query)
	var terms []string
	for _, t := range tokens {
		if idx := strings.Index(t, ":"); idx > 0 && idx < len(t)-1 {
			// Field-prefixed: extract the value
			val := t[idx+1:]
			val = strings.Trim(val, `"`)
			if val != "" {
				terms = append(terms, val)
			}
		} else if !strings.HasSuffix(t, ":") {
			terms = append(terms, t)
		}
	}
	return terms
}

// highlightMatches wraps case-insensitive matches of terms in the given style.
func highlightMatches(text string, terms []string) string {
	if len(terms) == 0 {
		return text
	}
	lower := strings.ToLower(text)
	// Mark positions that should be highlighted
	highlights := make([]bool, len(text))
	for _, term := range terms {
		termLower := strings.ToLower(term)
		start := 0
		for {
			idx := strings.Index(lower[start:], termLower)
			if idx == -1 {
				break
			}
			for j := start + idx; j < start+idx+len(termLower) && j < len(highlights); j++ {
				highlights[j] = true
			}
			start += idx + len(termLower)
		}
	}
	// Build result with styled segments
	var result strings.Builder
	inHighlight := false
	segStart := 0
	for i := 0; i <= len(text); i++ {
		currentHL := i < len(text) && highlights[i]
		if i == len(text) || currentHL != inHighlight {
			seg := text[segStart:i]
			if inHighlight {
				result.WriteString(searchMatchStyle.Render(seg))
			} else {
				result.WriteString(seg)
			}
			segStart = i
			inHighlight = currentHL
		}
	}
	return result.String()
}

// helpEntry is one key/description row in the help modal.
type helpEntry struct{ key, desc string }

// helpSection is a titled group of help entries.
type helpSection struct {
	title   string
	entries []helpEntry
}

var helpSections = []helpSection{
	{"Navigation", []helpEntry{
		{"j/k, ↑/↓", "Move up/down"},
		{"h/l, ←/→", "Switch panes"},
		{"Tab", "Next pane"},
		{"Ctrl+d/u", "Scroll half page"},
		{"Enter", "Edit note"},
	}},
	{"Actions", []helpEntry{
		{"n", "New note"},
		{"d", "Delete note"},
		{"/", "Search notes"},
		{"t", "Filter by tag"},
		{"o", "Cycle sort order"},
		{"s", "Sync & re-index"},
		{"T", "Todos view"},
	}},
	{"Search Syntax", []helpEntry{
		{"word", "Full-text search"},
		{"title:word", "Search titles"},
		{"tag:word", "Search by tag"},
		{"project:word", "Search by project"},
		{"people:word", "Search by people"},
		{"status:word", "Search by status"},
	}},
	{"General", []helpEntry{
		{"?", "Toggle help"},
		{"Esc", "Cancel / back"},
		{"q", "Quit"},
	}},
	{"Todos View (T)", []helpEntry{
		{"space/x", "Toggle todo"},
		{"enter", "Jump to note"},
		{"f", "Cycle filter"},
		{"Esc", "Back to notes"},
	}},
	{"Editor (Enter on note)", []helpEntry{
		{"Tab", "Next field"},
		{"Shift+Tab", "Previous field"},
		{"Enter", "Jump to body"},
		{"Ctrl+S", "Save"},
		{"Esc", "Save & close"},
	}},
	{"List Indicators", []helpEntry{
		{"+", "Contains TODOs"},
	}},
}

func (m Model) helpView() string {
	const keyWidth = 13

	var b strings.Builder
	b.WriteString(helpSectionStyle.Render("Moss — Help"))
	b.WriteString("\n")
	for _, sec := range helpSections {
		b.WriteString("\n")
		b.WriteString(helpSectionStyle.Render(sec.title))
		b.WriteString("\n")
		for _, e := range sec.entries {
			key := helpKeyStyle.Render(fmt.Sprintf("%-*s", keyWidth, e.key))
			b.WriteString("  " + key + helpDescStyle.Render(e.desc) + "\n")
		}
	}

	return modal(m.width, m.height, b.String())
}

// SetWatcher attaches a file watcher to the model.
func (m *Model) SetWatcher(w *msync.Watcher) {
	m.watcher = w
}
