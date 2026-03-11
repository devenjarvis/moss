package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devenjarvis/moss/internal/note"
)

// TestSummaryPipeline_Parsing tests the full parsing chain that summaries go through:
// 1. GenerateFrontmatter parses Haiku's raw output (simulated)
// 2. processTask re-serializes the map as "key: value" lines
// 3. queueFrontmatterTasks re-parses those lines and applies them to the note
// 4. WriteFrontmatter persists to disk
// 5. ParseFile reads it back
func TestSummaryPipeline_Parsing(t *testing.T) {
	tests := []struct {
		name           string
		haikuOutput    string // simulated raw output from Haiku
		wantSummary    string // expected summary after full round-trip
		wantInFile     bool   // should summary appear in file after WriteFrontmatter?
	}{
		{
			name:        "simple summary",
			haikuOutput: "summary: This note covers Go testing patterns and best practices.",
			wantSummary: "This note covers Go testing patterns and best practices.",
			wantInFile:  true,
		},
		{
			name:        "summary with colons",
			haikuOutput: "summary: Key finding: the system works. Details: see section 2.",
			wantSummary: "Key finding: the system works. Details: see section 2.",
			wantInFile:  true,
		},
		{
			name:        "summary with quotes from Haiku",
			haikuOutput: `summary: "This is a quoted summary from Haiku."`,
			wantSummary: `"This is a quoted summary from Haiku."`,
			wantInFile:  true,
		},
		{
			name: "full frontmatter response with summary",
			haikuOutput: `title: Meeting Notes
date: 2024-01-15
summary: Discussion of Q1 roadmap priorities. Team agreed on three key initiatives.
tags: [meetings, planning, roadmap]
status: active
source: written`,
			wantSummary: "Discussion of Q1 roadmap priorities. Team agreed on three key initiatives.",
			wantInFile:  true,
		},
		{
			name: "multi-line summary (YAML block scalar) - only first line survives",
			haikuOutput: `summary: This is the first sentence.
  This is a continuation line that will be lost.`,
			wantSummary: "This is the first sentence.",
			wantInFile:  true,
		},
		{
			name:        "empty summary value",
			haikuOutput: "summary: ",
			wantSummary: "",
			wantInFile:  false,
		},
		{
			name:        "no summary in output",
			haikuOutput: "title: Just a Title\ntags: [test]",
			wantSummary: "",
			wantInFile:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Simulate GenerateFrontmatter parsing
			// This replicates ai.go lines 80-93
			parsedFields := make(map[string]string)
			for _, line := range strings.Split(tt.haikuOutput, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					parsedFields[key] = val
				}
			}

			// Step 2: Simulate processTask re-serialization
			// This replicates ai.go lines 214-219
			var resultParts []string
			for k, v := range parsedFields {
				resultParts = append(resultParts, fmt.Sprintf("%s: %s", k, v))
			}
			resultOutput := strings.Join(resultParts, "\n")

			// Step 3: Simulate queueFrontmatterTasks re-parsing
			// This replicates main.go lines 172-211
			n := &note.Note{
				Title:  "Existing Title",
				Date:   "2024-01-01",
				Status: "active",
				Source: "written",
				Tags:   []string{"existing"},
				Body:   "Some note body content.",
			}

			for _, line := range strings.Split(resultOutput, "\n") {
				parts := strings.SplitN(line, ": ", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				if key == "summary" && n.Summary == "" {
					n.Summary = val
				}
			}

			if n.Summary != tt.wantSummary {
				t.Errorf("after parsing pipeline:\n  got summary:  %q\n  want summary: %q", n.Summary, tt.wantSummary)
			}

			// Step 4 & 5: WriteFrontmatter + ParseFile round-trip
			if tt.wantInFile {
				dir := t.TempDir()
				path := filepath.Join(dir, "test-note.md")
				n.FilePath = path

				if err := n.WriteFrontmatter(); err != nil {
					t.Fatalf("WriteFrontmatter error: %v", err)
				}

				// Read raw file to check summary is there
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("ReadFile error: %v", err)
				}
				if !strings.Contains(string(data), "summary") {
					t.Errorf("written file does not contain 'summary' field:\n%s", string(data))
				}

				// Parse it back
				parsed, err := note.ParseFile(path)
				if err != nil {
					t.Fatalf("ParseFile error: %v", err)
				}
				if parsed.Summary != tt.wantSummary {
					t.Errorf("after file round-trip:\n  got summary:  %q\n  want summary: %q", parsed.Summary, tt.wantSummary)
				}
			}
		})
	}
}

// TestSummaryPipeline_QuotedSummaryRoundTrip specifically tests whether
// Haiku-style quoted summaries survive the YAML marshal/unmarshal cycle.
func TestSummaryPipeline_QuotedSummaryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quoted-summary.md")

	n := &note.Note{
		Title:    "Test",
		Date:     "2024-01-01",
		Summary:  `"A quoted summary from Haiku"`,
		Status:   "active",
		Source:   "written",
		FilePath: path,
		Body:     "Body content.",
	}

	if err := n.WriteFrontmatter(); err != nil {
		t.Fatal(err)
	}

	parsed, err := note.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// YAML marshal/unmarshal may strip or transform quotes
	t.Logf("Original summary:    %q", n.Summary)
	t.Logf("Round-tripped summary: %q", parsed.Summary)

	if parsed.Summary == "" {
		t.Error("summary was lost during YAML round-trip")
	}
}

// TestSummaryPipeline_SplitDifference tests the subtle difference between
// the two parsing steps: GenerateFrontmatter splits on ":" while
// queueFrontmatterTasks splits on ": " (with space).
func TestSummaryPipeline_SplitDifference(t *testing.T) {
	// If processTask produces "summary:value" (no space), the second
	// parser won't find it because it splits on ": " (with space).
	// But processTask uses fmt.Sprintf("%s: %s", k, v), so this should
	// always have a space. Let's verify.

	key := "summary"
	val := "A test summary."
	serialized := fmt.Sprintf("%s: %s", key, val)

	// Parse with ": " split (as in queueFrontmatterTasks)
	parts := strings.SplitN(serialized, ": ", 2)
	if len(parts) != 2 {
		t.Fatalf("split on ': ' produced %d parts, want 2", len(parts))
	}
	if parts[0] != "summary" {
		t.Errorf("key = %q, want 'summary'", parts[0])
	}
	if parts[1] != "A test summary." {
		t.Errorf("value = %q, want 'A test summary.'", parts[1])
	}

	// Edge case: what if the value is empty after the colon?
	emptyVal := "summary: "
	parts = strings.SplitN(emptyVal, ": ", 2)
	if len(parts) != 2 {
		t.Fatalf("split on ': ' for empty value produced %d parts, want 2", len(parts))
	}
	trimmed := strings.TrimSpace(parts[1])
	if trimmed != "" {
		t.Errorf("trimmed empty value = %q, want empty string", trimmed)
	}
}
