package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// --- tokenizeLine tests ---

func TestTokenizeLine_H1(t *testing.T) {
	spans, toggles := tokenizeLine("# Hello World", false)
	if toggles {
		t.Error("H1 should not toggle fence")
	}
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans, got %d", len(spans))
	}
	if spans[0].kind != spanH1Marker {
		t.Errorf("span[0].kind = %d, want spanH1Marker (%d)", spans[0].kind, spanH1Marker)
	}
	if spans[0].text != "# " {
		t.Errorf("span[0].text = %q, want %q", spans[0].text, "# ")
	}
	if spans[0].rawStart != 0 || spans[0].rawEnd != 2 {
		t.Errorf("span[0] raw = [%d, %d), want [0, 2)", spans[0].rawStart, spans[0].rawEnd)
	}
	// Content span
	if spans[1].kind != spanH1Content {
		t.Errorf("span[1].kind = %d, want spanH1Content (%d)", spans[1].kind, spanH1Content)
	}
	if spans[1].text != "Hello World" {
		t.Errorf("span[1].text = %q, want %q", spans[1].text, "Hello World")
	}
}

func TestTokenizeLine_H2(t *testing.T) {
	spans, _ := tokenizeLine("## Sub", false)
	if spans[0].kind != spanH2Marker || spans[0].text != "## " {
		t.Errorf("span[0] = {%q, %d}, want {%q, spanH2Marker}", spans[0].text, spans[0].kind, "## ")
	}
	if spans[1].kind != spanH2Content {
		t.Errorf("span[1].kind = %d, want spanH2Content", spans[1].kind)
	}
}

func TestTokenizeLine_H3(t *testing.T) {
	spans, _ := tokenizeLine("### Third", false)
	if spans[0].kind != spanH3Marker {
		t.Errorf("span[0].kind = %d, want spanH3Marker", spans[0].kind)
	}
	if spans[1].kind != spanH3Content {
		t.Errorf("span[1].kind = %d, want spanH3Content", spans[1].kind)
	}
}

func TestTokenizeLine_H4(t *testing.T) {
	spans, _ := tokenizeLine("#### Fourth", false)
	if spans[0].kind != spanH4Marker {
		t.Errorf("span[0].kind = %d, want spanH4Marker", spans[0].kind)
	}
	if spans[1].kind != spanH4Content {
		t.Errorf("span[1].kind = %d, want spanH4Content", spans[1].kind)
	}
}

func TestTokenizeLine_H4NotMatchingH3(t *testing.T) {
	// "#### " should match H4, not H3
	spans, _ := tokenizeLine("#### Heading", false)
	if spans[0].kind != spanH4Marker {
		t.Errorf("#### should produce spanH4Marker, got %d", spans[0].kind)
	}
}

func TestTokenizeLine_Bold(t *testing.T) {
	spans, _ := tokenizeLine("hello **world** end", false)
	// Find bold span
	var found bool
	for _, s := range spans {
		if s.kind == spanBold && s.text == "world" {
			found = true
		}
	}
	if !found {
		t.Error("expected spanBold with text 'world'")
	}
}

func TestTokenizeLine_Italic(t *testing.T) {
	spans, _ := tokenizeLine("*italic*", false)
	var found bool
	for _, s := range spans {
		if s.kind == spanItalic && s.text == "italic" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected spanItalic, got spans: %v", spans)
	}
}

func TestTokenizeLine_BoldItalic(t *testing.T) {
	spans, _ := tokenizeLine("***bi***", false)
	var found bool
	for _, s := range spans {
		if s.kind == spanBoldItalic && s.text == "bi" {
			found = true
		}
	}
	if !found {
		t.Error("expected spanBoldItalic")
	}
}

func TestTokenizeLine_InlineCode(t *testing.T) {
	spans, _ := tokenizeLine("use `code` here", false)
	var found bool
	for _, s := range spans {
		if s.kind == spanCode && s.text == "code" {
			found = true
		}
	}
	if !found {
		t.Error("expected spanCode")
	}
}

func TestTokenizeLine_UnorderedListDash(t *testing.T) {
	spans, _ := tokenizeLine("- item", false)
	if spans[0].kind != spanBulletMarker {
		t.Errorf("span[0].kind = %d, want spanBulletMarker", spans[0].kind)
	}
	if spans[0].text != "• " {
		t.Errorf("bullet display text = %q, want %q", spans[0].text, "• ")
	}
	if spans[0].rawStart != 0 || spans[0].rawEnd != 2 {
		t.Errorf("bullet raw = [%d, %d), want [0, 2)", spans[0].rawStart, spans[0].rawEnd)
	}
	if spans[1].kind != spanBulletContent {
		t.Errorf("span[1].kind = %d, want spanBulletContent", spans[1].kind)
	}
}

func TestTokenizeLine_UnorderedListStar(t *testing.T) {
	spans, _ := tokenizeLine("* item", false)
	if spans[0].kind != spanBulletMarker {
		t.Errorf("span[0].kind = %d, want spanBulletMarker", spans[0].kind)
	}
}

func TestTokenizeLine_OrderedList(t *testing.T) {
	spans, _ := tokenizeLine("1. first", false)
	if spans[0].kind != spanOrderedMarker {
		t.Errorf("span[0].kind = %d, want spanOrderedMarker", spans[0].kind)
	}
	if spans[0].text != "1. " {
		t.Errorf("span[0].text = %q, want %q", spans[0].text, "1. ")
	}
	if spans[1].kind != spanOrderedContent {
		t.Errorf("span[1].kind = %d, want spanOrderedContent", spans[1].kind)
	}
}

func TestTokenizeLine_Blockquote(t *testing.T) {
	spans, _ := tokenizeLine("> quoted", false)
	if spans[0].kind != spanBlockquoteMarker {
		t.Errorf("span[0].kind = %d, want spanBlockquoteMarker", spans[0].kind)
	}
	if spans[0].text != "│ " {
		t.Errorf("blockquote display text = %q, want %q", spans[0].text, "│ ")
	}
	if spans[1].kind != spanBlockquoteContent {
		t.Errorf("span[1].kind = %d, want spanBlockquoteContent", spans[1].kind)
	}
}

func TestTokenizeLine_HorizontalRule(t *testing.T) {
	for _, line := range []string{"---", "***", "___", "----", "****"} {
		spans, toggles := tokenizeLine(line, false)
		if toggles {
			t.Errorf("HRule %q should not toggle fence", line)
		}
		if len(spans) != 1 || spans[0].kind != spanHRule {
			t.Errorf("HRule %q: expected 1 spanHRule span, got %v", line, spans)
		}
	}
}

func TestTokenizeLine_FenceOpen(t *testing.T) {
	spans, toggles := tokenizeLine("```go", false)
	if !toggles {
		t.Error("fence open should toggle fence state")
	}
	if len(spans) != 1 || spans[0].kind != spanFenceMarker {
		t.Errorf("expected spanFenceMarker, got %v", spans)
	}
}

func TestTokenizeLine_FenceClose(t *testing.T) {
	spans, toggles := tokenizeLine("```", true)
	if !toggles {
		t.Error("fence close should toggle fence state")
	}
	if len(spans) != 1 || spans[0].kind != spanFenceMarker {
		t.Errorf("expected spanFenceMarker, got %v", spans)
	}
}

func TestTokenizeLine_FenceInterior(t *testing.T) {
	spans, toggles := tokenizeLine("x := 42", true)
	if toggles {
		t.Error("fence interior should not toggle fence state")
	}
	if len(spans) != 1 || spans[0].kind != spanFenceContent {
		t.Errorf("expected spanFenceContent, got %v", spans)
	}
	if spans[0].text != "x := 42" {
		t.Errorf("fence content text = %q, want %q", spans[0].text, "x := 42")
	}
}

func TestTokenizeLine_InlineInsideHeading(t *testing.T) {
	spans, _ := tokenizeLine("# Hello **bold** world", false)
	// Should have: H1Marker, H1Content("Hello "), BoldMarker, Bold("bold"), BoldMarker, H1Content(" world")
	if spans[0].kind != spanH1Marker {
		t.Errorf("span[0] should be H1Marker")
	}
	var foundBold bool
	for _, s := range spans {
		if s.kind == spanBold {
			foundBold = true
		}
	}
	if !foundBold {
		t.Error("expected spanBold inside heading content")
	}
}

func TestTokenizeLine_PlainText(t *testing.T) {
	spans, _ := tokenizeLine("just plain text", false)
	if len(spans) != 1 || spans[0].kind != spanText {
		t.Errorf("plain text should produce single spanText, got %v", spans)
	}
	if spans[0].text != "just plain text" {
		t.Errorf("span text = %q, want %q", spans[0].text, "just plain text")
	}
}

// --- Cursor injection tests ---

func TestInjectCursor_MiddleOfSpan(t *testing.T) {
	sourceLine := "hello world"
	spans := parseInline(sourceLine, 0)
	result := injectCursor(spans, sourceLine, 5) // cursor on ' '

	var foundCursor bool
	for _, s := range result {
		if s.kind == spanCursor && s.text == " " {
			foundCursor = true
		}
	}
	if !foundCursor {
		t.Errorf("expected spanCursor with ' ', got %v", result)
	}
}

func TestInjectCursor_OnMarkerCharacter(t *testing.T) {
	sourceLine := "**bold**"
	spans, _ := tokenizeLine(sourceLine, false)
	result := injectCursor(spans, sourceLine, 0) // cursor on first '*'

	var foundCursor bool
	for _, s := range result {
		if s.kind == spanCursor {
			foundCursor = true
		}
	}
	if !foundCursor {
		t.Error("expected spanCursor when cursor is on marker")
	}
}

func TestInjectCursor_AtEOL(t *testing.T) {
	sourceLine := "hello"
	spans := parseInline(sourceLine, 0)
	result := injectCursor(spans, sourceLine, 5) // past end

	last := result[len(result)-1]
	if last.kind != spanCursorEOL {
		t.Errorf("last span should be spanCursorEOL, got %d", last.kind)
	}
	if last.text != " " {
		t.Errorf("EOL cursor text = %q, want %q", last.text, " ")
	}
}

func TestInjectCursor_EmptyLine(t *testing.T) {
	sourceLine := ""
	spans := parseInline(sourceLine, 0)
	result := injectCursor(spans, sourceLine, 0)

	// Empty line with cursor at 0 should produce EOL cursor
	var foundCursor bool
	for _, s := range result {
		if s.kind == spanCursorEOL || s.kind == spanCursor {
			foundCursor = true
		}
	}
	if !foundCursor {
		t.Error("expected cursor span on empty line")
	}
}

func TestInjectCursor_NotFocused(t *testing.T) {
	// When not focused, renderMarkdownBody should not call injectCursor
	// Test indirectly: renderMarkdownBody with focused=false should not add cursor spans
	output := renderMarkdownBody("hello", 0, 0, false, 20, 1, 0)
	// Should render "hello" without reversed cursor styling
	// We can't easily check for ANSI codes, but we can verify no extra spaces from cursor
	plain := strings.TrimSpace(stripANSI(output))
	if plain != "hello" {
		t.Errorf("unfocused render = %q, want %q", plain, "hello")
	}
}

// --- renderLine tests ---

func TestRenderLine_PadsToWidth(t *testing.T) {
	spans := []textSpan{{text: "hi", kind: spanText}}
	result := renderLine(spans, 10)
	width := lipgloss.Width(result)
	if width != 10 {
		t.Errorf("renderLine width = %d, want 10", width)
	}
}

func TestRenderLine_TruncatesToWidth(t *testing.T) {
	spans := []textSpan{{text: strings.Repeat("a", 50), kind: spanText}}
	result := renderLine(spans, 10)
	width := lipgloss.Width(result)
	if width > 10 {
		t.Errorf("renderLine width = %d, want <= 10", width)
	}
}

func TestRenderLine_ExactWidth(t *testing.T) {
	spans := []textSpan{{text: "1234567890", kind: spanText}}
	result := renderLine(spans, 10)
	width := lipgloss.Width(result)
	if width != 10 {
		t.Errorf("renderLine width = %d, want 10", width)
	}
}

// --- renderMarkdownBody tests ---

func TestRenderMarkdownBody_ScrollWindow(t *testing.T) {
	rawText := "line0\nline1\nline2\nline3\nline4"
	// topLine=2, height=2 — should show line2 and line3
	output := renderMarkdownBody(rawText, 0, 0, false, 20, 2, 2)
	lines := strings.Split(output, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(stripANSI(lines[0]), "line2") {
		t.Errorf("first visible line should contain 'line2', got %q", lines[0])
	}
	if !strings.Contains(stripANSI(lines[1]), "line3") {
		t.Errorf("second visible line should contain 'line3', got %q", lines[1])
	}
}

func TestRenderMarkdownBody_CursorInjectedInVisibleLine(t *testing.T) {
	rawText := "hello"
	// Cursor at line 0, col 0, topLine 0, focused
	output := renderMarkdownBody(rawText, 0, 0, true, 20, 1, 0)
	// The first character 'h' should have cursor styling (reversed)
	// We verify by checking the raw ANSI output contains reverse escape
	if !strings.Contains(output, "\x1b[") {
		t.Error("expected ANSI escape codes in focused render")
	}
}

func TestRenderMarkdownBody_EmptyBody(t *testing.T) {
	output := renderMarkdownBody("", 0, 0, false, 20, 3, 0)
	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		t.Errorf("empty body with height=3 should produce 3 lines, got %d", len(lines))
	}
}

func TestRenderMarkdownBody_FenceStateCarriedAcrossTopLine(t *testing.T) {
	rawText := "```go\ncode line\n```\nnormal"
	// topLine=1 — inside the fence block, should render as fence content
	output := renderMarkdownBody(rawText, 1, 0, false, 20, 1, 1)
	lines := strings.Split(output, "\n")
	// Line 1 is "code line" which should be inside the fence
	// renderSpan for spanFenceContent uses mdCodeBlockStyle
	// We can check that it renders without treating "code line" as markdown
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	plain := strings.TrimSpace(stripANSI(lines[0]))
	if plain != "code line" {
		t.Errorf("fence interior = %q, want %q", plain, "code line")
	}
}

func TestRenderMarkdownBody_HeightPaddedWithBlanks(t *testing.T) {
	rawText := "one"
	output := renderMarkdownBody(rawText, 0, 0, false, 20, 5, 0)
	lines := strings.Split(output, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestTokenizeLine_CheckboxOpen(t *testing.T) {
	spans, _ := tokenizeLine("- [ ] buy milk", false)
	if spans[0].kind != spanCheckboxOpen {
		t.Errorf("span[0].kind = %d, want spanCheckboxOpen (%d)", spans[0].kind, spanCheckboxOpen)
	}
	if spans[0].text != "- [ ] " {
		t.Errorf("checkbox open display text = %q, want %q", spans[0].text, "- [ ] ")
	}
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
	if spans[0].text != "- [x] " {
		t.Errorf("checkbox done display text = %q, want %q", spans[0].text, "- [x] ")
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
	spans, _ := tokenizeLine("- [] not a checkbox", false)
	if spans[0].kind == spanCheckboxOpen || spans[0].kind == spanCheckboxDone {
		t.Error("'- []' should not be treated as checkbox")
	}
}

func TestTokenizeLine_CheckboxContentInlineMarkdown(t *testing.T) {
	spans, _ := tokenizeLine("- [ ] buy **important** milk", false)
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

// stripANSI removes ANSI escape sequences for plain text comparison.
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if s[i] == 'm' || s[i] == 'K' {
				inEscape = false
			}
			continue
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

func TestInjectCursor_CheckboxMarker(t *testing.T) {
	// Cursor anywhere within the "- [ ] " prefix should produce a single
	// character cursor span (ASCII markers are not substituted).
	sourceLine := "- [ ] task"
	spans, _ := tokenizeLine(sourceLine, false)

	for col := 0; col <= 5; col++ {
		result := injectCursor(spans, sourceLine, col)
		var foundCursor bool
		for _, s := range result {
			if s.kind == spanCursor {
				foundCursor = true
			}
		}
		if !foundCursor {
			t.Errorf("col %d: expected spanCursor in result", col)
		}
	}
}
