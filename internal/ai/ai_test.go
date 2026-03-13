package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/moss/internal/note"
)

func TestNewWorker(t *testing.T) {
	w := NewWorker(10)
	if w == nil {
		t.Fatal("NewWorker returned nil")
	}
	if cap(w.tasks) != 10 {
		t.Errorf("task channel capacity = %d, want 10", cap(w.tasks))
	}
}

func TestWorkerPendingCount(t *testing.T) {
	w := NewWorker(10)

	if got := w.PendingCount(); got != 0 {
		t.Errorf("PendingCount() = %d, want 0", got)
	}

	// Submit a task without starting the worker (so it stays pending)
	resultCh := make(chan Result, 1)
	w.Submit(Task{
		Type:     "ask",
		Note:     &note.Note{Body: "test"},
		Prompt:   "test question",
		ResultCh: resultCh,
	})

	if got := w.PendingCount(); got != 1 {
		t.Errorf("PendingCount() = %d, want 1", got)
	}
}

func TestWorkerSubmit_QueueFull(t *testing.T) {
	w := NewWorker(1)

	resultCh := make(chan Result, 5)

	// Fill the queue
	w.Submit(Task{Type: "ask", Note: &note.Note{Body: "1"}, Prompt: "q1", ResultCh: resultCh})

	// This should be dropped with an error
	w.Submit(Task{Type: "ask", Note: &note.Note{Body: "2"}, Prompt: "q2", ResultCh: resultCh})

	// Should get an error result for the dropped task
	select {
	case result := <-resultCh:
		if result.Err == nil {
			t.Error("expected error for dropped task")
		}
		if result.Err.Error() != "task queue full" {
			t.Errorf("error = %q, want 'task queue full'", result.Err)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for queue full error")
	}
}

func TestWorkerSubmit_NoResultChannel(t *testing.T) {
	w := NewWorker(1)

	// Submit with nil ResultCh when queue is full - should not panic
	w.Submit(Task{Type: "ask", Note: &note.Note{Body: "1"}, Prompt: "q1"})
	w.Submit(Task{Type: "ask", Note: &note.Note{Body: "2"}, Prompt: "q2"}) // dropped, no ResultCh
}

func TestWorkerStartAndStop(t *testing.T) {
	w := NewWorker(10)
	ctx := context.Background()

	w.Start(ctx)

	// Verify it's running by checking that Stop doesn't hang
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out - worker may be stuck")
	}
}

func TestWorkerContextCancellation(t *testing.T) {
	w := NewWorker(10)
	ctx, cancel := context.WithCancel(context.Background())

	w.Start(ctx)
	cancel()

	// Give goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// Worker should have stopped
	w.Stop()
}

func TestModelConstants(t *testing.T) {
	if ModelHaiku != "claude-haiku-4-5-20251001" {
		t.Errorf("ModelHaiku = %q, want %q", ModelHaiku, "claude-haiku-4-5-20251001")
	}
	if ModelSonnet != "" {
		t.Errorf("ModelSonnet = %q, want empty string (CLI default)", ModelSonnet)
	}
}

func TestTaskTypes(t *testing.T) {
	// Verify that processTask handles known task types without panicking
	// We can't test the actual AI calls without the claude CLI, but we can
	// verify the task routing logic by checking that unknown types
	// produce empty output.
	w := NewWorker(10)

	resultCh := make(chan Result, 1)
	task := Task{
		Type:     "unknown",
		Note:     &note.Note{Body: "test"},
		Prompt:   "test",
		ResultCh: resultCh,
	}

	// processTask is called directly - uses context.Background()
	// since unknown type won't call RunClaude
	ctx := context.Background()
	w.processTask(ctx, task)

	select {
	case result := <-resultCh:
		if result.Err != nil {
			t.Errorf("unknown task type should not error, got: %v", result.Err)
		}
		if result.Output != "" {
			t.Errorf("unknown task type should produce empty output, got: %q", result.Output)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for result")
	}
}

func TestEnhanceResultJSON(t *testing.T) {
	// Verify the EnhanceResult struct marshals/unmarshals correctly
	input := `{"corrected_body": "Hello world.", "thoughts": "Consider adding more detail."}`
	var result EnhanceResult
	if err := json.Unmarshal([]byte(input), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.CorrectedBody != "Hello world." {
		t.Errorf("CorrectedBody = %q, want %q", result.CorrectedBody, "Hello world.")
	}
	if result.Thoughts != "Consider adding more detail." {
		t.Errorf("Thoughts = %q, want %q", result.Thoughts, "Consider adding more detail.")
	}
}

func TestEnhanceResultJSON_Empty(t *testing.T) {
	input := `{"corrected_body": "", "thoughts": ""}`
	var result EnhanceResult
	if err := json.Unmarshal([]byte(input), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.CorrectedBody != "" {
		t.Errorf("CorrectedBody = %q, want empty", result.CorrectedBody)
	}
	if result.Thoughts != "" {
		t.Errorf("Thoughts = %q, want empty", result.Thoughts)
	}
}

func TestWorkerProcessTask_EnhanceType(t *testing.T) {
	// The enhance task type should be routed correctly in processTask.
	// Since we can't call the actual CLI, we just verify it doesn't panic
	// and returns an error (because claude CLI isn't available in tests).
	w := NewWorker(10)
	resultCh := make(chan Result, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	w.processTask(ctx, Task{
		Type:     "enhance",
		Stdin:    "test body",
		Prompt:   "test diff",
		Model:    ModelHaiku,
		ResultCh: resultCh,
	})

	select {
	case result := <-resultCh:
		// Should error because claude CLI isn't available
		if result.Err == nil {
			t.Log("enhance task succeeded (claude CLI available)")
		}
		// Either way, verify the result was sent back
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for enhance result")
	}
}

func TestWorkerSubmit_EnhanceTask(t *testing.T) {
	w := NewWorker(10)

	resultCh := make(chan Result, 1)
	w.Submit(Task{
		Type:     "enhance",
		Stdin:    "test body content",
		Prompt:   "New content since last review.",
		Model:    ModelHaiku,
		ResultCh: resultCh,
	})

	if got := w.PendingCount(); got != 1 {
		t.Errorf("PendingCount() = %d, want 1", got)
	}
}

func TestResultThoughtsField(t *testing.T) {
	// Verify the Thoughts field is available on Result
	r := Result{
		Output:   "corrected body",
		Thoughts: "some thoughts",
	}
	if r.Thoughts != "some thoughts" {
		t.Errorf("Thoughts = %q, want %q", r.Thoughts, "some thoughts")
	}
}

func TestTaskStdinField(t *testing.T) {
	// Verify the Stdin field is available on Task
	task := Task{
		Type:  "enhance",
		Stdin: "note body via stdin",
	}
	if task.Stdin != "note body via stdin" {
		t.Errorf("Stdin = %q, want %q", task.Stdin, "note body via stdin")
	}
}

func TestGenerateFrontmatter_NoMissingFields(t *testing.T) {
	n := &note.Note{
		Title:   "Complete Note",
		Date:    "2024-01-01",
		Summary: "All fields present",
		Tags:    []string{"complete"},
		Status:  "active",
		Source:  "written",
	}

	result, err := GenerateFrontmatter(context.Background(), n)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for complete note, got %v", result)
	}
}

func TestSuggestTagsParsing(t *testing.T) {
	// Test the tag parsing logic by examining the output format expectations
	// We can't call the actual CLI, but we can verify the parsing would work
	// for known output formats by testing the string manipulation directly

	tests := []struct {
		name   string
		output string
		want   int
	}{
		{"standard format", "[go, testing, moss]", 3},
		{"with quotes", `["go", "testing"]`, 2},
		{"single tag", "[programming]", 1},
		{"empty", "[]", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the parsing logic from SuggestTags
			output := tt.output
			output = trimBrackets(output)
			var tags []string
			for _, tg := range splitAndTrim(output) {
				if tg != "" {
					tags = append(tags, tg)
				}
			}
			if len(tags) != tt.want {
				t.Errorf("got %d tags, want %d: %v", len(tags), tt.want, tags)
			}
		})
	}
}

func TestExtractStreamDelta(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			"stream_event wrapped text_delta",
			`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello"}},"session_id":"abc"}`,
			"Hello",
		},
		{
			"stream_event wrapped thinking_delta",
			`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}},"session_id":"abc"}`,
			"",
		},
		{
			"unwrapped content_block_delta (backwards compat)",
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
			"Hi",
		},
		{
			"assistant message (not a delta)",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			"",
		},
		{
			"stream_event non-delta (message_start)",
			`{"type":"stream_event","event":{"type":"message_start","message":{"model":"haiku"}},"session_id":"abc"}`,
			"",
		},
		{
			"invalid json",
			`not json`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStreamDelta(tt.line)
			if got != tt.want {
				t.Errorf("extractStreamDelta() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractTextSnapshot(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			"assistant message with text",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			"Hello world",
		},
		{
			"assistant with multiple content blocks",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]}}`,
			"Hello world",
		},
		{
			"result message",
			`{"type":"result","result":"Final text here"}`,
			"Final text here",
		},
		{
			"system init message",
			`{"type":"system","subtype":"init","session_id":"abc"}`,
			"",
		},
		{
			"content_block_delta (not a snapshot)",
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextSnapshot(tt.line)
			if got != tt.want {
				t.Errorf("extractTextSnapshot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStreamDelta_AccumulatedChunks(t *testing.T) {
	// Simulate a real streaming sequence with stream_event wrapped deltas
	messages := []string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Great "}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"note! "}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Consider"}}}`,
		`{"type":"result","result":"Great note! Consider"}`,
	}

	var accumulated strings.Builder
	var thoughtsSent int
	var chunks []string

	for _, line := range messages {
		if delta := extractStreamDelta(line); delta != "" {
			accumulated.WriteString(delta)
			fullText := accumulated.String()
			if len(fullText) > thoughtsSent {
				chunks = append(chunks, fullText[thoughtsSent:])
				thoughtsSent = len(fullText)
			}
			continue
		}
		if snapshot := extractTextSnapshot(line); snapshot != "" {
			if snapshot != accumulated.String() {
				accumulated.Reset()
				accumulated.WriteString(snapshot)
			}
		}
	}

	if got := strings.Join(chunks, ""); got != "Great note! Consider" {
		t.Errorf("accumulated chunks = %q, want %q", got, "Great note! Consider")
	}
	if len(chunks) != 3 {
		t.Errorf("got %d chunks, want 3: %v", len(chunks), chunks)
	}
}

func TestStreamEventTypes(t *testing.T) {
	// Verify StreamEvent fields work correctly
	chunk := StreamEvent{ThoughtsDelta: "Hello "}
	if chunk.ThoughtsDelta != "Hello " {
		t.Errorf("ThoughtsDelta = %q, want %q", chunk.ThoughtsDelta, "Hello ")
	}
	if chunk.Done {
		t.Error("chunk should not be done")
	}

	complete := StreamEvent{CorrectedBody: "Fixed text", Done: true}
	if !complete.Done {
		t.Error("complete event should be done")
	}
	if complete.CorrectedBody != "Fixed text" {
		t.Errorf("CorrectedBody = %q, want %q", complete.CorrectedBody, "Fixed text")
	}
}

// Helper functions that mirror the parsing logic in SuggestTags
func trimBrackets(s string) string {
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-1]
	}
	return s
}

func splitAndTrim(s string) []string {
	var result []string
	for _, part := range splitComma(s) {
		part = trimQuotes(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitComma(s string) []string {
	return append([]string(nil), splitByChar(s, ',')...)
}

func splitByChar(s string, c byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimQuotes(s string) string {
	s = trimSpaces(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func trimSpaces(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
