package note

import (
	"os"
	"strings"
)

// TodoItem represents a single checkbox item parsed from a note's body.
type TodoItem struct {
	Text        string
	Done        bool
	LineNumber  int // 1-based line number within the note body
	FilePath    string
	NoteTitle   string
	NoteDate    string
	NoteTags    []string
	NotePeople  []string
	NoteProject string
}

// ParseTodos scans the note body for markdown checkbox lines and returns them as TodoItems.
func (n *Note) ParseTodos() []TodoItem {
	if n.Body == "" {
		return nil
	}

	var todos []TodoItem
	lines := strings.Split(n.Body, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		var done bool
		var text string

		switch {
		case strings.HasPrefix(trimmed, "- [ ] "):
			text = strings.TrimPrefix(trimmed, "- [ ] ")
		case trimmed == "- [ ]":
			text = ""
		case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			done = true
			text = trimmed[6:]
		case trimmed == "- [x]" || trimmed == "- [X]":
			done = true
			text = ""
		default:
			continue
		}

		todos = append(todos, TodoItem{
			Text:        text,
			Done:        done,
			LineNumber:  i + 1, // 1-based
			FilePath:    n.FilePath,
			NoteTitle:   n.Title,
			NoteDate:    n.Date,
			NoteTags:    n.Tags,
			NotePeople:  n.People,
			NoteProject: n.Project,
		})
	}

	return todos
}

// CollectTodos aggregates todos from multiple notes into a single flat list.
func CollectTodos(notes []*Note) []TodoItem {
	var all []TodoItem
	for _, n := range notes {
		all = append(all, n.ParseTodos()...)
	}
	return all
}

// ToggleTodo reads the source note file, flips the checkbox on the given body line,
// and writes the file back. The line number is 1-based within the note body.
func ToggleTodo(filePath string, bodyLineNumber int) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	content := string(data)
	_, body := splitFrontmatter(content)

	// Find where the trimmed body starts in the original content.
	// Search only after the frontmatter closing delimiter to avoid
	// false matches if body text happens to appear in frontmatter.
	bodyStart := 0
	if body != "" && body != content {
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "---") {
			start := strings.Index(content, "---")
			if start != -1 {
				rest := content[start+3:]
				end := strings.Index(rest, "---")
				if end != -1 {
					searchFrom := start + 3 + end + 3
					idx := strings.Index(content[searchFrom:], body)
					if idx != -1 {
						bodyStart = searchFrom + idx
					}
				}
			}
		}
	}

	// Split full file into lines
	fileLines := strings.Split(content, "\n")

	// Count how many lines the pre-body section occupies
	preBody := content[:bodyStart]
	preBodyLineCount := strings.Count(preBody, "\n")

	// Target line in the file (0-based index)
	targetIdx := preBodyLineCount + bodyLineNumber - 1
	if targetIdx < 0 || targetIdx >= len(fileLines) {
		return nil
	}

	line := fileLines[targetIdx]
	trimmed := strings.TrimSpace(line)

	// Determine the leading whitespace
	leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

	switch {
	case strings.HasPrefix(trimmed, "- [ ] "), trimmed == "- [ ]":
		fileLines[targetIdx] = leading + strings.Replace(trimmed, "- [ ]", "- [x]", 1)
	case strings.HasPrefix(trimmed, "- [x] "), trimmed == "- [x]",
		strings.HasPrefix(trimmed, "- [X] "), trimmed == "- [X]":
		// Normalize to lowercase x when toggling back to unchecked
		toggled := strings.Replace(trimmed, "- [x]", "- [ ]", 1)
		toggled = strings.Replace(toggled, "- [X]", "- [ ]", 1)
		fileLines[targetIdx] = leading + toggled
	default:
		return nil
	}

	return os.WriteFile(filePath, []byte(strings.Join(fileLines, "\n")), 0644)
}
