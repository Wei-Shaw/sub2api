package handler

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// zhipuMCPMaxAttempts 首次尝试 + 至多 3 次换号，与 WebSearch 等网关转发的重试深度一致。
const zhipuMCPMaxAttempts = 4

// zhipuMCPForwardedRequestHeaders 允许透传给智谱上游的请求头（Go canonical 形式）。
// 采用白名单而非黑名单：Authorization / x-api-key / x-goog-api-key / Host /
// Content-Length / Connection / Transfer-Encoding / Accept-Encoding 等敏感或
// hop-by-hop 头天然不会进入上游请求；认证头由 handler 按选中账号显式重写。
// Accept-Encoding 不透传，交给 Go transport 自协商（自动解压），避免把压缩体原样转给客户端。
var zhipuMCPForwardedRequestHeaders = map[string]struct{}{
	"Accept":               {},
	"Accept-Language":      {},
	"Content-Type":         {},
	"User-Agent":           {},
	"Mcp-Session-Id":       {},
	"MCP-Protocol-Version": {},
}

// ZhipuMCPPassthrough 智谱远程 MCP Server 透传（MCP Streamable HTTP）。
// 客户端带 sub2api API key 访问 /api/mcp/zhipu/{slug}/mcp，本 handler 从 zhipu
// 账号池选号、重写认证头后把 JSON-RPC 请求原样转发给上游，响应（JSON 或 SSE）原样回传。
// 第一版仅支持 POST（JSON-RPC）与 DELETE（终止 session）；GET server-push 与
// /sse fallback 不支持。计费与 usage 记录在 Phase 2B 接入。
func (h *GatewayHandler) ZhipuMCPPassthrough(c *gin.Context) {
	if method := c.Request.Method; method != http.MethodPost && method != http.MethodDelete {
		c.Header("Allow", "POST, DELETE")
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": gin.H{
			"type":    "method_not_allowed",
			"message": "MCP passthrough supports POST and DELETE only",
		}})
		return
	}

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type":    "authentication_error",
			"message": "API key required",
		}})
		return
	}

	// composite 分组第一版不支持：MCP 请求没有模型概念，无法做 composite 路由决策。
	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformZhipu {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "MCP passthrough is only supported for zhipu groups",
		}})
		return
	}
	groupID := apiKey.GroupID
	if groupID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "group required",
		}})
		return
	}

	targetURL, ok := service.ResolveZhipuMCPServerURL(c.Param("slug"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"type":    "not_found",
			"message": "unknown MCP server slug",
		}})
		return
	}

	// 请求体原样透传；大小上限由路由层的 RequestBodyLimit 中间件保证。
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "failed to read request body",
		}})
		return
	}

	reqLog := requestLogger(c, "handler.gateway.zhipu_mcp")

	// 粘性亲和：客户端回传 Mcp-Session-Id 时优先粘到绑定账号。
	// 命中并校验通过后跳过调度直接使用该账号（MCP session 的上游服务端状态
	// 归属绑定时选中的账号，换号意味着 session 丢失）；校验失败（账号被删/
	// 停用/关闭 MCP 开关）立即清粘表，回退正常调度。
	var stickyAccount *service.Account
	incomingSessionID := c.GetHeader("Mcp-Session-Id")
	scheduleCtx := c.Request.Context()
	if incomingSessionID != "" {
		if boundAccountID, bound := h.gatewayService.LookupZhipuMCPSession(scheduleCtx, incomingSessionID); bound {
			loaded, loadErr := h.gatewayService.LoadZhipuMCPStickyAccount(scheduleCtx, boundAccountID)
			if loadErr == nil {
				stickyAccount = loaded
				reqLog.Debug("gateway.zhipu_mcp.sticky_hit",
					zap.Int64("account_id", boundAccountID),
					zap.String("session_prefix", incomingSessionID[:min(len(incomingSessionID), 8)]),
				)
			} else {
				// 删除失败由 TTL 兜底，最坏情况是多走一次失败校验。
				_ = h.gatewayService.UnbindZhipuMCPSession(scheduleCtx, incomingSessionID)
				reqLog.Info("gateway.zhipu_mcp.sticky_invalid",
					zap.Int64("account_id", boundAccountID),
					zap.Error(loadErr),
				)
			}
		}
	}

	failedAccounts := make(map[int64]struct{})
	var account *service.Account
	var accountReleaseFunc func()
	var upstreamResp *http.Response

	defer func() {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
	}()

	for attempt := 0; attempt < zhipuMCPMaxAttempts; attempt++ {
		var selected *service.AccountSelectionResult
		if attempt == 0 && stickyAccount != nil {
			// 粘表路径：Acquired=true 直接复用第一次尝试，不走调度与并发等待。
			// Phase 2A 简化：MCP session 通常被单个客户端串行使用，绑定账号的
			// 并发槽位压力远小于模型转发；如需严格控制再接入账号槽位。
			selected = &service.AccountSelectionResult{
				Account:     stickyAccount,
				Acquired:    true,
				ReleaseFunc: func() {},
			}
		} else {
			// requestedModel 传空：MCP JSON-RPC 没有模型概念，空模型在调度器里
			// 跳过 allowed_models 过滤、渠道定价限制与模型路由，是调度器对
			// "非模型流量"的既定语义（websearch 传伪模型名是因为它实际转发
			// 带 model 字段的 responses 请求，MCP 没有这个负担）。
			var selectErr error
			selected, selectErr = h.gatewayService.SelectAccountWithLoadAwareness(
				scheduleCtx, groupID, "", "", failedAccounts, "", 0,
			)
			if selectErr != nil || selected == nil || selected.Account == nil {
				if attempt == 0 {
					message := "No available accounts"
					if selectErr != nil {
						message = selectErr.Error()
					}
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
						"type":    "scheduling_error",
						"message": message,
					}})
					return
				}
				break
			}
		}

		// 复用 websearch 的账号槽位获取器（含等待队列，与平台无关）；
		// 首跳把并发错误映射给客户端，后续跳换号重试。
		release, acquireOK, acquireErr := h.acquireWebSearchAccountSlot(c, selected)
		if !acquireOK {
			if attempt == 0 && acquireErr != nil {
				h.handleConcurrencyError(c, acquireErr, "account", false)
				return
			}
			failedAccounts[selected.Account.ID] = struct{}{}
			continue
		}
		account = selected.Account
		accountReleaseFunc = release

		// 双保险：正常调度不会选到非 zhipu / 未开 MCP 的账号，但粘表路径
		// 与调度器之间可能有配置变更窗口，这里兜底换号。
		if !account.IsZhipuMCPCapable() {
			failedAccounts[account.ID] = struct{}{}
			accountReleaseFunc()
			accountReleaseFunc = nil
			account = nil
			continue
		}

		zhipuKey := account.GetCredential("api_key")
		if zhipuKey == "" {
			// 凭据缺失视为账号级故障，换号重试。
			reqLog.Warn("gateway.zhipu_mcp.missing_credential", zap.Int64("account_id", account.ID))
			failedAccounts[account.ID] = struct{}{}
			accountReleaseFunc()
			accountReleaseFunc = nil
			account = nil
			continue
		}

		upstreamReq, buildErr := http.NewRequestWithContext(scheduleCtx, c.Request.Method, targetURL, bytes.NewReader(body))
		if buildErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
				"type":    "zhipu_mcp_error",
				"message": "failed to build upstream request",
			}})
			return
		}
		upstreamReq.Header = buildZhipuMCPUpstreamHeaders(c)
		upstreamReq.Header.Set("Authorization", "Bearer "+zhipuKey)

		resp, doErr := h.gatewayService.DoZhipuMCPUpstream(account, upstreamReq)
		if doErr != nil {
			// 传输层失败（建连/代理/TLS）按账号级故障换号重试。
			reqLog.Warn("gateway.zhipu_mcp.upstream_transport_error",
				zap.Int64("account_id", account.ID),
				zap.String("slug", c.Param("slug")),
				zap.Error(doErr),
			)
			failedAccounts[account.ID] = struct{}{}
			accountReleaseFunc()
			accountReleaseFunc = nil
			account = nil
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusPaymentRequired {
			// 429（限流）/402（额度）是账号级信号 → 换号重试。
			// 第一版不触发持久冷却：MCP 流量与模型转发共用账号池，此处写限流
			// 标记会误伤正常的模型请求，等 MCP 侧有独立健康度模型后再接。
			_ = resp.Body.Close()
			failedAccounts[account.ID] = struct{}{}
			accountReleaseFunc()
			accountReleaseFunc = nil
			account = nil
			continue
		}

		// 其余状态码（含上游 4xx/5xx）原样透传：MCP JSON-RPC 是协议级请求，
		// 400/404 等属于客户端协议错误不该换号；5xx 不自动重试，避免
		// tools/call 这类非幂等调用被重复执行。
		upstreamResp = resp
		break
	}

	if upstreamResp == nil {
		message := "MCP upstream request failed"
		if account != nil {
			// 最后一次尝试停留在账号级失败（429/402），对外保持 502。
			reqLog.Warn("gateway.zhipu_mcp.attempts_exhausted", zap.Int64("account_id", account.ID))
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"type":    "zhipu_mcp_error",
			"message": message,
		}})
		return
	}
	defer func() { _ = upstreamResp.Body.Close() }()

	// ── 响应透传：状态码 + 白名单响应头 + body（SSE 逐块 flush）──
	contentType := upstreamResp.Header.Get("Content-Type")
	if contentType != "" {
		c.Writer.Header().Set("Content-Type", contentType)
	}
	upstreamSessionID := upstreamResp.Header.Get("Mcp-Session-Id")
	if upstreamSessionID != "" {
		c.Writer.Header().Set("Mcp-Session-Id", upstreamSessionID)
	}
	c.Status(upstreamResp.StatusCode)

	if isZhipuMCPSSE(contentType) {
		streamZhipuMCPBody(c, upstreamResp.Body)
	} else {
		_, _ = io.Copy(c.Writer, upstreamResp.Body)
	}

	// 绑定粘表：仅当上游下发 Mcp-Session-Id（initialize 语义）。
	// 上游没回发 session id 时不做本地改绑——session 的服务端状态归属原账号，
	// 把绑定改到 failover 账号会让后续请求撞上"session not found"。
	if upstreamSessionID != "" && account != nil {
		if bindErr := h.gatewayService.BindZhipuMCPSession(c.Request.Context(), upstreamSessionID, account.ID); bindErr != nil {
			reqLog.Warn("gateway.zhipu_mcp.bind_session_failed", zap.Error(bindErr))
		}
	}

	// 客户端显式终止 session：上游受理后清理粘表。
	if c.Request.Method == http.MethodDelete && upstreamResp.StatusCode < http.StatusBadRequest && incomingSessionID != "" {
		if unbindErr := h.gatewayService.UnbindZhipuMCPSession(c.Request.Context(), incomingSessionID); unbindErr != nil {
			reqLog.Warn("gateway.zhipu_mcp.unbind_session_failed", zap.Error(unbindErr))
		}
	}
}

// buildZhipuMCPUpstreamHeaders 按白名单复制客户端请求头。
func buildZhipuMCPUpstreamHeaders(c *gin.Context) http.Header {
	out := make(http.Header)
	for name, values := range c.Request.Header {
		// Go 已把请求头 canonical 化，自定义 mcp-* 头统一为 Mcp- 前缀。
		if _, allowed := zhipuMCPForwardedRequestHeaders[name]; !allowed && !strings.HasPrefix(name, "Mcp-") {
			continue
		}
		for _, value := range values {
			out.Add(name, value)
		}
	}
	return out
}

// isZhipuMCPSSE 判断上游响应是否为 SSE 流式（MCP Streamable HTTP 对
// notifications / 长任务的响应形态）。
func isZhipuMCPSSE(contentType string) bool {
	mediaType := contentType
	if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
		mediaType = contentType[:idx]
	}
	return strings.EqualFold(strings.TrimSpace(mediaType), "text/event-stream")
}

// streamZhipuMCPBody 逐块写透 SSE body 并每块 Flush，保证事件实时到达客户端
// （与网关 SSE 转发的 Flusher 用法一致）。
func streamZhipuMCPBody(c *gin.Context, body io.Reader) {
	flusher, _ := c.Writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				// 客户端断开等写失败：终止透传，上游 body 由调用方 defer 关闭。
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}
