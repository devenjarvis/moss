package db

import (
	"path/filepath"
	"testing"

	"github.com/devenjarvis/moss/internal/note"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func makeNote(path, title, date string, tags []string, body string) *note.Note {
	return &note.Note{
		FilePath:  path,
		Title:     title,
		Date:      date,
		Tags:      tags,
		Status:    "active",
		Source:    "written",
		Body:      body,
		WordCount: len(body) / 5, // rough approximation
	}
}

func TestOpenAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

func TestUpsertAndAllNotes(t *testing.T) {
	db := newTestDB(t)

	n1 := makeNote("/notes/note1.md", "First Note", "2024-01-01", []string{"go"}, "Hello world")
	n2 := makeNote("/notes/note2.md", "Second Note", "2024-01-02", []string{"testing"}, "Test content")

	if err := db.UpsertNote(n1); err != nil {
		t.Fatalf("UpsertNote(n1): %v", err)
	}
	if err := db.UpsertNote(n2); err != nil {
		t.Fatalf("UpsertNote(n2): %v", err)
	}

	notes, err := db.AllNotes()
	if err != nil {
		t.Fatalf("AllNotes(): %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}

	// Should be ordered by date DESC
	if notes[0].Date != "2024-01-02" {
		t.Errorf("first note date = %q, want 2024-01-02 (most recent first)", notes[0].Date)
	}
	if notes[1].Date != "2024-01-01" {
		t.Errorf("second note date = %q, want 2024-01-01", notes[1].Date)
	}
}

func TestUpsertNote_Update(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/note1.md", "Original", "2024-01-01", []string{"go"}, "Original body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	// Update the same note
	n.Title = "Updated"
	n.Body = "Updated body"
	n.Tags = []string{"go", "updated"}
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	notes, err := db.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1 (upsert should not duplicate)", len(notes))
	}
	if notes[0].Title != "Updated" {
		t.Errorf("Title = %q, want %q", notes[0].Title, "Updated")
	}
	if notes[0].Body != "Updated body" {
		t.Errorf("Body = %q, want %q", notes[0].Body, "Updated body")
	}
	if len(notes[0].Tags) != 2 {
		t.Errorf("Tags = %v, want 2 tags", notes[0].Tags)
	}
}

func TestDeleteNote(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/note1.md", "To Delete", "2024-01-01", nil, "Body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteNote("/notes/note1.md"); err != nil {
		t.Fatal(err)
	}

	notes, err := db.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("got %d notes after delete, want 0", len(notes))
	}
}

func TestDeleteNote_Nonexistent(t *testing.T) {
	db := newTestDB(t)

	// Deleting a nonexistent note should not error
	if err := db.DeleteNote("/notes/nonexistent.md"); err != nil {
		t.Errorf("DeleteNote on nonexistent should not error: %v", err)
	}
}

func TestSearch(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/go.md", "Go Programming", "2024-01-01", []string{"go", "programming"}, "Go is a systems language created at Google"),
		makeNote("/notes/rust.md", "Rust Language", "2024-01-02", []string{"rust", "programming"}, "Rust provides memory safety without garbage collection"),
		makeNote("/notes/cooking.md", "Pasta Recipe", "2024-01-03", []string{"cooking"}, "Boil water and add pasta for 10 minutes"),
	}

	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("search by title word", func(t *testing.T) {
		results, err := db.Search("programming")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results for 'programming', want 2", len(results))
		}
	})

	t.Run("search by body content", func(t *testing.T) {
		results, err := db.Search("pasta")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results for 'pasta', want 1", len(results))
		}
		if len(results) > 0 && results[0].Title != "Pasta Recipe" {
			t.Errorf("result title = %q, want 'Pasta Recipe'", results[0].Title)
		}
	})

	t.Run("search no results", func(t *testing.T) {
		results, err := db.Search("quantum")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results for 'quantum', want 0", len(results))
		}
	})

	t.Run("search with special characters", func(t *testing.T) {
		// Should not crash with FTS5 special chars
		results, err := db.Search("AND OR NOT")
		if err != nil {
			t.Fatalf("search with special chars should not error: %v", err)
		}
		_ = results // just ensure no panic
	})
}

func TestFilterByTag(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/n1.md", "Note 1", "2024-01-01", []string{"go", "testing"}, "Body 1"),
		makeNote("/notes/n2.md", "Note 2", "2024-01-02", []string{"go", "web"}, "Body 2"),
		makeNote("/notes/n3.md", "Note 3", "2024-01-03", []string{"rust"}, "Body 3"),
	}

	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("filter by shared tag", func(t *testing.T) {
		results, err := db.FilterByTag("go")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results for tag 'go', want 2", len(results))
		}
	})

	t.Run("filter by unique tag", func(t *testing.T) {
		results, err := db.FilterByTag("rust")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results for tag 'rust', want 1", len(results))
		}
	})

	t.Run("filter by nonexistent tag", func(t *testing.T) {
		results, err := db.FilterByTag("python")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results for tag 'python', want 0", len(results))
		}
	})
}

func TestEscapeFTS5(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", `"hello"`},
		{"hello world", `"hello" "world"`},
		{"AND OR NOT", `"AND" "OR" "NOT"`},
		{"", ""},
		{`quote"inside`, `"quote""inside"`},
	}

	for _, tt := range tests {
		got := escapeFTS5(tt.input)
		if got != tt.want {
			t.Errorf("escapeFTS5(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestJoinSplitStrings(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		input := []string{"go", "testing", "moss"}
		joined := joinStrings(input)
		split := splitStrings(joined)
		if len(split) != len(input) {
			t.Fatalf("got %d elements, want %d", len(split), len(input))
		}
		for i := range input {
			if split[i] != input[i] {
				t.Errorf("split[%d] = %q, want %q", i, split[i], input[i])
			}
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := splitStrings("")
		if result != nil {
			t.Errorf("splitStrings('') = %v, want nil", result)
		}
	})

	t.Run("single element", func(t *testing.T) {
		result := splitStrings("single")
		if len(result) != 1 || result[0] != "single" {
			t.Errorf("splitStrings('single') = %v, want [single]", result)
		}
	})
}

func TestAllTags(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/n1.md", "Note 1", "2024-01-01", []string{"go", "testing"}, "Body 1"),
		makeNote("/notes/n2.md", "Note 2", "2024-01-02", []string{"go", "web"}, "Body 2"),
		makeNote("/notes/n3.md", "Note 3", "2024-01-03", []string{"rust"}, "Body 3"),
		makeNote("/notes/n4.md", "Note 4", "2024-01-04", nil, "Body 4"),
	}

	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := db.AllTags()
	if err != nil {
		t.Fatal(err)
	}

	// Should be sorted alphabetically
	expected := []string{"go", "rust", "testing", "web"}
	if len(tags) != len(expected) {
		t.Fatalf("got %d tags, want %d: %v", len(tags), len(expected), tags)
	}
	for i, tag := range expected {
		if tags[i] != tag {
			t.Errorf("tags[%d] = %q, want %q", i, tags[i], tag)
		}
	}
}

func TestAllTags_Empty(t *testing.T) {
	db := newTestDB(t)

	tags, err := db.AllTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags, want 0", len(tags))
	}
}

func TestAllTags_NoDuplicates(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/n1.md", "Note 1", "2024-01-01", []string{"go", "testing"}, "Body 1"),
		makeNote("/notes/n2.md", "Note 2", "2024-01-02", []string{"go", "testing"}, "Body 2"),
	}

	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := db.AllTags()
	if err != nil {
		t.Fatal(err)
	}

	if len(tags) != 2 {
		t.Errorf("got %d tags (should deduplicate), want 2: %v", len(tags), tags)
	}
}

func TestAllNotesSorted_ByDate(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/a.md", "Alpha", "2024-01-01", nil, "Body"),
		makeNote("/notes/b.md", "Beta", "2024-01-03", nil, "Body"),
		makeNote("/notes/c.md", "Charlie", "2024-01-02", nil, "Body"),
	}
	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.AllNotesSorted("date")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d notes, want 3", len(result))
	}
	// Should be date DESC
	if result[0].Title != "Beta" {
		t.Errorf("first note = %q, want Beta (most recent)", result[0].Title)
	}
	if result[1].Title != "Charlie" {
		t.Errorf("second note = %q, want Charlie", result[1].Title)
	}
	if result[2].Title != "Alpha" {
		t.Errorf("third note = %q, want Alpha (oldest)", result[2].Title)
	}
}

func TestAllNotesSorted_ByTitle(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/c.md", "Charlie", "2024-01-01", nil, "Body"),
		makeNote("/notes/a.md", "Alpha", "2024-01-02", nil, "Body"),
		makeNote("/notes/b.md", "Beta", "2024-01-03", nil, "Body"),
	}
	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.AllNotesSorted("title")
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Title != "Alpha" {
		t.Errorf("first note = %q, want Alpha", result[0].Title)
	}
	if result[1].Title != "Beta" {
		t.Errorf("second note = %q, want Beta", result[1].Title)
	}
	if result[2].Title != "Charlie" {
		t.Errorf("third note = %q, want Charlie", result[2].Title)
	}
}

func TestAllNotesSorted_ByWordCount(t *testing.T) {
	db := newTestDB(t)

	n1 := makeNote("/notes/a.md", "Short", "2024-01-01", nil, "hello")
	n1.WordCount = 1
	n2 := makeNote("/notes/b.md", "Long", "2024-01-02", nil, "hello world foo bar baz")
	n2.WordCount = 5
	n3 := makeNote("/notes/c.md", "Medium", "2024-01-03", nil, "hello world foo")
	n3.WordCount = 3

	for _, n := range []*note.Note{n1, n2, n3} {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.AllNotesSorted("words")
	if err != nil {
		t.Fatal(err)
	}
	// Should be word_count DESC
	if result[0].Title != "Long" {
		t.Errorf("first note = %q, want Long (most words)", result[0].Title)
	}
	if result[2].Title != "Short" {
		t.Errorf("last note = %q, want Short (fewest words)", result[2].Title)
	}
}

func TestAllNotesSorted_DefaultFallback(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/a.md", "Note", "2024-01-01", nil, "Body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	// Unknown sort field should fall back to date sort
	result, err := db.AllNotesSorted("unknown")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d notes, want 1", len(result))
	}
}

func TestNoteFieldsPreservedThroughDB(t *testing.T) {
	db := newTestDB(t)

	original := &note.Note{
		FilePath:        "/notes/full.md",
		Title:           "Full Note",
		Date:            "2024-06-15",
		Tags:            []string{"tag1", "tag2"},
		People:          []string{"alice", "bob"},
		Project:         "moss",
		Status:          "active",
		Source:          "written",
		Summary:         "A comprehensive test note",
		GeneratedFrom:   []string{"source1.md", "source2.md"},
		GeneratedPrompt: "generate a test note",
		WordCount:       42,
		HasTodos:        true,
		Body:            "Full body content with - [ ] a todo",
	}

	if err := db.UpsertNote(original); err != nil {
		t.Fatal(err)
	}

	notes, err := db.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}

	got := notes[0]
	if got.Title != original.Title {
		t.Errorf("Title = %q, want %q", got.Title, original.Title)
	}
	if got.Date != original.Date {
		t.Errorf("Date = %q, want %q", got.Date, original.Date)
	}
	if got.Project != original.Project {
		t.Errorf("Project = %q, want %q", got.Project, original.Project)
	}
	if got.Status != original.Status {
		t.Errorf("Status = %q, want %q", got.Status, original.Status)
	}
	if got.Source != original.Source {
		t.Errorf("Source = %q, want %q", got.Source, original.Source)
	}
	if got.Summary != original.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, original.Summary)
	}
	if got.GeneratedPrompt != original.GeneratedPrompt {
		t.Errorf("GeneratedPrompt = %q, want %q", got.GeneratedPrompt, original.GeneratedPrompt)
	}
	if got.WordCount != original.WordCount {
		t.Errorf("WordCount = %d, want %d", got.WordCount, original.WordCount)
	}
	if got.HasTodos != original.HasTodos {
		t.Errorf("HasTodos = %v, want %v", got.HasTodos, original.HasTodos)
	}
	if got.Body != original.Body {
		t.Errorf("Body = %q, want %q", got.Body, original.Body)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "tag1" {
		t.Errorf("Tags = %v, want [tag1, tag2]", got.Tags)
	}
	if len(got.People) != 2 || got.People[0] != "alice" {
		t.Errorf("People = %v, want [alice, bob]", got.People)
	}
	if len(got.GeneratedFrom) != 2 {
		t.Errorf("GeneratedFrom = %v, want 2 entries", got.GeneratedFrom)
	}
}

// --- Todo Tests ---

func TestUpsertTodos(t *testing.T) {
	db := newTestDB(t)

	// Insert a note first (FK constraint)
	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", []string{"work"}, "- [ ] Task 1")
	n.HasTodos = true
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Task 1", Done: false, NoteTitle: "Tasks", NoteDate: "2024-01-01", NoteTags: []string{"work"}, NoteProject: "acme"},
		{FilePath: "/notes/tasks.md", LineNumber: 2, Text: "Task 2", Done: true, NoteTitle: "Tasks", NoteDate: "2024-01-01", NoteTags: []string{"work"}, NoteProject: "acme"},
	}

	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatalf("UpsertTodos: %v", err)
	}

	// Verify todos were inserted
	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatalf("AllTodos: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d todos, want 2", len(all))
	}
}

func TestUpsertTodos_ReplacesExisting(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	n.HasTodos = true
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	// Insert initial todos
	initial := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Old task", Done: false, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", initial); err != nil {
		t.Fatal(err)
	}

	// Replace with new todos
	updated := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "New task 1", Done: false, NoteTitle: "Tasks"},
		{FilePath: "/notes/tasks.md", LineNumber: 2, Text: "New task 2", Done: true, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", updated); err != nil {
		t.Fatal(err)
	}

	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d todos, want 2 after replacement", len(all))
	}
	if all[0].Text != "New task 1" {
		t.Errorf("todo[0].Text = %q, want 'New task 1'", all[0].Text)
	}
}

func TestUpsertTodos_ClearsWithNil(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Task", Done: false, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	// Clear by passing nil
	if err := db.UpsertTodos("/notes/tasks.md", nil); err != nil {
		t.Fatal(err)
	}

	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 todos after clearing, got %d", len(all))
	}
}

func TestAllTodos_FilterOpen(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Open task", Done: false, NoteTitle: "Tasks"},
		{FilePath: "/notes/tasks.md", LineNumber: 2, Text: "Done task", Done: true, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	open, err := db.AllTodos("open")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("got %d open todos, want 1", len(open))
	}
	if open[0].Text != "Open task" {
		t.Errorf("open todo text = %q, want 'Open task'", open[0].Text)
	}
	if open[0].Done {
		t.Error("open todo should not be done")
	}
}

func TestAllTodos_FilterDone(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Open task", Done: false, NoteTitle: "Tasks"},
		{FilePath: "/notes/tasks.md", LineNumber: 2, Text: "Done task", Done: true, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	done, err := db.AllTodos("done")
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 {
		t.Fatalf("got %d done todos, want 1", len(done))
	}
	if done[0].Text != "Done task" {
		t.Errorf("done todo text = %q, want 'Done task'", done[0].Text)
	}
}

func TestAllTodos_FilterAll(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Open", Done: false, NoteTitle: "Tasks"},
		{FilePath: "/notes/tasks.md", LineNumber: 2, Text: "Done", Done: true, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d todos with 'all' filter, want 2", len(all))
	}
}

func TestAllTodos_PreservesMetadata(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", []string{"work"}, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{
			FilePath:    "/notes/tasks.md",
			LineNumber:  1,
			Text:        "Important task",
			Done:        false,
			NoteTitle:   "Tasks",
			NoteDate:    "2024-01-01",
			NoteTags:    []string{"work", "urgent"},
			NotePeople:  []string{"alice"},
			NoteProject: "acme",
		},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d todos, want 1", len(all))
	}

	got := all[0]
	if got.NoteTitle != "Tasks" {
		t.Errorf("NoteTitle = %q, want Tasks", got.NoteTitle)
	}
	if got.NoteDate != "2024-01-01" {
		t.Errorf("NoteDate = %q, want 2024-01-01", got.NoteDate)
	}
	if len(got.NoteTags) != 2 || got.NoteTags[0] != "work" {
		t.Errorf("NoteTags = %v, want [work, urgent]", got.NoteTags)
	}
	if len(got.NotePeople) != 1 || got.NotePeople[0] != "alice" {
		t.Errorf("NotePeople = %v, want [alice]", got.NotePeople)
	}
	if got.NoteProject != "acme" {
		t.Errorf("NoteProject = %q, want acme", got.NoteProject)
	}
}

func TestAllTodos_OrderByDateDescThenFile(t *testing.T) {
	db := newTestDB(t)

	n1 := makeNote("/notes/old.md", "Old", "2024-01-01", nil, "body")
	n2 := makeNote("/notes/new.md", "New", "2024-01-15", nil, "body")
	if err := db.UpsertNote(n1); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertNote(n2); err != nil {
		t.Fatal(err)
	}

	if err := db.UpsertTodos("/notes/old.md", []note.TodoItem{
		{FilePath: "/notes/old.md", LineNumber: 1, Text: "Old task", NoteDate: "2024-01-01", NoteTitle: "Old"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertTodos("/notes/new.md", []note.TodoItem{
		{FilePath: "/notes/new.md", LineNumber: 1, Text: "New task", NoteDate: "2024-01-15", NoteTitle: "New"},
	}); err != nil {
		t.Fatal(err)
	}

	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d todos, want 2", len(all))
	}
	// Most recent date first
	if all[0].NoteDate != "2024-01-15" {
		t.Errorf("expected newest first, got date %q", all[0].NoteDate)
	}
}

func TestTodoProjects(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Task 1", Done: false, NoteTitle: "Tasks", NoteProject: "alpha"},
		{FilePath: "/notes/tasks.md", LineNumber: 2, Text: "Task 2", Done: false, NoteTitle: "Tasks", NoteProject: "beta"},
		{FilePath: "/notes/tasks.md", LineNumber: 3, Text: "Task 3", Done: true, NoteTitle: "Tasks", NoteProject: "gamma"},
		{FilePath: "/notes/tasks.md", LineNumber: 4, Text: "Task 4", Done: false, NoteTitle: "Tasks", NoteProject: ""},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	projects, err := db.TodoProjects()
	if err != nil {
		t.Fatal(err)
	}

	// Should only include projects from open todos, excluding empty, sorted alphabetically
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2: %v", len(projects), projects)
	}
	if projects[0] != "alpha" || projects[1] != "beta" {
		t.Errorf("projects = %v, want [alpha, beta]", projects)
	}
}

func TestTodoProjects_Empty(t *testing.T) {
	db := newTestDB(t)

	projects, err := db.TodoProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %v", projects)
	}
}

func TestTodoCascadeDelete(t *testing.T) {
	db := newTestDB(t)

	n := makeNote("/notes/tasks.md", "Tasks", "2024-01-01", nil, "body")
	if err := db.UpsertNote(n); err != nil {
		t.Fatal(err)
	}

	todos := []note.TodoItem{
		{FilePath: "/notes/tasks.md", LineNumber: 1, Text: "Task", Done: false, NoteTitle: "Tasks"},
	}
	if err := db.UpsertTodos("/notes/tasks.md", todos); err != nil {
		t.Fatal(err)
	}

	// Delete the note — todos should cascade delete
	if err := db.DeleteNote("/notes/tasks.md"); err != nil {
		t.Fatal(err)
	}

	all, err := db.AllTodos("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 todos after cascade delete, got %d", len(all))
	}
}

// --- Search Advanced Tests ---

func TestSearchWithTag(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		makeNote("/notes/go-web.md", "Go Web", "2024-01-01", []string{"go", "web"}, "Building web apps with Go"),
		makeNote("/notes/go-cli.md", "Go CLI", "2024-01-02", []string{"go", "cli"}, "Building CLI tools with Go"),
		makeNote("/notes/rust-web.md", "Rust Web", "2024-01-03", []string{"rust", "web"}, "Building web apps with Rust"),
	}
	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("search scoped to tag", func(t *testing.T) {
		results, err := db.SearchWithTag("web", "go")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1 (only Go+web note)", len(results))
		}
		if len(results) > 0 && results[0].Title != "Go Web" {
			t.Errorf("result = %q, want 'Go Web'", results[0].Title)
		}
	})

	t.Run("search matches all with tag", func(t *testing.T) {
		results, err := db.SearchWithTag("Building", "go")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2 (both Go notes)", len(results))
		}
	})

	t.Run("search no match in tag scope", func(t *testing.T) {
		results, err := db.SearchWithTag("Rust", "go")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results, want 0 (Rust not in go-tagged notes)", len(results))
		}
	})
}

func TestParseSearchQuery(t *testing.T) {
	t.Run("plain terms", func(t *testing.T) {
		pq := ParseSearchQuery("hello world")
		if pq.GeneralTerms == "" {
			t.Error("GeneralTerms should not be empty")
		}
		if len(pq.ColumnFilters) != 0 {
			t.Errorf("ColumnFilters should be empty, got %v", pq.ColumnFilters)
		}
		if len(pq.SQLFilters) != 0 {
			t.Errorf("SQLFilters should be empty, got %v", pq.SQLFilters)
		}
	})

	t.Run("title prefix", func(t *testing.T) {
		pq := ParseSearchQuery("title:meeting")
		if pq.GeneralTerms != "" {
			t.Errorf("GeneralTerms = %q, want empty", pq.GeneralTerms)
		}
		if pq.ColumnFilters["title"] != "meeting" {
			t.Errorf("ColumnFilters[title] = %q, want 'meeting'", pq.ColumnFilters["title"])
		}
	})

	t.Run("tag prefix normalizes to tags", func(t *testing.T) {
		pq := ParseSearchQuery("tag:work")
		if pq.ColumnFilters["tags"] != "work" {
			t.Errorf("ColumnFilters[tags] = %q, want 'work'", pq.ColumnFilters["tags"])
		}
	})

	t.Run("sql filter prefix", func(t *testing.T) {
		pq := ParseSearchQuery("project:moss")
		if pq.SQLFilters["project"] != "moss" {
			t.Errorf("SQLFilters[project] = %q, want 'moss'", pq.SQLFilters["project"])
		}
	})

	t.Run("mixed terms and prefixes", func(t *testing.T) {
		pq := ParseSearchQuery("meeting title:standup project:acme")
		if pq.GeneralTerms == "" {
			t.Error("GeneralTerms should contain 'meeting'")
		}
		if pq.ColumnFilters["title"] != "standup" {
			t.Errorf("ColumnFilters[title] = %q, want 'standup'", pq.ColumnFilters["title"])
		}
		if pq.SQLFilters["project"] != "acme" {
			t.Errorf("SQLFilters[project] = %q, want 'acme'", pq.SQLFilters["project"])
		}
	})

	t.Run("quoted value", func(t *testing.T) {
		pq := ParseSearchQuery(`project:"my project"`)
		if pq.SQLFilters["project"] != "my project" {
			t.Errorf("SQLFilters[project] = %q, want 'my project'", pq.SQLFilters["project"])
		}
	})

	t.Run("unknown prefix treated as plain term", func(t *testing.T) {
		pq := ParseSearchQuery("foo:bar")
		if pq.GeneralTerms == "" {
			t.Error("unknown prefix should be treated as general term")
		}
		if len(pq.ColumnFilters) != 0 {
			t.Errorf("ColumnFilters should be empty for unknown prefix, got %v", pq.ColumnFilters)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		pq := ParseSearchQuery("")
		if pq.GeneralTerms != "" {
			t.Errorf("GeneralTerms = %q, want empty", pq.GeneralTerms)
		}
	})

	t.Run("prefix with empty value", func(t *testing.T) {
		pq := ParseSearchQuery("title:")
		if len(pq.ColumnFilters) != 0 {
			t.Errorf("empty value should not create filter, got %v", pq.ColumnFilters)
		}
	})

	t.Run("people and status prefixes", func(t *testing.T) {
		pq := ParseSearchQuery("people:alice status:active")
		if pq.SQLFilters["people"] != "alice" {
			t.Errorf("SQLFilters[people] = %q, want 'alice'", pq.SQLFilters["people"])
		}
		if pq.SQLFilters["status"] != "active" {
			t.Errorf("SQLFilters[status] = %q, want 'active'", pq.SQLFilters["status"])
		}
	})
}

func TestSearchAdvanced(t *testing.T) {
	db := newTestDB(t)

	notes := []*note.Note{
		{FilePath: "/notes/meeting.md", Title: "Team Meeting", Date: "2024-01-01", Tags: []string{"work"}, People: []string{"alice"}, Project: "acme", Status: "active", Body: "Discussed project timeline"},
		{FilePath: "/notes/standup.md", Title: "Daily Standup", Date: "2024-01-02", Tags: []string{"work", "daily"}, People: []string{"bob"}, Project: "acme", Status: "active", Body: "Quick status update"},
		{FilePath: "/notes/recipe.md", Title: "Pasta Recipe", Date: "2024-01-03", Tags: []string{"cooking"}, Project: "", Status: "", Body: "Boil water and add pasta"},
	}
	for _, n := range notes {
		if err := db.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("general terms only", func(t *testing.T) {
		pq := ParseSearchQuery("project")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1 (only meeting body mentions 'project')", len(results))
		}
	})

	t.Run("title column filter", func(t *testing.T) {
		pq := ParseSearchQuery("title:meeting")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1", len(results))
		}
		if len(results) > 0 && results[0].Title != "Team Meeting" {
			t.Errorf("result = %q, want 'Team Meeting'", results[0].Title)
		}
	})

	t.Run("sql filter project", func(t *testing.T) {
		pq := ParseSearchQuery("project:acme")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2 (both acme project notes)", len(results))
		}
	})

	t.Run("sql filter people", func(t *testing.T) {
		pq := ParseSearchQuery("people:alice")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1", len(results))
		}
	})

	t.Run("combined fts and sql filter", func(t *testing.T) {
		pq := ParseSearchQuery("status project:acme")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		// "status" appears in body of standup note, and both work notes have project=acme
		if len(results) != 1 {
			t.Errorf("got %d results, want 1 (standup has 'status' in body + acme project)", len(results))
		}
	})

	t.Run("with tag scope", func(t *testing.T) {
		pq := ParseSearchQuery("status")
		results, err := db.SearchAdvanced(pq, "daily")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1 (only standup has daily tag)", len(results))
		}
	})

	t.Run("empty query returns all notes", func(t *testing.T) {
		pq := ParseSearchQuery("")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 {
			t.Errorf("got %d results, want 3 (all notes)", len(results))
		}
	})

	t.Run("no matches", func(t *testing.T) {
		pq := ParseSearchQuery("title:nonexistent")
		results, err := db.SearchAdvanced(pq, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results, want 0", len(results))
		}
	})
}
