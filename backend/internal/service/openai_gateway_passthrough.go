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

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const openAIResponsesClientToolMappingContextKey = "openai_responses_client_tool_mapping"

func hasOpenAIResponsesClientToolMapping(mapping apicompat.ResponsesClientToolMapping) bool {
	return len(mapping.CustomTools) > 0 || mapping.ToolSearch || len(mapping.NamespaceTools) > 0
REDACTED

func adaptOpenAIResponsesClientTools(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	if !needsOpenAIResponsesClientToolAdaptation(body) {
		return body, apicompat.ResponsesClientToolMapping{REDACTED, nil
REDACTED

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return body, apicompat.ResponsesClientToolMapping{REDACTED, fmt.Errorf("decode OpenAI Responses client tools: %w", err)
REDACTED
	var trailingValue any
	if err := decoder.Decode(&trailingValue); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
	REDACTED
		return body, apicompat.ResponsesClientToolMapping{REDACTED, fmt.Errorf("decode OpenAI Responses client tools trailing data: %w", err)
REDACTED
	mapping, changed, err := apicompat.AdaptResponsesClientTools(requestBody)
	if err != nil || !changed {
		return body, mapping, err
REDACTED
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{REDACTED, fmt.Errorf("encode OpenAI Responses client tools: %w", err)
REDACTED
	return rebuilt, mapping, nil
REDACTED

func needsOpenAIResponsesClientToolAdaptation(body []byte) bool {
	needsAdaptation := false
	var visit func(gjson.Result) bool
	visit = func(value gjson.Result) bool {
		if value.IsObject() {
			switch strings.TrimSpace(value.Get("type").String()) {
			case "custom", "custom_tool_call", "custom_tool_call_output",
				"tool_search", "tool_search_call", "tool_search_output":
				needsAdaptation = true
				return false
		REDACTED
	REDACTED
		if value.IsObject() || value.IsArray() {
			value.ForEach(func(_, child gjson.Result) bool {
				return visit(child)
		REDACTED)
	REDACTED
		return !needsAdaptation
REDACTED
	visit(gjson.ParseBytes(body))
	return needsAdaptation
REDACTED

func openAIResponsesClientToolMapping(c *gin.Context) (apicompat.ResponsesClientToolMapping, bool) {
	if c == nil {
		return apicompat.ResponsesClientToolMapping{REDACTED, false
REDACTED
	value, ok := c.Get(openAIResponsesClientToolMappingContextKey)
	mapping, typed := value.(apicompat.ResponsesClientToolMapping)
	return mapping, ok && typed && hasOpenAIResponsesClientToolMapping(mapping)
REDACTED

func setOpenAIResponsesClientToolMapping(c *gin.Context, mapping apicompat.ResponsesClientToolMapping) {
	if c == nil {
		return
REDACTED
	if !hasOpenAIResponsesClientToolMapping(mapping) {
		clearOpenAIResponsesClientToolMapping(c)
		return
REDACTED
	c.Set(openAIResponsesClientToolMappingContextKey, mapping)
REDACTED

// clearOpenAIResponsesClientToolMapping removes mapping state from the prior
// forwarding attempt. Forward retries accounts on the same Gin context.
func clearOpenAIResponsesClientToolMapping(c *gin.Context) {
	if c == nil {
		return
REDACTED
	if _, exists := c.Get(openAIResponsesClientToolMappingContextKey); exists {
		c.Set(openAIResponsesClientToolMappingContextKey, apicompat.ResponsesClientToolMapping{REDACTED)
REDACTED
REDACTED

func restoreOpenAIResponsesClientToolPayload(c *gin.Context, payload []byte) ([]byte, error) {
	mapping, ok := openAIResponsesClientToolMapping(c)
	if !ok || !bytes.Contains(payload, []byte(`"function_call"`)) || !json.Valid(payload) {
		return payload, nil
REDACTED
	restored, _, err := apicompat.RestoreResponsesClientToolPayload(payload, mapping)
	return restored, err
REDACTED

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
	requestedModel := reqModel
	upstreamPassthroughModel := ""
	if isOpenAIResponsesCompactPath(c) {
		compactMappedModel := s.resolveOpenAICompactFallbackModel(account, reqModel)
		if compactMappedModel != "" && compactMappedModel != reqModel {
			nextBody, setErr := sjson.SetBytes(body, "model", compactMappedModel)
			if setErr != nil {
				return nil, fmt.Errorf("set compact passthrough model: %w", setErr)
		REDACTED
			body = nextBody
			upstreamPassthroughModel = compactMappedModel
			attemptImageIntentInvalidated = true
	REDACTED
REDACTED

	if account != nil && account.UsesOpenAICodexProtocol() {
		if rejectReason := detectOpenAIPassthroughInstructionsRejectReason(reqModel, body); rejectReason != "" {
			rejectMsg := "OpenAI codex passthrough requires a non-empty instructions field"
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			logOpenAIPassthroughInstructionsRejected(ctx, c, account, reqModel, rejectReason, body)
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "forbidden_error",
					"message": rejectMsg,
			REDACTED,
		REDACTED)
			return nil, fmt.Errorf("openai passthrough rejected before upstream: %s", rejectReason)
	REDACTED
		if isOpenAICodexModel(reqModel) && !gjson.GetBytes(body, "instructions").Exists() {
			nextBody, setErr := sjson.SetBytes(body, "instructions", defaultCodexSynthInstructions(reqModel))
			if setErr != nil {
				return nil, fmt.Errorf("set passthrough codex instructions: %w", setErr)
		REDACTED
			body = nextBody
	REDACTED

		normalizedBody, normalized, err := normalizeOpenAIPassthroughOAuthBody(body, isOpenAIResponsesCompactPath(c))
		if err != nil {
			return nil, err
	REDACTED
		if normalized {
			body = normalizedBody
	REDACTED
		reqStream = gjson.GetBytes(body, "stream").Bool()

		accountScopedBody, accountScoped, scopeErr := applyCodexAccountIdentityClientMetadataRaw(body, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
		if scopeErr != nil {
			return nil, scopeErr
	REDACTED
		if accountScoped {
			body = accountScopedBody
	REDACTED

		stageCodexFingerprintIDs(c, nil)
		// 指纹收敛：与非透传路径同门控（仅 OAuth、legacy compact 形态跳过）。
		// 一次性解析收敛 ID：请求体 client_metadata 在此改写（raw 字节外科
		// 手术，透传热路径禁全量 Unmarshal），出站头改写由请求构造器读取
		// context 中的同一份 IDs 完成（turn_id 等随机字段两侧必须一致）。
		if !isOpenAIResponsesCompactPath(c) {
			var clientHeaders http.Header
			if c != nil && c.Request != nil {
				clientHeaders = c.Request.Header
		REDACTED
			fpIDs := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
			if fpIDs != nil {
				fpBody, fpChanged, fpErr := applyCodexFingerprintClientMetadataRaw(body, fpIDs)
				if fpErr != nil {
					return nil, fpErr
			REDACTED
				if fpChanged {
					body = fpBody
			REDACTED
		REDACTED
			stageCodexFingerprintIDs(c, fpIDs)
	REDACTED
REDACTED
	if account != nil && account.IsOpenAI() {
		normalizedBody, normalized, normalizeErr := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, account)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize passthrough Responses compatibility: %w", normalizeErr)
	REDACTED
		if normalized {
			body = normalizedBody
	REDACTED
		if account.IsOpenAIOAuthLike() {
			aliasedBody, reverse, aliased, aliasErr := aliasOpenAIOAuthReservedToolNamesBody(body)
			if aliasErr != nil {
				return nil, aliasErr
		REDACTED
			mergeCodexToolNameReverse(c, reverse)
			if aliased {
				body = aliasedBody
		REDACTED
	REDACTED
REDACTED

	if account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey &&
		!isOpenAIResponsesCompactPath(c) && needsOpenAIResponsesClientToolAdaptation(body) {
		adaptedBody, mapping, adaptErr := adaptOpenAIResponsesClientTools(body)
		if adaptErr != nil {
			return nil, adaptErr
	REDACTED
		body = adaptedBody
		setOpenAIResponsesClientToolMapping(c, mapping)
REDACTED

	sanitizedBody, sanitized, err := sanitizeEmptyBase64InputImagesInOpenAIBody(body)
	if err != nil {
		return nil, err
REDACTED
	if sanitized {
		body = sanitizedBody
REDACTED

	// Apply OpenAI fast policy to the passthrough body (filter/block by service_tier).
	// 统一使用 upstream 视角的 model：透传路径下 body 已经过 compact 映射 +
	// OAuth normalize，body 中的 model 字段即上游真正会看到的 slug。
	// 这样可以与 chat-completions / messages / native /responses 入口的
	// upstreamModel 保持一致，避免 whitelist 命中差异。当 body 中没有
	// model 字段时退回 reqModel。
	policyModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if policyModel == "" {
		policyModel = reqModel
REDACTED
	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, policyModel, body)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
	REDACTED
		return nil, policyErr
REDACTED
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
		REDACTED,
	REDACTED)
		return nil, errors.New("image generation disabled for group")
REDACTED
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
			REDACTED,
		REDACTED)
			return nil, imageCfgErr
	REDACTED
		imageBillingModel = imageCfg.Model
		imageSizeTier = imageCfg.SizeTier
		imageInputSize = imageCfg.InputSize
REDACTED

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
		REDACTED else {
				streamWarnLogger.Warn("OpenAI passthrough 检测到超时相关请求头，将按配置过滤以降低断流风险")
		REDACTED
	REDACTED
REDACTED

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	if c != nil {
		c.Set("openai_passthrough", true)
REDACTED

	agentTaskRecoveryTried := false
	compactModelFallbackRetried := false
	rejectedFieldRetryState := openAIResponsesRejectedFieldRetryStateForRequest(c, body)
	var resp *http.Response
	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	imageCount := 0
	var imageOutputSizes []string
	for {
		actualModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
		if actualModel == "" {
			actualModel = reqModel
	REDACTED
		SetOpsUpstreamModel(c, actualModel)
		upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
		upstreamReq, buildErr := s.buildUpstreamRequestOpenAIPassthrough(upstreamCtx, c, account, body, token)
		releaseUpstreamCtx()
		if buildErr != nil {
			return nil, buildErr
	REDACTED

		upstreamStart := time.Now()
		resp, err = s.doOpenAIUpstream(upstreamReq, proxyURL, account)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if err != nil {
			// Transport-level failure (proxy/DNS/TCP/TLS — no HTTP response). Convert to
			// a failover so the handler switches to a healthy account.
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	REDACTED
		if resp.StatusCode >= 400 {
			// Peek only to identify an invalid task. Restore the body so the existing
			// passthrough error handling sees the same response after recovery fails.
			probeBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(probeBody))
			if retryBody, reason, changed, retryErr := normalizeOpenAIResponsesRejectedFieldRetryBody(resp.StatusCode, body, probeBody); retryErr != nil {
				return nil, fmt.Errorf("normalize passthrough rejected Responses field retry body: %w", retryErr)
		REDACTED else if changed && rejectedFieldRetryState.Allow(retryBody) {
				body = retryBody
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Retrying passthrough request after %s (account: %s)", reason, account.Name)
				continue
		REDACTED
			if !agentTaskRecoveryTried && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, probeBody) {
				agentTaskRecoveryTried = true
				expectedTaskID := account.GetCredential("task_id")
				if recoveryErr := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); recoveryErr != nil {
					return nil, fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
			REDACTED
				continue
		REDACTED
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(probeBody)))
			if retryBody, fallbackModel, retry := s.prepareOpenAICompactFallbackRetry(
				c, account, requestedModel, body, resp.StatusCode, upstreamMsg, probeBody, compactModelFallbackRetried,
			); retry {
				s.appendOpenAICompactFallbackRetryOps(c, account, resp, probeBody, upstreamMsg, true)
				fromModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
				body = retryBody
				upstreamPassthroughModel = fallbackModel
				compactModelFallbackRetried = true
				SetOpsUpstreamModel(c, fallbackModel)
				logger.LegacyPrintf(
					"service.openai_gateway",
					"[OpenAI passthrough] Retrying explicit compact request once with fallback model (account: %s, from: %s, to: %s, upstream_code: %s)",
					account.Name, fromModel, fallbackModel, extractUpstreamErrorCode(probeBody),
				)
				continue
		REDACTED

			// 透传模式默认保持原样代理；容量错误以及 API-key 上游的瞬时
			// 5xx 应先触发多账号 failover，且此时尚未写入下游响应。
			// probeBody 已在上方任务探测时读取过一次，直接复用避免重复读取。
			if shouldFailoverOpenAIPassthroughResponse(account, resp.StatusCode, probeBody) {
				return nil, s.handleFailoverErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
		REDACTED
			return nil, s.handleErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
	REDACTED

		if mapping, ok := openAIResponsesClientToolMapping(c); ok && isEventStreamResponse(resp.Header) {
			maxLineSize := defaultMaxLineSize
			if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
				maxLineSize = s.cfg.Gateway.MaxLineSize
		REDACTED
			resp.Body = newGrokResponsesClientToolStreamBody(resp.Body, mapping, maxLineSize)
	REDACTED

		// x-codex-turn-state 溯源：下游回传由 writeOpenAIPassthroughResponseHeaders
		// 在各 handler 的写头点强制放行，铸造账号在此统一记录，供出站守卫剥离
		// failover 换号后的跨账号回带（openai_codex_turn_state.go）。
		if extractOpenAICodexTurnState(resp.Header) != "" {
			s.noteOpenAICodexTurnStateProvenance(c, account)
	REDACTED

		if reqStream {
			result, handleErr := s.handleStreamingResponsePassthrough(ctx, resp, c, account, startTime, reqModel, upstreamPassthroughModel)
			if handleErr != nil {
				if retryBody, fallbackModel, retry := s.applyOpenAIPassthroughCompactFallbackFromSignal(
					c, account, requestedModel, body, handleErr, compactModelFallbackRetried, resp,
				); retry {
					body = retryBody
					upstreamPassthroughModel = fallbackModel
					compactModelFallbackRetried = true
					continue
			REDACTED
				if signal, ok := asOpenAICompactFallbackSignal(handleErr); ok {
					_ = resp.Body.Close()
					compactResp, compactBody := openAICompactFallbackErrorResponse(resp, signal)
					if shouldFailoverOpenAIPassthroughResponse(account, compactResp.StatusCode, compactBody) {
						return nil, s.handleFailoverErrorResponsePassthrough(ctx, compactResp, c, account, body, compactBody)
				REDACTED
					return nil, s.handleErrorResponsePassthrough(ctx, compactResp, c, account, body, compactBody)
			REDACTED
				_ = resp.Body.Close()
				return nil, handleErr
		REDACTED
			usage = result.usage
			firstTokenMs = result.firstTokenMs
			responseID = strings.TrimSpace(result.responseID)
			imageCount = result.imageCount
			imageOutputSizes = result.imageOutputSizes
	REDACTED else {
			result, handleErr := s.handleNonStreamingResponsePassthrough(ctx, resp, c, account, reqModel, upstreamPassthroughModel)
			if handleErr != nil {
				if retryBody, fallbackModel, retry := s.applyOpenAIPassthroughCompactFallbackFromSignal(
					c, account, requestedModel, body, handleErr, compactModelFallbackRetried, resp,
				); retry {
					body = retryBody
					upstreamPassthroughModel = fallbackModel
					compactModelFallbackRetried = true
					continue
			REDACTED
				if signal, ok := asOpenAICompactFallbackSignal(handleErr); ok {
					_ = resp.Body.Close()
					compactResp, compactBody := openAICompactFallbackErrorResponse(resp, signal)
					if shouldFailoverOpenAIPassthroughResponse(account, compactResp.StatusCode, compactBody) {
						return nil, s.handleFailoverErrorResponsePassthrough(ctx, compactResp, c, account, body, compactBody)
				REDACTED
					return nil, s.handleErrorResponsePassthrough(ctx, compactResp, c, account, body, compactBody)
			REDACTED
				_ = resp.Body.Close()
				return nil, handleErr
		REDACTED
			usage = result.usage
			responseID = strings.TrimSpace(result.responseID)
			imageCount = result.imageCount
			imageOutputSizes = result.imageOutputSizes
	REDACTED
		break
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	serviceTier := extractOpenAIServiceTierFromBody(body)
	s.bindHTTPResponseAccount(ctx, c, account, responseID)

	// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
	if !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
	REDACTED
REDACTED else if account.ParentAccountID != nil {
		notifyOpenAIAutoReset(*account.ParentAccountID)
REDACTED

	if usage == nil {
		usage = &OpenAIUsage{REDACTED
REDACTED

	forwardResult := &OpenAIForwardResult{
		RequestID:                     resp.Header.Get("x-request-id"),
		ResponseID:                    responseID,
		Usage:                         *usage,
		Model:                         reqModel,
		UpstreamModel:                 upstreamPassthroughModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier:   observedUpstreamResponseServiceTier(c),
		ServiceTier:                   resolvedOpenAIUpstreamServiceTier(c, serviceTier),
		ReasoningEffort:               reasoningEffort,
		Stream:                        reqStream,
		OpenAIWSMode:                  false,
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
REDACTED
	if imageCount > 0 {
		forwardResult.ImageCount = imageCount
		forwardResult.ImageSize = imageSizeTier
		forwardResult.ImageInputSize = imageInputSize
		forwardResult.ImageOutputSizes = imageOutputSizes
		forwardResult.BillingModel = imageBillingModel
REDACTED
	return forwardResult, nil
REDACTED

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
REDACTED
	accountID := int64(0)
	accountName := ""
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
		accountType = strings.TrimSpace(string(account.Type))
REDACTED
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.String("account_type", accountType),
		zap.String("request_model", strings.TrimSpace(reqModel)),
		zap.String("reject_reason", strings.TrimSpace(rejectReason)),
REDACTED
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
	logger.FromContext(ctx).With(fields...).Warn("OpenAI passthrough 本地拦截：Codex 请求缺少有效 instructions")
REDACTED

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
	case AccountTypeSetupToken:
		if account.IsOpenAIOAuthLike() {
			targetURL = chatgptCodexURL
	REDACTED
	case AccountTypeAPIKey:
		baseURL := account.GetOpenAIBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
		REDACTED
			targetURL = buildOpenAIResponsesURLForPlatform(account.Platform, validatedURL)
	REDACTED
REDACTED
	targetURL = appendOpenAIResponsesRequestPathSuffix(targetURL, openAIResponsesRequestPathSuffix(c))

	// DeepSeek 原生 Responses 端点为无状态实现（见 normalizeDeepSeekResponsesRequestBody）。
	body = normalizeDeepSeekResponsesRequestBody(account, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	// 透传客户端请求头（安全白名单）。
	allowTimeoutHeaders := s.isOpenAIPassthroughTimeoutHeadersAllowed()
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !isOpenAIPassthroughAllowedRequestHeader(lower, allowTimeoutHeaders) {
				continue
		REDACTED
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// 客户端回带的 x-codex-turn-state 若已知由其他账号铸造（failover 换号），
	// 剥离后再出站（openai_codex_turn_state.go）。
	s.guardOpenAICodexTurnStateEcho(c, account, req.Header)

	// 覆盖入站鉴权残留，并注入上游认证
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
REDACTED
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
	REDACTED
REDACTED

	// OAuth 透传到 ChatGPT internal API 时补齐必要头。
	if account.UsesOpenAICodexProtocol() {
		// Current Codex OAuth HTTP no longer negotiates the legacy Responses
		// experiment. Passthrough may receive it from an older client, so remove
		// only that token while preserving any independent beta negotiation.
		stripOpenAILegacyResponsesBeta(req.Header)
		promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
		req.Host = "chatgpt.com"
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
			return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
	REDACTED
		apiKeyID := getAPIKeyIDFromContext(c)
		// 先保存客户端原始值，再做 compact 补充，避免后续统一隔离时读到已处理的值。
		clientSessionID := strings.TrimSpace(req.Header.Get("session_id"))
		clientConversationID := strings.TrimSpace(req.Header.Get("conversation_id"))
		if isOpenAIResponsesCompactPath(c) {
			req.Header.Set("accept", "application/json")
			if req.Header.Get("version") == "" {
				req.Header.Set("version", CodexCanonicalClientVersion())
		REDACTED
			if clientSessionID == "" {
				clientSessionID = resolveOpenAICompactSessionID(c)
		REDACTED
	REDACTED else if req.Header.Get("accept") == "" {
			req.Header.Set("accept", "text/event-stream")
	REDACTED
		if req.Header.Get("originator") == "" {
			req.Header.Set("originator", resolveCodexOutboundIdentity("").originator)
	REDACTED
		// 用隔离后的 session 标识符覆盖客户端透传值，防止跨用户会话碰撞。
		if clientSessionID == "" {
			clientSessionID = promptCacheKey
	REDACTED
		if clientConversationID == "" {
			clientConversationID = promptCacheKey
	REDACTED
		if clientSessionID != "" {
			req.Header.Set("session_id", isolateOpenAIUpstreamSessionID(apiKeyID, codexAccountIdentitySource(c, account), clientSessionID))
	REDACTED
		if clientConversationID != "" {
			req.Header.Set("conversation_id", isolateOpenAIUpstreamSessionID(apiKeyID, codexAccountIdentitySource(c, account), clientConversationID))
	REDACTED
REDACTED else if isOpenAIResponsesCompactPath(c) {
		// 透传白名单会放行客户端的 Accept: text/event-stream；compact 上游是
		// unary JSON 协议，API-key 账号同样强制 Accept，避免上游按 SSE 返回
		// （#3777 期望行为 4）。
		req.Header.Set("accept", "application/json")
REDACTED

	// 透传模式也支持账户自定义 User-Agent 与 ForceCodexCLI 兜底。
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
REDACTED
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", CodexCanonicalUserAgent())
REDACTED
	applyCodexAccountIdentityHeaders(req.Header, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))

	// 指纹收敛：使用 forwardOpenAIPassthrough 中预计算的收敛 ID 改写出站头，
	// 与请求体 client_metadata 共享同一份 IDs（与非透传路径相同的相对位置：
	// 会话隔离之后、终态身份收口之前）。
	applyStagedCodexFingerprintHeaders(c, account, req.Header)
	// 终态收口：透传路径的 OAuth 与非透传完全一致，同样强制统一出站身份
	// （User-Agent / originator / version 同源自洽），客户端自报身份不会到达上游。
	if account.UsesOpenAICodexProtocol() {
		enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
REDACTED

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)
	// x-codex-beta-features：按真实 Codex 的会话级行为补注（在账号级覆写之后，
	// 保证不被覆盖丢失）。
	applyOpenAICodexBetaFeatures(c, account, req.Header)
	setOpenAICodexRoutingHintFromBody(req.Header, account, body)
	logOpenAIRoutingDiagnosticsFromBody(ctx, account, "http_passthrough", req.Header, body, "not_applicable")

	return req, nil
REDACTED

func stripOpenAILegacyResponsesBeta(headers http.Header) {
	if headers == nil {
		return
REDACTED

	preserved := make([]string, 0)
	for key, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), "OpenAI-Beta") {
			continue
	REDACTED
		delete(headers, key)
		for _, value := range values {
			parts := strings.Split(value, ",")
			kept := parts[:0]
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" || strings.EqualFold(part, "responses=experimental") {
					continue
			REDACTED
				kept = append(kept, part)
		REDACTED
			if len(kept) > 0 {
				preserved = append(preserved, strings.Join(kept, ", "))
		REDACTED
	REDACTED
REDACTED
	for _, value := range preserved {
		headers.Add("OpenAI-Beta", value)
REDACTED
REDACTED

func shouldFailoverOpenAIPassthroughResponse(account *Account, statusCode int, responseBody []byte) bool {
	if hit, _, _ := detectOpenAICyberPolicy(responseBody); hit {
		return false
REDACTED
	if isOpenAIContextWindowError("", responseBody) {
		return false
REDACTED
	if isOpenAIHTTPUpstreamAccessStateError(statusCode, "", responseBody) {
		return true
REDACTED
	if isOpenAIRequestBodyTooLargeError(statusCode, "", responseBody) {
		return true
REDACTED
	if account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode) {
		return true
REDACTED
	switch statusCode {
	case http.StatusTooManyRequests, 529:
		return true
REDACTED
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
REDACTED
	switch statusCode {
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		520, 521, 522, 523, 524:
		return true
	default:
		return false
REDACTED
REDACTED

func writeOpenAIPassthroughErrorHeaders(dst, src http.Header) {
	if dst == nil {
		return
REDACTED
	dst.Set("Content-Type", "application/json; charset=utf-8")
	dst.Set("Cache-Control", "no-store")
	dst.Del("Retry-After")
	if src == nil {
		return
REDACTED
	rawRetryAfter := strings.TrimSpace(src.Get("Retry-After"))
	if validOpenAIPassthroughRetryAfter(rawRetryAfter, time.Now()) {
		dst.Set("Retry-After", rawRetryAfter)
REDACTED
REDACTED

func validOpenAIPassthroughRetryAfter(raw string, now time.Time) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
REDACTED
	delaySeconds := true
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			delaySeconds = false
			break
	REDACTED
REDACTED
	if delaySeconds {
		seconds, err := strconv.ParseUint(raw, 10, 64)
		return err == nil && seconds > 0
REDACTED
	parsed, err := http.ParseTime(raw)
	return err == nil && parsed.After(now)
REDACTED

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
	REDACTED
REDACTED
	writeOpenAIPassthroughErrorEnvelope(c, downstreamStatus, upstreamHeaders, message)
REDACTED

// writeOpenAIPassthroughErrorEnvelope 以本地 JSON 信封 + 净化后的头策略写出
// 错误响应；message 由调用方决定（净化通用文案或脱敏后的上游消息）。
func writeOpenAIPassthroughErrorEnvelope(c *gin.Context, downstreamStatus int, upstreamHeaders http.Header, message string) {
	if c == nil {
		return
REDACTED
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
	REDACTED,
REDACTED)
	if writeOpenAICompactSSEBridge(c, downstreamStatus, body) {
		return
REDACTED
	writeOpenAIPassthroughErrorHeaders(c.Writer.Header(), upstreamHeaders)
	c.Data(downstreamStatus, "application/json; charset=utf-8", body)
REDACTED

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
	REDACTED
		upstreamDetail = truncateString(string(body), maxBytes)
REDACTED
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
REDACTED)
	return s.newOpenAIAccountFailoverError(
		account,
		resp.StatusCode,
		resp.Header,
		body,
		upstreamMsg,
		shouldDisable,
		!shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
	)
REDACTED

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
	REDACTED)
REDACTED

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
	REDACTED
		upstreamDetail = truncateString(string(body), maxBytes)
REDACTED
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	// 错误体虽不会原样透传，运行态账号状态仍需更新，避免粘性路由继续复用
	// 刚被限流的账号。cyber 例外：不冷却账号。
	if !cyberHit {
		reqModel, _, _ := extractOpenAIRequestMetaFromBody(requestBody)
		canonicalModel := canonicalOpenAIAccountSchedulingModel(account, reqModel)
		_ = s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, canonicalModel)
REDACTED
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
REDACTED)
	// context-window 超限是确定性请求失败（shouldFailoverOpenAIPassthroughResponse
	// 已保证不切号），其文案对客户端可操作（如触发自动压缩）；在净化信封内保留
	// 脱敏后的上游消息，而不是抹成通用文案。
	if isOpenAIContextWindowError(upstreamMsg, body) && upstreamMsg != "" {
		writeOpenAIPassthroughErrorEnvelope(c, resp.StatusCode, resp.Header, upstreamMsg)
REDACTED else {
		writeSanitizedOpenAIPassthroughError(c, resp.StatusCode, resp.Header)
REDACTED

	return fmt.Errorf("upstream error: %d (client response sanitized)", resp.StatusCode)
REDACTED

func isOpenAIPassthroughAllowedRequestHeader(lowerKey string, allowTimeoutHeaders bool) bool {
	if lowerKey == "" {
		return false
REDACTED
	if isOpenAIPassthroughTimeoutHeader(lowerKey) {
		return allowTimeoutHeaders
REDACTED
	return openaiPassthroughAllowedHeaders[lowerKey]
REDACTED

func isOpenAIPassthroughTimeoutHeader(lowerKey string) bool {
	switch lowerKey {
	case "x-stainless-timeout", "x-stainless-read-timeout", "x-stainless-connect-timeout", "x-request-timeout", "request-timeout", "grpc-timeout":
		return true
	default:
		return false
REDACTED
REDACTED

func (s *OpenAIGatewayService) isOpenAIPassthroughTimeoutHeadersAllowed() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIPassthroughAllowTimeoutHeaders
REDACTED

func collectOpenAIPassthroughTimeoutHeaders(h http.Header) []string {
	if h == nil {
		return nil
REDACTED
	var matched []string
	for key, values := range h {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if isOpenAIPassthroughTimeoutHeader(lowerKey) {
			entry := lowerKey
			if len(values) > 0 {
				entry = fmt.Sprintf("%s=%s", lowerKey, strings.Join(values, "|"))
		REDACTED
			matched = append(matched, entry)
	REDACTED
REDACTED
	sort.Strings(matched)
	return matched
REDACTED

type openaiStreamingResultPassthrough struct {
	usage            *OpenAIUsage
	firstTokenMs     *int
	responseID       string
	imageCount       int
	imageOutputSizes []string
REDACTED

type openaiNonStreamingResultPassthrough struct {
	*OpenAIUsage
	usage            *OpenAIUsage
	responseID       string
	imageCount       int
	imageOutputSizes []string
REDACTED

const openAIStreamKeepaliveBytesKey = "openai_stream_keepalive_bytes"

func recordOpenAIStreamKeepaliveBytes(c *gin.Context, written int) {
	if c == nil || written <= 0 {
		return
REDACTED
	current := 0
	if value, ok := c.Get(openAIStreamKeepaliveBytesKey); ok {
		current, _ = value.(int)
REDACTED
	c.Set(openAIStreamKeepaliveBytesKey, current+written)
REDACTED

func openAIStreamClientOutputStarted(c *gin.Context, localStarted bool) bool {
	if localStarted {
		return true
REDACTED
	if c == nil || c.Writer == nil {
		return false
REDACTED
	// compact keepalive comments commit the HTTP response as 200, but they are
	// not semantic model output and therefore must not block a safe retry.
	// Without a compact keepalive this is equivalent to checking Writer.Size().
	return OpenAICompactKeepaliveAdjustedWrittenSize(c) >= 0
REDACTED

func openAIStreamEventIsPreamble(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.created", "response.in_progress":
		return true
	default:
		return false
REDACTED
REDACTED

func openAIStreamAddedEventStartsClientOutput(payload []byte, eventType string) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return true
REDACTED

	switch strings.TrimSpace(eventType) {
	case "response.output_item.added":
		item := gjson.GetBytes(payload, "item")
		if !item.Exists() || !item.IsObject() {
			return true
	REDACTED
		switch strings.TrimSpace(item.Get("type").String()) {
		case "reasoning":
			if item.Get("encrypted_content").String() != "" {
				return true
		REDACTED
			summary := item.Get("summary")
			if !summary.IsArray() {
				return false
		REDACTED
			for _, part := range summary.Array() {
				if strings.TrimSpace(part.Get("type").String()) != "summary_text" || part.Get("text").String() != "" {
					return true
			REDACTED
		REDACTED
			return false
		case "message":
			content := item.Get("content")
			if !content.IsArray() {
				return false
		REDACTED
			for _, part := range content.Array() {
				switch strings.TrimSpace(part.Get("type").String()) {
				case "output_text":
					if part.Get("text").String() != "" {
						return true
				REDACTED
				case "refusal":
					if part.Get("refusal").String() != "" {
						return true
				REDACTED
				default:
					return true
			REDACTED
		REDACTED
			return false
		case "function_call":
			return item.Get("arguments").String() != ""
		case "custom_tool_call":
			return item.Get("input").String() != ""
		case "compaction":
			return item.Get("encrypted_content").String() != ""
		default:
			return true
	REDACTED
	case "response.content_part.added":
		part := gjson.GetBytes(payload, "part")
		if !part.Exists() || !part.IsObject() {
			return true
	REDACTED
		switch strings.TrimSpace(part.Get("type").String()) {
		case "output_text":
			return part.Get("text").String() != ""
		case "refusal":
			return part.Get("refusal").String() != ""
		default:
			return true
	REDACTED
	case "response.reasoning_summary_part.added":
		part := gjson.GetBytes(payload, "part")
		if !part.Exists() || !part.IsObject() || strings.TrimSpace(part.Get("type").String()) != "summary_text" {
			return true
	REDACTED
		return part.Get("text").String() != ""
	default:
		return true
REDACTED
REDACTED

func openAIStreamDataStartsClientOutput(data, eventType string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
REDACTED
	switch strings.TrimSpace(eventType) {
	case "response.failed":
		return false
	case "error":
		// 上游降载/瞬时故障会先推 {"type":"error"REDACTED 帧、再以 response.failed 收尾。
		// 可重试类错误帧不能算客户端输出：一旦把它当首输出 flush，
		// clientOutputStarted 即被固化，随后的 failed 事件永远进不了 pre-output
		// failover 分支，只能把致命错误原样转发给客户端。不可重试类
		// （content_policy / invalid_request 等）维持原样转发，保留上游错误细节。
		payload := []byte(trimmed)
		return !openAIStreamFailedEventShouldFailover(payload, extractOpenAISSEErrorMessage(payload))
	case "response.output_item.added", "response.content_part.added", "response.reasoning_summary_part.added":
		return openAIStreamAddedEventStartsClientOutput([]byte(trimmed), eventType)
REDACTED
	return !openAIStreamEventIsPreamble(eventType)
REDACTED

func openAIStreamItemHasVisibleOutput(item gjson.Result) bool {
	if item.Get("arguments").String() != "" || item.Get("input").String() != "" || item.Get("result").String() != "" {
		return true
REDACTED
	for _, path := range []string{"content", "summary"REDACTED {
		for _, part := range item.Get(path).Array() {
			if part.Get("text").String() != "" || part.Get("transcript").String() != "" {
				return true
		REDACTED
	REDACTED
REDACTED
	return false
REDACTED

// Structural progress can commit an attempt and disarm first-output failover,
// but TTFT should start only when the stream carries content a client can use.
func openAIStreamDataStartsVisibleOutput(data, eventType string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" || !gjson.Valid(trimmed) {
		return false
REDACTED
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = strings.TrimSpace(gjson.Get(trimmed, "type").String())
REDACTED
	if strings.HasSuffix(eventType, ".delta") {
		delta := gjson.Get(trimmed, "delta")
		return delta.Exists() && delta.String() != ""
REDACTED
	switch eventType {
	case "response.output_text.done",
		"response.reasoning_summary_text.done",
		"response.reasoning_text.done",
		"response.audio_transcript.done":
		return gjson.Get(trimmed, "text").String() != ""
	case "response.function_call_arguments.done":
		return gjson.Get(trimmed, "arguments").String() != ""
	case "response.custom_tool_call_input.done":
		return gjson.Get(trimmed, "input").String() != ""
	case "response.image_generation_call.partial_image":
		return gjson.Get(trimmed, "partial_image_b64").String() != ""
	case "response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		part := gjson.Get(trimmed, "part")
		return part.Get("text").String() != "" || part.Get("transcript").String() != ""
	case "response.output_item.added", "response.output_item.done":
		return openAIStreamItemHasVisibleOutput(gjson.Get(trimmed, "item"))
	case "response.completed", "response.done":
		for _, item := range gjson.Get(trimmed, "response.output").Array() {
			if openAIStreamItemHasVisibleOutput(item) {
				return true
		REDACTED
	REDACTED
REDACTED
	return false
REDACTED

// openAIStreamFailedEventErrorCode 提取流内 failed 事件的错误码（小写），
// 兼容 response.failed 的嵌套形态与裸 error 形态。
func openAIStreamFailedEventErrorCode(payload []byte) string {
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
REDACTED
	return code
REDACTED

// isOpenAIUpstreamCapacityShedEvent 判断流内 failed 事件是否为上游容量降载信号。
// 上游在容量紧张时会把请求丢进降载路径：HTTP 200 之后立刻推 event: error
// （code=server_is_overloaded / slow_down）并以 response.failed 收尾。
func isOpenAIUpstreamCapacityShedEvent(payload []byte) bool {
	switch openAIStreamFailedEventErrorCode(payload) {
	case "server_is_overloaded", "slow_down":
		return true
REDACTED
	for _, path := range []string{"response.error.message", "error.message", "message"REDACTED {
		if isOpenAICapacityShedMessage(gjson.GetBytes(payload, path).String()) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

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
REDACTED
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("platform", account.Platform),
		)
REDACTED
	logger.FromContext(ctx).Warn("gateway.failover_suppressed_after_semantic_output", fields...)
REDACTED

// openAICapacityShedRetryableClientCode 是把上游容量降载错误转发给客户端时改写
// 使用的错误码。Codex CLI 按闭集对错误码分类：server_is_overloaded / slow_down
// 被判为致命错误（客户端提示 "Selected model is at capacity. Please try a
// different model." 并直接终止会话），而 server_error 等致命集之外的错误码会进入
// 客户端内置的退避重试。
const openAICapacityShedRetryableClientCode = "server_error"

// sanitizeOpenAICapacityShedErrorCodeForClient 把即将写给下游客户端的
// error / response.failed 事件中的容量降载错误码改写为客户端可重试的错误码。
// 走到转发这一步说明网关侧 failover 已不可用（流中途）或已用尽；保留原始降载码
// 只会让客户端就地终止会话。错误消息原样保留；监控与账号状态判定都基于改写前
// 的原始 payload，不受影响。rate_limit 等其他错误码一律不动（客户端依赖
// rate_limit_exceeded 原码解析重试延时）。
func sanitizeOpenAICapacityShedErrorCodeForClient(payload []byte) ([]byte, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) || !isOpenAIUpstreamCapacityShedEvent(payload) {
		return payload, false
REDACTED
	updated := payload
	changed := false
	for _, path := range []string{"response.error.code", "error.code"REDACTED {
		parent := strings.TrimSuffix(path, ".code")
		if !gjson.GetBytes(updated, parent).Exists() {
			continue
	REDACTED
		code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(updated, path).String()))
		if code != "" && code != "server_is_overloaded" && code != "slow_down" {
			continue
	REDACTED
		next, err := sjson.SetBytes(updated, path, openAICapacityShedRetryableClientCode)
		if err != nil {
			return payload, false
	REDACTED
		updated = next
		changed = true
REDACTED
	return updated, changed
REDACTED

func openAIStreamFailedEventSemanticStatus(payload []byte, message string) int {
	if isOpenAIContextWindowError(message, payload) {
		return http.StatusBadRequest
REDACTED

	code := openAIStreamFailedEventErrorCode(payload)
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
REDACTED
	combined := strings.TrimSpace(errType + " " + code + " " + strings.ToLower(strings.TrimSpace(message)))
	for _, path := range []string{"response.error.status_code", "error.status_code", "status_code"REDACTED {
		if status := int(gjson.GetBytes(payload, path).Int()); status == http.StatusUnauthorized ||
			status == http.StatusForbidden || status == http.StatusTooManyRequests || status == 529 {
			return status
	REDACTED
REDACTED
	switch {
	case strings.Contains(combined, "rate_limit"):
		return http.StatusTooManyRequests
	case strings.Contains(errType, "invalid_request"):
		return http.StatusBadRequest
	case strings.Contains(combined, "authentication") || strings.Contains(combined, "unauthorized") || strings.Contains(combined, "invalid_api_key"):
		return http.StatusUnauthorized
	case strings.Contains(combined, "permission") || strings.Contains(combined, "forbidden") || strings.Contains(combined, "access denied"):
		return http.StatusForbidden
	case isOpenAIUpstreamAccessStateError(message, payload):
		return http.StatusForbidden
	case isOpenAIUpstreamCapacityShedEvent(payload):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
REDACTED
REDACTED

func openAIStreamFailureStatus(payload []byte, message string) int {
	if len(bytes.TrimSpace(payload)) == 0 || !gjson.ValidBytes(payload) {
		return http.StatusBadGateway
REDACTED
	semanticStatus := openAIStreamFailedEventSemanticStatus(payload, message)
	switch semanticStatus {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, 529:
		return semanticStatus
	case http.StatusServiceUnavailable:
		if isOpenAIUpstreamCapacityShedEvent(payload) {
			return semanticStatus
	REDACTED
REDACTED
	return http.StatusBadGateway
REDACTED

// openAIStreamCredentialAuthFailure distinguishes credential failures from
// request/content permission denials carried inside an HTTP 200 stream. Do not
// infer credential health from free-form 403 messages: providers also use
// permission_error/forbidden/access denied for request-scoped policy failures.
func openAIStreamCredentialAuthFailure(payload []byte) bool {
	if len(bytes.TrimSpace(payload)) == 0 || !gjson.ValidBytes(payload) {
		return false
REDACTED
	for _, path := range []string{"response.error.status_code", "error.status_code", "status_code"REDACTED {
		if int(gjson.GetBytes(payload, path).Int()) == http.StatusUnauthorized {
			return true
	REDACTED
REDACTED
	for _, path := range []string{"response.error.type", "error.type", "type"REDACTED {
		errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, path).String()))
		if errType == "authentication_error" || errType == "authentication_failed" || errType == "unauthorized_error" {
			return true
	REDACTED
REDACTED
	for _, path := range []string{"response.error.code", "error.code", "code"REDACTED {
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, path).String())) {
		case "invalid_api_key", "api_key_disabled", "unauthorized", "authentication_error",
			"invalid_token", "access_token_invalid", "token_revoked", "token_invalidated",
			"invalid_credentials", "credential_invalid":
			return true
	REDACTED
REDACTED
	return false
REDACTED

func openAIStream403AccountFailure(payload []byte, message string) bool {
	return isOpenAIUpstreamAccessStateError(message, payload) || openAIStreamCredentialAuthFailure(payload)
REDACTED

func openAIStreamFailedEventPassthroughBody(payload []byte, failedMessage string) []byte {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload
REDACTED
	if gjson.GetBytes(payload, "error").Exists() {
		return payload
REDACTED
	responseError := gjson.GetBytes(payload, "response.error")
	if !responseError.Exists() {
		if strings.TrimSpace(failedMessage) == "" {
			return payload
	REDACTED
		body, err := marshalOpenAIUpstreamJSON(gin.H{
			"error": gin.H{
				"message": failedMessage,
		REDACTED,
	REDACTED)
		if err != nil {
			return payload
	REDACTED
		return body
REDACTED

	errorPayload := gin.H{REDACTED
	if errType := strings.TrimSpace(gjson.Get(responseError.Raw, "type").String()); errType != "" {
		errorPayload["type"] = errType
REDACTED
	if code := strings.TrimSpace(gjson.Get(responseError.Raw, "code").String()); code != "" {
		errorPayload["code"] = code
REDACTED
	if param := strings.TrimSpace(gjson.Get(responseError.Raw, "param").String()); param != "" {
		errorPayload["param"] = param
REDACTED
	message := strings.TrimSpace(gjson.Get(responseError.Raw, "message").String())
	if message == "" {
		message = strings.TrimSpace(failedMessage)
REDACTED
	if message != "" {
		errorPayload["message"] = message
REDACTED
	if len(errorPayload) == 0 {
		return payload
REDACTED
	body, err := marshalOpenAIUpstreamJSON(gin.H{"error": errorPayloadREDACTED)
	if err != nil {
		return payload
REDACTED
	return body
REDACTED

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
REDACTED

func openAIStreamFailedEventShouldFailover(payload []byte, message string) bool {
	if hit, _, _ := detectOpenAICyberPolicy(payload); hit {
		return false
REDACTED
	if isOpenAIContextWindowError(message, payload) {
		return false
REDACTED
	if isOpenAIUpstreamAccessStateError(message, payload) {
		return true
REDACTED
	semanticStatus := openAIStreamFailureStatus(payload, message)
	if semanticStatus == http.StatusForbidden {
		return openAIStream403AccountFailure(payload, message)
REDACTED
	// A response.failed event is transported over HTTP 200. Prefer its semantic
	// rate-limit status over a generic/invalid_request error type so it can enter
	// the same 429 retry policy as a regular upstream HTTP response.
	if semanticStatus == http.StatusTooManyRequests {
		return true
REDACTED
	if isOpenAITransientProcessingError(http.StatusBadRequest, message, payload) {
		return true
REDACTED
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
REDACTED
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
REDACTED
	combined := strings.ToLower(strings.TrimSpace(message + " " + code + " " + errType))
	if combined == "" {
		return true
REDACTED
	nonRetryableMarkers := []string{
		"invalid_request",
		"content_policy",
		"policy",
		"safety",
		"high-risk cyber",
		"not allowed",
		"violat",
REDACTED
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(combined, marker) {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func openAIStreamErrorEventShouldFailover(payload []byte, message string) bool {
	if hit, _, _ := detectOpenAICyberPolicy(payload); hit {
		return false
REDACTED
	if isOpenAIContextWindowError(message, payload) {
		return false
REDACTED
	if isOpenAIUpstreamAccessStateError(message, payload) {
		return true
REDACTED
	switch openAIStreamFailedEventSemanticStatus(payload, message) {
	case http.StatusForbidden:
		return openAIStream403AccountFailure(payload, message)
	case http.StatusUnauthorized, http.StatusTooManyRequests, 529:
		return true
REDACTED
	if isOpenAITransientProcessingError(http.StatusBadRequest, message, payload) {
		return true
REDACTED
	combined := strings.ToLower(strings.TrimSpace(message + " " +
		gjson.GetBytes(payload, "error.message").String() + " " +
		gjson.GetBytes(payload, "response.error.message").String()))
	return strings.Contains(combined, "temporary") ||
		strings.Contains(combined, "try again") ||
		strings.Contains(combined, "please retry")
REDACTED

func (s *OpenAIGatewayService) handleOpenAIStreamTerminalAccountSideEffects(
	c *gin.Context,
	account *Account,
	payload []byte,
	message string,
	headers http.Header,
) (int, bool) {
	statusCode := openAIStreamFailureStatus(payload, message)
	switch statusCode {
	case http.StatusForbidden:
		if !openAIStream403AccountFailure(payload, message) {
			return statusCode, false
	REDACTED
		fallthrough
	case http.StatusUnauthorized, http.StatusTooManyRequests, 529:
		ctx := context.Background()
		if c != nil && c.Request != nil {
			ctx = c.Request.Context()
	REDACTED
		accountHeaders := headers
		if statusCode == http.StatusTooManyRequests {
			// The enclosing HTTP response succeeded. Its quota snapshot describes
			// normal account state and must not become the reset for a semantic 429
			// carried by a stream terminal event.
			accountHeaders = nil
	REDACTED
		return statusCode, s.handleOpenAIAccountUpstreamError(ctx, account, statusCode, accountHeaders, payload)
	default:
		return statusCode, false
REDACTED
REDACTED

func openAIStreamFailedEventRetryableOnSameAccount(account *Account, payload []byte, message string) bool {
	if account == nil {
		return false
REDACTED
	// 容量降载是请求级信号，不是账号级故障：上游只是让本次请求稍后再试。
	// 换账号并不改变被降载的因素（客户端身份、模型容量都与账号无关），
	// 只会让单个请求把整池账号逐个消耗掉，最终仍以同一个错误告终。
	// 因此先在同一账号上做有界重试，用尽后才按常规流程切号。
	if isOpenAIUpstreamCapacityShedEvent(payload) {
		return true
REDACTED
	if !account.IsPoolMode() {
		return false
REDACTED
	semanticStatus := openAIStreamFailedEventSemanticStatus(payload, message)
	return account.IsPoolModeRetryableStatus(semanticStatus) ||
		isOpenAITransientProcessingError(http.StatusBadRequest, message, payload)
REDACTED

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
REDACTED
	statusCode := openAIStreamFailureStatus(payload, message)
	detail := ""
	if len(payload) > 0 && s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
	REDACTED
		detail = truncateString(string(payload), maxBytes)
REDACTED
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
	REDACTED
		if account != nil {
			event.Platform = account.Platform
			event.AccountID = account.ID
			event.AccountName = account.Name
	REDACTED
		appendOpsUpstreamError(c, event)
REDACTED
	return message
REDACTED

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
REDACTED
	var headers http.Header
	if len(responseHeaders) > 0 && responseHeaders[0] != nil {
		headers = responseHeaders[0].Clone()
REDACTED
	statusCode, shouldDisable := s.handleOpenAIStreamTerminalAccountSideEffects(c, account, payload, message, headers)
	// 流内 failed 事件承载于 HTTP 200；使用事件的语义状态更新账号健康，
	// 再由 failover 引擎按 StatusCode/RetryableOnSameAccount 决定恢复策略。
	message = s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "failover", payload, message)
	errType := "upstream_error"
	if statusCode == http.StatusTooManyRequests {
		errType = "rate_limit_error"
REDACTED
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
	retryableOnSameAccount := openAIStreamFailedEventRetryableOnSameAccount(account, payload, message)
	failoverErr := s.newOpenAIAccountFailoverError(account, statusCode, headers, payload, message, shouldDisable, retryableOnSameAccount)
	if failoverErr.IsCredentialFailure() || failoverErr.RequestScopedTransient {
		return failoverErr
REDACTED
	// Preserve the existing generic envelope for unclassified stream failures;
	// only typed access/capacity failures need the original payload downstream.
	failoverErr.ResponseBody = body
	return failoverErr
REDACTED

func (s *OpenAIGatewayService) handleStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	originalModel string,
	mappedModel string,
) (*openaiStreamingResultPassthrough, error) {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
REDACTED
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &OpenAIUsage{REDACTED
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	responseID := ""
	clientDisconnected := false
	sawDone := false
	sawTerminalEvent := false
	sawFailedEvent := false
	sawBareError := false
	sawResponseFailed := false
	terminalEventType := ""
	semanticOutputSeen := false
	capacityFailoverSuppressedLogged := false
	failedMessage := ""
	clientOutputStarted := false
	codexFailureTerminal := account != nil && account.Platform == PlatformOpenAI
	failureDelivered := false
	suppressCurrentEvent := false
	responseFailedPending := false
	var bareErrorPayload []byte
	bareErrorAccountSideEffectsPending := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	// pendingLines 在首个可见输出前保留前导事件，确保无输出失败仍可安全 failover。
	pendingLines := make([]string, 0, 8)
	// flushPending 表示已写入但未到 SSE 空行边界的脏状态；defer 兜底函数退出前的残留，断连后不再 Flush。
	flushPending := false
	pendingSSEEventType := ""
	flushPendingOutput := func() {
		if clientDisconnected || !flushPending {
			return
	REDACTED
		flusher.Flush()
		flushPending = false
REDACTED
	defer flushPendingOutput()
	writePendingLines := func() bool {
		for _, pending := range pendingLines {
			if _, err := fmt.Fprintln(w, pending); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				return false
		REDACTED
	REDACTED
		pendingLines = pendingLines[:0]
		return true
REDACTED
	ensureResponseFailedTerminal := func() {
		if !sawBareError || sawResponseFailed || failureDelivered {
			return
	REDACTED
		if bareErrorAccountSideEffectsPending {
			s.handleOpenAIStreamTerminalAccountSideEffects(c, account, bareErrorPayload, failedMessage, resp.Header)
			bareErrorAccountSideEffectsPending = false
	REDACTED
		if clientDisconnected || !writePendingLines() {
			return
	REDACTED
		if _, err := fmt.Fprint(w, buildOpenAIResponseFailedSSE(responseID, originalModel, bareErrorPayload, failedMessage)); err != nil {
			clientDisconnected = true
			return
	REDACTED
		clientOutputStarted = true
		failureDelivered = true
		flushPending = true
		flushPendingOutput()
REDACTED

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
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
	REDACTED
REDACTED

	for documentScanner.Scan() {
		line := documentScanner.Text()
		if eventType, ok := extractOpenAISSEEventLine(line); ok {
			pendingSSEEventType = eventType
			eventType = strings.TrimSpace(eventType)
			suppressCurrentEvent = codexFailureTerminal && (eventType == "error" || (sawBareError && !sawResponseFailed && eventType != "response.failed"))
	REDACTED
		lineStartsClientOutput := false
		forceFlushFailedEvent := false
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes := []byte(data)
			trimmedData := strings.TrimSpace(data)
			rawEventType := effectiveOpenAISSEEventType(dataBytes, pendingSSEEventType)
			observer.ObserveOpenAI(dataBytes, rawEventType)
			if needModelReplace && strings.Contains(data, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
				if replacedData, replaced := extractOpenAISSEDataLine(line); replaced {
					dataBytes = []byte(replacedData)
					trimmedData = strings.TrimSpace(replacedData)
			REDACTED
		REDACTED
			if normalizedData, normalized := normalizeOpenAIResponsesFunctionCallArguments(dataBytes); normalized {
				dataBytes = normalizedData
				trimmedData = strings.TrimSpace(string(normalizedData))
				line = "data: " + string(normalizedData)
		REDACTED
			if normalizedData, normalized := normalizeCompletedImageGenerationStatus(dataBytes); normalized {
				dataBytes = normalizedData
				trimmedData = strings.TrimSpace(string(normalizedData))
				line = "data: " + string(normalizedData)
		REDACTED
			if trimmedData != "[DONE]" {
				restoredData, restoreErr := restoreOpenAIResponsesNamespacePayload(c, dataBytes)
				if restoreErr != nil {
					return resultWithUsage(), fmt.Errorf("restore OpenAI passthrough namespace response: %w", restoreErr)
			REDACTED
				restoredData = restoreCodexToolNamesFromSSEContext(c, restoredData, rawEventType)
				if !bytes.Equal(restoredData, dataBytes) {
					dataBytes = restoredData
					trimmedData = strings.TrimSpace(string(restoredData))
					line = "data: " + string(restoredData)
			REDACTED
		REDACTED
			eventType := effectiveOpenAISSEEventType(dataBytes, rawEventType)
			if codexFailureTerminal && sawBareError && !sawResponseFailed && eventType != "response.failed" {
				suppressCurrentEvent = true
		REDACTED
			if !capacityFailoverSuppressedLogged && account != nil && account.Platform == PlatformOpenAI &&
				(eventType == "error" || eventType == "response.failed") &&
				openAIStreamClientOutputStarted(c, clientOutputStarted) &&
				isOpenAIUpstreamCapacityShedEvent(dataBytes) {
				logOpenAICapacityFailoverSuppressed(ctx, account, "passthrough_sse", upstreamRequestID, eventType)
				capacityFailoverSuppressedLogged = true
		REDACTED
			cyberHit := false
			if eventType == "response.failed" || eventType == "error" {
				if codexFailureTerminal && eventType == "error" {
					sawBareError = true
					bareErrorPayload = append(bareErrorPayload[:0], dataBytes...)
					suppressCurrentEvent = true
			REDACTED else if codexFailureTerminal && eventType == "response.failed" {
					sawResponseFailed = true
			REDACTED
				responseFailedPending = !codexFailureTerminal || eventType == "response.failed"
				failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				if failedMessage == "" {
					failedMessage = "Upstream response failed"
			REDACTED
				// response.failed 自带上游已消耗的 usage（input token 通常已扣）；必须先解析
				// 再打 cyber 标记，否则 mark 记到的是解析前的 0，导致流式 cyber 按 0 token 计费
				// 而漏记真实用量。对齐 WS V2 / Chat 流式路径（均先解析 usage 再 Mark）。
				s.parseSSEUsageBytesWithType(dataBytes, eventType, usage)
				if hit, code, msg := detectOpenAICyberPolicy(dataBytes); hit {
					cyberHit = true
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:           code,
						Message:        msg,
						Body:           truncateString(string(dataBytes), 4096),
						UpstreamStatus: http.StatusOK,
						UpstreamInTok:  usage.InputTokens,
						UpstreamOutTok: usage.OutputTokens,
				REDACTED)
			REDACTED
				outputStarted := openAIStreamClientOutputStarted(c, clientOutputStarted)
				if !outputStarted && !cyberHit {
					if compactErr := newOpenAICompactFallbackSignal(c, dataBytes, failedMessage); compactErr != nil {
						return resultWithUsage(), compactErr
				REDACTED
			REDACTED
				if outputStarted && !cyberHit {
					if codexFailureTerminal && eventType == "error" {
						// Wait for the authoritative response.failed before mutating
						// account health; EOF synthesis applies the pending effect.
						bareErrorAccountSideEffectsPending = true
				REDACTED else {
						s.handleOpenAIStreamTerminalAccountSideEffects(c, account, dataBytes, failedMessage, resp.Header)
						bareErrorAccountSideEffectsPending = false
				REDACTED
			REDACTED
				if !outputStarted {
					shouldFailover := false
					if !cyberHit {
						if eventType == "error" {
							shouldFailover = openAIStreamErrorEventShouldFailover(dataBytes, failedMessage)
					REDACTED else {
							shouldFailover = openAIStreamFailedEventShouldFailover(dataBytes, failedMessage)
					REDACTED
				REDACTED
					if shouldFailover {
						return resultWithUsage(),
							s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, dataBytes, failedMessage, resp.Header)
				REDACTED
					if !cyberHit && !sawBareError {
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
							REDACTED,
						REDACTED)
							return resultWithUsage(), fmt.Errorf("upstream response failed: passthrough rule matched message=%s", errMsg)
					REDACTED
				REDACTED
			REDACTED
				forceFlushFailedEvent = true
				sawFailedEvent = true
		REDACTED
			if trimmedData == "[DONE]" {
				sawDone = true
				terminalEventType = "[DONE]"
		REDACTED
			if openAIStreamEventIsTerminalWithType(trimmedData, eventType) {
				sawTerminalEvent = true
				if trimmedData != "[DONE]" {
					terminalEventType = eventType
			REDACTED
		REDACTED
			if responseID == "" {
				responseID = extractOpenAIResponseIDFromJSONBytes(dataBytes)
		REDACTED
			imageCounter.AddSSEData(dataBytes)
			if sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
				dataBytes,
				eventType,
				openAIStreamClientOutputStarted(c, clientOutputStarted),
			); sanitized {
				dataBytes = sanitizedData
				trimmedData = strings.TrimSpace(string(sanitizedData))
				line = "data: " + string(sanitizedData)
		REDACTED
			lineStartsClientOutput = forceFlushFailedEvent || openAIStreamDataStartsClientOutput(trimmedData, eventType)
			if lineStartsClientOutput && trimmedData != "[DONE]" && !openAIStreamEventTypeIsTerminal(eventType) {
				semanticOutputSeen = true
		REDACTED
			// OpenAI Responses streams that terminate with an empty
			// response.completed (no output, no usage, no error, nothing sent
			// to the client) are silent upstream refusals: fail over instead of
			// recording a successful 0/0 usage turn (issue #5009).
			if (eventType == "response.completed" || eventType == "response.done") &&
				!sawFailedEvent && !semanticOutputSeen && !clientOutputStarted &&
				openAIResponsesCompletedEventIsEmpty(dataBytes, usage) {
				return resultWithUsage(), newOpenAIResponsesEmptyCompletedFailoverError(c, account, upstreamRequestID)
		REDACTED
			if firstTokenMs == nil && openAIStreamDataStartsVisibleOutput(trimmedData, eventType) {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
		REDACTED
			s.parseSSEUsageBytesWithType(dataBytes, eventType, usage)
	REDACTED
		if line == "" {
			pendingSSEEventType = ""
			if suppressCurrentEvent {
				suppressCurrentEvent = false
				responseFailedPending = false
				continue
		REDACTED
	REDACTED

		if !clientDisconnected && !failureDelivered && !suppressCurrentEvent {
			if !clientOutputStarted && !lineStartsClientOutput {
				pendingLines = append(pendingLines, line)
				continue
		REDACTED
			if !clientOutputStarted && len(pendingLines) > 0 {
				if !writePendingLines() {
					continue
			REDACTED
		REDACTED
			if _, err := fmt.Fprintln(w, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
		REDACTED else {
				clientOutputStarted = true
				flushPending = true
				if line == "" {
					flushPendingOutput()
			REDACTED
		REDACTED
	REDACTED
		if line == "" && responseFailedPending {
			responseFailedPending = false
			failureDelivered = true
	REDACTED
REDACTED
	ensureResponseFailedTerminal()
	if err := documentScanner.Err(); err != nil {
		if (sawDone || sawTerminalEvent) && !sawFailedEvent {
			s.clearOpenAIProxyStreamDisconnect(account)
			return resultWithUsage(), nil
	REDACTED
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
	REDACTED
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
	REDACTED
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return resultWithUsage(), err
	REDACTED
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
			msg := "OpenAI stream disconnected before completion"
			if errText := strings.TrimSpace(err.Error()); errText != "" {
				msg += ": " + errText
		REDACTED
			return resultWithUsage(),
				s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, msg)
	REDACTED
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", err)
	REDACTED
		s.recordOpenAIProxyStreamDisconnect(account, err, upstreamRequestID)
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI passthrough] 流读取异常中断: account=%d request_id=%s err=%v",
			account.ID,
			upstreamRequestID,
			err,
		)
		return resultWithUsage(), fmt.Errorf("stream read error: %w", err)
REDACTED
	if sawFailedEvent {
		return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
REDACTED
	if !clientDisconnected && !sawDone && !sawTerminalEvent && ctx.Err() == nil {
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.String("upstream_request_id", upstreamRequestID),
		).Info("OpenAI passthrough 上游流在未收到 [DONE] 时结束，疑似断流")
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
			return resultWithUsage(),
				s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, "OpenAI stream ended before a terminal event")
	REDACTED
		s.recordOpenAIProxyStreamDisconnect(account, errors.New("stream ended before terminal event"), upstreamRequestID)
		return resultWithUsage(), errors.New("stream usage incomplete: missing terminal event")
REDACTED
	if (sawDone || sawTerminalEvent) && !sawFailedEvent {
		s.clearOpenAIProxyStreamDisconnect(account)
REDACTED
	logOpenAISuccessMissingUsage(ctx, c, account, resp, usage, terminalEventType, clientDisconnected)

	return resultWithUsage(), nil
REDACTED

func (s *OpenAIGatewayService) handleNonStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	mappedModel string,
) (*openaiNonStreamingResultPassthrough, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
REDACTED
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
REDACTED
	if bodyHasSSEFraming(body) {
		observeOpenAISSEBody(observer, string(body))
REDACTED else {
		observer.ObserveOpenAI(body, strings.TrimSpace(gjson.GetBytes(body, "type").String()))
REDACTED

	// Detect SSE responses from upstream and convert to JSON.
	// Some upstreams (e.g. other sub2api instances) may return SSE even when
	// stream=false was requested. Without this conversion the client would
	// receive raw SSE text or a terminal event with empty output.
	if isEventStreamResponse(resp.Header) {
		return s.handlePassthroughSSEToJSON(resp, c, account, body, originalModel, mappedModel)
REDACTED

	usage := &OpenAIUsage{REDACTED
	usageParsed := false
	if len(body) > 0 {
		if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
			*usage = parsedUsage
			usageParsed = true
	REDACTED
REDACTED
	if !usageParsed {
		// 兜底：尝试从 SSE 文本中解析 usage
		usage = s.parseSSEUsageFromBody(string(body))
REDACTED
	logOpenAISuccessMissingUsage(ctx, c, account, resp, usage, "json", false)

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
REDACTED
	if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
REDACTED
	body, err = restoreOpenAIResponsesNamespacePayload(c, body)
	if err != nil {
		return nil, fmt.Errorf("restore OpenAI passthrough namespace response: %w", err)
REDACTED
	body = restoreCodexToolNamesFromContext(c, body)
	body, err = restoreOpenAIResponsesClientToolPayload(c, body)
	if err != nil {
		return nil, fmt.Errorf("restore OpenAI Responses client tools: %w", err)
REDACTED
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
REDACTED
	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIResponseImageOutputsFromJSONBytes(body),
		imageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(body),
REDACTED, nil
REDACTED

// handlePassthroughSSEToJSON converts an SSE response body into a JSON
// response for the passthrough path. It mirrors handleSSEToJSON while
// preserving passthrough payloads, except compact-only model remapping may
// rewrite model fields back to the original requested model.
func (s *OpenAIGatewayService) handlePassthroughSSEToJSON(resp *http.Response, c *gin.Context, account *Account, body []byte, originalModel string, mappedModel string) (*openaiNonStreamingResultPassthrough, error) {
	bodyText := string(body)
	terminalType, terminalPayload, terminalOK := extractOpenAISSETerminalEvent(bodyText)
	if terminalOK && (terminalType == "response.failed" || terminalType == "error") {
		msg := extractOpenAISSEErrorMessage(terminalPayload)
		if msg == "" {
			msg = "Upstream compact response failed"
	REDACTED
		if compactErr := newOpenAICompactFallbackSignal(c, terminalPayload, msg); compactErr != nil {
			return nil, compactErr
	REDACTED
		return nil, s.writeOpenAINonStreamingProtocolError(resp, c, msg)
REDACTED
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := s.parseSSEUsageFromBody(bodyText)
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
	REDACTED
		// When the terminal event has an empty output array, reconstruct
		// output from accumulated delta events so the client gets full content.
		if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
			if outputJSON, reconstructed := reconstructResponseOutputFromSSE(bodyText); reconstructed {
				if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
					finalResponse = patched
			REDACTED
		REDACTED
	REDACTED
		finalResponse = supplementCompactionItemFromSSE(c, finalResponse, bodyText)
		body = finalResponse
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	REDACTED
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
		restoredBody, restoreErr := restoreOpenAIResponsesNamespacePayload(c, body)
		if restoreErr != nil {
			return nil, fmt.Errorf("restore OpenAI passthrough namespace response: %w", restoreErr)
	REDACTED
		restoredBody = restoreCodexToolNamesFromContext(c, restoredBody)
		body = restoredBody
REDACTED else {
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
	REDACTED
		body = []byte(bodyText)
REDACTED

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	logOpenAISuccessMissingUsage(c.Request.Context(), c, account, resp, usage, terminalType, false)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
	REDACTED
REDACTED
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
REDACTED

	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIImageOutputsFromSSEBody(bodyText),
		imageOutputSizes: collectOpenAIImageOutputSizesFromSSEBody(bodyText),
REDACTED, nil
REDACTED

func writeOpenAIPassthroughResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
REDACTED
	if filter != nil {
		responseheaders.WriteFilteredHeaders(dst, src, filter)
REDACTED else {
		// 兜底：尽量保留最基础的 content-type
		if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
			dst.Set("Content-Type", v)
	REDACTED
REDACTED
	// 透传模式强制放行 x-codex-* 响应头（若上游返回）。
	// 注意：真实 http.Response.Header 的 key 一般会被 canonicalize；但为了兼容测试/自建响应，
	// 这里用 EqualFold 做一次大小写不敏感的查找。
	getCaseInsensitiveValues := func(h http.Header, want string) []string {
		if h == nil {
			return nil
	REDACTED
		for k, vals := range h {
			if strings.EqualFold(k, want) {
				return vals
		REDACTED
	REDACTED
		return nil
REDACTED

	for _, rawKey := range []string{
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
REDACTED {
		vals := getCaseInsensitiveValues(src, rawKey)
		if len(vals) == 0 {
			continue
	REDACTED
		key := http.CanonicalHeaderKey(rawKey)
		dst.Del(key)
		for _, v := range vals {
			dst.Add(key, v)
	REDACTED
REDACTED

	// x-codex-turn-state：Codex 回合状态头，客户端会在同回合后续请求回带。
	// 与上面的用量头不同，这里在上游缺失时也主动清除——failover 换号后残留
	// 上一账号的 blob 会构成跨账号矛盾（openai_codex_turn_state.go）。
	turnStateKey := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	dst.Del(turnStateKey)
	for _, v := range getCaseInsensitiveValues(src, openAICodexTurnStateHeader) {
		dst.Add(turnStateKey, v)
REDACTED
REDACTED
