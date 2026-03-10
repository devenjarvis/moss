# Design: Checkbox Styling in the Note Editor

**Date:** 2026-03-09
**Status:** Approved

## Goal

Make todos a first-class citizen in moss by styling GFM-style checkboxes in the inline markdown renderer. Checked and unchecked items should be visually distinct while editing, without reducing readability.

## Behavior

| Source text     | Displayed as | Checkbox color  | Content color       |
|-----------------|--------------|-----------------|---------------------|
| `- [ ] task`    | `☐ task`     | yellow (warning) | normal (colorFg)    |
| `- [x] task`    | `☑ task`     | green (accent)   | green + faint       |
| `- [X] task`    | `☑ task`     | green (accent)   | green + faint       |
| `* [ ] task`    | `☐ task`     | yellow (warning) | normal (colorFg)    |

No strikethrough on checked content (keeps text readable while editing).

## Architecture

### New span kinds (`markdown_renderer.go`)

```
spanCheckboxOpen        // "☐ " replacing "- [ ] " — yellow
spanCheckboxDone        // "☑ " replacing "- [x] " — green
spanCheckboxOpenContent // task text — normal fg
spanCheckboxDoneContent // task text — green + faint
```

### Detection (`tokenizeLine`)

After identifying a bullet prefix (`- ` or `* `), check if the remainder starts with `[ ] ` or `[xX] ` before falling through to the regular bullet path. The entire `- [ ] ` (6 bytes) becomes the marker span; the text after becomes the content span.

### New styles (`styles.go`)

```go
mdCheckboxOpenStyle        = lipgloss.NewStyle().Foreground(colorWarning)
mdCheckboxDoneStyle        = lipgloss.NewStyle().Foreground(colorAccent)
mdCheckboxOpenContentStyle = lipgloss.NewStyle().Foreground(colorFg)
mdCheckboxDoneContentStyle = lipgloss.NewStyle().Foreground(colorAccent).Faint(true)
```

## Files Changed

- `internal/tui/markdown_renderer.go` — add span kinds, detection logic, render cases
- `internal/tui/styles.go` — add four new style vars
- `internal/tui/markdown_renderer_test.go` — add tokenizer + render tests for checkboxes
