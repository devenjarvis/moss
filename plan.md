# Moss Sync — Implementation Plan

## Overview

Sync notes across devices by letting users point `notes_dir` at any cloud-synced folder (iCloud Drive, Dropbox, Syncthing, Google Drive, OneDrive, etc.). Moss doesn't implement its own transport — it becomes a good citizen of whatever sync tool the user already has.

On iPhone: use iCloud Drive + a free markdown editor like Paper.

The database (`moss.db`) is **never synced** — it's a local-only index rebuilt from files. Only the notes directory is synced by the external tool.

---

## What needs to change

### 1. Conflict file detection and resolution (`internal/sync/conflict.go`)

Cloud sync services create conflict files with predictable naming patterns when the same file is modified on two devices:

| Service | Pattern |
|---------|---------|
| iCloud | `filename 2.md`, `filename 3.md` |
| Dropbox | `filename (conflicted copy 2026-03-10).md` |
| Syncthing | `filename.sync-conflict-20260310-123456.md` |
| OneDrive | `filename-DEVICE.md` |
| Google Drive | Creates separate versions, no filename change |

**New functions:**

- `IsConflictFile(filename string) bool` — Match against known conflict patterns
- `OriginalFilename(conflictFile string) string` — Extract the original filename from a conflict file
- `DetectConflicts(notesDir string) []ConflictGroup` — Scan for conflict pairs/groups

**`ConflictGroup` struct:**
```go
type ConflictGroup struct {
    Original  string   // path to the original file
    Conflicts []string // paths to conflict copies
    Service   string   // detected sync service ("icloud", "dropbox", etc.)
}
```

### 2. Conflict resolution UI in TUI (`internal/tui/`)

When conflicts are detected (at startup or via watcher), surface them to the user.

**Approach:** Add a conflicts list/notification to the TUI.

- On startup and when watcher detects a new conflict file, check for conflicts
- Show a `[2 conflicts]` indicator in the status bar
- Keybinding (e.g., `C`) opens a conflict resolution view:
  - Shows the original and conflict files side by side (or sequentially in the preview pane)
  - User picks: **Keep original**, **Keep conflict version**, or **Keep both** (rename conflict to a proper note)
  - Resolving deletes the losing file and re-indexes

**Keep it simple for v1:** Just detect and surface conflicts. The user can also resolve manually by deleting the unwanted file — Moss doesn't need to force a workflow.

### 3. Watcher hardening (`internal/sync/sync.go`)

The existing watcher works well for local edits but needs hardening for cloud sync edge cases:

**a. Handle temporary/partial files:**
- Cloud services often write `.tmp`, `.partial`, or `~filename` files during sync
- Filter these out in `debounceEvent()` (currently only checks `.md` extension — this is already correct, but add explicit exclusion for patterns like `.md.icloud`, `.md.tmp`)

**b. Handle iCloud placeholder files:**
- On macOS, iCloud creates `.icloud` placeholder files for files not downloaded locally (e.g., `.filename.md.icloud`)
- These should be ignored by the watcher
- Consider: should Moss trigger a download? (`brctl download <path>` on macOS) — probably not for v1, just ignore

**c. Increase debounce for cloud sync:**
- Cloud services can trigger multiple write events as they download/reconstruct a file
- Consider making debounce configurable or increasing it slightly (200ms → 500ms) when the notes dir is on a cloud-synced volume
- Add a config option: `sync_debounce_ms` (default 200, suggest 500 for cloud folders)

**d. Handle bulk file appearances:**
- On first sync (or after being offline), many files may appear at once
- The current watcher handles this fine (each file gets debounced individually), but ensure the DB upserts don't cause UI lag
- The existing `SyncNotes()` full-scan at startup already handles this case

### 4. `ListNotes` filtering (`internal/note/note.go`)

Update `ListNotes()` to skip conflict files and cloud sync artifacts:

```go
func ListNotes(dir string) ([]string, error) {
    // ... existing logic ...
    for _, e := range entries {
        if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
            if !IsCloudArtifact(e.Name()) {
                paths = append(paths, filepath.Join(dir, e.Name()))
            }
        }
    }
    return paths, nil
}
```

`IsCloudArtifact` filters out:
- `.filename.md.icloud` (iCloud placeholders)
- Files matching known temp patterns

Conflict files should **not** be filtered from `ListNotes` — they're real notes that need to be visible so the user can resolve them. But they should be **tagged** as conflicts in the UI.

### 5. Config guidance (documentation only)

No config changes needed for the sync feature itself. But document recommended setups:

**iCloud Drive (macOS + iPhone):**
```yaml
notes_dir: ~/Library/Mobile Documents/com~apple~CloudDocs/moss/notes
```

**Dropbox:**
```yaml
notes_dir: ~/Dropbox/moss/notes
```

**Syncthing:**
```yaml
notes_dir: ~/Syncthing/moss/notes
```

The user just changes `notes_dir` in `~/moss/config.yaml`. That's it.

### 6. `moss sync` command update (`cmd/moss/main.go`)

The existing `moss sync` command (re-index files to DB) stays as-is. Add conflict detection to its output:

```
$ moss sync
Synced 47 notes
⚠ 2 conflicts detected:
  meeting-notes.md ↔ meeting-notes 2.md (iCloud)
  project-plan.md ↔ project-plan (conflicted copy 2026-03-10).md (Dropbox)
```

---

## Implementation order

1. **Conflict detection** (`internal/sync/conflict.go`) — Pure functions, easy to test
2. **Watcher hardening** (`internal/sync/sync.go`) — Filter cloud artifacts, adjust debounce
3. **ListNotes filtering** (`internal/note/note.go`) — Skip cloud placeholders
4. **`moss sync` CLI output** (`cmd/moss/main.go`) — Surface conflicts in CLI
5. **TUI conflict indicator + resolution** (`internal/tui/`) — UI for viewing and resolving conflicts

Phases 1-3 are small, independent changes. Phase 4 is a quick CLI tweak. Phase 5 is the most work but can be iterated on.

---

## What's NOT in scope

- Building a sync transport/relay (user brings their own via iCloud, Dropbox, etc.)
- Syncing the database (local index, rebuilt from files)
- Syncing config.yaml (device-specific)
- Git integration (not needed for the core sync use case)
- Encryption (user's responsibility via their sync tool)
- Auto-downloading iCloud placeholders (v1 just ignores them)
- A Moss mobile app (use Paper, iA Writer, or any markdown editor on iPhone)
