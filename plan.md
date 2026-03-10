# Moss Git Sync — Implementation Plan

## Overview

Add git-based syncing to Moss so notes stay in sync across 2 MacBooks and an iPhone (via Working Copy). The approach: auto-commit locally on every file change, explicit `moss sync` to push/pull with a remote.

The database (`moss.db`) is **never synced** — it's a local index rebuilt from files. Only the `~/moss/notes/` directory is tracked by git.

---

## Phase 1: Git wrapper package (`internal/git/git.go`)

Thin wrapper around `os/exec` calls to the `git` CLI. No libraries — keep it lo-fi.

**Functions:**

- `IsRepo(dir string) bool` — Check if dir is a git repo (`git rev-parse --git-dir`)
- `Init(dir string) error` — `git init` + create `.gitignore` (ignore `*.db`, `*.db-wal`, `*.db-shm`)
- `AddAll(dir string) error` — `git add -A` (within notes dir only)
- `Commit(dir string, message string) error` — `git commit -m message`; no-op if nothing to commit
- `HasRemote(dir string) (bool, error)` — Check if `origin` remote exists
- `AddRemote(dir string, url string) error` — `git remote add origin <url>`
- `Pull(dir string) error` — `git pull origin <branch> --rebase` with conflict detection
- `Push(dir string) error` — `git push -u origin <branch>`
- `Status(dir string) (clean bool, err error)` — Any uncommitted changes?
- `HasConflicts(dir string) ([]string, error)` — List files with merge conflicts
- `CurrentBranch(dir string) string` — Get current branch name
- `Log(dir string, n int) ([]string, error)` — Last n commit messages (for status display)

All functions take `dir` as the working directory and shell out to `git -C <dir> ...`.

---

## Phase 2: Auto-commit on file change

Hook into the **existing `sync.Watcher`** to auto-commit when files change.

**Changes to `internal/sync/sync.go`:**

- Add a `GitAutoCommit` option to the Watcher
- After `handleWrite()` successfully indexes a note, call `git.AddAll()` + `git.Commit()` with an auto-generated message like `"update: <filename>"`
- After handling a delete event, commit the deletion: `"delete: <filename>"`
- After handling a rename, commit: `"rename: <old> → <new>"`
- **Debounce commits**: Use a separate timer (e.g., 1 second after last change) to batch rapid edits into a single commit instead of committing every keystroke-save
- Only auto-commit if the notes dir is a git repo (check once at watcher startup)

**Commit message format:**
```
moss: update 2026-03-10-meeting-notes.md
moss: delete 2026-03-08-old-note.md
moss: add 2026-03-10-new-idea.md
```

---

## Phase 3: Enhanced `moss sync` command

Expand `moss sync` to handle git operations. The current behavior (re-index files to DB) becomes a sub-step.

**New subcommands:**

```
moss sync              Pull from remote, re-index DB, push local commits
moss sync init         Initialize git repo in notes dir
moss sync remote <url> Set the git remote URL
moss sync status       Show sync status (ahead/behind, last sync time)
```

**`moss sync` flow (the main command):**

1. Check notes dir is a git repo → error with hint to run `moss sync init` if not
2. Check remote is configured → error with hint to run `moss sync remote <url>` if not
3. `git add -A` + commit any uncommitted changes (safety net)
4. `git pull --rebase origin main`
   - If conflicts → run conflict resolution (Phase 4), then commit the resolution
5. Re-index all notes to DB (`msync.SyncNotes()` — existing behavior)
6. `git push origin main`
7. Print summary: "Synced: 3 notes pulled, 2 notes pushed"

**`moss sync init` flow:**

1. `git init` in notes dir
2. Create `.gitignore` with `*.db*` patterns
3. Initial commit of all existing notes
4. Print next steps: "Run `moss sync remote <url>` to set up remote"

**`moss sync remote <url>` flow:**

1. Validate notes dir is a git repo
2. `git remote add origin <url>` (or `set-url` if already exists)
3. Print confirmation

---

## Phase 4: Conflict resolution

Keep it simple. For markdown files, conflicts are rare (single user, different devices, forgot to sync).

**Strategy: "last write wins" with backup**

When `git pull --rebase` fails due to conflicts:

1. For each conflicted file:
   a. Copy the conflicted version to `<filename>.conflict.md` (so nothing is lost)
   b. Accept the incoming (remote) version: `git checkout --theirs <file>`
   c. The local version is preserved in the `.conflict.md` file
2. `git add -A` + commit: `"moss: resolve conflicts (local copies saved as .conflict.md)"`
3. Print to user: "Conflict in meeting-notes.md — remote version kept, your local version saved as meeting-notes.conflict.md"
4. The user can manually reconcile later

**Why "theirs wins":** If you forgot to sync before editing on another device, the remote version is the one you most recently worked on intentionally. The local stale version is the one you forgot about. Saving it as `.conflict.md` means nothing is lost.

---

## Phase 5: Config changes (`internal/config/config.go`)

Add git sync settings to the config struct:

```go
type Config struct {
    NotesDir    string `yaml:"notes_dir"`
    DBPath      string `yaml:"db_path"`
    Autocorrect *bool  `yaml:"autocorrect,omitempty"`
    GitSync     *GitSyncConfig `yaml:"git_sync,omitempty"`
}

type GitSyncConfig struct {
    Enabled    bool   `yaml:"enabled"`              // default: true if repo exists
    AutoCommit bool   `yaml:"auto_commit"`           // default: true
    Remote     string `yaml:"remote,omitempty"`       // e.g., "git@github.com:user/notes.git"
    Branch     string `yaml:"branch,omitempty"`       // default: "main"
}
```

Config is optional — sync works with sensible defaults. If the notes dir is a git repo, auto-commit is on. The remote can be set via `moss sync remote` or directly in config.yaml.

---

## Phase 6: TUI integration (`internal/tui/model.go`)

Minimal TUI changes:

- **Keybinding**: `Ctrl+S` (or `S` in normal mode) → trigger sync (pull + push)
- **Status indicator**: Small text in the bottom bar showing sync state:
  - `[synced]` — everything pushed, nothing pending
  - `[3 pending]` — 3 local commits not yet pushed
  - `[syncing...]` — sync in progress
  - `[sync error]` — last sync failed (show message on hover/select)
  - `[no remote]` — git repo exists but no remote configured
  - ` ` (blank) — not a git repo, sync not configured
- **Sync runs in a goroutine** — non-blocking, sends a tea.Msg when complete to update the status

---

## Implementation Order

1. **Phase 1**: `internal/git/git.go` — Foundation, can test independently
2. **Phase 5**: Config changes — Need this before wiring things up
3. **Phase 3**: `moss sync` CLI commands — Core sync flow, testable from command line
4. **Phase 4**: Conflict resolution — Part of the sync flow
5. **Phase 2**: Auto-commit in watcher — Depends on git package existing
6. **Phase 6**: TUI integration — Last, depends on everything else

Phases 1 + 5 can be done together as they're independent. Phase 3 + 4 are tightly coupled. Phase 2 and 6 each build on top.

---

## What's NOT in scope

- Syncing the database (it's a local index, rebuilt from files)
- Syncing config.yaml (device-specific)
- Branch management (single branch: main)
- SSH key setup or git credential management (user's responsibility)
- Encryption at rest (notes are plaintext, user manages access to remote)
- Any cloud service integration
