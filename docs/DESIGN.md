# moss Design System

This document is the **single source of truth** for how moss looks and how its
UI is built. It describes the design system as it *should be* — the target every
screen and component is expected to converge on. When a screen disagrees with
this document, the screen is wrong.

moss is a fast, fully local, keyboard-first terminal note app. The design system
exists to make the interface **calm, consistent, and legible** while staying out
of the writer's way.

---

## 1. Principles

1. **Calm, low chrome.** The notes are the content; the UI is scaffolding. Prefer
   muted neutrals, reserve color for meaning, and never decorate for decoration's
   sake.
2. **Meaning, not decoration.** Every color, glyph, and emphasis must carry
   information (focus, state, kind). If it doesn't mean something, drop it.
3. **One way to do a thing.** A pane is rendered one way. A list item is rendered
   one way. A status segment is rendered one way. There is no second
   implementation of the same UI element.
4. **Tokens over literals.** No raw hex, no ad-hoc `lipgloss.NewStyle()` at call
   sites, no magic spacing numbers in screen code. Everything flows from tokens.
5. **Terminal-native.** Respect the constraints of a TTY: a single monospace
   cell grid, limited color, no pixel positioning. Lean on bold / faint /
   reverse and box-drawing glyphs rather than fighting the medium.
6. **Keyboard-first.** Every action has a key. The UI advertises keys (status
   bar, help) using one consistent hint style.

---

## 2. Architecture

The UI is layered. Each layer may depend only on the layers above it. This
ordering is what keeps moss consistent: a change to a token ripples everywhere,
and there is nowhere to "sneak in" an off-system value.

```
┌─────────────────────────────────────────────────────────────┐
│ Tokens        theme.go      colors, spacing, borders, glyphs  │  ← raw values live here ONLY
├─────────────────────────────────────────────────────────────┤
│ Styles        styles.go     named lipgloss.Style values       │  ← compose tokens; no hex
├─────────────────────────────────────────────────────────────┤
│ Components    component.go   pane, list item, status bar, …   │  ← compose styles; reusable
├─────────────────────────────────────────────────────────────┤
│ Screens       model.go,      assemble components into a view   │  ← no styling logic
│               editor.go                                        │
└─────────────────────────────────────────────────────────────┘
```

Rules that enforce the layering:

- **Raw hex colors appear in exactly one place:** the raw palette in `theme.go`.
  Nothing else may call `lipgloss.Color("#…")`.
- **`lipgloss.NewStyle()` is only called in `styles.go`** (and inside
  `component.go` helpers when a style is genuinely component-local). Screen code
  (`model.go`, `editor.go`) and the markdown renderer reference named styles —
  never build styles inline.
- **Screens assemble components; they do not draw boxes.** Borders, titles,
  padding, gap-fill, and truncation belong to components, not to `View()`.
- **Layout magic numbers are named constants** in `theme.go` (border widths,
  pane ratios, the status bar height), not literals scattered through
  dimension math.

---

## 3. Color Tokens

moss ships a dark, Catppuccin-Mocha-adjacent palette. The **raw palette** is
private to `theme.go`. Everything else references the **semantic tokens** below,
which describe a *role*, not a hue — so the palette can be re-themed without
touching a single screen.

### Neutrals

| Token              | Hex       | Role |
|--------------------|-----------|------|
| `colorBg`          | `#1E1E2E` | App / editor background, code-block background |
| `colorSurface`     | `#313244` | Raised surface — inline code, chips |
| `colorText`        | `#CDD6F4` | Default foreground text |
| `colorMuted`       | `#6B7280` | Secondary text, chrome, inactive borders, hints |
| `colorInverse`     | `#1E1E2E` | Text drawn on a colored background |

### Brand & accents

| Token              | Hex       | Role |
|--------------------|-----------|------|
| `colorPrimary`     | `#7C3AED` | Brand purple — focus, H1, blockquote, active border |
| `colorSecondary`   | `#06B6D4` | Cyan — metadata chips, H2, field labels |

### Status

| Token              | Hex       | Role |
|--------------------|-----------|------|
| `colorSuccess`     | `#A6E3A1` | Green — done / saved / positive, list markers |
| `colorWarning`     | `#F9E2AF` | Yellow — pending / dirty / attention, search match |

### Usage rules

- **Focus is always `colorPrimary`.** A focused pane border, the selected list
  item, and the H1 heading all share the brand purple so "where am I" reads
  instantly.
- **Metadata is always `colorSecondary`.** Tag filters, project chips, the
  update notice, and field labels are cyan.
- **State maps to status colors:** dirty/pending → `colorWarning`,
  saved/done/positive → `colorSuccess`.
- **Muted is the default for chrome:** inactive borders, hints, separators,
  timestamps, and the markdown markers (`**`, `#`, `` ` ``) are all `colorMuted`.
- Never use a raw hue name ("the green one") in conversation or code — use the
  role token.

---

## 4. Typography & Emphasis

The terminal has one font. "Typography" here means how we use the four levers a
TTY gives us: **bold**, **faint**, **reverse**, and **color**.

| Intent            | Treatment |
|-------------------|-----------|
| Heading / title   | Bold + color (H1 primary, H2 secondary, H3 text, H4 plain bold) |
| Emphasis (strong) | Bold |
| Emphasis (em)     | Italic |
| De-emphasis       | Faint (markdown markers, completed-todo text) |
| Selection/cursor  | Reverse |
| Inline code       | `colorWarning` on `colorSurface` |

Rules:

- One emphasis lever per role. Don't stack bold+italic+color+reverse to mean a
  single thing.
- Markdown *markers* (the `#`, `**`, `` ` ``, `>` characters) are always faint +
  muted so the rendered content stands out from its syntax.

---

## 5. Spacing & Layout

Spacing is uniform and small — this is a dense, information-first TUI.

| Token               | Value | Use |
|---------------------|-------|-----|
| `spacePanePadX`     | `1`   | Horizontal padding inside every pane and the status bar |
| `paneBorderWidth`   | `2`   | Border cells consumed on each axis by a pane border |
| `paneTitleHeight`   | `1`   | Rows a pane title occupies |
| `statusBarHeight`   | `1`   | Rows the status bar occupies |
| `listWidthRatio`    | `0.22`| List pane width as a fraction of terminal width |

Layout model:

- The screen is a **vertical stack**: `body` above, a one-line **status bar**
  below.
- In normal mode the body is a **horizontal split**: a narrow **list pane**
  (`listWidthRatio` of width) beside a wide **preview pane**.
- In edit mode the body is a **single full-width editor pane**; the list is
  hidden.
- A pane's usable content height is `height − paneBorderWidth − paneTitleHeight`.
  Components compute this; screens must not re-derive it with raw arithmetic.

---

## 6. Borders & Elevation

- **Pane border:** `lipgloss.RoundedBorder()`.
- **Inactive border:** `colorMuted`. **Focused border:** `colorPrimary`.
- There is exactly one elevation cue: a focused pane's purple border. moss does
  not use shadows, double borders, or nested boxes. (In particular, a modal is a
  single rounded box — never a box drawn inside another box.)

---

## 7. Iconography (Glyphs)

Glyphs are tokens too. Defined once in `theme.go`, referenced everywhere.

| Glyph | Token              | Meaning |
|-------|--------------------|---------|
| `▸ `  | `glyphCursor`      | Selected list row (prefix) |
| `  `  | `glyphNoCursor`    | Unselected list row (prefix, keeps alignment) |
| `•`   | `glyphBullet`      | Unordered list marker (rendered) |
| `☐`   | `glyphCheckboxOpen`| Open task |
| `☑`   | `glyphCheckboxDone`| Completed task |
| `│ `  | `glyphBlockquote`  | Blockquote marker (rendered, replaces `> `) |
| `─`   | `glyphRule`        | Horizontal rule / separator fill |
| `●`   | `glyphDirty`       | Unsaved changes |
| `✓`   | `glyphSaved`       | Saved |
| ` │ ` | `glyphStatusSep`   | Separator between status-bar segments |

Rules:

- A meaning has one glyph. Don't represent "done" as both `[x]` and `☑` in
  different views — pick the token and use it.
- Glyphs that have width-2 implications (box drawing, check boxes) are chosen to
  render in a single cell in common terminals.

---

## 8. Component Catalog

Components are the only thing screens are allowed to compose. Each one owns its
borders, padding, truncation, and state styling.

### Pane (`pane`)
A bordered region with a title and a body, sized to exact `width × height`.
Inputs: `title`, `width`, `height`, `active`. The active flag drives the border
color. Both the list and preview/editor regions are panes — there is no other
way to draw a bordered region.

### List item (`renderListItem`)
A single selectable row. Inputs: a pre-formatted label and a `selected` flag.
Emits the `glyphCursor` / `glyphNoCursor` prefix and applies
`selectedItemStyle` / `normalItemStyle`. Both the notes list and the todos list
use it; neither re-implements cursor prefixing.

### Status bar (`statusBar`)
The bottom line. Inputs: `width`, ordered `left` segments, ordered `right`
segments. Joins each side with `glyphStatusSep`, gap-fills to `width`, and
applies the status background. All three modes (normal, todos, edit) build one
of these — the join/gap-fill logic exists once.

### Metadata chip / badge
A short `key:value` segment for the status bar (`tag:go`, `sort:title`,
`project:x`, `vN available`). Metadata chips are `colorSecondary`; the
de-emphasized sort chip is `colorMuted`. Rendered via the metadata styles, never
an inline style.

### Field label
The `title:` / `tags:` / `date:` labels in the editor and the prompt labels in
the list pane (`Title:`, `Tag:`). Labels are colored by role (editor frontmatter
= `colorSecondary`; new-note prompt = `colorSuccess`) and bold.

### Key hint
The advertised-shortcut text in the status bar and editor footer
(`? help · q quit`, `Tab: indent  …`). Always `helpStyle` (muted). One style for
every hint in the app.

### Modal / dialog (`modal`)
Center-placed overlay over the full screen: a single rounded box (primary
border, text foreground) holding content. The help overlay is a modal. A modal
never nests a second border inside itself.

### Markdown view
The renderer (`markdown_renderer.go`) turns markdown into styled spans using the
`md*` styles. It is the canonical renderer for both the read-only preview and
the live editor body; both share the token-driven span styles.

---

## 9. States

| State      | Cue |
|------------|-----|
| Focused    | `colorPrimary` border on the pane |
| Selected   | `glyphCursor` prefix + `selectedItemStyle` (primary, bold) |
| Dirty      | `glyphDirty` + `colorWarning` |
| Saved      | `glyphSaved` + `colorSuccess` |
| Empty      | Muted helper text ("No notes found. Press 'n' to create one.") |
| Disabled / inactive | `colorMuted` |

Empty states always tell the user the next action.

---

## 10. Anti-patterns (do not do these)

- ❌ `lipgloss.Color("#…")` outside the raw palette in `theme.go`.
- ❌ `lipgloss.NewStyle()…` at a screen call site. Add a named style instead.
- ❌ Re-deriving pane content height / border math inline. Use the component.
- ❌ A second way to render a list row, a status segment, or a bordered box.
- ❌ Color used decoratively (color that doesn't encode focus, kind, or state).
- ❌ A box drawn inside another box; stacked emphasis to mean one thing.

---

## 11. Adding to the system

1. Need a new color? Add a **semantic token** in `theme.go` mapped to a raw
   palette entry. Don't reuse a hue for an unrelated role.
2. Need a new styled text treatment? Add a **named style** in `styles.go`.
3. Building a new piece of UI used more than once? Add a **component** in
   `component.go` and document it in §8.
4. Update this document in the same change. The doc and the code move together.
