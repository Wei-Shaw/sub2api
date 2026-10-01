package cursor

import (
	"crypto/sha256"
	"encoding/json"
	"regexp"
)

// Cursor builds the model's actual prompt from
// ConversationStateStructure.root_prompt_messages_json: a list of SHA-256
// blob IDs whose payloads are Vercel-AI-SDK-shaped JSON messages. The client
// serves those blobs through the KV exec handshake. turns[] is display
// metadata only — replaying history there (as this gateway once did) leaves
// the model blind to prior turns.

var toolCallIDInvalid = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// NormalizeToolCallID maps a call id onto Cursor's `^[a-zA-Z0-9_-]+$` charset
// (max 64 chars). Cursor-issued ids can carry separators (newlines) that the
// replay path rejects; the same normalization on the tool-result side keeps
// the pairing intact.
func NormalizeToolCallID(id string) string {
	sanitized := toolCallIDInvalid.ReplaceAllString(id, "_")
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}
	return sanitized
}

// RootPromptBlobs is the pre-seeded KV blob set plus the id list to declare
// on the conversation state.
type RootPromptBlobs struct {
	IDs   [][]byte
	ByIDs map[string][]byte
}

// storeBlob adds data under its SHA-256 id and returns the id.
func (b *RootPromptBlobs) storeBlob(data []byte) []byte {
	id := sha256.Sum256(data)
	b.ByIDs[string(id[:])] = data
	return id[:]
}

// BuildRootPromptBlobs renders the conversation history as root-prompt
// message blobs: one system blob, then per turn the user message, assistant
// text, tool calls, and paired tool results.
func BuildRootPromptBlobs(systemPrompt string, turns []AgentTurn) *RootPromptBlobs {
	out := &RootPromptBlobs{ByIDs: make(map[string][]byte)}
	push := func(obj any) {
		data, err := json.Marshal(obj)
		if err != nil {
			return
		}
		id := sha256.Sum256(data)
		out.IDs = append(out.IDs, id[:])
		out.ByIDs[string(id[:])] = data
	}
	textPart := func(text string) map[string]any {
		return map[string]any{"type": "text", "text": text}
	}

	if systemPrompt != "" {
		push(map[string]any{"role": "system", "content": systemPrompt})
	}
	for _, turn := range turns {
		if turn.UserText != "" {
			push(map[string]any{"role": "user", "content": []any{textPart(turn.UserText)}})
		}
		for _, step := range turn.Steps {
			if step.AssistantText != "" {
				push(map[string]any{"role": "assistant", "content": []any{textPart(step.AssistantText)}})
			}
			if step.ToolCall == nil {
				continue
			}
			call := step.ToolCall
			callID := NormalizeToolCallID(call.CallID)
			args := decodeToolArgsObject(call.ArgsJSON)
			push(map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"type":       "tool-call",
					"toolCallId": callID,
					"toolName":   call.Name,
					"args":       args,
				}},
			})
			if call.Result != nil {
				push(map[string]any{
					"role": "tool",
					"id":   callID,
					"content": []any{map[string]any{
						"type":       "tool-result",
						"toolCallId": callID,
						"toolName":   call.Name,
						"result":     call.Result.ContentText,
						"isError":    call.Result.IsError,
					}},
				})
			}
		}
	}
	return out
}

// decodeToolArgsObject parses the recorded arguments into a map; non-object
// JSON collapses under a single "args" key to keep the field an object.
func decodeToolArgsObject(raw json.RawMessage) map[string]any {
	trimmed := string(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return map[string]any{"args": trimmed}
	}
	if obj == nil {
		return map[string]any{}
	}
	return obj
}

// EncodeAgentTurnBlobs renders replay turns the way the modern protocol
// expects: every UserMessage, ConversationStep, and ConversationTurn is
// serialized, stored as a KV blob, and referenced by id — turns[] holds the
// OUTER turn blob ids, never inline messages. The blob payloads join the
// same pre-seeded KV set as the root prompt.
func EncodeAgentTurnBlobs(blobs *RootPromptBlobs, turns []AgentTurn) [][]byte {
	if blobs == nil {
		blobs = &RootPromptBlobs{ByIDs: make(map[string][]byte)}
	}
	ids := make([][]byte, 0, len(turns))
	for _, turn := range turns {
		var user ProtobufWriter
		user.String(fieldUserMsgText, turn.UserText)
		user.String(fieldUserMsgID, newReplayMessageID())
		user.Varint(fieldUserMsgMode, AgentModeAgent)
		userBlob := blobs.storeBlob(user.Result())

		var stepIDs [][]byte
		for _, step := range turn.Steps {
			var stepMsg []byte
			switch {
			case step.ToolCall != nil:
				stepMsg = encodeToolCallStep(*step.ToolCall)
			case step.AssistantText != "":
				var assistant ProtobufWriter
				assistant.String(fieldStepAssistant, step.AssistantText)
				var s ProtobufWriter
				s.Bytes(fieldStepAssistant, assistant.Result())
				stepMsg = s.Result()
			case step.ThinkingText != "":
				var thinking ProtobufWriter
				thinking.String(fieldStepThinking, step.ThinkingText)
				var s ProtobufWriter
				s.Bytes(fieldStepThinking, thinking.Result())
				stepMsg = s.Result()
			default:
				continue
			}
			stepIDs = append(stepIDs, blobs.storeBlob(stepMsg))
		}

		var agentTurn ProtobufWriter
		agentTurn.Bytes(fieldAgentTurnUser, userBlob)
		for _, stepID := range stepIDs {
			agentTurn.Bytes(fieldAgentTurnStep, stepID)
		}
		var turnMsg ProtobufWriter
		turnMsg.Bytes(fieldTurnAgent, agentTurn.Result())
		ids = append(ids, blobs.storeBlob(turnMsg.Result()))
	}
	return ids
}
