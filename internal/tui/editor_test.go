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
	if e.focus != editorFocusBody {
		t.Errorf("initial focus = %d, want %d (body)", e.focus, editorFocusBody)
	}
	if e.dirty {
		t.Error("editor should not be dirty initially")
	}
}

func TestEditor_FocusCycling_Tab(t *testing.T) {
	e, _ := newTestEditor(t)

	// Start in body (default); Tab indents, focus stays on body
	if e.focus != editorFocusBody {
		t.Fatalf("initial focus = %d, want %d (body)", e.focus, editorFocusBody)
	}
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusBody {
		t.Errorf("tab in body: focus = %d, want %d (body, tab indents not cycles)", e.focus, editorFocusBody)
	}

	// Tab in frontmatter cycles title -> tags -> date -> title (no body)
	e.setFocus(editorFocusTitle)
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusTags {
		t.Errorf("after tab from title: focus = %d, want %d (tags)", e.focus, editorFocusTags)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusDate {
		t.Errorf("after tab from tags: focus = %d, want %d (date)", e.focus, editorFocusDate)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if e.focus != editorFocusTitle {
		t.Errorf("after tab from date: focus = %d, want %d (title, wraps within frontmatter)", e.focus, editorFocusTitle)
	}
}

func TestEditor_FocusCycling_ShiftTab(t *testing.T) {
	e, _ := newTestEditor(t)

	// Shift+Tab in body outdents, focus stays on body
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if e.focus != editorFocusBody {
		t.Errorf("shift+tab in body: focus = %d, want %d (body, shift+tab outdents)", e.focus, editorFocusBody)
	}

	// Shift+Tab in frontmatter cycles backwards: title -> date -> tags -> title
	e.setFocus(editorFocusTitle)
	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if e.focus != editorFocusDate {
		t.Errorf("after shift+tab from title: focus = %d, want %d (date, wraps within frontmatter)", e.focus, editorFocusDate)
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if e.focus != editorFocusTags {
		t.Errorf("after shift+tab from date: focus = %d, want %d (tags)", e.focus, editorFocusTags)
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

// Helper to create an editor focused on body with given content and cursor at end.
func newBodyEditor(t *testing.T, body string) Editor {
	t.Helper()
	e, _ := newTestEditor(t)
	e.setFocus(editorFocusBody)
	e.body.SetValue(body)
	// SetValue resets cursor to end; move to beginning for predictable positioning
	e.body.MoveToBegin()
	return e
}

func TestEditor_SuperB_InsertsBoldMarkers(t *testing.T) {
	e := newBodyEditor(t, "hello")
	// Cursor is at beginning; press super+b
	e, _, close := e.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModSuper})
	if close {
		t.Fatal("super+b should not close editor")
	}
	got := e.body.Value()
	if got != "****hello" {
		t.Errorf("body = %q, want %q", got, "****hello")
	}
	if e.body.Column() != 2 {
		t.Errorf("cursor col = %d, want 2 (between bold markers)", e.body.Column())
	}
	if !e.dirty {
		t.Error("should be dirty after super+b")
	}
}

func TestEditor_SuperB_ToggleOff(t *testing.T) {
	e := newBodyEditor(t, "hello")
	// Insert bold markers first
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModSuper})
	// Cursor should be between **|** — press super+b again to toggle off
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "hello" {
		t.Errorf("after toggle off: body = %q, want %q", got, "hello")
	}
}

func TestEditor_SuperI_InsertsItalicMarkers(t *testing.T) {
	e := newBodyEditor(t, "hello")
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "**hello" {
		t.Errorf("body = %q, want %q", got, "**hello")
	}
	if e.body.Column() != 1 {
		t.Errorf("cursor col = %d, want 1 (between italic markers)", e.body.Column())
	}
}

func TestEditor_SuperI_ToggleOff(t *testing.T) {
	e := newBodyEditor(t, "hello")
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModSuper})
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "hello" {
		t.Errorf("after toggle off: body = %q, want %q", got, "hello")
	}
}

func TestEditor_SuperI_NotConfusedByBold(t *testing.T) {
	e := newBodyEditor(t, "hello")
	// Insert bold markers: **|**hello
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModSuper})
	// Now press super+i — should NOT remove bold, should insert italic
	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModSuper})
	got := e.body.Value()
	// Should have italic markers inserted between the bold markers: ****|****hello
	if got != "******hello" {
		t.Errorf("body = %q, want %q", got, "******hello")
	}
}

func TestEditor_Super1_TogglesH1(t *testing.T) {
	e := newBodyEditor(t, "hello")
	e, _, _ = e.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "# hello" {
		t.Errorf("body = %q, want %q", got, "# hello")
	}
}

func TestEditor_Super1_ToggleOff(t *testing.T) {
	e := newBodyEditor(t, "# hello")
	e, _, _ = e.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "hello" {
		t.Errorf("after toggle off: body = %q, want %q", got, "hello")
	}
}

func TestEditor_Super2_TogglesH2(t *testing.T) {
	e := newBodyEditor(t, "hello")
	e, _, _ = e.Update(tea.KeyPressMsg{Code: '2', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "## hello" {
		t.Errorf("body = %q, want %q", got, "## hello")
	}
}

func TestEditor_Super3_TogglesH3(t *testing.T) {
	e := newBodyEditor(t, "hello")
	e, _, _ = e.Update(tea.KeyPressMsg{Code: '3', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "### hello" {
		t.Errorf("body = %q, want %q", got, "### hello")
	}
}

func TestEditor_Heading_SwitchLevel(t *testing.T) {
	e := newBodyEditor(t, "## hello")
	// Switch from H2 to H1
	e, _, _ = e.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModSuper})
	got := e.body.Value()
	if got != "# hello" {
		t.Errorf("body = %q, want %q", got, "# hello")
	}
}

func TestEditor_MarkdownShortcuts_NoOpWhenNotBody(t *testing.T) {
	e, _ := newTestEditor(t)
	// Move to title explicitly (default is body)
	e.setFocus(editorFocusTitle)
	originalBody := e.body.Value()

	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModSuper})
	if e.body.Value() != originalBody {
		t.Error("super+b should be a no-op when not focused on body")
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModSuper})
	if e.body.Value() != originalBody {
		t.Error("super+i should be a no-op when not focused on body")
	}

	e, _, _ = e.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModSuper})
	if e.body.Value() != originalBody {
		t.Error("super+1 should be a no-op when not focused on body")
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
