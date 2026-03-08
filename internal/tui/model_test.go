package tui

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	t.Cleanup(func() { database.Close() })

	cfg := config.Config{
		NotesDir: t.TempDir(),
		DBPath:   dbPath,
		Editor:   "vi",
	}
	worker := ai.NewWorker(10)

	m := New(cfg, database, worker)
	// Set a reasonable window size
	m.width = 120
	m.height = 40
	m.updateLayout()
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

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func specialKeyMsg(keyType tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: keyType}
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

	view := m.View()
	if view != "Loading..." {
		t.Errorf("View() = %q, want 'Loading...'", view)
	}
}

func TestView_NoPanic(t *testing.T) {
	m := newTestModelWithNotes(t)

	// Should not panic
	view := m.View()
	if view == "" {
		t.Error("View() should not return empty string")
	}
}

func TestView_HelpOverlay(t *testing.T) {
	m := newTestModelWithNotes(t)
	m.showHelp = true

	view := m.View()
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
	view := m.View()
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
