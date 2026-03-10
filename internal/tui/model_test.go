package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/devenjarvis/moss/internal/ai"
	"github.com/devenjarvis/moss/internal/config"
	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := config.Config{
		NotesDir: t.TempDir(),
		DBPath:   dbPath,
	}
	worker := ai.NewWorker(10)

	m := New(cfg, database, worker)
	// Set a reasonable window size
	m.width = 120
	m.height = 40
	m.updateLayout()
	// Clear startup input suppression for tests
	m.suppressInputUntil = time.Time{}
	return m
}

func newTestModelWithNotes(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	notes := []*note.Note{
		{FilePath: "/notes/a.md", Title: "Alpha", Date: "2024-01-03", Body: "Alpha body", WordCount: 2},
		{FilePath: "/notes/b.md", Title: "Beta", Date: "2024-01-02", Body: "Beta body", WordCount: 2},
		{FilePath: "/notes/c.md", Title: "Charlie", Date: "2024-01-01", Body: "Charlie body", WordCount: 2},
	}
	m.notes = notes
	m.filteredNotes = notes
	return m
}

func keyMsg(key string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
}

func specialKeyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// --- Pane Navigation Tests ---

func TestPaneNavigation_Tab(t *testing.T) {
	m := newTestModelWithNotes(t)

	if m.activePane != paneList {
		t.Fatalf("initial activePane = %d, want %d (paneList)", m.activePane, paneList)
	}

	// Tab cycles through panes
	model, _ := m.Update(keyMsg("tab"))
	m = model.(Model)
	if m.activePane != panePreview {
		t.Errorf("after 1st tab: activePane = %d, want %d (panePreview)", m.activePane, panePreview)
	}

	model, _ = m.Update(keyMsg("tab"))
	m = model.(Model)
	if m.activePane != paneChat {
		t.Errorf("after 2nd tab: activePane = %d, want %d (paneChat)", m.activePane, paneChat)
	}

	model, _ = m.Update(keyMsg("tab"))
	m = model.(Model)
	if m.activePane != paneList {
		t.Errorf("after 3rd tab: activePane = %d, want %d (paneList, wraps around)", m.activePane, paneList)
	}
}

func TestPaneNavigation_LeftRight(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Right moves forward
	model, _ := m.Update(keyMsg("l"))
	m = model.(Model)
	if m.activePane != panePreview {
		t.Errorf("after 'l': activePane = %d, want %d", m.activePane, panePreview)
	}

	model, _ = m.Update(keyMsg("l"))
	m = model.(Model)
	if m.activePane != paneChat {
		t.Errorf("after 2nd 'l': activePane = %d, want %d", m.activePane, paneChat)
	}

	// Right at rightmost does nothing
	model, _ = m.Update(keyMsg("l"))
	m = model.(Model)
	if m.activePane != paneChat {
		t.Errorf("after 3rd 'l': activePane = %d, should stay at %d", m.activePane, paneChat)
	}

	// Left moves back
	model, _ = m.Update(keyMsg("h"))
	m = model.(Model)
	if m.activePane != panePreview {
		t.Errorf("after 'h': activePane = %d, want %d", m.activePane, panePreview)
	}

	model, _ = m.Update(keyMsg("h"))
	m = model.(Model)
	if m.activePane != paneList {
		t.Errorf("after 2nd 'h': activePane = %d, want %d", m.activePane, paneList)
	}

	// Left at leftmost does nothing
	model, _ = m.Update(keyMsg("h"))
	m = model.(Model)
	if m.activePane != paneList {
		t.Errorf("after 3rd 'h': activePane = %d, should stay at %d", m.activePane, paneList)
	}
}

// --- List Cursor Tests ---

func TestListCursor_UpDown(t *testing.T) {
	m := newTestModelWithNotes(t)

	if m.listCursor != 0 {
		t.Fatalf("initial listCursor = %d, want 0", m.listCursor)
	}

	// Move down
	model, _ := m.Update(keyMsg("j"))
	m = model.(Model)
	if m.listCursor != 1 {
		t.Errorf("after 'j': listCursor = %d, want 1", m.listCursor)
	}

	model, _ = m.Update(keyMsg("j"))
	m = model.(Model)
	if m.listCursor != 2 {
		t.Errorf("after 2nd 'j': listCursor = %d, want 2", m.listCursor)
	}

	// Can't go past the end
	model, _ = m.Update(keyMsg("j"))
	m = model.(Model)
	if m.listCursor != 2 {
		t.Errorf("after 3rd 'j': listCursor = %d, should stay at 2", m.listCursor)
	}

	// Move up
	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.listCursor != 1 {
		t.Errorf("after 'k': listCursor = %d, want 1", m.listCursor)
	}

	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.listCursor != 0 {
		t.Errorf("after 2nd 'k': listCursor = %d, want 0", m.listCursor)
	}

	// Can't go above 0
	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.listCursor != 0 {
		t.Errorf("after 3rd 'k': listCursor = %d, should stay at 0", m.listCursor)
	}
}

func TestListCursor_EmptyList(t *testing.T) {
	m := newTestModel(t)
	m.notes = nil
	m.filteredNotes = nil

	// Moving in empty list should not panic
	model, _ := m.Update(keyMsg("j"))
	m = model.(Model)
	if m.listCursor != 0 {
		t.Errorf("listCursor = %d, want 0", m.listCursor)
	}

	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.listCursor != 0 {
		t.Errorf("listCursor = %d, want 0", m.listCursor)
	}
}

// --- Mode Transition Tests ---

func TestModeTransition_Search(t *testing.T) {
	m := newTestModelWithNotes(t)

	if m.mode != modeNormal {
		t.Fatalf("initial mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}

	// Enter search mode
	model, _ := m.Update(keyMsg("/"))
	m = model.(Model)
	if m.mode != modeSearch {
		t.Errorf("after '/': mode = %d, want %d (modeSearch)", m.mode, modeSearch)
	}

	// Escape returns to normal
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)
	if m.mode != modeNormal {
		t.Errorf("after Esc: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestModeTransition_Chat(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Enter chat mode
	model, _ := m.Update(keyMsg("c"))
	m = model.(Model)
	if m.mode != modeChat {
		t.Errorf("after 'c': mode = %d, want %d (modeChat)", m.mode, modeChat)
	}
	if m.activePane != paneChat {
		t.Errorf("after 'c': activePane = %d, want %d (paneChat)", m.activePane, paneChat)
	}

	// Escape returns to normal
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)
	if m.mode != modeNormal {
		t.Errorf("after Esc: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestModeTransition_Help(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Toggle help on
	model, _ := m.Update(keyMsg("?"))
	m = model.(Model)
	if !m.showHelp {
		t.Error("after '?': showHelp should be true")
	}

	// Toggle help off with ?
	model, _ = m.Update(keyMsg("?"))
	m = model.(Model)
	if m.showHelp {
		t.Error("after 2nd '?': showHelp should be false")
	}

	// Toggle help on, then dismiss with Esc
	model, _ = m.Update(keyMsg("?"))
	m = model.(Model)
	if !m.showHelp {
		t.Fatal("showHelp should be true")
	}
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)
	if m.showHelp {
		t.Error("after Esc: showHelp should be false")
	}
}

// --- Message Handling Tests ---

func TestNotesLoadedMsg(t *testing.T) {
	m := newTestModel(t)

	notes := []*note.Note{
		{Title: "Note 1", Date: "2024-01-01", Body: "Body 1"},
		{Title: "Note 2", Date: "2024-01-02", Body: "Body 2"},
	}

	model, _ := m.Update(notesLoadedMsg{notes: notes})
	m = model.(Model)

	if len(m.notes) != 2 {
		t.Errorf("notes count = %d, want 2", len(m.notes))
	}
	if len(m.filteredNotes) != 2 {
		t.Errorf("filteredNotes count = %d, want 2", len(m.filteredNotes))
	}
}

func TestNotesLoadedMsg_Empty(t *testing.T) {
	m := newTestModel(t)

	model, cmd := m.Update(notesLoadedMsg{notes: nil})
	m = model.(Model)

	if len(m.notes) != 0 {
		t.Errorf("notes count = %d, want 0", len(m.notes))
	}
	if cmd != nil {
		t.Error("should not issue preview command for empty notes")
	}
}

func TestSyncCompleteMsg(t *testing.T) {
	m := newTestModelWithNotes(t)

	newNotes := []*note.Note{
		{Title: "Synced Note", Date: "2024-02-01", Body: "Synced body", WordCount: 2},
	}

	model, _ := m.Update(syncCompleteMsg{notes: newNotes})
	m = model.(Model)

	if m.syncing {
		t.Error("syncing should be false after sync complete")
	}
	if len(m.notes) != 1 {
		t.Errorf("notes count = %d, want 1", len(m.notes))
	}
	if m.statusMsg == "" {
		t.Error("statusMsg should be set after sync")
	}
}

func TestSyncCompleteMsg_Error(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.syncing = true

	model, _ := m.Update(syncCompleteMsg{err: fmt.Errorf("sync failed")})
	m = model.(Model)

	if m.syncing {
		t.Error("syncing should be false after error")
	}
	if m.statusMsg == "" {
		t.Error("statusMsg should contain error message")
	}
}

func TestSyncCompleteMsg_CursorClamping(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.listCursor = 2 // pointing at third note

	// Sync returns only one note - cursor should be clamped
	newNotes := []*note.Note{
		{Title: "Only Note", Date: "2024-01-01", Body: "Body", WordCount: 1},
	}

	model, _ := m.Update(syncCompleteMsg{notes: newNotes})
	m = model.(Model)

	if m.listCursor != 0 {
		t.Errorf("listCursor = %d, want 0 (clamped to valid range)", m.listCursor)
	}
}

func TestSyncCompleteMsg_EmptyResult(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.listCursor = 1

	model, _ := m.Update(syncCompleteMsg{notes: nil})
	m = model.(Model)

	if m.listCursor != 0 {
		t.Errorf("listCursor = %d, want 0", m.listCursor)
	}
	if m.previewContent != "" {
		t.Error("previewContent should be empty when no notes")
	}
}

func TestAiResponseMsg(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.aiPending = 1

	model, _ := m.Update(aiResponseMsg{response: "AI says hello"})
	m = model.(Model)

	if m.aiPending != 0 {
		t.Errorf("aiPending = %d, want 0", m.aiPending)
	}
	if len(m.chatHistory) != 1 {
		t.Fatalf("chatHistory len = %d, want 1", len(m.chatHistory))
	}
	if m.chatHistory[0].role != "assistant" {
		t.Errorf("role = %q, want 'assistant'", m.chatHistory[0].role)
	}
	if m.chatHistory[0].content != "AI says hello" {
		t.Errorf("content = %q, want 'AI says hello'", m.chatHistory[0].content)
	}
}

func TestAiResponseMsg_Error(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.aiPending = 1

	model, _ := m.Update(aiResponseMsg{err: fmt.Errorf("AI error")})
	m = model.(Model)

	if m.aiPending != 0 {
		t.Errorf("aiPending = %d, want 0", m.aiPending)
	}
	if len(m.chatHistory) != 1 {
		t.Fatalf("chatHistory len = %d, want 1", len(m.chatHistory))
	}
	if m.chatHistory[0].role != "assistant" {
		t.Errorf("role = %q, want 'assistant'", m.chatHistory[0].role)
	}
}

func TestClearStatusMsg(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.statusMsg = "Some status"

	model, _ := m.Update(clearStatusMsg{})
	m = model.(Model)

	if m.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty", m.statusMsg)
	}
}

func TestErrMsg(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(errMsg{err: fmt.Errorf("something broke")})
	m = model.(Model)

	if m.statusMsg == "" {
		t.Error("statusMsg should contain error message")
	}
}

func TestNotePreviewMsg(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(notePreviewMsg{content: "# Preview Content"})
	m = model.(Model)

	if m.previewContent != "# Preview Content" {
		t.Errorf("previewContent = %q, want '# Preview Content'", m.previewContent)
	}
}

// --- Window Resize Tests ---

func TestWindowSizeMsg(t *testing.T) {
	m := newTestModel(t)

	model, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = model.(Model)

	if m.width != 200 {
		t.Errorf("width = %d, want 200", m.width)
	}
	if m.height != 60 {
		t.Errorf("height = %d, want 60", m.height)
	}
}

// --- View Tests ---

func TestView_Loading(t *testing.T) {
	m := newTestModel(t)
	m.width = 0 // zero width triggers "Loading..."

	view := m.View().Content
	if view != "Loading..." {
		t.Errorf("View() = %q, want 'Loading...'", view)
	}
}

func TestView_NoPanic(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Should not panic
	view := m.View().Content
	if view == "" {
		t.Error("View() should not return empty string")
	}
}

func TestView_HelpOverlay(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.showHelp = true

	view := m.View().Content
	if view == "" {
		t.Error("help view should not be empty")
	}
}

func TestView_EmptyNotes(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.updateLayout()

	// Should not panic with empty notes
	view := m.View().Content
	if view == "" {
		t.Error("View() should not return empty string")
	}
}

// --- Sync Trigger Tests ---

func TestSyncKey(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, cmd := m.Update(keyMsg("s"))
	m = model.(Model)

	if !m.syncing {
		t.Error("syncing should be true after pressing 's'")
	}
	if m.statusMsg != "Syncing..." {
		t.Errorf("statusMsg = %q, want 'Syncing...'", m.statusMsg)
	}
	if cmd == nil {
		t.Error("should return a sync command")
	}
}

// --- Quit Tests ---

func TestQuitKey(t *testing.T) {
	m := newTestModelWithNotes(t)

	_, cmd := m.Update(keyMsg("q"))

	// cmd should be a quit command
	if cmd == nil {
		t.Error("pressing 'q' should return a command")
	}
}

// --- Search Mode Behavior Tests ---

func TestSearchMode_EscapeRestoresNotes(t *testing.T) {
	m := newTestModelWithNotes(t)
	originalCount := len(m.notes)

	// Enter search mode
	model, _ := m.Update(keyMsg("/"))
	m = model.(Model)

	// Simulate having filtered notes
	m.filteredNotes = m.notes[:1]

	// Escape should restore all notes
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if len(m.filteredNotes) != originalCount {
		t.Errorf("filteredNotes = %d, want %d (restored after Esc)", len(m.filteredNotes), originalCount)
	}
}

func TestSearchMode_EnterKeepsResults(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Enter search mode
	model, _ := m.Update(keyMsg("/"))
	m = model.(Model)

	if m.mode != modeSearch {
		t.Fatalf("mode = %d, want modeSearch", m.mode)
	}

	// Simulate having filtered results (as if live search ran)
	m.filteredNotes = m.notes[:1]
	m.searchQuery = "alpha"
	m.searchTerms = []string{"alpha"}

	// Press Enter — should exit search, keep results
	model, _ = m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal after Enter", m.mode)
	}
	if len(m.filteredNotes) != 1 {
		t.Errorf("filteredNotes = %d, want 1 (should keep search results)", len(m.filteredNotes))
	}
}

func TestSearchMode_EscapePreservesTagFilter(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Set active tag
	m.activeTag = "go"

	// Enter search mode
	model, _ := m.Update(keyMsg("/"))
	m = model.(Model)

	// Escape should preserve activeTag and re-filter by tag (not show all notes)
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.activeTag != "go" {
		t.Errorf("activeTag = %q, want 'go' (should preserve tag filter)", m.activeTag)
	}
	if m.searchQuery != "" {
		t.Errorf("searchQuery = %q, want empty after Esc", m.searchQuery)
	}
	if m.searchTerms != nil {
		t.Errorf("searchTerms = %v, want nil after Esc", m.searchTerms)
	}
}

func TestSearchMode_SlashClearsSearchState(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Set previous search state
	m.searchQuery = "old"
	m.searchTerms = []string{"old"}

	// Enter search mode
	model, _ := m.Update(keyMsg("/"))
	m = model.(Model)

	if m.searchQuery != "" {
		t.Errorf("searchQuery = %q, want empty when entering search", m.searchQuery)
	}
	if m.searchTerms != nil {
		t.Errorf("searchTerms = %v, want nil when entering search", m.searchTerms)
	}
}

func TestExtractSearchTerms(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"plain terms", "hello world", []string{"hello", "world"}},
		{"field prefix extracts value", "title:meeting", []string{"meeting"}},
		{"mixed", "notes title:standup", []string{"notes", "standup"}},
		{"trailing colon ignored", "title:", nil},
		{"quoted value unquoted", `project:myproject`, []string{"myproject"}},
		{"empty", "", nil},
		{"unknown prefix extracts value", "foo:bar", []string{"bar"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSearchTerms(tt.query)
			if len(got) != len(tt.want) {
				t.Fatalf("extractSearchTerms(%q) = %v, want %v", tt.query, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("term[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHighlightMatches(t *testing.T) {
	t.Run("no terms returns original", func(t *testing.T) {
		result := highlightMatches("hello world", nil)
		if result != "hello world" {
			t.Errorf("got %q, want original text", result)
		}
	})

	t.Run("contains all text", func(t *testing.T) {
		result := highlightMatches("hello world", []string{"hello"})
		// In test environments lipgloss may not emit ANSI codes,
		// but the result should still contain all original text
		if !strings.Contains(result, "hello") {
			t.Error("result should contain 'hello'")
		}
		if !strings.Contains(result, " world") {
			t.Error("result should contain ' world'")
		}
	})

	t.Run("case insensitive preserves original case", func(t *testing.T) {
		result := highlightMatches("Hello World", []string{"hello"})
		if !strings.Contains(result, "Hello") {
			t.Error("should preserve original casing of 'Hello'")
		}
	})

	t.Run("no match returns original", func(t *testing.T) {
		result := highlightMatches("hello world", []string{"xyz"})
		if result != "hello world" {
			t.Errorf("got %q, want original text when no match", result)
		}
	})

	t.Run("multiple terms preserves all text", func(t *testing.T) {
		result := highlightMatches("hello beautiful world", []string{"hello", "world"})
		if !strings.Contains(result, "hello") {
			t.Error("should contain 'hello'")
		}
		if !strings.Contains(result, "beautiful") {
			t.Error("should contain 'beautiful'")
		}
		if !strings.Contains(result, "world") {
			t.Error("should contain 'world'")
		}
	})

	t.Run("empty text", func(t *testing.T) {
		result := highlightMatches("", []string{"hello"})
		if result != "" {
			t.Errorf("got %q for empty text, want empty", result)
		}
	})

	t.Run("empty terms", func(t *testing.T) {
		result := highlightMatches("hello", []string{})
		if result != "hello" {
			t.Errorf("got %q, want original with empty terms", result)
		}
	})
}

// --- Layout Tests ---

func TestLayoutWidths(t *testing.T) {
	m := newTestModel(t)
	m.width = 100
	m.height = 40

	listW := m.listWidth()
	previewW := m.previewWidth()
	chatW := m.chatWidth()

	if listW+previewW+chatW != m.width {
		t.Errorf("pane widths sum to %d, want %d", listW+previewW+chatW, m.width)
	}

	// List should be ~22%
	if listW != 22 {
		t.Errorf("listWidth = %d, expected ~22", listW)
	}
	// Preview should be ~46%
	if previewW != 46 {
		t.Errorf("previewWidth = %d, expected ~46", previewW)
	}
}

func TestListHeight(t *testing.T) {
	m := newTestModel(t)
	m.height = 40

	h := m.listHeight()
	if h != 34 { // 40 - 6
		t.Errorf("listHeight() = %d, want 34", h)
	}
}

func TestListHeight_Minimum(t *testing.T) {
	m := newTestModel(t)
	m.height = 3 // very small

	h := m.listHeight()
	if h < 1 {
		t.Errorf("listHeight() = %d, should be at least 1", h)
	}
}

// --- EnsureListVisible Tests ---

func TestEnsureListVisible(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.height = 10 // small height so listHeight = max(10-6, 1) = 4

	// Add many notes
	notes := make([]*note.Note, 20)
	for i := range notes {
		notes[i] = &note.Note{
			Title: fmt.Sprintf("Note %d", i),
			Date:  "2024-01-01",
			Body:  "body",
		}
	}
	m.notes = notes
	m.filteredNotes = notes

	// Move cursor down past visible area
	m.listCursor = 10
	m.ensureListVisible()

	listH := m.listHeight()
	if m.listOffset+listH <= m.listCursor {
		t.Errorf("cursor %d not visible: offset=%d, height=%d", m.listCursor, m.listOffset, listH)
	}

	// Move cursor back up
	m.listCursor = 0
	m.ensureListVisible()
	if m.listOffset > 0 {
		t.Errorf("offset should be 0 when cursor is at top, got %d", m.listOffset)
	}
}

// --- Rendering Helper Tests ---

func TestClearStatusAfter(t *testing.T) {
	cmd := clearStatusAfter(10 * time.Millisecond)
	if cmd == nil {
		t.Error("clearStatusAfter should return a command")
	}
}

// --- Chat Viewport Tests ---

func TestUpdateChatViewport(t *testing.T) {
	m := newTestModelWithNotes(t)

	m.chatHistory = []chatMessage{
		{role: "user", content: "Hello"},
		{role: "assistant", content: "Hi there"},
	}
	m.updateChatViewport()

	// Should not panic and viewport should have content set
}

func TestUpdateChatViewport_WithPending(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.aiPending = 1
	m.updateChatViewport()
	// Should not panic - should show "Thinking..."
}

// --- SetWatcher Tests ---

func TestSetWatcher(t *testing.T) {
	m := newTestModel(t)

	if m.watcher != nil {
		t.Error("initial watcher should be nil")
	}

	// SetWatcher is a pointer receiver method, test that it works
	// We can't easily test with a real watcher without more setup,
	// but we can verify nil doesn't panic
	m.SetWatcher(nil)
}

// --- Keys Not Active in Wrong Mode Tests ---

// --- Confirm Mode Tests ---

func TestConfirmMode_DeleteTrigger(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Press 'd' to enter confirm mode
	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	if m.mode != modeConfirm {
		t.Errorf("after 'd': mode = %d, want %d (modeConfirm)", m.mode, modeConfirm)
	}
	if m.confirmMsg == "" {
		t.Error("confirmMsg should be set")
	}
	if m.confirmAction == nil {
		t.Error("confirmAction should be set")
	}
}

func TestConfirmMode_DismissWithN(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Enter confirm mode
	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	// Press 'n' to cancel
	model, _ = m.Update(keyMsg("n"))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("after 'n': mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
	if m.confirmMsg != "" {
		t.Error("confirmMsg should be cleared after cancel")
	}
	if m.confirmAction != nil {
		t.Error("confirmAction should be cleared after cancel")
	}
}

func TestConfirmMode_DismissWithEsc(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("after Esc: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestConfirmMode_AcceptWithY(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	// Press 'y' to confirm
	model, cmd := m.Update(keyMsg("y"))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("after 'y': mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
	if cmd == nil {
		t.Error("confirming should return a command (delete action)")
	}
}

func TestConfirmMode_IgnoresOtherKeys(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	// Pressing random keys should stay in confirm mode
	for _, key := range []string{"q", "j", "k", "/"} {
		model, _ = m.Update(keyMsg(key))
		m = model.(Model)
		if m.mode != modeConfirm {
			t.Errorf("after %q in confirm mode: mode = %d, want %d (modeConfirm)", key, m.mode, modeConfirm)
		}
	}
}

func TestConfirmMode_EmptyList(t *testing.T) {
	m := newTestModel(t)
	m.notes = nil
	m.filteredNotes = nil

	// Press 'd' with no notes - should stay in normal mode
	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("'d' with empty list should stay in modeNormal, got %d", m.mode)
	}
}

// --- Delete Message Tests ---

func TestDeleteNoteMsg_Success(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(deleteNoteMsg{})
	m = model.(Model)

	if m.statusMsg != "Note deleted" {
		t.Errorf("statusMsg = %q, want 'Note deleted'", m.statusMsg)
	}
	if !m.syncing {
		t.Error("should trigger re-sync after deletion")
	}
}

func TestDeleteNoteMsg_Error(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(deleteNoteMsg{err: fmt.Errorf("permission denied")})
	m = model.(Model)

	if m.statusMsg == "" {
		t.Error("statusMsg should contain error")
	}
}

// --- New Note Mode Tests ---

func TestNewNoteMode_Enter(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("n"))
	m = model.(Model)

	if m.mode != modeNewNote {
		t.Errorf("after 'n': mode = %d, want %d (modeNewNote)", m.mode, modeNewNote)
	}
}

func TestNewNoteMode_EscapeCancels(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("n"))
	m = model.(Model)

	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("after Esc: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestNewNoteMode_KeysNotInterpretedAsCommands(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("n"))
	m = model.(Model)

	// Typing 'q' should not quit
	model, cmd := m.Update(keyMsg("q"))
	m = model.(Model)

	if m.mode != modeNewNote {
		t.Errorf("after 'q' in new note mode: mode = %d, want %d (modeNewNote)", m.mode, modeNewNote)
	}
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("'q' in new note mode should not quit")
		}
	}
}

// --- Generate Mode Tests ---

func TestGenerateMode_Enter(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("g"))
	m = model.(Model)

	if m.mode != modeGenerate {
		t.Errorf("after 'g': mode = %d, want %d (modeGenerate)", m.mode, modeGenerate)
	}
}

func TestGenerateMode_EscapeCancels(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("g"))
	m = model.(Model)

	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("after Esc: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestGenerateMode_EmptyPromptDoesNothing(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(keyMsg("g"))
	m = model.(Model)

	// Submit empty prompt
	model, _ = m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
	if m.aiPending != 0 {
		t.Errorf("aiPending = %d, want 0 (empty prompt should not generate)", m.aiPending)
	}
}

func TestGenerateCompleteMsg_Success(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.aiPending = 1

	model, _ := m.Update(generateCompleteMsg{path: "/notes/generated.md"})
	m = model.(Model)

	if m.aiPending != 0 {
		t.Errorf("aiPending = %d, want 0", m.aiPending)
	}
	if m.statusMsg == "" {
		t.Error("statusMsg should contain generated path")
	}
	if !m.syncing {
		t.Error("should trigger re-sync after generation")
	}
}

func TestGenerateCompleteMsg_Error(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.aiPending = 1

	model, _ := m.Update(generateCompleteMsg{err: fmt.Errorf("AI unavailable")})
	m = model.(Model)

	if m.aiPending != 0 {
		t.Errorf("aiPending = %d, want 0", m.aiPending)
	}
	if m.statusMsg == "" {
		t.Error("statusMsg should contain error")
	}
}

// --- Tag Filter Mode Tests ---

func TestTagFilterMode_Enter(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, cmd := m.Update(keyMsg("t"))
	m = model.(Model)

	if m.mode != modeTagFilter {
		t.Errorf("after 't': mode = %d, want %d (modeTagFilter)", m.mode, modeTagFilter)
	}
	if cmd == nil {
		t.Error("should return commands (blink + load tags)")
	}
}

func TestTagFilterMode_EscapeClearsFilter(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.activeTag = "go" // pretend a tag filter is active
	m.filteredNotes = m.notes[:1]

	// Enter tag filter mode
	model, _ := m.Update(keyMsg("t"))
	m = model.(Model)

	// Escape should clear the tag filter
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
	if m.activeTag != "" {
		t.Errorf("activeTag = %q, should be cleared", m.activeTag)
	}
	if len(m.filteredNotes) != len(m.notes) {
		t.Error("filteredNotes should be restored to all notes")
	}
}

func TestTagsLoadedMsg(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(tagsLoadedMsg{tags: []string{"go", "rust", "web"}})
	m = model.(Model)

	if len(m.allTags) != 3 {
		t.Errorf("allTags = %d, want 3", len(m.allTags))
	}
}

// --- Sort Cycling Tests ---

func TestSortCycling(t *testing.T) {
	m := newTestModelWithNotes(t)

	if m.sortMode != sortDate {
		t.Fatalf("initial sortMode = %q, want %q", m.sortMode, sortDate)
	}

	// Cycle: date -> title
	model, _ := m.Update(keyMsg("o"))
	m = model.(Model)
	if m.sortMode != sortTitle {
		t.Errorf("after 1st 'o': sortMode = %q, want %q", m.sortMode, sortTitle)
	}

	// title -> modified
	model, _ = m.Update(keyMsg("o"))
	m = model.(Model)
	if m.sortMode != sortModified {
		t.Errorf("after 2nd 'o': sortMode = %q, want %q", m.sortMode, sortModified)
	}

	// modified -> words
	model, _ = m.Update(keyMsg("o"))
	m = model.(Model)
	if m.sortMode != sortWords {
		t.Errorf("after 3rd 'o': sortMode = %q, want %q", m.sortMode, sortWords)
	}

	// words -> date (wraps around)
	model, _ = m.Update(keyMsg("o"))
	m = model.(Model)
	if m.sortMode != sortDate {
		t.Errorf("after 4th 'o': sortMode = %q, want %q (should wrap)", m.sortMode, sortDate)
	}
}

func TestSortCycling_ResetsCursor(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.listCursor = 2
	m.listOffset = 1

	model, _ := m.Update(keyMsg("o"))
	m = model.(Model)

	if m.listCursor != 0 {
		t.Errorf("listCursor = %d, want 0 (reset after sort change)", m.listCursor)
	}
	if m.listOffset != 0 {
		t.Errorf("listOffset = %d, want 0 (reset after sort change)", m.listOffset)
	}
}

// --- Responsive Pane Layout Tests ---

func TestResponsiveLayout_NarrowHidesChat(t *testing.T) {
	m := newTestModel(t)

	// Simulate narrow terminal
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = model.(Model)

	if m.chatVisible {
		t.Error("chatVisible should be false on narrow terminal (< 100)")
	}
	if m.chatWidth() != 0 {
		t.Errorf("chatWidth() = %d, want 0 when chat hidden", m.chatWidth())
	}
}

func TestResponsiveLayout_WideShowsChat(t *testing.T) {
	m := newTestModel(t)

	model, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = model.(Model)

	if !m.chatVisible {
		t.Error("chatVisible should be true on wide terminal (>= 100)")
	}
	if m.chatWidth() == 0 {
		t.Error("chatWidth() should be > 0 when chat visible")
	}
}

func TestResponsiveLayout_ChatKeyShowsHiddenChat(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Simulate narrow terminal
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = model.(Model)

	if m.chatVisible {
		t.Fatal("chat should be hidden initially on narrow terminal")
	}

	// Press 'c' to show chat
	model, _ = m.Update(keyMsg("c"))
	m = model.(Model)

	if !m.chatVisible {
		t.Error("chatVisible should be true after pressing 'c'")
	}
	if m.mode != modeChat {
		t.Errorf("mode = %d, want %d (modeChat)", m.mode, modeChat)
	}
}

func TestResponsiveLayout_TabSkipsHiddenChat(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Simulate narrow terminal
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = model.(Model)

	// Tab from list should go to preview
	model, _ = m.Update(keyMsg("tab"))
	m = model.(Model)
	if m.activePane != panePreview {
		t.Errorf("after tab: activePane = %d, want %d (panePreview)", m.activePane, panePreview)
	}

	// Tab from preview should wrap to list (skipping hidden chat)
	model, _ = m.Update(keyMsg("tab"))
	m = model.(Model)
	if m.activePane != paneList {
		t.Errorf("after 2nd tab: activePane = %d, want %d (paneList, wraps around)", m.activePane, paneList)
	}
}

func TestResponsiveLayout_RightKeyStopsAtPreview(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Simulate narrow terminal
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = model.(Model)

	// Move right to preview
	model, _ = m.Update(keyMsg("l"))
	m = model.(Model)
	if m.activePane != panePreview {
		t.Errorf("after 'l': activePane = %d, want %d", m.activePane, panePreview)
	}

	// Another right should stay at preview (chat is hidden)
	model, _ = m.Update(keyMsg("l"))
	m = model.(Model)
	if m.activePane != panePreview {
		t.Errorf("after 2nd 'l': activePane = %d, should stay at %d (chat hidden)", m.activePane, panePreview)
	}
}

// --- View Rendering Tests for New Features ---

func TestView_ConfirmDialog(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Enter confirm mode
	model, _ := m.Update(keyMsg("d"))
	m = model.(Model)

	view := m.View().Content
	if view == "" {
		t.Error("View() should not be empty in confirm mode")
	}
}

func TestView_NoPanicNarrowTerminal(t *testing.T) {
	m := newTestModelWithNotes(t)

	model, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = model.(Model)

	// Should not panic on narrow terminal
	view := m.View().Content
	if view == "" {
		t.Error("View() should not be empty on narrow terminal")
	}
}

func TestView_HelpOverlayShowsNewKeys(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.showHelp = true

	view := m.View().Content
	// Verify new keybindings are in help
	for _, expected := range []string{"Delete note", "Generate AI note", "Filter by tag", "Cycle sort"} {
		if !strings.Contains(view, expected) {
			t.Errorf("help view should contain %q", expected)
		}
	}
}

func TestStatusBar_ShowsActiveTag(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.activeTag = "go"

	view := m.View().Content
	if !strings.Contains(view, "tag:go") {
		t.Error("status bar should show active tag filter")
	}
}

func TestStatusBar_ShowsSortMode(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.sortMode = sortTitle

	view := m.View().Content
	if !strings.Contains(view, "sort:title") {
		t.Error("status bar should show non-default sort mode")
	}
}

func TestStatusBar_HidesDefaultSort(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.sortMode = sortDate

	view := m.View().Content
	if strings.Contains(view, "sort:date") {
		t.Error("status bar should not show default sort mode")
	}
}

// --- Keys Not Active in Wrong Mode Tests ---

func TestNormalKeysIgnoredInSearchMode(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Enter search mode
	model, _ := m.Update(keyMsg("/"))
	m = model.(Model)

	// 'q' in search mode should be typed into search, not quit
	model, cmd := m.Update(keyMsg("q"))
	m = model.(Model)

	// Should still be in search mode
	if m.mode != modeSearch {
		t.Errorf("mode = %d, want %d (modeSearch) - 'q' should not quit in search mode", m.mode, modeSearch)
	}
	// Should not issue quit command
	if cmd != nil {
		// Check it's not a quit command (it should be a textinput update)
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("'q' in search mode should not issue quit")
		}
	}
}

func TestNormalKeysIgnoredInChatMode(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Enter chat mode
	model, _ := m.Update(keyMsg("c"))
	m = model.(Model)

	// 'q' in chat mode should be typed, not quit
	model, cmd := m.Update(keyMsg("q"))
	m = model.(Model)

	if m.mode != modeChat {
		t.Errorf("mode = %d, want %d (modeChat)", m.mode, modeChat)
	}
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("'q' in chat mode should not issue quit")
		}
	}
}

// --- Todo View Tests ---

func newTestModelWithTodos(t *testing.T) Model {
	t.Helper()
	m := newTestModelWithNotes(t)
	m.todos = []note.TodoItem{
		{Text: "Buy groceries", Done: false, LineNumber: 1, FilePath: "/notes/a.md", NoteTitle: "Alpha", NoteDate: "2024-01-03", NoteProject: "personal"},
		{Text: "Write tests", Done: true, LineNumber: 2, FilePath: "/notes/a.md", NoteTitle: "Alpha", NoteDate: "2024-01-03", NoteProject: "moss"},
		{Text: "Review PR", Done: false, LineNumber: 1, FilePath: "/notes/b.md", NoteTitle: "Beta", NoteDate: "2024-01-02", NoteProject: "moss"},
	}
	m.filteredTodos = m.todos
	m.mode = modeTodos
	m.todoFilter = "open"
	return m
}

func TestTodosMode_EnterViaT(t *testing.T) {
	m := newTestModelWithNotes(t)
	if m.mode != modeNormal {
		t.Fatalf("initial mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}

	model, cmd := m.Update(keyMsg("T"))
	m = model.(Model)

	if m.mode != modeTodos {
		t.Errorf("mode = %d, want %d (modeTodos)", m.mode, modeTodos)
	}
	if m.activePane != paneList {
		t.Errorf("activePane = %d, want %d (paneList)", m.activePane, paneList)
	}
	if m.todoCursor != 0 {
		t.Errorf("todoCursor = %d, want 0", m.todoCursor)
	}
	if cmd == nil {
		t.Error("expected cmd to load todos")
	}
}

func TestTodosMode_ExitViaEsc(t *testing.T) {
	m := newTestModelWithTodos(t)

	model, _ := m.Update(specialKeyMsg(tea.KeyEsc))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestTodosMode_CursorNavigation(t *testing.T) {
	m := newTestModelWithTodos(t)

	// Move down
	model, _ := m.Update(keyMsg("j"))
	m = model.(Model)
	if m.todoCursor != 1 {
		t.Errorf("todoCursor = %d, want 1 after j", m.todoCursor)
	}

	// Move down again
	model, _ = m.Update(keyMsg("j"))
	m = model.(Model)
	if m.todoCursor != 2 {
		t.Errorf("todoCursor = %d, want 2 after j", m.todoCursor)
	}

	// At bottom, should not go further
	model, _ = m.Update(keyMsg("j"))
	m = model.(Model)
	if m.todoCursor != 2 {
		t.Errorf("todoCursor = %d, want 2 at bottom", m.todoCursor)
	}

	// Move up
	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.todoCursor != 1 {
		t.Errorf("todoCursor = %d, want 1 after k", m.todoCursor)
	}

	// Move up to top
	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.todoCursor != 0 {
		t.Errorf("todoCursor = %d, want 0 at top", m.todoCursor)
	}

	// At top, should not go further
	model, _ = m.Update(keyMsg("k"))
	m = model.(Model)
	if m.todoCursor != 0 {
		t.Errorf("todoCursor = %d, want 0 at top", m.todoCursor)
	}
}

func TestTodosMode_Toggle(t *testing.T) {
	m := newTestModelWithTodos(t)

	// Space should trigger toggleTodo command
	_, cmd := m.Update(keyMsg(" "))
	if cmd == nil {
		t.Error("expected toggle command on space")
	}

	// x should also trigger toggle
	_, cmd = m.Update(keyMsg("x"))
	if cmd == nil {
		t.Error("expected toggle command on x")
	}
}

func TestTodosMode_FilterCycle(t *testing.T) {
	m := newTestModelWithTodos(t)

	if m.todoFilter != "open" {
		t.Fatalf("initial todoFilter = %q, want 'open'", m.todoFilter)
	}

	// Press f to cycle: open -> done
	model, cmd := m.Update(keyMsg("f"))
	m = model.(Model)
	if m.todoFilter != "done" {
		t.Errorf("todoFilter = %q, want 'done'", m.todoFilter)
	}
	if cmd == nil {
		t.Error("expected loadTodos command after filter cycle")
	}

	// Press f again: done -> all
	model, _ = m.Update(keyMsg("f"))
	m = model.(Model)
	if m.todoFilter != "all" {
		t.Errorf("todoFilter = %q, want 'all'", m.todoFilter)
	}

	// Press f again: all -> open
	model, _ = m.Update(keyMsg("f"))
	m = model.(Model)
	if m.todoFilter != "open" {
		t.Errorf("todoFilter = %q, want 'open'", m.todoFilter)
	}
}

func TestTodosMode_EnterJumpsToNote(t *testing.T) {
	m := newTestModelWithTodos(t)

	// Select second todo (from /notes/a.md)
	model, _ := m.Update(keyMsg("j"))
	m = model.(Model)

	// Press enter to jump to source note
	model, _ = m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestTodosMode_QuitStillWorks(t *testing.T) {
	m := newTestModelWithTodos(t)

	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Error("expected QuitMsg from q in todos mode")
	}
}

func TestTodosMode_NormalKeysIgnored(t *testing.T) {
	m := newTestModelWithTodos(t)

	// 'n' (new note) should not work in todos mode
	model, _ := m.Update(keyMsg("n"))
	m = model.(Model)
	if m.mode != modeTodos {
		t.Errorf("mode = %d, want %d (modeTodos) - 'n' should be ignored in todos mode", m.mode, modeTodos)
	}
}

func TestTodosLoadedMsg(t *testing.T) {
	m := newTestModel(t)
	m.mode = modeTodos

	todos := []note.TodoItem{
		{Text: "Task 1", Done: false, FilePath: "/notes/a.md"},
		{Text: "Task 2", Done: true, FilePath: "/notes/b.md"},
	}

	model, _ := m.Update(todosLoadedMsg{todos: todos})
	m = model.(Model)

	if len(m.todos) != 2 {
		t.Errorf("got %d todos, want 2", len(m.todos))
	}
	if len(m.filteredTodos) != 2 {
		t.Errorf("got %d filteredTodos, want 2", len(m.filteredTodos))
	}
	if m.todoCursor != 0 {
		t.Errorf("todoCursor = %d, want 0", m.todoCursor)
	}
	if m.todoOffset != 0 {
		t.Errorf("todoOffset = %d, want 0", m.todoOffset)
	}
}

func TestTodosLoadedMsg_Empty(t *testing.T) {
	m := newTestModel(t)
	m.mode = modeTodos

	model, _ := m.Update(todosLoadedMsg{todos: nil})
	m = model.(Model)

	if len(m.todos) != 0 {
		t.Errorf("expected 0 todos, got %d", len(m.todos))
	}
	if m.previewContent != "" {
		t.Errorf("expected empty preview, got %q", m.previewContent)
	}
}

func TestTodoToggledMsg_Error(t *testing.T) {
	m := newTestModelWithTodos(t)

	model, cmd := m.Update(todoToggledMsg{err: fmt.Errorf("toggle failed")})
	m = model.(Model)

	if m.statusMsg != "Toggle error: toggle failed" {
		t.Errorf("statusMsg = %q, want error message", m.statusMsg)
	}
	if cmd == nil {
		t.Error("expected clearStatus command")
	}
}

func TestTodoToggledMsg_Success(t *testing.T) {
	m := newTestModelWithTodos(t)

	_, cmd := m.Update(todoToggledMsg{err: nil})

	if cmd == nil {
		t.Error("expected loadTodos command after successful toggle")
	}
}

func TestTodosView_RenderListPane(t *testing.T) {
	m := newTestModelWithTodos(t)

	view := m.renderListPane(40, 20)
	if view == "" {
		t.Error("expected non-empty list pane in todos mode")
	}
	if !strings.Contains(view, "Todos") {
		t.Error("expected 'Todos' title in list pane")
	}
}

func TestTodosView_RenderListPane_Empty(t *testing.T) {
	m := newTestModel(t)
	m.mode = modeTodos
	m.filteredTodos = nil

	view := m.renderListPane(40, 20)
	if !strings.Contains(view, "No todos found") {
		t.Error("expected 'No todos found' message for empty todos list")
	}
}

func TestTodosView_StatusBar(t *testing.T) {
	m := newTestModelWithTodos(t)

	status := m.renderStatusBar()
	if !strings.Contains(status, "todos") {
		t.Error("expected 'todos' in status bar when in todos mode")
	}
	if !strings.Contains(status, "open") {
		t.Error("expected filter name in status bar")
	}
}

func TestTodosView_NoPanic(t *testing.T) {
	m := newTestModelWithTodos(t)

	// Should not panic with various states
	_ = m.View()

	// With empty todos
	m.filteredTodos = nil
	m.todos = nil
	_ = m.View()
}

func TestSyncCompleteMsg_ReloadsTodosWhenInTodosMode(t *testing.T) {
	m := newTestModelWithTodos(t)

	notes := []*note.Note{
		{FilePath: "/notes/a.md", Title: "Alpha", Date: "2024-01-03"},
	}
	_, cmd := m.Update(syncCompleteMsg{notes: notes})

	if cmd == nil {
		t.Error("expected command to reload todos after sync in todos mode")
	}
}

func TestEnsureTodoVisible(t *testing.T) {
	m := newTestModel(t)
	m.height = 10 // small height = listHeight around 4

	// Create more todos than fit on screen
	var todos []note.TodoItem
	for i := 0; i < 20; i++ {
		todos = append(todos, note.TodoItem{Text: fmt.Sprintf("Task %d", i), FilePath: "/notes/a.md"})
	}
	m.filteredTodos = todos

	// Move cursor to bottom
	m.todoCursor = 15
	m.ensureTodoVisible()

	if m.todoOffset == 0 {
		t.Error("expected todoOffset to be adjusted for cursor at 15")
	}
	if m.todoCursor < m.todoOffset {
		t.Error("cursor should be >= offset")
	}
	listH := m.listHeight()
	if m.todoCursor >= m.todoOffset+listH {
		t.Error("cursor should be visible (within offset + listHeight)")
	}
}

func TestTodoFilterDefault(t *testing.T) {
	m := newTestModel(t)
	if m.todoFilter != "open" {
		t.Errorf("default todoFilter = %q, want 'open'", m.todoFilter)
	}
}

// --- Edit Mode Tests ---

func newTestModelWithRealNotes(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)

	// Create real note files on disk so ParseFile works
	content := "---\ntitle: Alpha\ndate: 2024-01-03\ntags:\n- go\n---\n\nAlpha body"
	path := filepath.Join(m.cfg.NotesDir, "2024-01-03-alpha.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	notes := []*note.Note{
		{FilePath: path, Title: "Alpha", Date: "2024-01-03", Body: "Alpha body", WordCount: 2},
	}
	m.notes = notes
	m.filteredNotes = notes
	return m
}

func TestModeTransition_EditViaEnter(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter on list pane should open editor
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeEdit {
		t.Errorf("after Enter: mode = %d, want %d (modeEdit)", m.mode, modeEdit)
	}
	if m.activePane != panePreview {
		t.Errorf("after Enter: activePane = %d, want %d (panePreview)", m.activePane, panePreview)
	}
	if m.editingPath == "" {
		t.Error("editingPath should be set")
	}
}

func TestEditMode_EscClosesAndSyncs(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeEdit {
		t.Fatalf("expected modeEdit, got %d", m.mode)
	}

	// Esc should close editor and return to normal mode
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("after Esc: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
	if m.editingPath != "" {
		t.Error("editingPath should be cleared after closing editor")
	}
	if !m.syncing {
		t.Error("should trigger sync after closing editor")
	}
}

func TestEditMode_KeysDelegatedToEditor(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	// 'q' should NOT quit (delegated to editor, not interpreted as quit)
	model, cmd := m.Update(keyMsg("q"))
	m = model.(Model)

	if m.mode != modeEdit {
		t.Errorf("'q' in edit mode should not change mode, got %d", m.mode)
	}
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("'q' in edit mode should not quit")
		}
	}
}

func TestEditMode_TabDelegatedToEditor(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	// Tab should be handled by editor (indent in body), not switch panes
	model, _ = m.Update(keyMsg("tab"))
	m = model.(Model)

	if m.mode != modeEdit {
		t.Error("should still be in edit mode after Tab")
	}
	// Active pane should not have changed
	if m.activePane != panePreview {
		t.Errorf("activePane = %d, should stay at panePreview during edit", m.activePane)
	}
}

func TestEditMode_EnterOnEmptyList(t *testing.T) {
	m := newTestModel(t)
	m.notes = nil
	m.filteredNotes = nil

	// Enter on empty list should not panic or enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("Enter on empty list: mode = %d, want %d (modeNormal)", m.mode, modeNormal)
	}
}

func TestEditMode_PreviewPaneShowsEditor(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	view := m.View().Content
	if !strings.Contains(view, "Editor") {
		t.Error("view should show 'Editor' pane title in edit mode")
	}
}

func TestEditMode_StatusBarShowsEditing(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	status := m.renderStatusBar()
	if !strings.Contains(status, "Editing") {
		t.Error("status bar should show 'Editing' in edit mode")
	}
	if !strings.Contains(status, "Esc") {
		t.Error("status bar should mention Esc keybinding in edit mode")
	}
}

func TestEditMode_EditorSavedMsg(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	// Simulate a successful save
	model, _ = m.Update(editorSavedMsg{})
	m = model.(Model)

	if m.mode != modeEdit {
		t.Error("should still be in edit mode after save")
	}
}

func TestEditMode_EditorSavedMsg_WithRename(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)
	oldPath := m.editingPath

	// Simulate save with rename
	newPath := filepath.Join(m.cfg.NotesDir, "2024-01-03-renamed.md")
	model, _ = m.Update(editorSavedMsg{newPath: newPath})
	m = model.(Model)

	if m.editingPath != newPath {
		t.Errorf("editingPath = %q, want %q", m.editingPath, newPath)
	}
	if m.editingPath == oldPath {
		t.Error("editingPath should have been updated from old path")
	}
}

func TestEditMode_EditorSavedMsg_Error(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	model, _ = m.Update(editorSavedMsg{err: fmt.Errorf("write failed")})
	m = model.(Model)

	if !strings.Contains(m.statusMsg, "Save error") {
		t.Errorf("statusMsg = %q, should contain 'Save error'", m.statusMsg)
	}
}

func TestEditMode_AutoSaveTickDelegated(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	// Set editor dirty and matching tickID
	m.editor.dirty = true
	m.editor.tickID = 3

	model, cmd := m.Update(editorAutoSaveTickMsg{id: 3})
	m = model.(Model)

	if cmd == nil {
		t.Error("matching auto-save tick should produce save command")
	}
}

func TestNoteCreatedMsg_OpensEditor(t *testing.T) {
	m := newTestModel(t)

	// Create a note file for noteCreatedMsg to parse
	content := "---\ntitle: New Note\ndate: 2024-02-01\nstatus: active\nsource: written\n---\n\n# New Note\n\n"
	path := filepath.Join(m.cfg.NotesDir, "2024-02-01-new-note.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	model, _ := m.Update(noteCreatedMsg{path: path})
	m = model.(Model)

	if m.mode != modeEdit {
		t.Errorf("after noteCreatedMsg: mode = %d, want %d (modeEdit)", m.mode, modeEdit)
	}
	if m.editingPath != path {
		t.Errorf("editingPath = %q, want %q", m.editingPath, path)
	}
}

func TestNewNoteMode_SubmitCreatesNote(t *testing.T) {
	m := newTestModel(t)

	// Enter new note mode
	model, _ := m.Update(keyMsg("n"))
	m = model.(Model)

	// Type a title
	for _, r := range "My Test Note" {
		model, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = model.(Model)
	}

	// Submit
	model, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal after submitting new note title", m.mode)
	}

	// Execute the command - should return noteCreatedMsg
	if cmd == nil {
		t.Fatal("expected a command from new note submission")
	}
	msg := cmd()
	if _, ok := msg.(noteCreatedMsg); !ok {
		if _, ok := msg.(errMsg); ok {
			// errMsg is also acceptable if dir doesn't exist, etc.
		} else {
			t.Errorf("expected noteCreatedMsg or errMsg, got %T", msg)
		}
	}
}

func TestView_HelpOverlayShowsEditorKeys(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.showHelp = true

	view := m.View().Content
	for _, expected := range []string{"Editor", "Next field", "Save & close"} {
		if !strings.Contains(view, expected) {
			t.Errorf("help view should contain %q", expected)
		}
	}
}

func TestEditMode_ResizeUpdatesEditor(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)

	// Resize - should not panic
	model, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = model.(Model)

	if m.mode != modeEdit {
		t.Error("should still be in edit mode after resize")
	}
}

// --- Input Suppression Tests ---
// These test the fix for misparsed terminal query responses (OSC/CSI)
// being dispatched as individual character key events.

func TestInputSuppression_StartupDropsKeys(t *testing.T) {
	m := newTestModelWithNotes(t)
	// Re-enable suppression (newTestModel clears it)
	m.suppressInputUntil = time.Now().Add(500 * time.Millisecond)

	// Simulate escape sequence fragment characters that arrive at startup
	// (e.g. from OSC 11 background color response: ]11;rgb:...)
	for _, ch := range []string{"]", "1", "1", ";", "r", "g", "b"} {
		model, _ := m.Update(keyMsg(ch))
		m = model.(Model)
	}

	// 'g' should NOT have triggered generate mode
	if m.mode != modeNormal {
		t.Errorf("mode = %d, want %d (modeNormal); suppressed keys should not trigger mode changes", m.mode, modeNormal)
	}
}

func TestInputSuppression_AfterWindowKeysWork(t *testing.T) {
	m := newTestModelWithNotes(t)
	// Set suppression in the past so it's already expired
	m.suppressInputUntil = time.Now().Add(-1 * time.Second)

	// Keys should work normally after suppression expires
	model, _ := m.Update(keyMsg("j"))
	m = model.(Model)

	if m.listCursor != 1 {
		t.Errorf("listCursor = %d, want 1; keys should work after suppression expires", m.listCursor)
	}
}

func TestInputSuppression_EditModeExitSetsSuppression(t *testing.T) {
	m := newTestModelWithRealNotes(t)

	// Enter edit mode
	model, _ := m.Update(specialKeyMsg(tea.KeyEnter))
	m = model.(Model)
	if m.mode != modeEdit {
		t.Fatalf("expected modeEdit, got %d", m.mode)
	}

	// Exit edit mode
	model, _ = m.Update(specialKeyMsg(tea.KeyEscape))
	m = model.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after Esc, got %d", m.mode)
	}

	// Suppression should be set after exiting editor
	if m.suppressInputUntil.IsZero() || m.suppressInputUntil.Before(time.Now()) {
		t.Error("suppressInputUntil should be set to a future time after exiting editor")
	}

	// Simulate CSI cursor position report fragment: [38;60R
	for _, ch := range []string{"[", "3", "8", ";", "6", "0"} {
		model, _ = m.Update(keyMsg(ch))
		m = model.(Model)
	}

	// None of these should have changed mode or state
	if m.mode != modeNormal {
		t.Errorf("mode = %d, want %d; suppressed post-editor keys should be dropped", m.mode, modeNormal)
	}
}

func TestInputSuppression_NewModelHasSuppression(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := config.Config{
		NotesDir: t.TempDir(),
		DBPath:   dbPath,
	}
	worker := ai.NewWorker(10)

	// New() should set startup suppression
	m := New(cfg, database, worker)
	if m.suppressInputUntil.IsZero() {
		t.Error("New() should set suppressInputUntil for startup protection")
	}
	if m.suppressInputUntil.Before(time.Now()) {
		t.Error("suppressInputUntil should be in the future immediately after New()")
	}
}

func TestInputSuppression_ZeroTimeDisablesSuppression(t *testing.T) {
	m := newTestModelWithNotes(t)
	// suppressInputUntil is zeroed by newTestModel

	if !m.suppressInputUntil.IsZero() {
		t.Fatal("test setup: suppressInputUntil should be zero")
	}

	// Keys should work normally when suppression is zero
	model, _ := m.Update(keyMsg("g"))
	m = model.(Model)

	if m.mode != modeGenerate {
		t.Errorf("mode = %d, want %d; zero suppression should not block keys", m.mode, modeGenerate)
	}
}
