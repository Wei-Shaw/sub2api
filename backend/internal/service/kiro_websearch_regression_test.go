//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// Kiro WebSearch 多轮回归测试。
//
// 背景：AdjustSSEChunk 会抑制每轮上游流的 message_delta/message_stop，多轮
// web search 的终止事件与全轮累计用量必须由 streamKiroWebSearchAsAnthropic /
// executeKiroWebSearch 显式补齐；缓存前缀必须在一轮真正解析/写出成功之后才提交。
// 这三点是审计（docs/KIRO_MIGRATION_AUDIT.md P2-2）要求补的回归场景。

func kiroWebSearchRegressionAccount() *Account {
	return &Account{
		ID:          451,
		Name:        "kiro-websearch-regression",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"profile_arn":  "arn:aws:codewhisperer:us-east-1:123456789012:profile/WEBSEARCH",
		},
	}
}

// kiroMCPResultResponse 构造一个带一条搜索结果的 MCP tools/call 响应。
func kiroMCPResultResponse(t *testing.T) *http.Response {
	t.Helper()
	results, err := json.Marshal(kiropkg.WebSearchResults{Results: []kiropkg.WebSearchResult{
		{Title: "Example", URL: "https://example.com/doc"},
	}})
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(results)}},
		},
	})
	require.NoError(t, err)
	return newJSONResponse(http.StatusOK, string(body))
}

// kiroWebSearchRoundStream 构造一轮 Kiro 上游事件流：
// 工具轮携带内嵌 web_search 调用标记（触发多轮），末轮为普通文本回复。
// usage 事件上报当轮的 input/output tokens。
func kiroWebSearchRoundStream(t *testing.T, content string, toolCallMarker bool, inputTokens, outputTokens int) io.ReadCloser {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	payload := map[string]any{"assistantResponseEvent": map[string]any{"content": content}}
	if toolCallMarker {
		payload = map[string]any{"assistantResponseEvent": map[string]any{
			"content": "Let me verify that. [Called web_search with args: {\"query\":\"go gc approach\"}] and summarize.",
		}}
		_ = content
	}
	_, _ = buf.Write(buildKiroEventStreamFrame(t, "assistantResponseEvent", payload))
	_, _ = buf.Write(buildKiroEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": inputTokens,
				"outputTokens":        outputTokens,
			},
		},
	}))
	return io.NopCloser(buf)
}

func kiroWebSearchStreamingBody() []byte {
	return []byte(`{
		"model": "claude-sonnet-4-6",
		"stream": true,
		"messages": [{"role": "user", "content": "What GC approach does Go use?"}],
		"tools": [{"type": "web_search_20250305", "name": "web_search"}]
	}`)
}

func kiroWebSearchGatewayService(t *testing.T, upstream *queuedHTTPUpstream) *GatewayService {
	t.Helper()
	// 预热 MCP 工具描述缓存，避免 prefetch 额外发请求打乱 mock 队列。
	mcpEndpoint := kiropkg.BuildMcpEndpoint("us-east-1")
	kiroWebSearchDescCache.Store(mcpEndpoint, "web search description")
	return &GatewayService{
		httpUpstream:        upstream,
		kiroCooldownStore:   &stubKiroCooldownStore{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 0,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
	}
}

// TestKiroWebSearchStreaming_MultiRoundAccumulatesUsageAndEmitsTerminalEvents
// 验证流式多轮 web search：终止事件恰好一份（AdjustSSEChunk 抑制上游每轮的
// 终止事件后由网关补发），usage 为全轮累计而非最后一轮。
func TestKiroWebSearchStreaming_MultiRoundAccumulatesUsageAndEmitsTerminalEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			kiroMCPResultResponse(t),
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
				Body:       kiroWebSearchRoundStream(t, "round1", true, 100, 10),
			},
			kiroMCPResultResponse(t),
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
				Body:       kiroWebSearchRoundStream(t, "Go uses a concurrent tri-color mark-and-sweep GC.", false, 200, 20),
			},
		},
	}
	svc := kiroWebSearchGatewayService(t, upstream)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(kiroWebSearchStreamingBody()), domain.PlatformAnthropic)
	require.NoError(t, err)

	result, err := svc.Forward(context.Background(), c, kiroWebSearchRegressionAccount(), parsed)
	require.NoError(t, err, "web search stream must terminate cleanly with terminal events")
	require.NotNil(t, result)
	require.True(t, result.Stream)
	// 全轮累计：round1 100/10 + round2 200/20
	require.Equal(t, 300, result.Usage.InputTokens)
	require.Equal(t, 30, result.Usage.OutputTokens)
	require.Len(t, upstream.requests, 4)

	clientOut := rec.Body.String()
	require.Equal(t, 1, strings.Count(clientOut, "event: message_stop"), "exactly one terminal message_stop")
	require.Equal(t, 1, strings.Count(clientOut, "event: message_delta"), "exactly one cumulative message_delta")
	require.Contains(t, clientOut, "Go uses a concurrent tri-color mark-and-sweep GC.")
	// message_delta 必须出现在 message_stop 之前（终止事件顺序合法）
	require.Less(t, strings.LastIndex(clientOut, "event: message_delta"), strings.Index(clientOut, "event: message_stop"))
	// 内部 credits 标记不得泄漏到客户端
	require.NotContains(t, clientOut, "_sub2api_kiro_credits")
}

// TestKiroWebSearchNonStreaming_MultiRoundAccumulatesUsage 验证非流式多轮
// web search 的计费用量为全轮累计：最后一轮响应体只含当轮 usage，若只按
// 响应体计费会漏掉前几轮。
func TestKiroWebSearchNonStreaming_MultiRoundAccumulatesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := kiroWebSearchStreamingBody()
	body = []byte(strings.ReplaceAll(string(body), `"stream": true`, `"stream": false`))

	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			kiroMCPResultResponse(t),
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
				Body:       kiroWebSearchRoundStream(t, "round1", true, 100, 10),
			},
			kiroMCPResultResponse(t),
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
				Body:       kiroWebSearchRoundStream(t, "Go uses a concurrent tri-color mark-and-sweep GC.", false, 200, 20),
			},
		},
	}
	svc := kiroWebSearchGatewayService(t, upstream)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
	require.NoError(t, err)

	result, err := svc.Forward(context.Background(), c, kiroWebSearchRegressionAccount(), parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, 300, result.Usage.InputTokens, "usage must accumulate across all rounds, not just the last")
	require.Equal(t, 30, result.Usage.OutputTokens)
	require.Len(t, upstream.requests, 4)
}

// TestKiroWebSearchStreaming_UpstreamErrorDoesNotCommitCachePrefix 验证上游流
// 失败（首轮流解析失败）时缓存前缀不提交：提交后下一次相同前缀请求会被误判为
// 缓存命中，污染计费估算。
func TestKiroWebSearchStreaming_UpstreamErrorDoesNotCommitCachePrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	group := &Group{
		ID:                        1,
		Platform:                  PlatformKiro,
		KiroCacheEmulationEnabled: true,
	}

	entriesBefore := kiroCacheTrackerEntryCount(t)

	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			kiroMCPResultResponse(t),
			// 首轮 200 但流被截断（帧长声明 100 字节、实际只写了帧头）：
			// 解析在 readEventStreamMessage 处报错，整轮失败。
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
				Body:       io.NopCloser(bytes.NewBuffer([]byte{0, 0, 0, 100, 0, 0, 0, 0})),
			},
		},
	}
	svc := kiroWebSearchGatewayService(t, upstream)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(kiroWebSearchStreamingBody()), domain.PlatformAnthropic)
	require.NoError(t, err)
	parsed.Group = group

	_, err = svc.Forward(context.Background(), c, kiroWebSearchRegressionAccount(), parsed)
	require.Error(t, err, "malformed upstream stream must fail the request")

	require.Equal(t, entriesBefore, kiroCacheTrackerEntryCount(t),
		"cache prefix must not be committed when the upstream stream fails")
}

// kiroCacheTrackerEntryCount 统计全局缓存追踪器中的前缀条目数（含嵌套 key）。
func kiroCacheTrackerEntryCount(t *testing.T) int {
	t.Helper()
	globalKiroCacheTracker.mu.Lock()
	defer globalKiroCacheTracker.mu.Unlock()
	total := 0
	for _, byPrefix := range globalKiroCacheTracker.entries {
		total += len(byPrefix)
	}
	return total
}
