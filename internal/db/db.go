package db

import (
	"database/sql"
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
			summary, generated_from, generated_prompt, word_count, has_todos
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
			n.source, n.summary, n.generated_from, n.generated_prompt, n.word_count, n.has_todos
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

// FilterByTag returns notes that contain the given tag.
func (db *DB) FilterByTag(tag string) ([]*note.Note, error) {
	rows, err := db.conn.Query(`
		SELECT file_path, title, date, tags, people, project, status, source,
			summary, generated_from, generated_prompt, word_count, has_todos
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
