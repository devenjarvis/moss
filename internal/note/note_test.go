package note

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   string
		wantBody string
	}{
		{
			name:     "valid frontmatter",
			content:  "---\ntitle: Test\ntags: [a, b]\n---\n\nHello world",
			wantFM:   "title: Test\ntags: [a, b]",
			wantBody: "Hello world",
		},
		{
			name:     "no frontmatter",
			content:  "Just a plain note",
			wantFM:   "",
			wantBody: "Just a plain note",
		},
		{
			name:     "empty content",
			content:  "",
			wantFM:   "",
			wantBody: "",
		},
		{
			name:     "only opening delimiter",
			content:  "---\ntitle: Test\nno closing",
			wantFM:   "",
			wantBody: "---\ntitle: Test\nno closing",
		},
		{
			name:     "empty frontmatter",
			content:  "---\n---\n\nBody text",
			wantFM:   "",
			wantBody: "Body text",
		},
		{
			name:     "frontmatter with leading whitespace",
			content:  "  \n---\ntitle: Trimmed\n---\nBody",
			wantFM:   "title: Trimmed",
			wantBody: "Body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body := splitFrontmatter(tt.content)
			if fm != tt.wantFM {
				t.Errorf("frontmatter = %q, want %q", fm, tt.wantFM)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  multiple   spaces   here  ", 3},
		{"line one\nline two\nline three", 6},
	}

	for _, tt := range tests {
		got := countWords(tt.input)
		if got != tt.want {
			t.Errorf("countWords(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDetectTodos(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"no tasks here", false},
		{"- [ ] unchecked task", true},
		{"- [x] checked task", true},
		{"TODO: finish this", true},
		{"this is a todo item", true},
		{"nothing special", false},
		{"- [X] uppercase checked", true}, // ToLower makes this match
	}

	for _, tt := range tests {
		got := detectTodos(tt.input)
		if got != tt.want {
			t.Errorf("detectTodos(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"My Cool Note!", "my-cool-note"},
		{"already-slugified", "already-slugified"},
		{"  spaces  and---dashes  ", "spaces-and-dashes"},
		{"special @#$ chars", "special-chars"},
		{"under_score", "under-score"},
		{"UPPERCASE", "uppercase"},
		{"123 numbers", "123-numbers"},
	}

	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMissingFrontmatterFields(t *testing.T) {
	t.Run("all fields missing", func(t *testing.T) {
		n := &Note{}
		missing := n.MissingFrontmatterFields()
		expected := []string{"title", "date", "summary", "tags", "status", "source"}
		if len(missing) != len(expected) {
			t.Fatalf("got %d missing fields, want %d: %v", len(missing), len(expected), missing)
		}
		for i, f := range expected {
			if missing[i] != f {
				t.Errorf("missing[%d] = %q, want %q", i, missing[i], f)
			}
		}
	})

	t.Run("no fields missing", func(t *testing.T) {
		n := &Note{
			Title:   "Test",
			Date:    "2024-01-01",
			Summary: "A summary",
			Tags:    []string{"tag1"},
			Status:  "active",
			Source:  "written",
		}
		missing := n.MissingFrontmatterFields()
		if len(missing) != 0 {
			t.Errorf("expected no missing fields, got %v", missing)
		}
	})

	t.Run("partial fields missing", func(t *testing.T) {
		n := &Note{
			Title: "Test",
			Date:  "2024-01-01",
		}
		missing := n.MissingFrontmatterFields()
		if len(missing) != 4 {
			t.Fatalf("expected 4 missing fields, got %d: %v", len(missing), missing)
		}
	})
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid note with frontmatter", func(t *testing.T) {
		content := "---\ntitle: Test Note\ndate: 2024-01-15\ntags: [go, testing]\nstatus: active\nsource: written\nsummary: A test note\n---\n\nThis is the body of the note.\n\nIt has multiple paragraphs."
		path := filepath.Join(dir, "test-note.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		n, err := ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if n.Title != "Test Note" {
			t.Errorf("Title = %q, want %q", n.Title, "Test Note")
		}
		if n.Date != "2024-01-15" {
			t.Errorf("Date = %q, want %q", n.Date, "2024-01-15")
		}
		if len(n.Tags) != 2 || n.Tags[0] != "go" || n.Tags[1] != "testing" {
			t.Errorf("Tags = %v, want [go, testing]", n.Tags)
		}
		if n.Status != "active" {
			t.Errorf("Status = %q, want %q", n.Status, "active")
		}
		if n.Source != "written" {
			t.Errorf("Source = %q, want %q", n.Source, "written")
		}
		if n.FilePath != path {
			t.Errorf("FilePath = %q, want %q", n.FilePath, path)
		}
		if n.WordCount == 0 {
			t.Error("WordCount should be > 0")
		}
		if !strings.Contains(n.Body, "body of the note") {
			t.Errorf("Body should contain 'body of the note', got %q", n.Body)
		}
	})

	t.Run("note without frontmatter", func(t *testing.T) {
		content := "Just plain markdown content"
		path := filepath.Join(dir, "plain.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		n, err := ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}

		// Title derived from filename
		if n.Title != "plain" {
			t.Errorf("Title = %q, want %q", n.Title, "plain")
		}
		if n.Body != content {
			t.Errorf("Body = %q, want %q", n.Body, content)
		}
	})

	t.Run("note with todos", func(t *testing.T) {
		content := "---\ntitle: Tasks\n---\n\n- [ ] Buy groceries\n- [x] Write tests"
		path := filepath.Join(dir, "tasks.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		n, err := ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !n.HasTodos {
			t.Error("HasTodos should be true")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ParseFile(filepath.Join(dir, "nonexistent.md"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestCreateNew(t *testing.T) {
	dir := t.TempDir()

	path, err := CreateNew(dir, "My Test Note")
	if err != nil {
		t.Fatal(err)
	}

	// Check file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("file not created at %s", path)
	}

	// Check filename contains slug
	base := filepath.Base(path)
	if !strings.Contains(base, "my-test-note") {
		t.Errorf("filename %q should contain slug 'my-test-note'", base)
	}
	if !strings.HasSuffix(base, ".md") {
		t.Errorf("filename %q should end with .md", base)
	}

	// Parse and verify contents
	n, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "My Test Note" {
		t.Errorf("Title = %q, want %q", n.Title, "My Test Note")
	}
	if n.Status != "active" {
		t.Errorf("Status = %q, want %q", n.Status, "active")
	}
	if n.Source != "written" {
		t.Errorf("Source = %q, want %q", n.Source, "written")
	}
}

func TestCreateNew_SubdirectoryCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	path, err := CreateNew(dir, "Nested Note")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("file not created at %s", path)
	}
}

func TestWriteFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "write-test.md")

	// Create initial note
	n := &Note{
		Title:    "Original",
		Date:     "2024-01-01",
		Status:   "active",
		Source:   "written",
		FilePath: path,
		Body:     "Original body content",
	}

	if err := n.WriteFrontmatter(); err != nil {
		t.Fatal(err)
	}

	// Update frontmatter and write again
	n.Title = "Updated"
	n.Tags = []string{"updated", "test"}
	if err := n.WriteFrontmatter(); err != nil {
		t.Fatal(err)
	}

	// Re-parse and verify
	parsed, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Title != "Updated" {
		t.Errorf("Title = %q, want %q", parsed.Title, "Updated")
	}
	if len(parsed.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 tags", parsed.Tags)
	}
	if !strings.Contains(parsed.Body, "Original body content") {
		t.Errorf("Body should contain 'Original body content', got %q", parsed.Body)
	}
}

func TestRenameToTitle(t *testing.T) {
	dir := t.TempDir()

	t.Run("renames file to match title", func(t *testing.T) {
		// Create a note with "untitled" filename
		path := filepath.Join(dir, "2024-01-15-untitled.md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		n := &Note{
			FilePath: path,
			Title:    "My Great Note",
			Date:     "2024-01-15",
		}

		newPath, err := n.RenameToTitle()
		if err != nil {
			t.Fatal(err)
		}

		expectedPath := filepath.Join(dir, "2024-01-15-my-great-note.md")
		if newPath != expectedPath {
			t.Errorf("newPath = %q, want %q", newPath, expectedPath)
		}
		if n.FilePath != expectedPath {
			t.Errorf("n.FilePath = %q, want %q", n.FilePath, expectedPath)
		}
		// Old file should not exist
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("old file should not exist after rename")
		}
		// New file should exist
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			t.Error("new file should exist after rename")
		}
	})

	t.Run("no-op when path already matches", func(t *testing.T) {
		path := filepath.Join(dir, "2024-01-15-correct-title.md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		n := &Note{
			FilePath: path,
			Title:    "Correct Title",
			Date:     "2024-01-15",
		}

		newPath, err := n.RenameToTitle()
		if err != nil {
			t.Fatal(err)
		}
		if newPath != path {
			t.Errorf("should not rename when path matches, got %q", newPath)
		}
	})

	t.Run("no-op when title is empty", func(t *testing.T) {
		path := filepath.Join(dir, "2024-01-15-empty.md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		n := &Note{
			FilePath: path,
			Title:    "",
			Date:     "2024-01-15",
		}

		newPath, err := n.RenameToTitle()
		if err != nil {
			t.Fatal(err)
		}
		if newPath != path {
			t.Errorf("should not rename when title is empty, got %q", newPath)
		}
	})

	t.Run("no-op when target exists", func(t *testing.T) {
		// Create both the source and target files
		sourcePath := filepath.Join(dir, "2024-02-01-old-name.md")
		targetPath := filepath.Join(dir, "2024-02-01-existing-note.md")
		if err := os.WriteFile(sourcePath, []byte("source"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}

		n := &Note{
			FilePath: sourcePath,
			Title:    "Existing Note",
			Date:     "2024-02-01",
		}

		newPath, err := n.RenameToTitle()
		if err != nil {
			t.Fatal(err)
		}
		// Should return original path since target exists
		if newPath != sourcePath {
			t.Errorf("should not rename when target exists, got %q", newPath)
		}
	})

	t.Run("uses current date when Date is empty", func(t *testing.T) {
		path := filepath.Join(dir, "some-old-path.md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		n := &Note{
			FilePath: path,
			Title:    "No Date Note",
			Date:     "",
		}

		newPath, err := n.RenameToTitle()
		if err != nil {
			t.Fatal(err)
		}

		// Should contain today's date
		today := strings.Split(newPath, string(filepath.Separator))
		base := today[len(today)-1]
		if !strings.Contains(base, "no-date-note") {
			t.Errorf("renamed file %q should contain slug 'no-date-note'", base)
		}
		if !strings.HasSuffix(base, ".md") {
			t.Errorf("renamed file %q should end with .md", base)
		}
	})
}

func TestListNotes(t *testing.T) {
	dir := t.TempDir()

	// Create some files
	for _, name := range []string{"note1.md", "note2.md", "not-a-note.txt", "note3.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a subdirectory (should be ignored)
	if err := os.Mkdir(filepath.Join(dir, "subdir.md"), 0755); err != nil {
		t.Fatal(err)
	}

	paths, err := ListNotes(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(paths) != 3 {
		t.Errorf("got %d paths, want 3: %v", len(paths), paths)
	}

	for _, p := range paths {
		if !strings.HasSuffix(p, ".md") {
			t.Errorf("path %q should end with .md", p)
		}
	}
}

func TestListNotes_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	paths, err := ListNotes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("expected empty list, got %v", paths)
	}
}

func TestListNotes_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")

	paths, err := ListNotes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("expected empty list, got %v", paths)
	}

	// Directory should have been created
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected directory to be created")
	}
}
