package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	OpenAIAudioTranscriptionModel       = "gpt-4o-transcribe"
	openAIAudioTranscriptionMaxBytes    = 25 << 20
	openAIAudioTranscriptionsEndpoint   = "/v1/audio/transcriptions"
	chatgptAudioTranscriptionEndpoint   = "/backend-api/transcribe"
	chatgptAudioTranscriptionUpstream   = "https://chatgpt.com/backend-api/transcribe"
	defaultAudioTranscriptionFileName   = "audio.wav"
	defaultAudioTranscriptionMediaType  = "application/octet-stream"
	maxAudioTranscriptionTextFieldBytes = 64 << 10
)

var openAIAudioTranscriptionOAuthHeaderPrefixes = [...]string{"x-openai-", "x-codex-"}

type OpenAIAudioTranscriptionFormField struct {
	Name  string
	Value string
}

type OpenAIAudioTranscriptionRequest struct {
	FileName        string
	ContentType     string
	FileBytes       []byte
	Model           string
	Prompt          string
	ExtraFields     []OpenAIAudioTranscriptionFormField
	DurationSeconds int
	DurationParsed  bool
}

type OpenAIAudioTranscriptionOutput struct {
	Result      *OpenAIForwardResult
	Body        []byte
	Text        string
	StatusCode  int
	Header      http.Header
	ContentType string
}

type OpenAIAudioTranscriptionRequestError struct {
	Status  int
	Type    string
	Message string
	Param   string
}

func (e *OpenAIAudioTranscriptionRequestError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type OpenAIAudioTranscriptionUpstreamError struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

func (e *OpenAIAudioTranscriptionUpstreamError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("audio transcription upstream returned status %d", e.StatusCode)
}

func (e *OpenAIAudioTranscriptionUpstreamError) contentType() string {
	if e == nil || e.Header == nil {
		return "application/json"
	}
	if ct := strings.TrimSpace(e.Header.Get("Content-Type")); ct != "" {
		return ct
	}
	return "application/json"
}

func (s *OpenAIGatewayService) ParseOpenAIAudioTranscriptionRequest(body []byte, contentType string) (*OpenAIAudioTranscriptionRequest, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Expected multipart/form-data request", "")
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "multipart boundary is required", "")
	}
	if len(body) == 0 {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Request body is empty", "")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	parsed := &OpenAIAudioTranscriptionRequest{}
	modelSeen := false
	fileSeen := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Failed to parse multipart request", "")
		}

		formName := strings.TrimSpace(part.FormName())
		filename := strings.TrimSpace(part.FileName())
		switch formName {
		case "file":
			if filename == "" {
				_ = part.Close()
				return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "file must be a file upload", "file")
			}
			if fileSeen {
				_ = part.Close()
				return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Only one file is supported", "file")
			}
			data, readErr := readAudioTranscriptionFilePart(part)
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			parsed.FileName = filename
			parsed.ContentType = strings.TrimSpace(part.Header.Get("Content-Type"))
			parsed.FileBytes = data
			fileSeen = true
		case "model":
			if filename != "" {
				_ = part.Close()
				return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "model must be a form field", "model")
			}
			value, readErr := readAudioTranscriptionTextField(part)
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			parsed.Model = strings.TrimSpace(value)
			modelSeen = true
		case "prompt":
			if filename != "" {
				_ = part.Close()
				return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "prompt must be a form field", "prompt")
			}
			value, readErr := readAudioTranscriptionTextField(part)
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			parsed.Prompt = strings.TrimSpace(value)
		case "language", "response_format", "temperature", "stream", "chunking_strategy",
			"include", "include[]",
			"timestamp_granularities", "timestamp_granularities[]",
			"known_speaker_names", "known_speaker_names[]",
			"known_speaker_references", "known_speaker_references[]":
			if filename != "" {
				_ = part.Close()
				return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", formName+" must be a form field", formName)
			}
			value, readErr := readAudioTranscriptionTextField(part)
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			if value = strings.TrimSpace(value); value != "" {
				parsed.ExtraFields = append(parsed.ExtraFields, OpenAIAudioTranscriptionFormField{
					Name:  formName,
					Value: value,
				})
			}
		case "":
			_ = part.Close()
			return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Multipart field name is required", "")
		default:
			_ = part.Close()
			return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Unsupported audio transcription field: "+formName, formName)
		}
	}

	if !fileSeen || len(parsed.FileBytes) == 0 {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "file is required", "file")
	}
	if !modelSeen || strings.TrimSpace(parsed.Model) == "" {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "model is required", "model")
	}
	if parsed.Model != OpenAIAudioTranscriptionModel {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Unsupported audio transcription model: "+parsed.Model, "model")
	}
	if parsed.ContentType == "" {
		parsed.ContentType = defaultAudioTranscriptionMediaType
	}
	if seconds, ok := ExtractBillableAudioDurationSeconds(parsed.FileBytes); ok {
		parsed.DurationSeconds = seconds
		parsed.DurationParsed = true
	}
	return parsed, nil
}

func readAudioTranscriptionFilePart(part *multipart.Part) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(part, openAIAudioTranscriptionMaxBytes+1))
	if err != nil {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Failed to read file", "file")
	}
	if len(data) == 0 {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "file is required", "file")
	}
	if len(data) > openAIAudioTranscriptionMaxBytes {
		return nil, newOpenAIAudioTranscriptionRequestError(http.StatusRequestEntityTooLarge, "invalid_request_error", "Audio file exceeds 25 MiB limit", "file")
	}
	return data, nil
}

func readAudioTranscriptionTextField(part *multipart.Part) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxAudioTranscriptionTextFieldBytes+1))
	if err != nil {
		return "", newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Failed to read form field", part.FormName())
	}
	if len(data) > maxAudioTranscriptionTextFieldBytes {
		return "", newOpenAIAudioTranscriptionRequestError(http.StatusBadRequest, "invalid_request_error", "Form field is too large", part.FormName())
	}
	return string(data), nil
}

func newOpenAIAudioTranscriptionRequestError(status int, errType, message, param string) *OpenAIAudioTranscriptionRequestError {
	if status == 0 {
		status = http.StatusBadRequest
	}
	if strings.TrimSpace(errType) == "" {
		errType = "invalid_request_error"
	}
	return &OpenAIAudioTranscriptionRequestError{
		Status:  status,
		Type:    errType,
		Message: message,
		Param:   strings.TrimSpace(param),
	}
}

func (s *OpenAIGatewayService) ForwardAudioTranscription(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input *OpenAIAudioTranscriptionRequest,
) (*OpenAIAudioTranscriptionOutput, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, fmt.Errorf("openai gateway service is not configured")
	}
	if account == nil || !account.IsOpenAI() {
		return nil, fmt.Errorf("audio transcription requires an OpenAI account")
	}
	if input == nil {
		return nil, fmt.Errorf("audio transcription request is required")
	}
	switch account.Type {
	case AccountTypeAPIKey:
		return s.forwardOpenAIAudioTranscriptionAPIKey(ctx, c, account, input)
	case AccountTypeOAuth:
		return s.forwardOpenAIAudioTranscriptionOAuth(ctx, c, account, input)
	default:
		return nil, fmt.Errorf("unsupported OpenAI account type: %s", account.Type)
	}
}

func (s *OpenAIGatewayService) forwardOpenAIAudioTranscriptionAPIKey(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input *OpenAIAudioTranscriptionRequest,
) (*OpenAIAudioTranscriptionOutput, error) {
	apiKey := account.GetOpenAIApiKey()
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIAudioTranscriptionsURL(validatedURL)
	body, contentType, err := buildOpenAIAudioTranscriptionMultipart(input, true, true)
	if err != nil {
		return nil, err
	}
	req, err := s.buildOpenAIAudioTranscriptionHTTPRequest(ctx, targetURL, body, contentType)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else if c != nil {
		req.Header.Set("User-Agent", c.GetHeader("User-Agent"))
	}
	return s.forwardOpenAIAudioTranscriptionHTTP(ctx, c, account, req, input, openAIAudioTranscriptionsEndpoint)
}

func (s *OpenAIGatewayService) forwardOpenAIAudioTranscriptionOAuth(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input *OpenAIAudioTranscriptionRequest,
) (*OpenAIAudioTranscriptionOutput, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	body, contentType, err := buildOpenAIAudioTranscriptionMultipart(input, false, false)
	if err != nil {
		return nil, err
	}
	req, err := s.buildOpenAIAudioTranscriptionHTTPRequest(ctx, chatgptAudioTranscriptionUpstream, body, contentType)
	if err != nil {
		return nil, err
	}
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+token)
	if chatgptAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); chatgptAccountID != "" {
		req.Header.Set("chatgpt-account-id", chatgptAccountID)
	}
	applyOpenAIAudioTranscriptionOAuthClientHeaders(req, c)
	return s.forwardOpenAIAudioTranscriptionHTTP(ctx, c, account, req, input, chatgptAudioTranscriptionEndpoint)
}

func applyOpenAIAudioTranscriptionOAuthClientHeaders(req *http.Request, c *gin.Context) {
	if req == nil {
		return
	}
	copiedUserAgent := false
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(key)
			if lowerKey == "user-agent" {
				if setOpenAIAudioTranscriptionFirstNonEmptyHeader(req.Header, key, values) {
					copiedUserAgent = true
				}
				continue
			}
			if hasOpenAIAudioTranscriptionHeaderPrefix(lowerKey, openAIAudioTranscriptionOAuthHeaderPrefixes[:]) {
				addOpenAIAudioTranscriptionNonEmptyHeaders(req.Header, key, values)
			}
		}
	}
	if !copiedUserAgent {
		req.Header.Set("User-Agent", "")
	}
}

func setOpenAIAudioTranscriptionFirstNonEmptyHeader(header http.Header, key string, values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		header.Set(key, value)
		return true
	}
	return false
}

func addOpenAIAudioTranscriptionNonEmptyHeaders(header http.Header, key string, values []string) {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		header.Add(key, value)
	}
}

func hasOpenAIAudioTranscriptionHeaderPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) buildOpenAIAudioTranscriptionHTTPRequest(ctx context.Context, targetURL string, body []byte, contentType string) (*http.Request, error) {
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	req, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	releaseUpstreamCtx()
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", contentType)
	return req, nil
}

func (s *OpenAIGatewayService) forwardOpenAIAudioTranscriptionHTTP(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	req *http.Request,
	input *OpenAIAudioTranscriptionRequest,
	upstreamEndpoint string,
) (*OpenAIAudioTranscriptionOutput, error) {
	start := time.Now()
	proxyURL := resolveAccountProxyURL(account)

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(req.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, OpenAIAudioTranscriptionModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			UpstreamURL:        safeUpstreamURL(req.URL.String()),
			Kind:               "http_error",
			Message:            upstreamMsg,
		})
		return nil, &OpenAIAudioTranscriptionUpstreamError{
			StatusCode: resp.StatusCode,
			Body:       respBody,
			Header:     resp.Header.Clone(),
		}
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	textResult := gjson.GetBytes(respBody, "text")
	text := ""
	if textResult.Exists() && textResult.Type == gjson.String {
		text = textResult.String()
	} else if shouldAcceptRawAudioTranscriptionBody(input, contentType) {
		text = string(respBody)
	} else {
		return nil, fmt.Errorf("audio transcription upstream returned invalid response")
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(respBody)
	result := &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		ResponseID:      extractOpenAIResponseIDFromJSONBytes(respBody),
		Usage:           usage,
		Model:           OpenAIAudioTranscriptionModel,
		BillingModel:    OpenAIAudioTranscriptionModel,
		UpstreamModel:   OpenAIAudioTranscriptionModel,
		Stream:          false,
		OpenAIWSMode:    false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(start),
	}
	if !OpenAIUsageHasBillableTokens(usage) && input.DurationParsed {
		result.BillableDurationSeconds = input.DurationSeconds
	}

	logger.L().Debug("openai audio transcription: upstream succeeded",
		zap.Int64("account_id", account.ID),
		zap.String("upstream_endpoint", upstreamEndpoint),
		zap.Bool("has_token_usage", OpenAIUsageHasBillableTokens(usage)),
		zap.Int("billable_duration_seconds", result.BillableDurationSeconds),
	)
	return &OpenAIAudioTranscriptionOutput{
		Result:      result,
		Body:        respBody,
		Text:        text,
		StatusCode:  resp.StatusCode,
		Header:      resp.Header.Clone(),
		ContentType: contentType,
	}, nil
}

func buildOpenAIAudioTranscriptionMultipart(input *OpenAIAudioTranscriptionRequest, includeModel bool, includeExtraFields bool) ([]byte, string, error) {
	if input == nil {
		return nil, "", fmt.Errorf("audio transcription request is required")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	filename := strings.TrimSpace(input.FileName)
	if filename == "" {
		filename = defaultAudioTranscriptionFileName
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = defaultAudioTranscriptionMediaType
	}
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", `form-data; name="file"; filename="`+escapeMultipartQuotedString(filename)+`"`)
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(input.FileBytes); err != nil {
		return nil, "", err
	}
	if includeModel {
		if err := writer.WriteField("model", OpenAIAudioTranscriptionModel); err != nil {
			return nil, "", err
		}
	}
	if prompt := strings.TrimSpace(input.Prompt); prompt != "" {
		if err := writer.WriteField("prompt", prompt); err != nil {
			return nil, "", err
		}
	}
	if includeExtraFields {
		for _, field := range input.ExtraFields {
			name := strings.TrimSpace(field.Name)
			value := strings.TrimSpace(field.Value)
			if name == "" || value == "" {
				continue
			}
			if err := writer.WriteField(name, value); err != nil {
				return nil, "", err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func escapeMultipartQuotedString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func buildOpenAIAudioTranscriptionsURL(base string) string {
	return buildOpenAIEndpointURL(base, openAIAudioTranscriptionsEndpoint)
}

func HashOpenAIAudioTranscriptionPayload(model string, fileBytes []byte, prompt string, fields ...OpenAIAudioTranscriptionFormField) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(model)))
	_, _ = hash.Write([]byte{0})
	fileHash := sha256.Sum256(fileBytes)
	_, _ = hash.Write([]byte(hex.EncodeToString(fileHash[:])))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strings.TrimSpace(prompt)))
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		value := strings.TrimSpace(field.Value)
		if name == "" || value == "" {
			continue
		}
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func shouldAcceptRawAudioTranscriptionBody(input *OpenAIAudioTranscriptionRequest, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return true
	}
	if input == nil {
		return false
	}
	for _, field := range input.ExtraFields {
		name := strings.TrimSpace(field.Name)
		value := strings.ToLower(strings.TrimSpace(field.Value))
		switch name {
		case "stream":
			if value == "true" || value == "1" {
				return true
			}
		case "response_format":
			switch value {
			case "text", "srt", "vtt":
				return true
			}
		}
	}
	return false
}

func OpenAIUsageHasBillableTokens(usage OpenAIUsage) bool {
	return usage.InputTokens > 0 ||
		usage.OutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 ||
		usage.CacheReadInputTokens > 0 ||
		usage.ImageOutputTokens > 0
}

func (s *OpenAIGatewayService) CanBillOpenAIAudioDuration(ctx context.Context, apiKey *APIKey, model string, durationSeconds int) bool {
	if s == nil || apiKey == nil || apiKey.Group == nil || durationSeconds <= 0 {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = OpenAIAudioTranscriptionModel
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	return resolved != nil && resolved.Mode == BillingModeDuration && resolved.DefaultPerRequestPrice > 0
}

func (s *OpenAIGatewayService) WriteOpenAIAudioTranscriptionResponse(c *gin.Context, output *OpenAIAudioTranscriptionOutput) {
	if c == nil || output == nil || c.Writer.Written() {
		return
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), output.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(output.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	statusCode := output.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	c.Data(statusCode, contentType, output.Body)
}

func (s *OpenAIGatewayService) WriteOpenAIAudioTranscriptionUpstreamError(c *gin.Context, err *OpenAIAudioTranscriptionUpstreamError) {
	if c == nil || err == nil || c.Writer.Written() {
		return
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), err.Header, s.responseHeaderFilter)
	statusCode := err.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusBadGateway
	}
	c.Data(statusCode, err.contentType(), err.Body)
}

func (s *OpenAIGatewayService) EnsureOpenAIAudioTranscriptionBillable(output *OpenAIAudioTranscriptionOutput) error {
	if output == nil || output.Result == nil {
		return errors.New("audio transcription result is nil")
	}
	if OpenAIUsageHasBillableTokens(output.Result.Usage) || output.Result.BillableDurationSeconds > 0 {
		return nil
	}
	return errors.New("audio transcription upstream usage unavailable")
}
