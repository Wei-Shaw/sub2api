package antigravity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestThoughtSignatureCacheSetGet(t *testing.T) {
	cache := NewThoughtSignatureCache(time.Minute, 8)
	cache.Set("call_1", "sig-real")
	require.Equal(t, "sig-real", cache.Get("call_1"))
	require.Equal(t, "", cache.Get("missing"))

	cache.Set("call_sentinel", DummyThoughtSignature)
	require.Equal(t, "", cache.Get("call_sentinel"), "哨兵值不应进入缓存")
	cache.Set("", "sig-empty-id")
	require.Equal(t, "", cache.Get(""))
}

func TestThoughtSignatureCacheExpires(t *testing.T) {
	cache := NewThoughtSignatureCache(10*time.Millisecond, 8)
	cache.Set("call_1", "sig-real")
	require.Equal(t, "sig-real", cache.Get("call_1"))
	time.Sleep(25 * time.Millisecond)
	require.Equal(t, "", cache.Get("call_1"))
}

func TestThoughtSignatureCacheEvictsBeyondCapacity(t *testing.T) {
	cache := NewThoughtSignatureCache(time.Hour, 2)
	cache.Set("a", "sig-a")
	cache.Set("b", "sig-b")
	cache.Set("c", "sig-c")

	require.Equal(t, "sig-c", cache.Get("c"))
	require.True(t, cache.Get("a") == "" || cache.Get("b") == "", "容量满时应淘汰旧条目")
}

func TestStreamProcessorCachesToolSignatureAndTracksThinking(t *testing.T) {
	processor := NewStreamingProcessor("gemini-3.8-flash-high")
	events := processor.ProcessLine(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"cmd":"ls"},"id":"call-stream-1"},"thoughtSignature":"sig-stream"}]}}]}}`)
	require.NotEmpty(t, events)
	require.True(t, processor.SawUpstreamResponse())
	require.False(t, processor.SawThinking())
	require.Equal(t, "sig-stream", LookupToolThoughtSignature("call-stream-1"))
}

func TestStreamProcessorSignatureOnlyFrameCountsAsThinking(t *testing.T) {
	processor := NewStreamingProcessor("gemini-3.8-flash-high")
	events := processor.ProcessLine(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig-only"}]},"finishReason":"STOP"}]}}`)
	// 只有签名帧时兼容层不补发结束事件，但处理器必须记录“上游有响应 + 有思考”。
	_ = events
	require.True(t, processor.SawUpstreamResponse())
	require.True(t, processor.SawThinking())
	require.False(t, processor.HasContent())
	require.Equal(t, "STOP", processor.LastFinishReason())
	require.Equal(t, "", LookupToolThoughtSignature("call-none"))
}
