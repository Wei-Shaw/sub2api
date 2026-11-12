package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// handleOpenAIImagesOAuthUpstreamHTTPError turns an upstream HTTP error whose
// body is already read and redacted into an account failover or a client error
// response. The single-request path and the fanout path share it.
func (s *OpenAIGatewayService) handleOpenAIImagesOAuthUpstreamHTTPError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamModel string,
	upstreamURL string,
	resp *http.Response,
	respBody []byte,
	direct bool,
) (*OpenAIForwardResult, error) {
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	if !direct && isOpenAIImagesMainModelError(resp.StatusCode, respBody) {
		return s.handleOpenAIImagesErrorResponse(ctx, resp, c, account, upstreamModel)
	}
	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMsg, respBody) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			UpstreamURL:        upstreamURL,
			Kind:               "failover",
			Message:            upstreamMsg,
		})
		shouldDisable := s.handleFailoverSideEffects(ctx, resp, account, respBody, upstreamModel)
		return nil, s.newOpenAIAccountFailoverError(
			account,
			resp.StatusCode,
			resp.Header,
			respBody,
			upstreamMsg,
			shouldDisable,
			!shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		)
	}
	return s.handleOpenAIImagesErrorResponse(ctx, resp, c, account, upstreamModel)
}

// openAIImagesOAuthFanoutMaxN bounds how many upstream calls one request may
// run at once on a single account.
const openAIImagesOAuthFanoutMaxN = 10

// openAIImagesOAuthFanoutAttempt is the outcome of one single-image upstream
// call. Exactly one of these describes it: requestErr (no HTTP response),
// statusCode >= 400 with errorBody, bodyErr (2xx whose body could not be read
// or held no image), or collected.
type openAIImagesOAuthFanoutAttempt struct {
	statusCode int
	headers    http.Header
	requestErr error
	errorBody  []byte
	body       []byte
	bodyErr    error
	collected  openAIImagesOAuthNonStreamingResult
}

func (a *openAIImagesOAuthFanoutAttempt) failed() bool {
	return a.requestErr != nil || a.statusCode >= http.StatusBadRequest || a.bodyErr != nil
}

// forwardOpenAIImagesOAuthFanout serves a non-streaming n > 1 request on the
// Responses bridge as n concurrent single-image calls and merges their images
// and usage into one Images API response. One failed call fails the request:
// it cancels the others and is reported the way the single-request path
// reports the same failure, so account retry and failover behave identically.
// The caller guarantees 1 < parsed.N <= openAIImagesOAuthFanoutMaxN.
func (s *OpenAIGatewayService) forwardOpenAIImagesOAuthFanout(
	ctx context.Context,
	upstreamCtx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
	requestModel string,
	upstreamModel string,
	token string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	single := *parsed
	single.N = 1
	responsesBody, err := buildOpenAIImagesResponsesRequest(&single, upstreamModel)
	if err != nil {
		return nil, err
	}
	targetURL, err := url.Parse(chatgptCodexURL)
	if err != nil {
		return nil, err
	}

	fanoutCtx, cancelFanout := context.WithCancel(withOpenAIImagesSelfBuiltRequest(upstreamCtx))
	defer cancelFanout()

	// Requests are built here because buildUpstreamRequest reads gin.Context;
	// the attempt goroutines below must not touch it.
	stickySeed := parsed.StickySessionSeed()
	requests := make([]*http.Request, 0, parsed.N)
	for i := 0; i < parsed.N; i++ {
		attemptSeed := stickySeed
		if attemptSeed != "" {
			attemptSeed = fmt.Sprintf("%s|fanout=%d", attemptSeed, i+1)
		}
		upstreamReq, err := s.buildUpstreamRequest(fanoutCtx, c, account, responsesBody, token, true, attemptSeed, false)
		if err != nil {
			return nil, err
		}
		attemptURL := *targetURL
		upstreamReq.URL = &attemptURL
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Accept", "text/event-stream")
		upstreamReq.Header.Set("OpenAI-Beta", "responses=experimental")
		requests = append(requests, upstreamReq)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	clientCtx := c.Request.Context()
	writerSizeBeforeResponse := OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)

	upstreamStart := time.Now()
	attempts := make([]openAIImagesOAuthFanoutAttempt, len(requests))
	// The first call to fail is the one reported: it is recorded before the
	// others are cancelled, so a cancellation it caused can never precede it.
	var (
		wg           sync.WaitGroup
		failureMu    sync.Mutex
		firstFailure = -1
	)
	for i, upstreamReq := range requests {
		wg.Add(1)
		go func(i int, upstreamReq *http.Request) {
			defer wg.Done()
			attempts[i] = s.runOpenAIImagesOAuthFanoutAttempt(upstreamReq, proxyURL, account, requestModel, clientCtx)
			if attempts[i].failed() {
				failureMu.Lock()
				if firstFailure < 0 {
					firstFailure = i
				}
				failureMu.Unlock()
				cancelFanout()
			}
		}(i, upstreamReq)
	}
	wg.Wait()
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())

	upstreamURL := safeUpstreamURL(targetURL.String())
	if firstFailure >= 0 {
		return s.handleOpenAIImagesOAuthFanoutFailure(ctx, upstreamCtx, c, account, parsed, channelMappedModel, upstreamModel, upstreamURL, writerSizeBeforeResponse, &attempts[firstFailure])
	}

	var (
		usage     OpenAIUsage
		results   []openAIResponsesImageResult
		createdAt int64
		firstMeta openAIResponsesImageResult
	)
	for i := range attempts {
		collected := attempts[i].collected
		addOpenAIImagesFanoutUsage(&usage, collected.usage)
		if len(results) == 0 {
			createdAt = collected.createdAt
			firstMeta = collected.firstMeta
		}
		results = append(results, collected.results...)
	}

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, openAIImagesFanoutUsageRaw(usage), firstMeta, parsed.ResponseFormat)
	if err != nil {
		return nil, err
	}
	// The response headers and request ID are those of the first call.
	first := attempts[0]
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), first.headers, s.responseHeaderFilter)
	c.Data(http.StatusOK, "application/json; charset=utf-8", responseBody)

	return &OpenAIForwardResult{
		RequestID:                     first.headers.Get("x-request-id"),
		UpstreamHeaders:               first.headers,
		Usage:                         usage,
		Model:                         requestModel,
		UpstreamModel:                 upstreamModel,
		UpstreamEndpoint:              targetURL.Path,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		Stream:                        false,
		ResponseHeaders:               first.headers.Clone(),
		Duration:                      time.Since(startTime),
		ImageEndpoint:                 true,
		ImageCount:                    resolveBillableImageCount(len(results), parsed.N, imageCountBillingEnabled(getAPIKeyFromContext(c))),
		ImageSize:                     parsed.SizeTier,
		ImageInputSize:                parsed.Size,
		ImageOutputSizes:              openAIResponsesImageResultSizes(results),
	}, nil
}

// runOpenAIImagesOAuthFanoutAttempt performs one upstream call and parses its
// body. It runs off the request goroutine, so it never touches gin.Context;
// handleOpenAIImagesOAuthFanoutFailure does the ops records and client writes.
func (s *OpenAIGatewayService) runOpenAIImagesOAuthFanoutAttempt(
	upstreamReq *http.Request,
	proxyURL string,
	account *Account,
	requestModel string,
	clientCtx context.Context,
) openAIImagesOAuthFanoutAttempt {
	var attempt openAIImagesOAuthFanoutAttempt
	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	if err != nil {
		attempt.requestErr = err
		return attempt
	}
	defer func() { _ = resp.Body.Close() }()
	attempt.statusCode = resp.StatusCode
	attempt.headers = resp.Header.Clone()
	if resp.StatusCode >= http.StatusBadRequest {
		attempt.errorBody = s.readUpstreamErrorBody(resp)
		return attempt
	}

	body, err := readUpstreamResponseBodyLimited(resp.Body, resolveUpstreamResponseReadLimit(s.cfg))
	if err != nil {
		if shouldClassifyOpenAIUpstreamStreamReadError(err, upstreamReq.Context(), clientCtx) {
			err = newOpenAIUpstreamStreamReadError(err)
		}
		attempt.bodyErr = err
		return attempt
	}
	attempt.body = body
	attempt.collected, attempt.bodyErr = s.collectOpenAIImagesOAuthNonStreamingBody(body, requestModel)
	return attempt
}

// handleOpenAIImagesOAuthFanoutFailure reports one failed call on the request
// goroutine with the same ops records, client responses and failover errors as
// forwardOpenAIImagesOAuth produces for a single call.
func (s *OpenAIGatewayService) handleOpenAIImagesOAuthFanoutFailure(
	ctx context.Context,
	upstreamCtx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
	upstreamModel string,
	upstreamURL string,
	writerSizeBeforeResponse int,
	failure *openAIImagesOAuthFanoutAttempt,
) (*OpenAIForwardResult, error) {
	if failure.requestErr != nil {
		safeErr := sanitizeUpstreamErrorMessage(failure.requestErr.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        upstreamURL,
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}

	resp := &http.Response{StatusCode: failure.statusCode, Header: failure.headers}
	if failure.statusCode >= http.StatusBadRequest {
		respBody := s.redactAgentIdentitySensitiveBody(upstreamCtx, account, failure.errorBody)
		if !agentIdentityTaskRecoveryWasTried(ctx) && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, respBody) {
			expectedTaskID := account.GetCredential("task_id")
			if err := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); err != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", err)
			}
			return s.forwardOpenAIImagesOAuth(markAgentIdentityTaskRecoveryTried(ctx), c, account, parsed, channelMappedModel)
		}
		return s.handleOpenAIImagesOAuthUpstreamHTTPError(upstreamCtx, c, account, upstreamModel, upstreamURL, resp, respBody, false)
	}

	if errors.Is(failure.bodyErr, ErrUpstreamResponseBodyTooLarge) {
		setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
		openAITooLargeError(c)
	} else {
		s.reportOpenAIImagesOAuthCollectError(c, failure.body, failure.bodyErr)
	}
	return nil, s.handleOpenAIImagesOAuthResponseError(upstreamCtx, c, account, upstreamModel, upstreamURL, resp, writerSizeBeforeResponse, failure.bodyErr)
}

func addOpenAIImagesFanoutUsage(dst *OpenAIUsage, src OpenAIUsage) {
	dst.InputTokens += src.InputTokens
	dst.ImageInputTokens += src.ImageInputTokens
	dst.ImageCacheReadTokens += src.ImageCacheReadTokens
	dst.OutputTokens += src.OutputTokens
	dst.CacheCreationInputTokens += src.CacheCreationInputTokens
	dst.CacheCreation30mInputTokens += src.CacheCreation30mInputTokens
	dst.CacheCreation12hInputTokens += src.CacheCreation12hInputTokens
	dst.CacheReadInputTokens += src.CacheReadInputTokens
	dst.ImageOutputTokens += src.ImageOutputTokens
}

// openAIImagesFanoutUsageRaw renders the summed usage as the Images API usage
// object, at the paths openAIUsageFromGJSON reads.
func openAIImagesFanoutUsageRaw(usage OpenAIUsage) []byte {
	if usage == (OpenAIUsage{}) {
		return nil
	}
	body := []byte(`{}`)
	body, _ = sjson.SetBytes(body, "input_tokens", usage.InputTokens)
	body, _ = sjson.SetBytes(body, "output_tokens", usage.OutputTokens)
	body, _ = sjson.SetBytes(body, "total_tokens", usage.InputTokens+usage.OutputTokens)
	if usage.CacheReadInputTokens > 0 {
		body, _ = sjson.SetBytes(body, "input_tokens_details.cached_tokens", usage.CacheReadInputTokens)
	}
	if usage.CacheCreationInputTokens > 0 {
		body, _ = sjson.SetBytes(body, "input_tokens_details.cache_creation_tokens", usage.CacheCreationInputTokens)
	}
	if usage.ImageInputTokens > 0 {
		body, _ = sjson.SetBytes(body, "input_tokens_details.image_tokens", usage.ImageInputTokens)
	}
	if usage.ImageOutputTokens > 0 {
		body, _ = sjson.SetBytes(body, "output_tokens_details.image_tokens", usage.ImageOutputTokens)
	}
	return body
}
