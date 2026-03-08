package note

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Note struct {
	// Frontmatter fields
	Title           string   `yaml:"title"`
	Date            string   `yaml:"date"`
	Tags            []string `yaml:"tags,omitempty"`
	People          []string `yaml:"people,omitempty"`
	Project         string   `yaml:"project,omitempty"`
	Status          string   `yaml:"status,omitempty"`
	Source          string   `yaml:"source,omitempty"`
	Summary         string   `yaml:"summary,omitempty"`
	GeneratedFrom   []string `yaml:"generated_from,omitempty"`
	GeneratedPrompt string   `yaml:"generated_prompt,omitempty"`

	// Computed fields (not in frontmatter)
	FilePath     string    `yaml:"-"`
	Body         string    `yaml:"-"`
	LastModified time.Time `yaml:"-"`
	WordCount    int       `yaml:"-"`
	HasTodos     bool      `yaml:"-"`
}

// ParseFile reads a markdown file and parses its frontmatter and body.
func ParseFile(path string) (*Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	note := &Note{FilePath: path}

	info, err := os.Stat(path)
	if err == nil {
		note.LastModified = info.ModTime()
	}

	content := string(data)
	frontmatter, body := splitFrontmatter(content)

	if frontmatter != "" {
		if err := yaml.Unmarshal([]byte(frontmatter), note); err != nil {
			// If frontmatter is invalid, treat entire content as body
			note.Body = content
		} else {
			note.Body = body
		}
	} else {
		note.Body = content
	}

	note.WordCount = countWords(note.Body)
	note.HasTodos = detectTodos(note.Body)

	// Derive title from filename if not set
	if note.Title == "" {
		base := filepath.Base(path)
		note.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return note, nil
}

// splitFrontmatter splits YAML frontmatter (between --- delimiters) from body.
func splitFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return "", content
	}

	trimmed := strings.TrimSpace(content)
	// Find the opening ---
	start := strings.Index(trimmed, "---")
	if start == -1 {
		return "", content
	}

	// Find the closing ---
	rest := trimmed[start+3:]
	end := strings.Index(rest, "---")
	if end == -1 {
		return "", content
	}

	fm := strings.TrimSpace(rest[:end])
	body := strings.TrimSpace(rest[end+3:])
	return fm, body
}

func countWords(s string) int {
	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Split(bufio.ScanWords)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}

func detectTodos(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "- [ ]") || strings.Contains(lower, "- [x]") ||
		strings.Contains(lower, "todo")
}

// MissingFrontmatterFields returns a list of core fields that are empty.
func (n *Note) MissingFrontmatterFields() []string {
	var missing []string
	if n.Title == "" {
		missing = append(missing, "title")
	}
	if n.Date == "" {
		missing = append(missing, "date")
	}
	if n.Summary == "" {
		missing = append(missing, "summary")
	}
	if len(n.Tags) == 0 {
		missing = append(missing, "tags")
	}
	if n.Status == "" {
		missing = append(missing, "status")
	}
	if n.Source == "" {
		missing = append(missing, "source")
	}
	return missing
}

// WriteFrontmatter updates the frontmatter in the file without touching the body.
func (n *Note) WriteFrontmatter() error {
	fmBytes, err := yaml.Marshal(n)
	if err != nil {
		return err
	}

	content := fmt.Sprintf("---\n%s---\n\n%s", string(fmBytes), n.Body)
	return os.WriteFile(n.FilePath, []byte(content), 0644)
}

// CreateNew creates a new note file with the given title.
func CreateNew(dir, title string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Generate filename from title
	slug := slugify(title)
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", date, slug)
	path := filepath.Join(dir, filename)

	note := &Note{
		Title:  title,
		Date:   date,
		Status: "active",
		Source: "written",
	}

	fmBytes, err := yaml.Marshal(note)
	if err != nil {
		return "", err
	}

	content := fmt.Sprintf("---\n%s---\n\n# %s\n\n", string(fmBytes), title)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return path, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// RenameToTitle renames the note file to match its frontmatter title.
// Returns the new file path, or the original path if no rename was needed.
func (n *Note) RenameToTitle() (string, error) {
	if n.Title == "" {
		return n.FilePath, nil
	}

	dir := filepath.Dir(n.FilePath)
	slug := slugify(n.Title)
	date := n.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	newFilename := fmt.Sprintf("%s-%s.md", date, slug)
	newPath := filepath.Join(dir, newFilename)

	// Don't rename if path is already correct
	if newPath == n.FilePath {
		return n.FilePath, nil
	}

	// Don't overwrite an existing file
	if _, err := os.Stat(newPath); err == nil {
		return n.FilePath, nil
	}

	if err := os.Rename(n.FilePath, newPath); err != nil {
		return n.FilePath, err
	}

	n.FilePath = newPath
	return newPath, nil
}

// ListNotes returns all markdown files in the given directory.
func ListNotes(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths, nil
}
