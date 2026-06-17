# Contributing to Moss

Thanks for your interest in contributing to Moss! This guide will help you get started.

## Development Setup

1. Clone the repository:

```bash
git clone https://github.com/devenjarvis/moss.git
cd moss
```

2. Install [Go](https://go.dev/dl/) (1.24 or later).

3. Install [Mage](https://magefile.org/) (build tool):

```bash
go install github.com/magefile/mage@latest
```

4. Build:

```bash
mage build
```

## Running Tests

```bash
mage test
```

## Code Style

- Standard Go conventions — run `go vet ./...` before submitting
- No external linter is enforced beyond `go vet`
- Keep imports organized (stdlib, then external, then internal)

## Submitting Changes

1. Fork the repository
2. Create a feature branch (`git checkout -b my-feature`)
3. Make your changes
4. Run `mage test` and `mage vet` to verify
5. Commit with a clear message describing what and why
6. Open a pull request against `main`

## Project Structure

```
cmd/moss/           CLI entry point and subcommands
internal/
  autocorrect/      Local, lightweight autocorrect
  config/           YAML config loading
  db/               SQLite + FTS5 indexing
  note/             Note model, frontmatter parsing, file I/O
  sync/             File scanning and fsnotify watcher
  tui/              Bubble Tea TUI (model, editor, styles, keybindings)
```

## Reporting Issues

Please open a GitHub issue with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Your OS and Go version
