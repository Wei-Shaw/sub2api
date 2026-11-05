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

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const openAIPassthroughTransportRetryReason GatewayFailureReason = "openai_passthrough_transport_retry"

func markOpenAIPassthroughTransportRetry(err *UpstreamFailoverError) {
	if err == nil {
		return
	}
	err.RetryableOnSameAccount = true
	err.Reason = openAIPassthroughTransportRetryReason
}

const openAIPassthroughPreOutputBufferLimit = openAIStreamPreOutputBufferLimit

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
	responseHeaderSnapshot := cloneOpenAIUncommittedResponseHeaders(c)
	budget := openAIPassthroughBudgetForContext(c)
	replaySafe := openAIPassthroughRequestReplaySafe(
		c,
		reqModel,
		body,
		canonicalImageIntentBody,
		attemptImageIntentInvalidated,
	)
	resetOpenAITransportRetryState(c)
	agentTaskRecoveryTried := false
	var fingerprintIDs *codexFingerprintIDs
	if account != nil && account.Type == AccountTypeOAuth &&
		(!isOpenAIResponsesCompactPath(c) || s.codexFullSimulationEnabledForAccount(c, account)) {
		fingerprintIDs = resolveCodexFingerprintIDsFromGinContext(account, c)
	}
	// Reset the request-scoped plan even when this attempt uses an API-key or
	// legacy compact account. Failover reuses the Gin context across attempts.
	stageCodexFingerprintIDs(c, fingerprintIDs)
	for completedRetries := 0; ; completedRetries++ {
		if completedRetries > 0 && ctx != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
		}
		result, err := s.forwardOpenAIPassthroughOnce(
			ctx,
			c,
			account,
			body,
			canonicalImageIntentBody,
			reqModel,
			attemptImageIntentInvalidated,
			reasoningEffort,
			reqStream,
			startTime,
			&agentTaskRecoveryTried,
			fingerprintIDs,
			budget,
			replaySafe,
		)
		err = normalizeOpenAIPassthroughRetrySafety(err, replaySafe)
		if err == nil {
			if replaySafe && openAITransportTimeoutPending(c) {
				s.ClearAccountSchedulingBlockIfReason(account.ID, "transport_timeout")
			}
			resetOpenAITransportRetryState(c)
			return result, nil
		}
		restoreOpenAIUncommittedResponseHeaders(c, responseHeaderSnapshot)

		if ctx != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
		}
		var failoverErr *UpstreamFailoverError
		if !errors.As(err, &failoverErr) || !failoverErr.RetryableOnSameAccount {
			return result, err
		}
		if failoverErr.Reason != openAIPassthroughTransportRetryReason {
			return result, err
		}
		if completedRetries >= len(openAIPassthroughTransportRetryBackoffs) {
			// The service has exhausted the same-account retry budget. The handler
			// must switch accounts immediately instead of applying its pool retry.
			failoverErr.RetryableOnSameAccount = false
			if replaySafe && openAITransportTimeoutPending(c) {
				s.BlockAccountScheduling(account, time.Now().Add(openAITransportTimeoutRuntimeCooldown), "transport_timeout")
			}
			resetOpenAITransportRetryState(c)
			return result, err
		}

		delay := openAIPassthroughTransportRetryBackoffs[completedRetries]
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.Int("retry_count", completedRetries+1),
			zap.Int("retry_max", len(openAIPassthroughTransportRetryBackoffs)),
			zap.Duration("retry_delay", delay),
			zap.Error(err),
		).Warn("openai.passthrough_response_retry")
		if waitErr := waitOpenAITransportRetry(ctx, delay); waitErr != nil {
			return result, waitErr
		}
	}
}

func (s *OpenAIGatewayService) forwardOpenAIPassthroughOnce(
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
	agentTaskRecoveryTried *bool,
	fingerprintIDs *codexFingerprintIDs,
	budget *openAIPassthroughRetryBudget,
	replaySafe bool,
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
	if account != nil && account.Platform == PlatformOpenAI {
		if sanitized, changed, sanitizeErr := sanitizeOpenAIResponsesAccessPrograms(body); sanitizeErr != nil {
			return nil, sanitizeErr
		} else if changed {
			body = sanitized
		}
	}
	promptCacheKey := openAIPassthroughTurnStateKey(c, account, body)

	if account != nil && account.Type == AccountTypeOAuth {

		if isOpenAICodexModel(reqModel) && !gjson.GetBytes(body, "instructions").Exists() {
			resolvedModel := normalizeOpenAIModelForUpstream(account, reqModel)
			if resolvedModel == "" {
				resolvedModel = reqModel
			}
			nextBody, setErr := sjson.SetBytes(body, "instructions", defaultCodexSynthInstructions(resolvedModel))
			if setErr != nil {
				return nil, fmt.Errorf("set passthrough codex instructions: %w", setErr)
			}
			body = nextBody
		}

		normalizedBody, normalized, err := normalizeOpenAIPassthroughOAuthBody(body, isOpenAIResponsesCompactPath(c))
		if err != nil {
			return nil, err
		}
		if normalized {
			body = normalizedBody
		}
		if account.IsOpenAIOAuth() {
			responsesLite := isOpenAIResponsesLiteHeader(c.GetHeader(responsesLiteHeader)) || isOpenAIResponsesLiteWebSocketPayload(body)
			replayBody, replayChanged, replayErr := normalizeOpenAIOAuthReplayHistoryForAccount(body, account, isOpenAIResponsesCompactPath(c), responsesLite)
			if replayErr != nil {
				return nil, fmt.Errorf("normalize passthrough Responses replay history: %w", replayErr)
			}
			if replayChanged {
				body = replayBody
			}
		}
		if !isOpenAIResponsesCompactPath(c) || (fingerprintIDs != nil && fingerprintIDs.fullSimulation) {
			fingerprintedBody, fingerprinted, fingerprintErr := applyCodexFingerprintClientMetadataToBody(body, fingerprintIDs, c)
			if fingerprintErr != nil {
				return nil, fingerprintErr
			}
			if fingerprinted {
				body = fingerprintedBody
			}
		}
		// The OAuth upstream requires stream=true, but the downstream response
		// shape follows the client's original stream flag. The caller already
		// captured that flag before normalization; compact requests are always
		// unary.
		if isOpenAIResponsesCompactPath(c) {
			reqStream = false
		}
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

	// Protect the credential lookup as well as the later upstream header wait.
	defer s.startOpenAIResponsePreparationKeepalive(ctx, c, account, reqStream)()
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

	var resp *http.Response
	reasoningEffortValue := ""
	if reasoningEffort != nil {
		reasoningEffortValue = strings.TrimSpace(*reasoningEffort)
	}
	firstOutputTimeout := time.Duration(0)
	if reqStream && account != nil && account.Platform == PlatformOpenAI {
		firstOutputTimeout = s.openAIFirstOutputTimeoutWithContext(ctx, reasoningEffortValue)
	}
	openAIAuthRecoveryTried := false
	for {
		upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
		var headerGuard *openAIFirstOutputHeaderGuard
		if firstOutputTimeout > 0 {
			upstreamCtx, headerGuard = newOpenAIFirstOutputHeaderGuard(
				upstreamCtx, releaseUpstreamCtx, startTime.Add(firstOutputTimeout),
			)
		}
		syncOpenAIResponsesInstructionsEcho(c, body)
		upstreamReq, buildErr := s.buildUpstreamRequestOpenAIPassthroughWithFingerprint(upstreamCtx, c, account, body, token, fingerprintIDs)
		if headerGuard == nil {
			releaseUpstreamCtx()
		}
		if buildErr != nil {
			if headerGuard != nil {
				headerGuard.close()
			}
			return nil, buildErr
		}
		attempt, maxAttempts, reserved := budget.reserve()
		if !reserved {
			used, limit := budget.snapshot()
			if used == 0 {
				used = attempt
			}
			if limit == 0 {
				limit = maxAttempts
			}
			if headerGuard != nil {
				headerGuard.close()
			}
			if replaySafe && openAITransportTimeoutPending(c) {
				s.BlockAccountScheduling(account, time.Now().Add(openAITransportTimeoutRuntimeCooldown), "transport_timeout")
				resetOpenAITransportRetryState(c)
			}
			logger.FromContext(ctx).With(
				zap.String("component", "service.openai_gateway"),
				zap.Int64("account_id", account.ID),
				zap.Int("attempts_used", used),
				zap.Int("attempt_budget", limit),
			).Warn("openai.passthrough_attempt_budget_exhausted")
			return nil, budget.exhaustedError()
		}

		upstreamStart := time.Now()
		resp, err = s.doOpenAIResponsesUpstream(ctx, c, upstreamReq, proxyURL, account, reqStream)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if headerGuard != nil && headerGuard.stopHeaderWait() {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			headerGuard.close()
			return nil, s.newOpenAIFirstOutputTimeoutError(
				ctx, c, account, startTime, reqModel, reasoningEffortValue,
				firstOutputTimeout, "passthrough_response_headers", nil,
			)
		}
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if headerGuard != nil {
				headerGuard.close()
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// Transport-level failure (proxy/DNS/TCP/TLS — no HTTP response). Convert to
			// a failover so the handler switches to a healthy account, and temporarily
			// unschedule the account on durable faults (e.g. rejected proxy credentials).
			transportCtx := ctx
			if replaySafe && isOpenAITransportTimeout(err) && !IsOpenAIStreamResponseHeaderTimeout(err) {
				markOpenAITransportTimeoutPending(c)
				transportCtx = withOpenAITransportRetryCandidate(ctx)
			}
			failoverErr := s.handleOpenAIUpstreamTransportError(transportCtx, c, account, err, true)
			if typed, ok := failoverErr.(*UpstreamFailoverError); ok && !classifyOpenAITransportError(err).Persistent {
				markOpenAIPassthroughTransportRetry(typed)
			}
			return nil, failoverErr
		}
		if headerGuard != nil {
			resp.Body = &openAIRequestContextReadCloser{ReadCloser: resp.Body, cleanup: headerGuard.close}
		}
		if resp.StatusCode < http.StatusBadRequest {
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			_ = resp.Body.Close()
			return nil, ctxErr
		}

		// Peek only to identify an invalid task. Restore the body so the existing
		// passthrough error handling sees the same response after recovery fails.
		probeBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(probeBody))
		if agentTaskRecoveryTried != nil && !*agentTaskRecoveryTried && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, probeBody) {
			*agentTaskRecoveryTried = true
			expectedTaskID := account.GetCredential("task_id")
			if recoveryErr := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); recoveryErr != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
			}
			continue
		}
		if !openAIAuthRecoveryTried && s.openAITokenProvider != nil &&
			shouldRefreshOpenAIOAuth401(account, resp.StatusCode, probeBody) {
			openAIAuthRecoveryTried = true
			if refreshErr := s.openAITokenProvider.RefreshAfterUnauthorized(ctx, account); refreshErr == nil {
				_ = resp.Body.Close()
				token, _, err = s.GetAccessToken(ctx, account)
				if err != nil {
					return nil, err
				}
				continue
			}
		}

		// Retry account failures before committing downstream output. Deterministic
		// request errors are returned directly without exhausting the account pool.
		if shouldFailoverOpenAIPassthroughResponse(account, resp.StatusCode, probeBody) {
			return nil, s.handleFailoverErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
		}
		return nil, s.handleErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
	}
	defer func() { _ = resp.Body.Close() }()
	serviceTier := extractOpenAIServiceTierFromBody(body)
	responseCtx := ctx
	if replaySafe {
		responseCtx = withOpenAITransportRetryCandidate(ctx)
	}

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	imageCount := 0
	var imageOutputSizes []string
	if reqStream {
		result, err := s.handleStreamingResponsePassthrough(
			responseCtx, resp, c, account, startTime, reqModel, upstreamPassthroughModel, reasoningEffortValue,
		)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		firstTokenMs = result.firstTokenMs
		responseID = strings.TrimSpace(result.responseID)
		imageCount = result.imageCount
		imageOutputSizes = result.imageOutputSizes
	} else {
		result, err := s.handleNonStreamingResponsePassthrough(responseCtx, resp, c, account, reqModel, upstreamPassthroughModel)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		responseID = strings.TrimSpace(result.responseID)
		imageCount = result.imageCount
		imageOutputSizes = result.imageOutputSizes
	}
	if account.IsOpenAIOAuth() {
		if turnState := extractOpenAICodexTurnState(resp.Header); turnState != "" {
			s.bindOpenAICompatSessionTurnState(ctx, c, account, promptCacheKey, turnState)
			s.bindOpenAIHTTPSharedTurnState(ctx, c, account, body, turnState)
		}
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
		RequestID:                     resp.Header.Get("x-request-id"),
		ResponseID:                    responseID,
		Usage:                         *usage,
		Model:                         reqModel,
		UpstreamModel:                 upstreamPassthroughModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		ServiceTier:                   serviceTier,
		ReasoningEffort:               reasoningEffort,
		Stream:                        reqStream,
		OpenAIWSMode:                  false,
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
		OpenAITiming:                  observedOpenAITiming(c),
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

func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	return s.buildUpstreamRequestOpenAIPassthroughWithFingerprint(ctx, c, account, body, token, nil)
}

func openAIPassthroughTurnStateKey(c *gin.Context, account *Account, body []byte) string {
	key := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	if key != "" || account == nil || !account.IsOpenAIOAuth() {
		return key
	}
	ids := resolveCodexOutboundSessionIDs(c, account, body, "")
	if ids == nil {
		return ""
	}
	return strings.TrimSpace(ids.sessionID)
}

func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthroughWithFingerprint(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	fingerprintIDs *codexFingerprintIDs,
) (*http.Request, error) {
	// Failover reuses the request context; replace any prior account's plan
	// before headers are assembled so identities cannot cross account attempts.
	stageCodexFingerprintIDs(c, fingerprintIDs)
	targetURL := openaiPlatformAPIURL
	var workspaceRouting *openAIWorkspaceRouting
	useOfficialCodexEndpoint := false
	switch account.Type {
	case AccountTypeOAuth:
		var err error
		targetURL, useOfficialCodexEndpoint, err = s.openAIOAuthCodexTargetURLWithContext(ctx, account)
		if err != nil {
			return nil, err
		}
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
	if account.IsOpenAIOAuth() {
		var routingErr error
		workspaceRouting, routingErr = s.resolveOpenAIWorkspaceRouting(ctx, account, token)
		if routingErr != nil {
			return nil, newOpenAIWorkspaceRoutingFailoverError(routingErr)
		}
		if workspaceRouting != nil {
			var urlErr error
			targetURL, urlErr = applyOpenAIWorkspaceRoutingURL(targetURL, workspaceRouting, true)
			if urlErr != nil {
				return nil, newOpenAIWorkspaceRoutingFailoverError(urlErr)
			}
		}
	}

	promptCacheKey := openAIPassthroughTurnStateKey(c, account, body)
	outboundBody := body
	var codexSessionIDs *codexOutboundSessionIDs
	if account.IsOpenAIOAuth() && (fingerprintIDs == nil || fingerprintIDs.mode == codexFingerprintOff) {
		codexSessionIDs = resolveCodexOutboundSessionIDs(c, account, body, promptCacheKey)
		var rewriteErr error
		outboundBody, rewriteErr = rewriteCodexOutboundSessionMetadata(body, account, codexSessionIDs, c)
		if rewriteErr != nil {
			return nil, rewriteErr
		}
	}
	s.observeCodexEncryptedContentPayload(ctx, c, account, gjson.GetBytes(outboundBody, "model").String(), outboundBody, "http_passthrough_request")

	req, err := newOpenAIHTTPUpstreamRequest(ctx, http.MethodPost, targetURL, account, outboundBody)
	if err != nil {
		return nil, err
	}
	stream := gjson.GetBytes(outboundBody, "stream").Bool()
	requestCtx := WithHTTPUpstreamProfile(req.Context(), openAIHTTPUpstreamProfile(ctx, account, stream))
	requestCtx = withOpenAIWorkspaceRoutingRedirectPolicy(requestCtx, workspaceRouting)
	req = req.WithContext(requestCtx)
	req = req.WithContext(WithHTTPUpstreamTLSProfile(req.Context(), s.resolveTLSProfile(account)))

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
	if account.IsOpenAIOAuth() && s.CodexSimulationRequestEnabled(c) {
		req.Header.Del(CodexProjectIDHeader)
	}
	// A pre-semantic keepalive may commit downstream HTTP headers before this
	// response's turn state is available to the client. Keep the opaque state
	// server-side as a per-account/session binding and restore it on the next
	// OAuth request instead of relying on a late HTTP header mutation.
	if account.IsOpenAIOAuth() && strings.TrimSpace(req.Header.Get(openAICodexTurnStateHeader)) == "" {
		savedTurnState := s.getOpenAICompatSessionTurnState(ctx, c, account, promptCacheKey)
		if savedTurnState == "" {
			savedTurnState = s.getOpenAIHTTPSharedTurnState(ctx, c, account, body)
		}
		if savedTurnState != "" {
			req.Header.Set(openAICodexTurnStateHeader, savedTurnState)
		}
	}
	// Failover can reuse the downstream turn-state with a different account.
	// Strip only values whose provenance is known to be cross-account.
	s.guardOpenAICodexTurnStateEcho(c, account, req.Header)
	stageOpenAICodexTurnStateModel(c, gjson.GetBytes(outboundBody, "model").String())
	s.applyConfiguredCodexTurnStateReplay(c, account, req.Header)

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
		// Current Codex OAuth HTTP no longer negotiates the legacy Responses
		// experiment. Passthrough may receive it from an older client, so remove
		// only that token while preserving any independent beta negotiation.
		stripOpenAILegacyResponsesBeta(req.Header)
		if useOfficialCodexEndpoint && isOfficialChatGPTCodexURL(targetURL) {
			req.Host = "chatgpt.com"
		}
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
				req.Header.Set("version", CodexCanonicalClientVersion())
			}
			if clientSessionID == "" {
				clientSessionID = resolveOpenAICompactSessionID(c)
			}
		} else if req.Header.Get("accept") == "" {
			req.Header.Set("accept", "text/event-stream")
		}
		if req.Header.Get("originator") == "" {
			req.Header.Set("originator", resolveCodexOutboundIdentity("").originator)
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
		applyResolvedCodexOutboundSessionHeaders(c, account, req.Header, fingerprintIDs, codexSessionIDs)
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
		req.Header.Set("user-agent", CodexCanonicalUserAgent())
	}
	if account.Type == AccountTypeOAuth {
		if !isOpenAIResponsesCompactPath(c) || (fingerprintIDs != nil && fingerprintIDs.fullSimulation) {
			applyStagedCodexFingerprintHeaders(c, account, req.Header)
		}
		enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
	}

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)
	applyOpenCodeSessionHeader(c, account, targetURL, req.Header, body)
	applyOpenAICodexBetaFeatures(c, account, req.Header)
	applyOpenAICodexRoutingHintFromBody(ctx, account, "http_passthrough", req.Header, outboundBody, "not_applicable")
	applyCodexSimulationProfileHeaders(req.Header, fingerprintIDs)
	applyOpenAICodexSemanticRequestHeaders(req.Header, c, account, outboundBody)
	applyOpenAIWorkspaceRoutingHeader(req.Header, workspaceRouting)
	normalizeOpenAICodexHTTPProtocolHeaders(req, c, account)
	req = req.WithContext(withOpenAIProofIdentity(req.Context(), newOpenAIProofIdentityFromHeaders(account, fingerprintIDs, req.Header)))

	return req, nil
}

func stripOpenAILegacyResponsesBeta(headers http.Header) {
	if headers == nil {
		return
	}

	preserved := make([]string, 0)
	for key, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), "OpenAI-Beta") {
			continue
		}
		delete(headers, key)
		for _, value := range values {
			parts := strings.Split(value, ",")
			kept := parts[:0]
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" || strings.EqualFold(part, "responses=experimental") {
					continue
				}
				kept = append(kept, part)
			}
			if len(kept) > 0 {
				preserved = append(preserved, strings.Join(kept, ", "))
			}
		}
	}
	for _, value := range preserved {
		headers.Add("OpenAI-Beta", value)
	}
}

func shouldFailoverOpenAIPassthroughResponse(account *Account, statusCode int, responseBody []byte) bool {
	if statusCode < http.StatusBadRequest {
		return false
	}
	if _, policy := openAIUpstreamPolicyErrorDetails(responseBody); policy {
		return false
	}
	if statusCode == http.StatusBadRequest && isOpenAIContextWindowError("", responseBody) {
		return false
	}
	// Providers sometimes return the gateway's generic envelope with HTTP 400.
	// It carries no request-specific remediation and must stay inside the
	// bounded account failover loop instead of being committed as a client 400.
	if IsOpenAIGenericUpstreamFailureBody(responseBody) {
		return true
	}
	if isOpenAIRequestBodyTooLargeError(statusCode, "", responseBody) {
		return true
	}
	if account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode) {
		return true
	}
	switch statusCode {
	case http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity:
		// These normally describe the request itself. Trying every account only
		// multiplies latency and upstream traffic for a deterministic failure.
		return false
	case http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests:
		return true
	default:
		return statusCode >= http.StatusInternalServerError
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
	downstreamStatus, message := sanitizedOpenAIPassthroughErrorStatusAndMessage(upstreamStatus)
	writeOpenAIPassthroughErrorEnvelope(c, downstreamStatus, upstreamHeaders, message)
}

func sanitizedOpenAIPassthroughErrorStatusAndMessage(upstreamStatus int) (int, string) {
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
	return downstreamStatus, message
}

func marshalOpenAIPassthroughErrorEnvelope(message string) []byte {
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	return body
}

// writeOpenAIPassthroughErrorEnvelope 以本地 JSON 信封 + 净化后的头策略写出
// 错误响应；message 由调用方决定（净化通用文案或脱敏后的上游消息）。
func writeOpenAIPassthroughErrorEnvelope(c *gin.Context, downstreamStatus int, upstreamHeaders http.Header, message string) {
	if c == nil {
		return
	}
	if writeOpenAIResponsesErrorAfterKeepalive(c, downstreamStatus, "upstream_error", message) {
		return
	}
	body := marshalOpenAIPassthroughErrorEnvelope(message)
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
	MarkOpenAIVerificationRecommendation(c, body, resp.StatusCode)
	genericUpstreamFailure := IsOpenAIGenericUpstreamFailureBody(body)
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
		Kind:                 "failover",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})
	failoverErr := newOpenAIUpstreamFailoverError(
		resp.StatusCode,
		resp.Header,
		body,
		upstreamMsg,
		account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
	)
	_, clientMessage := sanitizedOpenAIPassthroughErrorStatusAndMessage(resp.StatusCode)
	if isOpenAIContextWindowError(upstreamMsg, body) && upstreamMsg != "" {
		clientMessage = upstreamMsg
	}
	clientHeaders := make(http.Header)
	writeOpenAIPassthroughErrorHeaders(clientHeaders, resp.Header)
	failoverErr.ResponseBody = marshalOpenAIPassthroughErrorEnvelope(clientMessage)
	failoverErr.ResponseHeaders = clientHeaders
	// The generic envelope is an internal retry sentinel, not useful upstream
	// diagnostics. Keep the body for classification/ops, but force the handler
	// to use its local terminal message after all accounts are exhausted.
	failoverErr.PreserveUpstreamResponse = !genericUpstreamFailure
	return failoverErr
}

func (s *OpenAIGatewayService) handleErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
	responseBody []byte,
) error {
	model, _, _ := extractOpenAIRequestMetaFromBody(requestBody)
	s.observeCodexEncryptedContentPayload(ctx, c, account, model, responseBody, "http_failure")
	MarkResponseCommitted(c)
	body := s.redactAgentIdentitySensitiveBody(ctx, account, responseBody)
	MarkOpenAIVerificationRecommendation(c, body, resp.StatusCode)

	if policyErr, handled := writeOpenAIUpstreamPolicyError(c, resp.StatusCode, body); handled {
		return policyErr
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
	// Policy rejections and context overflow do not affect account health.
	contextWindowClientError := resp.StatusCode == http.StatusBadRequest && isOpenAIContextWindowError(upstreamMsg, body)
	if !contextWindowClientError {
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
	if IsOpenAIGenericUpstreamFailureBody(body) {
		writeOpenAIPassthroughErrorEnvelope(
			c,
			http.StatusBadGateway,
			resp.Header,
			OpenAIGenericUpstreamFailureClientMessage,
		)
		return fmt.Errorf("upstream error: %d (generic failure sanitized)", resp.StatusCode)
	}

	// context-window 超限是确定性请求失败（shouldFailoverOpenAIPassthroughResponse
	// 已保证不切号），其文案对客户端可操作（如触发自动压缩）；在净化信封内保留
	// 脱敏后的上游消息，而不是抹成通用文案。
	if contextWindowClientError {
		writeOpenAIPassthroughErrorHeaders(c.Writer.Header(), resp.Header)
		writeOpenAIUpstreamClientError(c, resp.StatusCode, body, upstreamMsg)
	} else if isOpenAIContextWindowError(upstreamMsg, body) && upstreamMsg != "" {
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

const openAIStreamKeepaliveBytesKey = "openai_stream_keepalive_bytes"
const openAIStreamLastWriteKey = "openai_stream_last_write"
const openAIStreamControlOutputBytesKey = "openai_stream_control_output_bytes"
const openAIStreamSemanticOutputBytesKey = "openai_stream_semantic_output_bytes"

func addOpenAIStreamOutputBytes(c *gin.Context, key string, written int) {
	if c == nil || written <= 0 {
		return
	}
	current := 0
	if value, ok := c.Get(key); ok {
		current, _ = value.(int)
	}
	c.Set(key, current+written)
}

func recordOpenAIStreamControlOutput(c *gin.Context, written int) {
	addOpenAIStreamOutputBytes(c, openAIStreamControlOutputBytesKey, written)
}

func recordOpenAIStreamSemanticOutput(c *gin.Context, written int) {
	addOpenAIStreamOutputBytes(c, openAIStreamSemanticOutputBytesKey, written)
}

func openAIStreamControlOnlyOutputCommitted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	controlBytes, _ := c.Get(openAIStreamControlOutputBytesKey)
	semanticBytes, _ := c.Get(openAIStreamSemanticOutputBytesKey)
	control, _ := controlBytes.(int)
	semantic, _ := semanticBytes.(int)
	return control > 0 && semantic == 0 && OpenAICompactKeepaliveAdjustedWrittenSize(c) < 0
}

func recordOpenAIStreamKeepaliveBytes(c *gin.Context, written int) {
	if c == nil || written <= 0 {
		return
	}
	current := 0
	if value, ok := c.Get(openAIStreamKeepaliveBytesKey); ok {
		current, _ = value.(int)
	}
	c.Set(openAIStreamKeepaliveBytesKey, current+written)
	c.Set(openAIStreamLastWriteKey, time.Now())
}

func openAIStreamKeepaliveDelay(c *gin.Context, interval time.Duration) time.Duration {
	delay := time.Until(openAIStreamLastWriteAt(c).Add(interval))
	if delay <= 0 {
		return time.Nanosecond
	}
	return delay
}

func openAIStreamLastWriteAt(c *gin.Context) time.Time {
	if c == nil {
		return time.Now()
	}
	if value, ok := c.Get(openAIStreamLastWriteKey); ok {
		if lastWrite, valid := value.(time.Time); valid {
			return lastWrite
		}
	}
	now := time.Now()
	c.Set(openAIStreamLastWriteKey, now)
	return now
}

func openAIStreamClientOutputStarted(c *gin.Context, localStarted bool) bool {
	if localStarted {
		return true
	}
	if openAIStreamControlOnlyOutputCommitted(c) {
		// The first Codex rate-limit frame is deliberately flushed for
		// perceived latency, but it remains replay-safe and is not semantic
		// output for failover decisions.
		return false
	}
	if c == nil || c.Writer == nil {
		return false
	}
	// Compact keepalives commit HTTP 200 but are not semantic model output.
	return OpenAICompactKeepaliveAdjustedWrittenSize(c) >= 0
}

func openAIStreamEventIsPreambleTrimmed(eventType string) bool {
	switch eventType {
	case "response.created", "response.queued", "response.in_progress", "response.metadata", "codex.rate_limits", "codex.response.metadata":
		// Transport metadata is not model output, even when carried in data frames.
		return true
	default:
		return false
	}
}

func openAIStreamEventIsReplaySafeStructureTrimmed(eventType string) bool {
	switch eventType {
	case "response.output_item.added",
		"response.output_item.done",
		"response.content_part.added",
		"response.content_part.done",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_part.done":
		return true
	default:
		return false
	}
}

var (
	openAIStreamDeltaSemanticFields = []string{
		"delta", "text", "arguments", "summary", "content", "audio",
	}
	openAIStreamToolDoneSemanticFields  = []string{"arguments", "delta", "input"}
	openAIStreamStructureSemanticFields = []string{
		"delta", "text", "arguments", "summary", "content", "audio", "encrypted_content", "result",
		"item.arguments", "item.text", "item.content", "item.summary", "item.encrypted_content", "item.result",
	}
)

func openAIStreamJSONValueHasSemanticValue(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	switch value.Type {
	case gjson.String:
		return strings.TrimSpace(value.String()) != ""
	case gjson.Number, gjson.True:
		return true
	case gjson.JSON:
		raw := strings.TrimSpace(value.Raw)
		return raw != "" && raw != "null" && raw != "[]" && raw != "{}"
	default:
		return false
	}
}

func openAIStreamDataHasSemanticField(data string, path string) bool {
	return openAIStreamJSONValueHasSemanticValue(gjson.Get(data, path))
}

func openAIStreamDataHasAnySemanticField(data string, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	if len(paths) == 1 {
		return openAIStreamDataHasSemanticField(data, paths[0])
	}
	for _, path := range paths {
		value := gjson.Get(data, path)
		if openAIStreamJSONValueHasSemanticValue(value) {
			return true
		}
	}
	return false
}

func openAIStreamStructureDataHasSemanticOutput(data, eventType string) bool {
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		// The common empty message preamble is structurally replay-safe. Keep a
		// compact-wire fast path; non-compact or richer items use the validated
		// structural inspection below.
		if strings.Contains(data, `"item":{"type":"message","content":[]}`) {
			return false
		}
		item := gjson.Get(data, "item")
		if !item.Exists() {
			return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
		}
		itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
		if openAIStreamItemTypeIsReplayUnsafe(itemType) {
			return true
		}
		switch itemType {
		case "message":
			return openAIStreamJSONValueHasSemanticValue(item.Get("content")) ||
				openAIStreamJSONValueHasSemanticValue(item.Get("text"))
		case "reasoning":
			return openAIStreamJSONValueHasSemanticValue(item.Get("summary")) ||
				openAIStreamJSONValueHasSemanticValue(item.Get("encrypted_content")) ||
				openAIStreamJSONValueHasSemanticValue(item.Get("content"))
		}
		return openAIStreamJSONValueHasSemanticValue(item.Get("arguments")) ||
			openAIStreamJSONValueHasSemanticValue(item.Get("result")) ||
			openAIStreamJSONValueHasSemanticValue(item.Get("content"))
	case "response.content_part.added", "response.content_part.done":
		part := gjson.Get(data, "part")
		if part.Exists() {
			return openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("content")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("audio")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("transcript"))
		}
		return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		part := gjson.Get(data, "part")
		if part.Exists() {
			return openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("summary")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("content"))
		}
		return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
	default:
		return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
	}
}

func openAIStreamItemTypeIsReplayUnsafe(itemType string) bool {
	return strings.Contains(itemType, "function_call") ||
		strings.Contains(itemType, "tool_call") ||
		strings.Contains(itemType, "computer_call") ||
		strings.Contains(itemType, "custom_tool")
}

func openAIStreamItemIsReplayUnsafe(data string) bool {
	itemType := strings.ToLower(strings.TrimSpace(gjson.Get(data, "item.type").String()))
	return openAIStreamItemTypeIsReplayUnsafe(itemType)
}

func openAIStreamItemHasVisibleOutput(item gjson.Result) bool {
	if openAIStreamJSONValueHasSemanticValue(item.Get("arguments")) ||
		openAIStreamJSONValueHasSemanticValue(item.Get("input")) ||
		openAIStreamJSONValueHasSemanticValue(item.Get("result")) {
		return true
	}
	for _, path := range []string{"content", "summary"} {
		for _, part := range item.Get(path).Array() {
			if openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("transcript")) {
				return true
			}
		}
	}
	return false
}

// Output progress and replay safety remain separate from TTFT. This predicate
// records latency only when a client-usable value is present.
func openAIStreamDataStartsVisibleOutput(data, eventType string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" {
		return false
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = strings.TrimSpace(gjson.Get(trimmed, "type").String())
	}
	if strings.HasSuffix(eventType, ".delta") {
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "delta"))
	}
	switch eventType {
	case "response.output_text.done",
		"response.reasoning_summary_text.done",
		"response.reasoning_text.done",
		"response.audio_transcript.done":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "text"))
	case "response.function_call_arguments.done":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "arguments"))
	case "response.custom_tool_call_input.done":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "input"))
	case "response.image_generation_call.partial_image":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "partial_image_b64"))
	case "response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		part := gjson.Get(trimmed, "part")
		return openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
			openAIStreamJSONValueHasSemanticValue(part.Get("transcript"))
	case "response.output_item.added", "response.output_item.done":
		return openAIStreamItemHasVisibleOutput(gjson.Get(trimmed, "item"))
	case "response.completed", "response.done":
		for _, item := range gjson.Get(trimmed, "response.output").Array() {
			if openAIStreamItemHasVisibleOutput(item) {
				return true
			}
		}
	}
	return false
}

func openAIStreamDataStartsClientOutput(data, eventType string) bool {
	return openAIStreamDataStartsClientOutputTrimmed(strings.TrimSpace(data), strings.TrimSpace(eventType))
}

func openAIStreamDataStartsClientOutputTrimmed(trimmed, eventType string) bool {
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return false
	}
	if eventType == "response.failed" || eventType == "error" || openAIStreamEventTypeIsTerminal(eventType) {
		return false
	}
	if openAIStreamEventIsPreambleTrimmed(eventType) {
		return false
	}
	if strings.Contains(eventType, "metadata") {
		return false
	}
	if openAIStreamEventIsReplaySafeStructureTrimmed(eventType) {
		return openAIStreamStructureDataHasSemanticOutput(trimmed, eventType)
	}

	if strings.Contains(eventType, ".delta") {
		// These are the overwhelmingly common Responses events. A non-empty
		// delta is enough to classify semantic output; event-boundary flushing
		// performs the separate malformed-payload check when the value is empty.
		switch eventType {
		case "response.output_text.delta", "response.function_call_arguments.delta",
			"response.reasoning_summary_text.delta", "response.reasoning_content.delta",
			"response.audio.delta":
			return openAIStreamDataHasSemanticField(trimmed, "delta")
		default:
			return openAIStreamDataHasAnySemanticField(trimmed, openAIStreamDeltaSemanticFields)
		}
	}

	if strings.HasSuffix(eventType, ".done") {
		// output_item.done may carry a complete tool call with an empty
		// arguments string. Its item identity is already irreversible output.
		if openAIStreamItemIsReplayUnsafe(trimmed) {
			return true
		}
		if strings.Contains(eventType, "function_call") || strings.Contains(eventType, "tool_call") || strings.Contains(eventType, "computer_call") {
			return openAIStreamDataHasAnySemanticField(trimmed, openAIStreamToolDoneSemanticFields) ||
				strings.TrimSpace(gjson.Get(trimmed, "name").String()) != "" ||
				strings.TrimSpace(gjson.Get(trimmed, "call_id").String()) != ""
		}
		return openAIStreamDataHasAnySemanticField(trimmed, openAIStreamStructureSemanticFields)
	}

	// Unknown event families are conservatively treated as semantic output.
	// This keeps newly introduced upstream event types fail-closed without a
	// validation scan on the hot path.
	return true
}

// openAIStreamDataSignalsOutputProgressTrimmed preserves the historical distinction
// between a pure response.created/in_progress preamble and a stream that has
// entered an output phase. Progress alone is still replay-safe for an explicit
// response.failed capacity signal, but an ambiguous transport EOF must retain
// the existing incomplete-stream behavior.
func openAIStreamDataSignalsOutputProgressTrimmed(trimmed, eventType string) bool {
	if trimmed == "" || trimmed == "[DONE]" {
		return false
	}
	if eventType == "response.failed" || eventType == "error" || openAIStreamEventTypeIsTerminal(eventType) {
		return false
	}
	return !openAIStreamEventIsPreambleTrimmed(eventType)
}

func openAIStreamEventNeedsFlushKnownValidity(trimmed, eventType string, semanticOutput, forceFailureBoundary, payloadValid bool) bool {
	if semanticOutput || forceFailureBoundary {
		return true
	}
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" || openAIStreamEventTypeIsTerminal(eventType) {
		return true
	}
	if eventType == "error" {
		return true
	}
	// A complete but malformed/unknown SSE data event cannot safely remain
	// hidden behind a later event's flush.
	return !payloadValid
}

func openAIStreamFailedEventErrorCode(payload []byte) string {
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	return code
}

func isOpenAIUpstreamCapacityShedEvent(payload []byte) bool {
	switch openAIStreamFailedEventErrorCode(payload) {
	case "server_is_overloaded", "slow_down":
		return true
	}
	for _, path := range []string{"response.error.message", "error.message", "message"} {
		if isOpenAICapacityShedMessage(gjson.GetBytes(payload, path).String()) {
			return true
		}
	}
	return false
}

func logOpenAICapacityFailoverSuppressed(
	ctx context.Context,
	account *Account,
	path string,
	upstreamRequestID string,
	eventType string,
) {
	fields := []zap.Field{
		zap.String("path", path),
		zap.String("event_type", strings.TrimSpace(eventType)),
		zap.String("upstream_request_id", strings.TrimSpace(upstreamRequestID)),
	}
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("platform", account.Platform),
		)
	}
	logger.FromContext(ctx).Warn("gateway.failover_suppressed_after_semantic_output", fields...)
}

const openAICapacityShedRetryableClientCode = "server_error"

// sanitizeOpenAICapacityShedErrorCodeForClient only changes the copy sent to
// the client. Account health and failover classification continue to use the
// original upstream payload.
func sanitizeOpenAICapacityShedErrorCodeForClient(payload []byte) ([]byte, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) || !isOpenAIUpstreamCapacityShedEvent(payload) {
		return payload, false
	}
	updated := payload
	changed := false
	for _, path := range []string{"response.error.code", "error.code"} {
		parent := strings.TrimSuffix(path, ".code")
		if !gjson.GetBytes(updated, parent).Exists() {
			continue
		}
		code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(updated, path).String()))
		if code != "" && code != "server_is_overloaded" && code != "slow_down" {
			continue
		}
		next, err := sjson.SetBytes(updated, path, openAICapacityShedRetryableClientCode)
		if err != nil {
			return payload, false
		}
		updated = next
		changed = true
	}
	return updated, changed
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
	if account != nil {
		observeCtx := context.Background()
		if c != nil && c.Request != nil {
			observeCtx = c.Request.Context()
		}
		s.observeCodexEncryptedContentPayload(observeCtx, c, account, openAICodexTurnStateModel(c), payload, "upstream_error")
	}
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI upstream response failed"
	}
	statusCode := openAIStreamFailureStatus(payload, message)
	detail := ""
	if len(payload) > 0 && s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(payload), maxBytes)
	}
	if c != nil {
		setOpsUpstreamError(c, statusCode, message, detail)
		event := OpsUpstreamErrorEvent{
			Platform:           PlatformOpenAI,
			UpstreamStatusCode: statusCode,
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
	responseHeaders ...http.Header,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI stream disconnected before completion"
	}
	statusCode := openAIStreamFailureStatus(payload, message)
	var headers http.Header
	if len(responseHeaders) > 0 && responseHeaders[0] != nil {
		headers = responseHeaders[0].Clone()
	}
	message = s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "failover", payload, message)
	errType := "upstream_error"
	if statusCode == http.StatusTooManyRequests {
		errType = "rate_limit_error"
	}
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	failoverErr := &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		ResponseHeaders:        headers,
		RetryableOnSameAccount: openAIStreamFailedEventRetryableOnSameAccount(account, payload, message),
	}
	requestScopedCapacity := isOpenAIUpstreamCapacityShedEvent(payload)
	if requestScopedCapacity {
		failoverErr.Scope = GatewayFailureScopeRequest
		failoverErr.SameAccountRetryDelay = openAIRequestScopedRetryDelay(headers, payload)
	} else if statusCode == http.StatusBadGateway && shouldCountOpenAIStreamBadGatewayFailure(payload, message) &&
		s != nil && s.rateLimitService != nil && account != nil {
		failureCtx := context.Background()
		if c != nil && c.Request != nil {
			failureCtx = c.Request.Context()
		}
		s.rateLimitService.maybeAutoDisableOpenAIAccountOnFailure(failureCtx, account, statusCode, headers, payload)
	}
	return failoverErr
}

func shouldCountOpenAIStreamBadGatewayFailure(payload []byte, message string) bool {
	if IsOpenAIGenericUpstreamFailureBody(payload) {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "stream disconnected before completion") ||
		strings.Contains(lower, "stream ended before a terminal event") ||
		strings.Contains(lower, "upstream connection error")
}

func (s *OpenAIGatewayService) newOpenAIStreamClientError(
	c *gin.Context,
	account *Account,
	upstreamRequestID string,
	statusCode int,
	errType string,
	message string,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI request failed"
	}
	errType = strings.TrimSpace(errType)
	if errType == "" {
		errType = "invalid_request_error"
	}
	if statusCode < 400 || statusCode >= 500 {
		statusCode = http.StatusBadRequest
	}
	message = s.recordOpenAIStreamUpstreamError(c, account, false, upstreamRequestID, "client_error", nil, message)
	fields := []zap.Field{
		zap.String("upstream_request_id", strings.TrimSpace(upstreamRequestID)),
		zap.Int("status_code", statusCode),
		zap.String("error_type", errType),
		zap.String("message", message),
		zap.Bool("retryable_on_same_account", false),
	}
	if c != nil && c.Request != nil {
		if requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			fields = append(fields, zap.String("request_id", strings.TrimSpace(requestID)))
		}
		if clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			fields = append(fields, zap.String("client_request_id", strings.TrimSpace(clientRequestID)))
		}
	}
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("account_platform", account.Platform),
		)
	}
	logger.L().Warn("openai_messages.stream_client_error", fields...)
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		RetryableOnSameAccount: false,
	}
}

func (s *OpenAIGatewayService) newRetryableOpenAIStreamFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
) *UpstreamFailoverError {
	err := s.newOpenAIStreamFailoverError(c, account, passthrough, upstreamRequestID, payload, message)
	markOpenAIPassthroughTransportRetry(err)
	return err
}

func (s *OpenAIGatewayService) handleStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	originalModel string,
	mappedModel string,
	reasoningEfforts ...string,
) (*openaiStreamingResultPassthrough, error) {
	beginOpenAITimingObservation(c)
	visibleOutputTTFT := s.useOpenAIVisibleOutputTTFT(ctx)
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	preservePreOutputForFailover := account != nil && account.Platform == PlatformOpenAI
	var attemptResponseHeaders http.Header
	if preservePreOutputForFailover {
		attemptResponseHeaders = make(http.Header)
		writeOpenAIPassthroughResponseHeaders(attemptResponseHeaders, resp.Header, s.responseHeaderFilter)
		declareOpenAIStreamResponseMetadataTrailers(c)
	} else {
		writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		if preservePreOutputForFailover {
			attemptResponseHeaders.Set("x-request-id", v)
		} else {
			c.Header("x-request-id", v)
		}
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	// Control-plane Codex events can be the first upstream data. Keep their
	// local timestamp separate until this attempt reaches a non-failed terminal
	// event, so a retryable attempt does not become a successful TTFT sample.
	var localFirstEventTTFTMs *int
	visibleOutputObserved := false
	responseID := ""
	clientDisconnected := false
	sawDone := false
	sawTerminalEvent := false
	sawFailedEvent := false
	successfulTerminalEvent := false
	sawResponseFailedEvent := false
	pendingErrorEvent := false
	responsesSemanticOutputSeen := false
	capacityFailoverSuppressedLogged := false
	failedMessage := ""
	clientOutputStarted := false
	controlEventInProgress := false
	sawOutputProgressEvent := false
	seenUpstreamDataEvent := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	reasoningEffort := ""
	if len(reasoningEfforts) > 0 {
		reasoningEffort = strings.TrimSpace(reasoningEfforts[0])
	}
	firstOutputTimeout := time.Duration(0)
	if account != nil && account.Platform == PlatformOpenAI {
		firstOutputTimeout = s.openAIFirstOutputTimeoutWithContext(ctx, reasoningEffort)
	}
	var firstOutputTimer *time.Timer
	var firstOutputCh <-chan time.Time
	if firstOutputTimeout > 0 {
		remaining := time.Until(startTime.Add(firstOutputTimeout))
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		firstOutputTimer = time.NewTimer(remaining)
		firstOutputCh = firstOutputTimer.C
	}
	stopFirstOutputWatchdog := func() {
		if firstOutputTimer == nil {
			return
		}
		if !firstOutputTimer.Stop() {
			select {
			case <-firstOutputTimer.C:
			default:
			}
		}
		firstOutputTimer = nil
		firstOutputCh = nil
	}
	defer stopFirstOutputWatchdog()

	keepaliveInterval := time.Duration(0)
	keepaliveInterval = s.openAIStreamKeepaliveIntervalWithContext(ctx)
	var keepaliveTimer *time.Timer
	var keepaliveCh <-chan time.Time
	if keepaliveInterval > 0 {
		keepaliveTimer = time.NewTimer(openAIStreamKeepaliveDelay(c, keepaliveInterval))
		keepaliveCh = keepaliveTimer.C
		defer keepaliveTimer.Stop()
	}
	lastDownstreamWriteAt := openAIStreamLastWriteAt(c)
	downstreamEventInProgress := false
	// pendingLines/stage 在首个语义输出前保留前导事件；rate_limits 是可重放
	// 的控制帧，会单独即时下发，不影响其余前导事件的 failover 安全性。
	var preOutputStage *openAIFirstOutputStage
	if preservePreOutputForFailover {
		preOutputStage = newDefaultOpenAIFirstOutputStage()
		defer func() {
			if preOutputStage == nil {
				return
			}
			if err := preOutputStage.Close(); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough first-output staging cleanup failed: account=%d error=%v", account.ID, err)
			}
		}()
	}
	pendingLines := make([]string, 0, 16)
	pendingBytes := 0
	pendingOutputBytes := func() int64 {
		if preOutputStage != nil {
			return preOutputStage.Buffered()
		}
		return int64(pendingBytes)
	}
	turnStateNoted := false
	applyAttemptResponseHeaders := func() {
		if !preservePreOutputForFailover || len(attemptResponseHeaders) == 0 {
			return
		}
		if c.Writer.Written() {
			setOpenAIStreamResponseMetadataTrailers(c, attemptResponseHeaders)
			return
		}
		for key, values := range attemptResponseHeaders {
			c.Writer.Header().Del(key)
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
	}
	noteTurnStateCommitted := func() {
		if turnStateNoted || extractOpenAICodexTurnState(resp.Header) == "" {
			return
		}
		s.noteStagedOpenAICodexTurnStateCommitted(c, account, resp.Header)
		turnStateNoted = true
	}
	// flushPending 表示已写入但未到 SSE 空行边界的脏状态；defer 兜底函数退出前的残留，断连后不再 Flush。
	flushPending := false
	flushPendingOutput := func() {
		if clientDisconnected || !flushPending {
			return
		}
		flusher.Flush()
		lastDownstreamWriteAt = time.Now()
		flushPending = false
	}
	defer flushPendingOutput()
	writePendingLines := func() bool {
		applyAttemptResponseHeaders()
		if preOutputStage != nil {
			if preOutputStage.Buffered() == 0 {
				return true
			}
			stagedBytes := preOutputStage.Buffered()
			if err := preOutputStage.CommitTo(w); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during staged output commit: account=%d error=%v", account.ID, err)
				return false
			}
			preOutputStage = nil
			noteTurnStateCommitted()
			recordOpenAIStreamSemanticOutput(c, int(stagedBytes))
			return true
		}
		pendingEndedEvent := len(pendingLines) == 0 || pendingLines[len(pendingLines)-1] == ""
		for _, pending := range pendingLines {
			if _, err := fmt.Fprintln(w, pending); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				return false
			}
		}
		noteTurnStateCommitted()
		pendingLines = pendingLines[:0]
		pendingBytes = 0
		downstreamEventInProgress = !pendingEndedEvent
		return true
	}
	commitPendingOutputProgress := func() {
		if clientDisconnected || clientOutputStarted || !sawOutputProgressEvent || pendingOutputBytes() == 0 {
			return
		}
		if writePendingLines() {
			clientOutputStarted = true
			flushPending = true
			flushPendingOutput()
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	documentScanner := newOpenAISSEJSONDocumentScanner(scanner)
	type passthroughScanEvent struct {
		line      string
		dataValid bool
		err       error
	}
	scanCtx, cancelScan := context.WithCancel(context.Background())
	defer cancelScan()
	events := make(chan passthroughScanEvent, openAIFirstOutputEventQueueSize(firstOutputTimeout > 0))
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for documentScanner.Scan() {
			event := passthroughScanEvent{
				line:      documentScanner.Text(),
				dataValid: documentScanner.DataValid(),
			}
			select {
			case events <- event:
			case <-scanCtx.Done():
				return
			}
		}
		if err := documentScanner.Err(); err != nil {
			select {
			case events <- passthroughScanEvent{err: err}:
			case <-scanCtx.Done():
			}
		}
	}(scanBuf)

	needModelReplace := strings.TrimSpace(originalModel) != "" && strings.TrimSpace(mappedModel) != "" && strings.TrimSpace(originalModel) != strings.TrimSpace(mappedModel)
	resultWithUsage := func() *openaiStreamingResultPassthrough {
		if localFirstEventTTFTMs != nil && successfulTerminalEvent {
			firstTokenMs = localFirstEventTTFTMs
		}
		return &openaiStreamingResultPassthrough{
			usage:            usage,
			firstTokenMs:     firstTokenMs,
			responseID:       responseID,
			imageCount:       imageCounter.BillableCount(),
			imageOutputSizes: imageCounter.Sizes(),
		}
	}
	var scanErr error
	streamEnded := false
	for !streamEnded {
		var line string
		dataValid := false
		select {
		case event, ok := <-events:
			if !ok {
				streamEnded = true
				continue
			}
			if event.err != nil {
				scanErr = event.err
				streamEnded = true
				continue
			}
			line = event.line
			dataValid = event.dataValid
		case <-firstOutputCh:
			_ = resp.Body.Close()
			cancelScan()
			return resultWithUsage(), s.newOpenAIFirstOutputTimeoutError(
				ctx, c, account, startTime, originalModel, reasoningEffort,
				firstOutputTimeout, "passthrough_semantic_output", resp.Header,
			)
		case <-keepaliveCh:
			keepaliveTimer.Reset(keepaliveInterval)
			if clientDisconnected || downstreamEventInProgress || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			n, writeErr := w.Write([]byte(":\n\n"))
			recordOpenAIStreamKeepaliveBytes(c, n)
			if writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during keepalive, continue draining upstream for usage: account=%d", account.ID)
				continue
			}
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
			continue
		}
		if isOpenAISSEErrorEventLine(line) {
			pendingErrorEvent = true
			continue
		}
		if line == "" {
			pendingErrorEvent = false
		}
		lineStartsClientOutput := false
		lineNeedsFlush := false
		forceFlushFailedEvent := false
		immediateFirstEvent := false
		if data, ok := extractOpenAISSEDataLine(line); ok {
			firstUpstreamDataEvent := !seenUpstreamDataEvent
			seenUpstreamDataEvent = true
			dataBytes := []byte(data)
			normalizedData, normalizedPolicyEvent := normalizeOpenAIPolicyErrorEvent(dataBytes)
			if normalizedPolicyEvent {
				dataBytes = normalizedData
				data = string(normalizedData)
				line = "data: " + data
			}
			trimmedData := strings.TrimSpace(data)
			MarkOpenAIVerificationRecommendation(c, dataBytes, http.StatusOK)
			rawEventType := strings.TrimSpace(gjson.GetBytes(dataBytes, "type").String())
			observer.ObserveOpenAI(dataBytes, rawEventType)
			observeOpenAITiming(c, dataBytes, rawEventType)
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
				restoredData = restoreOpenAIResponsesInstructionsEchoInEvent(c, restoredData, gjson.GetBytes(dataBytes, "type").String())
				if !bytes.Equal(restoredData, dataBytes) {
					dataBytes = restoredData
					trimmedData = strings.TrimSpace(string(restoredData))
					line = "data: " + string(restoredData)
				}
			}
			eventType := strings.TrimSpace(gjson.Get(trimmedData, "type").String())
			captureOpenAICodexTurnStateMetadata(resp.Header, dataBytes)
			s.observeCodexEncryptedContentPayload(ctx, c, account, openAICodexTurnStateModel(c), dataBytes, "http_passthrough_sse")
			if openAIStreamDataSignalsOutputProgressTrimmed(trimmedData, eventType) {
				sawOutputProgressEvent = true
			}
			failureEnvelopeMessage := ""
			isFailureEnvelope := eventType == "response.failed"
			if eventType == "error" {
				failureEnvelopeMessage = extractOpenAISSEErrorMessage(dataBytes)
				isFailureEnvelope = openAIStreamFailureIsExplicitlyRetryable(dataBytes, failureEnvelopeMessage)
			}
			if isFailureEnvelope {
				sawResponseFailedEvent = eventType == "response.failed"
				failedMessage = failureEnvelopeMessage
				if failedMessage == "" {
					failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				}
				// response.failed 自带上游已消耗的 usage（input token 通常已扣）；必须先解析
				// 再打 cyber 标记，否则 mark 记到的是解析前的 0，导致流式 cyber 按 0 token 计费
				// 而漏记真实用量。对齐 WS V2 / Chat 流式路径（均先解析 usage 再 Mark）。
				s.parseSSEUsageBytes(dataBytes, usage)
				if hit, code, msg := detectOpenAICyberPolicy(dataBytes); hit {
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:            code,
						Message:         msg,
						Body:            truncateString(string(dataBytes), 4096),
						UpstreamStatus:  http.StatusOK,
						UpstreamInTok:   usage.InputTokens,
						UpstreamOutTok:  usage.OutputTokens,
						ReasoningTokens: usage.ReasoningTokens,
					})
				}
				clientHadSemanticOutput := openAIStreamClientOutputStarted(c, clientOutputStarted)
				if !clientHadSemanticOutput {
					if handledErr, handled := s.handleOpenAIResponseFailedBeforeOutput(
						c, account, true, upstreamRequestID, dataBytes, failedMessage, resp.Header,
					); handled {
						if handledErr != nil {
							return resultWithUsage(), handledErr
						}
						return resultWithUsage(), fmt.Errorf("upstream response failed")
					}
				} else if openAIStreamFailureIsExplicitlyRetryable(dataBytes, failedMessage) {
					s.recordOpenAIStreamUpstreamError(c, account, true, upstreamRequestID, "http_error", dataBytes, failedMessage)
					if !capacityFailoverSuppressedLogged && isOpenAIUpstreamCapacityShedEvent(dataBytes) {
						logOpenAICapacityFailoverSuppressed(ctx, account, "passthrough_sse", upstreamRequestID, eventType)
						capacityFailoverSuppressedLogged = true
					}
					accountID := int64(0)
					if account != nil {
						accountID = account.ID
					}
					logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough response.failed after semantic output; failover skipped: response already committed account=%d request_id=%s", accountID, upstreamRequestID)
				}
				forceFlushFailedEvent = true
				sawFailedEvent = true
				if eventType == "error" {
					sawTerminalEvent = true
				}
			}
			if trimmedData == "[DONE]" {
				sawDone = true
			}
			if openAIStreamEventIsTerminal(trimmedData) {
				sawTerminalEvent = true
				if !isFailureEnvelope && (eventType == "response.completed" || eventType == "response.done" || trimmedData == "[DONE]") {
					successfulTerminalEvent = true
				}
				stopFirstOutputWatchdog()
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
			line = openAIPolicySSEEventHeader(line, pendingErrorEvent, normalizedPolicyEvent)
			pendingErrorEvent = false
			startsLocalFirstEventTTFT := openAIStreamDataStartsLocalFirstEventTTFT(trimmedData, eventType)
			immediateFirstEvent = firstUpstreamDataEvent && isOpenAIImmediateFirstEventType(eventType)
			if !clientOutputStarted || !visibleOutputObserved {
				lineStartsClientOutput = openAIStreamDataStartsClientOutputTrimmed(trimmedData, eventType)
				lineNeedsFlush = openAIStreamEventNeedsFlushKnownValidity(
					trimmedData,
					eventType,
					lineStartsClientOutput,
					forceFlushFailedEvent,
					dataValid,
				)
				if immediateFirstEvent {
					// The first Codex control event is replay-safe and should not
					// wait behind the semantic-output staging buffer.
					lineNeedsFlush = true
					controlEventInProgress = true
				}
			} else {
				lineNeedsFlush = forceFlushFailedEvent
			}
			if lineStartsClientOutput && !openAIStreamEventTypeIsTerminal(eventType) {
				responsesSemanticOutputSeen = true
			}
			if account != nil && account.Platform == PlatformOpenAI &&
				(eventType == "response.completed" || eventType == "response.done") &&
				!sawFailedEvent && !responsesSemanticOutputSeen &&
				!openAIStreamClientOutputStarted(c, clientOutputStarted) &&
				openAIResponsesCompletedEventIsEmpty(dataBytes, usage) {
				return resultWithUsage(), newOpenAIResponsesEmptyCompletedFailoverError(c, account, upstreamRequestID)
			}
			if !visibleOutputObserved && openAIStreamDataStartsVisibleOutput(trimmedData, eventType) {
				visibleOutputObserved = true
			}
			startsTTFT := openAIStreamDataStartsTTFT(trimmedData, eventType, visibleOutputTTFT)
			if firstTokenMs == nil && startsTTFT {
				ms := int(time.Since(startTime).Milliseconds())
				if startsLocalFirstEventTTFT {
					if localFirstEventTTFTMs == nil {
						localFirstEventTTFTMs = &ms
					}
				} else {
					firstTokenMs = &ms
				}
			}
			if lineStartsClientOutput {
				stopFirstOutputWatchdog()
			}
			s.parseSSEUsageBytes(dataBytes, usage)
		}

		if !clientDisconnected {
			if !clientOutputStarted && !lineNeedsFlush && !controlEventInProgress {
				if preOutputStage != nil {
					incomingBytes := int64(len(line) + 1)
					if incomingBytes > preOutputStage.limit-preOutputStage.Buffered() {
						stopFirstOutputWatchdog()
						failoverErr := s.newOpenAIStreamFailoverError(
							c, account, true, upstreamRequestID, nil,
							"OpenAI passthrough first-output staging limit exceeded",
							resp.Header,
						)
						failoverErr.SafeToFailoverAfterWrite = true
						return resultWithUsage(), failoverErr
					}
					_, writeErr := preOutputStage.WriteString(line)
					if writeErr == nil {
						_, writeErr = preOutputStage.WriteString("\n")
					}
					if writeErr != nil {
						failoverErr := s.newOpenAIStreamFailoverError(
							c, account, true, upstreamRequestID, nil,
							"OpenAI passthrough first-output staging failed",
							resp.Header,
						)
						failoverErr.SafeToFailoverAfterWrite = true
						return resultWithUsage(), failoverErr
					}
					continue
				}
				pendingLines = append(pendingLines, line)
				pendingBytes += len(line) + 1
				if pendingBytes >= openAIPassthroughPreOutputBufferLimit {
					stopFirstOutputWatchdog()
					if writePendingLines() {
						clientOutputStarted = true
						flusher.Flush()
						lastDownstreamWriteAt = time.Now()
					}
				}
				continue
			}
			if !clientOutputStarted && controlEventInProgress && preOutputStage != nil && preOutputStage.Buffered() > 0 {
				stagedBytes := preOutputStage.Buffered()
				completedStage := preOutputStage
				if err := completedStage.CommitTo(w); err != nil {
					clientDisconnected = true
					continue
				}
				preOutputStage = newDefaultOpenAIFirstOutputStage()
				if err := completedStage.Close(); err != nil {
					logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough control-output staging cleanup failed: account=%d error=%v", account.ID, err)
				}
				recordOpenAIStreamControlOutput(c, int(stagedBytes))
			}
			if !clientOutputStarted && pendingOutputBytes() > 0 {
				if !writePendingLines() {
					continue
				}
			}
			if !controlEventInProgress {
				applyAttemptResponseHeaders()
			}
			if written, err := fmt.Fprintln(w, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
			} else {
				if controlEventInProgress {
					recordOpenAIStreamControlOutput(c, written)
				} else {
					recordOpenAIStreamSemanticOutput(c, written)
					if !clientOutputStarted {
						noteTurnStateCommitted()
					}
				}
				if !controlEventInProgress {
					clientOutputStarted = true
				}
				downstreamEventInProgress = line != ""
				flushPending = true
				if line == "" {
					flushPendingOutput()
					if controlEventInProgress {
						if !clientDisconnected {
							recordOpenAIRequestFirstEventDelivered(c)
						}
						controlEventInProgress = false
						downstreamEventInProgress = false
					}
				}
			}
		}
		if sawResponseFailedEvent && line == "" {
			// A failed response is final. Do not forward later synthetic progress
			// or wait for a relay to close an otherwise still-live connection.
			_ = resp.Body.Close()
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
		}
	}
	if scanErr != nil {
		err := scanErr
		if (sawDone || sawTerminalEvent) && !sawFailedEvent {
			s.clearOpenAIProxyStreamDisconnect(account, startTime)
			return resultWithUsage(), nil
		}
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", ctxErr)
		}
		if errors.Is(err, context.Canceled) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return resultWithUsage(), err
		}
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) && !sawOutputProgressEvent {
			if classifyOpenAITransportError(err).Persistent {
				return resultWithUsage(), s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
			}
			if isOpenAITransportTimeout(err) && isOpenAITransportRetryCandidate(ctx) && !IsOpenAIStreamResponseHeaderTimeout(err) {
				markOpenAITransportTimeoutPending(c)
			}
			msg := "OpenAI stream disconnected before completion"
			if errText := strings.TrimSpace(err.Error()); errText != "" {
				msg += ": " + errText
			}
			return resultWithUsage(),
				s.newRetryableOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, msg)
		}
		commitPendingOutputProgress()
		if errors.Is(err, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", err)
		}
		s.recordOpenAIProxyStreamDisconnect(account, err, upstreamRequestID, startTime)
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
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) && !sawOutputProgressEvent {
			return resultWithUsage(),
				s.newRetryableOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, "OpenAI stream ended before a terminal event")
		}
		commitPendingOutputProgress()
		s.recordOpenAIProxyStreamDisconnect(account, errors.New("stream ended before terminal event"), upstreamRequestID, startTime)
		return resultWithUsage(), errors.New("stream usage incomplete: missing terminal event")
	}
	if (sawDone || sawTerminalEvent) && !sawFailedEvent {
		s.clearOpenAIProxyStreamDisconnect(account, startTime)
		s.bindStagedOpenAICodexTurnState(c, account, resp.Header)
	}

	return resultWithUsage(), nil
}

func (s *OpenAIGatewayService) handleNonStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	mappedModel string,
) (*openaiNonStreamingResultPassthrough, error) {
	beginOpenAITimingObservation(c)
	if openAIStreamResponseMetadataTrailersActive(c) {
		declareOpenAIStreamResponseMetadataTrailers(c)
	}
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		failoverErr := s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
		if typed, ok := failoverErr.(*UpstreamFailoverError); ok && !classifyOpenAITransportError(err).Persistent {
			markOpenAIPassthroughTransportRetry(typed)
		}
		return nil, failoverErr
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observeOpenAITimingBody(c, body)
	if bodyHasSSEFraming(body) {
		observeOpenAISSEBody(observer, string(body))
	} else {
		observer.ObserveOpenAI(body, strings.TrimSpace(gjson.GetBytes(body, "type").String()))
	}

	// Detect SSE responses from upstream and convert to JSON.
	// Some upstreams (e.g. other sub2api instances) may return SSE even when
	// stream=false was requested. Without this conversion the client would
	// receive raw SSE text or a terminal event with empty output.
	if isEventStreamResponse(resp.Header) || bodyHasSSEFraming(body) {
		return s.handlePassthroughSSEToJSONWithAccount(resp, c, body, originalModel, mappedModel, account)
	}
	if failurePayload, failure := extractOpenAIJSONFailureEnvelope(body); failure {
		return nil, s.handleOpenAINonStreamingFailureEnvelope(resp, c, account, failurePayload, body, true)
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

	stagedHeaders := stageOpenAIPassthroughResponseHeaders(resp.Header, s.responseHeaderFilter)
	if c.Writer.Written() {
		setOpenAIStreamResponseMetadataTrailers(c, stagedHeaders)
		s.noteStagedOpenAICodexTurnStateCommitted(c, account, stagedHeaders)
	} else {
		applyOpenAIPassthroughResponseHeaders(c, stagedHeaders)
		s.relayOpenAICodexTurnState(c, account, resp.Header)
		if requestID := strings.TrimSpace(resp.Header.Get("x-request-id")); requestID != "" {
			c.Header("x-request-id", requestID)
		}
	}

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
	body = restoreOpenAIResponsesInstructionsEchoInResponse(c, body)
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
	}
	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       extractOpenAIImagesBillableCountFromJSONBytes(body),
		imageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(body),
	}, nil
}

func (s *OpenAIGatewayService) handlePassthroughSSEToJSONWithAccount(
	resp *http.Response,
	c *gin.Context,
	body []byte,
	originalModel string,
	mappedModel string,
	account *Account,
) (*openaiNonStreamingResultPassthrough, error) {
	if openAIStreamResponseMetadataTrailersActive(c) {
		declareOpenAIStreamResponseMetadataTrailers(c)
	}
	bodyText := string(body)
	captureOpenAICodexTurnStateFromSSE(resp.Header, bodyText)
	if failurePayload, failure := extractOpenAISSEFailureEvent(bodyText); failure {
		if policyErr, handled := writeOpenAIUpstreamPolicyError(c, resp.StatusCode, failurePayload); handled {
			return nil, policyErr
		}
		failedMessage := extractOpenAISSEErrorMessage(failurePayload)
		if failedMessage == "" {
			failedMessage = "Upstream response failed"
		}
		if !openAIResponseFailureHasSemanticOutput(bodyText, failurePayload) {
			if handledErr, handled := s.handleOpenAIResponseFailedBeforeOutput(
				c, account, true, strings.TrimSpace(resp.Header.Get("x-request-id")),
				failurePayload, failedMessage, resp.Header,
			); handled {
				return nil, handledErr
			}
		} else {
			s.recordOpenAIStreamUpstreamError(c, account, true, strings.TrimSpace(resp.Header.Get("x-request-id")), "http_error", failurePayload, failedMessage)
			logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough non-streaming response.failed after semantic output; failover skipped: response already produced output")
			if openAIStreamFailureIsExplicitlyRetryable(failurePayload, failedMessage) {
				failedMessage = openAIReplayUnsafeTransientFailureMessage
			}
		}
		return nil, s.writeOpenAINonStreamingProtocolError(resp, c, failedMessage)
	}
	finalResponse, ok := extractOpenAINonStreamingFinalResponse(bodyText)

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
		body = restoreOpenAIResponsesInstructionsEchoInResponse(c, restoredBody)
	} else {
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
		}
		bodyText = restoreOpenAIResponsesInstructionsEchoInSSEBody(c, bodyText)
		body = []byte(bodyText)
	}

	stagedHeaders := stageOpenAIPassthroughResponseHeaders(resp.Header, s.responseHeaderFilter)
	if c.Writer.Written() {
		setOpenAIStreamResponseMetadataTrailers(c, stagedHeaders)
		s.noteStagedOpenAICodexTurnStateCommitted(c, account, stagedHeaders)
	} else {
		applyOpenAIPassthroughResponseHeaders(c, stagedHeaders)
		s.relayOpenAICodexTurnState(c, account, resp.Header)
		if requestID := strings.TrimSpace(resp.Header.Get("x-request-id")); requestID != "" {
			c.Header("x-request-id", requestID)
		}
	}

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
		}
	}
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		if ok {
			c.Writer.Header().Set("Content-Type", contentType)
		}
		c.Data(resp.StatusCode, contentType, body)
	}

	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       extractOpenAIImagesBillableCountFromSSEBody(bodyText),
		imageOutputSizes: collectOpenAIImageOutputSizesFromSSEBody(bodyText),
	}, nil
}

func stageOpenAIPassthroughResponseHeaders(src http.Header, filter *responseheaders.CompiledHeaderFilter) http.Header {
	staged := make(http.Header)
	writeOpenAIPassthroughResponseHeaders(staged, src, filter)
	return staged
}

func applyOpenAIPassthroughResponseHeaders(c *gin.Context, staged http.Header) {
	if c == nil || c.Writer == nil || staged == nil {
		return
	}
	for key, values := range staged {
		c.Writer.Header().Del(key)
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
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

	// Codex clients echo this opaque response header on later requests in the
	// same turn. Clear stale values when the selected upstream omits it.
	turnStateKey := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	dst.Del(turnStateKey)
	for _, value := range getCaseInsensitiveValues(src, openAICodexTurnStateHeader) {
		dst.Add(turnStateKey, value)
	}
}
