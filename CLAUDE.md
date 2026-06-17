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
- `internal/tui/theme.go` — Design tokens: color palette, spacing, borders, glyphs (the only place raw hex/glyphs live)
- `internal/tui/styles.go` — Named Lip Gloss styles composed from tokens
- `internal/tui/component.go` — Reusable UI components (pane, list item, status bar, modal)
- `internal/tui/markdown_renderer.go` — Markdown rendering for the preview and editor

## Design System

`docs/DESIGN.md` is the single source of truth for moss's UI. **Read it before
touching anything visual.** The UI is layered — tokens (`theme.go`) → styles
(`styles.go`) → components (`component.go`) → screens (`model.go`, `editor.go`) —
and each layer may only depend on the ones above it. Enforced rules:

- Raw hex colors and glyphs live **only** in `theme.go`. Reference semantic
  tokens (`colorPrimary`, `colorMuted`, `glyphCursor`, …) everywhere else.
- `lipgloss.NewStyle()` is called **only** in `styles.go` / `component.go`.
  Screens reference named styles — never build styles inline.
- Screens assemble components; they do not draw borders, pad, gap-fill, or
  truncate by hand. Add a component for any UI element used more than once.
- When you change the design system, update `docs/DESIGN.md` in the same change.
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
