package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/devenjarvis/moss/internal/config"
	"github.com/devenjarvis/moss/internal/db"
	"github.com/devenjarvis/moss/internal/note"
	msync "github.com/devenjarvis/moss/internal/sync"
	"github.com/devenjarvis/moss/internal/tui"
	"github.com/devenjarvis/moss/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "new":
			cmdNew(os.Args[2:])
		case "sync":
			cmdSync()
		case "uninstall":
			cmdUninstall(os.Args[2:])
		case "version", "--version", "-v":
			fmt.Println("moss " + version.Full())
		case "help", "--help", "-h":
			printUsage()
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
		return
	}

	// Launch TUI
	runTUI()
}

func printUsage() {
	fmt.Printf("Moss %s - a fast, friendly note-taking TUI\n", version.Version)
	fmt.Println(`
Usage:
  moss                    Launch the TUI
  moss new [title]        Create a new note
  moss sync               Scan for new/changed files and rebuild index
  moss uninstall [--all]  Remove moss (preserves notes by default)
  moss version            Show version information
  moss help               Show this help message`)
}

func mustLoadConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func mustOpenDB(cfg config.Config) *db.DB {
	// Ensure parent directory exists
	if err := os.MkdirAll(cfg.NotesDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating notes directory: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	return database
}

func runTUI() {
	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close() //nolint:errcheck

	// Initial sync
	_, _ = msync.SyncNotes(cfg.NotesDir, database)

	// Create TUI model
	model := tui.New(cfg, database)

	// File watcher
	watcher, err := msync.NewWatcher(cfg.NotesDir, database, nil)
	if err == nil {
		watcher.Start()
		defer watcher.Stop() //nolint:errcheck
		model.SetWatcher(watcher)
	}

	// Silence the default logger during TUI mode — log.Printf writes to
	// stderr which corrupts the alt-screen display in Bubble Tea v2.
	log.SetOutput(io.Discard)

	// Run the TUI
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdNew(args []string) {
	cfg := mustLoadConfig()

	title := "untitled"
	if len(args) > 0 {
		title = strings.Join(args, " ")
	}

	path, err := note.CreateNew(cfg.NotesDir, title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating note: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created: %s\n", path)

	// Index the new note
	database := mustOpenDB(cfg)
	defer database.Close() //nolint:errcheck

	n, err := note.ParseFile(path)
	if err == nil {
		_ = database.UpsertNote(n)
	}

	fmt.Println("Run 'moss' to edit in the TUI")
}

func cmdSync() {
	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close() //nolint:errcheck

	notes, err := msync.SyncNotes(cfg.NotesDir, database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Synced %d notes\n", len(notes))
}

func cmdUninstall(args []string) {
	removeAll := false
	for _, arg := range args {
		if arg == "--all" {
			removeAll = true
		}
	}

	cfg := mustLoadConfig()

	// Determine binary location
	execPath, err := os.Executable()
	if err != nil {
		execPath = "(could not determine)"
	} else {
		execPath, _ = filepath.EvalSymlinks(execPath)
	}

	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, "moss", "config.yaml")
	versionCheck := filepath.Join(home, "moss", ".version-check")

	fmt.Println("Moss Uninstall")
	fmt.Println("==============")
	fmt.Println()
	fmt.Println("This will remove:")
	fmt.Printf("  Binary:    %s\n", execPath)
	fmt.Printf("  Database:  %s\n", cfg.DBPath)
	fmt.Printf("  Config:    %s\n", configPath)

	if removeAll {
		fmt.Println()
		fmt.Printf("  Notes:     %s (ALL NOTES WILL BE DELETED)\n", cfg.NotesDir)
	} else {
		fmt.Println()
		fmt.Println("Your notes will be PRESERVED:")
		fmt.Printf("  Notes:     %s\n", cfg.NotesDir)
		fmt.Println()
		fmt.Println("To also remove notes, run: moss uninstall --all")
	}

	fmt.Println()
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("Uninstall cancelled.")
		return
	}

	fmt.Println()

	// Remove database
	removeFile(cfg.DBPath)
	removeFile(cfg.DBPath + "-wal")
	removeFile(cfg.DBPath + "-shm")

	// Remove config
	removeFile(configPath)

	// Remove version check cache
	removeFile(versionCheck)

	// Remove notes if --all
	if removeAll {
		if err := os.RemoveAll(cfg.NotesDir); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not remove %s: %v\n", cfg.NotesDir, err)
		} else {
			fmt.Printf("  Removed %s\n", cfg.NotesDir)
		}
		// Try to remove the ~/moss directory if empty
		mossDir := filepath.Join(home, "moss")
		_ = os.Remove(mossDir) // only succeeds if empty
	}

	// Remove binary last (process can continue after unlinking on Unix)
	if execPath != "(could not determine)" {
		if err := os.Remove(execPath); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not remove binary %s: %v\n", execPath, err)
			fmt.Println("  You may need to remove it manually (e.g., with sudo).")
		} else {
			fmt.Printf("  Removed %s\n", execPath)
		}
	}

	fmt.Println()
	if removeAll {
		fmt.Println("Uninstall complete.")
	} else {
		fmt.Printf("Uninstall complete. Your notes are still at %s\n", cfg.NotesDir)
	}
}

func removeFile(path string) {
	if _, err := os.Stat(path); err != nil {
		return // doesn't exist
	}
	if err := os.Remove(path); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not remove %s: %v\n", path, err)
	} else {
		fmt.Printf("  Removed %s\n", path)
	}
}
