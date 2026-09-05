package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

var grokPassthroughForwardHeaders = []string{
	"x-grok-req-id",
	"x-grok-session-id",
	"x-grok-agent-id",
	"x-grok-turn-idx",
	"x-grok-client-mode",
	"x-grok-client-surface",
	"x-grok-doom-loop-check",
	"x-grok-model-override",
	"x-compactions-remaining",
	"x-compaction-at",
}

func (s *OpenAIGatewayService) forwardGrokPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	upstreamModel := strings.TrimSpace(originalModel)
	if isGrokImageGenerationModel(upstreamModel) {
		return nil, fmt.Errorf("model %s is an image model and is not available on the Responses endpoint; use /v1/images/generations instead", upstreamModel)
	}

	cacheIdentity := resolveGrokCacheIdentity(c, body, "", upstreamModel)
	forwardBody, err := applyGrokPassthroughCacheIdentity(body, cacheIdentity)
	if err != nil {
		return nil, fmt.Errorf("apply grok prompt cache identity: %w", err)
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		upstreamReq, buildErr := buildGrokPassthroughRequest(upstreamCtx, c, account, forwardBody, token, cacheIdentity, s.cfg, s.settingService)
		if buildErr != nil {
			return nil, buildErr
		}

		resp, err = s.doOpenAIUpstream(upstreamReq, proxyURL, account)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if err != nil {
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
		}

		if attempt > 0 || (resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity) {
			break
		}
		respBody := s.readUpstreamErrorBody(resp)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		invalidEncryptedContent := isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody)
		if !invalidEncryptedContent && !isGrokCompactionReplayDecodeError(resp.StatusCode, respBody) {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}

		var retryBody []byte
		var changed bool
		var trimErr error
		if invalidEncryptedContent {
			retryBody, changed, trimErr = trimGrokInvalidEncryptedContentRetryBody(forwardBody)
		} else {
			retryBody, changed, trimErr = sanitizeGrokCompactionReplayBody(forwardBody)
		}
		if trimErr != nil {
			return nil, fmt.Errorf("prepare Grok replay decode retry: %w", trimErr)
		}
		if !changed {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}

		forwardBody = retryBody
		slog.Info("grok_replay_decode_retry", "account_id", account.ID, "cache_identity_present", cacheIdentity != "", "passthrough", true)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
		}
		kind := "http_error"
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			kind = "failover"
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               kind,
			Message:            upstreamMsg,
		})
		errCtx := withGrokTeamRateLimitModel(ctx, upstreamModel)
		s.handleGrokAccountUpstreamError(errCtx, account, resp.StatusCode, resp.Header, respBody)
		if shouldMarkGrokTeamModelRateLimit(resp.StatusCode, respBody) {
			markGrokTeamModelRateLimit(account, upstreamModel, resolveGrokTeamRateLimitUntil(time.Now().Add(grokTeamRateLimitDefaultTTL), time.Now()))
		}
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			retryable, retryDelay, retryDeadline, retryMax := grokSameAccountRetryMetadata(account, resp.StatusCode, respBody)
			return nil, &UpstreamFailoverError{
				StatusCode:               resp.StatusCode,
				ResponseBody:             respBody,
				ResponseHeaders:          resp.Header.Clone(),
				RetryableOnSameAccount:   retryable,
				RequestScopedTransient:   retryable && resp.StatusCode == http.StatusTooManyRequests,
				SameAccountRetryDelay:    retryDelay,
				SameAccountRetryDeadline: retryDeadline,
				SameAccountRetryMax:      retryMax,
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, forwardBody, upstreamModel)
	}

	stateCtx := withGrokTeamRateLimitModel(ctx, upstreamModel)
	s.updateGrokUsageFromResponse(stateCtx, account, resp.Header, resp.StatusCode)

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	searchCount := 0
	imageCount := 0
	var imageOutputSizes []string
	if reqStream {
		maxLineSize := defaultMaxLineSize
		if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
			maxLineSize = s.cfg.Gateway.MaxLineSize
		}
		resp.Body = newGrokResponsesBillingPingFilterBody(resp.Body, account, maxLineSize)
		streamResult, streamErr := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, upstreamModel)
		if streamErr != nil {
			return nil, streamErr
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		responseID = strings.TrimSpace(streamResult.responseID)
		searchCount = streamResult.searchCount
		imageCount = streamResult.imageCount
		imageOutputSizes = streamResult.imageOutputSizes
	} else {
		nonStreamResult, nonStreamErr := s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, upstreamModel)
		if nonStreamErr != nil {
			return nil, nonStreamErr
		}
		usage = nonStreamResult.usage
		responseID = strings.TrimSpace(nonStreamResult.responseID)
		searchCount = nonStreamResult.searchCount
		imageCount = nonStreamResult.imageCount
		imageOutputSizes = nonStreamResult.imageOutputSizes
	}

	if usage == nil {
		usage = &OpenAIUsage{}
	}
	reasoningEffort := extractOpenAIReasoningEffortFromBody(forwardBody, originalModel)
	result := &OpenAIForwardResult{
		RequestID:       firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		UpstreamHeaders: resp.Header,
		ResponseID:      responseID,
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}
	if searchCount > 0 {
		result.SearchCount = searchCount
	}
	if imageCount > 0 {
		result.ImageCount = imageCount
		result.ImageOutputSizes = imageOutputSizes
	}
	return result, nil
}

func applyGrokPassthroughCacheIdentity(body []byte, identity string) ([]byte, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return body, nil
	}
	return sjson.SetBytes(body, "prompt_cache_key", identity)
}

func buildGrokPassthroughRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, cacheIdentity string, cfg *config.Config, settings ...*SettingService) (*http.Request, error) {
	targetURL, err := buildGrokResponsesURL(account, cfg, settings...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileGrok))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c != nil {
		if v := strings.TrimSpace(c.GetHeader("OpenAI-Beta")); v != "" {
			req.Header.Set("OpenAI-Beta", v)
		}
	}

	officialIdentity := grokInboundHasSupportedOfficialIdentity(c)
	if officialIdentity {
		copyGrokOfficialIdentityHeaders(req.Header, c.Request.Header)
	} else if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
	}
	if isGrokCLIProxyTarget(targetURL) {
		xai.EnsureCLIProxyAuthHeaders(req.Header)
	}
	copyGrokPassthroughForwardHeaders(req.Header, c)
	applyGrokCacheHeaders(req.Header, cacheIdentity)
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func grokInboundHasSupportedOfficialIdentity(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return xai.HasSupportedOfficialIdentity(c.Request.Header)
}

func copyGrokOfficialIdentityHeaders(dst, src http.Header) {
	if dst == nil || src == nil {
		return
	}
	if ua := strings.TrimSpace(src.Get("User-Agent")); ua != "" {
		dst.Set("User-Agent", ua)
	}
	if version := strings.TrimSpace(firstNonEmpty(src.Get("x-grok-client-version"), src.Get("X-Grok-Client-Version"))); version != "" {
		dst.Set("X-Grok-Client-Version", version)
		dst.Set("x-grok-client-version", version)
	}
	if identifier := strings.TrimSpace(src.Get("x-grok-client-identifier")); identifier != "" {
		dst.Set("x-grok-client-identifier", identifier)
	}
	if mode := strings.TrimSpace(firstNonEmpty(src.Get("x-grok-client-mode"), src.Get("X-Grok-Client-Mode"))); mode != "" {
		dst.Set("X-Grok-Client-Mode", mode)
		dst.Set("x-grok-client-mode", mode)
	}
}

func copyGrokPassthroughForwardHeaders(dst http.Header, c *gin.Context) {
	if dst == nil || c == nil {
		return
	}
	for _, key := range grokPassthroughForwardHeaders {
		value := strings.TrimSpace(c.GetHeader(key))
		if value == "" {
			continue
		}
		dst.Set(key, value)
	}
}
