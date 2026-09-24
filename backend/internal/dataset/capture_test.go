package dataset

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func captureTestRequest(stream bool) []byte {
	if stream {
		return []byte(`{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	}
	return []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`)
}

func captureTestResponse() []byte {
	return []byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"world"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
}

func captureTestStream() []byte {
	return []byte("data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
		"data: [DONE]\n\n")
}

func installCaptureTestCollector(t *testing.T) *Collector {
	t.Helper()
	previous := CollectorSingleton
	collector := NewCollector(10)
	CollectorSingleton = collector
	t.Cleanup(func() { CollectorSingleton = previous })
	return collector
}

func TestCaptureFromOpenAIRequest_CapturesNonStreamingSuccess(t *testing.T) {
	collector := installCaptureTestCollector(t)

	CaptureFromOpenAIRequest(captureTestRequest(false), captureTestResponse(), 200)

	entries := collector.Drain(1)
	require.Len(t, entries, 1)
	require.Equal(t, "gpt-test", entries[0].Request["model"])
	message, ok := entries[0].Response["message"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "world", message["content"])
	usage, ok := entries[0].Response["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, int64(5), usage["total_tokens"])
}

func TestCaptureFromOpenAIRequest_SkipsCaptureOnErrorStatus(t *testing.T) {
	collector := installCaptureTestCollector(t)

	for _, status := range []int{400, 500} {
		CaptureFromOpenAIRequest(captureTestRequest(false), captureTestResponse(), status)
	}

	require.Empty(t, collector.Drain(1))
}

func TestCaptureFromOpenAIRequest_SkipsCaptureWhenDisabled(t *testing.T) {
	previous := CollectorSingleton
	CollectorSingleton = nil
	t.Cleanup(func() { CollectorSingleton = previous })

	CaptureFromOpenAIRequest(captureTestRequest(false), captureTestResponse(), 200)
}

func TestCaptureFromOpenAIStream_CapturesCompletedStream(t *testing.T) {
	collector := installCaptureTestCollector(t)

	CaptureFromOpenAIStream(captureTestRequest(true), captureTestStream(), 200)

	entries := collector.Drain(1)
	require.Len(t, entries, 1)
	message, ok := entries[0].Response["message"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hello", message["content"])
	require.Equal(t, "stop", entries[0].Response["finish_reason"])
}

func TestCaptureFromOpenAIStream_DropsPartialStream(t *testing.T) {
	collector := installCaptureTestCollector(t)
	partial := bytes.TrimSuffix(captureTestStream(), []byte("data: [DONE]\n\n"))

	CaptureFromOpenAIStream(captureTestRequest(true), partial, 200)

	require.Empty(t, collector.Drain(1))
}

func TestCaptureFromOpenAIRequest_CapsResponseAt10MB(t *testing.T) {
	collector := installCaptureTestCollector(t)
	oversized := bytes.Repeat([]byte("x"), MaxCaptureBytes+1)

	CaptureFromOpenAIRequest(captureTestRequest(false), oversized, 200)

	require.Empty(t, collector.Drain(1))
}
