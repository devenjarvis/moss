package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/devenjarvis/moss/internal/ai"
	"github.com/devenjarvis/moss/internal/config"
	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
	msync "github.com/devenjarvis/moss/internal/sync"
)

// Pane focus constants
const (
	paneList    = 0
	panePreview = 1
	paneChat    = 2
)

// Mode constants
const (
	modeNormal = iota
	modeSearch
	modeChat
	modeHelp
	modeConfirm
	modeNewNote
	modeGenerate
	modeTagFilter
	modeTodos
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

type aiResponseMsg struct {
	response string
	err      error
}

type generateCompleteMsg struct {
	path string
	err  error
}

type editorFinishedMsg struct {
	err error
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

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// Model is the main TUI model.
type Model struct {
	cfg      config.Config
	database *db.DB
	worker   *ai.Worker
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

	// Chat
	chatInput    textinput.Model
	chatHistory  []chatMessage
	chatViewport viewport.Model

	// Search
	searchInput textinput.Model
	searchQuery string   // active search query text
	searchTerms []string // terms to highlight in results

	// New note title input
	newNoteInput textinput.Model

	// Generate prompt input
	generateInput textinput.Model

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
	aiPending int
	syncing   bool

	// Help overlay
	showHelp bool

	// Todos view
	todos         []note.TodoItem
	filteredTodos []note.TodoItem
	todoCursor    int
	todoOffset    int
	todoFilter    string // "open", "done", "all"

	// Responsive: track which panes are visible
	chatVisible bool
}

type chatMessage struct {
	role    string // "user" or "assistant"
	content string
}

// New creates a new TUI model.
func New(cfg config.Config, database *db.DB, worker *ai.Worker) Model {
	ti := textinput.New()
	ti.Placeholder = "Ask about your notes..."
	ti.CharLimit = 500

	si := textinput.New()
	si.Placeholder = "Search notes..."
	si.CharLimit = 200

	ni := textinput.New()
	ni.Placeholder = "Note title (enter for untitled)..."
	ni.CharLimit = 200

	gi := textinput.New()
	gi.Placeholder = "Describe the note to generate..."
	gi.CharLimit = 500

	tagi := textinput.New()
	tagi.Placeholder = "Tag name..."
	tagi.CharLimit = 100

	preview := viewport.New(0, 0)
	chatVp := viewport.New(0, 0)

	return Model{
		cfg:           cfg,
		database:      database,
		worker:        worker,
		chatInput:     ti,
		searchInput:   si,
		newNoteInput:  ni,
		generateInput: gi,
		tagInput:      tagi,
		preview:       preview,
		chatViewport:  chatVp,
		sortMode:      sortDate,
		todoFilter:    "open",
		chatVisible:   true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadNotes(m.database),
		textinput.Blink,
	)
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
		// Build display content
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
		if n.Source == "generated" {
			sb.WriteString("**Source:** AI generated\n")
		}
		if n.Summary != "" {
			sb.WriteString(fmt.Sprintf("\n> %s\n", n.Summary))
		}
		sb.WriteString("\n---\n\n")
		sb.WriteString(n.Body)

		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			return notePreviewMsg{content: sb.String()}
		}

		rendered, err := renderer.Render(sb.String())
		if err != nil {
			return notePreviewMsg{content: sb.String()}
		}

		return notePreviewMsg{content: rendered}
	}
}

func askAI(ctx context.Context, question string, notes []*note.Note) tea.Cmd {
	return func() tea.Msg {
		// Gather note contents for context
		var sb strings.Builder
		for _, n := range notes {
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", n.Title, n.Body))
		}

		response, err := ai.Ask(ctx, question, sb.String())
		return aiResponseMsg{response, err}
	}
}

func openEditor(editor, path string) tea.Cmd {
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err}
	})
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

func generateNote(cfg config.Config, database *db.DB, prompt string, notes []*note.Note) tea.Cmd {
	return func() tea.Msg {
		var sb strings.Builder
		var sourcePaths []string
		for _, n := range notes {
			fullNote, err := note.ParseFile(n.FilePath)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", n.Title, fullNote.Body))
			sourcePaths = append(sourcePaths, n.FilePath)
		}

		content, err := ai.GenerateNote(context.Background(), prompt, sb.String())
		if err != nil {
			return generateCompleteMsg{err: err}
		}

		// Parse generated content to extract title for filename
		title := "generated"
		if fm, _ := extractFrontmatter(content); fm != "" {
			for _, line := range strings.Split(fm, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "title:") {
					title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "title:"))
					title = strings.Trim(title, "\"'")
					break
				}
			}
		}

		path, err := note.CreateNew(cfg.NotesDir, title)
		if err != nil {
			return generateCompleteMsg{err: err}
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return generateCompleteMsg{err: err}
		}

		n, err := note.ParseFile(path)
		if err == nil {
			n.Source = "generated"
			n.GeneratedPrompt = prompt
			n.GeneratedFrom = sourcePaths
			n.WriteFrontmatter()
			database.UpsertNote(n)
		}

		return generateCompleteMsg{path: path}
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

func extractFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return "", content
	}
	trimmed := strings.TrimSpace(content)
	start := strings.Index(trimmed, "---")
	rest := trimmed[start+3:]
	end := strings.Index(rest, "---")
	if end == -1 {
		return "", content
	}
	return strings.TrimSpace(rest[:end]), strings.TrimSpace(rest[end+3:])
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Auto-hide chat on narrow terminals
		m.chatVisible = m.width >= 100
		// Clamp activePane if chat pane is no longer visible
		if !m.chatVisible && m.activePane == paneChat {
			m.activePane = panePreview
		}
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
		m.preview.SetContent(msg.content)
		m.preview.GotoTop()
		return m, nil

	case aiResponseMsg:
		m.aiPending--
		if msg.err != nil {
			m.chatHistory = append(m.chatHistory, chatMessage{
				role:    "assistant",
				content: fmt.Sprintf("Error: %v", msg.err),
			})
		} else {
			m.chatHistory = append(m.chatHistory, chatMessage{
				role:    "assistant",
				content: msg.response,
			})
		}
		m.updateChatViewport()
		return m, nil

	case generateCompleteMsg:
		m.aiPending--
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Generate error: %v", msg.err)
			return m, clearStatusAfter(5 * time.Second)
		}
		m.statusMsg = fmt.Sprintf("Generated: %s", filepath.Base(msg.path))
		// Re-sync to pick up the new note
		m.syncing = true
		return m, tea.Batch(
			syncNotes(m.cfg.NotesDir, m.database),
			clearStatusAfter(5*time.Second),
		)

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

	case editorFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Editor error: %v", msg.err)
			return m, clearStatusAfter(5 * time.Second)
		}
		// Re-sync after editing
		return m, syncNotes(m.cfg.NotesDir, m.database)

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

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Update focused input
	var cmd tea.Cmd
	switch {
	case m.mode == modeSearch:
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	case m.mode == modeChat || m.activePane == paneChat:
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd
	case m.mode == modeNewNote:
		m.newNoteInput, cmd = m.newNoteInput.Update(msg)
		return m, cmd
	case m.mode == modeGenerate:
		m.generateInput, cmd = m.generateInput.Update(msg)
		return m, cmd
	case m.mode == modeTagFilter:
		m.tagInput, cmd = m.tagInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if m.mode == modeChat {
			m.mode = modeNormal
			m.chatInput.Blur()
			return m, nil
		}
		if m.mode == modeNewNote {
			m.mode = modeNormal
			m.newNoteInput.Blur()
			return m, nil
		}
		if m.mode == modeGenerate {
			m.mode = modeNormal
			m.generateInput.Blur()
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

	// Chat mode input
	if m.mode == modeChat {
		switch key {
		case "enter":
			question := m.chatInput.Value()
			if question == "" {
				return m, nil
			}
			m.chatInput.SetValue("")
			m.chatHistory = append(m.chatHistory, chatMessage{
				role:    "user",
				content: question,
			})
			m.aiPending++
			m.updateChatViewport()
			return m, askAI(context.Background(), question, m.filteredNotes)
		default:
			var cmd tea.Cmd
			m.chatInput, cmd = m.chatInput.Update(msg)
			return m, cmd
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

	// Generate prompt input
	if m.mode == modeGenerate {
		switch key {
		case "enter":
			prompt := m.generateInput.Value()
			m.mode = modeNormal
			m.generateInput.Blur()
			m.generateInput.SetValue("")
			if prompt == "" {
				return m, nil
			}
			m.aiPending++
			m.statusMsg = "Generating note..."
			return m, generateNote(m.cfg, m.database, prompt, m.notes)
		default:
			var cmd tea.Cmd
			m.generateInput, cmd = m.generateInput.Update(msg)
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
		case " ", "x":
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
		if m.chatVisible {
			m.activePane = (m.activePane + 1) % 3
		} else {
			m.activePane = (m.activePane + 1) % 2
		}
		return m, nil

	case "h", "left":
		if m.activePane > 0 {
			m.activePane--
		}
		return m, nil

	case "l", "right":
		maxPane := 2
		if !m.chatVisible {
			maxPane = 1
		}
		if m.activePane < maxPane {
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
			m.preview.LineDown(3)
		case paneChat:
			m.chatViewport.LineDown(3)
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
			m.preview.LineUp(3)
		case paneChat:
			m.chatViewport.LineUp(3)
		}
		return m, nil

	case "ctrl+d":
		if m.activePane == panePreview {
			m.preview.HalfViewDown()
		}
		return m, nil

	case "ctrl+u":
		if m.activePane == panePreview {
			m.preview.HalfViewUp()
		}
		return m, nil

	case "enter":
		if m.activePane == paneList && len(m.filteredNotes) > 0 && m.listCursor < len(m.filteredNotes) {
			n := m.filteredNotes[m.listCursor]
			return m, openEditor(m.cfg.Editor, n.FilePath)
		}
		return m, nil

	case "/":
		m.mode = modeSearch
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		m.searchQuery = ""
		m.searchTerms = nil
		return m, textinput.Blink

	case "c":
		m.mode = modeChat
		m.activePane = paneChat
		// Show chat pane if hidden
		if !m.chatVisible {
			m.chatVisible = true
			m.updateLayout()
		}
		m.chatInput.Focus()
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

	case "g":
		m.mode = modeGenerate
		m.generateInput.SetValue("")
		m.generateInput.Focus()
		return m, textinput.Blink

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
		c := exec.Command(m.cfg.Editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return editorFinishedMsg{err}
		})()
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
	// Account for borders, title, status bar
	h := m.height - 6
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
	chatWidth := m.chatWidth()
	contentHeight := m.height - 4 // status bar + borders

	m.preview.Width = previewWidth - 4
	m.preview.Height = contentHeight - 2

	if m.chatVisible {
		m.chatViewport.Width = chatWidth - 4
		m.chatViewport.Height = contentHeight - 6 // leave room for input
	}

	if m.previewContent != "" {
		m.preview.SetContent(m.previewContent)
	}
}

func (m Model) listWidth() int {
	return int(float64(m.width) * 0.22)
}

func (m Model) previewWidth() int {
	if !m.chatVisible {
		return m.width - m.listWidth()
	}
	return int(float64(m.width) * 0.46)
}

func (m Model) chatWidth() int {
	if !m.chatVisible {
		return 0
	}
	return m.width - m.listWidth() - m.previewWidth()
}

func (m *Model) updateChatViewport() {
	var sb strings.Builder
	for _, msg := range m.chatHistory {
		if msg.role == "user" {
			sb.WriteString(lipgloss.NewStyle().
				Foreground(colorSecondary).Bold(true).
				Render("You: "))
			sb.WriteString(msg.content)
		} else {
			sb.WriteString(lipgloss.NewStyle().
				Foreground(colorAccent).Bold(true).
				Render("Moss: "))
			sb.WriteString(msg.content)
		}
		sb.WriteString("\n\n")
	}
	if m.aiPending > 0 {
		sb.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).Italic(true).
			Render("Thinking..."))
	}
	m.chatViewport.SetContent(sb.String())
	m.chatViewport.GotoBottom()
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.showHelp {
		return m.helpView()
	}

	listW := m.listWidth()
	previewW := m.previewWidth()
	contentHeight := m.height - 2 // status bar

	// Render panes
	listPane := m.renderListPane(listW, contentHeight)
	previewPane := m.renderPreviewPane(previewW, contentHeight)

	var body string
	if m.chatVisible {
		chatW := m.chatWidth()
		chatPane := m.renderChatPane(chatW, contentHeight)
		body = lipgloss.JoinHorizontal(lipgloss.Top, listPane, previewPane, chatPane)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, listPane, previewPane)
	}

	// Status bar
	status := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

func (m Model) renderListPane(width, height int) string {
	style := paneStyle
	if m.activePane == paneList {
		style = activePaneStyle
	}

	// Todos mode rendering
	if m.mode == modeTodos {
		title := titleStyle.Render(fmt.Sprintf("Todos (%s)", m.todoFilter))

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

			if i == m.todoCursor {
				items = append(items, selectedItemStyle.Render("▸ "+display))
			} else {
				items = append(items, normalItemStyle.Render("  "+display))
			}
		}

		content := strings.Join(items, "\n")
		if len(m.filteredTodos) == 0 {
			content = helpStyle.Render("\n  No todos found.")
		}

		// Pad to fill height
		lines := strings.Count(content, "\n") + 1
		for lines < height-3 {
			content += "\n"
			lines++
		}

		inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
		return style.Width(width - 2).Height(height - 2).Render(inner)
	}

	title := titleStyle.Render("Notes")

	// Search bar or other input modes that show in list pane
	var inputBar string
	if m.mode == modeSearch {
		inputBar = m.searchInput.View()
	} else if m.mode == modeNewNote {
		inputBar = lipgloss.NewStyle().Foreground(colorAccent).Render("Title: ") + m.newNoteInput.View()
	} else if m.mode == modeGenerate {
		inputBar = lipgloss.NewStyle().Foreground(colorWarning).Render("Gen: ") + m.generateInput.View()
	} else if m.mode == modeTagFilter {
		label := "Tag: "
		if len(m.allTags) > 0 {
			label = fmt.Sprintf("Tag (%s): ", strings.Join(m.allTags, ", "))
			// Truncate if too long for pane
			maxLen := width - 6
			if maxLen > 0 && len(label) > maxLen {
				label = label[:maxLen-3] + "..."
			}
		}
		inputBar = lipgloss.NewStyle().Foreground(colorSecondary).Render(label) + m.tagInput.View()
	} else if m.mode == modeConfirm {
		inputBar = lipgloss.NewStyle().Foreground(colorWarning).Bold(true).Render(m.confirmMsg)
	}

	// Note list
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
		if n.Source == "generated" {
			indicators += "*"
		}
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

		if i == m.listCursor {
			items = append(items, selectedItemStyle.Render("▸ "+display))
		} else {
			items = append(items, normalItemStyle.Render("  "+display))
		}
	}

	content := strings.Join(items, "\n")
	if inputBar != "" {
		content = inputBar + "\n" + content
	}

	// Pad to fill height
	lines := strings.Count(content, "\n") + 1
	for lines < height-3 {
		content += "\n"
		lines++
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return style.Width(width - 2).Height(height - 2).Render(inner)
}

func (m Model) renderPreviewPane(width, height int) string {
	style := paneStyle
	if m.activePane == panePreview {
		style = activePaneStyle
	}

	title := titleStyle.Render("Preview")

	var content string
	if m.mode == modeTodos && len(m.filteredTodos) == 0 {
		content = helpStyle.Render("\n  No todos found.\n  Press 'f' to change filter.")
	} else if m.mode != modeTodos && len(m.filteredNotes) == 0 {
		content = helpStyle.Render("\n  No notes found.\n  Press 'n' to create one.")
	} else {
		content = m.preview.View()
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return style.Width(width - 2).Height(height - 2).Render(inner)
}

func (m Model) renderChatPane(width, height int) string {
	style := paneStyle
	if m.activePane == paneChat {
		style = activePaneStyle
	}

	title := titleStyle.Render("Chat")

	var content string
	if len(m.chatHistory) == 0 && m.aiPending == 0 {
		content = helpStyle.Render("\n  Press 'c' to ask\n  about your notes.")
	} else {
		content = m.chatViewport.View()
	}

	var input string
	if m.mode == modeChat {
		input = chatInputStyle.Width(width - 6).Render(m.chatInput.View())
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content, input)
	return style.Width(width - 2).Height(height - 2).Render(inner)
}

func (m Model) renderStatusBar() string {
	var parts []string

	if m.mode == modeTodos {
		parts = append(parts, fmt.Sprintf("%d todos (%s)", len(m.filteredTodos), m.todoFilter))
		if len(m.filteredTodos) > 0 && m.todoCursor < len(m.filteredTodos) {
			t := m.filteredTodos[m.todoCursor]
			if t.NoteProject != "" {
				parts = append(parts, lipgloss.NewStyle().Foreground(colorSecondary).Render("project:"+t.NoteProject))
			}
		}
		parts = append(parts, helpStyle.Render("f:filter  space:toggle  enter:go to note  Esc:back"))

		left := strings.Join(parts, " │ ")
		gap := m.width - lipgloss.Width(left)
		if gap < 0 {
			gap = 0
		}
		return statusBarStyle.Render(left + strings.Repeat(" ", gap))
	}

	// Note count
	parts = append(parts, fmt.Sprintf("%d notes", len(m.filteredNotes)))

	// Current note info
	if len(m.filteredNotes) > 0 && m.listCursor < len(m.filteredNotes) {
		n := m.filteredNotes[m.listCursor]
		if n.Date != "" {
			parts = append(parts, n.Date)
		}
		parts = append(parts, fmt.Sprintf("%d words", n.WordCount))
	}

	// Active tag filter
	if m.activeTag != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorSecondary).Render("tag:"+m.activeTag))
	}

	// Sort mode (show if not default)
	if m.sortMode != sortDate {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render("sort:"+m.sortMode))
	}

	// Sync/AI status
	if m.syncing {
		parts = append(parts, statusActiveStyle.Render("syncing..."))
	}
	if m.aiPending > 0 {
		parts = append(parts, statusActiveStyle.Render(fmt.Sprintf("AI: %d pending", m.aiPending)))
	}

	// Status message
	if m.statusMsg != "" {
		parts = append(parts, m.statusMsg)
	}

	left := strings.Join(parts, " │ ")
	right := helpStyle.Render("? help │ q quit")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	return statusBarStyle.Render(left + strings.Repeat(" ", gap) + right)
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

func (m Model) helpView() string {
	help := `
  ┌─────────────────────────────────────┐
  │           Moss - Help               │
  ├─────────────────────────────────────┤
  │                                     │
  │  Navigation                         │
  │    j/k, ↑/↓     Move up/down       │
  │    h/l, ←/→     Switch panes       │
  │    Tab           Next pane          │
  │    Ctrl+d/u      Scroll half page   │
  │    Enter         Open in editor     │
  │                                     │
  │  Actions                            │
  │    n             New note           │
  │    d             Delete note        │
  │    g             Generate AI note   │
  │    /             Search notes       │
  │    t             Filter by tag      │
  │    o             Cycle sort order   │
  │    c             Chat with AI       │
  │    s             Sync & re-index    │
  │    T             Todos view         │
  │                                     │
  │  Search Syntax                      │
  │    word          Full-text search   │
  │    title:word    Search titles      │
  │    tag:word      Search by tag      │
  │    project:word  Search by project  │
  │    people:word   Search by people   │
  │    status:word   Search by status   │
  │                                     │
  │  General                            │
  │    ?             Toggle help        │
  │    Esc           Cancel / back      │
  │    q             Quit               │
  │                                     │
  │  Todos View (T)                     │
  │    space/x       Toggle todo        │
  │    enter         Jump to note       │
  │    f             Cycle filter       │
  │    Esc           Back to notes      │
  │                                     │
  │  List Indicators                    │
  │    *  AI generated note             │
  │    +  Contains TODOs                │
  │                                     │
  └─────────────────────────────────────┘
`
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Foreground(colorFg).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Render(help),
	)
}

// SetWatcher attaches a file watcher to the model.
func (m *Model) SetWatcher(w *msync.Watcher) {
	m.watcher = w
}
