package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

type antigravityCompatProtocol uint8

const (
	antigravityCompatChatCompletions antigravityCompatProtocol = iota
	antigravityCompatResponses
)

const (
	// AntigravityCredentialRejectedClientMessage 是可安全返回给客户端的认证修复提示。
	AntigravityCredentialRejectedClientMessage = "Antigravity rejected the OAuth credential after refresh; reauthorize the account and verify project_id"
	// AntigravityCredentialRejectedReason 标识上游拒绝已刷新 OAuth 凭据。
	AntigravityCredentialRejectedReason GatewayFailureReason = "antigravity_oauth_credential_rejected"
)

const antigravityCompatMaxTokens = 64000

type antigravityCompatRequest struct {
	protocol        antigravityCompatProtocol
	originalBody    []byte
	claudeBody      []byte
	originalModel   string
	clientStream    bool
	includeUsage    bool
	startTime       time.Time
	reasoningEffort *string
}

type antigravityCompatUpstreamCall struct {
	request      antigravityCompatRequest
	billingModel string
	prefix       string
	proxyURL     string
	accessToken  string
	geminiBody   []byte
}

// ForwardAsChatCompletions 使用 Antigravity 原生 OAuth 账号转发 Chat Completions 请求。
func (s *AntigravityGatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	_ *ParsedRequest,
) (*ForwardResult, error) {
	if err := s.validateAntigravityCompatAccount(c, account); err != nil {
		return nil, err
	}

	var request apicompat.ChatCompletionsRequest
	if json.Unmarshal(body, &request) != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}

	responsesRequest, err := apicompat.ChatCompletionsToResponses(&request)
	if err != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	claudeRequest, err := apicompat.ResponsesToAnthropicRequest(responsesRequest)
	if err != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	preserveChatCompletionTokenLimit(&request, claudeRequest)
	claudeRequest.Stream = request.Stream
	claudeBody, err := json.Marshal(claudeRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	return s.forwardAntigravityCompat(ctx, c, account, antigravityCompatRequest{
		protocol:        antigravityCompatChatCompletions,
		originalBody:    body,
		claudeBody:      claudeBody,
		originalModel:   request.Model,
		clientStream:    request.Stream,
		includeUsage:    request.StreamOptions != nil && request.StreamOptions.IncludeUsage,
		startTime:       time.Now(),
		reasoningEffort: extractCCReasoningEffortFromBody(body),
	})
}

// ForwardAsResponses 使用 Antigravity 原生 OAuth 账号转发 Responses 请求。
func (s *AntigravityGatewayService) ForwardAsResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	_ *ParsedRequest,
) (*ForwardResult, error) {
	if err := s.validateAntigravityCompatAccount(c, account); err != nil {
		return nil, err
	}

	var request apicompat.ResponsesRequest
	if json.Unmarshal(body, &request) != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}

	claudeRequest, err := apicompat.ResponsesToAnthropicRequest(&request)
	if err != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	claudeRequest.Stream = request.Stream
	claudeBody, err := json.Marshal(claudeRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	return s.forwardAntigravityCompat(ctx, c, account, antigravityCompatRequest{
		protocol:        antigravityCompatResponses,
		originalBody:    body,
		claudeBody:      claudeBody,
		originalModel:   request.Model,
		clientStream:    request.Stream,
		startTime:       time.Now(),
		reasoningEffort: ExtractResponsesReasoningEffortFromBody(body),
	})
}

func (s *AntigravityGatewayService) validateAntigravityCompatAccount(c *gin.Context, account *Account) error {
	if account != nil && account.Platform == PlatformAntigravity && account.Type == AccountTypeOAuth {
		return nil
	}
	return s.writeAntigravityCompatError(
		c,
		http.StatusBadRequest,
		"invalid_request_error",
		"native OAuth account required for antigravity compatibility mode",
	)
}

func preserveChatCompletionTokenLimit(request *apicompat.ChatCompletionsRequest, claudeRequest *apicompat.AnthropicRequest) {
	if request == nil || claudeRequest == nil {
		return
	}
	limit := request.MaxTokens
	if request.MaxCompletionTokens != nil {
		limit = request.MaxCompletionTokens
	}
	if limit != nil && *limit > 0 {
		claudeRequest.MaxTokens = min(*limit, antigravityCompatMaxTokens)
	}
}

func (s *AntigravityGatewayService) forwardAntigravityCompat(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	request antigravityCompatRequest,
) (*ForwardResult, error) {
	beginUpstreamResponseModelObservation(c)
	call, err := s.prepareAntigravityCompatCall(ctx, c, account, request)
	if err != nil {
		return nil, err
	}

	result, err := s.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             ctx,
		prefix:          call.prefix,
		account:         account,
		proxyURL:        call.proxyURL,
		accessToken:     call.accessToken,
		action:          "streamGenerateContent",
		body:            call.geminiBody,
		c:               c,
		httpUpstream:    s.httpUpstream,
		settingService:  s.settingService,
		accountRepo:     s.accountRepo,
		handleError:     s.handleUpstreamError,
		requestedModel:  request.originalModel,
		isStickySession: false,
		groupID:         0,
		sessionHash:     "",
	})
	if err != nil {
		return nil, s.handleAntigravityCompatTransportError(c, err)
	}

	return s.consumeAntigravityCompatResponse(ctx, c, account, call, result.resp)
}

func (s *AntigravityGatewayService) prepareAntigravityCompatCall(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	request antigravityCompatRequest,
) (*antigravityCompatUpstreamCall, error) {
	var claudeRequest antigravity.ClaudeRequest
	if json.Unmarshal(request.claudeBody, &claudeRequest) != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
	}

	mappedModel := s.getMappedModel(account, request.originalModel)
	if mappedModel == "" {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		message := fmt.Sprintf("model %s not in whitelist", request.originalModel)
		return nil, s.writeAntigravityCompatError(c, http.StatusForbidden, "permission_error", message)
	}
	thinkingEnabled := claudeRequest.Thinking != nil &&
		(claudeRequest.Thinking.Type == "enabled" || claudeRequest.Thinking.Type == "adaptive")
	mappedModel = applyThinkingModelSuffix(mappedModel, thinkingEnabled)

	if s.tokenProvider == nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadGateway, "api_error", "Antigravity token provider not configured")
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
		_ = s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	geminiBody, err := s.buildAntigravityCompatGeminiBody(ctx, request.claudeBody, &claudeRequest, projectID, mappedModel)
	if err != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request")
	}

	request.reasoningEffort = ApplyThinkingEnabledFallback(request.reasoningEffort, request.originalBody, mappedModel)
	return &antigravityCompatUpstreamCall{
		request:      request,
		billingModel: mappedModel,
		prefix:       logPrefix(getSessionID(c), account.Name),
		proxyURL:     antigravityCompatProxyURL(account),
		accessToken:  accessToken,
		geminiBody:   geminiBody,
	}, nil
}

func (s *AntigravityGatewayService) buildAntigravityCompatGeminiBody(
	ctx context.Context,
	claudeBody []byte,
	claudeRequest *antigravity.ClaudeRequest,
	projectID string,
	mappedModel string,
) ([]byte, error) {
	if strings.HasPrefix(strings.ToLower(mappedModel), "gemini-") {
		body, err := convertClaudeMessagesToGeminiGenerateContent(claudeBody)
		if err != nil {
			return nil, err
		}
		body, err = enableMixedGeminiToolInvocations(body)
		if err != nil {
			return nil, err
		}
		body = ensureGeminiFunctionCallThoughtSignatures(body)
		body, err = injectIdentityPatchToGeminiRequest(body)
		if err != nil {
			return nil, err
		}
		if cleaned, cleanErr := cleanGeminiRequest(body); cleanErr == nil {
			body = cleaned
		}
		return s.wrapV1InternalRequest(projectID, mappedModel, body)
	}

	options := s.getClaudeTransformOptions(ctx)
	options.EnableIdentityPatch = true
	return antigravity.TransformClaudeToGeminiWithOptions(claudeRequest, projectID, mappedModel, options)
}

func enableMixedGeminiToolInvocations(body []byte) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}

	var hasGoogleSearch, hasFunctionDeclarations bool
	if tools, ok := request["tools"].([]any); ok {
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				continue
			}
			_, hasSearch := tool["googleSearch"]
			declarations, hasFunctions := tool["functionDeclarations"].([]any)
			hasGoogleSearch = hasGoogleSearch || hasSearch
			hasFunctionDeclarations = hasFunctionDeclarations || hasFunctions && len(declarations) > 0
		}
	}
	if !hasGoogleSearch || !hasFunctionDeclarations {
		return body, nil
	}

	toolConfig, _ := request["toolConfig"].(map[string]any)
	if toolConfig == nil {
		toolConfig = make(map[string]any)
		request["toolConfig"] = toolConfig
	}
	toolConfig["includeServerSideToolInvocations"] = true
	return json.Marshal(request)
}

func antigravityCompatProxyURL(account *Account) string {
	if account.ProxyID == nil || account.Proxy == nil {
		return ""
	}
	return account.Proxy.URL()
}

func (s *AntigravityGatewayService) handleAntigravityCompatTransportError(c *gin.Context, err error) error {
	if switchErr, ok := IsAntigravityAccountSwitchError(err); ok {
		return &UpstreamFailoverError{
			StatusCode:        http.StatusServiceUnavailable,
			ForceCacheBilling: switchErr.IsStickySession,
		}
	}
	if c.Request.Context().Err() != nil {
		return s.writeAntigravityCompatError(c, http.StatusBadGateway, "client_disconnected", "Client disconnected before upstream response")
	}
	return s.writeAntigravityCompatError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
}

func (s *AntigravityGatewayService) consumeAntigravityCompatResponse(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	call *antigravityCompatUpstreamCall,
	resp *http.Response,
) (*ForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, s.handleAntigravityCompatHTTPError(ctx, c, account, call, resp)
	}

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}
	streamResult, err := s.consumeAntigravityCompatSuccess(c, call, resp)
	if err != nil {
		return nil, err
	}
	if streamResult.usage == nil {
		streamResult.usage = &ClaudeUsage{}
	}

	return &ForwardResult{
		RequestID:                     requestID,
		Usage:                         *streamResult.usage,
		Model:                         call.request.originalModel,
		UpstreamModel:                 call.billingModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		Stream:                        call.request.clientStream,
		Duration:                      time.Since(call.request.startTime),
		FirstTokenMs:                  streamResult.firstTokenMs,
		ReasoningEffort:               call.request.reasoningEffort,
		ClientDisconnect:              streamResult.clientDisconnect,
	}, nil
}

func (s *AntigravityGatewayService) consumeAntigravityCompatSuccess(
	c *gin.Context,
	call *antigravityCompatUpstreamCall,
	resp *http.Response,
) (*antigravityStreamResult, error) {
	if call.request.clientStream {
		if call.request.protocol == antigravityCompatChatCompletions {
			return s.handleChatCompletionsStreamingFromAntigravity(
				c,
				resp,
				call.request.startTime,
				call.request.originalModel,
				call.request.includeUsage,
			)
		}
		return s.handleResponsesStreamingFromAntigravity(c, resp, call.request.startTime, call.request.originalModel)
	}

	if call.request.protocol == antigravityCompatChatCompletions {
		return s.handleChatCompletionsNonStreamingFromAntigravity(c, resp, call.request.startTime, call.request.originalModel)
	}
	return s.handleResponsesNonStreamingFromAntigravity(c, resp, call.request.startTime, call.request.originalModel)
}

func (s *AntigravityGatewayService) handleAntigravityCompatHTTPError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	call *antigravityCompatUpstreamCall,
	resp *http.Response,
) error {
	body := s.readUpstreamErrorBody(resp)
	s.handleUpstreamError(
		ctx,
		call.prefix,
		account,
		resp.StatusCode,
		resp.Header,
		body,
		call.request.originalModel,
		0,
		"",
		false,
	)
	if s.shouldFailoverUpstreamError(resp.StatusCode) {
		message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(body)))
		event := OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "failover",
			Message:            message,
			Detail:             s.getUpstreamErrorDetail(body),
		}
		if resp.StatusCode == http.StatusUnauthorized {
			event.Stage = string(GatewayFailureStageAccountAuth)
			event.Scope = string(GatewayFailureScopeAccount)
			event.Reason = string(AntigravityCredentialRejectedReason)
			appendOpsUpstreamError(c, event)
			return antigravityCredentialRejectedError(resp, body)
		}
		appendOpsUpstreamError(c, event)
		return &UpstreamFailoverError{
			StatusCode:      resp.StatusCode,
			ResponseBody:    body,
			ResponseHeaders: resp.Header.Clone(),
		}
	}
	return s.writeMappedAntigravityCompatError(c, account, resp.StatusCode, resp.Header.Get("x-request-id"), body)
}

func antigravityCredentialRejectedError(resp *http.Response, body []byte) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:        resp.StatusCode,
		ResponseBody:      body,
		ResponseHeaders:   resp.Header.Clone(),
		Stage:             GatewayFailureStageAccountAuth,
		Scope:             GatewayFailureScopeAccount,
		Reason:            AntigravityCredentialRejectedReason,
		NextAccountAction: NextAccountRetry,
		ClientStatusCode:  http.StatusBadGateway,
		ClientMessage:     AntigravityCredentialRejectedClientMessage,
	}
}

func (s *AntigravityGatewayService) writeAntigravityCompatError(
	c *gin.Context,
	status int,
	errType string,
	message string,
) error {
	MarkResponseCommitted(c)
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"param":   nil,
			"code":    nil,
		},
	})
	return errors.New(message)
}

func (s *AntigravityGatewayService) writeMappedAntigravityCompatError(
	c *gin.Context,
	account *Account,
	upstreamStatus int,
	upstreamRequestID string,
	body []byte,
) error {
	MarkResponseCommitted(c)
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(body)))
	setOpsUpstreamError(c, upstreamStatus, message, s.getUpstreamErrorDetail(body))
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:            opsUpstreamProxyID(account),
		ProxyName:          opsUpstreamProxyName(account),
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: upstreamStatus,
		UpstreamRequestID:  upstreamRequestID,
		Kind:               "http_error",
		Message:            message,
	})
	c.JSON(mapUpstreamStatusCode(upstreamStatus), gin.H{
		"error": gin.H{
			"message": getPassthroughOrDefault(message, "Upstream request failed"),
			"type":    "upstream_error",
			"param":   nil,
			"code":    nil,
		},
	})
	return fmt.Errorf("upstream error: %d %s", upstreamStatus, message)
}

func (s *AntigravityGatewayService) handleChatCompletionsNonStreamingFromAntigravity(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
) (*antigravityStreamResult, error) {
	claudeResponse, result, err := s.collectClaudeStreamResponse(c, resp, startTime, originalModel)
	if err != nil {
		return nil, s.mapAntigravityCompatCollectionError(c, err)
	}
	var anthropicResponse apicompat.AnthropicResponse
	if json.Unmarshal(claudeResponse, &anthropicResponse) != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}
	responsesResponse := apicompat.AnthropicToResponsesResponse(&anthropicResponse)
	c.JSON(http.StatusOK, apicompat.ResponsesToChatCompletions(responsesResponse, originalModel))
	return result, nil
}

func (s *AntigravityGatewayService) handleResponsesNonStreamingFromAntigravity(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
) (*antigravityStreamResult, error) {
	claudeResponse, result, err := s.collectClaudeStreamResponse(c, resp, startTime, originalModel)
	if err != nil {
		return nil, s.mapAntigravityCompatCollectionError(c, err)
	}
	var anthropicResponse apicompat.AnthropicResponse
	if json.Unmarshal(claudeResponse, &anthropicResponse) != nil {
		return nil, s.writeAntigravityCompatError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}
	c.JSON(http.StatusOK, apicompat.AnthropicToResponsesResponse(&anthropicResponse))
	return result, nil
}

func (s *AntigravityGatewayService) mapAntigravityCompatCollectionError(c *gin.Context, err error) error {
	var failoverError *UpstreamFailoverError
	if errors.As(err, &failoverError) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if strings.Contains(err.Error(), "stream data interval timeout") {
		return s.writeAntigravityCompatError(c, http.StatusBadGateway, "upstream_timeout", "Upstream stream data interval timeout")
	}
	if errors.Is(err, bufio.ErrTooLong) {
		return s.writeAntigravityCompatError(c, http.StatusBadGateway, "response_too_large", "Upstream response line too long")
	}
	return s.writeAntigravityCompatError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
}
