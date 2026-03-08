package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func writeNote(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncNotes_BasicSync(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	writeNote(t, dir, "note1.md", "---\ntitle: Note One\ndate: 2024-01-01\n---\n\nFirst note body")
	writeNote(t, dir, "note2.md", "---\ntitle: Note Two\ndate: 2024-01-02\n---\n\nSecond note body")

	notes, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}

	// Verify notes are in the database
	allNotes, err := database.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(allNotes) != 2 {
		t.Errorf("database has %d notes, want 2", len(allNotes))
	}
}

func TestSyncNotes_RemovesStale(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	// Insert a note directly into the DB that doesn't exist on disk
	staleNote := &note.Note{
		FilePath: filepath.Join(dir, "deleted.md"),
		Title:    "Deleted Note",
		Date:     "2024-01-01",
	}
	if err := database.UpsertNote(staleNote); err != nil {
		t.Fatal(err)
	}

	// Create one real note on disk
	writeNote(t, dir, "real.md", "---\ntitle: Real Note\n---\n\nStill here")

	notes, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}

	// Verify stale note was removed
	allNotes, err := database.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(allNotes) != 1 {
		t.Errorf("database has %d notes, want 1 (stale should be removed)", len(allNotes))
	}
	if allNotes[0].Title != "Real Note" {
		t.Errorf("remaining note title = %q, want 'Real Note'", allNotes[0].Title)
	}
}

func TestSyncNotes_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	notes, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("got %d notes, want 0", len(notes))
	}
}

func TestSyncNotes_IgnoresNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	writeNote(t, dir, "note.md", "---\ntitle: Real\n---\n\nBody")
	writeNote(t, dir, "readme.txt", "Not a note")
	writeNote(t, dir, "data.json", `{"not": "a note"}`)

	notes, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Errorf("got %d notes, want 1 (only .md files)", len(notes))
	}
}

func TestSyncNotes_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	path := writeNote(t, dir, "note.md", "---\ntitle: Original\n---\n\nOriginal body")

	// First sync
	_, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}

	// Modify the note
	if err := os.WriteFile(path, []byte("---\ntitle: Updated\n---\n\nUpdated body"), 0644); err != nil {
		t.Fatal(err)
	}

	// Second sync
	notes, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}

	// Verify update
	allNotes, err := database.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if allNotes[0].Title != "Updated" {
		t.Errorf("title = %q, want 'Updated'", allNotes[0].Title)
	}
}

func TestSyncNotes_NonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	database := newTestDB(t)

	// SyncNotes should handle nonexistent dir gracefully (ListNotes creates it)
	notes, err := SyncNotes(dir, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("got %d notes, want 0", len(notes))
	}
}

func TestWatcher_CreateAndStop(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	callCount := 0
	onChange := func() { callCount++ }

	w, err := NewWatcher(dir, database, onChange)
	if err != nil {
		t.Fatal(err)
	}

	w.Start()

	if err := w.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestWatcher_NilOnChange(t *testing.T) {
	dir := t.TempDir()
	database := newTestDB(t)

	w, err := NewWatcher(dir, database, nil)
	if err != nil {
		t.Fatal(err)
	}

	w.Start()

	// Create a file - should not panic with nil onChange
	writeNote(t, dir, "test.md", "---\ntitle: Test\n---\n\nBody")

	// Give watcher time to process
	// Note: we can't easily assert the file was indexed without a callback,
	// but the key thing is it shouldn't panic
	if err := w.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestWatcher_NonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	database := newTestDB(t)

	// NewWatcher should create the directory
	w, err := NewWatcher(dir, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Verify directory was created
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory should have been created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}
