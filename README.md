# Moss

[![Release](https://img.shields.io/github/v/release/devenjarvis/moss)](https://github.com/devenjarvis/moss/releases)
[![Test](https://github.com/devenjarvis/moss/actions/workflows/test.yml/badge.svg)](https://github.com/devenjarvis/moss/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/devenjarvis/moss)](https://goreportcard.com/report/github.com/devenjarvis/moss)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

AI-powered note-taking TUI built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

Notes are plain markdown files with YAML frontmatter, stored in `~/moss/notes/`. A SQLite database indexes all notes for fast full-text search. AI features (summarization, tagging, Q&A, note generation) are powered by the Claude CLI.

## Install

### Homebrew (macOS/Linux)

```bash
brew install devenjarvis/tap/moss
```

### Shell Script (macOS/Linux)

```bash
curl -sSfL https://raw.githubusercontent.com/devenjarvis/moss/main/install.sh | sh
```

### Go Install

```bash
go install github.com/devenjarvis/moss/cmd/moss@latest
```

### GitHub Releases

Download pre-built binaries from [Releases](https://github.com/devenjarvis/moss/releases).

### Build from Source

```bash
git clone https://github.com/devenjarvis/moss.git
cd moss
go build -o moss ./cmd/moss/
```

### Updating

- **Homebrew:** `brew upgrade moss`
- **Shell script:** re-run the install script above
- **Go:** re-run `go install github.com/devenjarvis/moss/cmd/moss@latest`

## Usage

```
moss                    Launch the TUI
moss new [title]        Create a new note and open in $EDITOR
moss ask "question"     Query across your notes using AI
moss sync               Scan for new/changed files and rebuild index
moss generate "prompt"  Generate a new note from a prompt
moss version            Show version information
moss uninstall [--all]  Remove moss (preserves notes by default)
moss help               Show usage information
```

## TUI Keybindings

| Key | Action |
|-----|--------|
| `j/k`, `↑/↓` | Move up/down |
| `h/l`, `←/→` | Switch panes |
| `Tab` / `Shift+Tab` | Next/previous pane |
| `Enter` | Open note in editor |
| `/` | Search notes |
| `c` | Chat with AI |
| `n` | New note |
| `d` | Delete note |
| `g` | Generate note from AI prompt |
| `t` | Filter by tag |
| `T` | View TODOs |
| `o` | Cycle sort order (date/title/modified/words) |
| `s` | Sync & re-index |
| `Ctrl+d/u` | Scroll half page |
| `?` | Help overlay |
| `Esc` | Cancel / go back |
| `q` | Quit |

## Search Syntax

Press `/` to search. Moss supports full-text search with field-specific prefixes:

- Type freely for general full-text search across all notes
- `title:keyword` — search by title
- `tag:go` or `tags:go` — filter by tag
- `project:moss` — filter by project
- `people:name` — filter by people
- `status:active` — filter by status
- Use quotes for values with spaces: `project:"my project"`

## TODO Management

Press `T` to open the TODO view, which collects all `- [ ]` and `- [x]` items from your notes.

| Key | Action |
|-----|--------|
| `Space` / `x` | Toggle checkbox |
| `Enter` | Jump to source note |
| `f` | Cycle filter (open/done/all) |
| `Esc` | Return to notes |

## Layout

Three-pane TUI:
- **Left** — Note list (filterable via `/` search). AI-generated notes are marked with `*`, notes with TODOs with `+`.
- **Center** — Markdown preview (rendered with [Glamour](https://github.com/charmbracelet/glamour))
- **Right** — AI chat pane

## Note Format

Notes are markdown files with YAML frontmatter:

```yaml
---
title: My Note
date: 2026-03-08
tags: [go, tui]
people: []
project: moss
status: active
source: written
summary: A short summary of the note contents.
---
```

Generated notes also include `generated_from` (source note paths) and `generated_prompt` fields for provenance tracking.

## AI Integration

Moss calls the `claude` CLI as a subprocess — it does not call the Anthropic API directly. Two tiers:

- **Haiku** (`claude-haiku-4-5-20251001`) — Background tasks: frontmatter generation, summarization, tag suggestion. Runs automatically on notes with missing fields.
- **Sonnet** (default) — User-facing tasks: queries, cross-note questions, note generation.

Requires the [Claude CLI](https://docs.anthropic.com/en/docs/claude-code) to be installed and authenticated.

## Configuration

Optional config file at `~/moss/config.yaml`:

```yaml
notes_dir: ~/moss/notes
db_path: ~/moss/moss.db
editor: vim
```

All fields are optional and fall back to defaults.

## Architecture

```
cmd/moss/           CLI entry point and subcommands
internal/
  ai/               Claude CLI subprocess integration, background worker
  config/           YAML config loading
  db/               SQLite + FTS5 indexing
  note/             Note model, frontmatter parsing, file operations
  sync/             File scanning and fsnotify watcher
  tui/              Bubble Tea TUI (model, styles, keybindings)
```

## Uninstall

```bash
moss uninstall        # Removes binary, database, config (preserves notes)
moss uninstall --all  # Removes everything including notes
```

If installed via Homebrew:

```bash
brew uninstall moss
```

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — TUI styling
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering
- [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3) — Pure Go SQLite (no CGo)
- [fsnotify](https://github.com/fsnotify/fsnotify) — File system watcher
