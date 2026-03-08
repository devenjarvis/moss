# CLAUDE.md

## Build & Test

```bash
# Build (using mage)
mage build

# Or directly with go
go build -o moss ./cmd/moss/

# Run
./moss              # TUI
./moss sync         # re-index notes
./moss help         # usage

# Test
mage test           # or: go test ./...

# Vet + Test
mage check

# Tidy modules
mage tidy           # or: go mod tidy
```

## Architecture

This is a Go TUI app using Bubble Tea. The codebase follows Go conventions with `cmd/` for entry points and `internal/` for private packages.

- `cmd/moss/main.go` — CLI entry point, subcommand dispatch, TUI startup
- `internal/tui/model.go` — Main Bubble Tea model, three-pane layout (list, preview, chat), all key handling
- `internal/tui/styles.go` — Lip Gloss styles and color palette
- `internal/tui/keys.go` — Key binding definitions
- `internal/note/note.go` — Note struct, YAML frontmatter parsing, file I/O
- `internal/db/db.go` — SQLite database with FTS5, uses `ncruces/go-sqlite3` (pure Go, no CGo)
- `internal/ai/ai.go` — Claude CLI subprocess calls (`os/exec`), background worker goroutine
- `internal/sync/sync.go` — Directory scanning and `fsnotify` file watcher
- `internal/config/config.go` — YAML config loading from `~/moss/config.yaml`

## Key Conventions

- Notes are plain markdown with YAML frontmatter, stored flat in `~/moss/notes/`
- Note type (daily, permanent, generated) is tracked via frontmatter `source` field, not folder structure
- AI features call the `claude` CLI as a subprocess — never call the Anthropic API directly
- Two AI tiers: Haiku for background/automatic tasks, Sonnet (default) for user-facing tasks
- Claude subprocess input is piped via stdin, not passed as CLI arguments
- SQLite database at `~/moss/moss.db` indexes frontmatter fields + full-text search via FTS5
- Never overwrite frontmatter fields that already have values during AI generation
