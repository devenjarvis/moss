package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	home, _ := os.UserHomeDir()
	expectedNotesDir := filepath.Join(home, "moss", "notes")
	expectedDBPath := filepath.Join(home, "moss", "moss.db")

	if cfg.NotesDir != expectedNotesDir {
		t.Errorf("NotesDir = %q, want %q", cfg.NotesDir, expectedNotesDir)
	}
	if cfg.DBPath != expectedDBPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, expectedDBPath)
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	// Load should succeed even without a config file, using defaults
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.NotesDir == "" {
		t.Error("NotesDir should have a default value")
	}
	if cfg.DBPath == "" {
		t.Error("DBPath should have a default value")
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", originalHome) })

	// Create temp home directory with config
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, "moss")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `notes_dir: /custom/notes
db_path: /custom/db.sqlite
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.NotesDir != "/custom/notes" {
		t.Errorf("NotesDir = %q, want %q", cfg.NotesDir, "/custom/notes")
	}
	if cfg.DBPath != "/custom/db.sqlite" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/custom/db.sqlite")
	}
}

func TestLoad_PartialConfig(t *testing.T) {
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", originalHome) })

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, "moss")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Only set notes_dir, leave others to defaults
	configContent := `notes_dir: /custom/notes
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.NotesDir != "/custom/notes" {
		t.Errorf("NotesDir = %q, want %q", cfg.NotesDir, "/custom/notes")
	}
	// DBPath should have default
	if cfg.DBPath == "" {
		t.Error("DBPath should have a default value")
	}
}
