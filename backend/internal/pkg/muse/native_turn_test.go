//go:build unit

package muse

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func nativeRecord(event string, sequence int64, payload any) SubscriptionRecord {
	body, _ := json.Marshal(payload)
	raw, _ := json.Marshal(map[string]any{"type": "event", "event": event, "seq": sequence, "payload": json.RawMessage(body)})
	return SubscriptionRecord{Type: "event", Event: event, Payload: body, Raw: raw}
}
func nativeAck() SubscriptionRecord {
	raw := json.RawMessage(`{"type":"res","status":"ok","payload":{"session_id":"fresh-chat","agent_id":"root-agent","is_thread":true,"is_primary":false,"subscribed":true}}`)
	return SubscriptionRecord{Type: "res", Status: "ok", Raw: raw}
}
func nativeUser(id string) SubscriptionRecord {
	return nativeRecord("message.user", 100, map[string]any{"session_id": "fresh-chat", "is_thread": true, "parent_agent_id": "root-agent", "message_id": id, "role": "user", "seq": 100})
}
func nativeDone(text, status string) SubscriptionRecord {
	return nativeRecord("delta.message_done", 120, map[string]any{"session_id": "fresh-chat", "is_thread": true, "parent_agent_id": "root-agent", "agent_id": "root-agent", "message_id": "output-message", "message_seq": 101, "seq": 4, "status": status, "transcript": map[string]any{"agent_id": "root-agent", "message_id": "transcript-container", "status": status, "metadata": map[string]any{"model": "observed-model"}, "messages": []any{map[string]any{"id": "output-message", "role": "assistant", "status": status, "content": []any{map[string]any{"type": "text", "text": text}}}}}})
}
func nativeTask(sequence int64, status string, subagents int) SubscriptionRecord {
	return nativeRecord("task.status", sequence, map[string]any{"session_id": "fresh-chat", "is_thread": true, "agent_id": "root-agent", "root_agent_id": "root-agent", "task_id": "root-agent", "status": status, "active_subagent_count": subagents})
}
func TestNativeTurnReplayedCompletionNeedsNewerMatchingTerminalTask(t *testing.T) {
	var events []Event
	turn := newNativeTurn("fresh-chat", "accepted-user", "requested-model", "durable-reference", "local-operation", func(event Event) error { events = append(events, event); return nil })
	require.NoError(t, turn.record(nativeAck()))
	require.NoError(t, turn.record(nativeTask(90, "completed", 0)))
	require.NoError(t, turn.record(nativeUser("accepted-user")))
	require.NoError(t, turn.record(nativeDone("observed response", "completed")))
	require.Nil(t, turn.result, "a snapshot from before the accepted request cannot settle this output")
	require.NoError(t, turn.record(nativeTask(121, "completed", 1)))
	require.Nil(t, turn.result, "live subagents retain workspace occupancy")
	require.ErrorIs(t, turn.record(nativeTask(122, "completed", 0)), io.EOF)
	require.Equal(t, "observed-model", turn.result.Response.Model)
	require.Nil(t, turn.result.Response.Usage)
	require.Equal(t, "observed response", turn.result.Response.Output[0].Content[0].Text)
	for i, event := range events {
		require.Equal(t, int64(i+1), event.Sequence)
		require.Equal(t, i, event.Data.SequenceNumber)
	}
}

func TestNativeTurnTerminalTaskMayArriveBeforeFinalTranscript(t *testing.T) {
	turn := newNativeTurn("fresh-chat", "accepted-user", "requested-model", "durable-reference", "local-operation", nil)
	require.NoError(t, turn.record(nativeAck()))
	require.NoError(t, turn.record(nativeUser("accepted-user")))
	require.NoError(t, turn.record(nativeTask(110, "interrupted", 0)))
	require.True(t, turn.confirmedInterruption())
	require.Nil(t, turn.result, "a result still requires its authoritative transcript")
	require.ErrorIs(t, turn.record(nativeDone("partial output", "interrupted")), io.EOF)
	require.Equal(t, "incomplete", turn.result.Response.Status)
}
func TestNativeTurnStreamsOrderedDeltasAndRejectsContradictoryRevision(t *testing.T) {
	var text string
	turn := newNativeTurn("fresh-chat", "accepted-user", "requested-model", "durable-reference", "local-operation", func(event Event) error {
		if event.Data.Type == "response.output_text.delta" {
			text += event.Data.Delta
		}
		return nil
	})
	require.NoError(t, turn.record(nativeAck()))
	require.NoError(t, turn.record(nativeUser("accepted-user")))
	payload := func(version int64, value string) map[string]any {
		return map[string]any{"session_id": "fresh-chat", "is_thread": true, "parent_agent_id": "root-agent", "agent_id": "root-agent", "message_id": "output-message", "message_seq": 101, "seq": version, "text": value}
	}
	require.NoError(t, turn.record(nativeRecord("delta.message_start", 101, payload(1, ""))))
	later := nativeRecord("delta.text_append", 103, payload(3, " world"))
	require.NoError(t, turn.record(later))
	require.Empty(t, text)
	require.NoError(t, turn.record(nativeRecord("delta.text_append", 102, payload(2, "Hello"))))
	require.Equal(t, "Hello world", text)
	require.NoError(t, turn.record(later))
	require.Equal(t, "Hello world", text)
	require.NoError(t, turn.record(nativeDone("Changed response", "completed")))
	require.ErrorIs(t, turn.record(nativeTask(121, "completed", 0)), ErrOwnerReview)
	require.Nil(t, turn.result)
}
func TestNativeTurnExcludesOtherChatsAndProactiveCanonicalMessages(t *testing.T) {
	turn := newNativeTurn("fresh-chat", "accepted-user", "requested-model", "durable-reference", "local-operation", nil)
	require.NoError(t, turn.record(nativeAck()))
	require.NoError(t, turn.record(nativeUser("accepted-user")))
	other := nativeRecord("delta.message_done", 119, map[string]any{"session_id": "another-chat", "is_thread": true, "message_id": "unrelated", "agent_id": "root-agent"})
	require.NoError(t, turn.record(other))
	require.Nil(t, turn.message)
	proactive := nativeRecord("delta.message_start", 119, map[string]any{"session_id": "fresh-chat", "is_thread": true, "parent_agent_id": "root-agent", "agent_id": "root-agent", "message_id": "proactive", "message_seq": 102, "seq": 1})
	require.NoError(t, turn.record(proactive))
	require.Nil(t, turn.message)
	require.ErrorIs(t, turn.record(nativeUser("another-user-message")), ErrOwnerReview)
}
func TestNativeTurnApprovalNeverAutoApprovesOrReturnsSuccess(t *testing.T) {
	turn := newNativeTurn("fresh-chat", "accepted-user", "requested-model", "durable-reference", "local-operation", nil)
	require.NoError(t, turn.record(nativeAck()))
	require.ErrorIs(t, turn.record(nativeRecord("approvals.snapshot", 110, map[string]any{"pending_approvals": []any{map[string]any{"id": "approval"}}})), ErrOwnerReview)
	require.Nil(t, turn.result)
}
func TestNativeInputRejectsUnrepresentableSemantics(t *testing.T) {
	for _, raw := range []string{
		`{"model":"observed-model","input":[{"role":"user","content":"one"},{"role":"assistant","content":"two"}]}`,
		`{"model":"observed-model","input":"hello","max_output_tokens":32}`,
		`{"model":"observed-model","input":"hello","instructions":"special"}`,
		`{"model":"observed-model","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"type":"ephemeral"}}]}]}`,
		`{"model":"observed-model","input":"hello","previous_response_id":"old"}`,
	} {
		var request apicompat.ResponsesRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		_, err := nativeInput(&request)
		require.Error(t, err)
	}
	var request apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"observed-model","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`), &request))
	text, err := nativeInput(&request)
	require.NoError(t, err)
	require.Equal(t, "hello", text)
}
