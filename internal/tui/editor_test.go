package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
)

func newTestEditor(t *testing.T) (Editor, *note.Note) {
	t.Helper()
	dir := t.TempDir()

	// Create a note file on disk
	content := "---\ntitle: Test Note\ndate: 2024-01-15\ntags:\n- go\n- test\n---\n\nHello world body"
	path := filepath.Join(dir, "2024-01-15-test-note.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	n, err := note.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	e := NewEditor(n, database, 80, 30)
	return e, n
}

func TestNewEditor_PopulatesFields(t *testing.T) {
	e, _ := newTestEditor(t)

	if e.titleInput.Value() != "Test Note" {
		t.Errorf("title = %q, want %q", e.titleInput.Value(), "Test Note")
	}
	if e.dateInput.Value() != "2024-01-15" {
		t.Errorf("date = %q, want %q", e.dateInput.Value(), "2024-01-15")
	}
	if e.tagsInput.Value() != "go, test" {
		t.Errorf("tags = %q, want %q", e.tagsInput.Value(), "go, test")
	}
	if e.body.Value() != "Hello world body" {
		t.Errorf("body = %q, want %q", e.body.Value(), "Hello world body")
	}
	if e.focus != editorFocusTitle {
		t.Errorf("initial focus = %d, want %d (title)", e.focus, editorFocusTitle)
	}
	if e.dirty {
		t.Error("editor should not be dirty initially")
	}
}

func TestEditor_FocusCycling_Tab(t *testing.T) {
	e, _ := newTestEditor(t)

	if e.focus != editorFocusTitle {
		t.Fatalf("initial focus = %d, want %d", e.focus, editorFocusTitle)
	}

	// Tab cycles: title -> tags -> date -> body -> title
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusTags {
		t.Errorf("after tab: focus = %d, want %d (tags)", e.focus, editorFocusTags)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusDate {
		t.Errorf("after 2nd tab: focus = %d, want %d (date)", e.focus, editorFocusDate)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusBody {
		t.Errorf("after 3rd tab: focus = %d, want %d (body)", e.focus, editorFocusBody)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusTitle {
		t.Errorf("after 4th tab: focus = %d, want %d (title, wraps)", e.focus, editorFocusTitle)
	}
}

func TestEditor_FocusCycling_ShiftTab(t *testing.T) {
	e, _ := newTestEditor(t)

	// Shift+Tab goes backwards: title -> body -> date -> tags -> title
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if e.focus != editorFocusBody {
		t.Errorf("after shift+tab: focus = %d, want %d (body)", e.focus, editorFocusBody)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if e.focus != editorFocusDate {
		t.Errorf("after 2nd shift+tab: focus = %d, want %d (date)", e.focus, editorFocusDate)
	}
}

func TestEditor_EnterInFrontmatterMovesToBody(t *testing.T) {
	e, _ := newTestEditor(t)

	// Focus is on title, Enter should move to body
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if e.focus != editorFocusBody {
		t.Errorf("after enter in title: focus = %d, want %d (body)", e.focus, editorFocusBody)
	}
}

func TestEditor_EnterInTagsMovesToBody(t *testing.T) {
	e, _ := newTestEditor(t)
	e.cycleFocus(1) // move to tags

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if e.focus != editorFocusBody {
		t.Errorf("after enter in tags: focus = %d, want %d (body)", e.focus, editorFocusBody)
	}
}

func TestEditor_EscClosesEditor(t *testing.T) {
	e, _ := newTestEditor(t)

	_, _, shouldClose := e.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !shouldClose {
		t.Error("Esc should signal editor close")
	}
}

func TestEditor_CtrlS_DoesNotClose(t *testing.T) {
	e, _ := newTestEditor(t)

	e, _, shouldClose := e.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if shouldClose {
		t.Error("Ctrl+S should not close editor")
	}
	if !e.saving {
		t.Error("Ctrl+S should set saving=true")
	}
}

func TestEditor_TypingMarksDirty(t *testing.T) {
	e, _ := newTestEditor(t)

	if e.dirty {
		t.Fatal("should not be dirty initially")
	}

	// Type a character
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !e.dirty {
		t.Error("should be dirty after typing")
	}
}

func TestEditor_AutoSaveTickMsg(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = true
	e.tickID = 5

	// Matching tick should trigger save
	e, cmd, _ := e.Update(editorAutoSaveTickMsg{id: 5})
	if cmd == nil {
		t.Error("matching tick with dirty editor should produce save command")
	}
	if !e.saving {
		t.Error("should be saving after matching tick")
	}
}

func TestEditor_AutoSaveTickMsg_StaleIgnored(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = true
	e.tickID = 5

	// Non-matching tick should be ignored
	e, cmd, _ := e.Update(editorAutoSaveTickMsg{id: 3})
	if cmd != nil {
		t.Error("stale tick should not produce a command")
	}
}

func TestEditor_AutoSaveTickMsg_CleanIgnored(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = false
	e.tickID = 5

	// Matching tick but not dirty should be ignored
	_, cmd, _ := e.Update(editorAutoSaveTickMsg{id: 5})
	if cmd != nil {
		t.Error("clean editor should not produce save command even on matching tick")
	}
}

func TestEditor_SavedMsg_ClearsDirty(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = true
	e.saving = true

	e, _, _ = e.Update(editorSavedMsg{})
	if e.dirty {
		t.Error("should not be dirty after successful save")
	}
	if e.saving {
		t.Error("should not be saving after save completes")
	}
	if !e.saved {
		t.Error("should be marked saved after successful save")
	}
}

func TestEditor_SavedMsg_WithRename(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = true
	e.saving = true

	e, _, _ = e.Update(editorSavedMsg{newPath: "/new/path.md"})
	if e.note.FilePath != "/new/path.md" {
		t.Errorf("FilePath = %q, want %q", e.note.FilePath, "/new/path.md")
	}
}

func TestEditor_SavedMsg_WithError(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = true
	e.saving = true

	e, _, _ = e.Update(editorSavedMsg{err: os.ErrPermission})
	if !e.dirty {
		t.Error("should remain dirty on save error")
	}
	if e.saving {
		t.Error("saving should be cleared even on error")
	}
}

func TestEditor_View_ContainsFields(t *testing.T) {
	e, _ := newTestEditor(t)

	view := e.View(80, 30)
	if !strings.Contains(view, "title:") {
		t.Error("view should contain title label")
	}
	if !strings.Contains(view, "tags:") {
		t.Error("view should contain tags label")
	}
	if !strings.Contains(view, "date:") {
		t.Error("view should contain date label")
	}
}

func TestEditor_View_DirtyIndicator(t *testing.T) {
	e, _ := newTestEditor(t)
	e.dirty = true

	view := e.View(80, 30)
	if !strings.Contains(view, "modified") {
		t.Error("view should show 'modified' when dirty")
	}
}

func TestEditor_View_SavedIndicator(t *testing.T) {
	e, _ := newTestEditor(t)
	e.saved = true

	view := e.View(80, 30)
	if !strings.Contains(view, "saved") {
		t.Error("view should show 'saved' indicator")
	}
}

func TestEditor_FilePath(t *testing.T) {
	e, n := newTestEditor(t)
	if e.FilePath() != n.FilePath {
		t.Errorf("FilePath() = %q, want %q", e.FilePath(), n.FilePath)
	}
}

func TestEditor_SetSize(t *testing.T) {
	e, _ := newTestEditor(t)

	// Should not panic on various sizes
	e.SetSize(40, 10)
	e.SetSize(200, 60)
	e.SetSize(10, 5) // very small
}

func TestEditor_SaveNow_WritesToDisk(t *testing.T) {
	e, n := newTestEditor(t)

	// Modify the title
	e.titleInput.SetValue("Updated Title")
	e.body.SetValue("Updated body content")

	// Execute the save command
	cmd := e.saveNow()
	msg := cmd()

	savedMsg, ok := msg.(editorSavedMsg)
	if !ok {
		t.Fatalf("expected editorSavedMsg, got %T", msg)
	}
	if savedMsg.err != nil {
		t.Fatalf("save error: %v", savedMsg.err)
	}

	// Verify file was written
	parsed, err := note.ParseFile(n.FilePath)
	if err != nil {
		// Title was changed, file may have been renamed
		if savedMsg.newPath != "" {
			parsed, err = note.ParseFile(savedMsg.newPath)
			if err != nil {
				t.Fatalf("failed to parse saved file at %q: %v", savedMsg.newPath, err)
			}
		} else {
			t.Fatalf("failed to parse saved file: %v", err)
		}
	}

	if parsed.Title != "Updated Title" {
		t.Errorf("saved title = %q, want %q", parsed.Title, "Updated Title")
	}
	if parsed.Body != "Updated body content" {
		t.Errorf("saved body = %q, want %q", parsed.Body, "Updated body content")
	}
}
