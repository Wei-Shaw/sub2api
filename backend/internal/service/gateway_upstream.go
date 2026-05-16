package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

// executeBedrockUpstream 执行 Bedrock 上游请求（含重试逻辑）
func (s *GatewayService) executeBedrockUpstream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	modelID string,
	region string,
	stream bool,
	signer *BedrockSigner,
	apiKey string,
	proxyURL string,
) (*http.Response, error) {
	var resp *http.Response
	var err error
	retryStart := time.Now()
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		var upstreamReq *http.Request
		if account.IsBedrockAPIKey() {
			upstreamReq, err = s.buildUpstreamRequestBedrockAPIKey(ctx, body, modelID, region, stream, apiKey)
		} else {
			upstreamReq, err = s.buildUpstreamRequestBedrock(ctx, body, modelID, region, stream, signer)
		}
		if err != nil {
			return nil, err
		}

		resp, err = s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, nil)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}

		if resp.StatusCode >= 400 && resp.StatusCode != 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetryAttempts {
				elapsed := time.Since(retryStart)
				if elapsed >= maxRetryElapsed {
					break
				}

				delay := retryBackoffDelay(attempt)
				remaining := maxRetryElapsed - elapsed
				if delay > remaining {
					delay = remaining
				}
				if delay <= 0 {
					break
				}

				respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
					Kind:               "retry",
					Message:            extractUpstreamErrorMessage(respBody),
					Detail: func() string {
						if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
							return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
						}
						return ""
					}(),
				})
				logger.LegacyPrintf("service.gateway", "[Bedrock] account %d: upstream error %d, retry %d/%d after %v",
					account.ID, resp.StatusCode, attempt, maxRetryAttempts, delay)
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			break
		}

		break
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("upstream request failed: empty response")
	}
	return resp, nil
}

// handleBedrockUpstreamErrors 处理 Bedrock 上游 4xx/5xx 错误（failover + 错误响应）
func (s *GatewayService) handleBedrockUpstreamErrors(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ForwardResult, error) {
	// retry exhausted + failover
	if s.shouldRetryUpstreamError(account, resp.StatusCode) {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			logger.LegacyPrintf("service.gateway", "[Bedrock] Upstream error (retry exhausted, failover): Account=%d(%s) Status=%d Body=%s",
				account.ID, account.Name, resp.StatusCode, truncateString(string(respBody), 1000))

			s.handleRetryExhaustedSideEffects(ctx, resp, account)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				Kind:               "retry_exhausted_failover",
				Message:            extractUpstreamErrorMessage(respBody),
			})
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleRetryExhaustedError(ctx, resp, c, account)
	}

	// non-retryable failover
	if s.shouldFailoverUpstreamError(resp.StatusCode) {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		s.handleFailoverSideEffects(ctx, resp, account)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			Kind:               "failover",
			Message:            extractUpstreamErrorMessage(respBody),
		})
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	// other errors
	return s.handleErrorResponse(ctx, resp, c, account)
}

// buildUpstreamRequestBedrock 构建 Bedrock 上游请求
func (s *GatewayService) buildUpstreamRequestBedrock(
	ctx context.Context,
	body []byte,
	modelID string,
	region string,
	stream bool,
	signer *BedrockSigner,
) (*http.Request, error) {
	targetURL := BuildBedrockURL(region, modelID, stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// SigV4 签名
	if err := signer.SignRequest(ctx, req, body); err != nil {
		return nil, fmt.Errorf("sign bedrock request: %w", err)
	}

	return req, nil
}

// buildUpstreamRequestBedrockAPIKey 构建 Bedrock API Key (Bearer Token) 上游请求
func (s *GatewayService) buildUpstreamRequestBedrockAPIKey(
	ctx context.Context,
	body []byte,
	modelID string,
	region string,
	stream bool,
	apiKey string,
) (*http.Request, error) {
	targetURL := BuildBedrockURL(region, modelID, stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	return req, nil
}

// handleBedrockNonStreamingResponse 处理 Bedrock 非流式响应
// Bedrock InvokeModel 非流式响应的 body 格式与 Claude API 兼容
func (s *GatewayService) handleBedrockNonStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ClaudeUsage, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, anthropicTooLargeError)
	if err != nil {
		return nil, err
	}

	// 转换 Bedrock 特有的 amazon-bedrock-invocationMetrics 为标准 Anthropic usage 格式
	// 并移除该字段避免透传给客户端
	body = transformBedrockInvocationMetrics(body)

	usage := parseClaudeUsageFromResponseBody(body)

	c.Header("Content-Type", "application/json")
	if v := resp.Header.Get("x-amzn-requestid"); v != "" {
		c.Header("x-request-id", v)
	}
	c.Data(resp.StatusCode, "application/json", body)
	return usage, nil
}

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string, reqStream bool, mimicClaudeCode bool) (*http.Request, error) {
	if account.Platform == PlatformAnthropic && account.Type == AccountTypeServiceAccount {
		return s.buildUpstreamRequestAnthropicVertex(ctx, c, account, body, token, modelID, reqStream)
	}

	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == AccountTypeAPIKey {
		baseURL := account.GetBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = validatedURL + "/v1/messages?beta=true"
		}
	} else if account.IsCustomBaseURLEnabled() {
		customURL := account.GetCustomBaseURL()
		if customURL == "" {
			return nil, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
		}
		validatedURL, err := s.validateUpstreamBaseURL(customURL)
		if err != nil {
			return nil, err
		}
		targetURL = s.buildCustomRelayURL(validatedURL, "/v1/messages", account)
	}

	clientHeaders := http.Header{}
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
	}

	// OAuth账号：应用统一指纹和metadata重写（受设置开关控制）
	var fingerprint *Fingerprint
	enableFP, enableMPT, enableCCH := true, false, false
	if s.settingService != nil {
		enableFP, enableMPT, enableCCH = s.settingService.GetGatewayForwardingSettings(ctx)
	}
	if account.IsOAuth() && s.identityService != nil {
		// 1. 获取或创建指纹（包含随机生成的ClientID）
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, clientHeaders)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Warning: failed to get fingerprint for account %d: %v", account.ID, err)
			// 失败时降级为透传原始headers
		} else {
			if enableFP {
				fingerprint = fp
			}

			// 2. 重写metadata.user_id（需要指纹中的ClientID和账号的account_uuid）
			// 如果启用了会话ID伪装，会在重写后替换 session 部分为固定值
			// 当 metadata 透传开启时跳过重写
			if !enableMPT {
				accountUUID := account.GetExtraString("account_uuid")
				if accountUUID != "" && fp.ClientID != "" {
					if newBody, err := s.identityService.RewriteUserIDWithMasking(ctx, body, account, accountUUID, fp.ClientID, fp.UserAgent); err == nil && len(newBody) > 0 {
						body = newBody
					}
				}
			}
		}
	}

	// 同步 billing header cc_version 与实际发送的 User-Agent 版本
	if fingerprint != nil {
		body = syncBillingHeaderVersion(body, fingerprint.UserAgent)
	}
	// CCH 签名：将 cch=00000 占位符替换为 xxHash64 签名（需在所有 body 修改之后）
	if enableCCH {
		body = signBillingHeaderCCH(body)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 设置认证头（保持原始大小写）
	if tokenType == "oauth" {
		setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	} else {
		setHeaderRaw(req.Header, "x-api-key", token)
	}

	// 白名单透传 headers
	// OAuth mimicry 路径：跳过客户端 header 透传，与 Parrot 对齐。
	// Parrot 的 build_upstream_headers 只发 9 个精确 header，不透传任何客户端 header。
	// 透传客户端 header 会引入不一致的 x-stainless-* / anthropic-beta / user-agent /
	// x-claude-code-session-id 等值，和我们注入的伪装 header 冲突，被 Anthropic 判 third-party。
	if tokenType != "oauth" || !mimicClaudeCode {
		for key, values := range clientHeaders {
			lowerKey := strings.ToLower(key)
			if allowedHeaders[lowerKey] {
				wireKey := resolveWireCasing(key)
				for _, v := range values {
					addHeaderRaw(req.Header, wireKey, v)
				}
			}
		}
	}

	// OAuth账号：应用缓存的指纹到请求头（覆盖白名单透传的头）
	if fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
	}

	// 确保必要的headers存在（保持原始大小写）
	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
	}
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	}
	if tokenType == "oauth" {
		applyClaudeOAuthHeaderDefaults(req)
	}

	// Build effective drop set: merge static defaults with dynamic beta policy filter rules
	policyFilterSet := s.getBetaPolicyFilterSet(ctx, c, account, modelID)
	effectiveDropSet := mergeDropSets(policyFilterSet)

	// 处理 anthropic-beta header（OAuth 账号需要包含 oauth beta）
	if tokenType == "oauth" {
		if mimicClaudeCode {
			// 非 Claude Code 客户端：按 opencode 的策略处理：
			// - 强制 Claude Code 指纹相关请求头（尤其是 user-agent/x-stainless/x-app）
			// - 保留 incoming beta 的同时，确保 OAuth 所需 beta 存在
			applyClaudeCodeMimicHeaders(req, reqStream)

			incomingBeta := getHeaderRaw(req.Header, "anthropic-beta")
			// Claude Code OAuth credentials are scoped to Claude Code.
			// Non-haiku models MUST include claude-code beta for Anthropic to recognize
			// this as a legitimate Claude Code request; without it, the request is
			// rejected as third-party ("out of extra usage").
			// Haiku models are exempt from third-party detection and don't need it.
			requiredBetas := []string{claude.BetaOAuth, claude.BetaInterleavedThinking}
			if !strings.Contains(strings.ToLower(modelID), "haiku") {
				requiredBetas = claude.FullClaudeCodeMimicryBetas()
			}
			setHeaderRaw(req.Header, "anthropic-beta", mergeAnthropicBetaDropping(requiredBetas, incomingBeta, effectiveDropSet))
		} else {
			// Claude Code 客户端：尽量透传原始 header，仅补齐 oauth beta
			clientBetaHeader := getHeaderRaw(req.Header, "anthropic-beta")
			setHeaderRaw(req.Header, "anthropic-beta", stripBetaTokensWithSet(s.getBetaHeader(modelID, clientBetaHeader), effectiveDropSet))
		}
	} else {
		// API-key accounts: apply beta policy filter to strip controlled tokens
		if existingBeta := getHeaderRaw(req.Header, "anthropic-beta"); existingBeta != "" {
			setHeaderRaw(req.Header, "anthropic-beta", stripBetaTokensWithSet(existingBeta, effectiveDropSet))
		} else if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey {
			// API-key：仅在请求显式使用 beta 特性且客户端未提供时，按需补齐（默认关闭）
			if requestNeedsBetaFeatures(body) {
				if beta := defaultAPIKeyBetaHeader(body); beta != "" {
					setHeaderRaw(req.Header, "anthropic-beta", beta)
				}
			}
		}
	}

	// 同步 X-Claude-Code-Session-Id 头：取 body 中已处理的 metadata.user_id 的 session_id 覆盖
	if sessionHeader := getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"); sessionHeader != "" {
		if uid := gjson.GetBytes(body, "metadata.user_id").String(); uid != "" {
			if parsed := ParseMetadataUserID(uid); parsed != nil {
				setHeaderRaw(req.Header, "X-Claude-Code-Session-Id", parsed.SessionID)
			}
		}
	}

	// === DEBUG: 打印上游转发请求（headers + body 摘要），与 CLIENT_ORIGINAL 对比 ===
	s.debugLogGatewaySnapshot("UPSTREAM_FORWARD", req.Header, body, map[string]string{
		"url":                 req.URL.String(),
		"token_type":          tokenType,
		"mimic_claude_code":   strconv.FormatBool(mimicClaudeCode),
		"fingerprint_applied": strconv.FormatBool(fingerprint != nil),
		"enable_fp":           strconv.FormatBool(enableFP),
		"enable_mpt":          strconv.FormatBool(enableMPT),
	})

	// Always capture a compact fingerprint line for later error diagnostics.
	// We only print it when needed (or when the explicit debug flag is enabled).
	if c != nil && tokenType == "oauth" {
		c.Set(claudeMimicDebugInfoKey, buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode))
	}
	if s.debugClaudeMimicEnabled() {
		logClaudeMimicDebug(req, body, account, tokenType, mimicClaudeCode)
	}

	return req, nil
}

func (s *GatewayService) buildUpstreamRequestAnthropicVertex(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	modelID string,
	reqStream bool,
) (*http.Request, error) {
	vertexBody, err := buildVertexAnthropicRequestBody(body)
	if err != nil {
		return nil, err
	}
	setOpsUpstreamRequestBody(c, vertexBody)
	fullURL, err := buildVertexAnthropicURL(account.VertexProjectID(), account.VertexLocation(modelID), modelID, reqStream)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(vertexBody))
	if err != nil {
		return nil, err
	}

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if !allowedHeaders[lowerKey] || lowerKey == "anthropic-version" {
				continue
			}
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
			}
		}
	}

	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Del("cookie")
	req.Header.Del("anthropic-version")
	setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	setHeaderRaw(req.Header, "content-type", "application/json")

	s.debugLogGatewaySnapshot("UPSTREAM_FORWARD_VERTEX_ANTHROPIC", req.Header, vertexBody, map[string]string{
		"url":        req.URL.String(),
		"token_type": "service_account",
		"model":      modelID,
		"stream":     strconv.FormatBool(reqStream),
	})

	return req, nil
}

// buildCustomRelayURL 构建自定义中继转发 URL
// 在 path 后附加 beta=true 和可选的 proxy 查询参数
func (s *GatewayService) buildCustomRelayURL(baseURL, path string, account *Account) string {
	u := strings.TrimRight(baseURL, "/") + path + "?beta=true"
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL := account.Proxy.URL()
		if proxyURL != "" {
			u += "&proxy=" + url.QueryEscape(proxyURL)
		}
	}
	return u
}

func (s *GatewayService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return "", fmt.Errorf("invalid base_url: %w", err)
		}
		return normalized, nil
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
}
