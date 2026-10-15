package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSStrictWireNormalizerKeepsMessageLifecycleConsistent(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	added, changed, err := normalizer.normalize([]byte(`{
		"type":"response.output_item.added",
		"output_index":0,
		"item":{"type":"message","role":"assistant","phase":"final_answer","content":[]}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	messageID := gjson.GetBytes(added, "item.id").String()
	require.True(t, strings.HasPrefix(messageID, "msg_"))
	require.Equal(t, "in_progress", gjson.GetBytes(added, "item.status").String())

	done, changed, err := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"type":"message","status":"completed","phase":"final_answer","role":"assistant","content":[{"type":"output_text","text":"MATRIX_TEXT_OK","annotations":[],"logprobs":[]}]}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, messageID, gjson.GetBytes(done, "item.id").String())

	completed, changed, err := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"MATRIX_TEXT_OK"}],"vendor_extension":{"keep":true}}]}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, messageID, gjson.GetBytes(completed, "response.output.0.id").String())
	require.Equal(t, "completed", gjson.GetBytes(completed, "response.output.0.status").String())
	require.Equal(t, "final_answer", gjson.GetBytes(completed, "response.output.0.phase").String())
	require.Equal(t, "MATRIX_TEXT_OK", gjson.GetBytes(completed, "response.output.0.content.0.text").String())
	require.True(t, gjson.GetBytes(completed, "response.output.0.content.0.annotations").IsArray())
	require.True(t, gjson.GetBytes(completed, "response.output.0.content.0.logprobs").IsArray())
	require.True(t, gjson.GetBytes(completed, "response.output.0.vendor_extension.keep").Bool())
}

func TestOpenAIWSStrictWireNormalizerRepairsFunctionCallTerminalItem(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	done, changed, err := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"id":"fc_1","type":"function_call","status":"completed","call_id":"call_1","name":"lookup_order_status","arguments":"{}"}
	}`))
	require.NoError(t, err)
	require.False(t, changed, "a complete item needs no repair")
	require.Equal(t, "fc_1", gjson.GetBytes(done, "item.id").String())

	completed, changed, err := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"lookup_order_status","arguments":"{}"}]}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "fc_1", gjson.GetBytes(completed, "response.output.0.id").String())
	require.Equal(t, "completed", gjson.GetBytes(completed, "response.output.0.status").String())
	require.Equal(t, "lookup_order_status", gjson.GetBytes(completed, "response.output.0.name").String())
}

func TestOpenAIWSStrictWireNormalizerDoesNotTouchUnrelatedEvents(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()
	raw := []byte(`{"type":"response.output_text.delta","output_index":0,"delta":"hello"}`)
	got, changed, err := normalizer.normalize(raw)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(raw), string(got))
}

func TestOpenAIWSStrictWireNormalizerMapsTerminalResponseStatusToItemStatus(t *testing.T) {
	for _, tc := range []struct {
		name           string
		eventType      string
		responseStatus string
		wantStatus     string
	}{
		{name: "failed", eventType: "response.failed", responseStatus: "failed", wantStatus: "incomplete"},
		{name: "cancelled", eventType: "response.cancelled", responseStatus: "cancelled", wantStatus: "incomplete"},
		{name: "incomplete", eventType: "response.incomplete", responseStatus: "incomplete", wantStatus: "incomplete"},
		{name: "completed", eventType: "response.completed", responseStatus: "completed", wantStatus: "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			normalizer := newOpenAIWSStrictWireNormalizer()
			event := []byte(`{
				"type":"` + tc.eventType + `",
				"response":{"status":"` + tc.responseStatus + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]}
			}`)
			normalized, changed, err := normalizer.normalize(event)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, tc.wantStatus, gjson.GetBytes(normalized, "response.output.0.status").String())
		})
	}
}

func TestOpenAIWSStrictWireNormalizerKeepsFinishedItemStatusOnFailedResponse(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	_, changed, err := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[],"logprobs":[]}]}
	}`))
	require.NoError(t, err)
	require.False(t, changed, "a complete item needs no repair")

	failed, changed, err := normalizer.normalize([]byte(`{
		"type":"response.failed",
		"response":{"status":"failed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "msg_1", gjson.GetBytes(failed, "response.output.0.id").String())
	require.Equal(t, "completed", gjson.GetBytes(failed, "response.output.0.status").String(),
		"an item that already reported completed must not be downgraded by the response status")
}

func TestOpenAIWSStrictWireNormalizerLeavesUnknownResponseStatusAlone(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	normalized, _, err := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"some_vendor_state","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}
	}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(normalized, "response.output.0.status").Exists(),
		"an unmappable response status must not be copied onto the item")
}

// A terminal rebuild can drop items, so the array position of an item in
// response.output need not be the output_index of the lifecycle events reported
// there. Remembered state must not leak across that mismatch.
func TestOpenAIWSStrictWireNormalizerDoesNotLeakStateAcrossItemTypes(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	_, _, err := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}],"encrypted_content":"secret"}
	}`))
	require.NoError(t, err)
	_, _, err = normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":1,
		"item":{"id":"msg_1","type":"message","status":"completed","phase":"final_answer","role":"assistant","content":[{"type":"output_text","text":"hi","annotations":[],"logprobs":[]}]}
	}`))
	require.NoError(t, err)

	// The terminal event kept only one message, now sitting at index 0 where
	// the reasoning item used to be.
	completed, _, err := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}
	}`))
	require.NoError(t, err)

	terminal := gjson.GetBytes(completed, "response.output.0")
	require.Equal(t, "message", terminal.Get("type").String())
	require.NotEqual(t, "rs_1", terminal.Get("id").String(),
		"a message must not inherit the reasoning item's id")
	require.True(t, strings.HasPrefix(terminal.Get("id").String(), "msg_"))
	require.False(t, terminal.Get("summary").Exists(),
		"the reasoning item's summary must not leak onto a message")
	require.False(t, terminal.Get("encrypted_content").Exists(),
		"the reasoning item's encrypted_content must not leak onto a message")
}

// Two items of the same type may both appear in one turn. If the terminal
// rebuild drops or reorders one of them the array position no longer identifies
// which lifecycle item it came from, so nothing may be inherited on position
// alone.
func TestOpenAIWSStrictWireNormalizerDoesNotConfuseTwoFunctionCalls(t *testing.T) {
	newTurn := func(t *testing.T) *openAIWSStrictWireNormalizer {
		t.Helper()
		normalizer := newOpenAIWSStrictWireNormalizer()
		for _, event := range []string{
			`{"type":"response.output_item.done","output_index":0,
			  "item":{"id":"fc_first","type":"function_call","status":"completed","call_id":"call_first","name":"lookup_a","arguments":"{\"a\":1}"}}`,
			`{"type":"response.output_item.done","output_index":1,
			  "item":{"id":"fc_second","type":"function_call","status":"completed","call_id":"call_second","name":"lookup_b","arguments":"{\"b\":2}"}}`,
		} {
			_, _, err := normalizer.normalize([]byte(event))
			require.NoError(t, err)
		}
		return normalizer
	}

	t.Run("call_id identifies the surviving item", func(t *testing.T) {
		normalizer := newTurn(t)
		// fc_first was dropped, so fc_second now sits at terminal index 0.
		completed, _, err := normalizer.normalize([]byte(`{
			"type":"response.completed",
			"response":{"status":"completed","output":[{"type":"function_call","call_id":"call_second","name":"lookup_b","arguments":"{\"b\":2}"}]}
		}`))
		require.NoError(t, err)

		terminal := gjson.GetBytes(completed, "response.output.0")
		require.Equal(t, "fc_second", terminal.Get("id").String(),
			"call_id must identify the item, not its position")
		require.Equal(t, "lookup_b", terminal.Get("name").String())
		require.Equal(t, `{"b":2}`, terminal.Get("arguments").String(),
			"the other call's arguments must not be copied in")
	})

	t.Run("an unidentifiable item inherits nothing", func(t *testing.T) {
		normalizer := newTurn(t)
		// Neither id nor call_id survived, and two function_calls are
		// remembered, so this item cannot be proven to be either of them.
		completed, _, err := normalizer.normalize([]byte(`{
			"type":"response.completed",
			"response":{"status":"completed","output":[{"type":"function_call"}]}
		}`))
		require.NoError(t, err)

		terminal := gjson.GetBytes(completed, "response.output.0")
		require.True(t, strings.HasPrefix(terminal.Get("id").String(), "fc_"))
		require.NotEqual(t, "fc_first", terminal.Get("id").String())
		require.NotEqual(t, "fc_second", terminal.Get("id").String())
		require.False(t, terminal.Get("call_id").Exists(),
			"an unidentifiable item must not adopt another call's call_id")
		require.False(t, terminal.Get("arguments").Exists(),
			"an unidentifiable item must not adopt another call's arguments")
		require.False(t, terminal.Get("name").Exists())
	})
}

// Reasoning items carry encrypted_content and summary, which must never be
// attributed to a different reasoning item.
func TestOpenAIWSStrictWireNormalizerDoesNotConfuseTwoReasoningItems(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()
	for _, event := range []string{
		`{"type":"response.output_item.done","output_index":0,
		  "item":{"id":"rs_first","type":"reasoning","summary":[{"type":"summary_text","text":"first"}],"encrypted_content":"secret_first"}}`,
		`{"type":"response.output_item.done","output_index":1,
		  "item":{"id":"rs_second","type":"reasoning","summary":[{"type":"summary_text","text":"second"}],"encrypted_content":"secret_second"}}`,
	} {
		_, _, err := normalizer.normalize([]byte(event))
		require.NoError(t, err)
	}

	// The terminal list reordered the two reasoning items; position 0 now holds
	// the item that was reported at index 1.
	completed, _, err := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"id":"rs_second","type":"reasoning"},{"type":"reasoning"}]}
	}`))
	require.NoError(t, err)

	first := gjson.GetBytes(completed, "response.output.0")
	require.Equal(t, "rs_second", first.Get("id").String())
	require.Equal(t, "secret_second", first.Get("encrypted_content").String(),
		"an id-identified item must inherit its own encrypted_content")
	require.Equal(t, "second", first.Get("summary.0.text").String())

	second := gjson.GetBytes(completed, "response.output.1")
	require.True(t, strings.HasPrefix(second.Get("id").String(), "rs_"))
	require.NotEqual(t, "rs_first", second.Get("id").String())
	require.NotEqual(t, "rs_second", second.Get("id").String())
	require.False(t, second.Get("encrypted_content").Exists(),
		"an unidentifiable reasoning item must not adopt another item's encrypted_content")
	require.False(t, second.Get("summary").Exists(),
		"an unidentifiable reasoning item must not adopt another item's summary")
}

// A passthrough WebSocket connection serves several turns. State remembered for
// one turn must not be visible to the next, even when the upstream reuses the
// same output_index and item type.
func TestOpenAIWSStrictWireNormalizerDoesNotInheritAcrossTurns(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	runTurn := func(t *testing.T, text string) string {
		t.Helper()
		added, _, err := normalizer.normalize([]byte(`{
			"type":"response.output_item.added",
			"output_index":0,
			"item":{"type":"message","role":"assistant","phase":"final_answer","content":[]}
		}`))
		require.NoError(t, err)
		messageID := gjson.GetBytes(added, "item.id").String()
		require.True(t, strings.HasPrefix(messageID, "msg_"))

		_, _, err = normalizer.normalize([]byte(`{
			"type":"response.output_item.done",
			"output_index":0,
			"item":{"type":"message","status":"completed","phase":"final_answer","role":"assistant","content":[{"type":"output_text","text":"` + text + `","annotations":[],"logprobs":[]}]}
		}`))
		require.NoError(t, err)

		completed, _, err := normalizer.normalize([]byte(`{
			"type":"response.completed",
			"response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"` + text + `"}]}]}
		}`))
		require.NoError(t, err)
		require.Equal(t, messageID, gjson.GetBytes(completed, "response.output.0.id").String())
		require.Equal(t, text, gjson.GetBytes(completed, "response.output.0.content.0.text").String())
		return messageID
	}

	firstID := runTurn(t, "TURN_ONE")

	// The relay resets the normalizer once a turn's terminal frame has been
	// written to the client.
	normalizer.reset()

	secondID := runTurn(t, "TURN_TWO")

	require.NotEqual(t, firstID, secondID,
		"turn 2 must mint its own item id instead of reusing turn 1's")
}

// Without the per-turn reset the second turn would silently adopt the first
// turn's item id at the same output_index.
func TestOpenAIWSStrictWireNormalizerResetClearsRememberedItems(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	added, _, err := normalizer.normalize([]byte(`{
		"type":"response.output_item.added",
		"output_index":0,
		"item":{"type":"message","role":"assistant","content":[]}
	}`))
	require.NoError(t, err)
	firstID := gjson.GetBytes(added, "item.id").String()
	require.NotEmpty(t, normalizer.items)
	require.NotEmpty(t, normalizer.indexIDs)

	normalizer.reset()
	require.Empty(t, normalizer.items)
	require.Empty(t, normalizer.indexIDs)

	completed, _, err := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}
	}`))
	require.NoError(t, err)
	require.NotEqual(t, firstID, gjson.GetBytes(completed, "response.output.0.id").String(),
		"a reset normalizer must not hand out the previous turn's id")
}

func TestGenerateOpenAIWSStrictItemIDIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 128)
	for i := 0; i < 128; i++ {
		id, err := generateOpenAIWSStrictItemID("msg")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(id, "msg_"))
		require.NotContains(t, seen, id)
		seen[id] = struct{}{}
	}
}

// An output_item.added that mints in_progress must not pin the item there: the
// done event and the rebuilt terminal item state their own phase.
func TestOpenAIWSStrictWireNormalizerLifecycleStatusOutranksSnapshot(t *testing.T) {
	n := newOpenAIWSStrictWireNormalizer()
	_, _, err := n.normalize([]byte(`{"type":"response.output_item.added","output_index":0,
		"item":{"type":"message","role":"assistant","content":[]}}`))
	require.NoError(t, err)
	done, _, err := n.normalize([]byte(`{"type":"response.output_item.done","output_index":0,
		"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`))
	require.NoError(t, err)
	require.Equal(t, "completed", gjson.GetBytes(done, "item.status").String(), "done must be completed")

	completed, _, err := n.normalize([]byte(`{"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}}`))
	require.NoError(t, err)
	require.Equal(t, "completed", gjson.GetBytes(completed, "response.output.0.status").String(), "terminal must be completed")
}

// One remembered item may only be claimed by one terminal item. Two same-type
// items that both lack an id and a call_id are indistinguishable, so neither
// may take the memory and both must mint their own id.
func TestOpenAIWSStrictWireNormalizerDoesNotHandOneMemoryToTwoItems(t *testing.T) {
	n := newOpenAIWSStrictWireNormalizer()
	_, _, err := n.normalize([]byte(`{"type":"response.output_item.done","output_index":0,
		"item":{"id":"fc_only","type":"function_call","status":"completed","call_id":"call_only","name":"lookup","arguments":"{\"a\":1}"}}`))
	require.NoError(t, err)

	completed, _, err := n.normalize([]byte(`{"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"function_call"},{"type":"function_call"}]}}`))
	require.NoError(t, err)
	id0 := gjson.GetBytes(completed, "response.output.0.id").String()
	id1 := gjson.GetBytes(completed, "response.output.1.id").String()
	require.NotEqual(t, id0, id1, "two terminal items must not share one id")
}

// An upstream may only reveal call_id on done, after added already minted an id
// without one. The trusted output_index must still keep that id stable.
func TestOpenAIWSStrictWireNormalizerKeepsIDWhenCallIDArrivesLate(t *testing.T) {
	n := newOpenAIWSStrictWireNormalizer()
	added, _, err := n.normalize([]byte(`{"type":"response.output_item.added","output_index":0,
		"item":{"type":"function_call","name":"lookup"}}`))
	require.NoError(t, err)
	addedID := gjson.GetBytes(added, "item.id").String()

	done, _, err := n.normalize([]byte(`{"type":"response.output_item.done","output_index":0,
		"item":{"type":"function_call","status":"completed","call_id":"call_1","name":"lookup","arguments":"{}"}}`))
	require.NoError(t, err)
	require.Equal(t, addedID, gjson.GetBytes(done, "item.id").String(), "added/done at one index must share an id")
}

// A response can settle while an item is still in flight, with no
// output_item.done ever reported for it. The provisional in_progress minted by
// output_item.added must not pin the item there: it has to follow the response
// into a settled status.
func TestOpenAIWSStrictWireNormalizerAdvancesItemsThatNeverReachedDone(t *testing.T) {
	for _, tc := range []struct {
		eventType      string
		responseStatus string
		wantStatus     string
	}{
		{eventType: "response.completed", responseStatus: "completed", wantStatus: "completed"},
		{eventType: "response.failed", responseStatus: "failed", wantStatus: "incomplete"},
		{eventType: "response.incomplete", responseStatus: "incomplete", wantStatus: "incomplete"},
		{eventType: "response.cancelled", responseStatus: "cancelled", wantStatus: "incomplete"},
	} {
		t.Run(tc.responseStatus, func(t *testing.T) {
			normalizer := newOpenAIWSStrictWireNormalizer()
			_, _, err := normalizer.normalize([]byte(`{
				"type":"response.output_item.added",
				"output_index":0,
				"item":{"type":"message","role":"assistant","content":[]}
			}`))
			require.NoError(t, err)

			// No output_item.done for this item; the response settles first.
			terminal, _, err := normalizer.normalize([]byte(`{
				"type":"` + tc.eventType + `",
				"response":{"status":"` + tc.responseStatus + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]}
			}`))
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, gjson.GetBytes(terminal, "response.output.0.status").String(),
				"a provisional in_progress must not survive the response settling")
		})
	}
}

// A memory named outright by one item belongs to that item. An anonymous item
// of the same type must not take it just because it was listed first, which
// would hand both items the same id.
func TestOpenAIWSStrictWireNormalizerReservesExplicitlyNamedMemories(t *testing.T) {
	run := func(t *testing.T, output string) (gjson.Result, gjson.Result) {
		t.Helper()
		normalizer := newOpenAIWSStrictWireNormalizer()
		_, _, err := normalizer.normalize([]byte(`{
			"type":"response.output_item.done",
			"output_index":0,
			"item":{"id":"fc_1","type":"function_call","status":"completed","call_id":"call_1","name":"lookup","arguments":"{\"a\":1}"}
		}`))
		require.NoError(t, err)

		completed, _, err := normalizer.normalize([]byte(`{
			"type":"response.completed",
			"response":{"status":"completed","output":` + output + `}
		}`))
		require.NoError(t, err)
		return gjson.GetBytes(completed, "response.output.0"), gjson.GetBytes(completed, "response.output.1")
	}

	assertDistinct := func(t *testing.T, anonymous, named gjson.Result) {
		t.Helper()
		require.Equal(t, "fc_1", named.Get("id").String())
		require.NotEqual(t, "fc_1", anonymous.Get("id").String(),
			"the anonymous item must not claim a memory an explicit id names")
		require.True(t, strings.HasPrefix(anonymous.Get("id").String(), "fc_"))
		require.False(t, anonymous.Get("call_id").Exists())
		require.False(t, anonymous.Get("arguments").Exists())
		require.False(t, anonymous.Get("name").Exists())
	}

	t.Run("anonymous item listed first", func(t *testing.T) {
		anonymous, named := run(t, `[{"type":"function_call"},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"a\":1}"}]`)
		assertDistinct(t, anonymous, named)
	})

	t.Run("anonymous item listed last", func(t *testing.T) {
		named, anonymous := run(t, `[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"a\":1}"},{"type":"function_call"}]`)
		assertDistinct(t, anonymous, named)
	})

	t.Run("memory named by call_id alone is still reserved", func(t *testing.T) {
		anonymous, named := run(t, `[{"type":"function_call"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"a\":1}"}]`)
		require.Equal(t, "fc_1", named.Get("id").String())
		require.NotEqual(t, "fc_1", anonymous.Get("id").String())
		require.False(t, anonymous.Get("call_id").Exists())
	})
}
