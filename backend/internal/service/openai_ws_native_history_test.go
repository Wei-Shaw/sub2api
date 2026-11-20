package service

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeHistoryItems(raw ...string) []json.RawMessage {
	items := make([]json.RawMessage, len(raw))
	for i := range raw {
		items[i] = json.RawMessage(raw[i])
	}
	return items
}

func nativeHistoryResult(id string, items []json.RawMessage) *OpenAIForwardResult {
	if items == nil {
		items = []json.RawMessage{}
	}
	return &OpenAIForwardResult{RequestID: id, UpstreamTerminalEvent: "response.completed", wsAccountFailoverReplayInput: items}
}

func TestOpenAIWSNativeHistoryCompletedOutput(t *testing.T) {
	require.Nil(t, openAIWSNativeCompletedOutput([]byte(`{"response":{"id":"resp_missing_output"}}`)))
	require.Nil(t, openAIWSNativeCompletedOutput([]byte(`{"response":{"output":[null]}}`)))
	require.NotNil(t, openAIWSNativeCompletedOutput([]byte(`{"response":{"output":[]}}`)))
	payload := []byte(`{"response":{"output":[{"type":"reasoning","id":"first"},{"type":"message","id":"second"}]}}`)
	items := openAIWSNativeCompletedOutput(payload)
	require.Len(t, items, 2)
	require.Equal(t, "first", gjson.GetBytes(items[0], "id").String())
	clear(payload)
	require.True(t, json.Valid(items[0]), "cache must own its body independently of the wire buffer")
	h := (&OpenAIGatewayService{}).newOpenAIWSNativeHistory()
	defer h.close()
	h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
	h.commit(&OpenAIForwardResult{RequestID: "resp_partial", UpstreamTerminalEvent: "response.completed"})
	require.False(t, h.complete)
}

func TestOpenAIWSNativeHistoryCompleteReplay(t *testing.T) {
	s := &OpenAIGatewayService{}
	h := s.newOpenAIWSNativeHistory()
	defer h.close()
	root := nativeHistoryItems(`{"type":"message","role":"user","content":"window-67"}`)
	h.prepare(root, true, "")
	outputs := nativeHistoryItems(
		`{"type":"reasoning","id":"rs_1","encrypted_content":"ciphertext","summary":[]}`,
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"working"}]}`,
		`{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":"patch"}`,
	)
	h.commit(nativeHistoryResult("resp_1", outputs))
	oldBody := h.items[0]
	delta := nativeHistoryItems(`{"type":"custom_tool_call_output","call_id":"call_1","output":"done"}`)
	h.prepare(delta, true, "resp_1")
	require.True(t, h.ready)
	require.Same(t, &oldBody[0], &h.items[0][0], "owned history bodies must be shared")
	require.NotSame(t, &delta[0][0], &h.items[4][0], "new input must not pin the request buffer")
	source := []byte(`{"type":"response.create","model":"mapped-model","previous_response_id":"resp_1","prompt_cache_key":"client-key","client_metadata":{"x-codex-window-id":"67"},"input":[]}`)
	retry, safe, err := h.retryPayload(source, "original-model")
	require.NoError(t, err)
	require.True(t, safe)
	require.False(t, gjson.GetBytes(retry, "previous_response_id").Exists())
	require.Equal(t, "original-model", gjson.GetBytes(retry, "model").String())
	require.Equal(t, "client-key", gjson.GetBytes(retry, "prompt_cache_key").String())
	require.Equal(t, int64(5), gjson.GetBytes(retry, "input.#").Int())
	require.Equal(t, "ciphertext", gjson.GetBytes(retry, "input.1.encrypted_content").String())
	require.Contains(t, string(retry), "custom_tool_call_output")
	require.Equal(t, "resp_1", gjson.GetBytes(source, "previous_response_id").String())
	// A second account's failure must replay exactly the same current request.
	h2 := s.newOpenAIWSNativeHistory()
	defer h2.close()
	items, exists, err := openAIWSExtractNormalizedInputSequence(retry)
	require.NoError(t, err)
	h2.prepare(items, exists, "")
	retry2, safe, err := h2.retryPayload(retry, "original-model")
	require.NoError(t, err)
	require.True(t, safe)
	require.JSONEq(t, string(retry), string(retry2))

	// Compaction replaces the window, including assistant and encrypted history.
	h.prepare(nativeHistoryItems(`{"type":"message","role":"user","content":"summary-window-68"}`), true, "")
	require.Len(t, h.items, 1)
	require.NotContains(t, string(h.items[0]), "67")
	require.Less(t, h.reserved, int64(256))
}

func TestOpenAIWSNativeHistoryFullPrefixAndEmptyDelta(t *testing.T) {
	h := (&OpenAIGatewayService{}).newOpenAIWSNativeHistory()
	defer h.close()
	root := nativeHistoryItems(`{"role":"user","content":"hi"}`)
	h.prepare(root, true, "")
	h.commit(nativeHistoryResult("resp_1", nativeHistoryItems(`{"type":"message","role":"assistant","content":"hello"}`)))
	full := combineOpenAIWSReplayItems(h.items, nativeHistoryItems(`{"role":"user","content":"next"}`))
	h.prepare(full, true, "resp_1")
	require.Len(t, h.items, 3)
	h.commit(nativeHistoryResult("resp_2", nil))
	h.prepare(nil, true, "resp_2")
	require.Len(t, h.items, 3)
	require.True(t, h.ready)
}

func TestOpenAIWSNativeHistoryFailClosed(t *testing.T) {
	for _, scenario := range []string{"unknown_chain", "missing_input", "incomplete", "failed", "missing_response_id", "orphan_output", "item_reference", "shorthand_input", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			s := &OpenAIGatewayService{}
			h := s.newOpenAIWSNativeHistory()
			defer h.close()
			h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
			switch scenario {
			case "unknown_chain":
				h.prepare(nil, true, "resp_unknown")
			case "missing_input":
				h.prepare(nil, false, "")
			case "incomplete", "failed":
				r := nativeHistoryResult("resp_1", nil)
				r.UpstreamTerminalEvent = "response." + scenario
				h.commit(r)
			case "missing_response_id":
				h.commit(nativeHistoryResult("", nil))
			case "orphan_output":
				h.prepare(nativeHistoryItems(`{"type":"function_call_output","call_id":"missing","output":"x"}`), true, "")
			case "item_reference":
				h.prepare(nativeHistoryItems(`{"type":"item_reference","id":"msg_remote"}`), true, "")
			case "shorthand_input":
				h.prepare(nativeHistoryItems(`"hello"`), true, "")
			case "closed":
				h.close()
			}
			_, safe, err := h.retryPayload([]byte(`{"type":"response.create","input":[]}`), "m")
			require.NoError(t, err)
			require.False(t, safe)
			h.close()
			require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
		})
	}
}

func TestOpenAIWSNativeHistoryBudgets(t *testing.T) {
	for _, limit := range []string{"session", "items", "disabled", "disabled_items"} {
		t.Run(limit, func(t *testing.T) {
			s := &OpenAIGatewayService{}
			h := s.newOpenAIWSNativeHistory()
			defer h.close()
			h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
			h.commit(nativeHistoryResult("resp_1", nil))
			switch limit {
			case "session":
				h.maxSession = 100
			case "items":
				h.maxItems = 1
			case "disabled":
				h.maxSession = 0
			case "disabled_items":
				h.maxItems = 0
			}
			h.prepare(nativeHistoryItems(`{"role":"user","content":"next"}`), true, "resp_1")
			require.False(t, h.ready)
			require.Empty(t, h.items)
			require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
			h.maxSession, h.maxItems = 1024, 8
			h.prepare(nativeHistoryItems(`{"role":"user","content":"fresh"}`), true, "")
			require.True(t, h.ready, "a later full root can recover caching")
		})
	}
}

func TestOpenAIWSNativeHistorySessionLimitBoundary(t *testing.T) {
	s := &OpenAIGatewayService{}
	h := s.newOpenAIWSNativeHistory()
	defer h.close()
	require.EqualValues(t, 64<<20, h.maxSession)
	require.Equal(t, 32768, h.maxItems)
	root := nativeHistoryItems(`{"role":"user","content":"hi"}`)
	h.maxSession = int64(len(root[0])) + openAIWSNativeHistoryItemOverhead
	h.prepare(root, true, "")
	require.True(t, h.ready, "exactly the byte limit is allowed")
	require.Equal(t, h.maxSession, h.reserved)

	h.prepare(nativeHistoryItems(string(root[0])+" "), true, "")
	require.False(t, h.ready, "one byte over the limit must release history")
	require.Equal(t, "session_budget", h.reason)
	require.Nil(t, h.items)
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())

	h.prepare(root, true, "")
	require.True(t, h.ready, "a smaller full root can restart caching")
	h.commit(nativeHistoryResult("resp_overflow", nativeHistoryItems(`{"type":"message","role":"assistant","content":"hello"}`)))
	require.False(t, h.complete, "upstream output also counts toward the limit")
	require.Equal(t, "session_budget", h.reason)
	require.Nil(t, h.items)
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
	h.prepare(root, true, "resp_overflow")
	require.False(t, h.ready, "a delta cannot recover discarded history")
}

func TestOpenAIWSNativeHistoryConcurrentAccounting(t *testing.T) {
	s := &OpenAIGatewayService{}
	const reservation = int64(5 << 20)
	histories := make([]*openAIWSNativeHistory, 32)
	for i := range histories {
		h := s.newOpenAIWSNativeHistory()
		histories[i] = h
		t.Cleanup(h.close)
		// Exercise accounting without allocating 160 MiB in the regular suite.
		require.True(t, h.resize(reservation))
	}
	require.Equal(t, int64(len(histories))*reservation, s.openaiWSNativeHistoryBytes.Load(),
		"aggregate accounting must not impose a shared admission limit")
	var wg sync.WaitGroup
	for _, h := range histories {
		wg.Go(func() {
			defer h.close()
			for range 100 {
				if !h.resize(1) || !h.resize(reservation) {
					t.Error("another connection must not prevent a reservation")
					return
				}
			}
		})
	}
	wg.Wait()
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
}

// Compare the existing native input-replay maintenance with that same work plus
// the new cache. This excludes networking, model inference and the rest of ingress.
// One operation is a 128-turn conversation, not a single model request.
func BenchmarkOpenAIWSNativeHistory(b *testing.B) {
	for _, size := range []int{1024, 10 * 1024} {
		var inputs, terminals [128][]byte
		for turn := range inputs {
			inputs[turn] = []byte(fmt.Sprintf(`{"input":[{"role":"user","content":"%d:%s"}]}`, turn, strings.Repeat("u", size)))
			terminals[turn] = []byte(fmt.Sprintf(`{"response":{"output":[{"type":"message","role":"assistant","content":"%d:%s"}]}}`, turn, strings.Repeat("a", size)))
		}
		for _, mode := range []string{"before_existing_replay", "after_replay_and_history"} {
			b.Run(fmt.Sprintf("%dB/%s", size, mode), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var h *openAIWSNativeHistory
					if mode == "after_replay_and_history" {
						h = (&OpenAIGatewayService{}).newOpenAIWSNativeHistory()
					}
					var legacy []json.RawMessage
					var legacyExists bool
					prev := ""
					for turn, payload := range inputs {
						items, exists, err := openAIWSExtractNormalizedInputSequence(payload)
						if err != nil {
							b.Fatal(err)
						}
						legacy, legacyExists = buildOpenAIWSReplayInputSequenceFromItems(legacy, legacyExists, items, exists, prev != "")
						if h != nil {
							h.prepare(items, exists, prev)
							h.commit(nativeHistoryResult("resp", openAIWSNativeCompletedOutput(terminals[turn])))
						}
						prev = "resp"
					}
					if h != nil {
						b.ReportMetric(float64(h.reserved), "added-cache-B/session")
						h.close()
					} else {
						b.ReportMetric(0, "added-cache-B/session")
					}
					runtime.KeepAlive(legacy)
				}
			})
		}
	}
}

func BenchmarkOpenAIWSNativeHistoryRebuild4MiB(b *testing.B) {
	h := (&OpenAIGatewayService{}).newOpenAIWSNativeHistory()
	defer h.close()
	items := make([]json.RawMessage, 128)
	for i := range items {
		items[i] = json.RawMessage(`{"role":"user","content":"` + strings.Repeat("x", 32*1024) + `"}`)
	}
	h.prepare(items, true, "")
	source := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_old","input":[]}`)
	b.ReportAllocs()
	b.SetBytes(4 << 20)
	for b.Loop() {
		_, safe, err := h.retryPayload(source, "gpt-5.1")
		if !safe || err != nil {
			b.Fatal("rebuild failed", err)
		}
	}
}

func TestOpenAIWSNativeHistoryMemoryPressure(t *testing.T) {
	if os.Getenv("SUB2_HISTORY_MEMORY_TEST") != "1" {
		t.Skip("opt-in memory pressure measurement")
	}
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("native_cache=%t", enabled), func(t *testing.T) {
			const sessions = 64
			const bodySize = 4 << 20
			s := &OpenAIGatewayService{}
			runtime.GC()
			var before, peak, after runtime.MemStats
			runtime.ReadMemStats(&before)
			// Independent roots model the existing replay retention in both cases.
			roots := make([][]json.RawMessage, sessions)
			histories := make([]*openAIWSNativeHistory, sessions)
			for i := range roots {
				roots[i] = nativeHistoryItems(`{"role":"user","content":"` + strings.Repeat("x", bodySize) + `"}`)
				if enabled {
					histories[i] = s.newOpenAIWSNativeHistory()
					histories[i].prepare(roots[i], true, "")
					require.True(t, histories[i].ready)
				}
			}
			runtime.GC()
			runtime.ReadMemStats(&peak)
			reserved := s.openaiWSNativeHistoryBytes.Load()
			if enabled {
				require.Greater(t, reserved, int64(256<<20))
			} else {
				require.Zero(t, reserved)
			}
			runtime.KeepAlive(roots)
			for _, h := range histories {
				if h != nil {
					h.close()
				}
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
			t.Logf("sessions=%d reserved_bytes=%d heap_before=%d heap_peak=%d heap_after_close=%d",
				sessions, reserved, before.HeapAlloc, peak.HeapAlloc, after.HeapAlloc)
			runtime.KeepAlive(histories)
			require.Less(t, after.HeapAlloc, before.HeapAlloc+(8<<20))
		})
	}
}

func TestOpenAIWSNativeHistoryLargeSession(t *testing.T) {
	if os.Getenv("SUB2_HISTORY_MEMORY_TEST") != "1" {
		t.Skip("opt-in large session measurement")
	}
	s := &OpenAIGatewayService{}
	h := s.newOpenAIWSNativeHistory()
	defer h.close()
	body := strings.Repeat("x", 4<<20)
	previous := ""
	for turn := range 15 {
		input := nativeHistoryItems(fmt.Sprintf(`{"role":"user","content":"%s","id":"msg_%d"}`, body, turn))
		h.prepare(input, true, previous)
		require.True(t, h.ready, "turn %d must fit below 64 MiB", turn)
		previous = fmt.Sprintf("resp_%d", turn)
		h.commit(nativeHistoryResult(previous, nil))
	}
	require.Greater(t, h.reserved, int64(60<<20))
	t.Logf("retained_before_overflow=%d session_limit=%d", h.reserved, h.maxSession)
	h.prepare(nativeHistoryItems(fmt.Sprintf(`{"role":"user","content":"%s","id":"msg_15"}`, body)), true, previous)
	require.Equal(t, "session_budget", h.reason)
	require.False(t, h.complete)
	require.Nil(t, h.items)
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
	h.prepare(nativeHistoryItems(`{"role":"user","content":"new window"}`), true, "")
	require.True(t, h.ready, "a fresh window must recover after the oversized session")
}
