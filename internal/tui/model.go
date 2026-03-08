package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

type editorFinishedMsg struct {
	err error
}

type errMsg struct {
	err error
}

type clearStatusMsg struct{}

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
	width     int
	height    int
	activePane int
	mode       int

	// Note list
	notes       []*note.Note
	filteredNotes []*note.Note
	listCursor  int
	listOffset  int

	// Preview
	preview  viewport.Model
	previewContent string

	// Chat
	chatInput    textinput.Model
	chatHistory  []chatMessage
	chatViewport viewport.Model

	// Search
	searchInput textinput.Model

	// Status
	statusMsg   string
	aiPending   int
	syncing     bool

	// Help overlay
	showHelp bool
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

	preview := viewport.New(0, 0)
	chatVp := viewport.New(0, 0)

	return Model{
		cfg:       cfg,
		database:  database,
		worker:    worker,
		chatInput: ti,
		searchInput: si,
		preview:   preview,
		chatViewport: chatVp,
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
		m.statusMsg = fmt.Sprintf("Synced %d notes", len(msg.notes))
		clearCmd := clearStatusAfter(3 * time.Second)
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

	case editorFinishedMsg:
		// Re-sync after editing
		return m, syncNotes(m.cfg.NotesDir, m.database)

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case errMsg:
		m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		return m, clearStatusAfter(5 * time.Second)

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
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global escape
	if key == "esc" {
		if m.mode == modeSearch {
			m.mode = modeNormal
			m.searchInput.Blur()
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
			query := m.searchInput.Value()
			m.mode = modeNormal
			m.searchInput.Blur()
			if query == "" {
				m.filteredNotes = m.notes
			} else {
				results, err := m.database.Search(query)
				if err != nil {
					m.statusMsg = fmt.Sprintf("Search error: %v", err)
					m.filteredNotes = m.notes
				} else {
					m.filteredNotes = results
					if len(results) == 0 {
						m.statusMsg = "No notes found"
					} else {
						m.statusMsg = fmt.Sprintf("Found %d notes", len(results))
					}
				}
			}
			m.listCursor = 0
			m.listOffset = 0
			clearCmd := clearStatusAfter(3 * time.Second)
			if len(m.filteredNotes) > 0 {
				return m, tea.Batch(renderPreview(m.filteredNotes[0]), clearCmd)
			}
			// Clear preview when no results
			m.previewContent = ""
			m.preview.SetContent("")
			return m, clearCmd
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
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

	// Normal mode
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.activePane = (m.activePane + 1) % 3
		return m, nil

	case "h", "left":
		if m.activePane > 0 {
			m.activePane--
		}
		return m, nil

	case "l", "right":
		if m.activePane < 2 {
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
		return m, textinput.Blink

	case "c":
		m.mode = modeChat
		m.activePane = paneChat
		m.chatInput.Focus()
		return m, textinput.Blink

	case "n":
		return m, m.createNewNote()

	case "s":
		m.syncing = true
		m.statusMsg = "Syncing..."
		return m, syncNotes(m.cfg.NotesDir, m.database)
	}

	return m, nil
}

func (m *Model) createNewNote() tea.Cmd {
	return func() tea.Msg {
		path, err := note.CreateNew(m.cfg.NotesDir, "untitled")
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

func (m *Model) updateLayout() {
	previewWidth := m.previewWidth()
	chatWidth := m.chatWidth()
	contentHeight := m.height - 4 // status bar + borders

	m.preview.Width = previewWidth - 4
	m.preview.Height = contentHeight - 2

	m.chatViewport.Width = chatWidth - 4
	m.chatViewport.Height = contentHeight - 6 // leave room for input

	if m.previewContent != "" {
		m.preview.SetContent(m.previewContent)
	}
}

func (m Model) listWidth() int {
	return int(float64(m.width) * 0.22)
}

func (m Model) previewWidth() int {
	return int(float64(m.width) * 0.46)
}

func (m Model) chatWidth() int {
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
	chatW := m.chatWidth()
	contentHeight := m.height - 2 // status bar

	// Render panes
	listPane := m.renderListPane(listW, contentHeight)
	previewPane := m.renderPreviewPane(previewW, contentHeight)
	chatPane := m.renderChatPane(chatW, contentHeight)

	// Join panes horizontally
	body := lipgloss.JoinHorizontal(lipgloss.Top, listPane, previewPane, chatPane)

	// Status bar
	status := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

func (m Model) renderListPane(width, height int) string {
	style := paneStyle
	if m.activePane == paneList {
		style = activePaneStyle
	}

	title := titleStyle.Render("Notes")

	// Search bar
	var searchBar string
	if m.mode == modeSearch {
		searchBar = m.searchInput.View()
	}

	// Note list
	listH := m.listHeight()
	var items []string
	for i := m.listOffset; i < len(m.filteredNotes) && i < m.listOffset+listH; i++ {
		n := m.filteredNotes[i]
		display := n.Title
		if width > 9 && len(display) > width-6 {
			display = display[:width-9] + "..."
		} else if width <= 9 && len(display) > 3 {
			display = display[:3] + "..."
		}

		if i == m.listCursor {
			items = append(items, selectedItemStyle.Render("▸ "+display))
		} else {
			items = append(items, normalItemStyle.Render("  "+display))
		}
	}

	content := strings.Join(items, "\n")
	if searchBar != "" {
		content = searchBar + "\n" + content
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
	if len(m.filteredNotes) == 0 {
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
  │    /             Search notes       │
  │    c             Chat with AI       │
  │    s             Sync & re-index    │
  │                                     │
  │  General                            │
  │    ?             Toggle help        │
  │    Esc           Cancel / back      │
  │    q             Quit               │
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
