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
