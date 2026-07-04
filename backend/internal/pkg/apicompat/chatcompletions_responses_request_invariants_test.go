package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// assertChatInvariants enforces the DeepSeek / OpenAI Chat Completions message
// invariants that, when violated, surface as upstream 400s. Used to validate the
// request-direction converter against golden codex request shapes.
func assertChatInvariants(t *testing.T, messages []ChatMessage) {
	t.Helper()
	for i, m := range messages {
		// Every assistant tool_calls message must be immediately followed by one
		// tool message per tool_call_id, in order.
		if len(m.ToolCalls) > 0 {
			for j, tc := range m.ToolCalls {
				k := i + 1 + j
				require.Lessf(t, k, len(messages), "tool_call %s has no following tool message", tc.ID)
				require.Equalf(t, "tool", messages[k].Role, "tool_call %s not followed by a tool message", tc.ID)
				require.Equalf(t, tc.ID, messages[k].ToolCallID, "tool reply order mismatch for %s", tc.ID)
			}
		}
		// No two consecutive assistant messages.
		if i > 0 && m.Role == "assistant" && messages[i-1].Role == "assistant" {
			t.Fatalf("consecutive assistant messages at %d", i)
		}
		// No orphan tool replies.
		if m.Role == "tool" {
			require.NotEmptyf(t, m.ToolCallID, "tool message without tool_call_id at %d", i)
		}
	}
}

func convertGolden(t *testing.T, input string) []ChatMessage {
	t.Helper()
	msgs, err := responsesInputToChatMessages("You are a helpful assistant.", json.RawMessage(input))
	require.NoError(t, err)
	return msgs
}

// Golden sample: a single tool-call turn (codex runs one shell/curl command),
// the shape that produced the original "no response" / 400.
func TestGolden_SingleToolCall(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"latest sha?"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"need to run curl"}]},
		{"type":"function_call","call_id":"call_a","name":"exec_command","arguments":"{\"cmd\":\"curl x\"}"},
		{"type":"function_call_output","call_id":"call_a","output":"deadbeef"}
	]`)
	assertChatInvariants(t, msgs)
	// reasoning_content must ride on the assistant tool-call message.
	var asst *ChatMessage
	for i := range msgs {
		if len(msgs[i].ToolCalls) > 0 {
			asst = &msgs[i]
		}
	}
	require.NotNil(t, asst)
	require.Equal(t, "need to run curl", asst.ReasoningContent)
}

// Golden sample: parallel tool calls (codex runs git log + git tag at once).
func TestGolden_ParallelToolCalls(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"features?"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"inspect repo"}]},
		{"type":"function_call","call_id":"c0","name":"exec_command","arguments":"{\"cmd\":\"git log\"}"},
		{"type":"function_call","call_id":"c1","name":"exec_command","arguments":"{\"cmd\":\"git tag\"}"},
		{"type":"function_call_output","call_id":"c0","output":"log"},
		{"type":"function_call_output","call_id":"c1","output":"tags"}
	]`)
	assertChatInvariants(t, msgs)
	// Both parallel calls share ONE assistant message.
	var toolMsgs int
	for _, m := range msgs {
		if len(m.ToolCalls) == 2 {
			require.Equal(t, "c0", m.ToolCalls[0].ID)
			require.Equal(t, "c1", m.ToolCalls[1].ID)
		}
		if m.Role == "tool" {
			toolMsgs++
		}
	}
	require.Equal(t, 2, toolMsgs)
}

// Golden sample: an unknown item type (web_search_call from a 联网查询) sitting
// between a function_call and its output must not break tool↔reply adjacency.
func TestGolden_UnknownItemBetweenToolCallAndOutput(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"search"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"let me search"}]},
		{"type":"function_call","call_id":"c0","name":"exec_command","arguments":"{}"},
		{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"x"}},
		{"type":"function_call_output","call_id":"c0","output":"result"}
	]`)
	assertChatInvariants(t, msgs)
}

// Sequential tool calls (a tool reply between two calls) must stay in distinct
// assistant messages.
func TestRequest_SequentialToolCallsStaySeparate(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"function_call","call_id":"c1","name":"exec","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"r1"},
		{"type":"function_call","call_id":"c2","name":"exec","arguments":"{}"},
		{"type":"function_call_output","call_id":"c2","output":"r2"}
	]`)
	assertChatInvariants(t, msgs)
	assistants := 0
	for _, m := range msgs {
		if len(m.ToolCalls) == 1 {
			assistants++
		}
	}
	require.Equal(t, 2, assistants)
}

// Golden sample: codex injects a message (e.g. an "Approved command prefix
// saved" notice) between a function_call and its output. The intervening message
// must be moved after the tool reply so the assistant tool_calls is immediately
// followed by its reply.
func TestGolden_MessageBetweenToolCallAndOutput(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"do it"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"run cmd"}]},
		{"type":"function_call","call_id":"A","name":"exec","arguments":"{}"},
		{"type":"message","role":"developer","content":[{"type":"input_text","text":"Approved command prefix saved"}]},
		{"type":"function_call_output","call_id":"A","output":"ok"}
	]`)
	assertChatInvariants(t, msgs)
	// The assistant tool_calls message is immediately followed by its tool reply.
	for i, m := range msgs {
		if len(m.ToolCalls) > 0 {
			require.Equal(t, "tool", msgs[i+1].Role)
			require.Equal(t, "A", msgs[i+1].ToolCallID)
		}
	}
}

// Golden sample: a parallel tool call where one sibling's output is missing
// (codex interrupted/reconnected mid-execution). The unanswered tool_call must
// be dropped so the remaining assistant tool_calls are all answered.
func TestGolden_PartialParallelDropsUnansweredCall(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"q"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"r"}]},
		{"type":"function_call","call_id":"A","name":"exec","arguments":"{}"},
		{"type":"function_call","call_id":"B","name":"exec","arguments":"{}"},
		{"type":"function_call_output","call_id":"A","output":"oa"}
	]`)
	assertChatInvariants(t, msgs)
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			require.NotEqual(t, "B", tc.ID, "unanswered tool_call B should have been dropped")
		}
	}
}

// Golden sample: a dangling tool_call at the end of the history (no output yet).
// The assistant message holding only that call must be dropped entirely.
func TestGolden_DanglingToolCallDropped(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"q"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"r"}]},
		{"type":"function_call","call_id":"A","name":"exec","arguments":"{}"}
	]`)
	assertChatInvariants(t, msgs)
	for _, m := range msgs {
		require.Empty(t, m.ToolCalls, "dangling unanswered tool_call should have been dropped")
	}
}

// normalizeChatMessages drops an orphan tool reply whose tool_call was never
// announced.
func TestNormalize_DropsOrphanToolReply(t *testing.T) {
	msgs := convertGolden(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"q"}]},
		{"type":"function_call_output","call_id":"ghost","output":"orphan"}
	]`)
	for _, m := range msgs {
		require.NotEqualf(t, "tool", m.Role, "orphan tool reply should have been dropped")
	}
}
