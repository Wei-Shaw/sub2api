package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

type kiroEndpointConfig struct {
	URL       string
	AmzTarget string
	Name      string
}

const kiroInvalidModelTempUnschedDuration = time.Minute

const (
	kiroRetryBaseDelay = 200 * time.Millisecond
	kiroRetryMaxDelay  = 2 * time.Second
)

var kiroRetrySleep = sleepWithContext

func kiroRetryBackoffDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := kiroRetryBaseDelay * time.Duration(1<<attempt)
	if delay > kiroRetryMaxDelay {
		delay = kiroRetryMaxDelay
	}
	jitterMax := delay / 4
	if jitterMax <= 0 {
		return delay
	}
	return delay + time.Duration(mathrand.Int63n(int64(jitterMax)+1))
}

func sleepKiroRetry(ctx context.Context, attempt int) error {
	return kiroRetrySleep(ctx, kiroRetryBackoffDelay(attempt))
}

func resolveKiroUpstreamModel(mappedModel string) string {
	upstreamModel := kiropkg.MapModel(mappedModel)
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = mappedModel
	}
	return upstreamModel
}

func (s *GatewayService) forwardKiroMessages(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest, startTime time.Time) (*ForwardResult, error) {
	if account == nil || parsed == nil {
		return nil, fmt.Errorf("kiro forward: missing account or request")
	}

	originalModel := parsed.Model
	mappedModel := originalModel
	if next := account.GetMappedModel(originalModel); next != "" {
		mappedModel = next
	}
	body := parsed.Body.Bytes()
	if mappedModel != originalModel {
		body = s.replaceModelInBody(body, mappedModel)
	}
	logger.L().Debug("gateway forward_kiro_messages: request prepared",
		zap.Int64("account_id", account.ID),
		zap.String("auth_method", strings.TrimSpace(account.GetCredential("auth_method"))),
		zap.String("requested_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("has_profile_arn", strings.TrimSpace(account.GetCredential("profile_arn")) != ""),
	)

	if s.shouldEmulateWebSearch(ctx, account, parsed.GroupID, body) {
		parsedForEmulation, err := parsed.CloneForBody(body)
		if err != nil {
			return nil, err
		}
		parsedForEmulation.Model = mappedModel
		return s.handleWebSearchEmulation(ctx, c, account, parsedForEmulation)
	}

	if parsed.Stream {
		resp, _, err := s.openKiroAnthropicStreamResponse(ctx, account, parsed, body, mappedModel, originalModel, c.Request.Header, parsed.Group)
		if err != nil {
			var failoverErr *UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: failoverErr.StatusCode,
					Kind:               "failover",
					Message:            sanitizeUpstreamErrorMessage(err.Error()),
				})
				return nil, failoverErr
			}
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "api_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("kiro upstream request failed: %s", safeErr)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return nil, s.handleKiroHTTPError(ctx, resp, c, account, mappedModel, body)
		}
		upstreamModel := resolveKiroUpstreamModel(mappedModel)
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, mappedModel, false)
		if err != nil {
			return nil, err
		}
		if streamResult.usage == nil {
			streamResult.usage = &ClaudeUsage{}
		}
		requestID := buildKiroRequestID(resp)
		return &ForwardResult{
			RequestID:        requestID,
			Usage:            *streamResult.usage,
			Model:            originalModel,
			UpstreamModel:    upstreamModel,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     streamResult.firstTokenMs,
			ClientDisconnect: streamResult.clientDisconnect,
		}, nil
	}

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if tokenType != "oauth" {
		return nil, fmt.Errorf("kiro requires oauth token, got %s", tokenType)
	}
	if isOnlyWebSearchToolInBody(body) {
		webSearchResult, webSearchErr := s.executeKiroWebSearch(ctx, account, parsed.Group, body, mappedModel, originalModel, token, c.Request.Header)
		switch {
		case errors.Is(webSearchErr, errKiroWebSearchFallback):
		case webSearchErr == nil:
			upstreamModel := resolveKiroUpstreamModel(mappedModel)
			c.Header("Content-Type", "application/json")
			claudeReqID := kiropkg.NewClaudeRequestID()
			c.Header("x-request-id", claudeReqID)
			c.Header("request-id", claudeReqID)
			c.Data(http.StatusOK, "application/json", webSearchResult.ResponseBody)
			return &ForwardResult{
				RequestID:     webSearchResult.RequestID,
				Usage:         webSearchResult.Usage,
				Model:         originalModel,
				UpstreamModel: upstreamModel,
				Stream:        false,
				Duration:      time.Since(startTime),
			}, nil
		default:
			var httpErr *kiroWebSearchHTTPError
			if errors.As(webSearchErr, &httpErr) && httpErr.Response != nil {
				return nil, s.handleKiroHTTPError(ctx, httpErr.Response, c, account, mappedModel, body)
			}
			var failoverErr *UpstreamFailoverError
			if errors.As(webSearchErr, &failoverErr) {
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: failoverErr.StatusCode,
					Kind:               "failover",
					Message:            sanitizeUpstreamErrorMessage(webSearchErr.Error()),
				})
				return nil, failoverErr
			}
			safeErr := sanitizeUpstreamErrorMessage(webSearchErr.Error())
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "api_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("kiro upstream request failed: %s", safeErr)
		}
	}

	inputTokens := estimateKiroInputTokens(body)
	resp, requestCtx, err := s.executeKiroUpstreamWithParsed(ctx, account, parsed, body, mappedModel, originalModel, token, c.Request.Header)
	if err != nil {
		var failoverErr *UpstreamFailoverError
		if errors.As(err, &failoverErr) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: failoverErr.StatusCode,
				Kind:               "failover",
				Message:            sanitizeUpstreamErrorMessage(err.Error()),
			})
			return nil, failoverErr
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		c.JSON(http.StatusBadGateway, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": "Upstream request failed",
			},
		})
		return nil, fmt.Errorf("kiro upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, s.handleKiroHTTPError(ctx, resp, c, account, mappedModel, body)
	}

	cacheUsage := s.buildKiroCacheEmulationUsage(account, parsed.Group, body, mappedModel, inputTokens)
	requestCtx.CacheEmulationUsage = cacheUsage.toKiroUsage()
	parseResult, err := kiropkg.ParseNonStreamingEventStreamWithContext(resp.Body, originalModel, requestCtx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": "Failed to parse Kiro upstream response",
			},
		})
		return nil, err
	}

	c.Header("Content-Type", "application/json")
	requestID := buildKiroRequestID(resp)
	claudeReqID := kiropkg.NewClaudeRequestID()
	c.Header("x-request-id", claudeReqID)
	c.Header("request-id", claudeReqID)
	c.Data(http.StatusOK, "application/json", parseResult.ResponseBody)

	upstreamModel := resolveKiroUpstreamModel(mappedModel)

	return &ForwardResult{
		RequestID:     requestID,
		Usage:         kiroUsageToClaude(parseResult.Usage, inputTokens),
		Model:         originalModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *GatewayService) openKiroAnthropicStreamResponse(ctx context.Context, account *Account, parsed *ParsedRequest, anthropicBody []byte, mappedModel, requestModel string, headers http.Header, group *Group) (*http.Response, int, error) {
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, 0, err
	}
	if tokenType != "oauth" {
		return nil, 0, fmt.Errorf("kiro requires oauth token, got %s", tokenType)
	}

	inputTokens := estimateKiroInputTokens(anthropicBody)
	if isOnlyWebSearchToolInBody(anthropicBody) {
		cacheUsage := s.buildKiroCacheEmulationUsage(account, group, anthropicBody, mappedModel, inputTokens)
		pr, pw := io.Pipe()
		headers := make(http.Header)
		headers.Set("Content-Type", "text/event-stream")
		go func() {
			streamErr := s.streamKiroWebSearchAsAnthropic(ctx, account, anthropicBody, mappedModel, requestModel, token, inputTokens, headers, pw, cacheUsage)
			if streamErr != nil {
				_ = pw.CloseWithError(streamErr)
				return
			}
			_ = pw.Close()
		}()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       pr,
		}, inputTokens, nil
	}

	resp, requestCtx, err := s.executeKiroUpstreamWithParsed(ctx, account, parsed, anthropicBody, mappedModel, requestModel, token, headers)
	if err != nil {
		var failoverErr *UpstreamFailoverError
		if errors.As(err, &failoverErr) {
			return nil, inputTokens, err
		}
		return nil, inputTokens, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, inputTokens, nil
	}
	cacheUsage := s.buildKiroCacheEmulationUsage(account, group, anthropicBody, mappedModel, inputTokens)
	requestCtx.CacheEmulationUsage = cacheUsage.toKiroUsage()

	pr, pw := io.Pipe()
	wrappedHeaders := resp.Header.Clone()
	wrappedHeaders.Set("Content-Type", "text/event-stream")
	claudeReqID := kiropkg.NewClaudeRequestID()
	wrappedHeaders.Set("x-request-id", claudeReqID)
	wrappedHeaders.Set("request-id", claudeReqID)

	go func() {
		defer func() { _ = resp.Body.Close() }()
		_, streamErr := kiropkg.StreamEventStreamAsAnthropicWithContext(ctx, resp.Body, pw, requestModel, inputTokens, requestCtx)
		if streamErr != nil {
			_, _ = io.WriteString(pw, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"stream interrupted\"}}\n\n")
			_ = pw.CloseWithError(streamErr)
			return
		}
		_ = pw.Close()
	}()

	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     wrappedHeaders,
		Body:       pr,
	}, inputTokens, nil
}

func (s *GatewayService) executeKiroUpstream(ctx context.Context, account *Account, anthropicBody []byte, mappedModel, requestModel, token string, headers http.Header) (*http.Response, kiropkg.KiroRequestContext, error) {
	return s.executeKiroUpstreamWithParsed(ctx, account, nil, anthropicBody, mappedModel, requestModel, token, headers)
}

func (s *GatewayService) executeKiroUpstreamWithParsed(ctx context.Context, account *Account, parsed *ParsedRequest, anthropicBody []byte, mappedModel, requestModel, token string, headers http.Header) (*http.Response, kiropkg.KiroRequestContext, error) {
	var requestCtx kiropkg.KiroRequestContext
	if err := s.checkAndWaitKiroCooldown(ctx, buildKiroAccountKey(account)); err != nil {
		if failoverErr := asKiroCooldownFailoverError(err); failoverErr != nil {
			return nil, requestCtx, failoverErr
		}
		return nil, requestCtx, err
	}

	modelID := kiropkg.MapModel(mappedModel)
	currentToken := token
	buildResult, err := s.buildKiroPayloadForAccount(ctx, account, parsed, anthropicBody, modelID, currentToken, requestModel, headers)
	if err != nil {
		return nil, requestCtx, err
	}
	payload := buildResult.Payload
	requestCtx = buildResult.Context
	logKiroStatelessReplay(account, buildResult.Payload)

	endpoints := buildKiroEndpoints(account, kiroEndpointModeForRequest(parsed))
	proxyURL := kiroProxyURL(account)
	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)
	accountKey := buildKiroAccountKey(account)
	maxRetries := 2

	for idx, endpoint := range endpoints {
		for attempt := 0; attempt <= maxRetries; attempt++ {
			req, err := newKiroJSONRequest(ctx, endpoint.URL, payload, currentToken, accountKey, buildKiroMachineID(account), endpoint.AmzTarget, account)
			if err != nil {
				return nil, requestCtx, err
			}

			resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
			if err != nil {
				if attempt < maxRetries {
					if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
						return nil, requestCtx, sleepErr
					}
					continue
				}
				return nil, requestCtx, err
			}

			if resp.StatusCode == http.StatusTooManyRequests {
				dumpKiro429ResponseForDebug(resp, account.ID, endpoint.URL, endpoint.Name)

				cooldown, err := s.markKiro429(ctx, account.ID, accountKey)
				if err != nil {
					_ = resp.Body.Close()
					return nil, requestCtx, err
				}
				if idx+1 < len(endpoints) {
					_ = resp.Body.Close()
					if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
						return nil, requestCtx, sleepErr
					}
					break
				}
				resp.Header.Set("x-kiro-cooldown", cooldown.String())
				return resp, requestCtx, nil
			}

			if resp.StatusCode == http.StatusRequestTimeout || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
				if attempt < maxRetries {
					_ = resp.Body.Close()
					if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
						return nil, requestCtx, sleepErr
					}
					continue
				}
				if idx+1 < len(endpoints) {
					_ = resp.Body.Close()
					if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
						return nil, requestCtx, sleepErr
					}
					break
				}
				return resp, requestCtx, nil
			}

			if resp.StatusCode == http.StatusPaymentRequired {
				respBody, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					return nil, requestCtx, readErr
				}
				classification := classifyKiroHTTPError(resp.StatusCode, string(respBody))
				if classification.Category == kiroErrorMonthlyRequest {
					s.markKiroMonthlyRequestCountRateLimited(ctx, account, string(respBody))
				}
				return nil, requestCtx, &UpstreamFailoverError{
					StatusCode:      resp.StatusCode,
					ResponseBody:    respBody,
					ResponseHeaders: resp.Header.Clone(),
				}
			}

			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				respBody, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					return nil, requestCtx, readErr
				}

				if resp.StatusCode == http.StatusForbidden && isKiroSuspendedBody(respBody) {
					if _, err := s.markKiroSuspended(ctx, accountKey); err != nil {
						return nil, requestCtx, err
					}
					resetHTTPResponseBody(resp, respBody)
					return resp, requestCtx, nil
				}

				if s.kiroTokenProvider != nil && (resp.StatusCode == http.StatusUnauthorized || isKiroTokenErrorBody(respBody)) && attempt < maxRetries {
					refreshedToken, refreshErr := s.kiroTokenProvider.ForceRefreshAccessToken(ctx, account)
					if refreshErr == nil && strings.TrimSpace(refreshedToken) != "" {
						currentToken = refreshedToken
						accountKey = buildKiroAccountKey(account)
						buildResult, err = s.buildKiroPayloadForAccount(ctx, account, parsed, anthropicBody, modelID, currentToken, requestModel, headers)
						if err != nil {
							return nil, requestCtx, err
						}
						payload = buildResult.Payload
						requestCtx = buildResult.Context
						logKiroStatelessReplay(account, buildResult.Payload)
						if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
							return nil, requestCtx, sleepErr
						}
						continue
					}
					if refreshErr != nil && isNonRetryableRefreshError(refreshErr) {
						resetHTTPResponseBody(resp, respBody)
						return resp, requestCtx, nil
					}
				}

				if classifyKiroHTTPError(resp.StatusCode, string(respBody)).Category == kiroErrorAuthError {
					s.markKiroAuthTemporarilyUnavailable(ctx, account, resp.StatusCode, string(respBody))
				}

				resetHTTPResponseBody(resp, respBody)
				return resp, requestCtx, nil
			}

			if resp.StatusCode == http.StatusBadRequest {
				respBody, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					return nil, requestCtx, readErr
				}
				classification := classifyKiroHTTPError(resp.StatusCode, string(respBody))
				logKiroBadRequestClassification(classification, account, mappedModel, resp.Header, respBody)
				resetHTTPResponseBody(resp, respBody)
				return resp, requestCtx, nil
			}

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if err := s.markKiroSuccess(ctx, account.ID, accountKey); err != nil {
					_ = resp.Body.Close()
					return nil, requestCtx, err
				}
			}
			return resp, requestCtx, nil
		}
	}
	return nil, requestCtx, fmt.Errorf("kiro upstream endpoints exhausted")
}

// kiroKRSEndpointURL 是 Kiro 自家前置网关（KRS = Kiro Runtime Service）的固定 URL。
// KRS 仅支持 us-east-1 / eu-central-1 两个 region；这里固定走 us-east-1。
const kiroKRSEndpointURL = "https://runtime.us-east-1.kiro.dev/generateAssistantResponse"

func buildKiroEndpoints(account *Account, mode string) []kiroEndpointConfig {
	if mode == KiroEndpointModeKRS {
		return []kiroEndpointConfig{
			{
				URL:  kiroKRSEndpointURL,
				Name: "KiroRuntime",
			},
		}
	}
	region := kiroAPIRegion(account)
	return []kiroEndpointConfig{
		{
			URL:  fmt.Sprintf("https://q.%s.amazonaws.com/generateAssistantResponse", region),
			Name: "AmazonQ",
		},
	}
}

// kiroEndpointModeForRequest 从 ParsedRequest 取 group 配置的 Kiro endpoint 模式；
// parsed/Group 为 nil 时安全兜底为 "q"。
func kiroEndpointModeForRequest(parsed *ParsedRequest) string {
	if parsed == nil || parsed.Group == nil {
		return KiroEndpointModeQ
	}
	return parsed.Group.EffectiveKiroEndpointMode()
}

func (s *GatewayService) buildKiroPayloadForAccount(ctx context.Context, account *Account, parsed *ParsedRequest, anthropicBody []byte, modelID, token, requestModel string, headers http.Header) (*kiropkg.KiroBuildResult, error) {
	_ = s
	_ = ctx
	_ = token
	profileArn := resolveKiroPayloadProfileArn(account)
	anthropicBody = prepareKiroPayloadBodyForRequestModel(anthropicBody, requestModel)
	buildResult, err := kiropkg.BuildKiroPayloadWithContext(anthropicBody, modelID, profileArn, "AI_EDITOR", headers)
	if err != nil {
		return nil, err
	}
	if stableID := stableKiroConversationID(account, parsed, anthropicBody, modelID, profileArn); stableID != "" {
		if next, setErr := sjson.SetBytes(buildResult.Payload, "conversationState.conversationId", stableID); setErr == nil {
			buildResult.Payload = next
		}
	}
	return buildResult, nil
}

func stableKiroConversationID(account *Account, parsed *ParsedRequest, anthropicBody []byte, modelID, profileArn string) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SUB2API_KIRO_CONVERSATION_ID_MODE"))) {
	case "random", "uuid", "off", "false", "0":
		return ""
	}
	seed := stableKiroConversationSeed(account, parsed, anthropicBody, modelID, profileArn)
	if seed == "" {
		return ""
	}
	return generateSessionUUID(seed)
}

func stableKiroConversationSeed(account *Account, parsed *ParsedRequest, anthropicBody []byte, modelID, profileArn string) string {
	var anchorType, anchor string
	if parsed != nil {
		if explicitID := strings.TrimSpace(parsed.ExplicitSessionID); explicitID != "" {
			anchorType, anchor = "explicit", explicitID
		} else if metadataUserID := strings.TrimSpace(parsed.MetadataUserID); metadataUserID != "" {
			anchorType, anchor = "metadata", metadataUserID
		} else if systemText := extractTextFromSystemRaw(parsed.SystemRaw()); systemText != "" {
			anchorType, anchor = "system", systemText
		}
	}
	if anchor == "" && len(anthropicBody) > 0 {
		if systemText := extractTextFromSystemRaw([]byte(gjson.GetBytes(anthropicBody, "system").Raw)); systemText != "" {
			anchorType, anchor = "system", systemText
		} else if firstUserText := extractFirstUserText(anthropicBody); firstUserText != "" {
			anchorType, anchor = "first_user", firstUserText
		}
	}
	if anchor == "" {
		return ""
	}

	var sb strings.Builder
	_, _ = sb.WriteString("kiro-conversation-v1|")
	if account != nil {
		_, _ = sb.WriteString("account:")
		_, _ = sb.WriteString(strconv.FormatInt(account.ID, 10))
		_, _ = sb.WriteString("|credential:")
		_, _ = sb.WriteString(kiroCacheCredentialIdentity(account))
		_, _ = sb.WriteString("|")
	}
	if parsed != nil && parsed.SessionContext != nil {
		_, _ = sb.WriteString("api_key:")
		_, _ = sb.WriteString(strconv.FormatInt(parsed.SessionContext.APIKeyID, 10))
		_, _ = sb.WriteString("|")
	}
	_, _ = sb.WriteString("model:")
	_, _ = sb.WriteString(strings.TrimSpace(modelID))
	_, _ = sb.WriteString("|profile:")
	_, _ = sb.WriteString(strings.TrimSpace(profileArn))
	_, _ = sb.WriteString("|anchor:")
	_, _ = sb.WriteString(anchorType)
	_, _ = sb.WriteString(":")
	_, _ = sb.WriteString(anchor)
	return sb.String()
}

func logKiroStatelessReplay(account *Account, payload []byte) {
	if account == nil {
		return
	}
	conversationID := gjson.GetBytes(payload, "conversationState.conversationId").String()
	systemPrompt := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	currentContent := gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String()
	logger.L().Info("kiro.stateless_replay",
		zap.Int64("selected_account_id", account.ID),
		zap.Bool("stateless_replay", true),
		zap.Int("history_count", len(gjson.GetBytes(payload, "conversationState.history").Array())),
		zap.Bool("has_agent_continuation_id", gjson.GetBytes(payload, "conversationState.agentContinuationId").Exists()),
		zap.String("conversation_id_hash", hashKiroLogString(conversationID)),
		zap.String("payload_hash_no_conversation_id", hashKiroPayloadWithoutConversationID(payload)),
		zap.String("system_prompt_hash", hashKiroLogString(systemPrompt)),
		zap.Int("system_prompt_len", len(systemPrompt)),
		zap.String("current_content_hash", hashKiroLogString(currentContent)),
		zap.Int("tool_count", len(gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools").Array())),
	)
}

func hashKiroPayloadWithoutConversationID(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	normalized := payload
	if next, err := sjson.DeleteBytes(payload, "conversationState.conversationId"); err == nil {
		normalized = next
	}
	return strconv.FormatUint(xxhash.Sum64(normalized), 36)
}

func hashKiroLogString(value string) string {
	if value == "" {
		return ""
	}
	return strconv.FormatUint(xxhash.Sum64String(value), 36)
}

func prepareKiroPayloadBodyForRequestModel(anthropicBody []byte, requestModel string) []byte {
	requestModel = strings.TrimSpace(requestModel)
	if requestModel == "" || !strings.Contains(strings.ToLower(requestModel), "thinking") {
		return anthropicBody
	}
	bodyModel := strings.TrimSpace(gjson.GetBytes(anthropicBody, "model").String())
	if bodyModel == "" || strings.EqualFold(bodyModel, requestModel) || strings.Contains(strings.ToLower(bodyModel), "thinking") {
		return anthropicBody
	}
	if next, ok := setJSONValueBytes(anthropicBody, "model", requestModel); ok {
		return next
	}
	return anthropicBody
}

func (s *GatewayService) markKiroAuthTemporarilyUnavailable(ctx context.Context, account *Account, statusCode int, body string) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}
	until := time.Now().Add(10 * time.Minute)
	reason := fmt.Sprintf("kiro auth failure (%d): %s", statusCode, strings.TrimSpace(body))
	_ = s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason)
}

func (s *GatewayService) markKiroMonthlyRequestCountRateLimited(ctx context.Context, account *Account, body string) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}
	resetAt := nextKiroMonthlyResetUTC(time.Now())
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		logger.L().Warn("kiro monthly request count rate-limit failed",
			zap.Int64("account_id", account.ID),
			zap.Time("reset_at", resetAt),
			zap.Error(err),
		)
		return
	}
	reason := "kiro monthly request count exhausted (402): MONTHLY_REQUEST_COUNT"
	if trimmed := strings.TrimSpace(body); trimmed != "" {
		reason = fmt.Sprintf("%s body=%s", reason, truncateForLog([]byte(trimmed), 512))
	}
	logger.L().Warn("kiro monthly request count rate-limited",
		zap.Int64("account_id", account.ID),
		zap.Time("reset_at", resetAt),
		zap.String("reason", reason),
	)
}

func nextKiroMonthlyResetUTC(now time.Time) time.Time {
	utc := now.UTC()
	year, month, _ := utc.Date()
	return time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
}

func resetHTTPResponseBody(resp *http.Response, body []byte) {
	if resp == nil {
		return
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
}

func estimateKiroInputTokens(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		return countKiroInputTokensFromPayload(payload)
	}
	tokens := len(body) / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func kiroUsageToClaude(usage kiropkg.Usage, fallbackInput int) ClaudeUsage {
	inputTokens := usage.InputTokens
	if inputTokens == 0 {
		inputTokens = fallbackInput
	}
	return ClaudeUsage{
		InputTokens:              inputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheCreation5mTokens:    usage.CacheCreation5mInputTokens,
		CacheCreation1hTokens:    usage.CacheCreation1hInputTokens,
		KiroCredits:              usage.KiroCredits,
	}
}

func (s *GatewayService) markKiroInvalidModelRateLimited(ctx context.Context, account *Account, mappedModel string) {
	if s == nil || s.accountRepo == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	resetAt := time.Now().Add(kiroInvalidModelTempUnschedDuration)
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		logger.L().Warn("kiro invalid model rate-limit failed",
			zap.Int64("account_id", account.ID),
			zap.String("mapped_model", strings.TrimSpace(mappedModel)),
			zap.Time("reset_at", resetAt),
			zap.Error(err),
		)
		return
	}
	logger.L().Warn("kiro invalid model rate-limited",
		zap.Int64("account_id", account.ID),
		zap.String("mapped_model", strings.TrimSpace(mappedModel)),
		zap.Time("reset_at", resetAt),
	)
}

func (s *GatewayService) handleKiroHTTPError(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, mappedModel string, requestBody []byte) error {
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if upstreamMsg == "" {
		upstreamMsg = strings.TrimSpace(string(respBody))
	}
	classification := classifyKiroHTTPError(resp.StatusCode, string(respBody))
	if resp.StatusCode == http.StatusBadRequest {
		logKiroBadRequestClassification(classification, account, "", resp.Header, respBody)
	}
	if classification.Category == kiroErrorMonthlyRequest {
		s.markKiroMonthlyRequestCountRateLimited(ctx, account, string(respBody))
	}
	if classification.Category == kiroErrorBadRequestInvalidModel && account != nil && account.Type == AccountTypeOAuth {
		s.markKiroInvalidModelRateLimited(ctx, account, mappedModel)
		event := s.buildKiroInvalidModelUpstreamEvent(account, resp, upstreamMsg, mappedModel, requestBody, c)
		appendOpsUpstreamError(c, event)
		return &UpstreamFailoverError{
			StatusCode:      resp.StatusCode,
			ResponseBody:    respBody,
			ResponseHeaders: resp.Header.Clone(),
		}
	}

	if resp.StatusCode == http.StatusPaymentRequired || s.shouldFailoverUpstreamError(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  buildKiroRequestID(resp),
			Kind:               "failover",
			Message:            upstreamMsg,
		})
		// 429 已经被 executeKiroUpstreamWithParsed → markKiro429 完整处理（Redis 1-5min
		// 指数退避 + DB rate_limit_reset_at 同步）。这里再走 HandleUpstreamError 会进入
		// handle429 → apply429FallbackRateLimit，把 DB cooldown 反写成 5s flat，
		// 直接抹掉我们刚算好的退避时长。所以 429 跳过通用 handler。
		if s.rateLimitService != nil && resp.StatusCode != http.StatusTooManyRequests {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		return &UpstreamFailoverError{
			StatusCode:      resp.StatusCode,
			ResponseBody:    respBody,
			ResponseHeaders: resp.Header.Clone(),
		}
	}

	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  buildKiroRequestID(resp),
		Kind:               "http_error",
		Message:            upstreamMsg,
	})
	c.JSON(mapUpstreamStatusCode(resp.StatusCode), gin.H{
		"type": "error",
		"error": gin.H{
			"type":    claudeErrorType(resp.StatusCode),
			"message": coalesceKiroErrorMessage(resp.StatusCode, upstreamMsg),
		},
	})
	return fmt.Errorf("kiro upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func claudeErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable:
		return "overloaded_error"
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	default:
		return "api_error"
	}
}

func (s *GatewayService) buildKiroInvalidModelUpstreamEvent(account *Account, resp *http.Response, upstreamMsg, mappedModel string, requestBody []byte, c *gin.Context) OpsUpstreamErrorEvent {
	_ = s
	requestedModel := strings.TrimSpace(gjson.GetBytes(requestBody, "model").String())
	hasTools := gjson.GetBytes(requestBody, "tools").Exists()
	hasAdaptiveThinking := strings.EqualFold(strings.TrimSpace(gjson.GetBytes(requestBody, "thinking.type").String()), "adaptive")
	hasContext1MBeta := false
	if c != nil {
		hasContext1MBeta = strings.Contains(c.GetHeader("Anthropic-Beta"), "context-1m")
	}
	return OpsUpstreamErrorEvent{
		Platform:            account.Platform,
		AccountID:           account.ID,
		AccountName:         account.Name,
		UpstreamStatusCode:  resp.StatusCode,
		UpstreamRequestID:   buildKiroRequestID(resp),
		Kind:                "failover",
		Message:             upstreamMsg,
		RequestedModel:      requestedModel,
		MappedModel:         strings.TrimSpace(mappedModel),
		KiroModelID:         kiropkg.MapModel(mappedModel),
		HasTools:            hasTools,
		HasAdaptiveThinking: hasAdaptiveThinking,
		HasContext1MBeta:    hasContext1MBeta,
	}
}

func logKiroBadRequestClassification(classification kiroErrorClassification, account *Account, model string, headers http.Header, body []byte) {
	if classification.StatusCode != http.StatusBadRequest {
		return
	}
	var accountID int64
	if account != nil {
		accountID = account.ID
	}
	logger.L().Warn("kiro upstream bad request classified",
		zap.String("category", classification.Category),
		zap.Int("status", classification.StatusCode),
		zap.Int64("account_id", accountID),
		zap.String("model", strings.TrimSpace(model)),
		zap.String("request_id", headers.Get("x-request-id")),
		zap.String("body_excerpt", truncateForLog(body, 512)),
	)
}

// dumpKiro429ResponseForDebug captures the first 2KB of a Kiro 429 response body
// and the rate-limit-relevant headers, then restores resp.Body so the caller can
// still consume it. Used to investigate whether Kiro returns a reset-time field
// (e.g. nextDateReset) we should parse instead of falling back to fixed cooldown.
func dumpKiro429ResponseForDebug(resp *http.Response, accountID int64, endpointURL, endpointName string) {
	if resp == nil || resp.Body == nil {
		return
	}
	const maxBytes = 2048
	limited := io.LimitReader(resp.Body, maxBytes+1)
	sample, err := io.ReadAll(limited)
	if err != nil {
		logger.L().Warn("kiro.429_debug_read_failed",
			zap.Int64("account_id", accountID),
			zap.String("endpoint", endpointName),
			zap.Error(err),
		)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return
	}
	truncated := false
	if len(sample) > maxBytes {
		sample = sample[:maxBytes]
		truncated = true
	}
	rest, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(append(append([]byte{}, sample...), rest...)))

	headers := map[string]string{}
	for k, v := range resp.Header {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "ratelimit") || strings.Contains(lk, "retry") || strings.Contains(lk, "reset") ||
			lk == "content-type" || lk == "x-amzn-requestid" || lk == "x-amzn-errortype" {
			headers[k] = strings.Join(v, ",")
		}
	}

	logger.L().Warn("kiro.429_raw_response",
		zap.Int64("account_id", accountID),
		zap.String("endpoint_url", endpointURL),
		zap.String("endpoint_name", endpointName),
		zap.String("content_type", resp.Header.Get("Content-Type")),
		zap.Any("relevant_headers", headers),
		zap.Int("body_bytes", len(sample)),
		zap.Bool("truncated", truncated),
		zap.String("body_sample", string(sample)),
	)
}

func coalesceKiroErrorMessage(statusCode int, upstreamMsg string) string {
	if upstreamMsg != "" {
		return upstreamMsg
	}
	switch statusCode {
	case http.StatusTooManyRequests:
		return "Rate limit exceeded"
	case http.StatusForbidden:
		return "Access denied"
	case http.StatusUnauthorized:
		return "Authentication failed"
	default:
		return "Upstream request failed"
	}
}
