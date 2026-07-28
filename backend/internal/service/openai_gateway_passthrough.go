package service

// 本文件承载 /v1/responses 透传转发及其流式、非流式响应与错误处理。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) forwardOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	canonicalImageIntentBody []byte,
	reqModel string,
	attemptImageIntentInvalidated bool,
	reasoningEffort *string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	upstreamPassthroughModel := ""
	if isOpenAIResponsesCompactPath(c) {
		compactMappedModel := resolveOpenAICompactForwardModel(account, reqModel)
		if compactMappedModel != "" && compactMappedModel != reqModel {
			nextBody, setErr := sjson.SetBytes(body, "model", compactMappedModel)
			if setErr != nil {
				return nil, fmt.Errorf("set compact passthrough model: %w", setErr)
			}
			body = nextBody
			upstreamPassthroughModel = compactMappedModel
			attemptImageIntentInvalidated = true
		}
	}

	if account != nil && account.Type == AccountTypeOAuth {
		if rejectReason := detectOpenAIPassthroughInstructionsRejectReason(reqModel, body); rejectReason != "" {
			rejectMsg := "OpenAI codex passthrough requires a non-empty instructions field"
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			logOpenAIPassthroughInstructionsRejected(ctx, c, account, reqModel, rejectReason, body)
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "forbidden_error",
					"message": rejectMsg,
				},
			})
			return nil, fmt.Errorf("openai passthrough rejected before upstream: %s", rejectReason)
		}

		normalizedBody, normalized, err := normalizeOpenAIPassthroughOAuthBody(body, isOpenAIResponsesCompactPath(c))
		if err != nil {
			return nil, err
		}
		if normalized {
			body = normalizedBody
		}
		reqStream = gjson.GetBytes(body, "stream").Bool()
	}

	sanitizedBody, sanitized, err := sanitizeEmptyBase64InputImagesInOpenAIBody(body)
	if err != nil {
		return nil, err
	}
	if sanitized {
		body = sanitizedBody
	}

	// Apply OpenAI fast policy to the passthrough body (filter/block by service_tier).
	// 统一使用 upstream 视角的 model：透传路径下 body 已经过 compact 映射 +
	// OAuth normalize，body 中的 model 字段即上游真正会看到的 slug。
	// 这样可以与 chat-completions / messages / native /responses 入口的
	// upstreamModel 保持一致，避免 whitelist 命中差异。当 body 中没有
	// model 字段时退回 reqModel。
	policyModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if policyModel == "" {
		policyModel = reqModel
	}
	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, policyModel, body)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, policyErr
	}
	body = updatedBody

	apiKey := getAPIKeyFromContext(c)
	// 同一 attempt 的最终 model/body 只判定一次，权限检查与后续图片状态设置共用该结果。
	imageIntent := resolveOpenAIPassthroughImageIntent(
		c,
		reqModel,
		canonicalImageIntentBody,
		policyModel,
		body,
		attemptImageIntentInvalidated,
		IsImageGenerationIntent,
	)
	if imageIntent && !GroupAllowsImageGeneration(apiKeyGroup(apiKey)) {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "permission_error",
				"message": ImageGenerationPermissionMessage(),
			},
		})
		return nil, errors.New("image generation disabled for group")
	}
	imageBillingModel := ""
	imageSizeTier := ""
	imageInputSize := ""
	if imageIntent {
		var imageCfgErr error
		imageCfg, imageCfgErr := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, reqModel)
		if imageCfgErr != nil {
			setOpsUpstreamError(c, http.StatusBadRequest, imageCfgErr.Error(), "")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": imageCfgErr.Error(),
					"param":   "size",
				},
			})
			return nil, imageCfgErr
		}
		imageBillingModel = imageCfg.Model
		imageSizeTier = imageCfg.SizeTier
		imageInputSize = imageCfg.InputSize
	}

	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI 自动透传] 命中自动透传分支: account=%d name=%s type=%s model=%s stream=%v",
		account.ID,
		account.Name,
		account.Type,
		reqModel,
		reqStream,
	)
	if reqStream && c != nil && c.Request != nil {
		if timeoutHeaders := collectOpenAIPassthroughTimeoutHeaders(c.Request.Header); len(timeoutHeaders) > 0 {
			streamWarnLogger := logger.FromContext(ctx).With(
				zap.String("component", "service.openai_gateway"),
				zap.Int64("account_id", account.ID),
				zap.Strings("timeout_headers", timeoutHeaders),
			)
			if s.isOpenAIPassthroughTimeoutHeadersAllowed() {
				streamWarnLogger.Warn("OpenAI passthrough 透传请求包含超时相关请求头，且当前配置为放行，可能导致上游提前断流")
			} else {
				streamWarnLogger.Warn("OpenAI passthrough 检测到超时相关请求头，将按配置过滤以降低断流风险")
			}
		}
	}

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	if c != nil {
		c.Set("openai_passthrough", true)
	}

	agentTaskRecoveryTried := false
	var resp *http.Response
	for {
		upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
		upstreamReq, buildErr := s.buildUpstreamRequestOpenAIPassthrough(upstreamCtx, c, account, body, token)
		releaseUpstreamCtx()
		if buildErr != nil {
			return nil, buildErr
		}

		upstreamStart := time.Now()
		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if err != nil {
			// Transport-level failure (proxy/DNS/TCP/TLS — no HTTP response). Convert to
			// a failover so the handler switches to a healthy account.
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
		}
		if resp.StatusCode < 400 {
			break
		}

		// Peek only to identify an invalid task. Restore the body so the existing
		// passthrough error handling sees the same response after recovery fails.
		probeBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(probeBody))
		if !agentTaskRecoveryTried && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, probeBody) {
			agentTaskRecoveryTried = true
			expectedTaskID := account.GetCredential("task_id")
			if recoveryErr := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); recoveryErr != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
			}
			continue
		}

		// 透传模式默认保持原样代理；容量错误以及 API-key 上游的瞬时
		// 5xx 应先触发多账号 failover，且此时尚未写入下游响应。
		// probeBody 已在上方任务探测时读取过一次，直接复用避免重复读取。
		if shouldFailoverOpenAIPassthroughResponse(account, resp.StatusCode, probeBody) {
			return nil, s.handleFailoverErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
		}
		return nil, s.handleErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
	}
	defer func() { _ = resp.Body.Close() }()

	serviceTier := extractOpenAIServiceTierFromBody(body)

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	imageCount := 0
	var imageOutputSizes []string
	if reqStream {
		result, err := s.handleStreamingResponsePassthrough(ctx, resp, c, account, startTime, reqModel, upstreamPassthroughModel)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		firstTokenMs = result.firstTokenMs
		responseID = strings.TrimSpace(result.responseID)
		imageCount = result.imageCount
		imageOutputSizes = result.imageOutputSizes
	} else {
		result, err := s.handleNonStreamingResponsePassthrough(ctx, resp, c, reqModel, upstreamPassthroughModel)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		responseID = strings.TrimSpace(result.responseID)
		imageCount = result.imageCount
		imageOutputSizes = result.imageOutputSizes
	}
	s.bindHTTPResponseAccount(ctx, c, account, responseID)

	// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
	if !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	if usage == nil {
		usage = &OpenAIUsage{}
	}

	forwardResult := &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		ResponseID:      responseID,
		Usage:           *usage,
		Model:           reqModel,
		UpstreamModel:   upstreamPassthroughModel,
		ServiceTier:     serviceTier,
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}
	if imageCount > 0 {
		forwardResult.ImageCount = imageCount
		forwardResult.ImageSize = imageSizeTier
		forwardResult.ImageInputSize = imageInputSize
		forwardResult.ImageOutputSizes = imageOutputSizes
		forwardResult.BillingModel = imageBillingModel
	}
	return forwardResult, nil
}

func logOpenAIPassthroughInstructionsRejected(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	reqModel string,
	rejectReason string,
	body []byte,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	accountName := ""
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
		accountType = strings.TrimSpace(string(account.Type))
	}
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.String("account_type", accountType),
		zap.String("request_model", strings.TrimSpace(reqModel)),
		zap.String("reject_reason", strings.TrimSpace(rejectReason)),
	}
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
	logger.FromContext(ctx).With(fields...).Warn("OpenAI passthrough 本地拦截：Codex 请求缺少有效 instructions")
}

func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	targetURL := openaiPlatformAPIURL
	switch account.Type {
	case AccountTypeOAuth:
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		baseURL := account.GetOpenAIBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = buildOpenAIResponsesURL(validatedURL)
		}
	}
	targetURL = appendOpenAIResponsesRequestPathSuffix(targetURL, openAIResponsesRequestPathSuffix(c))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	// 透传客户端请求头（安全白名单）。
	allowTimeoutHeaders := s.isOpenAIPassthroughTimeoutHeadersAllowed()
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !isOpenAIPassthroughAllowedRequestHeader(lower, allowTimeoutHeaders) {
				continue
			}
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	// 覆盖入站鉴权残留，并注入上游认证
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// OAuth 透传到 ChatGPT internal API 时补齐必要头。
	if account.Type == AccountTypeOAuth {
		promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
		req.Host = "chatgpt.com"
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
			return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
		}
		apiKeyID := getAPIKeyIDFromContext(c)
		// 先保存客户端原始值，再做 compact 补充，避免后续统一隔离时读到已处理的值。
		clientSessionID := strings.TrimSpace(req.Header.Get("session_id"))
		clientConversationID := strings.TrimSpace(req.Header.Get("conversation_id"))
		if isOpenAIResponsesCompactPath(c) {
			req.Header.Set("accept", "application/json")
			if req.Header.Get("version") == "" {
				req.Header.Set("version", codexCLIVersion)
			}
			if clientSessionID == "" {
				clientSessionID = resolveOpenAICompactSessionID(c)
			}
		} else if req.Header.Get("accept") == "" {
			req.Header.Set("accept", "text/event-stream")
		}
		if req.Header.Get("OpenAI-Beta") == "" {
			req.Header.Set("OpenAI-Beta", "responses=experimental")
		}
		if req.Header.Get("originator") == "" {
			req.Header.Set("originator", "codex_cli_rs")
		}
		// 用隔离后的 session 标识符覆盖客户端透传值，防止跨用户会话碰撞。
		if clientSessionID == "" {
			clientSessionID = promptCacheKey
		}
		if clientConversationID == "" {
			clientConversationID = promptCacheKey
		}
		if clientSessionID != "" {
			req.Header.Set("session_id", isolateOpenAISessionID(apiKeyID, clientSessionID))
		}
		if clientConversationID != "" {
			req.Header.Set("conversation_id", isolateOpenAISessionID(apiKeyID, clientConversationID))
		}
	} else if isOpenAIResponsesCompactPath(c) {
		// 透传白名单会放行客户端的 Accept: text/event-stream；compact 上游是
		// unary JSON 协议，API-key 账号同样强制 Accept，避免上游按 SSE 返回
		// （#3777 期望行为 4）。
		req.Header.Set("accept", "application/json")
	}

	// 透传模式也支持账户自定义 User-Agent 与 ForceCodexCLI 兜底。
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}
	// 浏览器型 UA 兜底：仅 OAuth（ChatGPT 内部接口）账号生效，若最终 user-agent 仍为浏览器
	// （Chrome/Firefox/Safari/Edge 等），替换为后台配置的 Codex UA，避免 Cloudflare 触发 JS 质询。
	s.overrideBrowserUserAgent(ctx, account, req)

	// 终态收口：originator 必须与最终 User-Agent 首段配套且为官方身份，非官方 UA 整体回退为
	// 默认 Codex CLI 身份（承接原「非 Codex UA 安全兜底」，并修复其把 codex-tui 等官方 UA 改写为
	// codex_cli_rs 造成的 originator 错配 404），详见 issue #3901。
	if account.Type == AccountTypeOAuth {
		enforceCodexIdentityHeaders(req.Header)
	}

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)

	return req, nil
}

func shouldFailoverOpenAIPassthroughResponse(account *Account, statusCode int, responseBody []byte) bool {
	if isOpenAIContextWindowError("", responseBody) {
		return false
	}
	if isOpenAIRequestBodyTooLargeError(statusCode, "", responseBody) {
		return true
	}
	switch statusCode {
	case http.StatusTooManyRequests, 529:
		return true
	}
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	switch statusCode {
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		520, 521, 522, 523, 524:
		return true
	default:
		return false
	}
}

func writeOpenAIPassthroughErrorHeaders(dst, src http.Header) {
	if dst == nil {
		return
	}
	dst.Set("Content-Type", "application/json; charset=utf-8")
	dst.Set("Cache-Control", "no-store")
	dst.Del("Retry-After")
	if src == nil {
		return
	}
	rawRetryAfter := strings.TrimSpace(src.Get("Retry-After"))
	if validOpenAIPassthroughRetryAfter(rawRetryAfter, time.Now()) {
		dst.Set("Retry-After", rawRetryAfter)
	}
}

func validOpenAIPassthroughRetryAfter(raw string, now time.Time) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	delaySeconds := true
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			delaySeconds = false
			break
		}
	}
	if delaySeconds {
		seconds, err := strconv.ParseUint(raw, 10, 64)
		return err == nil && seconds > 0
	}
	parsed, err := http.ParseTime(raw)
	return err == nil && parsed.After(now)
}

func writeSanitizedOpenAIPassthroughError(c *gin.Context, upstreamStatus int, upstreamHeaders http.Header) {
	downstreamStatus := upstreamStatus
	message := "Upstream request failed"
	switch upstreamStatus {
	case http.StatusUnauthorized:
		downstreamStatus = http.StatusBadGateway
		message = "Upstream authentication failed"
	case http.StatusForbidden:
		downstreamStatus = http.StatusBadGateway
		message = "Upstream access denied"
	default:
		if upstreamStatus >= http.StatusInternalServerError {
			message = "Upstream service temporarily unavailable"
		}
	}
	writeOpenAIPassthroughErrorEnvelope(c, downstreamStatus, upstreamHeaders, message)
}

// writeOpenAIPassthroughErrorEnvelope 以本地 JSON 信封 + 净化后的头策略写出
// 错误响应；message 由调用方决定（净化通用文案或脱敏后的上游消息）。
func writeOpenAIPassthroughErrorEnvelope(c *gin.Context, downstreamStatus int, upstreamHeaders http.Header, message string) {
	if c == nil {
		return
	}
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	if writeOpenAICompactSSEBridge(c, downstreamStatus, body) {
		return
	}
	writeOpenAIPassthroughErrorHeaders(c.Writer.Header(), upstreamHeaders)
	c.Data(downstreamStatus, "application/json; charset=utf-8", body)
}

func (s *OpenAIGatewayService) handleFailoverErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
	responseBody []byte,
) error {
	body := s.redactAgentIdentitySensitiveBody(ctx, account, responseBody)

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	reqModel, _, _ := extractOpenAIRequestMetaFromBody(requestBody)
	canonicalModel := canonicalOpenAIAccountSchedulingModel(account, reqModel)
	shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, canonicalModel)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   resp.StatusCode,
		UpstreamRequestID:    resp.Header.Get("x-request-id"),
		Passthrough:          true,
		Kind:                 "failover",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})
	return newOpenAIUpstreamFailoverError(
		resp.StatusCode,
		resp.Header,
		body,
		upstreamMsg,
		!shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
	)
}

func (s *OpenAIGatewayService) handleErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
	responseBody []byte,
) error {
	MarkResponseCommitted(c)
	body := s.redactAgentIdentitySensitiveBody(ctx, account, responseBody)

	// cyber_policy 仍按原始 body 打内部标记，供 handler 事后写风控/邮件；面向客户端的
	// 错误体在下方统一重建。cyber 是上游网络安全策略拦截，不冷却账号，
	// 故下方跳过 handleOpenAIAccountUpstreamError（避免自定义 temp-unschedulable 规则误冷却）。
	cyberHit, cyberCode, cyberMsg := detectOpenAICyberPolicy(body)
	if cyberHit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code:           cyberCode,
			Message:        cyberMsg,
			Body:           truncateString(string(body), 4096),
			UpstreamStatus: resp.StatusCode,
		})
	}

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	// 错误体虽不会原样透传，运行态账号状态仍需更新，避免粘性路由继续复用
	// 刚被限流的账号。cyber 例外：不冷却账号。
	if !cyberHit {
		reqModel, _, _ := extractOpenAIRequestMetaFromBody(requestBody)
		canonicalModel := canonicalOpenAIAccountSchedulingModel(account, reqModel)
		_ = s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, canonicalModel)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   resp.StatusCode,
		UpstreamRequestID:    resp.Header.Get("x-request-id"),
		Passthrough:          true,
		Kind:                 "http_error",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})
	// context-window 超限是确定性请求失败（shouldFailoverOpenAIPassthroughResponse
	// 已保证不切号），其文案对客户端可操作（如触发自动压缩）；在净化信封内保留
	// 脱敏后的上游消息，而不是抹成通用文案。
	if isOpenAIContextWindowError(upstreamMsg, body) && upstreamMsg != "" {
		writeOpenAIPassthroughErrorEnvelope(c, resp.StatusCode, resp.Header, upstreamMsg)
	} else {
		writeSanitizedOpenAIPassthroughError(c, resp.StatusCode, resp.Header)
	}

	return fmt.Errorf("upstream error: %d (client response sanitized)", resp.StatusCode)
}

func isOpenAIPassthroughAllowedRequestHeader(lowerKey string, allowTimeoutHeaders bool) bool {
	if lowerKey == "" {
		return false
	}
	if isOpenAIPassthroughTimeoutHeader(lowerKey) {
		return allowTimeoutHeaders
	}
	return openaiPassthroughAllowedHeaders[lowerKey]
}

func isOpenAIPassthroughTimeoutHeader(lowerKey string) bool {
	switch lowerKey {
	case "x-stainless-timeout", "x-stainless-read-timeout", "x-stainless-connect-timeout", "x-request-timeout", "request-timeout", "grpc-timeout":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) isOpenAIPassthroughTimeoutHeadersAllowed() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIPassthroughAllowTimeoutHeaders
}

func collectOpenAIPassthroughTimeoutHeaders(h http.Header) []string {
	if h == nil {
		return nil
	}
	var matched []string
	for key, values := range h {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if isOpenAIPassthroughTimeoutHeader(lowerKey) {
			entry := lowerKey
			if len(values) > 0 {
				entry = fmt.Sprintf("%s=%s", lowerKey, strings.Join(values, "|"))
			}
			matched = append(matched, entry)
		}
	}
	sort.Strings(matched)
	return matched
}

type openaiStreamingResultPassthrough struct {
	usage            *OpenAIUsage
	firstTokenMs     *int
	responseID       string
	imageCount       int
	imageOutputSizes []string
}

type openaiNonStreamingResultPassthrough struct {
	*OpenAIUsage
	usage            *OpenAIUsage
	responseID       string
	imageCount       int
	imageOutputSizes []string
}

func openAIStreamClientOutputStarted(c *gin.Context, localStarted bool) bool {
	if localStarted {
		return true
	}
	return c != nil && c.Writer != nil && c.Writer.Written()
}

func openAIStreamEventIsPreamble(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.created", "response.in_progress":
		return true
	default:
		return false
	}
}

func openAIStreamDataStartsClientOutput(data, eventType string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if strings.TrimSpace(eventType) == "response.failed" {
		return false
	}
	return !openAIStreamEventIsPreamble(eventType)
}

func openAIStreamFailedEventSemanticStatus(payload []byte, message string) int {
	if isOpenAIContextWindowError(message, payload) {
		return http.StatusBadRequest
	}

	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.TrimSpace(errType + " " + code + " " + strings.ToLower(strings.TrimSpace(message)))
	switch {
	case strings.Contains(errType, "invalid_request"):
		return http.StatusBadRequest
	case strings.Contains(combined, "rate_limit"):
		return http.StatusTooManyRequests
	case strings.Contains(combined, "authentication") || strings.Contains(combined, "unauthorized") || strings.Contains(combined, "invalid_api_key"):
		return http.StatusUnauthorized
	case strings.Contains(combined, "permission") || strings.Contains(combined, "forbidden") || strings.Contains(combined, "access denied"):
		return http.StatusForbidden
	case code == "server_is_overloaded" || code == "slow_down":
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func openAIStreamFailedEventPassthroughBody(payload []byte, failedMessage string) []byte {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload
	}
	if gjson.GetBytes(payload, "error").Exists() {
		return payload
	}
	responseError := gjson.GetBytes(payload, "response.error")
	if !responseError.Exists() {
		if strings.TrimSpace(failedMessage) == "" {
			return payload
		}
		body, err := marshalOpenAIUpstreamJSON(gin.H{
			"error": gin.H{
				"message": failedMessage,
			},
		})
		if err != nil {
			return payload
		}
		return body
	}

	errorPayload := gin.H{}
	if errType := strings.TrimSpace(gjson.Get(responseError.Raw, "type").String()); errType != "" {
		errorPayload["type"] = errType
	}
	if code := strings.TrimSpace(gjson.Get(responseError.Raw, "code").String()); code != "" {
		errorPayload["code"] = code
	}
	if param := strings.TrimSpace(gjson.Get(responseError.Raw, "param").String()); param != "" {
		errorPayload["param"] = param
	}
	message := strings.TrimSpace(gjson.Get(responseError.Raw, "message").String())
	if message == "" {
		message = strings.TrimSpace(failedMessage)
	}
	if message != "" {
		errorPayload["message"] = message
	}
	if len(errorPayload) == 0 {
		return payload
	}
	body, err := marshalOpenAIUpstreamJSON(gin.H{"error": errorPayload})
	if err != nil {
		return payload
	}
	return body
}

// applyOpenAIStreamFailedErrorPassthroughRule 对 response.failed 事件应用错误透传规则：
// 归一化 body 供关键词匹配/消息提取，并推断语义状态码使按错误码配置的规则可以命中。
// platform 必须传 account.Platform——本服务同时承载 openai 与 grok 平台账号，规则按平台匹配。
func applyOpenAIStreamFailedErrorPassthroughRule(
	c *gin.Context,
	platform string,
	payload []byte,
	failedMessage string,
) (status int, errType string, errMsg string, matched bool) {
	ruleBody := openAIStreamFailedEventPassthroughBody(payload, failedMessage)
	upstreamStatus := openAIStreamFailedEventSemanticStatus(payload, failedMessage)
	return applyErrorPassthroughRule(
		c,
		platform,
		upstreamStatus,
		ruleBody,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	)
}

func openAIStreamFailedEventShouldFailover(payload []byte, message string) bool {
	if isOpenAIContextWindowError(message, payload) {
		return false
	}
	if isOpenAITransientProcessingError(http.StatusBadRequest, message, payload) {
		return true
	}
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.ToLower(strings.TrimSpace(message + " " + code + " " + errType))
	if combined == "" {
		return true
	}
	nonRetryableMarkers := []string{
		"invalid_request",
		"content_policy",
		"policy",
		"safety",
		"high-risk cyber",
		"not allowed",
		"violat",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(combined, marker) {
			return false
		}
	}
	return true
}

func (s *OpenAIGatewayService) recordOpenAIStreamUpstreamError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	kind string,
	payload []byte,
	message string,
) string {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI upstream response failed"
	}
	detail := ""
	if len(payload) > 0 && s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(payload), maxBytes)
	}
	if c != nil {
		setOpsUpstreamError(c, http.StatusBadGateway, message, detail)
		event := OpsUpstreamErrorEvent{
			Platform:           PlatformOpenAI,
			UpstreamStatusCode: http.StatusBadGateway,
			UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
			Passthrough:        passthrough,
			Kind:               kind,
			Message:            message,
			Detail:             detail,
		}
		if account != nil {
			event.Platform = account.Platform
			event.AccountID = account.ID
			event.AccountName = account.Name
		}
		appendOpsUpstreamError(c, event)
	}
	return message
}

func (s *OpenAIGatewayService) newOpenAIStreamFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI stream disconnected before completion"
	}
	message = s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "failover", payload, message)
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: body,
	}
}

func (s *OpenAIGatewayService) handleStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	originalModel string,
	mappedModel string,
) (*openaiStreamingResultPassthrough, error) {
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	responseID := ""
	clientDisconnected := false
	sawDone := false
	sawTerminalEvent := false
	sawFailedEvent := false
	failedMessage := ""
	clientOutputStarted := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	// pendingLines 在首个可见输出前保留前导事件，确保无输出失败仍可安全 failover。
	pendingLines := make([]string, 0, 8)
	// flushPending 表示已写入但未到 SSE 空行边界的脏状态；defer 兜底函数退出前的残留，断连后不再 Flush。
	flushPending := false
	flushPendingOutput := func() {
		if clientDisconnected || !flushPending {
			return
		}
		flusher.Flush()
		flushPending = false
	}
	defer flushPendingOutput()
	writePendingLines := func() bool {
		for _, pending := range pendingLines {
			if _, err := fmt.Fprintln(w, pending); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				return false
			}
		}
		pendingLines = pendingLines[:0]
		return true
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	defer putSSEScannerBuf64K(scanBuf)
	documentScanner := newOpenAISSEJSONDocumentScanner(scanner)

	needModelReplace := strings.TrimSpace(originalModel) != "" && strings.TrimSpace(mappedModel) != "" && strings.TrimSpace(originalModel) != strings.TrimSpace(mappedModel)
	resultWithUsage := func() *openaiStreamingResultPassthrough {
		return &openaiStreamingResultPassthrough{
			usage:            usage,
			firstTokenMs:     firstTokenMs,
			responseID:       responseID,
			imageCount:       imageCounter.Count(),
			imageOutputSizes: imageCounter.Sizes(),
		}
	}

	for documentScanner.Scan() {
		line := documentScanner.Text()
		lineStartsClientOutput := false
		forceFlushFailedEvent := false
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes := []byte(data)
			trimmedData := strings.TrimSpace(data)
			if needModelReplace && strings.Contains(data, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
				if replacedData, replaced := extractOpenAISSEDataLine(line); replaced {
					dataBytes = []byte(replacedData)
					trimmedData = strings.TrimSpace(replacedData)
				}
			}
			if normalizedData, normalized := normalizeOpenAIResponsesFunctionCallArguments(dataBytes); normalized {
				dataBytes = normalizedData
				trimmedData = strings.TrimSpace(string(normalizedData))
				line = "data: " + string(normalizedData)
			}
			if normalizedData, normalized := normalizeCompletedImageGenerationStatus(dataBytes); normalized {
				dataBytes = normalizedData
				trimmedData = strings.TrimSpace(string(normalizedData))
				line = "data: " + string(normalizedData)
			}
			if trimmedData != "[DONE]" {
				restoredData, restoreErr := restoreOpenAIResponsesNamespacePayload(c, dataBytes)
				if restoreErr != nil {
					return resultWithUsage(), fmt.Errorf("restore OpenAI passthrough namespace response: %w", restoreErr)
				}
				if !bytes.Equal(restoredData, dataBytes) {
					dataBytes = restoredData
					trimmedData = strings.TrimSpace(string(restoredData))
					line = "data: " + string(restoredData)
				}
			}
			eventType := strings.TrimSpace(gjson.Get(trimmedData, "type").String())
			if eventType == "response.failed" {
				failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				// response.failed 自带上游已消耗的 usage（input token 通常已扣）；必须先解析
				// 再打 cyber 标记，否则 mark 记到的是解析前的 0，导致流式 cyber 按 0 token 计费
				// 而漏记真实用量。对齐 WS V2 / Chat 流式路径（均先解析 usage 再 Mark）。
				s.parseSSEUsageBytes(dataBytes, usage)
				if hit, code, msg := detectOpenAICyberPolicy(dataBytes); hit {
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:           code,
						Message:        msg,
						Body:           truncateString(string(dataBytes), 4096),
						UpstreamStatus: http.StatusOK,
						UpstreamInTok:  usage.InputTokens,
						UpstreamOutTok: usage.OutputTokens,
					})
				}
				if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
					if status, errType, errMsg, matched := applyOpenAIStreamFailedErrorPassthroughRule(c, account.Platform, dataBytes, failedMessage); matched {
						// 命中透传规则也要记录 ops 上游错误事件（对齐 CC/Messages 与
						// antigravity 先例），否则透传命中的 failed 在监控中不可见。
						s.recordOpenAIStreamUpstreamError(c, account, true, upstreamRequestID, "http_error", dataBytes, failedMessage)
						MarkResponseCommitted(c)
						c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
						c.JSON(status, gin.H{
							"error": gin.H{
								"type":    errType,
								"message": errMsg,
							},
						})
						return resultWithUsage(), fmt.Errorf("upstream response failed: passthrough rule matched message=%s", errMsg)
					}
					if openAIStreamFailedEventShouldFailover(dataBytes, failedMessage) {
						return resultWithUsage(),
							s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, dataBytes, failedMessage)
					}
				}
				forceFlushFailedEvent = true
				sawFailedEvent = true
			}
			if trimmedData == "[DONE]" {
				sawDone = true
			}
			if openAIStreamEventIsTerminal(trimmedData) {
				sawTerminalEvent = true
			}
			if responseID == "" {
				responseID = extractOpenAIResponseIDFromJSONBytes(dataBytes)
			}
			imageCounter.AddSSEData(dataBytes)
			if sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
				dataBytes,
				eventType,
				openAIStreamClientOutputStarted(c, clientOutputStarted),
			); sanitized {
				dataBytes = sanitizedData
				trimmedData = strings.TrimSpace(string(sanitizedData))
				line = "data: " + string(sanitizedData)
			}
			lineStartsClientOutput = forceFlushFailedEvent || openAIStreamDataStartsClientOutput(trimmedData, eventType)
			if firstTokenMs == nil && lineStartsClientOutput && trimmedData != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			s.parseSSEUsageBytes(dataBytes, usage)
		}

		if !clientDisconnected {
			if !clientOutputStarted && !lineStartsClientOutput {
				pendingLines = append(pendingLines, line)
				continue
			}
			if !clientOutputStarted && len(pendingLines) > 0 {
				if !writePendingLines() {
					continue
				}
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
			} else {
				clientOutputStarted = true
				flushPending = true
				if line == "" {
					flushPendingOutput()
				}
			}
		}
	}
	if err := documentScanner.Err(); err != nil {
		if (sawDone || sawTerminalEvent) && !sawFailedEvent {
			s.clearOpenAIProxyStreamDisconnect(account)
			return resultWithUsage(), nil
		}
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return resultWithUsage(), err
		}
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
			msg := "OpenAI stream disconnected before completion"
			if errText := strings.TrimSpace(err.Error()); errText != "" {
				msg += ": " + errText
			}
			return resultWithUsage(),
				s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, msg)
		}
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", err)
		}
		s.recordOpenAIProxyStreamDisconnect(account, err, upstreamRequestID)
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI passthrough] 流读取异常中断: account=%d request_id=%s err=%v",
			account.ID,
			upstreamRequestID,
			err,
		)
		return resultWithUsage(), fmt.Errorf("stream read error: %w", err)
	}
	if sawFailedEvent {
		return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
	}
	if !clientDisconnected && !sawDone && !sawTerminalEvent && ctx.Err() == nil {
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.String("upstream_request_id", upstreamRequestID),
		).Info("OpenAI passthrough 上游流在未收到 [DONE] 时结束，疑似断流")
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
			return resultWithUsage(),
				s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, "OpenAI stream ended before a terminal event")
		}
		s.recordOpenAIProxyStreamDisconnect(account, errors.New("stream ended before terminal event"), upstreamRequestID)
		return resultWithUsage(), errors.New("stream usage incomplete: missing terminal event")
	}
	if (sawDone || sawTerminalEvent) && !sawFailedEvent {
		s.clearOpenAIProxyStreamDisconnect(account)
	}

	return resultWithUsage(), nil
}

func (s *OpenAIGatewayService) handleNonStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
) (*openaiNonStreamingResultPassthrough, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	// Detect SSE responses from upstream and convert to JSON.
	// Some upstreams (e.g. other sub2api instances) may return SSE even when
	// stream=false was requested. Without this conversion the client would
	// receive raw SSE text or a terminal event with empty output.
	if isEventStreamResponse(resp.Header) {
		return s.handlePassthroughSSEToJSON(resp, c, body, originalModel, mappedModel)
	}

	usage := &OpenAIUsage{}
	usageParsed := false
	if len(body) > 0 {
		if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
			*usage = parsedUsage
			usageParsed = true
		}
	}
	if !usageParsed {
		// 兜底：尝试从 SSE 文本中解析 usage
		usage = s.parseSSEUsageFromBody(string(body))
	}

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}
	body, err = restoreOpenAIResponsesNamespacePayload(c, body)
	if err != nil {
		return nil, fmt.Errorf("restore OpenAI passthrough namespace response: %w", err)
	}
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
	}
	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIResponseImageOutputsFromJSONBytes(body),
		imageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(body),
	}, nil
}

// handlePassthroughSSEToJSON converts an SSE response body into a JSON
// response for the passthrough path. It mirrors handleSSEToJSON while
// preserving passthrough payloads, except compact-only model remapping may
// rewrite model fields back to the original requested model.
func (s *OpenAIGatewayService) handlePassthroughSSEToJSON(resp *http.Response, c *gin.Context, body []byte, originalModel string, mappedModel string) (*openaiNonStreamingResultPassthrough, error) {
	bodyText := string(body)
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := &OpenAIUsage{}
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
		}
		// When the terminal event has an empty output array, reconstruct
		// output from accumulated delta events so the client gets full content.
		if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
			if outputJSON, reconstructed := reconstructResponseOutputFromSSE(bodyText); reconstructed {
				if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
					finalResponse = patched
				}
			}
		}
		finalResponse = supplementCompactionItemFromSSE(c, finalResponse, bodyText)
		body = finalResponse
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
		}
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
		restoredBody, restoreErr := restoreOpenAIResponsesNamespacePayload(c, body)
		if restoreErr != nil {
			return nil, fmt.Errorf("restore OpenAI passthrough namespace response: %w", restoreErr)
		}
		body = restoredBody
	} else {
		terminalType, terminalPayload, terminalOK := extractOpenAISSETerminalEvent(bodyText)
		if terminalOK && terminalType == "response.failed" {
			msg := extractOpenAISSEErrorMessage(terminalPayload)
			if msg == "" {
				msg = "Upstream compact response failed"
			}
			return nil, s.writeOpenAINonStreamingProtocolError(resp, c, msg)
		}
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
		}
		body = []byte(bodyText)
	}

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
		}
	}
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
	}

	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIImageOutputsFromSSEBody(bodyText),
		imageOutputSizes: collectOpenAIImageOutputSizesFromSSEBody(bodyText),
	}, nil
}

func writeOpenAIPassthroughResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
	}
	if filter != nil {
		responseheaders.WriteFilteredHeaders(dst, src, filter)
	} else {
		// 兜底：尽量保留最基础的 content-type
		if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
			dst.Set("Content-Type", v)
		}
	}
	// 透传模式强制放行 x-codex-* 响应头（若上游返回）。
	// 注意：真实 http.Response.Header 的 key 一般会被 canonicalize；但为了兼容测试/自建响应，
	// 这里用 EqualFold 做一次大小写不敏感的查找。
	getCaseInsensitiveValues := func(h http.Header, want string) []string {
		if h == nil {
			return nil
		}
		for k, vals := range h {
			if strings.EqualFold(k, want) {
				return vals
			}
		}
		return nil
	}

	for _, rawKey := range []string{
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
	} {
		vals := getCaseInsensitiveValues(src, rawKey)
		if len(vals) == 0 {
			continue
		}
		key := http.CanonicalHeaderKey(rawKey)
		dst.Del(key)
		for _, v := range vals {
			dst.Add(key, v)
		}
	}
}
