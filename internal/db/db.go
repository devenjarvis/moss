package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/devenjarvis/moss/internal/note"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS notes (
		file_path     TEXT PRIMARY KEY,
		title         TEXT NOT NULL DEFAULT '',
		date          TEXT NOT NULL DEFAULT '',
		tags          TEXT NOT NULL DEFAULT '',
		people        TEXT NOT NULL DEFAULT '',
		project       TEXT NOT NULL DEFAULT '',
		status        TEXT NOT NULL DEFAULT '',
		source        TEXT NOT NULL DEFAULT '',
		summary       TEXT NOT NULL DEFAULT '',
		generated_from  TEXT NOT NULL DEFAULT '',
		generated_prompt TEXT NOT NULL DEFAULT '',
		last_modified TEXT NOT NULL DEFAULT '',
		word_count    INTEGER NOT NULL DEFAULT 0,
		has_todos     INTEGER NOT NULL DEFAULT 0,
		body          TEXT NOT NULL DEFAULT ''
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
		title, body, tags, summary,
		content='notes',
		content_rowid='rowid'
	);

	CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
		INSERT INTO notes_fts(rowid, title, body, tags, summary)
		VALUES (new.rowid, new.title, new.body, new.tags, new.summary);
	END;

	CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
		INSERT INTO notes_fts(notes_fts, rowid, title, body, tags, summary)
		VALUES ('delete', old.rowid, old.title, old.body, old.tags, old.summary);
	END;

	CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
		INSERT INTO notes_fts(notes_fts, rowid, title, body, tags, summary)
		VALUES ('delete', old.rowid, old.title, old.body, old.tags, old.summary);
		INSERT INTO notes_fts(rowid, title, body, tags, summary)
		VALUES (new.rowid, new.title, new.body, new.tags, new.summary);
	END;

	CREATE TABLE IF NOT EXISTS todos (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path     TEXT NOT NULL REFERENCES notes(file_path) ON DELETE CASCADE,
		line_number   INTEGER NOT NULL,
		text          TEXT NOT NULL DEFAULT '',
		done          INTEGER NOT NULL DEFAULT 0,
		note_title    TEXT NOT NULL DEFAULT '',
		note_date     TEXT NOT NULL DEFAULT '',
		note_tags     TEXT NOT NULL DEFAULT '',
		note_people   TEXT NOT NULL DEFAULT '',
		note_project  TEXT NOT NULL DEFAULT '',
		UNIQUE(file_path, line_number)
	);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func joinStrings(ss []string) string {
	return strings.Join(ss, ",")
}

func splitStrings(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// UpsertNote inserts or updates a note in the database.
func (db *DB) UpsertNote(n *note.Note) error {
	query := `
	INSERT INTO notes (file_path, title, date, tags, people, project, status, source,
		summary, generated_from, generated_prompt, last_modified, word_count, has_todos, body)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_path) DO UPDATE SET
		title=excluded.title, date=excluded.date, tags=excluded.tags,
		people=excluded.people, project=excluded.project, status=excluded.status,
		source=excluded.source, summary=excluded.summary,
		generated_from=excluded.generated_from, generated_prompt=excluded.generated_prompt,
		last_modified=excluded.last_modified, word_count=excluded.word_count,
		has_todos=excluded.has_todos, body=excluded.body
	`

	_, err := db.conn.Exec(query,
		n.FilePath, n.Title, n.Date,
		joinStrings(n.Tags), joinStrings(n.People),
		n.Project, n.Status, n.Source, n.Summary,
		joinStrings(n.GeneratedFrom), n.GeneratedPrompt,
		n.LastModified.Format("2006-01-02T15:04:05Z"),
		n.WordCount, boolToInt(n.HasTodos), n.Body,
	)
	return err
}

// DeleteNote removes a note from the database.
func (db *DB) DeleteNote(filePath string) error {
	_, err := db.conn.Exec("DELETE FROM notes WHERE file_path = ?", filePath)
	return err
}

// AllNotes returns all notes from the database, ordered by date descending.
func (db *DB) AllNotes() ([]*note.Note, error) {
	rows, err := db.conn.Query(`
		SELECT file_path, title, date, tags, people, project, status, source,
			summary, generated_from, generated_prompt, word_count, has_todos, body
		FROM notes ORDER BY date DESC, title ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

// escapeFTS5 wraps each token in double quotes so special FTS5 characters
// (AND, OR, NOT, parentheses, asterisks, etc.) are treated as literals.
func escapeFTS5(query string) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return query
	}
	var escaped []string
	for _, t := range tokens {
		// Escape any double quotes inside the token
		t = strings.ReplaceAll(t, `"`, `""`)
		escaped = append(escaped, `"`+t+`"`)
	}
	return strings.Join(escaped, " ")
}

// Search performs full-text search across notes.
func (db *DB) Search(query string) ([]*note.Note, error) {
	safeQuery := escapeFTS5(query)
	rows, err := db.conn.Query(`
		SELECT n.file_path, n.title, n.date, n.tags, n.people, n.project, n.status,
			n.source, n.summary, n.generated_from, n.generated_prompt, n.word_count, n.has_todos, n.body
		FROM notes n
		JOIN notes_fts fts ON n.rowid = fts.rowid
		WHERE notes_fts MATCH ?
		ORDER BY rank
	`, safeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

// SearchWithTag performs full-text search scoped to notes with a specific tag.
func (db *DB) SearchWithTag(query, tag string) ([]*note.Note, error) {
	safeQuery := escapeFTS5(query)
	rows, err := db.conn.Query(`
		SELECT n.file_path, n.title, n.date, n.tags, n.people, n.project, n.status,
			n.source, n.summary, n.generated_from, n.generated_prompt, n.word_count, n.has_todos, n.body
		FROM notes n
		JOIN notes_fts fts ON n.rowid = fts.rowid
		WHERE notes_fts MATCH ?
			AND ',' || n.tags || ',' LIKE '%,' || ? || ',%'
		ORDER BY rank
	`, safeQuery, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

// ParsedQuery represents a parsed search query with field-specific filters.
type ParsedQuery struct {
	GeneralTerms  string            // general terms for FTS5 MATCH
	ColumnFilters map[string]string // FTS5 column -> search value (title, body, tags, summary)
	SQLFilters    map[string]string // non-FTS fields -> search value (project, people, status)
}

// ftsColumns are fields indexed in the FTS5 table.
var ftsColumns = map[string]bool{
	"title":   true,
	"body":    true,
	"tags":    true,
	"tag":     true, // alias for tags
	"summary": true,
}

// sqlColumns are fields that require SQL WHERE clauses (not in FTS5).
var sqlColumns = map[string]bool{
	"project": true,
	"people":  true,
	"status":  true,
}

// ParseSearchQuery splits a search input into field-specific and general terms.
// Recognized prefixes: title:, body:, tag:/tags:, summary:, project:, people:, status:.
// Supports quoted values like project:"my project".
func ParseSearchQuery(input string) ParsedQuery {
	pq := ParsedQuery{
		ColumnFilters: make(map[string]string),
		SQLFilters:    make(map[string]string),
	}

	runes := []rune(input)
	var generalTokens []string
	i := 0

	for i < len(runes) {
		// Skip whitespace
		for i < len(runes) && runes[i] == ' ' {
			i++
		}
		if i >= len(runes) {
			break
		}

		// Try to find a field prefix (word followed by colon)
		colonIdx := -1
		for j := i; j < len(runes); j++ {
			if runes[j] == ':' {
				colonIdx = j
				break
			}
			if runes[j] == ' ' {
				break
			}
		}

		if colonIdx > i {
			field := strings.ToLower(string(runes[i:colonIdx]))
			valueStart := colonIdx + 1

			if ftsColumns[field] || sqlColumns[field] {
				// Extract value (possibly quoted)
				value, newIdx := extractValue(runes, valueStart)
				i = newIdx

				if value != "" {
					// Normalize tag -> tags for FTS5
					if field == "tag" {
						field = "tags"
					}

					if ftsColumns[field] || field == "tags" {
						pq.ColumnFilters[field] = value
					} else {
						pq.SQLFilters[field] = value
					}
				}
				continue
			}
		}

		// Regular token (no recognized prefix)
		tokenEnd := i
		for tokenEnd < len(runes) && runes[tokenEnd] != ' ' {
			tokenEnd++
		}
		generalTokens = append(generalTokens, string(runes[i:tokenEnd]))
		i = tokenEnd
	}

	if len(generalTokens) > 0 {
		pq.GeneralTerms = escapeFTS5(strings.Join(generalTokens, " "))
	}

	return pq
}

// extractValue reads a value starting at idx, handling optional double-quote wrapping.
func extractValue(runes []rune, idx int) (string, int) {
	if idx >= len(runes) {
		return "", idx
	}

	if runes[idx] == '"' {
		// Quoted value: scan to closing quote
		idx++ // skip opening quote
		start := idx
		for idx < len(runes) && runes[idx] != '"' {
			idx++
		}
		value := string(runes[start:idx])
		if idx < len(runes) {
			idx++ // skip closing quote
		}
		return value, idx
	}

	// Unquoted value: scan to next space
	start := idx
	for idx < len(runes) && runes[idx] != ' ' {
		idx++
	}
	return string(runes[start:idx]), idx
}

// SearchAdvanced performs search with field-specific filters and an optional tag scope.
func (db *DB) SearchAdvanced(pq ParsedQuery, tag string) ([]*note.Note, error) {
	var conditions []string
	var args []interface{}
	useFTS := false

	// Build FTS5 MATCH clause
	var ftsTerms []string
	if pq.GeneralTerms != "" {
		ftsTerms = append(ftsTerms, pq.GeneralTerms)
	}
	for col, val := range pq.ColumnFilters {
		escaped := strings.ReplaceAll(val, `"`, `""`)
		ftsTerms = append(ftsTerms, fmt.Sprintf(`{%s} : "%s"`, col, escaped))
	}
	if len(ftsTerms) > 0 {
		useFTS = true
		conditions = append(conditions, "notes_fts MATCH ?")
		args = append(args, strings.Join(ftsTerms, " "))
	}

	// SQL WHERE filters for non-FTS columns
	for col, val := range pq.SQLFilters {
		conditions = append(conditions, fmt.Sprintf("n.%s LIKE '%%' || ? || '%%'", col))
		args = append(args, val)
	}

	// Tag scope filter
	if tag != "" {
		conditions = append(conditions, "',' || n.tags || ',' LIKE '%,' || ? || ',%'")
		args = append(args, tag)
	}

	// Build query
	var query string
	if useFTS {
		where := ""
		if len(conditions) > 0 {
			where = "WHERE " + strings.Join(conditions, " AND ")
		}
		query = fmt.Sprintf(`
			SELECT n.file_path, n.title, n.date, n.tags, n.people, n.project, n.status,
				n.source, n.summary, n.generated_from, n.generated_prompt, n.word_count, n.has_todos, n.body
			FROM notes n
			JOIN notes_fts fts ON n.rowid = fts.rowid
			%s
			ORDER BY rank
		`, where)
	} else if len(conditions) > 0 {
		query = fmt.Sprintf(`
			SELECT n.file_path, n.title, n.date, n.tags, n.people, n.project, n.status,
				n.source, n.summary, n.generated_from, n.generated_prompt, n.word_count, n.has_todos, n.body
			FROM notes n
			WHERE %s
			ORDER BY n.date DESC
		`, strings.Join(conditions, " AND "))
	} else {
		// No filters at all — return all notes
		return db.AllNotes()
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

// FilterByTag returns notes that contain the given tag.
func (db *DB) FilterByTag(tag string) ([]*note.Note, error) {
	rows, err := db.conn.Query(`
		SELECT file_path, title, date, tags, people, project, status, source,
			summary, generated_from, generated_prompt, word_count, has_todos, body
		FROM notes
		WHERE ',' || tags || ',' LIKE '%,' || ? || ',%'
		ORDER BY date DESC
	`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

// AllTags returns all unique tags across all notes, sorted alphabetically.
func (db *DB) AllTags() ([]string, error) {
	rows, err := db.conn.Query("SELECT tags FROM notes WHERE tags != ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			return nil, err
		}
		for _, t := range splitStrings(tags) {
			t = strings.TrimSpace(t)
			if t != "" {
				seen[t] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []string
	for t := range seen {
		result = append(result, t)
	}
	// Sort alphabetically
	sort.Strings(result)
	return result, nil
}

// AllNotesSorted returns all notes sorted by the specified field.
func (db *DB) AllNotesSorted(sortBy string) ([]*note.Note, error) {
	var orderClause string
	switch sortBy {
	case "title":
		orderClause = "ORDER BY title ASC, date DESC"
	case "modified":
		orderClause = "ORDER BY last_modified DESC, title ASC"
	case "words":
		orderClause = "ORDER BY word_count DESC, title ASC"
	default:
		orderClause = "ORDER BY date DESC, title ASC"
	}

	rows, err := db.conn.Query(fmt.Sprintf(`
		SELECT file_path, title, date, tags, people, project, status, source,
			summary, generated_from, generated_prompt, word_count, has_todos, body
		FROM notes %s
	`, orderClause))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

func scanNotes(rows *sql.Rows) ([]*note.Note, error) {
	var notes []*note.Note
	for rows.Next() {
		n := &note.Note{}
		var tags, people, genFrom string
		var hasTodos int

		err := rows.Scan(
			&n.FilePath, &n.Title, &n.Date, &tags, &people,
			&n.Project, &n.Status, &n.Source, &n.Summary,
			&genFrom, &n.GeneratedPrompt, &n.WordCount, &hasTodos,
			&n.Body,
		)
		if err != nil {
			return nil, err
		}

		n.Tags = splitStrings(tags)
		n.People = splitStrings(people)
		n.GeneratedFrom = splitStrings(genFrom)
		n.HasTodos = hasTodos != 0
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpsertTodos replaces all todos for a given note file path.
// Pass nil or empty todos to clear all todos for the file.
func (db *DB) UpsertTodos(filePath string, todos []note.TodoItem) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM todos WHERE file_path = ?", filePath); err != nil {
		return err
	}

	for _, t := range todos {
		_, err := tx.Exec(`
			INSERT INTO todos (file_path, line_number, text, done,
				note_title, note_date, note_tags, note_people, note_project)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.FilePath, t.LineNumber, t.Text, boolToInt(t.Done),
			t.NoteTitle, t.NoteDate,
			joinStrings(t.NoteTags), joinStrings(t.NotePeople), t.NoteProject,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// AllTodos returns todos filtered by status. Filter values: "open", "done", "all".
func (db *DB) AllTodos(filter string) ([]note.TodoItem, error) {
	var whereClause string
	switch filter {
	case "done":
		whereClause = "WHERE done = 1"
	case "all":
		whereClause = ""
	default: // "open"
		whereClause = "WHERE done = 0"
	}

	query := fmt.Sprintf(`
		SELECT file_path, line_number, text, done,
			note_title, note_date, note_tags, note_people, note_project
		FROM todos %s
		ORDER BY note_date DESC, file_path, line_number
	`, whereClause)

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTodos(rows)
}

// TodoProjects returns distinct project names from open todos.
func (db *DB) TodoProjects() ([]string, error) {
	rows, err := db.conn.Query(
		"SELECT DISTINCT note_project FROM todos WHERE note_project != '' AND done = 0 ORDER BY note_project",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func scanTodos(rows *sql.Rows) ([]note.TodoItem, error) {
	var todos []note.TodoItem
	for rows.Next() {
		var t note.TodoItem
		var done int
		var tags, people string

		err := rows.Scan(
			&t.FilePath, &t.LineNumber, &t.Text, &done,
			&t.NoteTitle, &t.NoteDate, &tags, &people, &t.NoteProject,
		)
		if err != nil {
			return nil, err
		}

		t.Done = done != 0
		t.NoteTags = splitStrings(tags)
		t.NotePeople = splitStrings(people)
		todos = append(todos, t)
	}
	return todos, rows.Err()
}
