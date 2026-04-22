package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIImagesGenerationsEndpoint = "/v1/images/generations"
	openAIImagesEditsEndpoint       = "/v1/images/edits"

	openAIImagesGenerationsURL = "https://api.openai.com/v1/images/generations"
	openAIImagesEditsURL       = "https://api.openai.com/v1/images/edits"

	openAIChatGPTStartURL               = "https://chatgpt.com/"
	openAIChatGPTFilesURL               = "https://chatgpt.com/backend-api/files"
	openAIChatGPTConversationInitURL    = "https://chatgpt.com/backend-api/conversation/init"
	openAIChatGPTConversationURL        = "https://chatgpt.com/backend-api/f/conversation"
	openAIChatGPTConversationPrepareURL = "https://chatgpt.com/backend-api/f/conversation/prepare"
	openAIChatGPTChatRequirementsURL    = "https://chatgpt.com/backend-api/sentinel/chat-requirements"

	openAIImageBackendUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	openAIImageRequirementsDiff = "0fffff"
)

type OpenAIImagesCapability string

const (
	OpenAIImagesCapabilityBasic  OpenAIImagesCapability = "images-basic"
	OpenAIImagesCapabilityNative OpenAIImagesCapability = "images-native"
)

type OpenAIImagesUpload struct {
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
	Width       int
	Height      int
}

type OpenAIImagesRequest struct {
	Endpoint           string
	ContentType        string
	Multipart          bool
	Model              string
	ExplicitModel      bool
	Prompt             string
	Stream             bool
	N                  int
	Size               string
	ExplicitSize       bool
	SizeTier           string
	ResponseFormat     string
	HasMask            bool
	HasNativeOptions   bool
	RequiredCapability OpenAIImagesCapability
	Uploads            []OpenAIImagesUpload
	Body               []byte
	bodyHash           string
}

func (r *OpenAIImagesRequest) IsEdits() bool {
	return r != nil && r.Endpoint == openAIImagesEditsEndpoint
}

func (r *OpenAIImagesRequest) StickySessionSeed() string {
	if r == nil {
		return ""
	}
	parts := []string{
		"openai-images",
		strings.TrimSpace(r.Endpoint),
		strings.TrimSpace(r.Model),
		strings.TrimSpace(r.Size),
		strings.TrimSpace(r.Prompt),
	}
	seed := strings.Join(parts, "|")
	if strings.TrimSpace(r.Prompt) == "" && r.bodyHash != "" {
		seed += "|body=" + r.bodyHash
	}
	return seed
}

func (s *OpenAIGatewayService) ParseOpenAIImagesRequest(c *gin.Context, body []byte) (*OpenAIImagesRequest, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("missing request context")
	}
	endpoint := normalizeOpenAIImagesEndpointPath(c.Request.URL.Path)
	if endpoint == "" {
		return nil, fmt.Errorf("unsupported images endpoint")
	}

	contentType := strings.TrimSpace(c.GetHeader("Content-Type"))
	req := &OpenAIImagesRequest{
		Endpoint:    endpoint,
		ContentType: contentType,
		N:           1,
		Body:        body,
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		req.bodyHash = hex.EncodeToString(sum[:8])
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		req.Multipart = true
		if parseErr := parseOpenAIImagesMultipartRequest(body, contentType, req); parseErr != nil {
			return nil, parseErr
		}
	} else {
		if len(body) == 0 {
			return nil, fmt.Errorf("request body is empty")
		}
		if !gjson.ValidBytes(body) {
			return nil, fmt.Errorf("failed to parse request body")
		}
		if parseErr := parseOpenAIImagesJSONRequest(body, req); parseErr != nil {
			return nil, parseErr
		}
	}

	applyOpenAIImagesDefaults(req)
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.RequiredCapability = classifyOpenAIImagesCapability(req)
	return req, nil
}

func parseOpenAIImagesJSONRequest(body []byte, req *OpenAIImagesRequest) error {
	if modelResult := gjson.GetBytes(body, "model"); modelResult.Exists() {
		req.Model = strings.TrimSpace(modelResult.String())
		req.ExplicitModel = req.Model != ""
	}
	req.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())

	if streamResult := gjson.GetBytes(body, "stream"); streamResult.Exists() {
		if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
			return fmt.Errorf("invalid stream field type")
		}
		req.Stream = streamResult.Bool()
	}

	if nResult := gjson.GetBytes(body, "n"); nResult.Exists() {
		if nResult.Type != gjson.Number {
			return fmt.Errorf("invalid n field type")
		}
		req.N = int(nResult.Int())
		if req.N <= 0 {
			return fmt.Errorf("n must be greater than 0")
		}
	}

	if sizeResult := gjson.GetBytes(body, "size"); sizeResult.Exists() {
		req.Size = strings.TrimSpace(sizeResult.String())
		req.ExplicitSize = req.Size != ""
	}
	req.ResponseFormat = strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "response_format").String()))
	req.HasMask = gjson.GetBytes(body, "mask").Exists()
	req.HasNativeOptions = hasOpenAINativeImageOptions(func(path string) bool {
		return gjson.GetBytes(body, path).Exists()
	})
	return nil
}

func parseOpenAIImagesMultipartRequest(body []byte, contentType string, req *OpenAIImagesRequest) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}

		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return fmt.Errorf("read multipart field %s: %w", name, err)
		}

		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
			if name == "mask" && len(data) > 0 {
				req.HasMask = true
			}
			if name == "image" || strings.HasPrefix(name, "image[") {
				width, height := parseOpenAIImageDimensions(part.Header)
				req.Uploads = append(req.Uploads, OpenAIImagesUpload{
					FieldName:   name,
					FileName:    fileName,
					ContentType: partContentType,
					Data:        data,
					Width:       width,
					Height:      height,
				})
			}
			continue
		}

		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			req.Model = value
			req.ExplicitModel = value != ""
		case "prompt":
			req.Prompt = value
		case "size":
			req.Size = value
			req.ExplicitSize = value != ""
		case "response_format":
			req.ResponseFormat = strings.ToLower(value)
		case "stream":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid stream field value")
			}
			req.Stream = parsed
		case "n":
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return fmt.Errorf("n must be a positive integer")
			}
			req.N = n
		default:
			if isOpenAINativeImageOption(name) && value != "" {
				req.HasNativeOptions = true
			}
		}
	}

	if len(req.Uploads) == 0 && req.IsEdits() {
		return fmt.Errorf("image file is required")
	}
	return nil
}

func parseOpenAIImageDimensions(_ textproto.MIMEHeader) (int, int) {
	return 0, 0
}

func applyOpenAIImagesDefaults(req *OpenAIImagesRequest) {
	if req == nil {
		return
	}
	if req.N <= 0 {
		req.N = 1
	}
	if strings.TrimSpace(req.Model) != "" {
		req.Model = strings.TrimSpace(req.Model)
		return
	}
	req.Model = "gpt-image-2"
}

func normalizeOpenAIImagesEndpointPath(path string) string {
	trimmed := strings.TrimSpace(path)
	switch {
	case strings.Contains(trimmed, "/images/generations"):
		return openAIImagesGenerationsEndpoint
	case strings.Contains(trimmed, "/images/edits"):
		return openAIImagesEditsEndpoint
	default:
		return ""
	}
}

func classifyOpenAIImagesCapability(req *OpenAIImagesRequest) OpenAIImagesCapability {
	if req == nil {
		return OpenAIImagesCapabilityNative
	}
	if req.ExplicitModel || req.ExplicitSize {
		return OpenAIImagesCapabilityNative
	}
	model := strings.ToLower(strings.TrimSpace(req.Model))
	if !strings.HasPrefix(model, "gpt-image-") {
		return OpenAIImagesCapabilityNative
	}
	if req.Stream || req.N != 1 || req.HasMask || req.HasNativeOptions {
		return OpenAIImagesCapabilityNative
	}
	if req.IsEdits() && !req.Multipart {
		return OpenAIImagesCapabilityNative
	}
	if req.ResponseFormat != "" && req.ResponseFormat != "b64_json" {
		return OpenAIImagesCapabilityNative
	}
	return OpenAIImagesCapabilityBasic
}

func hasOpenAINativeImageOptions(exists func(path string) bool) bool {
	for _, path := range []string{
		"background",
		"quality",
		"style",
		"output_format",
		"output_compression",
		"moderation",
	} {
		if exists(path) {
			return true
		}
	}
	return false
}

func isOpenAINativeImageOption(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "background", "quality", "style", "output_format", "output_compression", "moderation":
		return true
	default:
		return false
	}
}

func normalizeOpenAIImageSizeTier(size string) string {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "1024x1024":
		return "1K"
	case "1536x1024", "1024x1536", "1792x1024", "1024x1792", "", "auto":
		return "2K"
	default:
		return "2K"
	}
}

func (s *OpenAIGatewayService) ForwardImages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	switch account.Type {
	case AccountTypeAPIKey:
		return s.forwardOpenAIImagesAPIKey(ctx, c, account, body, parsed, channelMappedModel)
	case AccountTypeOAuth:
		return s.forwardOpenAIImagesOAuth(ctx, c, account, parsed, channelMappedModel)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKey(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	upstreamModel := account.GetMappedModel(requestModel)
	forwardBody, forwardContentType, err := rewriteOpenAIImagesModel(body, parsed.ContentType, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !parsed.Multipart {
		setOpsUpstreamRequestBody(c, forwardBody)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(ctx, c, account, forwardBody, forwardContentType, token, parsed.Endpoint)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
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
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, forwardBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var usage OpenAIUsage
	imageCount := parsed.N
	var firstTokenMs *int
	if parsed.Stream {
		streamUsage, streamCount, ttft, err := s.handleOpenAIImagesStreamingResponse(resp, c, startTime)
		if err != nil {
			return nil, err
		}
		usage = streamUsage
		imageCount = streamCount
		firstTokenMs = ttft
	} else {
		nonStreamUsage, nonStreamCount, err := s.handleOpenAIImagesNonStreamingResponse(resp, c)
		if err != nil {
			return nil, err
		}
		usage = nonStreamUsage
		if nonStreamCount > 0 {
			imageCount = nonStreamCount
		}
	}
	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           usage,
		Model:           requestModel,
		UpstreamModel:   upstreamModel,
		Stream:          parsed.Stream,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
		ImageCount:      imageCount,
		ImageSize:       parsed.SizeTier,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIImagesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	contentType string,
	token string,
	endpoint string,
) (*http.Request, error) {
	targetURL := openAIImagesGenerationsURL
	if endpoint == openAIImagesEditsEndpoint {
		targetURL = openAIImagesEditsURL
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIImagesURL(validatedURL, endpoint)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func buildOpenAIImagesURL(base string, endpoint string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	relative := strings.TrimPrefix(strings.TrimSpace(endpoint), "/v1")
	if strings.HasSuffix(normalized, endpoint) || strings.HasSuffix(normalized, relative) {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + relative
	}
	return normalized + endpoint
}

func rewriteOpenAIImagesModel(body []byte, contentType string, model string) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		rewrittenBody, rewrittenType, rewriteErr := rewriteOpenAIImagesMultipartModel(body, contentType, model)
		return rewrittenBody, rewrittenType, rewriteErr
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite image request model: %w", err)
	}
	return rewritten, contentType, nil
}

func rewriteOpenAIImagesMultipartModel(body []byte, contentType string, model string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	modelWritten := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}

		formName := strings.TrimSpace(part.FormName())
		partHeader := cloneMultipartHeader(part.Header)
		target, err := writer.CreatePart(partHeader)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}

		if formName == "model" && part.FileName() == "" {
			if _, err := target.Write([]byte(model)); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("rewrite multipart model: %w", err)
			}
			modelWritten = true
			_ = part.Close()
			continue
		}
		if _, err := io.Copy(target, part); err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("copy multipart part: %w", err)
		}
		_ = part.Close()
	}

	if !modelWritten {
		if err := writer.WriteField("model", model); err != nil {
			return nil, "", fmt.Errorf("append multipart model field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func cloneMultipartHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	dst := make(textproto.MIMEHeader, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func (s *OpenAIGatewayService) handleOpenAIImagesNonStreamingResponse(resp *http.Response, c *gin.Context) (OpenAIUsage, int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	c.Data(resp.StatusCode, contentType, body)

	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	return usage, extractOpenAIImageCountFromJSONBytes(body), nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
) (OpenAIUsage, int, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", contentType)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, 0, nil, fmt.Errorf("streaming is not supported by response writer")
	}

	reader := bufio.NewReader(resp.Body)
	usage := OpenAIUsage{}
	imageCount := 0
	var firstTokenMs *int

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			if _, writeErr := c.Writer.Write(line); writeErr != nil {
				return OpenAIUsage{}, 0, firstTokenMs, writeErr
			}
			flusher.Flush()

			if data, ok := extractOpenAISSEDataLine(strings.TrimRight(string(line), "\r\n")); ok && data != "" && data != "[DONE]" {
				dataBytes := []byte(data)
				mergeOpenAIUsage(&usage, dataBytes)
				if count := extractOpenAIImageCountFromJSONBytes(dataBytes); count > imageCount {
					imageCount = count
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return OpenAIUsage{}, 0, firstTokenMs, err
		}
	}
	return usage, imageCount, firstTokenMs, nil
}

func mergeOpenAIUsage(dst *OpenAIUsage, body []byte) {
	if dst == nil {
		return
	}
	if parsed, ok := extractOpenAIUsageFromJSONBytes(body); ok {
		if parsed.InputTokens > 0 {
			dst.InputTokens = parsed.InputTokens
		}
		if parsed.OutputTokens > 0 {
			dst.OutputTokens = parsed.OutputTokens
		}
		if parsed.CacheReadInputTokens > 0 {
			dst.CacheReadInputTokens = parsed.CacheReadInputTokens
		}
		if parsed.ImageOutputTokens > 0 {
			dst.ImageOutputTokens = parsed.ImageOutputTokens
		}
	}
}

func extractOpenAIImageCountFromJSONBytes(body []byte) int {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0
	}
	data := gjson.GetBytes(body, "data")
	if data.Exists() && data.IsArray() {
		return len(data.Array())
	}
	return 0
}

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuth(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	client, err := newOpenAIBackendAPIClient(resolveOpenAIProxyURL(account))
	if err != nil {
		return nil, err
	}
	headers, err := s.buildOpenAIBackendAPIHeaders(account, token)
	if err != nil {
		return nil, err
	}
	if bootstrapErr := bootstrapOpenAIBackendAPI(ctx, client, headers); bootstrapErr != nil {
		logger.LegacyPrintf("service.openai_gateway", "OpenAI image bootstrap failed: %v", bootstrapErr)
	}

	chatReqs, err := fetchOpenAIChatRequirements(ctx, client, headers)
	if err != nil {
		return nil, s.wrapOpenAIImageBackendError(ctx, c, account, err)
	}
	if chatReqs.Arkose.Required {
		return nil, s.wrapOpenAIImageBackendError(
			ctx,
			c,
			account,
			newOpenAIImageSyntheticStatusError(
				http.StatusForbidden,
				"chat-requirements requires unsupported challenge (arkose)",
				openAIChatGPTChatRequirementsURL,
			),
		)
	}

	parentMessageID := uuid.NewString()
	proofToken := generateOpenAIProofToken(chatReqs.ProofOfWork.Required, chatReqs.ProofOfWork.Seed, chatReqs.ProofOfWork.Difficulty, headers.Get("User-Agent"))
	_ = initializeOpenAIImageConversation(ctx, client, headers)
	conduitToken, err := prepareOpenAIImageConversation(ctx, client, headers, parsed.Prompt, parentMessageID, chatReqs.Token, proofToken)
	if err != nil {
		return nil, s.wrapOpenAIImageBackendError(ctx, c, account, err)
	}

	uploads, err := uploadOpenAIImageFiles(ctx, client, headers, parsed.Uploads)
	if err != nil {
		return nil, s.wrapOpenAIImageBackendError(ctx, c, account, err)
	}

	convReq := buildOpenAIImageConversationRequest(parsed, parentMessageID, uploads)
	if parsedContent, err := json.Marshal(convReq); err == nil {
		setOpsUpstreamRequestBody(c, parsedContent)
	}
	convHeaders := cloneHTTPHeader(headers)
	convHeaders.Set("Accept", "text/event-stream")
	convHeaders.Set("Content-Type", "application/json")
	convHeaders.Set("openai-sentinel-chat-requirements-token", chatReqs.Token)
	if conduitToken != "" {
		convHeaders.Set("x-conduit-token", conduitToken)
	}
	if proofToken != "" {
		convHeaders.Set("openai-sentinel-proof-token", proofToken)
	}

	resp, err := client.R().
		SetContext(ctx).
		DisableAutoReadResponse().
		SetHeaders(headerToMap(convHeaders)).
		SetBodyJsonMarshal(convReq).
		Post(openAIChatGPTConversationURL)
	if err != nil {
		return nil, fmt.Errorf("openai image conversation request failed: %w", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode >= 400 {
		return nil, s.wrapOpenAIImageBackendError(ctx, c, account, handleOpenAIImageBackendError(resp))
	}

	conversationID, pointerInfos, usage, firstTokenMs, err := readOpenAIImageConversationStream(resp, startTime)
	if err != nil {
		return nil, err
	}
	pointerInfos = mergeOpenAIImagePointerInfos(pointerInfos, nil)
	if conversationID != "" && !hasOpenAIFileServicePointerInfos(pointerInfos) {
		polledPointers, pollErr := pollOpenAIImageConversation(ctx, client, headers, conversationID)
		if pollErr != nil {
			return nil, s.wrapOpenAIImageBackendError(ctx, c, account, pollErr)
		}
		pointerInfos = mergeOpenAIImagePointerInfos(pointerInfos, polledPointers)
	}
	pointerInfos = preferOpenAIFileServicePointerInfos(pointerInfos)
	if len(pointerInfos) == 0 {
		return nil, fmt.Errorf("openai image conversation returned no downloadable images")
	}

	responseBody, imageCount, err := buildOpenAIImageResponse(ctx, client, headers, conversationID, pointerInfos)
	if err != nil {
		return nil, s.wrapOpenAIImageBackendError(ctx, c, account, err)
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", responseBody)
	return &OpenAIForwardResult{
		RequestID:     resp.Header.Get("x-request-id"),
		Usage:         usage,
		Model:         requestModel,
		UpstreamModel: requestModel,
		Stream:        false,
		Duration:      time.Since(startTime),
		FirstTokenMs:  firstTokenMs,
		ImageCount:    imageCount,
		ImageSize:     parsed.SizeTier,
	}, nil
}

func resolveOpenAIProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

func newOpenAIBackendAPIClient(proxyURL string) (*req.Client, error) {
	client := req.C().
		SetTimeout(180 * time.Second).
		ImpersonateChrome()
	trimmed, _, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if trimmed != "" {
		client.SetProxyURL(trimmed)
	}
	return client, nil
}

func (s *OpenAIGatewayService) buildOpenAIBackendAPIHeaders(account *Account, token string) (http.Header, error) {
	deviceID, sessionID := s.ensureOpenAIImageSessionCredentials(context.Background(), account)
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Accept", "application/json")
	headers.Set("Origin", "https://chatgpt.com")
	headers.Set("Referer", "https://chatgpt.com/")
	headers.Set("Sec-Fetch-Dest", "empty")
	headers.Set("Sec-Fetch-Mode", "cors")
	headers.Set("Sec-Fetch-Site", "same-origin")
	headers.Set("User-Agent", openAIImageBackendUserAgent)
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		headers.Set("User-Agent", customUA)
	}
	if chatgptAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); chatgptAccountID != "" {
		headers.Set("chatgpt-account-id", chatgptAccountID)
	}
	if deviceID != "" {
		headers.Set("oai-device-id", deviceID)
		headers.Set("Cookie", "oai-did="+deviceID)
	}
	if sessionID != "" {
		headers.Set("oai-session-id", sessionID)
	}
	return headers, nil
}

func (s *OpenAIGatewayService) ensureOpenAIImageSessionCredentials(ctx context.Context, account *Account) (string, string) {
	if account == nil {
		return "", ""
	}
	deviceID := account.GetOpenAIDeviceID()
	sessionID := account.GetOpenAISessionID()
	if deviceID != "" && sessionID != "" {
		return deviceID, sessionID
	}

	updates := map[string]any{}
	if deviceID == "" {
		deviceID = uuid.NewString()
		updates["openai_device_id"] = deviceID
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
		updates["openai_session_id"] = sessionID
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	if len(updates) == 0 || s == nil || s.accountRepo == nil {
		return deviceID, sessionID
	}

	updateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.accountRepo.UpdateExtra(updateCtx, account.ID, updates); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "persist openai image session creds failed: account=%d err=%v", account.ID, err)
	}
	return deviceID, sessionID
}

func bootstrapOpenAIBackendAPI(ctx context.Context, client *req.Client, headers http.Header) error {
	resp, err := client.R().
		SetContext(ctx).
		DisableAutoReadResponse().
		SetHeaders(headerToMap(headers)).
		Get(openAIChatGPTStartURL)
	if err != nil {
		return err
	}
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	return nil
}

func initializeOpenAIImageConversation(ctx context.Context, client *req.Client, headers http.Header) error {
	payload := map[string]any{
		"gizmo_id":                nil,
		"requested_default_model": nil,
		"conversation_id":         nil,
		"timezone_offset_min":     openAITimezoneOffsetMinutes(),
		"system_hints":            []string{"picture_v2"},
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(headers)).
		SetBodyJsonMarshal(payload).
		Post(openAIChatGPTConversationInitURL)
	if err != nil {
		return err
	}
	if !resp.IsSuccessState() {
		return newOpenAIImageStatusError(resp, "conversation init failed")
	}
	return nil
}

type openAIChatRequirements struct {
	Token     string `json:"token"`
	Turnstile struct {
		Required bool `json:"required"`
	} `json:"turnstile"`
	Arkose struct {
		Required bool `json:"required"`
	} `json:"arkose"`
	ProofOfWork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
}

func fetchOpenAIChatRequirements(ctx context.Context, client *req.Client, headers http.Header) (*openAIChatRequirements, error) {
	var lastErr error
	for _, payload := range []map[string]any{
		{"p": nil},
		{"p": generateOpenAIRequirementsToken(headers.Get("User-Agent"))},
	} {
		var result openAIChatRequirements
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetBodyJsonMarshal(payload).
			SetSuccessResult(&result).
			Post(openAIChatGPTChatRequirementsURL)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.IsSuccessState() && strings.TrimSpace(result.Token) != "" {
			return &result, nil
		}
		lastErr = newOpenAIImageStatusError(resp, "chat-requirements failed")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("chat-requirements failed")
	}
	return nil, lastErr
}

func prepareOpenAIImageConversation(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	prompt string,
	parentMessageID string,
	chatToken string,
	proofToken string,
) (string, error) {
	messageID := uuid.NewString()
	payload := map[string]any{
		"action":                "next",
		"client_prepare_state":  "success",
		"fork_from_shared_post": false,
		"parent_message_id":     parentMessageID,
		"model":                 "auto",
		"timezone_offset_min":   openAITimezoneOffsetMinutes(),
		"timezone":              openAITimezoneName(),
		"conversation_mode":     map[string]any{"kind": "primary_assistant"},
		"system_hints":          []string{"picture_v2"},
		"supports_buffering":    true,
		"supported_encodings":   []string{"v1"},
		"partial_query": map[string]any{
			"id":     messageID,
			"author": map[string]any{"role": "user"},
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{coalesceOpenAIFileName(prompt, "Generate an image.")},
			},
		},
		"client_contextual_info": map[string]any{
			"app_name": "chatgpt.com",
		},
	}
	prepareHeaders := cloneHTTPHeader(headers)
	prepareHeaders.Set("Accept", "*/*")
	prepareHeaders.Set("Content-Type", "application/json")
	if strings.TrimSpace(chatToken) != "" {
		prepareHeaders.Set("openai-sentinel-chat-requirements-token", strings.TrimSpace(chatToken))
	}
	if strings.TrimSpace(proofToken) != "" {
		prepareHeaders.Set("openai-sentinel-proof-token", strings.TrimSpace(proofToken))
	}
	var result struct {
		ConduitToken string `json:"conduit_token"`
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(prepareHeaders)).
		SetBodyJsonMarshal(payload).
		SetSuccessResult(&result).
		Post(openAIChatGPTConversationPrepareURL)
	if err != nil {
		return "", err
	}
	if !resp.IsSuccessState() {
		return "", newOpenAIImageStatusError(resp, "conversation prepare failed")
	}
	return strings.TrimSpace(result.ConduitToken), nil
}

type openAIUploadedImage struct {
	FileID   string
	FileName string
	FileSize int
	MimeType string
	Width    int
	Height   int
}

func uploadOpenAIImageFiles(ctx context.Context, client *req.Client, headers http.Header, uploads []OpenAIImagesUpload) ([]openAIUploadedImage, error) {
	if len(uploads) == 0 {
		return nil, nil
	}
	results := make([]openAIUploadedImage, 0, len(uploads))
	for i := range uploads {
		item := uploads[i]
		fileName := coalesceOpenAIFileName(item.FileName, "image.png")
		payload := map[string]any{
			"file_name": fileName,
			"file_size": len(item.Data),
			"use_case":  "multimodal",
		}
		var created struct {
			FileID    string `json:"file_id"`
			UploadURL string `json:"upload_url"`
		}
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetBodyJsonMarshal(payload).
			SetSuccessResult(&created).
			Post(openAIChatGPTFilesURL)
		if err != nil {
			return nil, err
		}
		if !resp.IsSuccessState() || strings.TrimSpace(created.FileID) == "" || strings.TrimSpace(created.UploadURL) == "" {
			return nil, newOpenAIImageStatusError(resp, "create upload slot failed")
		}

		uploadHeaders := map[string]string{
			"Content-Type":   coalesceOpenAIFileName(item.ContentType, "application/octet-stream"),
			"Origin":         "https://chatgpt.com",
			"x-ms-blob-type": "BlockBlob",
			"x-ms-version":   "2020-04-08",
			"User-Agent":     headers.Get("User-Agent"),
		}
		putResp, err := client.R().
			SetContext(ctx).
			SetHeaders(uploadHeaders).
			SetBody(item.Data).
			DisableAutoReadResponse().
			Put(created.UploadURL)
		if err != nil {
			return nil, err
		}
		if putResp.Response != nil && putResp.Body != nil {
			_, _ = io.Copy(io.Discard, putResp.Body)
			_ = putResp.Body.Close()
		}
		if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
			return nil, newOpenAIImageStatusError(putResp, "upload image bytes failed")
		}

		uploadedResp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetBodyJsonMarshal(map[string]any{}).
			Post(fmt.Sprintf("%s/%s/uploaded", openAIChatGPTFilesURL, created.FileID))
		if err != nil {
			return nil, err
		}
		if !uploadedResp.IsSuccessState() {
			return nil, newOpenAIImageStatusError(uploadedResp, "mark upload complete failed")
		}

		results = append(results, openAIUploadedImage{
			FileID:   created.FileID,
			FileName: fileName,
			FileSize: len(item.Data),
			MimeType: coalesceOpenAIFileName(item.ContentType, "application/octet-stream"),
			Width:    item.Width,
			Height:   item.Height,
		})
	}
	return results, nil
}

func coalesceOpenAIFileName(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func buildOpenAIImageConversationRequest(parsed *OpenAIImagesRequest, parentMessageID string, uploads []openAIUploadedImage) map[string]any {
	parts := []any{coalesceOpenAIFileName(parsed.Prompt, "Generate an image.")}
	attachments := make([]map[string]any, 0, len(uploads))
	if len(uploads) > 0 {
		parts = make([]any, 0, len(uploads)+1)
		for _, upload := range uploads {
			parts = append(parts, map[string]any{
				"content_type":  "image_asset_pointer",
				"asset_pointer": "file-service://" + upload.FileID,
				"size_bytes":    upload.FileSize,
				"width":         upload.Width,
				"height":        upload.Height,
			})
			attachment := map[string]any{
				"id":       upload.FileID,
				"mimeType": upload.MimeType,
				"name":     upload.FileName,
				"size":     upload.FileSize,
			}
			if upload.Width > 0 {
				attachment["width"] = upload.Width
			}
			if upload.Height > 0 {
				attachment["height"] = upload.Height
			}
			attachments = append(attachments, attachment)
		}
		parts = append(parts, coalesceOpenAIFileName(parsed.Prompt, "Edit this image."))
	}

	contentType := "text"
	if len(uploads) > 0 {
		contentType = "multimodal_text"
	}
	metadata := map[string]any{
		"developer_mode_connector_ids": []any{},
		"selected_github_repos":        []any{},
		"selected_all_github_repos":    false,
		"system_hints":                 []string{"picture_v2"},
		"serialization_metadata": map[string]any{
			"custom_symbol_offsets": []any{},
		},
	}
	message := map[string]any{
		"id":     uuid.NewString(),
		"author": map[string]any{"role": "user"},
		"content": map[string]any{
			"content_type": contentType,
			"parts":        parts,
		},
		"metadata":    metadata,
		"create_time": float64(time.Now().UnixMilli()) / 1000,
	}
	if len(attachments) > 0 {
		metadata["attachments"] = attachments
	}

	return map[string]any{
		"action":                               "next",
		"client_prepare_state":                 "sent",
		"parent_message_id":                    parentMessageID,
		"model":                                "auto",
		"timezone_offset_min":                  openAITimezoneOffsetMinutes(),
		"timezone":                             openAITimezoneName(),
		"conversation_mode":                    map[string]any{"kind": "primary_assistant"},
		"enable_message_followups":             true,
		"system_hints":                         []string{"picture_v2"},
		"supports_buffering":                   true,
		"supported_encodings":                  []string{"v1"},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 200,
			"page_height":       900,
			"page_width":        1440,
			"pixel_ratio":       1,
			"screen_height":     1080,
			"screen_width":      1920,
			"app_name":          "chatgpt.com",
		},
		"messages": []any{message},
	}
}

type openAIImagePointerInfo struct {
	Pointer string
	Prompt  string
}

type openAIImageToolMessage struct {
	MessageID    string
	CreateTime   float64
	PointerInfos []openAIImagePointerInfo
}

func readOpenAIImageConversationStream(resp *req.Response, startTime time.Time) (string, []openAIImagePointerInfo, OpenAIUsage, *int, error) {
	if resp == nil || resp.Body == nil {
		return "", nil, OpenAIUsage{}, nil, fmt.Errorf("empty conversation response")
	}
	reader := bufio.NewReader(resp.Body)
	var (
		conversationID string
		firstTokenMs   *int
		usage          OpenAIUsage
		pointers       []openAIImagePointerInfo
	)

	for {
		line, err := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" && firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		if data, ok := extractOpenAISSEDataLine(strings.TrimRight(line, "\r\n")); ok && data != "" && data != "[DONE]" {
			dataBytes := []byte(data)
			if conversationID == "" {
				conversationID = strings.TrimSpace(gjson.GetBytes(dataBytes, "v.conversation_id").String())
				if conversationID == "" {
					conversationID = strings.TrimSpace(gjson.GetBytes(dataBytes, "conversation_id").String())
				}
			}
			mergeOpenAIUsage(&usage, dataBytes)
			pointers = mergeOpenAIImagePointerInfos(pointers, collectOpenAIImagePointers(dataBytes))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, OpenAIUsage{}, firstTokenMs, err
		}
	}
	return conversationID, pointers, usage, firstTokenMs, nil
}

func collectOpenAIImagePointers(body []byte) []openAIImagePointerInfo {
	if len(body) == 0 {
		return nil
	}
	matches := openAIImagePointerMatches(body)
	if len(matches) == 0 {
		return nil
	}
	prompt := ""
	for _, path := range []string{
		"message.metadata.dalle.prompt",
		"metadata.dalle.prompt",
		"revised_prompt",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			prompt = value
			break
		}
	}
	out := make([]openAIImagePointerInfo, 0, len(matches))
	for _, pointer := range matches {
		out = append(out, openAIImagePointerInfo{Pointer: pointer, Prompt: prompt})
	}
	return out
}

func openAIImagePointerMatches(body []byte) []string {
	raw := string(body)
	matches := make([]string, 0, 4)
	for _, prefix := range []string{"file-service://", "sediment://"} {
		start := 0
		for {
			idx := strings.Index(raw[start:], prefix)
			if idx < 0 {
				break
			}
			idx += start
			end := idx + len(prefix)
			for end < len(raw) {
				ch := raw[end]
				if ch != '-' && ch != '_' &&
					(ch < '0' || ch > '9') &&
					(ch < 'a' || ch > 'z') &&
					(ch < 'A' || ch > 'Z') {
					break
				}
				end++
			}
			matches = append(matches, raw[idx:end])
			start = end
		}
	}
	return dedupeStrings(matches)
}

func mergeOpenAIImagePointerInfos(existing []openAIImagePointerInfo, next []openAIImagePointerInfo) []openAIImagePointerInfo {
	if len(next) == 0 {
		return existing
	}
	seen := make(map[string]openAIImagePointerInfo, len(existing)+len(next))
	out := make([]openAIImagePointerInfo, 0, len(existing)+len(next))
	for _, item := range existing {
		seen[item.Pointer] = item
		out = append(out, item)
	}
	for _, item := range next {
		if existingItem, ok := seen[item.Pointer]; ok {
			if existingItem.Prompt == "" && item.Prompt != "" {
				for i := range out {
					if out[i].Pointer == item.Pointer {
						out[i].Prompt = item.Prompt
						break
					}
				}
			}
			continue
		}
		seen[item.Pointer] = item
		out = append(out, item)
	}
	return out
}

func hasOpenAIFileServicePointerInfos(items []openAIImagePointerInfo) bool {
	for _, item := range items {
		if strings.HasPrefix(item.Pointer, "file-service://") {
			return true
		}
	}
	return false
}

func preferOpenAIFileServicePointerInfos(items []openAIImagePointerInfo) []openAIImagePointerInfo {
	if !hasOpenAIFileServicePointerInfos(items) {
		return items
	}
	out := make([]openAIImagePointerInfo, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Pointer, "file-service://") {
			out = append(out, item)
		}
	}
	return out
}

func extractOpenAIImageToolMessages(mapping map[string]any) []openAIImageToolMessage {
	if len(mapping) == 0 {
		return nil
	}
	out := make([]openAIImageToolMessage, 0, 4)
	for messageID, raw := range mapping {
		node, _ := raw.(map[string]any)
		if node == nil {
			continue
		}
		message, _ := node["message"].(map[string]any)
		if message == nil {
			continue
		}
		author, _ := message["author"].(map[string]any)
		metadata, _ := message["metadata"].(map[string]any)
		content, _ := message["content"].(map[string]any)
		if author == nil || metadata == nil || content == nil {
			continue
		}
		if role, _ := author["role"].(string); role != "tool" {
			continue
		}
		if asyncTaskType, _ := metadata["async_task_type"].(string); asyncTaskType != "image_gen" {
			continue
		}
		if contentType, _ := content["content_type"].(string); contentType != "multimodal_text" {
			continue
		}
		prompt := ""
		if title, _ := metadata["image_gen_title"].(string); strings.TrimSpace(title) != "" {
			prompt = strings.TrimSpace(title)
		}
		item := openAIImageToolMessage{MessageID: messageID}
		if createTime, ok := message["create_time"].(float64); ok {
			item.CreateTime = createTime
		}
		parts, _ := content["parts"].([]any)
		for _, part := range parts {
			switch value := part.(type) {
			case map[string]any:
				if assetPointer, _ := value["asset_pointer"].(string); strings.TrimSpace(assetPointer) != "" {
					for _, pointer := range openAIImagePointerMatches([]byte(assetPointer)) {
						item.PointerInfos = append(item.PointerInfos, openAIImagePointerInfo{
							Pointer: pointer,
							Prompt:  prompt,
						})
					}
				}
			case string:
				for _, pointer := range openAIImagePointerMatches([]byte(value)) {
					item.PointerInfos = append(item.PointerInfos, openAIImagePointerInfo{
						Pointer: pointer,
						Prompt:  prompt,
					})
				}
			}
		}
		if len(item.PointerInfos) == 0 {
			continue
		}
		item.PointerInfos = mergeOpenAIImagePointerInfos(nil, item.PointerInfos)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreateTime < out[j].CreateTime
	})
	return out
}

func pollOpenAIImageConversation(ctx context.Context, client *req.Client, headers http.Header, conversationID string) ([]openAIImagePointerInfo, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, nil
	}
	deadline := time.Now().Add(90 * time.Second)
	interval := 3 * time.Second
	previewWait := 15 * time.Second
	var (
		lastErr     error
		firstToolAt time.Time
	)
	for time.Now().Before(deadline) {
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			DisableAutoReadResponse().
			Get(fmt.Sprintf("https://chatgpt.com/backend-api/conversation/%s", conversationID))
		if err != nil {
			lastErr = err
		} else {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				body, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					lastErr = readErr
					goto waitNextPoll
				}
				pointers := mergeOpenAIImagePointerInfos(nil, collectOpenAIImagePointers(body))
				var decoded map[string]any
				if err := json.Unmarshal(body, &decoded); err == nil {
					if mapping, _ := decoded["mapping"].(map[string]any); len(mapping) > 0 {
						toolMessages := extractOpenAIImageToolMessages(mapping)
						if len(toolMessages) > 0 && firstToolAt.IsZero() {
							firstToolAt = time.Now()
						}
						for _, msg := range toolMessages {
							pointers = mergeOpenAIImagePointerInfos(pointers, msg.PointerInfos)
						}
					}
				}
				if hasOpenAIFileServicePointerInfos(pointers) {
					return preferOpenAIFileServicePointerInfos(pointers), nil
				}
				if len(pointers) > 0 && !firstToolAt.IsZero() && time.Since(firstToolAt) >= previewWait {
					return pointers, nil
				}
			} else {
				statusErr := newOpenAIImageStatusError(resp, "conversation poll failed")
				if isOpenAIImageTransientConversationNotFoundError(statusErr) {
					lastErr = statusErr
					goto waitNextPoll
				}
				return nil, statusErr
			}
		}

	waitNextPoll:
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func buildOpenAIImageResponse(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointers []openAIImagePointerInfo,
) ([]byte, int, error) {
	type responseItem struct {
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	}
	items := make([]responseItem, 0, len(pointers))
	for _, pointer := range pointers {
		downloadURL, err := fetchOpenAIImageDownloadURL(ctx, client, headers, conversationID, pointer.Pointer)
		if err != nil {
			return nil, 0, err
		}
		data, err := downloadOpenAIImageBytes(ctx, client, headers, downloadURL)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, responseItem{
			B64JSON:       base64.StdEncoding.EncodeToString(data),
			RevisedPrompt: pointer.Prompt,
		})
	}
	payload := map[string]any{
		"created": time.Now().Unix(),
		"data":    items,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	return body, len(items), nil
}

func fetchOpenAIImageDownloadURL(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer string,
) (string, error) {
	url := ""
	allowConversationRetry := false
	switch {
	case strings.HasPrefix(pointer, "file-service://"):
		fileID := strings.TrimPrefix(pointer, "file-service://")
		url = fmt.Sprintf("%s/%s/download", openAIChatGPTFilesURL, fileID)
	case strings.HasPrefix(pointer, "sediment://"):
		attachmentID := strings.TrimPrefix(pointer, "sediment://")
		url = fmt.Sprintf("https://chatgpt.com/backend-api/conversation/%s/attachment/%s/download", conversationID, attachmentID)
		allowConversationRetry = true
	default:
		return "", fmt.Errorf("unsupported image pointer: %s", pointer)
	}

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		var result struct {
			DownloadURL string `json:"download_url"`
		}
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetSuccessResult(&result).
			Get(url)
		if err != nil {
			lastErr = err
		} else if resp.IsSuccessState() && strings.TrimSpace(result.DownloadURL) != "" {
			return strings.TrimSpace(result.DownloadURL), nil
		} else {
			statusErr := newOpenAIImageStatusError(resp, "fetch image download url failed")
			if !allowConversationRetry || !isOpenAIImageTransientConversationNotFoundError(statusErr) {
				return "", statusErr
			}
			lastErr = statusErr
		}
		if attempt == 7 {
			break
		}
		timer := time.NewTimer(750 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("fetch image download url failed")
	}
	return "", lastErr
}

func downloadOpenAIImageBytes(ctx context.Context, client *req.Client, headers http.Header, downloadURL string) ([]byte, error) {
	request := client.R().
		SetContext(ctx).
		DisableAutoReadResponse()

	if strings.HasPrefix(downloadURL, openAIChatGPTStartURL) {
		downloadHeaders := cloneHTTPHeader(headers)
		downloadHeaders.Set("Accept", "image/*,*/*;q=0.8")
		downloadHeaders.Del("Content-Type")
		request.SetHeaders(headerToMap(downloadHeaders))
	} else {
		userAgent := strings.TrimSpace(headers.Get("User-Agent"))
		if userAgent == "" {
			userAgent = openAIImageBackendUserAgent
		}
		request.SetHeader("User-Agent", userAgent)
	}

	resp, err := request.Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newOpenAIImageStatusError(resp, "download image bytes failed")
	}
	return io.ReadAll(resp.Body)
}

func handleOpenAIImageBackendError(resp *req.Response) error {
	return newOpenAIImageStatusError(resp, "backend-api request failed")
}

type openAIImageStatusError struct {
	StatusCode      int
	Message         string
	ResponseBody    []byte
	ResponseHeaders http.Header
	RequestID       string
	URL             string
}

func (e *openAIImageStatusError) Error() string {
	if e == nil {
		return "openai image backend request failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai image backend request failed: status %d", e.StatusCode)
	}
	return "openai image backend request failed"
}

func newOpenAIImageStatusError(resp *req.Response, fallback string) error {
	if resp == nil {
		if strings.TrimSpace(fallback) == "" {
			fallback = "openai image backend request failed"
		}
		return fmt.Errorf("%s", fallback)
	}

	statusCode := resp.StatusCode
	headers := http.Header(nil)
	requestID := ""
	requestURL := ""
	body := []byte(nil)

	if resp.Response != nil {
		headers = resp.Header.Clone()
		requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
		if resp.Request != nil && resp.Request.URL != nil {
			requestURL = resp.Request.URL.String()
		}
		if resp.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
		}
	}

	message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body))
	if message == "" {
		prefix := strings.TrimSpace(fallback)
		if prefix == "" {
			prefix = "openai image backend request failed"
		}
		message = fmt.Sprintf("%s: status %d", prefix, statusCode)
	}

	return &openAIImageStatusError{
		StatusCode:      statusCode,
		Message:         message,
		ResponseBody:    body,
		ResponseHeaders: headers,
		RequestID:       requestID,
		URL:             requestURL,
	}
}

func newOpenAIImageSyntheticStatusError(statusCode int, message string, requestURL string) *openAIImageStatusError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "openai image backend request failed"
	}
	var body []byte
	if payload, err := json.Marshal(map[string]string{"detail": message}); err == nil {
		body = payload
	}
	return &openAIImageStatusError{
		StatusCode:   statusCode,
		Message:      message,
		ResponseBody: body,
		URL:          strings.TrimSpace(requestURL),
	}
}

func isOpenAIImageTransientConversationNotFoundError(err error) bool {
	statusErr, ok := err.(*openAIImageStatusError)
	if !ok || statusErr == nil || statusErr.StatusCode != http.StatusNotFound {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(statusErr.Message))
	if strings.Contains(msg, "conversation_not_found") {
		return true
	}
	if strings.Contains(msg, "conversation") && strings.Contains(msg, "not found") {
		return true
	}
	bodyMsg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(statusErr.ResponseBody)))
	if strings.Contains(bodyMsg, "conversation_not_found") {
		return true
	}
	return strings.Contains(bodyMsg, "conversation") && strings.Contains(bodyMsg, "not found")
}

func (s *OpenAIGatewayService) wrapOpenAIImageBackendError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	err error,
) error {
	var statusErr *openAIImageStatusError
	if !errors.As(err, &statusErr) || statusErr == nil {
		return err
	}

	upstreamMsg := sanitizeUpstreamErrorMessage(statusErr.Message)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: statusErr.StatusCode,
		UpstreamRequestID:  statusErr.RequestID,
		UpstreamURL:        safeUpstreamURL(statusErr.URL),
		Kind:               "request_error",
		Message:            upstreamMsg,
	})
	setOpsUpstreamError(c, statusErr.StatusCode, upstreamMsg, "")

	if s.shouldFailoverOpenAIUpstreamResponse(statusErr.StatusCode, upstreamMsg, statusErr.ResponseBody) {
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, statusErr.StatusCode, statusErr.ResponseHeaders, statusErr.ResponseBody)
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: statusErr.StatusCode,
			UpstreamRequestID:  statusErr.RequestID,
			UpstreamURL:        safeUpstreamURL(statusErr.URL),
			Kind:               "failover",
			Message:            upstreamMsg,
		})
		retryableOnSameAccount := account.IsPoolMode() && isPoolModeRetryableStatus(statusErr.StatusCode)
		if strings.Contains(strings.ToLower(statusErr.Message), "unsupported challenge") {
			retryableOnSameAccount = false
		}
		return &UpstreamFailoverError{
			StatusCode:             statusErr.StatusCode,
			ResponseBody:           statusErr.ResponseBody,
			RetryableOnSameAccount: retryableOnSameAccount,
		}
	}

	return statusErr
}

func cloneHTTPHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func headerToMap(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	result := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		result[key] = values[0]
	}
	return result
}

func openAITimezoneOffsetMinutes() int {
	_, offset := time.Now().Zone()
	return offset / 60
}

func openAITimezoneName() string {
	return time.Now().Location().String()
}

func generateOpenAIRequirementsToken(userAgent string) string {
	config := []any{
		"core" + strconv.Itoa(3008),
		time.Now().UTC().Format(time.RFC1123),
		nil,
		0.123456,
		coalesceOpenAIFileName(strings.TrimSpace(userAgent), openAIImageBackendUserAgent),
		nil,
		"prod-openai-images",
		"en-US",
		"en-US,en",
		0,
		"navigator.webdriver",
		"location",
		"document.body",
		float64(time.Now().UnixMilli()) / 1000,
		uuid.NewString(),
		"",
		8,
		time.Now().Unix(),
	}
	answer, solved := generateOpenAIChallengeAnswer(strconv.FormatInt(time.Now().UnixNano(), 10), openAIImageRequirementsDiff, config)
	if solved {
		return "gAAAAAC" + answer
	}
	return ""
}

func generateOpenAIChallengeAnswer(seed string, difficulty string, config []any) (string, bool) {
	diffBytes, err := hex.DecodeString(difficulty)
	if err != nil {
		return "", false
	}
	p1 := []byte(jsonCompactSlice(config[:3], true))
	p2 := []byte(jsonCompactSlice(config[4:9], false))
	p3 := []byte(jsonCompactSlice(config[10:], false))
	seedBytes := []byte(seed)

	for i := 0; i < 100000; i++ {
		payload := fmt.Sprintf("%s%d,%s,%d,%s", p1, i, p2, i>>1, p3)
		encoded := base64.StdEncoding.EncodeToString([]byte(payload))
		sum := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytes.Compare(sum[:len(diffBytes)], diffBytes) <= 0 {
			return encoded, true
		}
	}
	return "", false
}

func jsonCompactSlice(values []any, trimSuffixComma bool) string {
	raw, _ := json.Marshal(values)
	text := string(raw)
	if trimSuffixComma {
		return strings.TrimSuffix(text, "]")
	}
	return strings.TrimPrefix(text, "[")
}

func generateOpenAIProofToken(required bool, seed string, difficulty string, userAgent string) string {
	if !required || strings.TrimSpace(seed) == "" || strings.TrimSpace(difficulty) == "" {
		return ""
	}
	screen := 3008
	if len(seed)%2 == 0 {
		screen = 4010
	}
	proofToken := []any{
		screen,
		time.Now().UTC().Format(time.RFC1123),
		nil,
		0,
		coalesceOpenAIFileName(strings.TrimSpace(userAgent), openAIImageBackendUserAgent),
		"https://chatgpt.com/",
		"dpl=openai-images",
		"en",
		"en-US",
		nil,
		"plugins[object PluginArray]",
		"_reactListening",
		"alert",
	}
	diffLen := len(difficulty)
	for i := 0; i < 100000; i++ {
		proofToken[3] = i
		raw, _ := json.Marshal(proofToken)
		encoded := base64.StdEncoding.EncodeToString(raw)
		sum := sha3.Sum512([]byte(seed + encoded))
		if strings.Compare(hex.EncodeToString(sum[:])[:diffLen], difficulty) <= 0 {
			return "gAAAAAB" + encoded
		}
	}
	fallbackBase := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%q", seed)))
	return "gAAAAABwQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + fallbackBase
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
