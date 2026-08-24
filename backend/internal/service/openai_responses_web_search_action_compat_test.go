package service

import (
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runWebSearchActionCompat(t *testing.T, input string, maxBufferedBytes int) string {
	return runWebSearchActionCompatWithLimits(t, input, 64*1024, maxBufferedBytes)
}

func runWebSearchActionCompatWithLimits(t *testing.T, input string, maxLineSize, maxBufferedBytes int) string {
	t.Helper()
	body := newOpenAIResponsesWebSearchActionCompatStreamBody(
		io.NopCloser(strings.NewReader(input)),
		maxLineSize,
		maxBufferedBytes,
	)
	t.Cleanup(func() { _ = body.Close() })
	out, err := io.ReadAll(body)
	require.NoError(t, err)
	return string(out)
}

func webSearchCompleted(id string, index int, includeAction bool) string {
	action := ""
	if includeAction {
		action = `,"action":{"type":"search","query":"already-there"}`
	}
	return `event: response.web_search_call.completed` + "\n" +
		`data: {"type":"response.web_search_call.completed","sequence_number":2,"output_index":` + strconv.Itoa(index) + `,"item_id":"` + id + `"` + action + `}` + "\n\n"
}

func webSearchDone(id string, index int, action string) string {
	return `event: response.output_item.done` + "\n" +
		`data: {"type":"response.output_item.done","sequence_number":9,"output_index":` + strconv.Itoa(index) + `,"item":{"type":"web_search_call","id":"` + id + `"` + action + `}}` + "\n\n"
}

func webSearchAdded(id string, index int) string {
	return `event: response.output_item.added` + "\n" +
		`data: {"type":"response.output_item.added","sequence_number":1,"output_index":` + strconv.Itoa(index) + `,"item":{"type":"web_search_call","id":"` + id + `","status":"in_progress"}}` + "\n\n"
}

func TestOpenAIResponsesWebSearchActionCompatEnrichesCompletedWithoutReencodingAction(t *testing.T) {
	action := `,"action":{"type":"search","query":"weather","future_field":{"nested":[1,true]}}`
	input := ": keepalive\n\n" + webSearchCompleted("ws_1", 0, false) + webSearchDone("ws_1", 0, action)

	out := runWebSearchActionCompat(t, input, 1<<20)
	require.True(t, strings.HasPrefix(out, ": keepalive\n\n"))
	require.Less(t, strings.Index(out, "response.web_search_call.completed"), strings.LastIndex(out, "response.output_item.done"))
	require.Contains(t, out, `"future_field":{"nested":[1,true]}`)
	require.Contains(t, out, `"item_id":"ws_1","action":{"type":"search","query":"weather","future_field":{"nested":[1,true]}}`)
}

func TestOpenAIResponsesWebSearchActionCompatEnrichesEarlierOutputItemAdded(t *testing.T) {
	input := webSearchAdded("ws_1", 0) + webSearchCompleted("ws_1", 0, false) + webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"earlier"}`)
	out := runWebSearchActionCompat(t, input, 1<<20)
	added := strings.Index(out, `"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress","action":{"type":"search","query":"earlier"}}`)
	completed := strings.Index(out, `"type":"response.web_search_call.completed","sequence_number":2,"output_index":0,"item_id":"ws_1","action":{"type":"search","query":"earlier"}`)
	done := strings.Index(out, `"type":"response.output_item.done","sequence_number":9`)
	require.GreaterOrEqual(t, added, 0)
	require.Greater(t, completed, added)
	require.Greater(t, done, completed)
}

func TestOpenAIResponsesWebSearchActionCompatCachesExistingAddedActionForLegacyCompleted(t *testing.T) {
	input := `: before` + "\n\n" +
		`event: response.output_item.added` + "\n" +
		`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","action":{"type":"search","future":{"x":1}}}}` + "\n\n" +
		webSearchCompleted("ws_1", 0, false)
	out := runWebSearchActionCompat(t, input, 1<<20)
	require.Contains(t, out, `"item_id":"ws_1","action":{"type":"search","future":{"x":1}}`)
	require.Equal(t, 2, strings.Count(out, `"future":{"x":1}`), "existing added action must remain raw and completed receives one raw copy")
}

func TestOpenAIResponsesWebSearchActionCompatKeepsCommentFramingWhenPatching(t *testing.T) {
	input := `: data: not-an-SSE-data-field` + "\n" + webSearchCompleted("ws_1", 0, false) + webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"comment-safe"}`)
	out := runWebSearchActionCompat(t, input, 1<<20)
	require.Contains(t, out, `: data: not-an-SSE-data-field`)
	require.Contains(t, out, `"item_id":"ws_1","action":{"type":"search","query":"comment-safe"}`)
}

func TestOpenAIResponsesWebSearchActionCompatPreservesExistingActionImmediately(t *testing.T) {
	input := webSearchCompleted("ws_1", 0, true) + `data: {"type":"response.output_text.delta","delta":"after"}` + "\n\n"
	out := runWebSearchActionCompat(t, input, 1<<20)
	require.Equal(t, input, out)
}

func TestOpenAIResponsesWebSearchActionCompatMatchesIDBeforeOutputIndexAndKeepsOrder(t *testing.T) {
	input := webSearchCompleted("ws_a", 0, false) +
		webSearchCompleted("ws_b", 0, false) +
		webSearchDone("ws_b", 0, `,"action":{"type":"search","query":"B"}`) +
		webSearchDone("ws_a", 0, `,"action":{"type":"search","query":"A"}`)

	out := runWebSearchActionCompat(t, input, 1<<20)
	firstA := strings.Index(out, `"item_id":"ws_a","action":{"type":"search","query":"A"}`)
	firstB := strings.Index(out, `"item_id":"ws_b","action":{"type":"search","query":"B"}`)
	doneB := strings.Index(out, `"id":"ws_b","action":{"type":"search","query":"B"}`)
	doneA := strings.LastIndex(out, `"id":"ws_a","action":{"type":"search","query":"A"}`)
	require.GreaterOrEqual(t, firstA, 0)
	require.Greater(t, firstB, firstA)
	require.Greater(t, doneB, firstB)
	require.Greater(t, doneA, doneB)
}

func TestOpenAIResponsesWebSearchActionCompatFallsBackToOutputIndex(t *testing.T) {
	input := webSearchCompleted("", 1, false) + webSearchDone("", 1, `,"action":{"type":"search","query":"fallback"}`)
	out := runWebSearchActionCompat(t, input, 1<<20)
	require.Contains(t, out, `"item_id":"","action":{"type":"search","query":"fallback"}`)
}

func TestOpenAIResponsesWebSearchActionCompatDoesNotFallbackToIndexWhenDoneHasUnknownID(t *testing.T) {
	pending := webSearchCompleted("ws_a", 0, false)
	done := webSearchDone("ws_unknown", 0, `,"action":{"type":"search","query":"wrong"}`)
	terminal := `data: {"type":"response.completed","sequence_number":10}` + "\n\n"

	require.Equal(t, pending+done+terminal, runWebSearchActionCompat(t, pending+done+terminal, 1<<20))
}

func TestOpenAIResponsesWebSearchActionCompatDoesNotReuseIndexedActionForDifferentID(t *testing.T) {
	added := `data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_b","action":{"type":"search","query":"B"}}}` + "\n\n"
	pending := webSearchCompleted("ws_a", 0, false)
	terminal := `data: {"type":"response.completed","sequence_number":10}` + "\n\n"

	require.Equal(t, added+pending+terminal, runWebSearchActionCompat(t, added+pending+terminal, 1<<20))
}

func TestOpenAIResponsesWebSearchActionCompatTreatsNullAndNonObjectActionsAsMissing(t *testing.T) {
	for _, invalidAction := range []string{"null", `"not-an-object"`, `[]`, `123`, `{}`} {
		t.Run(invalidAction, func(t *testing.T) {
			added := `data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","action":` + invalidAction + `}}` + "\n\n"
			completed := `data: {"type":"response.web_search_call.completed","sequence_number":2,"output_index":0,"item_id":"ws_1","action":` + invalidAction + `}` + "\n\n"
			done := webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"valid"}`)

			out := runWebSearchActionCompat(t, added+completed+done, 1<<20)
			require.NotContains(t, out, `"action":`+invalidAction)
			require.Equal(t, 3, strings.Count(out, `"action":{"type":"search","query":"valid"}`))
		})
	}
}

func TestOpenAIResponsesWebSearchActionCompatMatchingDoneWithoutUsableActionReleasesImmediately(t *testing.T) {
	sourceReader, sourceWriter := io.Pipe()
	body := newOpenAIResponsesWebSearchActionCompatStreamBody(sourceReader, 64*1024, 1<<20)
	t.Cleanup(func() {
		_ = sourceWriter.Close()
		_ = body.Close()
	})

	input := webSearchCompleted("ws_1", 0, false) + webSearchDone("ws_1", 0, `,"action":null`)
	readResult := make(chan string, 1)
	go func() {
		buf := make([]byte, len(input))
		_, err := io.ReadFull(body, buf)
		if err != nil {
			readResult <- "read error: " + err.Error()
			return
		}
		readResult <- string(buf)
	}()

	_, err := io.WriteString(sourceWriter, input)
	require.NoError(t, err)
	select {
	case got := <-readResult:
		require.Equal(t, input, got)
	case <-time.After(time.Second):
		t.Fatal("matching output_item.done without a usable action kept the stream buffered")
	}
}

func TestOpenAIResponsesWebSearchActionCompatFailsOpenForTerminalAndLimit(t *testing.T) {
	pending := webSearchCompleted("ws_1", 0, false)
	for _, eventType := range []string{"response.failed", "response.cancelled", "response.canceled"} {
		terminal := `event: ` + eventType + "\n" + `data: {"type":"` + eventType + `","sequence_number":3}` + "\n\n"
		require.Equal(t, pending+terminal, runWebSearchActionCompat(t, pending+terminal, 1<<20))
	}
	require.Equal(t, pending, runWebSearchActionCompat(t, pending, 1<<20))
	require.Equal(t, pending+`data: [DONE]`+"\n\n", runWebSearchActionCompat(t, pending+`data: [DONE]`+"\n\n", 1<<20))
	require.Equal(t, pending+webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"late"}`), runWebSearchActionCompat(t, pending+webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"late"}`), 1))
}

func TestOpenAIResponsesWebSearchActionCompatFailsOpenForOversizedLineWithoutLosingBytes(t *testing.T) {
	input := `data: {"type":"response.web_search_call.completed","item_id":"ws_1","padding":"` + strings.Repeat("x", 256) + `"}` + "\n\n" +
		webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"late"}`)

	require.Equal(t, input, runWebSearchActionCompatWithLimits(t, input, 64, 1<<20))
}

func TestOpenAIResponsesWebSearchActionCompatFailsOpenForOversizedUnterminatedFrame(t *testing.T) {
	input := strings.Repeat(": short keepalive\n", 16) +
		webSearchCompleted("ws_1", 0, false) +
		webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"late"}`)

	require.Equal(t, input, runWebSearchActionCompatWithLimits(t, input, 64, 96))
}

func TestOpenAIResponsesWebSearchActionCompatPassesThroughNonSSEJSON(t *testing.T) {
	input := `{"id":"resp_json","object":"response","status":"completed","output":[]}`
	require.Equal(t, input, runWebSearchActionCompat(t, input, 1<<20))
}

func TestOpenAIResponsesWebSearchActionCompatFailsOpenForMultiDataFrame(t *testing.T) {
	input := "event: response.web_search_call.completed\n" +
		`data: {"type":"response.web_search_call.completed","item_id":"ws_1"}` + "\n" +
		`data: {"unexpected":true}` + "\n\n" +
		webSearchDone("ws_1", 0, `,"action":{"type":"search","query":"ignored"}`)
	require.Equal(t, input, runWebSearchActionCompat(t, input, 1<<20))
}
