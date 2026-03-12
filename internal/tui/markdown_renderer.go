package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

type spanKind int

const (
	spanText      spanKind = iota
	spanH1Marker           // "# " prefix — dimmed
	spanH1Content          // heading text — purple bold
	spanH2Marker           // "## " prefix
	spanH2Content          // cyan bold
	spanH3Marker
	spanH3Content // fg bold
	spanH4Marker
	spanH4Content        // bold
	spanBoldMarker       // "**" — dimmed
	spanBold             // bold text
	spanItalicMarker     // "*" or "_" — dimmed
	spanItalic           // italic text
	spanBoldItalicMarker // "***" — dimmed
	spanBoldItalic       // bold+italic text
	spanCodeMarker       // "`" — dimmed
	spanCode             // background highlight
	spanBulletMarker     // "• " replacing "- " — accent color
	spanBulletContent
	spanOrderedMarker // "1. " — accent color
	spanOrderedContent
	spanBlockquoteMarker    // "│ " replacing "> " — purple
	spanBlockquoteContent   // muted fg
	spanHRule               // whole line as styled rule
	spanFenceMarker         // ``` line — dimmed
	spanFenceContent        // lines inside fence — code bg
	spanCursor              // character under cursor — reversed
	spanCursorEOL           // phantom cursor at EOL — reversed space
	spanCheckboxOpen        // "☐ " replacing "- [ ] " — yellow
	spanCheckboxDone        // "☑ " replacing "- [x] " — green
	spanCheckboxOpenContent // task text — normal fg
	spanCheckboxDoneContent // task text — green + faint
)

type textSpan struct {
	text     string
	kind     spanKind
	rawStart int // byte offset in source line (for cursor hit-testing)
	rawEnd   int
}

var orderedListRe = regexp.MustCompile(`^[0-9]+\. `)

// renderMarkdownBody renders the body text with markdown styling, returning
// exactly height lines of width characters.
func renderMarkdownBody(rawText string, cursorLine, cursorCol int, focused bool, width, height, topLine int) string {
	lines := strings.Split(rawText, "\n")

	// Pre-scan lines before topLine to determine fence state at topLine
	inFence := false
	for i := 0; i < topLine && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
		}
	}

	rendered := make([]string, 0, height)
	for lineIdx := topLine; lineIdx < topLine+height; lineIdx++ {
		var line string
		if lineIdx < len(lines) {
			line = lines[lineIdx]
		}

		spans, togglesFence := tokenizeLine(line, inFence)
		if togglesFence {
			inFence = !inFence
		}

		// Replace HRule text with full-width rule now that we have width
		if len(spans) == 1 && spans[0].kind == spanHRule {
			spans[0].text = strings.Repeat("─", width)
		}

		if focused && lineIdx == cursorLine {
			if lineIdx < len(lines) {
				spans = injectCursor(spans, line, cursorCol)
			} else {
				spans = append(spans, textSpan{text: " ", kind: spanCursorEOL})
			}
		}

		rendered = append(rendered, renderLine(spans, width))
	}

	return strings.Join(rendered, "\n")
}

// tokenizeLine tokenizes one source line into styled spans.
// It returns the spans and whether this line toggles fence state.
func tokenizeLine(line string, inFence bool) (spans []textSpan, togglesFence bool) {
	trimmed := strings.TrimSpace(line)

	// Fenced code delimiter
	if strings.HasPrefix(trimmed, "```") {
		return []textSpan{{text: line, kind: spanFenceMarker, rawStart: 0, rawEnd: len(line)}}, true
	}

	// Inside fence — no inline parsing
	if inFence {
		return []textSpan{{text: line, kind: spanFenceContent, rawStart: 0, rawEnd: len(line)}}, false
	}

	// Horizontal rule: trimmed line is all -, *, or _ with 3+ chars
	if isHorizontalRule(trimmed) {
		// text will be replaced in renderMarkdownBody with full-width rule
		return []textSpan{{text: trimmed, kind: spanHRule, rawStart: 0, rawEnd: len(line)}}, false
	}

	// Headings — check longest prefix first to avoid ## matching as #
	if strings.HasPrefix(line, "#### ") {
		return tokenizeHeading(line, 5, spanH4Marker, spanH4Content), false
	}
	if strings.HasPrefix(line, "### ") {
		return tokenizeHeading(line, 4, spanH3Marker, spanH3Content), false
	}
	if strings.HasPrefix(line, "## ") {
		return tokenizeHeading(line, 3, spanH2Marker, spanH2Content), false
	}
	if strings.HasPrefix(line, "# ") {
		return tokenizeHeading(line, 2, spanH1Marker, spanH1Content), false
	}

	// Blockquote
	if strings.HasPrefix(line, "> ") {
		marker := textSpan{text: "│ ", kind: spanBlockquoteMarker, rawStart: 0, rawEnd: 2}
		content := parseInlineWithDefaultKind(line[2:], 2, spanBlockquoteContent)
		return append([]textSpan{marker}, content...), false
	}

	// Unordered and ordered lists — support leading indentation
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	stripped := line[indent:]

	// Unordered list (including checkboxes)
	if strings.HasPrefix(stripped, "- ") || strings.HasPrefix(stripped, "* ") {
		rest := stripped[2:]
		var spans []textSpan
		if indent > 0 {
			spans = append(spans, textSpan{text: line[:indent], kind: spanText, rawStart: 0, rawEnd: indent})
		}
		if strings.HasPrefix(rest, "[ ] ") {
			marker := textSpan{text: stripped[:6], kind: spanCheckboxOpen, rawStart: indent, rawEnd: indent + 6}
			content := parseInlineWithDefaultKind(rest[4:], indent+6, spanCheckboxOpenContent)
			return append(append(spans, marker), content...), false
		}
		if strings.HasPrefix(rest, "[x] ") || strings.HasPrefix(rest, "[X] ") {
			marker := textSpan{text: stripped[:6], kind: spanCheckboxDone, rawStart: indent, rawEnd: indent + 6}
			content := parseInlineWithDefaultKind(rest[4:], indent+6, spanCheckboxDoneContent)
			return append(append(spans, marker), content...), false
		}
		marker := textSpan{text: "• ", kind: spanBulletMarker, rawStart: indent, rawEnd: indent + 2}
		content := parseInlineWithDefaultKind(stripped[2:], indent+2, spanBulletContent)
		return append(append(spans, marker), content...), false
	}

	// Ordered list
	if loc := orderedListRe.FindStringIndex(stripped); loc != nil {
		prefixLen := loc[1]
		var spans []textSpan
		if indent > 0 {
			spans = append(spans, textSpan{text: line[:indent], kind: spanText, rawStart: 0, rawEnd: indent})
		}
		marker := textSpan{text: stripped[:prefixLen], kind: spanOrderedMarker, rawStart: indent, rawEnd: indent + prefixLen}
		content := parseInlineWithDefaultKind(stripped[prefixLen:], indent+prefixLen, spanOrderedContent)
		return append(append(spans, marker), content...), false
	}

	// Plain text
	return parseInline(line, 0), false
}

func tokenizeHeading(line string, prefixLen int, markerKind, contentKind spanKind) []textSpan {
	marker := textSpan{
		text:     line[:prefixLen],
		kind:     markerKind,
		rawStart: 0,
		rawEnd:   prefixLen,
	}
	content := parseInlineWithDefaultKind(line[prefixLen:], prefixLen, contentKind)
	return append([]textSpan{marker}, content...)
}

// parseInlineWithDefaultKind parses inline markdown, using defaultKind instead of spanText.
func parseInlineWithDefaultKind(raw string, rawOffset int, defaultKind spanKind) []textSpan {
	spans := parseInline(raw, rawOffset)
	for i := range spans {
		if spans[i].kind == spanText {
			spans[i].kind = defaultKind
		}
	}
	return spans
}

// parseInline parses inline markdown (bold, italic, code) within a substring.
// rawOffset is the byte offset of raw within the full source line.
func parseInline(raw string, rawOffset int) []textSpan {
	var spans []textSpan
	plainByteStart := 0
	i := 0

	flushPlain := func() {
		if i > plainByteStart {
			spans = append(spans, textSpan{
				text:     raw[plainByteStart:i],
				kind:     spanText,
				rawStart: rawOffset + plainByteStart,
				rawEnd:   rawOffset + i,
			})
		}
	}

	for i < len(raw) {
		// *** bold-italic (must check before **)
		if strings.HasPrefix(raw[i:], "***") {
			closeIdx := strings.Index(raw[i+3:], "***")
			if closeIdx >= 0 {
				flushPlain()
				openEnd := i + 3
				closeStart := openEnd + closeIdx
				closeEnd := closeStart + 3
				spans = append(spans,
					textSpan{text: "***", kind: spanBoldItalicMarker, rawStart: rawOffset + i, rawEnd: rawOffset + openEnd},
					textSpan{text: raw[openEnd:closeStart], kind: spanBoldItalic, rawStart: rawOffset + openEnd, rawEnd: rawOffset + closeStart},
					textSpan{text: "***", kind: spanBoldItalicMarker, rawStart: rawOffset + closeStart, rawEnd: rawOffset + closeEnd},
				)
				i = closeEnd
				plainByteStart = i
				continue
			}
		}

		// ** bold (must check before single *)
		if strings.HasPrefix(raw[i:], "**") {
			closeIdx := strings.Index(raw[i+2:], "**")
			if closeIdx >= 0 {
				flushPlain()
				openEnd := i + 2
				closeStart := openEnd + closeIdx
				closeEnd := closeStart + 2
				spans = append(spans,
					textSpan{text: "**", kind: spanBoldMarker, rawStart: rawOffset + i, rawEnd: rawOffset + openEnd},
					textSpan{text: raw[openEnd:closeStart], kind: spanBold, rawStart: rawOffset + openEnd, rawEnd: rawOffset + closeStart},
					textSpan{text: "**", kind: spanBoldMarker, rawStart: rawOffset + closeStart, rawEnd: rawOffset + closeEnd},
				)
				i = closeEnd
				plainByteStart = i
				continue
			}
		}

		// * italic (single, not part of **)
		if raw[i] == '*' && (i+1 >= len(raw) || raw[i+1] != '*') {
			closeIdx := strings.Index(raw[i+1:], "*")
			if closeIdx >= 0 {
				closePos := i + 1 + closeIdx
				// Ensure closing * is not part of **
				if closePos+1 >= len(raw) || raw[closePos+1] != '*' {
					flushPlain()
					openEnd := i + 1
					closeEnd := closePos + 1
					spans = append(spans,
						textSpan{text: "*", kind: spanItalicMarker, rawStart: rawOffset + i, rawEnd: rawOffset + openEnd},
						textSpan{text: raw[openEnd:closePos], kind: spanItalic, rawStart: rawOffset + openEnd, rawEnd: rawOffset + closePos},
						textSpan{text: "*", kind: spanItalicMarker, rawStart: rawOffset + closePos, rawEnd: rawOffset + closeEnd},
					)
					i = closeEnd
					plainByteStart = i
					continue
				}
			}
		}

		// _ italic
		if raw[i] == '_' {
			closeIdx := strings.Index(raw[i+1:], "_")
			if closeIdx >= 0 {
				flushPlain()
				openEnd := i + 1
				closeStart := openEnd + closeIdx
				closeEnd := closeStart + 1
				spans = append(spans,
					textSpan{text: "_", kind: spanItalicMarker, rawStart: rawOffset + i, rawEnd: rawOffset + openEnd},
					textSpan{text: raw[openEnd:closeStart], kind: spanItalic, rawStart: rawOffset + openEnd, rawEnd: rawOffset + closeStart},
					textSpan{text: "_", kind: spanItalicMarker, rawStart: rawOffset + closeStart, rawEnd: rawOffset + closeEnd},
				)
				i = closeEnd
				plainByteStart = i
				continue
			}
		}

		// ` inline code
		if raw[i] == '`' {
			closeIdx := strings.Index(raw[i+1:], "`")
			if closeIdx >= 0 {
				flushPlain()
				openEnd := i + 1
				closeStart := openEnd + closeIdx
				closeEnd := closeStart + 1
				spans = append(spans,
					textSpan{text: "`", kind: spanCodeMarker, rawStart: rawOffset + i, rawEnd: rawOffset + openEnd},
					textSpan{text: raw[openEnd:closeStart], kind: spanCode, rawStart: rawOffset + openEnd, rawEnd: rawOffset + closeStart},
					textSpan{text: "`", kind: spanCodeMarker, rawStart: rawOffset + closeStart, rawEnd: rawOffset + closeEnd},
				)
				i = closeEnd
				plainByteStart = i
				continue
			}
		}

		// Advance one rune
		_, size := utf8.DecodeRuneInString(raw[i:])
		i += size
	}

	// Flush remaining plain text
	if plainByteStart < len(raw) {
		spans = append(spans, textSpan{
			text:     raw[plainByteStart:],
			kind:     spanText,
			rawStart: rawOffset + plainByteStart,
			rawEnd:   rawOffset + len(raw),
		})
	}

	return spans
}

// injectCursor injects cursor styling into spans at the cursor position.
// cursorCol is a rune index into sourceLine.
func injectCursor(spans []textSpan, sourceLine string, cursorCol int) []textSpan {
	runeCount := utf8.RuneCountInString(sourceLine)

	// Cursor at or past EOL — add phantom cursor
	if cursorCol >= runeCount {
		result := make([]textSpan, len(spans)+1)
		copy(result, spans)
		result[len(spans)] = textSpan{text: " ", kind: spanCursorEOL}
		return result
	}

	byteOffset := runeIndexToByteOffset(sourceLine, cursorCol)

	var result []textSpan
	injected := false

	for _, span := range spans {
		if injected || byteOffset < span.rawStart || byteOffset >= span.rawEnd {
			result = append(result, span)
			continue
		}

		injected = true

		// Substituted markers have display text that differs from source
		// (bullet "• " replacing "- ", blockquote "│ " replacing "> ")
		isSubstituted := span.kind == spanBulletMarker || span.kind == spanBlockquoteMarker
		if isSubstituted {
			// Apply cursor styling to the whole substituted span
			result = append(result, textSpan{text: span.text, kind: spanCursor, rawStart: span.rawStart, rawEnd: span.rawEnd})
			continue
		}

		// Precise cursor injection — span text matches source bytes
		spanByteOffset := byteOffset - span.rawStart
		cursorRune, cursorSize := utf8.DecodeRuneInString(span.text[spanByteOffset:])

		if spanByteOffset > 0 {
			result = append(result, textSpan{
				text:     span.text[:spanByteOffset],
				kind:     span.kind,
				rawStart: span.rawStart,
				rawEnd:   span.rawStart + spanByteOffset,
			})
		}
		result = append(result, textSpan{
			text:     string(cursorRune),
			kind:     spanCursor,
			rawStart: span.rawStart + spanByteOffset,
			rawEnd:   span.rawStart + spanByteOffset + cursorSize,
		})
		afterStart := spanByteOffset + cursorSize
		if afterStart < len(span.text) {
			result = append(result, textSpan{
				text:     span.text[afterStart:],
				kind:     span.kind,
				rawStart: span.rawStart + afterStart,
				rawEnd:   span.rawEnd,
			})
		}
	}

	if !injected {
		// Cursor past end of all spans — add EOL cursor
		result = append(result, textSpan{text: " ", kind: spanCursorEOL})
	}

	return result
}

// renderSpan maps a spanKind to a lipgloss-styled string.
func renderSpan(span textSpan) string {
	switch span.kind {
	case spanH1Marker, spanH2Marker, spanH3Marker, spanH4Marker,
		spanBoldMarker, spanItalicMarker, spanBoldItalicMarker, spanCodeMarker,
		spanFenceMarker:
		return mdMarkerStyle.Render(span.text)
	case spanH1Content:
		return mdH1Style.Render(span.text)
	case spanH2Content:
		return mdH2Style.Render(span.text)
	case spanH3Content:
		return mdH3Style.Render(span.text)
	case spanH4Content:
		return mdH4Style.Render(span.text)
	case spanBold:
		return mdBoldStyle.Render(span.text)
	case spanItalic:
		return mdItalicStyle.Render(span.text)
	case spanBoldItalic:
		return mdBoldItalicStyle.Render(span.text)
	case spanCode:
		return mdCodeStyle.Render(span.text)
	case spanFenceContent:
		return mdCodeBlockStyle.Render(span.text)
	case spanBulletMarker:
		return mdBulletStyle.Render(span.text)
	case spanCheckboxOpen:
		return mdCheckboxOpenStyle.Render(span.text)
	case spanCheckboxDone:
		return mdCheckboxDoneStyle.Render(span.text)
	case spanCheckboxOpenContent:
		return mdCheckboxOpenContentStyle.Render(span.text)
	case spanCheckboxDoneContent:
		return mdCheckboxDoneContentStyle.Render(span.text)
	case spanOrderedMarker:
		return mdOrderedStyle.Render(span.text)
	case spanBlockquoteMarker:
		return mdBlockquoteStyle.Render(span.text)
	case spanBlockquoteContent:
		return lipgloss.NewStyle().Foreground(colorMuted).Render(span.text)
	case spanHRule:
		return mdHRuleStyle.Render(span.text)
	case spanCursor, spanCursorEOL:
		return mdCursorStyle.Render(span.text)
	default:
		return span.text
	}
}

// renderLine concatenates spans and pads/truncates to exact width.
func renderLine(spans []textSpan, width int) string {
	var sb strings.Builder
	for _, span := range spans {
		sb.WriteString(renderSpan(span))
	}
	assembled := sb.String()

	visWidth := lipgloss.Width(assembled)
	if visWidth < width {
		assembled += strings.Repeat(" ", width-visWidth)
	} else if visWidth > width {
		assembled = lipgloss.NewStyle().MaxWidth(width).Render(assembled)
	}
	return assembled
}

// isHorizontalRule returns true if trimmed is 3+ of the same char (-, *, _).
func isHorizontalRule(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	first := rune(trimmed[0])
	if first != '-' && first != '*' && first != '_' {
		return false
	}
	for _, c := range trimmed {
		if c != first {
			return false
		}
	}
	return true
}

// renderMarkdownPreview renders markdown text with syntax highlighting for
// read-only display (no cursor). Long lines are word-wrapped to fit width.
func renderMarkdownPreview(rawText string, width int) string {
	lines := strings.Split(rawText, "\n")

	inFence := false
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		// Wrap long lines before tokenizing (skip fenced code blocks)
		trimmed := strings.TrimSpace(line)
		isFenceDelim := strings.HasPrefix(trimmed, "```")
		wrappedLines := []string{line}
		if !inFence && !isFenceDelim && width > 0 {
			wrappedLines = wrapLine(line, width)
		}

		for _, wl := range wrappedLines {
			spans, togglesFence := tokenizeLine(wl, inFence)
			if togglesFence {
				inFence = !inFence
			}

			if len(spans) == 1 && spans[0].kind == spanHRule {
				spans[0].text = strings.Repeat("─", width)
			}

			rendered = append(rendered, renderLine(spans, width))
		}
	}

	return strings.Join(rendered, "\n")
}

// wrapLine wraps a single line to fit within width, preserving markdown
// prefixes (blockquote, list markers, indentation) on continuation lines.
func wrapLine(line string, width int) []string {
	if width <= 0 || utf8.RuneCountInString(line) <= width {
		return []string{line}
	}

	// Determine the markdown prefix and content
	prefix, content := splitMarkdownPrefix(line)
	prefixWidth := utf8.RuneCountInString(prefix)

	// Build continuation prefix (spaces matching the prefix width)
	contPrefix := strings.Repeat(" ", prefixWidth)

	// Available width for content on each line
	contentWidth := width - prefixWidth
	if contentWidth < 10 {
		// Too narrow to wrap meaningfully, just return as-is
		return []string{line}
	}

	// Word-wrap the content
	words := strings.Fields(content)
	if len(words) == 0 {
		return []string{line}
	}

	var result []string
	currentLine := prefix
	currentLen := prefixWidth

	for _, word := range words {
		wordLen := utf8.RuneCountInString(word)
		if currentLen == prefixWidth {
			// First word on the line
			currentLine += word
			currentLen += wordLen
		} else if currentLen+1+wordLen <= width {
			// Fits on current line
			currentLine += " " + word
			currentLen += 1 + wordLen
		} else {
			// Start a new line
			result = append(result, currentLine)
			currentLine = contPrefix + word
			currentLen = prefixWidth + wordLen
		}
	}
	if currentLine != "" {
		result = append(result, currentLine)
	}

	if len(result) == 0 {
		return []string{line}
	}
	return result
}

// splitMarkdownPrefix extracts the leading markdown prefix (blockquote,
// list marker, heading, indentation) and returns (prefix, rest).
func splitMarkdownPrefix(line string) (string, string) {
	// Blockquote
	if strings.HasPrefix(line, "> ") {
		return "> ", line[2:]
	}

	// Heading — don't wrap headings with prefix, just use indentation
	for _, h := range []string{"#### ", "### ", "## ", "# "} {
		if strings.HasPrefix(line, h) {
			return h, line[len(h):]
		}
	}

	// Indented content
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	stripped := line[indent:]

	// Checkbox lists
	if strings.HasPrefix(stripped, "- [ ] ") || strings.HasPrefix(stripped, "- [x] ") || strings.HasPrefix(stripped, "- [X] ") {
		pfx := line[:indent+6]
		return pfx, stripped[6:]
	}

	// Unordered list
	if strings.HasPrefix(stripped, "- ") || strings.HasPrefix(stripped, "* ") {
		pfx := line[:indent+2]
		return pfx, stripped[2:]
	}

	// Ordered list
	if loc := orderedListRe.FindStringIndex(stripped); loc != nil {
		pfxLen := indent + loc[1]
		return line[:pfxLen], line[pfxLen:]
	}

	// Plain text — use leading whitespace as prefix
	if indent > 0 {
		return line[:indent], stripped
	}

	return "", line
}

// wrapText wraps plain text content to fit within width. The first line
// starts after a label of labelWidth characters, continuation lines are
// indented by that amount. Newlines in the input are preserved.
func wrapText(text string, width, labelWidth int) string {
	if width <= labelWidth {
		return text
	}

	paragraphs := strings.Split(text, "\n")
	var result []string

	for pi, para := range paragraphs {
		if para == "" {
			result = append(result, "")
			continue
		}

		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		indent := strings.Repeat(" ", labelWidth)
		// First paragraph's first line has less space (label already printed)
		lineWidth := width
		if pi == 0 {
			lineWidth = width - labelWidth
		}

		var lines []string
		currentLine := ""
		currentLen := 0

		for _, word := range words {
			wordLen := utf8.RuneCountInString(word)
			if currentLen == 0 {
				currentLine = word
				currentLen = wordLen
			} else if currentLen+1+wordLen <= lineWidth {
				currentLine += " " + word
				currentLen += 1 + wordLen
			} else {
				lines = append(lines, currentLine)
				currentLine = word
				currentLen = wordLen
				lineWidth = width - labelWidth // subsequent lines have full indent
			}
		}
		if currentLine != "" {
			lines = append(lines, currentLine)
		}

		for i, l := range lines {
			if pi == 0 && i == 0 {
				result = append(result, l) // first line, label already printed
			} else {
				result = append(result, indent+l)
			}
		}
	}

	return strings.Join(result, "\n")
}

// runeIndexToByteOffset converts a rune index to a byte offset in s.
func runeIndexToByteOffset(s string, runeIdx int) int {
	byteOffset := 0
	for i := 0; i < runeIdx && byteOffset < len(s); i++ {
		_, size := utf8.DecodeRuneInString(s[byteOffset:])
		byteOffset += size
	}
	return byteOffset
}
