package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/httputil"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// chatgptTranscribeURL is a variable so tests can point the ChatGPT path at a
// local server.
var chatgptTranscribeURL = "https://chatgpt.com/backend-api/transcribe"

const (
	chatgptTranscribeRequestTimeout           = 110 * time.Second
	chatgptTranscribeResponseLimit            = 1 << 20
	openAIAudioTranscriptionChatGPTEndpoint   = "/backend-api/transcribe"
	openAIAudioTranscriptionPlatformEndpoint  = "/v1/audio/transcriptions"
	openAIAudioTranscriptionDefaultUpstreamUA = "audio.wav"
)

// ChatGPTUploadClientFactory builds the Chrome-impersonating client used for
// multipart uploads to chatgpt.com/backend-api.
type ChatGPTUploadClientFactory func(proxyURL string) (*req.Client, error)

// ForwardAudioTranscription transcribes parsed audio through the selected
// account: ChatGPT OAuth accounts use the chatgpt.com dictation endpoint, API
// key accounts receive the original multipart upload.
func (s *OpenAIGatewayService) ForwardAudioTranscription(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIAudioTranscriptionRequest,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	if s == nil || c == nil || account == nil || parsed == nil {
		return nil, fmt.Errorf("service, context, account and request are required")
	}
	billingModel := resolveOpenAIForwardModel(account, parsed.Model, defaultMappedModel)
	switch account.Type {
	case AccountTypeAPIKey:
		return s.forwardOpenAIAudioTranscriptionAPIKey(ctx, c, account, parsed, billingModel)
	case AccountTypeOAuth:
		return s.forwardOpenAIAudioTranscriptionChatGPT(ctx, c, account, parsed, billingModel)
	default:
		return nil, fmt.Errorf("account %d type %s cannot transcribe audio", account.ID, account.Type)
	}
}

func (s *OpenAIGatewayService) chatGPTUploadClient(proxyURL string) (*req.Client, error) {
	factory := s.chatGPTUploadClientFactory
	if factory == nil && s.openAITokenProvider != nil && s.openAITokenProvider.openAIOAuthService != nil {
		factory = s.openAITokenProvider.openAIOAuthService.chatGPTUploadClientFactory
	}
	if factory == nil {
		return nil, errors.New("chatgpt upload client is not configured")
	}
	return factory(proxyURL)
}

func (s *OpenAIGatewayService) forwardOpenAIAudioTranscriptionChatGPT(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIAudioTranscriptionRequest,
	billingModel string,
) (*OpenAIForwardResult, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("account %d has no access token for chatgpt transcription", account.ID)
	}
	client, err := s.chatGPTUploadClient(resolveAccountProxyURL(account))
	if err != nil {
		return nil, err
	}
	accountHeaders := http.Header{}
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, accountHeaders, account); err != nil {
		return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
	}
	fileName := parsed.FileName
	if fileName == "" {
		fileName = openAIAudioTranscriptionDefaultUpstreamUA
	}

	reqCtx, cancel := context.WithTimeout(ctx, chatgptTranscribeRequestTimeout)
	defer cancel()
	// The impersonated client presets Chrome's document-navigation headers;
	// the dictation endpoint expects an XHR-style call from the web app.
	request := client.R().
		SetContext(reqCtx).
		DisableAutoReadResponse().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Accept", "application/json").
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("sec-fetch-mode", "cors").
		SetHeader("sec-fetch-site", "same-origin").
		SetHeader("sec-fetch-dest", "empty").
		SetFileUpload(req.FileUpload{
			ParamName:   "file",
			FileName:    fileName,
			ContentType: parsed.FileContentType,
			FileSize:    int64(len(parsed.Audio)),
			GetFileContent: func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(parsed.Audio)), nil
			},
		})
	for key, values := range accountHeaders {
		for _, value := range values {
			request.SetHeader(key, value)
		}
	}

	upstreamStart := time.Now()
	resp, err := request.Post(chatgptTranscribeURL)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		if ctx.Err() == nil && errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			// Only our own budget expired; re-uploading to another account
			// would hit the same wall.
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, http.StatusGatewayTimeout, safeErr, "")
			writeOpenAIAudioTranscriptionError(c, http.StatusGatewayTimeout, "upstream_error", "Upstream transcription timed out")
			return nil, fmt.Errorf("chatgpt transcribe timed out: %s", safeErr)
		}
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, chatgptTranscribeResponseLimit))
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}

	if httputil.IsCloudflareChallengeResponse(resp.StatusCode, resp.Header, body) {
		// Every account shares the server's TLS fingerprint, so switching
		// accounts only repeats the upload against the same challenge.
		message := httputil.FormatCloudflareChallengeMessage("ChatGPT transcription was blocked by a Cloudflare challenge", resp.Header, body)
		setOpsUpstreamError(c, resp.StatusCode, message, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			Kind:               "http_error",
			Message:            message,
		})
		logger.L().With(zap.String("component", "service.openai_gateway")).Warn("openai_audio.transcribe_cf_blocked",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", resp.StatusCode),
			zap.String("cf_ray", httputil.ExtractCloudflareRayID(resp.Header, body)),
		)
		writeOpenAIAudioTranscriptionError(c, http.StatusBadGateway, "upstream_error", message)
		return nil, fmt.Errorf("chatgpt transcribe blocked by cloudflare challenge")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, s.chatGPTTranscribeUpstreamError(ctx, c, account, resp.StatusCode, resp.Header, body)
	}
	if !gjson.ValidBytes(body) || !gjson.GetBytes(body, "text").Exists() {
		setOpsUpstreamError(c, resp.StatusCode, "unexpected transcription response", "")
		writeOpenAIAudioTranscriptionError(c, http.StatusBadGateway, "upstream_error", "Upstream transcription returned an unexpected response")
		return nil, fmt.Errorf("chatgpt transcribe returned an unexpected body")
	}

	seconds := parsed.BilledSeconds()
	payload, contentType := openAIAudioTranscriptionResponse(gjson.GetBytes(body, "text").String(), parsed, seconds)
	c.Data(http.StatusOK, contentType, payload)
	return &OpenAIForwardResult{
		RequestID:        stableOpenAIAudioTranscriptionRequestID(resp.Header.Get("x-request-id")),
		Model:            parsed.Model,
		BillingModel:     billingModel,
		UpstreamEndpoint: openAIAudioTranscriptionChatGPTEndpoint,
		Duration:         time.Since(upstreamStart),
		AudioUsage:       openAIAudioTranscriptionUsage(seconds),
	}, nil
}

// chatGPTTranscribeUpstreamError maps a chatgpt.com error status onto the
// gateway's failover semantics. The dictation endpoint is undocumented, so a
// 401/403 from it never mutates account state: only 429 and 5xx feed the
// regular account error handling.
func (s *OpenAIGatewayService) chatGPTTranscribeUpstreamError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	statusCode int,
	header http.Header,
	body []byte,
) error {
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("chatgpt transcribe returned status %d", statusCode)
	}
	setOpsUpstreamError(c, statusCode, upstreamMsg, "")
	event := OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: statusCode,
		UpstreamRequestID:  header.Get("x-request-id"),
		Kind:               "http_error",
		Message:            upstreamMsg,
	}
	switch {
	case statusCode == http.StatusRequestEntityTooLarge:
		// Every account shares the same upload limit.
		appendOpsUpstreamError(c, event)
		writeOpenAIAudioTranscriptionError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", OpenAIRequestBodyTooLargeClientMessage)
		return fmt.Errorf("chatgpt transcribe rejected the upload size")
	case statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError:
		event.Kind = "failover"
		appendOpsUpstreamError(c, event)
		shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, statusCode, header, body, "")
		retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode)
		if statusCode == http.StatusTooManyRequests {
			return s.newOpenAIAccountFailoverError(account, statusCode, header, body, upstreamMsg, shouldDisable, retryableOnSameAccount)
		}
		return &UpstreamFailoverError{StatusCode: statusCode, ResponseBody: body, RetryableOnSameAccount: retryableOnSameAccount}
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		event.Kind = "failover"
		appendOpsUpstreamError(c, event)
		return &UpstreamFailoverError{StatusCode: statusCode, ResponseBody: body}
	default:
		appendOpsUpstreamError(c, event)
		writeOpenAIAudioTranscriptionError(c, statusCode, "upstream_error", upstreamMsg)
		return fmt.Errorf("chatgpt transcribe returned status %d", statusCode)
	}
}

func (s *OpenAIGatewayService) forwardOpenAIAudioTranscriptionAPIKey(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIAudioTranscriptionRequest,
	billingModel string,
) (*OpenAIForwardResult, error) {
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)
	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL := account.GetOpenAIFormatBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIEndpointURL(validatedURL, openAIAudioTranscriptionPlatformEndpoint)

	body, contentType := parsed.Body, parsed.ContentType
	if upstreamModel != "" && upstreamModel != parsed.Model {
		body, contentType, err = rewriteOpenAIImagesMultipartModel(body, contentType, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite transcription model: %w", err)
		}
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", contentType)
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	for key, values := range c.Request.Header {
		if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(upstreamReq, resolveAccountProxyURL(account), account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMsg, respBody) ||
				isOpenAIRequestBodyTooLargeError(resp.StatusCode, upstreamMsg, respBody) {
				return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, respBody, upstreamMsg, retryableOnSameAccount)
			}
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: retryableOnSameAccount}
		}
		writeOpenAIAudioTranscriptionUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeOpenAIAudioTranscriptionError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	writeOpenAIAudioTranscriptionUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)

	seconds := parsed.BilledSeconds()
	if upstreamSeconds, ok := openAIAudioTranscriptionUpstreamSeconds(respBody); ok {
		seconds = openAIAudioTranscriptionBilledSeconds(upstreamSeconds)
	}
	return &OpenAIForwardResult{
		RequestID:     stableOpenAIAudioTranscriptionRequestID(firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id"))),
		Model:         parsed.Model,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Duration:      time.Since(upstreamStart),
		AudioUsage:    openAIAudioTranscriptionUsage(seconds),
	}, nil
}
