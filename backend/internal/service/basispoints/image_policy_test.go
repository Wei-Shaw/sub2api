package basispoints

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func policyImageMessage(n int) object {
	parts := []any{}
	for i := 0; i < n; i++ {
		parts = append(parts, object{"type": "input_image", "image_url": "data:image/png;base64,synthetic"})
	}
	return object{"role": "user", "content": parts}
}
func policyHistory(t *testing.T, items ...any) *ImageHistory {
	t.Helper()
	raw, e := json.Marshal(object{"model": "test-model", "input": items})
	require.NoError(t, e)
	h, e := InspectImageHistory(raw)
	require.NoError(t, e)
	return h
}
func TestImagePolicyHistorySplit(t *testing.T) {
	old := policyHistory(t, policyImageMessage(19))
	snapshot := old.Progress()
	h := policyHistory(t, old.Input[0], message("assistant", "seen"), policyImageMessage(2))
	require.Equal(t, 21, h.Count)
	split, e := h.Split(&snapshot)
	require.NoError(t, e)
	require.Equal(t, 1, split)
	require.Equal(t, 2, CountInlineImages(h.Input[split:]))
	split, e = h.Split(nil)
	require.NoError(t, e)
	require.Equal(t, 2, split)
	unrelated := ImageProgress{Count: 1, Digest: "other"}
	split, e = h.Split(&unrelated)
	require.NoError(t, e)
	require.Equal(t, 2, split)
	raw, e := h.WithInput(h.Input[:split], true)
	require.NoError(t, e)
	require.Contains(t, string(raw), "compaction_trigger")
	require.Contains(t, string(raw), "none")
}
func TestImagePolicyToolTail(t *testing.T) {
	for _, kind := range []string{"function", "custom_tool"} {
		t.Run(kind, func(t *testing.T) {
			call := object{"type": kind + "_call", "call_id": "call-a"}
			out := object{"type": kind + "_call_output", "call_id": "call-a", "output": policyImageMessage(2)["content"]}
			h := policyHistory(t, policyImageMessage(19), message("assistant", "history"), call, out)
			split, e := h.Split(nil)
			require.NoError(t, e)
			require.Equal(t, 2, split)
			require.Equal(t, 2, CountInlineImages(h.Input[split:]))
			_, e = policyHistory(t, policyImageMessage(19), out).Split(nil)
			require.Error(t, e)
		})
	}
}
func TestImagePolicyCountsOnlyTypedInlineContent(t *testing.T) {
	h := policyHistory(t, object{"type": "agent_message", "content": policyImageMessage(2)["content"]}, object{"type": "function_call", "arguments": policyImageMessage(5)}, object{"role": "user", "content": []any{object{"type": "input_image", "image_url": "https://example.com/image.png"}, object{"type": "input_image", "file_id": "file-existing"}}})
	require.Equal(t, 2, h.Count)
}
func TestImagePolicyStreamWindowAndUsage(t *testing.T) {
	compact := object{"type": "compaction", "id": "cmp-test", "encrypted_content": "opaque-state"}
	resp := object{"status": "completed", "output": []any{compact}}
	window, _, e := CompactWindow(resp)
	require.NoError(t, e)
	events := []object{
		{"type": "response.created", "response": object{"id": "resp-final"}},
		{"type": "response.output_item.added", "output_index": 0, "item": object{"type": "message", "id": "msg-answer"}},
		{"type": "response.output_text.delta", "output_index": 0, "delta": "answer"},
		{"type": "response.completed", "response": object{"id": "resp-final", "status": "completed", "output": []any{object{"type": "message", "id": "msg-answer"}}, "usage": object{"input_tokens": 10, "output_tokens": 2, "total_tokens": 12}}},
	}
	var wire strings.Builder
	for _, event := range events {
		b, _ := json.Marshal(event)
		fmt.Fprintf(&wire, "data: %s\n\n", b)
	}
	stream := WithCompactedWindow(context.Background(), io.NopCloser(strings.NewReader(wire.String())), window, object{"input_tokens": json.Number("7"), "output_tokens": json.Number("3"), "total_tokens": json.Number("10")})
	defer func() { _ = stream.Close() }()
	out, e := io.ReadAll(stream)
	require.NoError(t, e)
	var got []object
	require.NoError(t, readEvents(strings.NewReader(string(out)), func(_ string, b []byte) error {
		var event object
		require.NoError(t, decode(b, &event))
		got = append(got, event)
		return nil
	}))
	require.Len(t, got, 6)
	for i, v := range got {
		require.Equal(t, fmt.Sprint(i), fmt.Sprint(v["sequence_number"]))
	}
	item, ok := got[1]["item"].(object)
	require.True(t, ok)
	require.Equal(t, "compaction", item["type"])
	require.Equal(t, json.Number("1"), got[3]["output_index"])
	final, ok := got[5]["response"].(object)
	require.True(t, ok)
	require.Len(t, final["output"], 2)
	finalUsage, ok := final["usage"].(object)
	require.True(t, ok)
	require.Equal(t, json.Number("17"), finalUsage["input_tokens"])
}
func TestImagePolicyRejectsFakeCompact(t *testing.T) {
	for _, r := range []object{{"status": "failed"}, {"status": "completed", "output": []any{message("assistant", "summary")}}, {"status": "completed", "output": []any{object{"type": "compaction", "encrypted_content": "opaque"}, object{"type": "function_call", "call_id": "unsafe"}}}} {
		_, _, e := CompactWindow(r)
		require.Error(t, e)
	}
}

func TestImagePolicyProgressiveUsageAndCancellation(t *testing.T) {
	for _, terminal := range []string{"response.completed", "response.failed", "response.incomplete"} {
		t.Run(terminal, func(t *testing.T) {
			wire := "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n" + "data: {\"type\":\"" + terminal + "\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":0}}}\n\n"
			stream := WithCompactedWindow(context.Background(), io.NopCloser(strings.NewReader(wire)), []any{object{"type": "compaction", "encrypted_content": "opaque"}}, object{"input_tokens": json.Number("7"), "output_tokens": json.Number("3")})
			defer func() { _ = stream.Close() }()
			var last object
			require.NoError(t, readEvents(stream, func(_ string, b []byte) error { require.NoError(t, decode(b, &last)); return nil }))
			response, ok := last["response"].(object)
			require.True(t, ok)
			usage, ok := response["usage"].(object)
			require.True(t, ok)
			require.Equal(t, json.Number("17"), usage["input_tokens"])
			require.Equal(t, json.Number("5"), usage["output_tokens"])
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	r, w := io.Pipe()
	stream := WithCompactedWindow(ctx, r, nil, nil)
	cancel()
	_, err := io.ReadAll(stream)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, stream.Close())
	_ = w.Close()
}
func TestImagePolicyCompactionReadFailuresAndUsage(t *testing.T) {
	prefix := "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":7}}}\n\n"
	for _, terminal := range []string{"response.completed", "response.failed", "response.incomplete"} {
		response, e := ReadImageCompaction(strings.NewReader(prefix+"data: {\"type\":\""+terminal+"\",\"response\":{\"output\":[]}}\n\n"), nil)
		if terminal == "response.completed" {
			require.NoError(t, e)
		} else {
			require.Error(t, e)
		}
		usage, ok := response["usage"].(object)
		require.True(t, ok)
		require.Equal(t, json.Number("7"), usage["input_tokens"])
	}
	for _, wire := range []string{prefix, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"unsafe\"}}\n\n"} {
		_, e := ReadImageCompaction(strings.NewReader(wire), nil)
		require.Error(t, e)
	}
}

func TestImagePolicyReconcilePreservesNewInputsAndMultipleCycles(t *testing.T) {
	h := policyHistory(t, policyImageMessage(19), message("assistant", "seen"), policyImageMessage(2))
	marker := object{"type": "compaction", "encrypted_content": "state-one"}
	cp, e := h.Checkpoint(2, []any{marker})
	require.NoError(t, e)
	original := append([]any{}, h.Input...)
	h.Input = append(h.Input, marker, message("assistant", "continued"), policyImageMessage(1))
	changed, e := h.Reconcile([]ImageCheckpoint{cp})
	require.NoError(t, e)
	require.True(t, changed)
	require.Equal(t, 3, h.Count)
	require.Equal(t, original[2], h.Input[1])
	canonical := append([]any{}, h.Input...)
	marker2 := object{"type": "compaction", "encrypted_content": "state-two"}
	cp2, e := h.Checkpoint(3, []any{marker2})
	require.NoError(t, e)
	raw := policyHistory(t, append(append(append([]any{}, original...), marker, message("assistant", "continued"), policyImageMessage(1)), marker2, message("assistant", "again"))...)
	changed, e = raw.Reconcile([]ImageCheckpoint{cp, cp2})
	require.NoError(t, e)
	require.True(t, changed)
	require.Equal(t, 1, raw.Count)
	require.Equal(t, canonical[3], raw.Input[1])
	encoded, _ := json.Marshal([]ImageCheckpoint{cp, cp2})
	require.NotContains(t, string(encoded), "state-one")
	require.NotContains(t, string(encoded), "base64")
}
func TestImagePolicyReconcileRejectsChangedWindowAndHandlesRetries(t *testing.T) {
	h := policyHistory(t, policyImageMessage(19), message("assistant", "seen"), policyImageMessage(2))
	marker := object{"type": "compaction", "encrypted_content": "authentic"}
	cp, e := h.Checkpoint(2, []any{marker})
	require.NoError(t, e)
	changed, e := h.Reconcile([]ImageCheckpoint{cp})
	require.NoError(t, e)
	require.False(t, changed)
	h.Input = append(h.Input, object{"type": "compaction", "encrypted_content": "tampered"})
	_, e = h.Reconcile([]ImageCheckpoint{cp})
	require.Error(t, e)
	// A client that has already replaced its history is left untouched.
	normalized := policyHistory(t, marker, policyImageMessage(2))
	changed, e = normalized.Reconcile([]ImageCheckpoint{cp})
	require.NoError(t, e)
	require.False(t, changed)
	require.Equal(t, 2, normalized.Count)
}
