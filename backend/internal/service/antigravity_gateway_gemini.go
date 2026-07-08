package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// ForwardGemini 转发 Gemini 协议请求
//
// 限流处理流程:
//
//	请求 → antigravityRetryLoop → 预检查(remaining>0? → 切换账号) → 发送上游
//	  ├─ 成功 → 正常返回
//	  └─ 429/503 → handleSmartRetry
//	      ├─ retryDelay >= 7s → 设置模型限流 + 清除粘性绑定 → 切换账号
//	      └─ retryDelay <  7s → 等待后重试 1 次
//	          ├─ 成功 → 正常返回
//	          └─ 失败 → 设置模型限流 + 清除粘性绑定 → 切换账号
type ForwardGeminiOption func(*forwardGeminiOptions)

type forwardGeminiOptions struct {
	groupID     int64
	sessionHash string
REDACTED

func WithForwardGeminiSession(groupID int64, sessionHash string) ForwardGeminiOption {
	return func(opts *forwardGeminiOptions) {
		opts.groupID = groupID
		opts.sessionHash = sessionHash
REDACTED
REDACTED

func (s *AntigravityGatewayService) ForwardGemini(ctx context.Context, c *gin.Context, account *Account, originalModel string, action string, stream bool, body []byte, isStickySession bool, options ...ForwardGeminiOption) (*ForwardResult, error) {
	startTime := time.Now()
	forwardOpts := forwardGeminiOptions{REDACTED
	for _, apply := range options {
		if apply != nil {
			apply(&forwardOpts)
	REDACTED
REDACTED

	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

	if strings.TrimSpace(originalModel) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing model in URL")
REDACTED
	if strings.TrimSpace(action) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing action in URL")
REDACTED
	if len(body) == 0 {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Request body is empty")
REDACTED

	// 解析请求以获取 image_size（用于图片计费）
	imageInputSize := s.extractImageInputSize(body)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)

	switch action {
	case "generateContent", "streamGenerateContent":
		// ok
	case "countTokens":
		// 直接返回空值，不透传上游
		c.JSON(http.StatusOK, map[string]any{"totalTokens": 0REDACTED)
		return &ForwardResult{
			RequestID:    "",
			Usage:        ClaudeUsage{REDACTED,
			Model:        originalModel,
			Stream:       false,
			Duration:     time.Since(startTime),
			FirstTokenMs: nil,
	REDACTED, nil
	default:
		return nil, s.writeGoogleError(c, http.StatusNotFound, "Unsupported action: "+action)
REDACTED

	mappedModel := s.getMappedModel(account, originalModel)
	if mappedModel == "" {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		return nil, s.writeGoogleError(c, http.StatusForbidden, fmt.Sprintf("model %s not in whitelist", originalModel))
REDACTED
	billingModel := mappedModel

	// 获取 access_token
	if s.tokenProvider == nil {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Antigravity token provider not configured")
REDACTED
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, &UpstreamFailoverError{
			StatusCode:   http.StatusBadGateway,
			ResponseBody: []byte(`{"error":{"message":"Failed to get upstream access token","status":"UNAVAILABLE"REDACTEDREDACTED`),
	REDACTED
REDACTED

	projectID, err := resolveAntigravityProjectID(account)
	if err != nil {
		_ = s.writeGoogleError(c, http.StatusBadRequest, err.Error())
		return nil, err
REDACTED

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// Antigravity 上游要求必须包含身份提示词，注入到请求中
	injectedBody, err := injectIdentityPatchToGeminiRequest(body)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Invalid request body")
REDACTED

	// 清理 Schema
	if cleanedBody, err := cleanGeminiRequest(injectedBody); err == nil {
		injectedBody = cleanedBody
		logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] Cleaned request schema in forwarded request for account %s", account.Name)
REDACTED else {
		logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] Failed to clean schema: %v", err)
REDACTED

	// 包装请求
	wrappedBody, err := s.wrapV1InternalRequest(projectID, mappedModel, injectedBody)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusInternalServerError, "Failed to build upstream request")
REDACTED

	// Antigravity 上游只支持流式请求，统一使用 streamGenerateContent
	// 如果客户端请求非流式，在响应处理阶段会收集完整流式响应后返回
	upstreamAction := "streamGenerateContent"

	// 执行带重试的请求
	result, err := s.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             ctx,
		prefix:          prefix,
		account:         account,
		proxyURL:        proxyURL,
		accessToken:     accessToken,
		action:          upstreamAction,
		body:            wrappedBody,
		c:               c,
		httpUpstream:    s.httpUpstream,
		settingService:  s.settingService,
		accountRepo:     s.accountRepo,
		handleError:     s.handleUpstreamError,
		requestedModel:  originalModel,
		isStickySession: isStickySession, // ForwardGemini 由上层判断粘性会话
		groupID:         forwardOpts.groupID,
		sessionHash:     forwardOpts.sessionHash,
REDACTED)
	if err != nil {
		// 检查是否是账号切换信号，转换为 UpstreamFailoverError 让 Handler 切换账号
		if switchErr, ok := IsAntigravityAccountSwitchError(err); ok {
			return nil, &UpstreamFailoverError{
				StatusCode:        http.StatusServiceUnavailable,
				ForceCacheBilling: switchErr.IsStickySession,
		REDACTED
	REDACTED
		// 区分客户端取消和真正的上游失败，返回更准确的错误消息
		if c.Request.Context().Err() != nil {
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Client disconnected before upstream response")
	REDACTED
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries")
REDACTED
	resp := result.resp
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
	REDACTED
REDACTED()

	// 处理错误响应
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		contentType := resp.Header.Get("Content-Type")
		// 尽早关闭原始响应体，释放连接；后续逻辑仍可能需要读取 body，因此用内存副本重新包装。
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		// 模型兜底：模型不存在且开启 fallback 时，自动用 fallback 模型重试一次
		if s.settingService != nil && s.settingService.IsModelFallbackEnabled(ctx) &&
			isModelNotFoundError(resp.StatusCode, respBody) {
			fallbackModel := s.settingService.GetFallbackModel(ctx, PlatformAntigravity)
			if fallbackModel != "" && fallbackModel != mappedModel {
				logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] Model not found (%s), retrying with fallback model %s (account: %s)", mappedModel, fallbackModel, account.Name)

				fallbackWrapped, err := s.wrapV1InternalRequest(projectID, fallbackModel, injectedBody)
				if err == nil {
					fallbackReq, err := antigravity.NewAPIRequest(ctx, upstreamAction, accessToken, fallbackWrapped)
					if err == nil {
						fallbackResp, err := s.httpUpstream.Do(fallbackReq, proxyURL, account.ID, account.Concurrency)
						if err == nil && fallbackResp.StatusCode < 400 {
							_ = resp.Body.Close()
							resp = fallbackResp
					REDACTED else if fallbackResp != nil {
							_ = fallbackResp.Body.Close()
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED

		// Gemini 原生请求中的 thoughtSignature 可能来自旧上下文/旧账号，触发上游严格校验后返回
		// "Corrupted thought signature."。检测到此类 400 时，将 thoughtSignature 清理为 dummy 值后重试一次。
		signatureCheckBody := respBody
		if unwrapped, unwrapErr := s.unwrapV1InternalResponse(respBody); unwrapErr == nil && len(unwrapped) > 0 {
			signatureCheckBody = unwrapped
	REDACTED
		if resp.StatusCode == http.StatusBadRequest &&
			s.settingService != nil &&
			s.settingService.IsSignatureRectifierEnabled(ctx) &&
			isSignatureRelatedError(signatureCheckBody) &&
			bytes.Contains(injectedBody, []byte(`"thoughtSignature"`)) {
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(signatureCheckBody)))
			upstreamDetail := s.getUpstreamErrorDetail(signatureCheckBody)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "signature_error",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
		REDACTED)

			logger.LegacyPrintf("service.antigravity_gateway", "Antigravity Gemini account %d: detected signature-related 400, retrying with cleaned thought signatures", account.ID)

			cleanedInjectedBody := CleanGeminiNativeThoughtSignatures(injectedBody)
			retryWrappedBody, wrapErr := s.wrapV1InternalRequest(projectID, mappedModel, cleanedInjectedBody)
			if wrapErr == nil {
				retryResult, retryErr := s.antigravityRetryLoop(antigravityRetryLoopParams{
					ctx:             ctx,
					prefix:          prefix,
					account:         account,
					proxyURL:        proxyURL,
					accessToken:     accessToken,
					action:          upstreamAction,
					body:            retryWrappedBody,
					c:               c,
					httpUpstream:    s.httpUpstream,
					settingService:  s.settingService,
					accountRepo:     s.accountRepo,
					handleError:     s.handleUpstreamError,
					requestedModel:  originalModel,
					isStickySession: isStickySession,
					groupID:         forwardOpts.groupID,
					sessionHash:     forwardOpts.sessionHash,
			REDACTED)
				if retryErr == nil {
					retryResp := retryResult.resp
					if retryResp.StatusCode < 400 {
						resp = retryResp
				REDACTED else {
						retryRespBody := s.readUpstreamErrorBody(retryResp)
						_ = retryResp.Body.Close()
						retryOpsBody := retryRespBody
						if retryUnwrapped, unwrapErr := s.unwrapV1InternalResponse(retryRespBody); unwrapErr == nil && len(retryUnwrapped) > 0 {
							retryOpsBody = retryUnwrapped
					REDACTED
						appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
							Platform:           account.Platform,
							AccountID:          account.ID,
							AccountName:        account.Name,
							UpstreamStatusCode: retryResp.StatusCode,
							UpstreamRequestID:  retryResp.Header.Get("x-request-id"),
							Kind:               "signature_retry",
							Message:            sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(retryOpsBody))),
							Detail:             s.getUpstreamErrorDetail(retryOpsBody),
					REDACTED)
						respBody = retryRespBody
						resp = &http.Response{
							StatusCode: retryResp.StatusCode,
							Header:     retryResp.Header.Clone(),
							Body:       io.NopCloser(bytes.NewReader(retryRespBody)),
					REDACTED
						contentType = resp.Header.Get("Content-Type")
				REDACTED
			REDACTED else {
					if switchErr, ok := IsAntigravityAccountSwitchError(retryErr); ok {
						appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
							Platform:           account.Platform,
							AccountID:          account.ID,
							AccountName:        account.Name,
							UpstreamStatusCode: http.StatusServiceUnavailable,
							Kind:               "failover",
							Message:            sanitizeUpstreamErrorMessage(retryErr.Error()),
					REDACTED)
						return nil, &UpstreamFailoverError{
							StatusCode:        http.StatusServiceUnavailable,
							ForceCacheBilling: switchErr.IsStickySession,
					REDACTED
				REDACTED
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: 0,
						Kind:               "signature_retry_request_error",
						Message:            sanitizeUpstreamErrorMessage(retryErr.Error()),
				REDACTED)
					logger.LegacyPrintf("service.antigravity_gateway", "Antigravity Gemini account %d: signature retry request failed: %v", account.ID, retryErr)
			REDACTED
		REDACTED else {
				logger.LegacyPrintf("service.antigravity_gateway", "Antigravity Gemini account %d: signature retry wrap failed: %v", account.ID, wrapErr)
		REDACTED
	REDACTED

		// fallback 成功：继续按正常响应处理
		if resp.StatusCode < 400 {
			goto handleSuccess
	REDACTED

		requestID := resp.Header.Get("x-request-id")
		if requestID != "" {
			c.Header("x-request-id", requestID)
	REDACTED

		unwrapped, unwrapErr := s.unwrapV1InternalResponse(respBody)
		unwrappedForOps := unwrapped
		if unwrapErr != nil || len(unwrappedForOps) == 0 {
			unwrappedForOps = respBody
	REDACTED
		s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody, originalModel, forwardOpts.groupID, forwardOpts.sessionHash, isStickySession)
		upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(unwrappedForOps))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		upstreamDetail := s.getUpstreamErrorDetail(unwrappedForOps)

		// Always record upstream context for Ops error logs, even when we will failover.
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

		// 精确匹配服务端配置类 400 错误，触发同账号重试 + failover
		if resp.StatusCode == http.StatusBadRequest && isGoogleProjectConfigError(strings.ToLower(upstreamMsg)) {
			log.Printf("%s status=400 google_config_error failover=true upstream_message=%q account=%d", prefix, upstreamMsg, account.ID)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
		REDACTED)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: unwrappedForOps, RetryableOnSameAccount: trueREDACTED
	REDACTED

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
		REDACTED)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: unwrappedForOpsREDACTED
	REDACTED
		if contentType == "" {
			contentType = "application/json"
	REDACTED
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestID,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
	REDACTED)
		logger.LegacyPrintf("service.antigravity_gateway", "[antigravity-Forward] upstream error status=%d body=%s", resp.StatusCode, truncateForLog(unwrappedForOps, 500))
		MarkResponseCommitted(c)
		c.Data(resp.StatusCode, contentType, unwrappedForOps)
		return nil, fmt.Errorf("antigravity upstream error: %d", resp.StatusCode)
REDACTED

handleSuccess:
	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool

	if stream {
		// 客户端要求流式，直接透传
		streamRes, err := s.handleGeminiStreamingResponse(c, resp, startTime)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_error error=%v", prefix, err)
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
		clientDisconnect = streamRes.clientDisconnect
REDACTED else {
		// 客户端要求非流式，收集流式响应后返回
		streamRes, err := s.handleGeminiStreamToNonStreaming(c, resp, startTime)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_collect_error error=%v", prefix, err)
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED

	if usage == nil {
		usage = &ClaudeUsage{REDACTED
REDACTED

	// 判断是否为图片生成模型
	imageCount := 0
	if isImageGenerationModel(mappedModel) {
		// Gemini 图片生成 API 每次请求只生成一张图片（API 限制）
		imageCount = 1
REDACTED

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    billingModel,
		Stream:           stream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
		ImageCount:       imageCount,
		ImageSize:        imageSize,
		ImageInputSize:   imageInputSize,
REDACTED, nil
REDACTED

// cleanGeminiRequest 清理 Gemini 请求体中的 Schema
func cleanGeminiRequest(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
REDACTED

	modified := false

	// 1. 清理 Tools
	if tools, ok := payload["tools"].([]any); ok && len(tools) > 0 {
		for _, t := range tools {
			toolMap, ok := t.(map[string]any)
			if !ok {
				continue
		REDACTED

			// function_declarations (snake_case) or functionDeclarations (camelCase)
			var funcs []any
			if f, ok := toolMap["functionDeclarations"].([]any); ok {
				funcs = f
		REDACTED else if f, ok := toolMap["function_declarations"].([]any); ok {
				funcs = f
		REDACTED

			if len(funcs) == 0 {
				continue
		REDACTED

			for _, f := range funcs {
				funcMap, ok := f.(map[string]any)
				if !ok {
					continue
			REDACTED

				if params, ok := funcMap["parameters"].(map[string]any); ok {
					antigravity.DeepCleanUndefined(params)
					cleaned := antigravity.CleanJSONSchema(params)
					funcMap["parameters"] = cleaned
					modified = true
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if !modified {
		return body, nil
REDACTED

	return json.Marshal(payload)
REDACTED

// filterEmptyPartsFromGeminiRequest 过滤掉 parts 为空的消息
// Gemini API 不接受空 parts，需要在请求前过滤
func filterEmptyPartsFromGeminiRequest(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
REDACTED

	contents, ok := payload["contents"].([]any)
	if !ok || len(contents) == 0 {
		return body, nil
REDACTED

	filtered := make([]any, 0, len(contents))
	modified := false

	for _, c := range contents {
		contentMap, ok := c.(map[string]any)
		if !ok {
			filtered = append(filtered, c)
			continue
	REDACTED

		parts, hasParts := contentMap["parts"]
		if !hasParts {
			filtered = append(filtered, c)
			continue
	REDACTED

		partsSlice, ok := parts.([]any)
		if !ok {
			filtered = append(filtered, c)
			continue
	REDACTED

		// 跳过 parts 为空数组的消息
		if len(partsSlice) == 0 {
			modified = true
			continue
	REDACTED

		filtered = append(filtered, c)
REDACTED

	if !modified {
		return body, nil
REDACTED

	payload["contents"] = filtered
	return json.Marshal(payload)
REDACTED
