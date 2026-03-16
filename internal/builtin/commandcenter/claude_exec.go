package commandcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	tea "github.com/charmbracelet/bubbletea"
)

// Message types for Claude AI results.

type claudeEditFinishedMsg struct {
	todoID string
	output string
	err    error
}

type claudeEnrichFinishedMsg struct {
	output string
	err    error
}

type claudeCommandFinishedMsg struct {
	output string
	err    error
}

type claudeFocusFinishedMsg struct {
	output string
	err    error
}

type claudeDateParseFinishedMsg struct {
	todoID string
	output string
	err    error
}

type claudeRefinePromptMsg struct {
	todoID string
	output string
	err    error
}

func claudeEditCmd(l llm.LLM, prompt, todoID string) tea.Cmd {
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeEditFinishedMsg{
			todoID: todoID,
			output: out,
			err:    err,
		}
	}
}

func claudeEnrichCmd(l llm.LLM, prompt string) tea.Cmd {
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeEnrichFinishedMsg{
			output: out,
			err:    err,
		}
	}
}

func claudeCommandCmd(l llm.LLM, prompt, projectDir string) tea.Cmd {
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeCommandFinishedMsg{
			output: out,
			err:    err,
		}
	}
}

func claudeDateParseCmd(l llm.LLM, input string, todoID string) tea.Cmd {
	prompt := fmt.Sprintf("Parse this into YYYY-MM-DD format. Today is %s. Input: %s. Reply with only the date in YYYY-MM-DD format, nothing else.",
		time.Now().Format("2006-01-02"), input)
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeDateParseFinishedMsg{
			todoID: todoID,
			output: out,
			err:    err,
		}
	}
}

type claudeReviewAddressedMsg struct {
	todoID string
	output string
	err    error
	round  int
}

func claudeReviewAddressCmd(l llm.LLM, todoID string, original string, annotated string, round int) tea.Cmd {
	prompt := fmt.Sprintf(`You are refining a task prompt based on the user's inline annotations. The user opened the prompt in an editor and added comments, questions, or changes directly in the text.

Your job: address every annotation the user made. Incorporate their feedback, answer their questions by updating the text, and produce a clean final prompt with no leftover annotations or TODO markers.

Original prompt:
"""
%s
"""

Annotated prompt (with the user's changes/comments):
"""
%s
"""

Output ONLY the updated prompt text. No explanation, no quotes, no markdown fences wrapping the whole thing.`, original, annotated)
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeReviewAddressedMsg{
			todoID: todoID,
			output: out,
			err:    err,
			round:  round,
		}
	}
}

func claudeRefinePromptCmd(l llm.LLM, todoID string, currentPrompt string) tea.Cmd {
	prompt := fmt.Sprintf(`You are refining a task prompt that will be sent to Claude Code (an AI coding agent) as its instruction. Improve the prompt to be clearer, more specific, and more actionable.

Rules:
- Keep the same intent and scope as the original
- Make instructions concrete and unambiguous
- Add structure (numbered steps, bullet points) where helpful
- Include acceptance criteria if not already present
- Remove vague language, replace with specifics
- Keep it concise — don't pad with unnecessary context

Original prompt:
"""
%s
"""

Output ONLY the refined prompt text. No explanation, no quotes, no markdown fences wrapping the whole thing.`, currentPrompt)
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeRefinePromptMsg{
			todoID: todoID,
			output: out,
			err:    err,
		}
	}
}

func claudeRefinePromptWithInstructionCmd(l llm.LLM, todoID string, currentPrompt string, instruction string) tea.Cmd {
	prompt := fmt.Sprintf(`You are rewriting a task prompt based on the user's instructions. The prompt will be sent to Claude Code (an AI coding agent) as its instruction.

User's instructions for rewriting:
"""
%s
"""

Current prompt:
"""
%s
"""

Rewrite the prompt according to the user's instructions. Output ONLY the rewritten prompt text. No explanation, no quotes, no markdown fences wrapping the whole thing.`, instruction, currentPrompt)
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeRefinePromptMsg{
			todoID: todoID,
			output: out,
			err:    err,
		}
	}
}

func claudeFocusCmd(l llm.LLM, prompt string) tea.Cmd {
	return func() tea.Msg {
		out, err := l.Complete(context.Background(), prompt)
		return claudeFocusFinishedMsg{
			output: out,
			err:    err,
		}
	}
}

func buildCommandPromptWithHistory(cc *db.CommandCenter, name string, conversation []commandTurn) string {
	ccJSON, _ := json.MarshalIndent(cc, "", "  ")

	var convoSection string
	if len(conversation) > 1 {
		var sb strings.Builder
		sb.WriteString("\n## Conversation So Far\n")
		for _, turn := range conversation[:len(conversation)-1] {
			if turn.role == "user" {
				sb.WriteString(fmt.Sprintf("**User:** %s\n", turn.text))
			} else {
				sb.WriteString(fmt.Sprintf("**You asked:** %s\n", turn.text))
			}
		}
		convoSection = sb.String()
	}

	instruction := conversation[len(conversation)-1].text
	return fmt.Sprintf(`You are %s, a command center assistant inside a terminal dashboard. The user is giving you a short instruction.

## Your ONLY allowed actions

You can ONLY do these things:
1. **Create todos** -- extract action items from what the user says
2. **Complete todos** -- mark existing todos as done
3. **Answer quick questions** -- about the current state (calendar, todos, threads)
4. **Calendar actions** -- decline/accept events, only when explicitly asked
5. **Slack/Gmail actions** -- send messages, only when explicitly asked

## What you must NEVER do

- Do NOT perform the work described in a todo
- Do NOT research, investigate, or explore topics
- Do NOT use tools unless the user explicitly asks you to take an external action
- Your job is to CAPTURE work, not DO work

## Decision logic

1. If the user describes something they need to do -> create a todo for it
2. If the user says "done with X" or "finished X" -> complete the matching todo
3. If the user explicitly says "decline", "accept", "send", "message" -> take that action
4. If the user asks a question about their state -> answer from the command center data below
5. Otherwise -> create a todo

When in doubt, CREATE A TODO.

## Asking for Clarification

If the user's instruction is genuinely ambiguous, you may ask ONE short question.

## Current Command Center State
%s

## Current Time
%s
%s
## User Instruction
%q

## Response Format
Return ONLY a JSON object (no markdown fences, no explanation):
{
  "message": "Brief summary of what you did",
  "ask": "",
  "todos": [],
  "complete_todo_ids": []
}

- "message": Brief human-readable summary (empty string if asking a question)
- "ask": A short clarifying question if genuinely needed.
- "todos": Array of new todo items. Each: {"title": "...", "due": "", "who_waiting": "", "effort": "", "context": "", "detail": "", "project_dir": ""}
- "complete_todo_ids": Array of existing todo IDs to mark as completed

Output ONLY the JSON object.`, name, string(ccJSON), time.Now().Format(time.RFC3339), convoSection, instruction)
}

func buildFocusPrompt(cc *db.CommandCenter) string {
	var todoItems []string
	for _, t := range cc.ActiveTodos() {
		item := fmt.Sprintf("- %s", t.Title)
		if t.Due != "" {
			item += fmt.Sprintf(" (due: %s)", t.Due)
		}
		if t.WhoWaiting != "" {
			item += fmt.Sprintf(" [%s waiting]", t.WhoWaiting)
		}
		if t.Effort != "" {
			item += fmt.Sprintf(" (~%s)", t.Effort)
		}
		todoItems = append(todoItems, item)
	}

	var calItems []string
	now := time.Now()
	for _, e := range cc.Calendar.Today {
		if e.Declined || e.End.Before(now) {
			continue
		}
		calItems = append(calItems, fmt.Sprintf("- %s (%s-%s)",
			e.Title, e.Start.Format("3:04pm"), e.End.Format("3:04pm")))
	}

	var sb strings.Builder
	if len(calItems) > 0 {
		sb.WriteString("Remaining calendar today:\n")
		sb.WriteString(strings.Join(calItems, "\n"))
		sb.WriteString("\n\n")
	}
	sb.WriteString("Active todos:\n")
	sb.WriteString(strings.Join(todoItems, "\n"))

	return fmt.Sprintf(`Given my current state:

%s

Write a 1-2 sentence recommendation of what to focus on next and why. Consider: deadlines, who's waiting, available time between meetings, effort required, and momentum. Be direct and specific.

Output ONLY the recommendation text, no quotes, no JSON, no explanation.`, sb.String())
}

func buildEmptyFocusPrompt(cc *db.CommandCenter) string {
	var calItems []string
	now := time.Now()
	for _, e := range cc.Calendar.Today {
		if e.Declined || e.End.Before(now) {
			continue
		}
		calItems = append(calItems, fmt.Sprintf("- %s (%s-%s)",
			e.Title, e.Start.Format("3:04pm"), e.End.Format("3:04pm")))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Current time: %s\n", now.Format("Monday 3:04pm")))
	if len(calItems) > 0 {
		sb.WriteString("\nRemaining calendar today:\n")
		sb.WriteString(strings.Join(calItems, "\n"))
	} else {
		sb.WriteString("\nNo more meetings today.")
	}

	return fmt.Sprintf(`The user's todo list is completely empty. Zero items. They cleared everything.

%s

Write a 1-2 sentence remark about their empty todo list. Be surprising, witty, and entertaining. You can be snarky, deadpan, celebratory, philosophical, or absurd — mix it up. Reference their calendar situation if it's funny. Don't be generic or corny. Make them smile.

Output ONLY the remark text, no quotes, no JSON, no explanation.`, sb.String())
}

func buildEditPrompt(todo db.Todo, instruction string) string {
	// Build a map from the todo so proposed_prompt always appears (even when empty).
	// The struct uses omitempty which hides the field, but the LLM needs to see it.
	todoJSON, _ := json.MarshalIndent(todo, "", "  ")
	var todoMap map[string]interface{}
	json.Unmarshal(todoJSON, &todoMap)
	if _, ok := todoMap["proposed_prompt"]; !ok {
		todoMap["proposed_prompt"] = ""
	}
	todoJSON, _ = json.MarshalIndent(todoMap, "", "  ")
	return fmt.Sprintf(`You are updating a todo item. Here is the current todo:

%s

The user says: %q

## Field Guide
- "title": Short summary of the task (~80 chars max)
- "context": Brief categorization/label (~30 chars)
- "detail": Background information and notes
- "proposed_prompt": The full prompt that will be sent to an AI coding agent. When the user says "update the prompt" or "change the prompt", THIS is the field they mean. It should contain detailed, actionable instructions for the agent. This is NOT the title or context.
- "due": Due date (YYYY-MM-DD)
- "who_waiting": Who is waiting on this
- "effort": Estimated effort (e.g. "30m", "2h", "1d")
- "project_dir": Path to the project directory

## Rules
- When the user mentions "prompt", they mean the "proposed_prompt" field — update it with the full agent instructions
- Always return ALL fields, including "proposed_prompt" even if empty
- Return the COMPLETE updated todo as a JSON object

Output ONLY the JSON object, no markdown code fences, no explanation, no other text. Just raw JSON.`, string(todoJSON), instruction)
}

func buildEnrichPrompt(rawText string) string {
	return fmt.Sprintf(`Extract a todo item from the following text. Return a JSON object with these fields:
- title: concise action item (string, max ~80 chars)
- due: date in YYYY-MM-DD format if mentioned, otherwise empty string
- who_waiting: person name if someone is waiting on this, otherwise empty string
- effort: estimated effort like "30m", "2h", "1d" if you can infer it, otherwise empty string
- context: short categorization/label (string, max ~30 chars)
- detail: comprehensive background
- project_dir: relevant project directory path if mentioned, otherwise empty string

Text:
"""
%s
"""

Output ONLY the JSON object, no markdown code fences, no explanation, no other text. Just raw JSON.`, rawText)
}

// extractJSON finds JSON object in a string, stripping markdown fences if present.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		var inner []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				inner = append(inner, line)
			}
		}
		s = strings.Join(inner, "\n")
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func formatTodoContext(todo db.Todo) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("## Task: %s\n", todo.Title))
	if todo.Context != "" {
		parts = append(parts, fmt.Sprintf("**Context:** %s", displayContext(todo.Context)))
	}
	if todo.WhoWaiting != "" {
		parts = append(parts, fmt.Sprintf("**Who's waiting:** %s", todo.WhoWaiting))
	}
	if todo.Due != "" {
		label := db.FormatDueLabel(todo.Due)
		parts = append(parts, fmt.Sprintf("**Due:** %s (%s)", todo.Due, label))
	}
	if todo.Effort != "" {
		parts = append(parts, fmt.Sprintf("**Effort:** %s", todo.Effort))
	}
	if todo.Source != "" && todo.Source != "manual" {
		parts = append(parts, fmt.Sprintf("**Source:** %s", todo.Source))
	}
	if todo.Detail != "" {
		parts = append(parts, fmt.Sprintf("\n### Detail\n%s", todo.Detail))
	}
	return strings.Join(parts, "\n")
}
