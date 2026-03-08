package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devenjarvis/moss/internal/ai"
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
		case "ask":
			cmdAsk(os.Args[2:])
		case "sync":
			cmdSync()
		case "generate":
			cmdGenerate(os.Args[2:])
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
	fmt.Printf("Moss %s - AI-powered note-taking TUI\n", version.Version)
	fmt.Println(`
Usage:
  moss                    Launch the TUI
  moss new [title]        Create a new note and open in $EDITOR
  moss ask "question"     Query across your notes
  moss sync               Scan for new/changed files and rebuild index
  moss generate "prompt"  Generate a new note from a prompt
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

	// Background AI worker
	worker := ai.NewWorker(100)
	worker.Start(context.Background())
	defer worker.Stop()

	// Queue frontmatter generation for notes with missing fields
	queueFrontmatterTasks(cfg, database, worker)

	// Create TUI model
	model := tui.New(cfg, database, worker)

	// File watcher
	watcher, err := msync.NewWatcher(cfg.NotesDir, database, nil)
	if err == nil {
		watcher.Start()
		defer watcher.Stop() //nolint:errcheck
		model.SetWatcher(watcher)
	}

	// Run the TUI
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func queueFrontmatterTasks(cfg config.Config, database *db.DB, worker *ai.Worker) {
	notes, err := database.AllNotes()
	if err != nil {
		return
	}

	// Collect notes that need frontmatter generation
	var toProcess []*note.Note
	for _, n := range notes {
		if len(n.MissingFrontmatterFields()) > 0 {
			fullNote, err := note.ParseFile(n.FilePath)
			if err != nil {
				continue
			}
			toProcess = append(toProcess, fullNote)
		}
	}

	if len(toProcess) == 0 {
		return
	}

	// Create a shared result channel and submit tasks
	resultCh := make(chan ai.Result, len(toProcess))
	for _, fullNote := range toProcess {
		worker.Submit(ai.Task{
			Type:     "frontmatter",
			Note:     fullNote,
			Model:    ai.ModelHaiku,
			ResultCh: resultCh,
		})
	}

	// Process results in background goroutine
	go func() {
		for i := 0; i < len(toProcess); i++ {
			result := <-resultCh
			if result.Err != nil || result.Output == "" {
				continue
			}
			n := result.Task.Note
			// Apply generated fields only for fields that are still missing
			for _, line := range strings.Split(result.Output, "\n") {
				parts := strings.SplitN(line, ": ", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				switch key {
				case "title":
					if n.Title == "" {
						n.Title = val
					}
				case "date":
					if n.Date == "" {
						n.Date = val
					}
				case "summary":
					if n.Summary == "" {
						n.Summary = val
					}
				case "tags":
					if len(n.Tags) == 0 {
						val = strings.Trim(val, "[]")
						for _, t := range strings.Split(val, ",") {
							t = strings.TrimSpace(t)
							t = strings.Trim(t, "\"'")
							if t != "" {
								n.Tags = append(n.Tags, t)
							}
						}
					}
				case "status":
					if n.Status == "" {
						n.Status = val
					}
				case "source":
					if n.Source == "" {
						n.Source = val
					}
				}
			}
			if err := n.WriteFrontmatter(); err != nil {
				continue
			}
			_ = database.UpsertNote(n)
		}
	}()
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

	// Open in editor
	editor := cfg.Editor
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Editor error: %v\n", err)
	}

	// Index the new note
	database := mustOpenDB(cfg)
	defer database.Close() //nolint:errcheck

	n, err := note.ParseFile(path)
	if err == nil {
		_ = database.UpsertNote(n)
	}
}

func cmdAsk(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: moss ask \"question\"")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close() //nolint:errcheck

	question := strings.Join(args, " ")

	// Gather all note content
	notes, err := database.AllNotes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading notes: %v\n", err)
		os.Exit(1)
	}

	var sb strings.Builder
	for _, n := range notes {
		fullNote, err := note.ParseFile(n.FilePath)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("--- %s (%s) ---\n%s\n\n", n.Title, n.Date, fullNote.Body))
	}

	response, err := ai.Ask(context.Background(), question, sb.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(response)
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

func cmdGenerate(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: moss generate \"prompt\"")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close() //nolint:errcheck

	prompt := strings.Join(args, " ")

	// Gather source notes for context
	notes, err := database.AllNotes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading notes: %v\n", err)
		os.Exit(1)
	}

	var sb strings.Builder
	var sourcePaths []string
	for _, n := range notes {
		fullNote, err := note.ParseFile(n.FilePath)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", n.Title, fullNote.Body))
		sourcePaths = append(sourcePaths, n.FilePath)
	}

	content, err := ai.GenerateNote(context.Background(), prompt, sb.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating note: %v\n", err)
		os.Exit(1)
	}

	// Parse the generated content to extract the title for the filename
	frontmatter, _ := extractFrontmatter(content)
	title := "generated"
	if frontmatter != "" {
		// Try to parse title from frontmatter
		for _, line := range strings.Split(frontmatter, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "title:") {
				title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "title:"))
				title = strings.Trim(title, "\"'")
				break
			}
		}
	}

	// Write the file
	path, err := note.CreateNew(cfg.NotesDir, title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
		os.Exit(1)
	}

	// Overwrite with generated content, adding provenance
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	// Index it
	n, err := note.ParseFile(path)
	if err == nil {
		n.Source = "generated"
		n.GeneratedPrompt = prompt
		n.GeneratedFrom = sourcePaths
		_ = n.WriteFrontmatter()
		_ = database.UpsertNote(n)
	}

	fmt.Printf("Generated: %s\n", path)
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

func extractFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return "", content
	}
	trimmed := strings.TrimSpace(content)
	start := strings.Index(trimmed, "---")
	rest := trimmed[start+3:]
	end := strings.Index(rest, "---")
	if end == -1 {
		return "", content
	}
	return strings.TrimSpace(rest[:end]), strings.TrimSpace(rest[end+3:])
}
