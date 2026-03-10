# Checkbox Styling Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Style GFM-style checkboxes (`- [ ]` / `- [x]`) in the inline markdown renderer so todos are visually distinct while editing.

**Architecture:** Two new marker span kinds (`spanCheckboxOpen`, `spanCheckboxDone`) and two content span kinds (`spanCheckboxOpenContent`, `spanCheckboxDoneContent`) are added to the existing `textSpan` system in `markdown_renderer.go`. Detection is added in `tokenizeLine` before the regular bullet path: if a bullet line's content starts with `[ ] ` or `[xX] `, it routes to a checkbox span instead of a bullet span.

**Tech Stack:** Go, `charm.land/lipgloss/v2` for styling, existing `textSpan`/`spanKind` pattern in `internal/tui/markdown_renderer.go`.

---

### Task 1: Add styles

**Files:**
- Modify: `internal/tui/styles.go`

**Step 1: Add four new style vars** inside the existing `var (...)` block, after the `mdHRuleStyle` line:

```go
// Checkbox styles
mdCheckboxOpenStyle        = lipgloss.NewStyle().Foreground(colorWarning)
mdCheckboxDoneStyle        = lipgloss.NewStyle().Foreground(colorAccent)
mdCheckboxOpenContentStyle = lipgloss.NewStyle().Foreground(colorFg)
mdCheckboxDoneContentStyle = lipgloss.NewStyle().Foreground(colorAccent).Faint(true)
```

**Step 2: Verify it compiles**

```bash
go build ./internal/tui/
```

Expected: no output (success).

**Step 3: Commit**

```bash
git add internal/tui/styles.go
git commit -m "feat: add checkbox styles"
```

---

### Task 2: Add span kinds and tokenizer detection

**Files:**
- Modify: `internal/tui/markdown_renderer.go`
- Test: `internal/tui/markdown_renderer_test.go`

**Step 1: Write failing tests** — add to `markdown_renderer_test.go`:

```go
func TestTokenizeLine_CheckboxOpen(t *testing.T) {
	spans, _ := tokenizeLine("- [ ] buy milk", false)
	if spans[0].kind != spanCheckboxOpen {
		t.Errorf("span[0].kind = %d, want spanCheckboxOpen (%d)", spans[0].kind, spanCheckboxOpen)
	}
	if spans[0].text != "☐ " {
		t.Errorf("checkbox open display text = %q, want %q", spans[0].text, "☐ ")
	}
	// raw offsets cover the full "- [ ] " prefix (6 bytes)
	if spans[0].rawStart != 0 || spans[0].rawEnd != 6 {
		t.Errorf("checkbox open raw = [%d, %d), want [0, 6)", spans[0].rawStart, spans[0].rawEnd)
	}
	if spans[1].kind != spanCheckboxOpenContent {
		t.Errorf("span[1].kind = %d, want spanCheckboxOpenContent (%d)", spans[1].kind, spanCheckboxOpenContent)
	}
	if spans[1].text != "buy milk" {
		t.Errorf("span[1].text = %q, want %q", spans[1].text, "buy milk")
	}
}

func TestTokenizeLine_CheckboxDoneLowercase(t *testing.T) {
	spans, _ := tokenizeLine("- [x] buy milk", false)
	if spans[0].kind != spanCheckboxDone {
		t.Errorf("span[0].kind = %d, want spanCheckboxDone (%d)", spans[0].kind, spanCheckboxDone)
	}
	if spans[0].text != "☑ " {
		t.Errorf("checkbox done display text = %q, want %q", spans[0].text, "☑ ")
	}
	if spans[0].rawStart != 0 || spans[0].rawEnd != 6 {
		t.Errorf("checkbox done raw = [%d, %d), want [0, 6)", spans[0].rawStart, spans[0].rawEnd)
	}
	if spans[1].kind != spanCheckboxDoneContent {
		t.Errorf("span[1].kind = %d, want spanCheckboxDoneContent (%d)", spans[1].kind, spanCheckboxDoneContent)
	}
}

func TestTokenizeLine_CheckboxDoneUppercase(t *testing.T) {
	spans, _ := tokenizeLine("- [X] buy milk", false)
	if spans[0].kind != spanCheckboxDone {
		t.Errorf("span[0].kind = %d, want spanCheckboxDone", spans[0].kind)
	}
}

func TestTokenizeLine_CheckboxWithStarBullet(t *testing.T) {
	spans, _ := tokenizeLine("* [ ] task", false)
	if spans[0].kind != spanCheckboxOpen {
		t.Errorf("* bullet checkbox: span[0].kind = %d, want spanCheckboxOpen", spans[0].kind)
	}
}

func TestTokenizeLine_CheckboxNotMatchedWithoutSpace(t *testing.T) {
	// "- []" (no space after bracket) should NOT match checkbox — falls through to bullet
	spans, _ := tokenizeLine("- [] not a checkbox", false)
	if spans[0].kind == spanCheckboxOpen || spans[0].kind == spanCheckboxDone {
		t.Error("'- []' should not be treated as checkbox")
	}
}

func TestTokenizeLine_CheckboxContentInlineMarkdown(t *testing.T) {
	spans, _ := tokenizeLine("- [ ] buy **important** milk", false)
	// Should find a spanBold span in there
	var foundBold bool
	for _, s := range spans {
		if s.kind == spanBold {
			foundBold = true
		}
	}
	if !foundBold {
		t.Error("expected spanBold inside checkbox content")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/ -run TestTokenizeLine_Checkbox -v
```

Expected: FAIL with `spanCheckboxOpen` undefined.

**Step 3: Add span kinds** — in `markdown_renderer.go`, add after `spanCursorEOL` in the const block:

```go
spanCheckboxOpen        // "☐ " replacing "- [ ] " — yellow
spanCheckboxDone        // "☑ " replacing "- [x] " — green
spanCheckboxOpenContent // task text — normal fg
spanCheckboxDoneContent // task text — green + faint
```

**Step 4: Add detection in `tokenizeLine`** — replace the existing unordered list block:

```go
// OLD:
// Unordered list
if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
    marker := textSpan{text: "• ", kind: spanBulletMarker, rawStart: 0, rawEnd: 2}
    content := parseInlineWithDefaultKind(line[2:], 2, spanBulletContent)
    return append([]textSpan{marker}, content...), false
}
```

```go
// NEW:
// Unordered list (including checkboxes)
if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
    rest := line[2:]
    if strings.HasPrefix(rest, "[ ] ") {
        marker := textSpan{text: "☐ ", kind: spanCheckboxOpen, rawStart: 0, rawEnd: 6}
        content := parseInlineWithDefaultKind(rest[4:], 6, spanCheckboxOpenContent)
        return append([]textSpan{marker}, content...), false
    }
    if strings.HasPrefix(rest, "[x] ") || strings.HasPrefix(rest, "[X] ") {
        marker := textSpan{text: "☑ ", kind: spanCheckboxDone, rawStart: 0, rawEnd: 6}
        content := parseInlineWithDefaultKind(rest[4:], 6, spanCheckboxDoneContent)
        return append([]textSpan{marker}, content...), false
    }
    marker := textSpan{text: "• ", kind: spanBulletMarker, rawStart: 0, rawEnd: 2}
    content := parseInlineWithDefaultKind(line[2:], 2, spanBulletContent)
    return append([]textSpan{marker}, content...), false
}
```

Note on raw offsets: `"- [ ] "` is 6 ASCII bytes (indices 0–5), so `rawEnd: 6` and the content `rawOffset: 6` are correct. Same for `"- [x] "` and `"* [ ] "`.

**Step 5: Add render cases** — in `renderSpan`, add after the `spanBulletMarker` case:

```go
case spanCheckboxOpen:
    return mdCheckboxOpenStyle.Render(span.text)
case spanCheckboxDone:
    return mdCheckboxDoneStyle.Render(span.text)
case spanCheckboxOpenContent:
    return mdCheckboxOpenContentStyle.Render(span.text)
case spanCheckboxDoneContent:
    return mdCheckboxDoneContentStyle.Render(span.text)
```

**Step 6: Run tests**

```bash
go test ./internal/tui/ -run TestTokenizeLine_Checkbox -v
```

Expected: all PASS.

**Step 7: Run full suite**

```bash
go test ./...
```

Expected: all PASS.

**Step 8: Commit**

```bash
git add internal/tui/markdown_renderer.go internal/tui/markdown_renderer_test.go
git commit -m "feat: style checkboxes in markdown renderer"
```

---

### Task 3: Verify end-to-end

**Step 1: Build and smoke-test**

```bash
mage build && ./moss
```

Open or create a note, type the following in the body, and verify visual appearance:
- `- [ ] unchecked task` → shows `☐ unchecked task` with yellow `☐` and normal text
- `- [x] done task` → shows `☑ done task` with green `☑` and faint green text
- `- [X] also done` → same as `[x]`
- `* [ ] star bullet checkbox` → yellow `☐`
- `- regular bullet` → still shows `•` (unchanged)

**Step 2: Verify cursor works on checkbox lines**

Position cursor on different columns of a checkbox line and verify the cursor highlight appears correctly and doesn't break the display.

**Step 3: Final commit if any tweaks needed**

```bash
git add -p
git commit -m "fix: checkbox styling tweaks"
```

---

## Critical Files

- `internal/tui/styles.go` — four new style vars
- `internal/tui/markdown_renderer.go` — four new span kinds, detection in `tokenizeLine`, render cases in `renderSpan`
- `internal/tui/markdown_renderer_test.go` — six new tokenizer tests
