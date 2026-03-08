package note

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTodos(t *testing.T) {
	t.Run("basic checkboxes", func(t *testing.T) {
		n := &Note{
			FilePath: "/notes/test.md",
			Title:    "Test",
			Date:     "2024-01-01",
			Tags:     []string{"go"},
			Project:  "moss",
			Body:     "- [ ] Buy groceries\n- [x] Write tests\n- [ ] Review PR",
		}

		todos := n.ParseTodos()
		if len(todos) != 3 {
			t.Fatalf("got %d todos, want 3", len(todos))
		}

		if todos[0].Text != "Buy groceries" || todos[0].Done {
			t.Errorf("todo[0] = %+v, want open 'Buy groceries'", todos[0])
		}
		if todos[1].Text != "Write tests" || !todos[1].Done {
			t.Errorf("todo[1] = %+v, want done 'Write tests'", todos[1])
		}
		if todos[2].Text != "Review PR" || todos[2].Done {
			t.Errorf("todo[2] = %+v, want open 'Review PR'", todos[2])
		}
	})

	t.Run("line numbers are correct", func(t *testing.T) {
		n := &Note{
			Body: "Some intro text\n\n- [ ] First todo\nMore text\n- [x] Second todo",
		}

		todos := n.ParseTodos()
		if len(todos) != 2 {
			t.Fatalf("got %d todos, want 2", len(todos))
		}
		if todos[0].LineNumber != 3 {
			t.Errorf("todo[0].LineNumber = %d, want 3", todos[0].LineNumber)
		}
		if todos[1].LineNumber != 5 {
			t.Errorf("todo[1].LineNumber = %d, want 5", todos[1].LineNumber)
		}
	})

	t.Run("inherits note metadata", func(t *testing.T) {
		n := &Note{
			FilePath: "/notes/project.md",
			Title:    "Project Tasks",
			Date:     "2024-06-15",
			Tags:     []string{"work", "urgent"},
			People:   []string{"alice"},
			Project:  "acme",
			Body:     "- [ ] Do something",
		}

		todos := n.ParseTodos()
		if len(todos) != 1 {
			t.Fatalf("got %d todos, want 1", len(todos))
		}

		todo := todos[0]
		if todo.FilePath != "/notes/project.md" {
			t.Errorf("FilePath = %q, want /notes/project.md", todo.FilePath)
		}
		if todo.NoteTitle != "Project Tasks" {
			t.Errorf("NoteTitle = %q, want Project Tasks", todo.NoteTitle)
		}
		if todo.NoteDate != "2024-06-15" {
			t.Errorf("NoteDate = %q, want 2024-06-15", todo.NoteDate)
		}
		if len(todo.NoteTags) != 2 || todo.NoteTags[0] != "work" {
			t.Errorf("NoteTags = %v, want [work, urgent]", todo.NoteTags)
		}
		if todo.NoteProject != "acme" {
			t.Errorf("NoteProject = %q, want acme", todo.NoteProject)
		}
	})

	t.Run("indented checkboxes", func(t *testing.T) {
		n := &Note{
			Body: "  - [ ] Indented task\n\t- [x] Tab indented",
		}

		todos := n.ParseTodos()
		if len(todos) != 2 {
			t.Fatalf("got %d todos, want 2", len(todos))
		}
		if todos[0].Text != "Indented task" {
			t.Errorf("todo[0].Text = %q, want 'Indented task'", todos[0].Text)
		}
		if todos[1].Text != "Tab indented" || !todos[1].Done {
			t.Errorf("todo[1] = %+v, want done 'Tab indented'", todos[1])
		}
	})

	t.Run("uppercase X", func(t *testing.T) {
		n := &Note{
			Body: "- [X] Uppercase checked",
		}

		todos := n.ParseTodos()
		if len(todos) != 1 {
			t.Fatalf("got %d todos, want 1", len(todos))
		}
		if !todos[0].Done {
			t.Error("expected uppercase [X] to be marked done")
		}
	})

	t.Run("empty checkbox", func(t *testing.T) {
		n := &Note{
			Body: "- [ ]\n- [x]",
		}

		todos := n.ParseTodos()
		if len(todos) != 2 {
			t.Fatalf("got %d todos, want 2", len(todos))
		}
		if todos[0].Text != "" {
			t.Errorf("todo[0].Text = %q, want empty", todos[0].Text)
		}
		if todos[1].Text != "" || !todos[1].Done {
			t.Errorf("todo[1] = %+v, want done empty", todos[1])
		}
	})

	t.Run("empty body", func(t *testing.T) {
		n := &Note{Body: ""}
		todos := n.ParseTodos()
		if len(todos) != 0 {
			t.Errorf("expected 0 todos from empty body, got %d", len(todos))
		}
	})

	t.Run("no checkboxes", func(t *testing.T) {
		n := &Note{
			Body: "Regular text\n- Normal bullet\n- Another bullet",
		}
		todos := n.ParseTodos()
		if len(todos) != 0 {
			t.Errorf("expected 0 todos, got %d", len(todos))
		}
	})
}

func TestCollectTodos(t *testing.T) {
	notes := []*Note{
		{
			FilePath: "/notes/a.md",
			Title:    "Note A",
			Body:     "- [ ] Task A1\n- [x] Task A2",
		},
		{
			FilePath: "/notes/b.md",
			Title:    "Note B",
			Body:     "- [ ] Task B1",
		},
		{
			FilePath: "/notes/c.md",
			Title:    "Note C",
			Body:     "No todos here",
		},
	}

	todos := CollectTodos(notes)
	if len(todos) != 3 {
		t.Fatalf("got %d todos, want 3", len(todos))
	}

	// Verify todos come from correct notes
	if todos[0].Text != "Task A1" || todos[0].FilePath != "/notes/a.md" {
		t.Errorf("todo[0] = %+v, want Task A1 from /notes/a.md", todos[0])
	}
	if todos[2].Text != "Task B1" || todos[2].FilePath != "/notes/b.md" {
		t.Errorf("todo[2] = %+v, want Task B1 from /notes/b.md", todos[2])
	}
}

func TestCollectTodos_NilSlice(t *testing.T) {
	todos := CollectTodos(nil)
	if len(todos) != 0 {
		t.Errorf("expected 0 todos from nil notes, got %d", len(todos))
	}
}

func TestToggleTodo(t *testing.T) {
	dir := t.TempDir()

	t.Run("toggle open to done", func(t *testing.T) {
		path := filepath.Join(dir, "toggle-open.md")
		content := "---\ntitle: Test\n---\n\nIntro text\n- [ ] Buy milk\n- [ ] Buy bread"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Line 2 in body = "- [ ] Buy milk"
		if err := ToggleTodo(path, 2); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result := string(data)

		if !strings.Contains(result, "- [x] Buy milk") {
			t.Errorf("expected '- [x] Buy milk' in result, got:\n%s", result)
		}
		// Other todo should be unchanged
		if !strings.Contains(result, "- [ ] Buy bread") {
			t.Errorf("expected '- [ ] Buy bread' unchanged in result, got:\n%s", result)
		}
	})

	t.Run("toggle done to open", func(t *testing.T) {
		path := filepath.Join(dir, "toggle-done.md")
		content := "---\ntitle: Test\n---\n\n- [x] Already done"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := ToggleTodo(path, 1); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "- [ ] Already done") {
			t.Errorf("expected '- [ ] Already done', got:\n%s", string(data))
		}
	})

	t.Run("toggle uppercase X to open", func(t *testing.T) {
		path := filepath.Join(dir, "toggle-upper.md")
		content := "---\ntitle: Test\n---\n\n- [X] Uppercase"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := ToggleTodo(path, 1); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "- [ ] Uppercase") {
			t.Errorf("expected '- [ ] Uppercase', got:\n%s", string(data))
		}
	})

	t.Run("preserves indentation", func(t *testing.T) {
		path := filepath.Join(dir, "toggle-indent.md")
		content := "---\ntitle: Test\n---\n\n  - [ ] Indented task"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := ToggleTodo(path, 1); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "  - [x] Indented task") {
			t.Errorf("expected preserved indentation '  - [x] Indented task', got:\n%s", string(data))
		}
	})

	t.Run("no-op for invalid line number", func(t *testing.T) {
		path := filepath.Join(dir, "toggle-invalid.md")
		content := "---\ntitle: Test\n---\n\n- [ ] Only one line"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Line 999 doesn't exist — should be a no-op
		if err := ToggleTodo(path, 999); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "- [ ] Only one line") {
			t.Errorf("expected no change, got:\n%s", string(data))
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		err := ToggleTodo(filepath.Join(dir, "nonexistent.md"), 1)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("note without frontmatter", func(t *testing.T) {
		path := filepath.Join(dir, "no-fm.md")
		content := "- [ ] No frontmatter todo"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := ToggleTodo(path, 1); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "- [x] No frontmatter todo") {
			t.Errorf("expected toggled todo, got:\n%s", string(data))
		}
	})
}
