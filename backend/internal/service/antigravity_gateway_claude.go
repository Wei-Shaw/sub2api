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

// Forward 转发 Claude 协议请求（Claude → Gemini 转换）
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
func (s *AntigravityGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte, isStickySession bool) (*ForwardResult, error) {
	// 上游透传账号直接转发，不走 OAuth token 刷新
	if account.Type == AccountTypeUpstream {
		return s.ForwardUpstream(ctx, c, account, body)
	}

	startTime := time.Now()

	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

	// 解析 Claude 请求
	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
	}
	if strings.TrimSpace(claudeReq.Model) == "" {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Missing model")
	}

	originalModel := claudeReq.Model
	mappedModel := s.getMappedModel(account, claudeReq.Model)
	if mappedModel == "" {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		return nil, s.writeClaudeError(c, http.StatusForbidden, "permission_error", fmt.Sprintf("model %s not in whitelist", claudeReq.Model))
	}
	// 应用 thinking 模式自动后缀：如果 thinking 开启且目标是 claude-sonnet-4-5，自动改为 thinking 版本
	thinkingEnabled := claudeReq.Thinking != nil && (claudeReq.Thinking.Type == "enabled" || claudeReq.Thinking.Type == "adaptive")
	mappedModel = applyThinkingModelSuffix(mappedModel, thinkingEnabled)
	billingModel := mappedModel

	// 获取 access_token
	if s.tokenProvider == nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Antigravity token provider not configured")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, &UpstreamFailoverError{
			StatusCode:   http.StatusBadGateway,
			ResponseBody: []byte(`{"error":{"type":"authentication_error","message":"Failed to get upstream access token"},"type":"error"}`),
		}
	}

	projectID, err := resolveAntigravityProjectID(account)
	if err != nil {
		_ = s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 获取转换选项
	// Antigravity 上游要求必须包含身份提示词，否则会返回 429
	transformOpts := s.getClaudeTransformOptions(ctx)
	transformOpts.EnableIdentityPatch = true // 强制启用，Antigravity 上游必需

	// 转换 Claude 请求为 Gemini 格式
	geminiBody, err := antigravity.TransformClaudeToGeminiWithOptions(&claudeReq, projectID, mappedModel, transformOpts)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request")
	}

	// Antigravity 上游只支持流式请求，统一使用 streamGenerateContent
	// 如果客户端请求非流式，在响应处理阶段会收集完整流式响应后转换返回
	action := "streamGenerateContent"

	// 执行带重试的请求
	result, err := s.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             ctx,
		prefix:          prefix,
		account:         account,
		proxyURL:        proxyURL,
		accessToken:     accessToken,
		action:          action,
		body:            geminiBody,
		c:               c,
		httpUpstream:    s.httpUpstream,
		settingService:  s.settingService,
		accountRepo:     s.accountRepo,
		handleError:     s.handleUpstreamError,
		requestedModel:  originalModel,
		isStickySession: isStickySession, // Forward 由上层判断粘性会话
		groupID:         0,               // Forward 方法没有 groupID，由上层处理粘性会话清除
		sessionHash:     "",              // Forward 方法没有 sessionHash，由上层处理粘性会话清除
	})
	if err != nil {
		// 检查是否是账号切换信号，转换为 UpstreamFailoverError 让 Handler 切换账号
		if switchErr, ok := IsAntigravityAccountSwitchError(err); ok {
			return nil, &UpstreamFailoverError{
				StatusCode:        http.StatusServiceUnavailable,
				ForceCacheBilling: switchErr.IsStickySession,
			}
		}
		// 区分客户端取消和真正的上游失败，返回更准确的错误消息
		if c.Request.Context().Err() != nil {
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "client_disconnected", "Client disconnected before upstream response")
		}
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	}
	resp := result.resp
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)

		// 优先检测 thinking block 的 signature 相关错误（400）并重试一次：
		// Antigravity /v1internal 链路在部分场景会对 thought/thinking signature 做严格校验，
		// 当历史消息携带的 signature 不合法时会直接 400；去除 thinking 后可继续完成请求。
		if resp.StatusCode == http.StatusBadRequest && isSignatureRelatedError(respBody) && s.settingService.IsSignatureRectifierEnabled(ctx) {
			upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			logBody, maxBytes := s.getLogConfig()
			upstreamDetail := s.getUpstreamErrorDetail(respBody)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "signature_error",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})

			// Conservative two-stage fallback:
			// 1) Disable top-level thinking + thinking->text
			// 2) Only if still signature-related 400: also downgrade tool_use/tool_result to text.

			retryStages := []struct {
				name  string
				strip func(*antigravity.ClaudeRequest) (bool, error)
			}{
				{name: "thinking-only", strip: stripThinkingFromClaudeRequest},
				{name: "thinking+tools", strip: stripSignatureSensitiveBlocksFromClaudeRequest},
			}

			for _, stage := range retryStages {
				retryClaudeReq := claudeReq
				retryClaudeReq.Messages = append([]antigravity.ClaudeMessage(nil), claudeReq.Messages...)

				stripped, stripErr := stage.strip(&retryClaudeReq)
				if stripErr != nil || !stripped {
					continue
				}

				logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: detected signature-related 400, retrying once (%s)", account.ID, stage.name)

				retryGeminiBody, txErr := antigravity.TransformClaudeToGeminiWithOptions(&retryClaudeReq, projectID, mappedModel, s.getClaudeTransformOptions(ctx))
				if txErr != nil {
					continue
				}
				retryResult, retryErr := s.antigravityRetryLoop(antigravityRetryLoopParams{
					ctx:             ctx,
					prefix:          prefix,
					account:         account,
					proxyURL:        proxyURL,
					accessToken:     accessToken,
					action:          action,
					body:            retryGeminiBody,
					c:               c,
					httpUpstream:    s.httpUpstream,
					settingService:  s.settingService,
					accountRepo:     s.accountRepo,
					handleError:     s.handleUpstreamError,
					requestedModel:  originalModel,
					isStickySession: isStickySession,
					groupID:         0,  // Forward 方法没有 groupID，由上层处理粘性会话清除
					sessionHash:     "", // Forward 方法没有 sessionHash，由上层处理粘性会话清除
				})
				if retryErr != nil {
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: 0,
						Kind:               "signature_retry_request_error",
						Message:            sanitizeUpstreamErrorMessage(retryErr.Error()),
					})
					logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: signature retry request failed (%s): %v", account.ID, stage.name, retryErr)
					continue
				}

				retryResp := retryResult.resp
				if retryResp.StatusCode < 400 {
					_ = resp.Body.Close()
					resp = retryResp
					respBody = nil
					break
				}

				retryBody, _ := io.ReadAll(io.LimitReader(retryResp.Body, 8<<10))
				_ = retryResp.Body.Close()
				if retryResp.StatusCode == http.StatusTooManyRequests {
					retryBaseURL := ""
					if retryResp.Request != nil && retryResp.Request.URL != nil {
						retryBaseURL = retryResp.Request.URL.Scheme + "://" + retryResp.Request.URL.Host
					}
					logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 rate_limited base_url=%s retry_stage=%s body=%s", prefix, retryBaseURL, stage.name, truncateForLog(retryBody, 200))
				}
				kind := "signature_retry"
				if strings.TrimSpace(stage.name) != "" {
					kind = "signature_retry_" + strings.ReplaceAll(stage.name, "+", "_")
				}
				retryUpstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(retryBody))
				retryUpstreamMsg = sanitizeUpstreamErrorMessage(retryUpstreamMsg)
				retryUpstreamDetail := ""
				if logBody {
					retryUpstreamDetail = truncateString(string(retryBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: retryResp.StatusCode,
					UpstreamRequestID:  retryResp.Header.Get("x-request-id"),
					Kind:               kind,
					Message:            retryUpstreamMsg,
					Detail:             retryUpstreamDetail,
				})

				// If this stage fixed the signature issue, we stop; otherwise we may try the next stage.
				if retryResp.StatusCode != http.StatusBadRequest || !isSignatureRelatedError(retryBody) {
					respBody = retryBody
					resp = &http.Response{
						StatusCode: retryResp.StatusCode,
						Header:     retryResp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(retryBody)),
					}
					break
				}

				// Still signature-related; capture context and allow next stage.
				respBody = retryBody
				resp = &http.Response{
					StatusCode: retryResp.StatusCode,
					Header:     retryResp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(retryBody)),
				}
			}
		}

		// Budget 整流：检测 budget_tokens 约束错误并自动修正重试
		if resp.StatusCode == http.StatusBadRequest && respBody != nil && !isSignatureRelatedError(respBody) {
			errMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
			if isThinkingBudgetConstraintError(errMsg) && s.settingService.IsBudgetRectifierEnabled(ctx) {
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "budget_constraint_error",
					Message:            errMsg,
					Detail:             s.getUpstreamErrorDetail(respBody),
				})

				// 修正 claudeReq 的 thinking 参数（adaptive 模式不修正）
				if claudeReq.Thinking == nil || claudeReq.Thinking.Type != "adaptive" {
					retryClaudeReq := claudeReq
					retryClaudeReq.Messages = append([]antigravity.ClaudeMessage(nil), claudeReq.Messages...)
					// 创建新的 ThinkingConfig 避免修改原始 claudeReq.Thinking 指针
					retryClaudeReq.Thinking = &antigravity.ThinkingConfig{
						Type:         "enabled",
						BudgetTokens: BudgetRectifyBudgetTokens,
					}
					if retryClaudeReq.MaxTokens < BudgetRectifyMinMaxTokens {
						retryClaudeReq.MaxTokens = BudgetRectifyMaxTokens
					}

					logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: detected budget_tokens constraint error, retrying with rectified budget (budget_tokens=%d, max_tokens=%d)", account.ID, BudgetRectifyBudgetTokens, BudgetRectifyMaxTokens)

					retryGeminiBody, txErr := antigravity.TransformClaudeToGeminiWithOptions(&retryClaudeReq, projectID, mappedModel, transformOpts)
					if txErr == nil {
						retryResult, retryErr := s.antigravityRetryLoop(antigravityRetryLoopParams{
							ctx:             ctx,
							prefix:          prefix,
							account:         account,
							proxyURL:        proxyURL,
							accessToken:     accessToken,
							action:          action,
							body:            retryGeminiBody,
							c:               c,
							httpUpstream:    s.httpUpstream,
							settingService:  s.settingService,
							accountRepo:     s.accountRepo,
							handleError:     s.handleUpstreamError,
							requestedModel:  originalModel,
							isStickySession: isStickySession,
							groupID:         0,
							sessionHash:     "",
						})
						if retryErr == nil {
							retryResp := retryResult.resp
							if retryResp.StatusCode < 400 {
								_ = resp.Body.Close()
								resp = retryResp
								respBody = nil
							} else {
								retryBody := s.readUpstreamErrorBody(retryResp)
								_ = retryResp.Body.Close()
								respBody = retryBody
								resp = &http.Response{
									StatusCode: retryResp.StatusCode,
									Header:     retryResp.Header.Clone(),
									Body:       io.NopCloser(bytes.NewReader(retryBody)),
								}
							}
						} else {
							logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: budget rectifier retry failed: %v", account.ID, retryErr)
						}
					}
				}
			}
		}

		// 处理错误响应（重试后仍失败或不触发重试）
		if resp.StatusCode >= 400 {
			// 检测 prompt too long 错误，返回特殊错误类型供上层 fallback
			if resp.StatusCode == http.StatusBadRequest && isPromptTooLongError(respBody) {
				upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := s.getUpstreamErrorDetail(respBody)
				logBody, maxBytes := s.getLogConfig()
				if logBody {
					logger.LegacyPrintf("service.antigravity_gateway", "%s status=400 prompt_too_long=true upstream_message=%q request_id=%s body=%s", prefix, upstreamMsg, resp.Header.Get("x-request-id"), truncateForLog(respBody, maxBytes))
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "prompt_too_long",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return nil, &PromptTooLongError{
					StatusCode: resp.StatusCode,
					RequestID:  resp.Header.Get("x-request-id"),
					Body:       respBody,
				}
			}

			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody, originalModel, 0, "", isStickySession)

			// 精确匹配服务端配置类 400 错误，触发同账号重试 + failover
			if resp.StatusCode == http.StatusBadRequest {
				msg := strings.ToLower(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
				if isGoogleProjectConfigError(msg) {
					upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
					upstreamDetail := s.getUpstreamErrorDetail(respBody)
					log.Printf("%s status=400 google_config_error failover=true upstream_message=%q account=%d", prefix, upstreamMsg, account.ID)
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: resp.StatusCode,
						UpstreamRequestID:  resp.Header.Get("x-request-id"),
						Kind:               "failover",
						Message:            upstreamMsg,
						Detail:             upstreamDetail,
					})
					return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: true}
				}
			}

			if s.shouldFailoverUpstreamError(resp.StatusCode) {
				upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := s.getUpstreamErrorDetail(respBody)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
			}

			return nil, s.writeMappedClaudeError(c, account, resp.StatusCode, resp.Header.Get("x-request-id"), respBody)
		}
	}

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if claudeReq.Stream {
		// 客户端要求流式，直接透传转换
		streamRes, err := s.handleClaudeStreamingResponse(c, resp, startTime, originalModel)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_error error=%v", prefix, err)
			return nil, err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
		clientDisconnect = streamRes.clientDisconnect
	} else {
		// 客户端要求非流式，收集流式响应后转换返回
		streamRes, err := s.handleClaudeStreamToNonStreaming(c, resp, startTime, originalModel)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_collect_error error=%v", prefix, err)
			return nil, err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    billingModel,
		Stream:           claudeReq.Stream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

func isSignatureRelatedError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
	if msg == "" {
		// Fallback: best-effort scan of the raw payload.
		msg = strings.ToLower(string(respBody))
	}

	// Keep this intentionally broad: different upstreams may use "signature" or "thought_signature".
	if strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature") {
		return true
	}

	// Also detect thinking block structural errors:
	// "Expected `thinking` or `redacted_thinking`, but found `text`"
	if strings.Contains(msg, "expected") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		return true
	}

	return false
}

// isPromptTooLongError 检测是否为 prompt too long 错误
func isPromptTooLongError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
	if msg == "" {
		msg = strings.ToLower(string(respBody))
	}
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "request is too long") ||
		strings.Contains(msg, "context length exceeded") ||
		strings.Contains(msg, "max_tokens")
}

// isPassthroughErrorMessage 检查错误消息是否在透传白名单中
func isPassthroughErrorMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, pattern := range antigravityPassthroughErrorMessages {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// getPassthroughOrDefault 若消息在白名单内则返回原始消息，否则返回默认消息
func getPassthroughOrDefault(upstreamMsg, defaultMsg string) string {
	if isPassthroughErrorMessage(upstreamMsg) {
		return upstreamMsg
	}
	return defaultMsg
}

func extractAntigravityErrorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	// Google-style: {"error": {"message": "..."}}
	if errObj, ok := payload["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return msg
		}
	}

	// Fallback: top-level message
	if msg, ok := payload["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}

	return ""
}

// stripThinkingFromClaudeRequest converts thinking blocks to text blocks in a Claude Messages request.
// This preserves the thinking content while avoiding signature validation errors.
// Note: redacted_thinking blocks are removed because they cannot be converted to text.
// It also disables top-level `thinking` to avoid upstream structural constraints for thinking mode.
func stripThinkingFromClaudeRequest(req *antigravity.ClaudeRequest) (bool, error) {
	if req == nil {
		return false, nil
	}

	changed := false
	if req.Thinking != nil {
		req.Thinking = nil
		changed = true
	}

	for i := range req.Messages {
		raw := req.Messages[i].Content
		if len(raw) == 0 {
			continue
		}

		// If content is a string, nothing to strip.
		var str string
		if json.Unmarshal(raw, &str) == nil {
			continue
		}

		// Otherwise treat as an array of blocks and convert thinking blocks to text.
		var blocks []map[string]any
		if err := json.Unmarshal(raw, &blocks); err != nil {
			continue
		}

		filtered := make([]map[string]any, 0, len(blocks))
		modifiedAny := false
		for _, block := range blocks {
			t, _ := block["type"].(string)
			switch t {
			case "thinking":
				thinkingText, _ := block["thinking"].(string)
				if thinkingText != "" {
					filtered = append(filtered, map[string]any{
						"type": "text",
						"text": thinkingText,
					})
				}
				modifiedAny = true
			case "redacted_thinking":
				modifiedAny = true
			case "":
				if thinkingText, hasThinking := block["thinking"].(string); hasThinking {
					if thinkingText != "" {
						filtered = append(filtered, map[string]any{
							"type": "text",
							"text": thinkingText,
						})
					}
					modifiedAny = true
				} else {
					filtered = append(filtered, block)
				}
			default:
				filtered = append(filtered, block)
			}
		}

		if !modifiedAny {
			continue
		}

		if len(filtered) == 0 {
			filtered = append(filtered, map[string]any{
				"type": "text",
				"text": "(content removed)",
			})
		}

		newRaw, err := json.Marshal(filtered)
		if err != nil {
			return changed, err
		}
		req.Messages[i].Content = newRaw
		changed = true
	}

	return changed, nil
}

// stripSignatureSensitiveBlocksFromClaudeRequest is a stronger retry degradation that additionally converts
// tool blocks to plain text. Use this only after a thinking-only retry still fails with signature errors.
func stripSignatureSensitiveBlocksFromClaudeRequest(req *antigravity.ClaudeRequest) (bool, error) {
	if req == nil {
		return false, nil
	}

	changed := false
	if req.Thinking != nil {
		req.Thinking = nil
		changed = true
	}

	for i := range req.Messages {
		raw := req.Messages[i].Content
		if len(raw) == 0 {
			continue
		}

		// If content is a string, nothing to strip.
		var str string
		if json.Unmarshal(raw, &str) == nil {
			continue
		}

		// Otherwise treat as an array of blocks and convert signature-sensitive blocks to text.
		var blocks []map[string]any
		if err := json.Unmarshal(raw, &blocks); err != nil {
			continue
		}

		filtered := make([]map[string]any, 0, len(blocks))
		modifiedAny := false
		for _, block := range blocks {
			t, _ := block["type"].(string)
			switch t {
			case "thinking":
				// Convert thinking to text, skip if empty
				thinkingText, _ := block["thinking"].(string)
				if thinkingText != "" {
					filtered = append(filtered, map[string]any{
						"type": "text",
						"text": thinkingText,
					})
				}
				modifiedAny = true
			case "redacted_thinking":
				// Remove redacted_thinking (cannot convert encrypted content)
				modifiedAny = true
			case "tool_use":
				// Convert tool_use to text to avoid upstream signature/thought_signature validation errors.
				// This is a retry-only degradation path, so we prioritise request validity over tool semantics.
				name, _ := block["name"].(string)
				id, _ := block["id"].(string)
				input := block["input"]
				inputJSON, _ := json.Marshal(input)
				text := "(tool_use)"
				if name != "" {
					text += " name=" + name
				}
				if id != "" {
					text += " id=" + id
				}
				if len(inputJSON) > 0 && string(inputJSON) != "null" {
					text += " input=" + string(inputJSON)
				}
				filtered = append(filtered, map[string]any{
					"type": "text",
					"text": text,
				})
				modifiedAny = true
			case "tool_result":
				// Convert tool_result to text so it stays consistent when tool_use is downgraded.
				toolUseID, _ := block["tool_use_id"].(string)
				isError, _ := block["is_error"].(bool)
				content := block["content"]
				contentJSON, _ := json.Marshal(content)
				text := "(tool_result)"
				if toolUseID != "" {
					text += " tool_use_id=" + toolUseID
				}
				if isError {
					text += " is_error=true"
				}
				if len(contentJSON) > 0 && string(contentJSON) != "null" {
					text += "\n" + string(contentJSON)
				}
				filtered = append(filtered, map[string]any{
					"type": "text",
					"text": text,
				})
				modifiedAny = true
			case "":
				// Handle untyped block with "thinking" field
				if thinkingText, hasThinking := block["thinking"].(string); hasThinking {
					if thinkingText != "" {
						filtered = append(filtered, map[string]any{
							"type": "text",
							"text": thinkingText,
						})
					}
					modifiedAny = true
				} else {
					filtered = append(filtered, block)
				}
			default:
				filtered = append(filtered, block)
			}
		}

		if !modifiedAny {
			continue
		}

		if len(filtered) == 0 {
			// Keep request valid: upstream rejects empty content arrays.
			filtered = append(filtered, map[string]any{
				"type": "text",
				"text": "(content removed)",
			})
		}

		newRaw, err := json.Marshal(filtered)
		if err != nil {
			return changed, err
		}
		req.Messages[i].Content = newRaw
		changed = true
	}

	return changed, nil
}
