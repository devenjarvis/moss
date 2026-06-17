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

This is a Go TUI app using Bubble Tea. It is a fast, fully local note-taking tool — there are no LLM/AI features and no network calls for note features. The codebase follows Go conventions with `cmd/` for entry points and `internal/` for private packages.

- `cmd/moss/main.go` — CLI entry point, subcommand dispatch, TUI startup
- `internal/tui/model.go` — Main Bubble Tea model, two-pane layout (list, preview), all key handling
- `internal/tui/editor.go` — In-app note editor with live markdown rendering and smart list continuation
- `internal/tui/styles.go` — Lip Gloss styles and color palette
- `internal/tui/markdown_renderer.go` — Markdown rendering for the preview and editor
- `internal/autocorrect/autocorrect.go` — Lightweight, fully local autocorrect (no network)
- `internal/note/note.go` — Note struct, YAML frontmatter parsing, file I/O
- `internal/db/db.go` — SQLite database with FTS5, uses `ncruces/go-sqlite3` (pure Go, no CGo)
- `internal/sync/sync.go` — Directory scanning and `fsnotify` file watcher
- `internal/config/config.go` — YAML config loading from `~/moss/config.yaml`

## Key Conventions

- Notes are plain markdown with YAML frontmatter, stored flat in `~/moss/notes/`
- Note type is tracked via frontmatter `source` field, not folder structure
- No LLM/AI: keep moss fast and local. Do not add subprocess calls to `claude` or direct Anthropic/other LLM API calls
- SQLite database at `~/moss/moss.db` indexes frontmatter fields + full-text search via FTS5
- Autocorrect is intentionally conservative and entirely local; corrections are undoable via backspace
