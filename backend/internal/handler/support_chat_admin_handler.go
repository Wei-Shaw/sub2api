// Package handler — support_chat_admin_handler.go
//
// admin 端"测试客服 LLM 连接"端点。挂在 admin 路由组下（自带 AdminAuthMiddleware）：
//
//	POST /api/v1/admin/support/chat/test-llm-connection
//
// 设计要点（change-support-chat-external-llm tasks §4）：
//
//  1. 用 admin 提供的 base_url + api_key（或在 api_key 等于"已存储值的掩码"时，
//     沿用已存储的 cleartext，免去 admin 重新粘贴 secret），向 `<base_url>/chat/completions`
//     发一个最小 payload（messages=[{role:user,content:ping}], max_tokens=1, stream:false）。
//  2. 5s 全局超时（含 connect / TLS / response read）：测试端点不能阻塞 admin UI 太久。
//  3. base_url 为空 / 不以 http(s):// 开头时直接返回 invalid_base_url，**不发起任何网络请求**——
//     可能拼出非法 URL 或意外打到本机服务，先在入口处拦掉。
//  4. 错误归一化：把 net/url 各种底层错误映射成 timeout / dns_lookup_failed / connection_refused
//     / tls_error 等少量分类常量；非 2xx 时尽量从 OpenAI-compat 风格 body.error.message 提取，
//     失败兜底 "upstream non-2xx"。让前端不需要解析 Go err.Error() 字符串就能展示友好提示。
//  5. **永远 HTTP 200** + 业务 ok 字段（含 ok=false 情形）。原因：探活接口本身的 HTTP 状态
//     与上游探测结果是两件事——把上游失败也用 200 包起来便于前端统一只读 body。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// TestSupportChatLLMConnectionRequest 是 POST /api/v1/admin/support/chat/test-llm-connection 入参。
//
// 三个字段都是 cleartext / 当前页面表单值；当 admin 没改 api_key 时前端会原样回传服务端
// 在 GET 里下发的"掩码值"（如 "sk-***xxxx"），handler 内部识别该哨兵并替换为已存储的真值。
type TestSupportChatLLMConnectionRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model,omitempty"`
}

// TestSupportChatLLMConnectionResponse 是探测结果体。
//
// status_code 用 *int 是为了显式区分 "没发出请求 → null" vs "拿到 0 → 异常零值"。
// JSON 输出时空指针 → null，调用方据此判断"是否真的发起过 HTTP"。
type TestSupportChatLLMConnectionResponse struct {
	OK         bool   `json:"ok"`
	LatencyMS  int64  `json:"latency_ms"`
	StatusCode *int   `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// testSupportChatLLMTimeout 上游探测的硬上限（含 dial / TLS / read）。
// 故意比 chat 转发时的 timeout 短得多——admin 在 UI 上点"测试连接"时希望快速反馈。
const testSupportChatLLMTimeout = 5 * time.Second

// TestLLMConnection 处理 POST /api/v1/admin/support/chat/test-llm-connection。
//
// 注意：路由组已经套了 AdminAuthMiddleware，这里不再做权限校验。
func (h *SupportChatHandler) TestLLMConnection(c *gin.Context) {
	ctx := c.Request.Context()

	var req TestSupportChatLLMConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = service.SupportChatDefaultModel
	}

	// 1. base_url 入口校验：空 / 非 http(s):// 直接 short-circuit，不发起 HTTP。
	if baseURL == "" || !(strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://")) {
		response.Success(c, TestSupportChatLLMConnectionResponse{
			OK:         false,
			LatencyMS:  0,
			StatusCode: nil,
			Error:      "invalid_base_url",
		})
		return
	}

	// 2. 解析 api_key：优先用 admin 输入的；若它等于"已存储值的掩码"则视为"沿用已存值"，
	//    替换为 cleartext。这样 admin 只改 base_url 时不必每次重新粘贴 secret。
	apiKey := strings.TrimSpace(req.APIKey)
	_, storedAPIKey := h.settingService.GetSupportChatLLMCredentials(ctx)
	if apiKey != "" && storedAPIKey != "" && apiKey == service.MaskSupportChatLLMAPIKey(storedAPIKey) {
		apiKey = storedAPIKey
	}
	if apiKey == "" {
		// 没传 api_key 也没已存值：等价于无凭据，不必真发出去拿 401。
		response.Success(c, TestSupportChatLLMConnectionResponse{
			OK:         false,
			LatencyMS:  0,
			StatusCode: nil,
			Error:      "missing_api_key",
		})
		return
	}

	// 3. 构造最小 chat completions 请求。
	body, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     false,
	})
	if err != nil {
		// 几乎不可能（map[string]any+原生类型），保险起见兜底。
		response.Success(c, TestSupportChatLLMConnectionResponse{
			OK:    false,
			Error: "build_request_failed",
		})
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, testSupportChatLLMTimeout)
	defer cancel()

	upstreamReq, err := http.NewRequestWithContext(probeCtx, http.MethodPost,
		buildUpstreamChatURL(baseURL), bytes.NewReader(body))
	if err != nil {
		response.Success(c, TestSupportChatLLMConnectionResponse{
			OK:    false,
			Error: "build_request_failed",
		})
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)

	// 独立 client（不复用 h.httpClient——后者 Timeout=0 用于 long-lived SSE）。
	client := &http.Client{Timeout: testSupportChatLLMTimeout}

	start := time.Now()
	resp, err := client.Do(upstreamReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		response.Success(c, TestSupportChatLLMConnectionResponse{
			OK:         false,
			LatencyMS:  latency,
			StatusCode: nil,
			Error:      classifyProbeError(err),
		})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	sc := resp.StatusCode
	if sc >= 200 && sc < 300 {
		response.Success(c, TestSupportChatLLMConnectionResponse{
			OK:         true,
			LatencyMS:  latency,
			StatusCode: &sc,
		})
		return
	}

	// 4. 非 2xx：尽量从 OpenAI-compat error body 抽 message；不行就用通用文案。
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	msg := extractUpstreamErrorMessage(raw)
	if msg == "" {
		msg = "upstream non-2xx"
	}
	response.Success(c, TestSupportChatLLMConnectionResponse{
		OK:         false,
		LatencyMS:  latency,
		StatusCode: &sc,
		Error:      msg,
	})
}

// classifyProbeError 把 net / http / context 各种底层错误映射成稳定的小写枚举字符串。
//
// 前端只需要硬编码这一组 token 做 i18n（timeout / dns_lookup_failed / connection_refused
// / tls_error / build_request_failed / invalid_base_url / missing_api_key），
// 不必解析 Go runtime 错误信息（不同 Go 版本的 err.Error() 可能漂移）。
//
// 兜底分支返回 err.Error() 原文：覆盖未分类的稀有错误（IPv6 only / proxy auth required 等），
// 让前端至少有调试线索；前端展示时建议截断到 200 字符。
func classifyProbeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	s := err.Error()
	sl := strings.ToLower(s)
	switch {
	case strings.Contains(sl, "no such host"):
		return "dns_lookup_failed"
	case strings.Contains(sl, "connection refused"):
		return "connection_refused"
	case strings.Contains(sl, "x509"),
		strings.Contains(sl, "tls"),
		strings.Contains(sl, "certificate"):
		return "tls_error"
	case strings.Contains(sl, "timeout"),
		strings.Contains(sl, "deadline exceeded"):
		return "timeout"
	default:
		return s
	}
}

// extractUpstreamErrorMessage 从 OpenAI-compat upstream 错误响应中抽 `error.message`。
// 非 JSON / 缺字段时返回空串，由调用方走"upstream non-2xx"兜底。
func extractUpstreamErrorMessage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	return strings.TrimSpace(env.Error.Message)
}
