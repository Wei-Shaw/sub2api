package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
)

// continuationUserText drives the follow-up Run when a conversation ends with
// tool results instead of a new user message (the OpenAI tool round-trip):
// the results are replayed inside the previous turn, so the action only needs
// to tell the model to keep going.
const continuationUserText = "Continue."

// cursorToolChoiceString extracts the string form of chat tool_choice
// ("none" | "auto" | "required"); object forms resolve to "" (auto).
func cursorToolChoiceString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var choice string
	if err := json.Unmarshal(raw, &choice); err == nil {
		return strings.TrimSpace(choice)
	}
	return ""
}

// cursorAgentMode reports whether a chat request runs in Agent mode: function
// tools declared and not disabled via tool_choice:"none".
func cursorAgentMode(tools []apicompat.ChatTool, toolChoice string) bool {
	if len(tools) == 0 {
		return false
	}
	return toolChoice != "none"
}

// buildCursorAgentTools converts declared function tools into the cursor tool
// table. Non-function tools (x_search and other provider-hosted tools) are
// skipped: the caller cannot execute those.
func buildCursorAgentTools(tools []apicompat.ChatTool) []cursor.AgentTool {
	out := make([]cursor.AgentTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		if tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
			continue
		}
		out = append(out, cursor.AgentTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildCursorAgentRunRequest converts a normalized chat-completions request
// (messages + tools) into a cursor AgentRunRequest. Without executable tools
// this is the Ask-mode flattened request; with tools the conversation is
// replayed as Agent turns and the caller's tool table is declared.
func buildCursorAgentRunRequest(model string, messages []apicompat.ChatMessage, tools []apicompat.ChatTool, toolChoice string) cursor.AgentRunRequest {
	if !cursorAgentMode(tools, toolChoice) {
		msgs := make([]cursor.ChatMessage, 0, len(messages))
		for _, message := range messages {
			msgs = append(msgs, cursor.ChatMessage{Role: message.Role, Content: chatRawContentText(message.Content)})
		}
		return cursor.AgentRunRequest{Model: model, Messages: msgs}
	}

	turns, systemPrompt, actionText := buildCursorAgentTurns(messages)
	msgs := make([]cursor.ChatMessage, 0, 2)
	if systemPrompt != "" {
		msgs = append(msgs, cursor.ChatMessage{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, cursor.ChatMessage{Role: "user", Content: actionText})
	return cursor.AgentRunRequest{
		Model:    model,
		Messages: msgs,
		Tools:    buildCursorAgentTools(tools),
		Turns:    turns,
	}
}

// cursorToolCallRef locates a recorded tool call step by absolute position:
// turn index in turns, or the still-open turn when turn == -1 is resolved
// during attach.
type cursorToolCallRef struct {
	turn int
	step int
}

// buildCursorAgentTurns replays chat messages as cursor Agent turns. One turn
// per user message: the user text plus every assistant step that followed it
// (text, thinking, tool calls), with tool results paired back onto their call
// records. Returns the closed turns, the hoisted system prompt, and the action
// text for the Run's user message.
func buildCursorAgentTurns(messages []apicompat.ChatMessage) (turns []cursor.AgentTurn, systemPrompt, actionText string) {
	var (
		open       *cursor.AgentTurn
		openIdx    = -1
		refs       = map[string]cursorToolCallRef{}
		terminated = true // no open turn pending as action
	)

	flush := func() {
		if open != nil {
			turns = append(turns, *open)
			open = nil
			openIdx = -1
		}
	}
	ensure := func() *cursor.AgentTurn {
		if open == nil {
			open = &cursor.AgentTurn{}
			openIdx = len(turns)
		}
		return open
	}

	for _, message := range messages {
		switch message.Role {
		case "system":
			if text := chatRawContentText(message.Content); text != "" {
				if systemPrompt != "" {
					systemPrompt += "\n\n"
				}
				systemPrompt += text
			}
		case "user":
			flush()
			open = &cursor.AgentTurn{UserText: chatRawContentText(message.Content)}
			openIdx = len(turns)
			terminated = false
		case "assistant":
			turn := ensure()
			if text := chatRawContentText(message.Content); text != "" {
				turn.Steps = append(turn.Steps, cursor.AgentTurnStep{AssistantText: text})
			}
			for _, call := range message.ToolCalls {
				turn.Steps = append(turn.Steps, cursor.AgentTurnStep{ToolCall: &cursor.AgentToolCallRecord{
					CallID:   call.ID,
					Name:     call.Function.Name,
					ArgsJSON: json.RawMessage(call.Function.Arguments),
				}})
				if call.ID != "" {
					refs[call.ID] = cursorToolCallRef{turn: openIdx, step: len(turn.Steps) - 1}
				}
			}
			if len(message.ToolCalls) > 0 || chatRawContentText(message.Content) != "" {
				terminated = true
			}
		case "tool":
			attachCursorToolResult(refs, &turns, &open, openIdx, message)
			terminated = true
		}
	}

	if open != nil && !terminated {
		// The trailing user message is the Run action, not replayed history.
		actionText = open.UserText
	} else {
		flush()
		actionText = continuationUserText
	}
	return turns, systemPrompt, actionText
}

// attachCursorToolResult pairs a role:"tool" message with its recorded call
// (matched by tool_call_id), wherever that call lives in the replay.
func attachCursorToolResult(
	refs map[string]cursorToolCallRef,
	turns *[]cursor.AgentTurn,
	open **cursor.AgentTurn,
	openIdx int,
	message apicompat.ChatMessage,
) {
	callID := message.ToolCallID
	if callID == "" {
		return
	}
	ref, ok := refs[callID]
	if !ok {
		return
	}
	var record *cursor.AgentToolCallRecord
	if open != nil && *open != nil && ref.turn == openIdx {
		record = (*open).Steps[ref.step].ToolCall
	} else if ref.turn >= 0 && ref.turn < len(*turns) {
		record = (*turns)[ref.turn].Steps[ref.step].ToolCall
	}
	if record == nil {
		return
	}
	record.Result = &cursor.AgentToolResultRecord{
		ContentText: chatRawContentText(message.Content),
	}
}
