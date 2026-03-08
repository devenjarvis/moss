package sync

import (
	"log"
	"os"
	"path/filepath"

	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
	"github.com/fsnotify/fsnotify"
)

// SyncNotes scans the notes directory and updates the database index.
func SyncNotes(notesDir string, database *db.DB) ([]*note.Note, error) {
	paths, err := note.ListNotes(notesDir)
	if err != nil {
		return nil, err
	}

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
				w.handleEvent(event)
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

func (w *Watcher) handleEvent(event fsnotify.Event) {
	name := event.Name
	if filepath.Ext(name) != ".md" {
		return
	}

	switch {
	case event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0:
		if err := w.database.DeleteNote(name); err != nil {
			log.Printf("warning: failed to remove %s from index: %v", name, err)
		}
		if w.onChange != nil {
			w.onChange()
		}

	case event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0:
		n, err := note.ParseFile(name)
		if err != nil {
			log.Printf("warning: failed to parse %s: %v", name, err)
			return
		}
		if err := w.database.UpsertNote(n); err != nil {
			log.Printf("warning: failed to index %s: %v", name, err)
			return
		}
		if w.onChange != nil {
			w.onChange()
		}
	}
}
