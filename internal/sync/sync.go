package sync

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
	"github.com/fsnotify/fsnotify"
)

// SyncNotes scans the notes directory and updates the database index.
// It also removes stale entries for files that no longer exist on disk.
func SyncNotes(notesDir string, database *db.DB) ([]*note.Note, error) {
	paths, err := note.ListNotes(notesDir)
	if err != nil {
		return nil, err
	}

	// Build set of current file paths on disk
	onDisk := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		onDisk[p] = struct{}{}
	}

	// Remove stale DB entries for files no longer on disk
	existing, err := database.AllNotes()
	if err == nil {
		for _, n := range existing {
			if _, ok := onDisk[n.FilePath]; !ok {
				database.DeleteNote(n.FilePath)
			}
		}
	}

	// Upsert current notes
	var notes []*note.Note
	for _, p := range paths {
		n, err := note.ParseFile(p)
		if err != nil {
			log.Printf("warning: failed to parse %s: %v", p, err)
			continue
		}
		if err := database.UpsertNote(n); err != nil {
			log.Printf("warning: failed to index %s: %v", p, err)
			continue
		}
		// Sync todos for this note
		todos := n.ParseTodos()
		if err := database.UpsertTodos(n.FilePath, todos); err != nil {
			log.Printf("warning: failed to sync todos for %s: %v", p, err)
		}
		notes = append(notes, n)
	}

	return notes, nil
}

// Watcher watches the notes directory for changes and updates the database.
type Watcher struct {
	watcher  *fsnotify.Watcher
	notesDir string
	database *db.DB
	onChange func()

	// Debouncing
	mu       sync.Mutex
	pending  map[string]*time.Timer
	debounce time.Duration
}

// NewWatcher creates a file watcher for the notes directory.
func NewWatcher(notesDir string, database *db.DB, onChange func()) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(notesDir, 0755); err != nil {
		w.Close()
		return nil, err
	}

	if err := w.Add(notesDir); err != nil {
		w.Close()
		return nil, err
	}

	return &Watcher{
		watcher:  w,
		notesDir: notesDir,
		database: database,
		onChange: onChange,
		pending:  make(map[string]*time.Timer),
		debounce: 200 * time.Millisecond,
	}, nil
}

// Start begins watching for file changes in a goroutine.
func (w *Watcher) Start() {
	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				w.debounceEvent(event)
			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()
}

// Stop closes the file watcher.
func (w *Watcher) Stop() error {
	return w.watcher.Close()
}

func (w *Watcher) debounceEvent(event fsnotify.Event) {
	name := event.Name
	if filepath.Ext(name) != ".md" {
		return
	}

	// Deletes and renames are handled immediately (no debounce needed)
	if event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0 {
		w.mu.Lock()
		if t, ok := w.pending[name]; ok {
			t.Stop()
			delete(w.pending, name)
		}
		w.mu.Unlock()

		if err := w.database.DeleteNote(name); err != nil {
			log.Printf("warning: failed to remove %s from index: %v", name, err)
		}
		if w.onChange != nil {
			w.onChange()
		}
		return
	}

	// Debounce writes and creates
	if event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0 {
		w.mu.Lock()
		if t, ok := w.pending[name]; ok {
			t.Stop()
		}
		w.pending[name] = time.AfterFunc(w.debounce, func() {
			w.mu.Lock()
			delete(w.pending, name)
			w.mu.Unlock()
			w.handleWrite(name)
		})
		w.mu.Unlock()
	}
}

func (w *Watcher) handleWrite(name string) {
	n, err := note.ParseFile(name)
	if err != nil {
		log.Printf("warning: failed to parse %s: %v", name, err)
		return
	}
	if err := w.database.UpsertNote(n); err != nil {
		log.Printf("warning: failed to index %s: %v", name, err)
		return
	}
	// Sync todos for this note
	todos := n.ParseTodos()
	if err := w.database.UpsertTodos(n.FilePath, todos); err != nil {
		log.Printf("warning: failed to sync todos for %s: %v", name, err)
	}
	if w.onChange != nil {
		w.onChange()
	}
}
