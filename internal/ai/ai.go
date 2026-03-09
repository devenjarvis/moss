package ai

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/devenjarvis/moss/internal/note"
)

const (
	ModelHaiku  = "claude-haiku-4-5-20251001"
	ModelSonnet = "" // empty string uses CLI default (Sonnet)
)

// Task represents an AI task to be processed by the background worker.
type Task struct {
	Type     string // "frontmatter", "summarize", "tags", "ask", "generate"
	Note     *note.Note
	Prompt   string
	Model    string
	ResultCh chan<- Result
}

// Result is the output of an AI task.
type Result struct {
	Task   Task
	Output string
	Err    error
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from controlling terminal

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

	// Parse the output as simple key-value pairs
	result := make(map[string]string)
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
	var err error

	switch task.Type {
	case "frontmatter":
		var fields map[string]string
		fields, err = GenerateFrontmatter(ctx, task.Note)
		if err == nil && fields != nil {
			var parts []string
			for k, v := range fields {
				parts = append(parts, fmt.Sprintf("%s: %s", k, v))
			}
			output = strings.Join(parts, "\n")
		}
	case "ask":
		output, err = Ask(ctx, task.Prompt, task.Note.Body)
	case "generate":
		output, err = GenerateNote(ctx, task.Prompt, task.Note.Body)
	}

	if task.ResultCh != nil {
		task.ResultCh <- Result{Task: task, Output: output, Err: err}
	}
}
