package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func legacyImageUsageCount(body []byte) int {
	if !gjson.ValidBytes(body) {
		return 0
	}
	count := 0
	for _, path := range []string{
		"usage.images", "response.usage.images", "data.usage.images", "data.response.usage.images",
		"tool_usage.image_gen.images", "response.tool_usage.image_gen.images", "data.tool_usage.image_gen.images", "data.response.tool_usage.image_gen.images",
	} {
		count = max(count, int(gjson.GetBytes(body, path).Int()))
	}
	return count
}

func legacyImageSSECount(counter *openAIImageOutputCounter, data []byte) {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "[DONE]" || !gjson.ValidBytes(data) {
		return
	}
	root := gjson.ParseBytes(data)
	counter.addDataArray(root.Get("data"))
	counter.reportedCount = max(counter.reportedCount, legacyImageUsageCount(data))
	switch strings.TrimSpace(root.Get("type").String()) {
	case "response.output_item.done":
		counter.addImageOutputItem(root.Get("item"))
	case "response.completed", "response.done":
		counter.addOutputArray(root.Get("response.output"))
	case "image_generation.completed":
		if item := root.Get("item"); item.Exists() {
			counter.addImageOutputItem(item)
		} else if output := root.Get("output"); output.Exists() {
			counter.addImageOutputItem(output)
		} else {
			counter.addImageOutputItem(root)
		}
	}
}

func legacyImageJSONCount(counter *openAIImageOutputCounter, body []byte) {
	if !gjson.ValidBytes(body) {
		return
	}
	counter.addDataArray(gjson.GetBytes(body, "data"))
	counter.addOutputArray(gjson.GetBytes(body, "output"))
	counter.addOutputArray(gjson.GetBytes(body, "response.output"))
	counter.reportedCount = max(counter.reportedCount, legacyImageUsageCount(body))
}

func TestImageAccountingProjectionPreservesReportsOnTextEvents(t *testing.T) {
	for _, body := range []string{
		`{"type":"response.output_text.delta","delta":"text","usage":{"images":2}}`,
		`{"type":"response.output_text.delta","delta":"text","data":{"response":{"tool_usage":{"image_gen":{"images":3}}}}}`,
		`{"type":"response.done","usage":{"images":2},"response":{"usage":{"images":4}},"data":{"usage":{"images":7}},"tool_usage":{"image_gen":{"images":6}}}`,
		`{"usage":null,"usage":{"images":9},"response":{"usage":{"images":3}}}`,
		`{"type":"response.done","response":null,"response":{"output":[{"id":"second_response","result":"image"}],"usage":{"images":4}}}`,
		`{"data":{"response":null,"response":{"usage":{"images":7}}}}`,
		`{"type":"image_generation.completed","id":"img_1","result":"payload","size":"1024x1024"}`,
		`{"type":"response.done","data":[{"url":"https://example.com/a.png","size":"small"}],"response":{"output":[{"type":"image_generation_call","id":"img_1","result":"payload"}]}}`,
		`{"type":"response.output_item.done","item":{"type":"image_generation_call","id":"img_partial","result":"partial_image"}}`,
	} {
		payload := []byte(body)
		want, got := newOpenAIImageOutputCounter(), newOpenAIImageOutputCounter()
		legacyImageSSECount(want, payload)
		got.AddSSEData(payload)
		require.Equal(t, want, got, body)
		require.Equal(t, legacyImageUsageCount(payload), countOpenAIImageUsageFromJSONBytes(payload), body)
		legacyImageJSONCount(want, payload)
		got.AddJSONResponse(payload)
		require.Equal(t, want, got, body)
	}
}

func TestImageAccountingDoesNotRetainReadBuffer(t *testing.T) {
	for _, body := range []string{
		`{"type":"response.output_item.done","item":{"type":"image_generation_call","id":"image_identity","result":"payload","size":"1024x1024"}}`,
		`{"data":[{"url":"https://example.com/img.png","size":"1024x1024"}]}`,
	} {
		payload := []byte(body)
		counter := newOpenAIImageOutputCounter()
		counter.AddSSEData(payload)
		for i := range payload {
			payload[i] = 'x'
		}
		require.Equal(t, 1, counter.Count())
		require.Equal(t, []string{"1024x1024"}, counter.Sizes())
		counter.AddSSEData([]byte(body))
		require.Equal(t, 1, counter.Count(), "dedup keys must survive read-buffer reuse")
	}
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte(`{"type":"response.output_item.done","item":{"id":"late_size","result":"image"}}`))
	withSize := []byte(`{"type":"response.output_item.done","item":{"id":"late_size","result":"image","size":"1024x1024"}}`)
	counter.AddSSEData(withSize)
	for i := range withSize {
		withSize[i] = 'x'
	}
	require.Equal(t, []string{"1024x1024"}, counter.Sizes())
}

func FuzzImageAccountingProjection(f *testing.F) {
	for _, seed := range []string{
		`{"type":"response.output_text.delta","delta":"你好","usage":{"images":2}}`,
		`{"data":{"response":{"tool_usage":{"image_gen":{"images":3}}}}}`,
		`{"type":"response.done","response":{"output":[{"type":"image_generation_call","id":"image_1","result":"data"}]}}`,
		`{"type":"image_generation.completed","item":null,"output":{"result":"image"}}`,
		`{"usage":{"images":2},"usage":null}`, `[]`, `not json`,
		`{"usage":null,"usage":{"images":9}}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64<<10 {
			return
		}
		body := []byte(raw)
		require.Equal(t, legacyImageUsageCount(body), countOpenAIImageUsageFromJSONBytes(body))
		before, after := newOpenAIImageOutputCounter(), newOpenAIImageOutputCounter()
		legacyImageSSECount(before, body)
		after.AddSSEData(body)
		require.Equal(t, before, after)
		legacyImageJSONCount(before, body)
		after.AddJSONResponse(body)
		require.Equal(t, before, after)
	})
}
