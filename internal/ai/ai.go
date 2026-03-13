package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/devenjarvis/moss/internal/note"
	"gopkg.in/yaml.v3"
)

// filterEnv returns a copy of os.Environ() with the named variable removed.
func filterEnv(name string) []string {
	prefix := name + "="
	var result []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// EnhanceResult holds the output of an AI note enhancement.
type EnhanceResult struct {
	CorrectedBody string `json:"corrected_body"`
	Thoughts      string `json:"thoughts"`
}

const (
	ModelHaiku  = "claude-haiku-4-5-20251001"
	ModelSonnet = "" // empty string uses CLI default (Sonnet)
)

// Task represents an AI task to be processed by the background worker.
type Task struct {
	Type     string // "frontmatter", "summarize", "tags", "ask", "generate", "enhance"
	Note     *note.Note
	Prompt   string
	Stdin    string // explicit stdin content (used by enhance)
	Model    string
	ResultCh chan<- Result
}

// Result is the output of an AI task.
type Result struct {
	Task     Task
	Output   string
	Fields   map[string]string // structured output for frontmatter tasks
	Thoughts string            // AI thoughts/questions (enhance tasks)
	Err      error
}

// RunClaude executes a claude CLI subprocess with the given model and prompt,
// piping input via stdin.
func RunClaude(ctx context.Context, model, prompt, stdin string) (string, error) {
	args := []string{"-p", prompt}
	if model != "" {
		args = append(args, "--model", model)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(stdin) // always set stdin to prevent inheriting terminal
	cmd.Env = filterEnv("CLAUDECODE")    // prevent "nested session" errors
	setSysProcAttr(cmd)                  // detach from controlling terminal (unix only)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude: %w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GenerateFrontmatter generates missing frontmatter fields for a note using Haiku.
func GenerateFrontmatter(ctx context.Context, n *note.Note) (map[string]string, error) {
	missing := n.MissingFrontmatterFields()
	if len(missing) == 0 {
		return nil, nil
	}

	prompt := fmt.Sprintf(`Analyze this markdown note and generate ONLY the following YAML frontmatter fields: %s

Rules:
- Output ONLY valid YAML key-value pairs, nothing else
- For tags: output as YAML array like [tag1, tag2, tag3], max 5 tags
- For summary: write 2-3 concise sentences
- For status: use "active"
- For source: use "written"
- For date: use the date from the filename or content if available, otherwise use today
- Do not include any explanation or markdown formatting`, strings.Join(missing, ", "))

	output, err := RunClaude(ctx, ModelHaiku, prompt, n.Body)
	if err != nil {
		return nil, err
	}

	// Strip any surrounding --- fences the model may include
	cleaned := output
	if idx := strings.Index(cleaned, "---"); idx >= 0 {
		cleaned = cleaned[idx+3:]
		if end := strings.Index(cleaned, "---"); end >= 0 {
			cleaned = cleaned[:end]
		}
	}

	// Parse output as YAML to correctly handle multi-line values (e.g. summaries)
	result := make(map[string]string)
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(cleaned), &parsed); err == nil && parsed != nil {
		for k, v := range parsed {
			result[k] = fmt.Sprintf("%v", v)
		}
	} else {
		// Fallback: naive line-by-line parsing for non-YAML output
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				result[key] = val
			}
		}
	}

	return result, nil
}

// SuggestTags suggests tags for a note using Haiku.
func SuggestTags(ctx context.Context, body string) ([]string, error) {
	prompt := `Suggest up to 5 tags for this note. Output ONLY a YAML array like [tag1, tag2, tag3]. No explanation.`

	output, err := RunClaude(ctx, ModelHaiku, prompt, body)
	if err != nil {
		return nil, err
	}

	// Simple parse of [tag1, tag2, tag3]
	output = strings.Trim(output, "[] \n")
	var tags []string
	for _, t := range strings.Split(output, ",") {
		t = strings.TrimSpace(t)
		t = strings.Trim(t, "\"'")
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

// Ask sends a question about notes to Claude using Sonnet.
func Ask(ctx context.Context, question string, noteContents string) (string, error) {
	prompt := fmt.Sprintf(`You are a helpful assistant for a note-taking app called Moss.
Answer the following question based on the provided notes.
Be concise and reference specific notes when relevant.

Question: %s`, question)

	return RunClaude(ctx, ModelSonnet, prompt, noteContents)
}

// GenerateNote generates a new note from a prompt using Sonnet.
func GenerateNote(ctx context.Context, userPrompt string, sourceNotes string) (string, error) {
	prompt := fmt.Sprintf(`Generate a markdown note based on this request: %s

Output ONLY the markdown content (with frontmatter). Use this frontmatter format:
---
title: <appropriate title>
date: <today's date>
tags: [relevant, tags]
status: active
source: generated
summary: <2-3 sentence summary>
---

Then write the note body in clean markdown.`, userPrompt)

	return RunClaude(ctx, ModelSonnet, prompt, sourceNotes)
}

// StreamEvent represents a chunk of streaming output from AI enhancement.
type StreamEvent struct {
	// ThoughtsDelta contains new text to append to the thoughts display.
	// Empty when the event is a completion or body event.
	ThoughtsDelta string

	// CorrectedBody is set only in the final event when streaming is complete.
	CorrectedBody string

	// Done is true for the final event.
	Done bool

	// Err is set if the stream encountered an error.
	Err error
}

// enhanceDelimiter separates thoughts from corrected body in streaming output.
const enhanceDelimiter = "===CORRECTED==="

// EnhanceStream starts a streaming AI enhancement and returns a channel of events.
// Thoughts are streamed as they arrive; the corrected body is sent in the final event.
func EnhanceStream(ctx context.Context, body string, diff string) <-chan StreamEvent {
	ch := make(chan StreamEvent, 16)

	prompt := fmt.Sprintf(`You are an editing assistant for a note-taking app. You will receive a markdown note body via stdin.

Your job:
1. First, provide 1-3 brief thoughts or questions about the note that might prompt the writer to expand, clarify, or think deeper. These should be conversational and helpful.
2. Then output the exact delimiter line: %s
3. Then output the corrected note body. Fix ONLY spelling, grammar, punctuation, and formatting errors. Do NOT add new content, change the meaning, restructure sections, or remove anything. Preserve all markdown formatting exactly. If there are no corrections needed, output the body unchanged.

Recent changes (diff) for context:
%s

Output format (no markdown fences, no extra text):
<your thoughts>
%s
<corrected body>`, enhanceDelimiter, diff, enhanceDelimiter)

	go func() {
		defer close(ch)

		args := []string{"-p", prompt, "--output-format", "stream-json", "--verbose", "--include-partial-messages"}
		args = append(args, "--model", ModelHaiku)

		cmd := exec.CommandContext(ctx, "claude", args...)
		cmd.Stdin = strings.NewReader(body)
		// Clear CLAUDECODE env var to prevent "nested session" errors
		cmd.Env = filterEnv("CLAUDECODE")
		setSysProcAttr(cmd)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- StreamEvent{Err: fmt.Errorf("enhance stream: %w", err)}
			return
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			ch <- StreamEvent{Err: fmt.Errorf("enhance stream: %w", err)}
			return
		}

		var thoughtsSent int // how many bytes of thoughts we've already sent
		delimiterFound := false
		scanner := bufio.NewScanner(stdout)
		// Increase scanner buffer for large responses
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		// Each partial message contains the FULL accumulated text so far.
		// We track how much thoughts text we've sent and detect the delimiter.
		var lastText string

		for scanner.Scan() {
			line := scanner.Text()
			fullText := extractTextContent(line)
			if fullText == "" || fullText == lastText {
				continue
			}
			lastText = fullText

			if delimiterFound {
				// Already past delimiter — body is growing, just keep lastText updated
				continue
			}

			// Check if delimiter has appeared in accumulated text
			if idx := strings.Index(fullText, enhanceDelimiter); idx >= 0 {
				// Send any unsent thoughts before the delimiter
				thoughtsPortion := fullText[:idx]
				if len(thoughtsPortion) > thoughtsSent {
					ch <- StreamEvent{ThoughtsDelta: thoughtsPortion[thoughtsSent:]}
				}
				delimiterFound = true
				continue
			}

			// Stream new thoughts since last partial message
			if len(fullText) > thoughtsSent {
				ch <- StreamEvent{ThoughtsDelta: fullText[thoughtsSent:]}
				thoughtsSent = len(fullText)
			}
		}

		if err := cmd.Wait(); err != nil {
			ch <- StreamEvent{Err: fmt.Errorf("enhance stream: %w: %s", err, stderr.String())}
			return
		}

		// Extract corrected body from the final accumulated text
		var correctedBody string
		if delimiterFound {
			if idx := strings.Index(lastText, enhanceDelimiter); idx >= 0 {
				correctedBody = strings.TrimSpace(lastText[idx+len(enhanceDelimiter):])
			}
		} else if lastText != "" {
			// Delimiter never appeared during streaming; try the final text
			if idx := strings.Index(lastText, enhanceDelimiter); idx >= 0 {
				thoughtsPortion := lastText[:idx]
				if len(thoughtsPortion) > thoughtsSent {
					ch <- StreamEvent{ThoughtsDelta: thoughtsPortion[thoughtsSent:]}
				}
				correctedBody = strings.TrimSpace(lastText[idx+len(enhanceDelimiter):])
			}
		}

		ch <- StreamEvent{
			CorrectedBody: correctedBody,
			Done:          true,
		}
	}()

	return ch
}

// streamJSON is the minimal structure for parsing Claude CLI stream-json output.
type streamJSON struct {
	Type    string `json:"type"`
	Message *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// extractTextContent extracts the full accumulated text from a stream-json line.
// With --include-partial-messages, each assistant message contains all text
// generated so far. We compute deltas by comparing with the previous message.
func extractTextContent(line string) string {
	var msg streamJSON
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return ""
	}
	if msg.Type == "assistant" && msg.Message != nil {
		for _, c := range msg.Message.Content {
			if c.Type == "text" {
				return c.Text
			}
		}
	}
	// Also handle result type which contains final text
	if msg.Type == "result" {
		var resultMsg struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &resultMsg); err == nil && resultMsg.Result != "" {
			return resultMsg.Result
		}
	}
	return ""
}

// Enhance sends a note body to Haiku for spelling/grammar corrections and
// returns the corrected body along with brief thoughts/questions about the note.
// This is the non-streaming version kept as fallback.
func Enhance(ctx context.Context, body string, diff string) (EnhanceResult, error) {
	prompt := fmt.Sprintf(`You are an editing assistant for a note-taking app. You will receive a markdown note body via stdin.

Your job:
1. Fix ONLY spelling, grammar, punctuation, and formatting errors in the note body. Do NOT add new content, change the meaning, restructure sections, or remove anything. Preserve all markdown formatting exactly.
2. Provide 1-3 brief thoughts or questions about the note that might prompt the writer to expand, clarify, or think deeper. These should be conversational and helpful.

Recent changes (diff) for context:
%s

Respond with ONLY valid JSON in this exact format (no markdown fences):
{"corrected_body": "<the full corrected note body>", "thoughts": "<your brief thoughts/questions, separated by newlines>"}`, diff)

	output, err := RunClaude(ctx, ModelHaiku, prompt, body)
	if err != nil {
		return EnhanceResult{}, err
	}

	// Strip markdown code fences if present
	output = strings.TrimSpace(output)
	if strings.HasPrefix(output, "```") {
		lines := strings.SplitN(output, "\n", 2)
		if len(lines) == 2 {
			output = lines[1]
		}
		if idx := strings.LastIndex(output, "```"); idx >= 0 {
			output = output[:idx]
		}
		output = strings.TrimSpace(output)
	}

	var result EnhanceResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return EnhanceResult{}, fmt.Errorf("enhance: failed to parse response: %w", err)
	}

	return result, nil
}

// Worker processes AI tasks from a channel in the background.
type Worker struct {
	tasks  chan Task
	cancel context.CancelFunc
}

// NewWorker creates a new background AI worker.
func NewWorker(bufSize int) *Worker {
	return &Worker{
		tasks: make(chan Task, bufSize),
	}
}

// Start begins processing tasks in a background goroutine.
func (w *Worker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case task, ok := <-w.tasks:
				if !ok {
					return
				}
				w.processTask(ctx, task)
			}
		}
	}()
}

// Submit queues a task for processing.
func (w *Worker) Submit(task Task) {
	select {
	case w.tasks <- task:
	default:
		// Queue full, drop the task
		if task.ResultCh != nil {
			task.ResultCh <- Result{Task: task, Err: fmt.Errorf("task queue full")}
		}
	}
}

// Stop shuts down the worker.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	close(w.tasks)
}

// PendingCount returns the number of pending tasks.
func (w *Worker) PendingCount() int {
	return len(w.tasks)
}

func (w *Worker) processTask(ctx context.Context, task Task) {
	var output string
	var fields map[string]string
	var err error

	var thoughts string

	switch task.Type {
	case "frontmatter":
		fields, err = GenerateFrontmatter(ctx, task.Note)
	case "ask":
		output, err = Ask(ctx, task.Prompt, task.Note.Body)
	case "generate":
		output, err = GenerateNote(ctx, task.Prompt, task.Note.Body)
	case "enhance":
		var result EnhanceResult
		result, err = Enhance(ctx, task.Stdin, task.Prompt)
		if err == nil {
			output = result.CorrectedBody
			thoughts = result.Thoughts
		}
	}

	if task.ResultCh != nil {
		task.ResultCh <- Result{Task: task, Output: output, Fields: fields, Thoughts: thoughts, Err: err}
	}
}
