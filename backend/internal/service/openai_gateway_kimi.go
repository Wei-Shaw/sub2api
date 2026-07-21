package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// forwardKimiChatCompletions 直转客户端的 Chat Completions 请求到 Kimi 上游
// `{base_url}/chat/completions`（kimi.com 订阅的 OpenAI 兼容端点，原生支持 CC 协议）。
//
// 对标 grok 的接入方式，差异点：
//   - 上游只支持 /chat/completions（无 /responses 端点），不做 CC↔Responses 协议转换；
//   - 强制携带 8 个指纹头（UA 前缀必须 KimiCLI/，X-Msh-* 系列），缺失会被 403/429；
//   - X-Msh-Device-Id 使用账号 credentials 中持久化的稳定 device_id（绑进签发的 token）。
//
// 错误冷却语义对标 grok：401→临时停调度 10min、403→30min、429→Retry-After、5xx→2min。
func (s *OpenAIGatewayService) forwardKimiChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	if account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("kimi account type %s is not supported by subscription forwarding", account.Type)
	}

	// 1. 解析路由/计费所需的最小字段
	originalModel := gjson.GetBytes(body, "model").String()
	if originalModel == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()

	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, originalModel)
	serviceTier := extractOpenAIServiceTierFromBody(body)

	// 2. 模型映射（未命中映射时透传原模型；空模型兜底 kimi-for-coding）
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := strings.TrimSpace(billingModel)
	if upstreamModel == "" {
		upstreamModel = "kimi-for-coding"
	}
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)

	// 3. body patch：仅改写 model；流式强制 include_usage 保证计费完整。
	// thinking / reasoning_effort / prompt_cache_key 为 Kimi 原生字段，直接透传。
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}
	if clientStream {
		var usageErr error
		upstreamBody, usageErr = ensureOpenAIChatStreamUsage(upstreamBody)
		if usageErr != nil {
			return nil, fmt.Errorf("enable stream usage: %w", usageErr)
		}
	}

	// 4. 获取访问令牌
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 5. 构造上游请求（含 8 个强制指纹头）
	targetURL, err := kimi.BuildChatCompletionsURL(account.GetKimiBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid kimi base_url: %w", err)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	deviceID := account.GetKimiDeviceID()
	if strings.TrimSpace(deviceID) == "" {
		// device_id 正常由建号流程写入 credentials；缺失时生成临时值兜底
		// （临时值未绑进 token，仅保证指纹头完整，后续刷新会回写稳定值）。
		if generated, genErr := kimi.GenerateDeviceID(); genErr == nil {
			deviceID = generated
		}
	}
	kimi.SetFingerprintHeaders(upstreamReq.Header, deviceID)

	// 6. 发送请求
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	// 7. 错误响应处理（冷却语义对标 grok）
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("kimi upstream returned status %d", resp.StatusCode)
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "failover",
			Message:            upstreamMsg,
		})
		s.handleKimiAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleChatCompletionsErrorResponse(resp, c, account, billingModel)
	}

	// 8. 透传上游 CC 响应（上游 chunk 已是 CC 格式）
	if clientStream {
		return s.streamRawChatCompletions(c, resp, account, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime, len(body))
	}
	return s.bufferRawChatCompletions(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
}

// handleKimiAccountUpstreamError 按上游状态码对账号施加冷却（语义对标 grok）：
// 401→10min（token 失效）、403→30min（指纹/套餐校验不通过）、429→Retry-After、5xx→2min。
func (s *OpenAIGatewayService) handleKimiAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) {
	if s == nil || account == nil {
		return
	}
	switch statusCode {
	case http.StatusUnauthorized:
		s.tempUnscheduleKimi(ctx, account, 10*time.Minute, "kimi oauth token unauthorized")
	case http.StatusForbidden:
		s.tempUnscheduleKimi(ctx, account, 30*time.Minute, "kimi fingerprint or subscription denied")
	case http.StatusTooManyRequests:
		cooldown := 2 * time.Minute
		if resetAt := parseRetryAfterResetTime(headers, time.Now()); resetAt != nil {
			if d := time.Until(*resetAt); d > 0 {
				cooldown = d
			}
		}
		s.tempUnscheduleKimi(ctx, account, cooldown, "kimi rate limited")
	default:
		if statusCode >= 500 {
			s.tempUnscheduleKimi(ctx, account, 2*time.Minute, "kimi upstream temporary error")
		}
	}
	_ = responseBody
}

func (s *OpenAIGatewayService) tempUnscheduleKimi(ctx context.Context, account *Account, cooldown time.Duration, reason string) {
	if s == nil || account == nil {
		return
	}
	until := time.Now().Add(cooldown)
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(until) {
		until = *account.TempUnschedulableUntil
	}
	s.BlockAccountScheduling(account, until, reason)
	if s.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		defer cancel()
		_ = s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason)
	}
}
