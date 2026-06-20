package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAICodexImagesGenerationsURL = "https://chatgpt.com/backend-api/codex/images/generations"
	openAICodexImagesEditsURL       = "https://chatgpt.com/backend-api/codex/images/edits"
	openAICodexImagesOriginator     = "codex-tui"
)

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuthDirect(
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
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s endpoint=%s account_type=%s oauth_transport=codex_images uploads=%d",
		requestModel,
		parsed.Endpoint,
		account.Type,
		len(parsed.Uploads),
	)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}

	directBody, err := buildOpenAICodexImagesRequestBody(parsed, requestModel)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAICodexImagesRequest(upstreamCtx, c, account, directBody, token, parsed.Endpoint, parsed.StickySessionSeed())
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
		respBody := s.readUpstreamErrorBody(resp)
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
			s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, requestModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, requestModel)
	}
	defer func() { _ = resp.Body.Close() }()

	writerSizeBeforeResponse := c.Writer.Size()
	usage, imageCount, imageOutputSizes, firstTokenMs, err := s.handleOpenAICodexImagesResponse(upstreamCtx, resp, c, parsed, requestModel, upstreamReq.Header, proxyURL)
	if err != nil {
		return nil, s.handleOpenAIImagesOAuthResponseError(
			upstreamCtx,
			c,
			account,
			requestModel,
			safeUpstreamURL(upstreamReq.URL.String()),
			resp,
			writerSizeBeforeResponse,
			err,
		)
	}
	if imageCount <= 0 {
		imageCount = parsed.N
	}
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            requestModel,
		UpstreamModel:    requestModel,
		Stream:           parsed.Stream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       imageCount,
		ImageSize:        parsed.SizeTier,
		ImageInputSize:   parsed.Size,
		ImageOutputSizes: imageOutputSizes,
	}, nil
}

func buildOpenAICodexImagesRequestBody(parsed *OpenAIImagesRequest, imageModel string) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	body := []byte(`{"model":"","prompt":"","n":1,"output_format":"png"}`)
	body, _ = sjson.SetBytes(body, "model", strings.TrimSpace(imageModel))
	body, _ = sjson.SetBytes(body, "prompt", prompt)
	n := parsed.N
	if n <= 0 {
		n = 1
	}
	body, _ = sjson.SetBytes(body, "n", n)

	outputFormat := strings.TrimSpace(parsed.OutputFormat)
	if outputFormat == "" {
		outputFormat = "png"
	}
	body, _ = sjson.SetBytes(body, "output_format", outputFormat)

	for _, field := range []struct {
		path  string
		value string
	}{
		{path: "size", value: parsed.Size},
		{path: "quality", value: parsed.Quality},
		{path: "background", value: parsed.Background},
		{path: "moderation", value: parsed.Moderation},
	} {
		if trimmed := strings.TrimSpace(field.value); trimmed != "" {
			body, _ = sjson.SetBytes(body, field.path, trimmed)
		}
	}
	if inputFidelity := strings.TrimSpace(parsed.InputFidelity); inputFidelity != "" && openAICodexImagesModelSupportsInputFidelity(imageModel) {
		body, _ = sjson.SetBytes(body, "input_fidelity", inputFidelity)
	}
	if parsed.OutputCompression != nil {
		body, _ = sjson.SetBytes(body, "output_compression", *parsed.OutputCompression)
	}

	if !parsed.IsEdits() {
		return body, nil
	}

	inputImages := make([]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
	for _, imageURL := range parsed.InputImageURLs {
		if trimmed := strings.TrimSpace(imageURL); trimmed != "" {
			inputImages = append(inputImages, trimmed)
		}
	}
	for _, upload := range parsed.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, err
		}
		inputImages = append(inputImages, dataURL)
	}
	if len(inputImages) == 0 {
		return nil, fmt.Errorf("image input is required")
	}

	body, _ = sjson.SetRawBytes(body, "images", []byte(`[]`))
	for _, imageURL := range inputImages {
		item := []byte(`{"image_url":""}`)
		item, _ = sjson.SetBytes(item, "image_url", imageURL)
		body, _ = sjson.SetRawBytes(body, "images.-1", item)
	}

	maskImageURL := strings.TrimSpace(parsed.MaskImageURL)
	if parsed.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*parsed.MaskUpload)
		if err != nil {
			return nil, err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		mask := []byte(`{"image_url":""}`)
		mask, _ = sjson.SetBytes(mask, "image_url", maskImageURL)
		body, _ = sjson.SetRawBytes(body, "mask", mask)
	}
	return body, nil
}

func openAICodexImagesModelSupportsInputFidelity(model string) bool {
	return !strings.EqualFold(strings.TrimSpace(model), "gpt-image-2")
}

func (s *OpenAIGatewayService) buildOpenAICodexImagesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	endpoint string,
	sessionSeed string,
) (*http.Request, error) {
	targetURL := openAICodexImagesGenerationsURL
	if endpoint == openAIImagesEditsEndpoint {
		targetURL = openAICodexImagesEditsURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Originator", openAICodexImagesOriginator)
	req.Header.Set("User-Agent", s.openAICodexImagesUserAgent(ctx, account))
	req.Header.Set("Session_id", generateSessionUUID(isolateOpenAISessionID(getAPIKeyIDFromContext(c), sessionSeed)))
	req.Header.Set("X-Client-Request-Id", uuid.NewString())
	if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
		req.Header.Set("Chatgpt-Account-Id", chatgptAccountID)
	}
	return req, nil
}

func (s *OpenAIGatewayService) openAICodexImagesUserAgent(ctx context.Context, account *Account) string {
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		return customUA
	}
	if s != nil && s.settingService != nil {
		if configured := strings.TrimSpace(s.settingService.GetOpenAICodexUserAgent(ctx)); configured != "" {
			return configured
		}
	}
	return DefaultOpenAICodexUserAgent
}

func (s *OpenAIGatewayService) handleOpenAICodexImagesResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	parsed *OpenAIImagesRequest,
	fallbackModel string,
	upstreamHeaders http.Header,
	proxyURL string,
) (OpenAIUsage, int, []string, *int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, nil, nil, err
	}
	if openAICodexImagesLooksLikeSSE(resp, body) {
		if parsed.Stream {
			replay := *resp
			replay.Body = io.NopCloser(bytes.NewReader(body))
			return s.handleOpenAIImagesOAuthStreamingResponse(&replay, c, time.Now(), parsed.ResponseFormat, openAIImagesStreamPrefix(parsed), fallbackModel)
		}
		usage, imageCount, imageOutputSizes, err := s.handleOpenAICodexImagesSSEBody(resp, c, body, parsed.ResponseFormat, fallbackModel)
		return usage, imageCount, imageOutputSizes, nil, err
	}

	if upstreamErr := openAICodexImagesUpstreamErrorFromJSON(resp, body); upstreamErr != nil {
		setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
		if !IsOpenAIImagesRetryableUpstreamError(upstreamErr) {
			writeOpenAIImagesUpstreamErrorResponse(c, upstreamErr)
		}
		return OpenAIUsage{}, 0, nil, nil, upstreamErr
	}

	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	fallbackMeta := openAIResponsesImageResult{
		OutputFormat: strings.TrimSpace(parsed.OutputFormat),
		Size:         strings.TrimSpace(parsed.Size),
		Background:   strings.TrimSpace(parsed.Background),
		Quality:      strings.TrimSpace(parsed.Quality),
		Model:        strings.TrimSpace(fallbackModel),
	}
	if fallbackMeta.OutputFormat == "" {
		fallbackMeta.OutputFormat = "png"
	}
	results, createdAt, usageRaw, firstMeta, err := collectOpenAICodexImagesDirectBody(ctx, upstreamHeaders, proxyURL, body, fallbackMeta)
	if err != nil {
		return OpenAIUsage{}, 0, nil, nil, err
	}
	if len(results) == 0 {
		imageCount := extractOpenAIImageCountFromJSONBytes(body)
		if imageCount <= 0 {
			return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("upstream did not return image output")
		}
		if parsed.Stream {
			return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("upstream returned unnormalizable image output")
		}
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
		return usage, imageCount, collectOpenAIResponseImageOutputSizesFromJSONBytes(body), nil, nil
	}
	if parsed.Stream {
		firstTokenMs, err := s.writeOpenAICodexImagesSyntheticStreamResponse(resp, c, parsed.ResponseFormat, openAIImagesStreamPrefix(parsed), results, createdAt, usageRaw)
		return usage, len(results), openAIResponsesImageResultSizes(results), firstTokenMs, err
	}

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, usageRaw, firstMeta, parsed.ResponseFormat)
	if err != nil {
		return OpenAIUsage{}, 0, nil, nil, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), openAIResponsesImageResultSizes(results), nil, nil
}

func openAICodexImagesLooksLikeSSE(resp *http.Response, body []byte) bool {
	if resp != nil && isEventStreamResponse(resp.Header) {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:"))
}

func (s *OpenAIGatewayService) handleOpenAICodexImagesSSEBody(
	resp *http.Response,
	c *gin.Context,
	body []byte,
	responseFormat string,
	fallbackModel string,
) (OpenAIUsage, int, []string, error) {
	var usage OpenAIUsage
	forEachOpenAISSEDataPayload(string(body), func(data []byte) {
		s.parseSSEUsageBytes(data, &usage)
	})
	results, createdAt, usageRaw, firstMeta, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	if len(results) == 0 {
		if upstreamErr := extractOpenAIImagesUpstreamError(body); upstreamErr != nil {
			setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
			if !IsOpenAIImagesRetryableUpstreamError(upstreamErr) {
				writeOpenAIImagesUpstreamErrorResponse(c, upstreamErr)
			}
			return OpenAIUsage{}, 0, nil, upstreamErr
		}
		return OpenAIUsage{}, 0, nil, fmt.Errorf("upstream did not return image output")
	}
	if strings.TrimSpace(firstMeta.Model) == "" {
		firstMeta.Model = strings.TrimSpace(fallbackModel)
	}

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, usageRaw, firstMeta, responseFormat)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), openAIResponsesImageResultSizes(results), nil
}

func openAICodexImagesUpstreamErrorFromJSON(resp *http.Response, body []byte) *OpenAIImagesUpstreamError {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	errorObj := gjson.GetBytes(body, "error")
	if !errorObj.Exists() {
		return nil
	}
	requestID := ""
	if resp != nil {
		requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
	}
	upstreamErr := openAIImagesUpstreamErrorFromGJSON(errorObj, requestID)
	if upstreamErr == nil {
		return nil
	}
	if upstreamErr.StatusCode < http.StatusBadRequest {
		upstreamErr.StatusCode = http.StatusBadGateway
	}
	return upstreamErr
}

func (s *OpenAIGatewayService) writeOpenAICodexImagesSyntheticStreamResponse(
	resp *http.Response,
	c *gin.Context,
	responseFormat string,
	streamPrefix string,
	results []openAIResponsesImageResult,
	createdAt int64,
	usageRaw []byte,
) (*int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming is not supported by response writer")
	}

	firstTokenMs := 0
	eventName := streamPrefix + ".completed"
	for _, img := range results {
		payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, responseFormat, createdAt, usageRaw)
		if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); err != nil {
			return &firstTokenMs, err
		}
	}
	if _, err := io.WriteString(c.Writer, "data: [DONE]\n\n"); err != nil {
		return &firstTokenMs, err
	}
	flusher.Flush()
	return &firstTokenMs, nil
}

func collectOpenAICodexImagesDirectBody(ctx context.Context, headers http.Header, proxyURL string, body []byte, fallbackMeta openAIResponsesImageResult) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, 0, nil, openAIResponsesImageResult{}, fmt.Errorf("upstream returned invalid image JSON")
	}
	root := gjson.ParseBytes(body)
	createdAt := root.Get("created").Int()
	if createdAt <= 0 {
		createdAt = root.Get("created_at").Int()
	}

	var usageRaw []byte
	if usage := root.Get("usage"); usage.Exists() && usage.IsObject() {
		usageRaw = []byte(usage.Raw)
	} else if usage := root.Get("tool_usage.image_gen"); usage.Exists() && usage.IsObject() {
		usageRaw = []byte(usage.Raw)
	}

	firstMeta := openAIResponsesImageResult{
		OutputFormat: strings.TrimSpace(root.Get("output_format").String()),
		Size:         strings.TrimSpace(root.Get("size").String()),
		Background:   strings.TrimSpace(root.Get("background").String()),
		Quality:      strings.TrimSpace(root.Get("quality").String()),
		Model:        strings.TrimSpace(root.Get("model").String()),
	}
	fillOpenAICodexImagesMeta(&firstMeta, fallbackMeta)

	results := make([]openAIResponsesImageResult, 0)
	seen := make(map[string]struct{})
	var downloadClient *req.Client
	var collectErr error
	appendDirectItemResults := func(items gjson.Result) {
		if !items.IsArray() || collectErr != nil {
			return
		}
		for _, item := range items.Array() {
			result, detectedFormat, resultErr := openAICodexImagesResultB64FromItem(ctx, &downloadClient, headers, proxyURL, item)
			if resultErr != nil {
				collectErr = resultErr
				return
			}
			if result == "" {
				continue
			}
			entry := openAIResponsesImageResult{
				Result:        result,
				RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
				OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
				Size:          strings.TrimSpace(item.Get("size").String()),
				Background:    strings.TrimSpace(item.Get("background").String()),
				Quality:       strings.TrimSpace(item.Get("quality").String()),
				Model:         strings.TrimSpace(item.Get("model").String()),
			}
			if entry.OutputFormat == "" {
				entry.OutputFormat = detectedFormat
			}
			fillOpenAICodexImagesMeta(&entry, firstMeta)
			appendOpenAIResponsesImageResultDedup(&results, seen, "", entry)
		}
	}
	appendDirectItemResults(root.Get("data"))
	appendDirectItemResults(root.Get("output"))
	appendDirectItemResults(root.Get("response.output"))
	if collectErr != nil {
		return nil, 0, nil, openAIResponsesImageResult{}, collectErr
	}
	return results, createdAt, usageRaw, firstMeta, nil
}

func fillOpenAICodexImagesMeta(dst *openAIResponsesImageResult, fallback openAIResponsesImageResult) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(dst.OutputFormat) == "" {
		dst.OutputFormat = strings.TrimSpace(fallback.OutputFormat)
	}
	if strings.TrimSpace(dst.Size) == "" {
		dst.Size = strings.TrimSpace(fallback.Size)
	}
	if strings.TrimSpace(dst.Background) == "" {
		dst.Background = strings.TrimSpace(fallback.Background)
	}
	if strings.TrimSpace(dst.Quality) == "" {
		dst.Quality = strings.TrimSpace(fallback.Quality)
	}
	if strings.TrimSpace(dst.Model) == "" {
		dst.Model = strings.TrimSpace(fallback.Model)
	}
}

func openAICodexImagesResultB64FromItem(ctx context.Context, downloadClient **req.Client, headers http.Header, proxyURL string, item gjson.Result) (string, string, error) {
	for _, path := range []string{"b64_json", "result"} {
		if result := strings.TrimSpace(item.Get(path).String()); result != "" {
			if b64, format := openAICodexImagesDataURLToB64(result); b64 != "" {
				return b64, format, nil
			}
			return result, "", nil
		}
	}
	for _, path := range []string{"url", "image_url"} {
		value := strings.TrimSpace(item.Get(path).String())
		if b64, format := openAICodexImagesDataURLToB64(value); b64 != "" {
			return b64, format, nil
		}
		if openAICodexImagesIsHTTPURL(value) {
			if downloadClient != nil && *downloadClient == nil {
				client := req.C()
				if trimmedProxy := strings.TrimSpace(proxyURL); trimmedProxy != "" {
					client.SetProxyURL(trimmedProxy)
				}
				*downloadClient = client
			}
			if downloadClient == nil || *downloadClient == nil {
				return "", "", fmt.Errorf("image URL downloader is unavailable")
			}
			imageBytes, err := downloadOpenAIImageBytes(ctx, *downloadClient, headers, value, openAIUpstreamErrorBodyReadLimit)
			if err != nil {
				return "", "", err
			}
			format := openAICodexImagesOutputFormatFromURL(value)
			if format == "" {
				detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(imageBytes)))
				if strings.HasPrefix(detected, "image/") {
					format = strings.TrimPrefix(detected, "image/")
				}
			}
			return base64.StdEncoding.EncodeToString(imageBytes), format, nil
		}
	}
	return "", "", nil
}

func openAICodexImagesIsHTTPURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func openAICodexImagesOutputFormatFromURL(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, suffix := range []struct {
		needle string
		format string
	}{
		{needle: ".png", format: "png"},
		{needle: ".jpg", format: "jpeg"},
		{needle: ".jpeg", format: "jpeg"},
		{needle: ".webp", format: "webp"},
	} {
		if strings.Contains(lower, suffix.needle) {
			return suffix.format
		}
	}
	return ""
}

func openAICodexImagesDataURLToB64(value string) (string, string) {
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return "", ""
	}
	comma := strings.Index(value, ",")
	if comma < 0 {
		return "", ""
	}
	meta := value[:comma]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return "", ""
	}
	mimeType := strings.TrimPrefix(strings.Split(meta, ";")[0], "data:")
	format := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/")
	return strings.TrimSpace(value[comma+1:]), format
}
