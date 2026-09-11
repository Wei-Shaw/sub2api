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

// forwardOpenAIImagesOAuthDirect keeps native image requests out of the
// Responses image_generation bridge. The bridge is a text-oriented transport
// and the Codex backend may silently replace native image controls there.
func (s *OpenAIGatewayService) forwardOpenAIImagesOAuthDirect(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	beginUpstreamResponseModelObservation(c)

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
	upstreamModel := account.GetMappedModel(requestModel)
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		return nil, err
	}

	targetEndpoint := openAICodexImagesGenerationsURL
	if parsed.IsEdits() {
		targetEndpoint = openAICodexImagesEditsURL
	}
	SetActualOpenAIUpstreamEndpoint(c, codexImagesEndpointPath(targetEndpoint))
	SetOpsUpstreamModel(c, upstreamModel)
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
	upstreamCtx = withOpenAIImagesSelfBuiltRequest(upstreamCtx)

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	directBody, err := buildOpenAICodexImagesRequestBody(parsed, upstreamModel)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAICodexImagesRequest(
		upstreamCtx,
		c,
		account,
		directBody,
		token,
		targetEndpoint,
		parsed.StickySessionSeed(),
	)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
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
	if resp == nil {
		return nil, fmt.Errorf("upstream returned an empty response")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			return s.forwardOpenAIImagesOAuth(withOpenAIImagesForceResponses(ctx), c, account, parsed, channelMappedModel)
		}
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				ProxyID:            opsUpstreamProxyID(account),
				ProxyName:          opsUpstreamProxyName(account),
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			shouldDisable := s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamModel)
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
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, upstreamModel)
	}
	defer func() { _ = resp.Body.Close() }()

	writerSizeBeforeResponse := OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
	usage, imageCount, imageOutputSizes, firstTokenMs, err := s.handleOpenAICodexImagesResponse(
		upstreamCtx,
		resp,
		c,
		parsed,
		upstreamModel,
		upstreamReq.Header,
		proxyURL,
	)
	if err != nil {
		if imageCount > 0 {
			return &OpenAIForwardResult{
				RequestID:                     resp.Header.Get("x-request-id"),
				UpstreamHeaders:               resp.Header,
				Usage:                         usage,
				Model:                         requestModel,
				UpstreamModel:                 upstreamModel,
				UpstreamResponseModel:         observedUpstreamResponseModel(c),
				UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
				UpstreamEndpoint:              codexImagesEndpointPath(targetEndpoint),
				Stream:                        parsed.Stream,
				ResponseHeaders:               resp.Header.Clone(),
				Duration:                      time.Since(startTime),
				FirstTokenMs:                  firstTokenMs,
				ImageCount:                    imageCount,
				ImageSize:                     parsed.SizeTier,
				ImageInputSize:                parsed.Size,
				ImageOutputSizes:              imageOutputSizes,
			}, err
		}
		return nil, s.handleOpenAIImagesOAuthResponseError(
			upstreamCtx,
			c,
			account,
			upstreamModel,
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
		RequestID:                     resp.Header.Get("x-request-id"),
		UpstreamHeaders:               resp.Header.Clone(),
		Usage:                         usage,
		Model:                         requestModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamEndpoint:              codexImagesEndpointPath(targetEndpoint),
		Stream:                        parsed.Stream,
		ResponseHeaders:               resp.Header.Clone(),
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
		ImageCount:                    imageCount,
		ImageSize:                     parsed.SizeTier,
		ImageInputSize:                parsed.Size,
		ImageOutputSizes:              imageOutputSizes,
	}, nil
}

func codexImagesEndpointPath(endpoint string) string {
	if parsed, err := http.NewRequest(http.MethodPost, endpoint, nil); err == nil && parsed.URL != nil {
		return parsed.URL.Path
	}
	return endpoint
}

func buildOpenAICodexImagesRequestBody(parsed *OpenAIImagesRequest, imageModel string) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if !parsed.Multipart && gjson.ValidBytes(parsed.Body) {
		if rawPrompt := gjson.GetBytes(parsed.Body, "prompt").String(); rawPrompt != "" {
			prompt = rawPrompt
		}
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
	if inputFidelity := strings.TrimSpace(parsed.InputFidelity); inputFidelity != "" &&
		!strings.EqualFold(strings.TrimSpace(imageModel), "gpt-image-2") {
		body, _ = sjson.SetBytes(body, "input_fidelity", inputFidelity)
	}
	if parsed.OutputCompression != nil {
		body, _ = sjson.SetBytes(body, "output_compression", *parsed.OutputCompression)
	}
	if parsed.PartialImages != nil {
		body, _ = sjson.SetBytes(body, "partial_images", *parsed.PartialImages)
	}
	if parsed.Stream {
		body, _ = sjson.SetBytes(body, "stream", true)
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

func (s *OpenAIGatewayService) buildOpenAICodexImagesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	targetURL string,
	sessionSeed string,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI))

	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	request.Host = "chatgpt.com"
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, request.Header, account); err != nil {
		return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if gjson.GetBytes(body, "stream").Bool() {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	request.Header.Set("Connection", "Keep-Alive")
	request.Header.Set("Originator", openAICodexImagesOriginator)
	request.Header.Set("User-Agent", s.openAICodexImagesUserAgent(ctx, account))
	request.Header.Set(
		"Session_id",
		generateSessionUUID(isolateOpenAIUpstreamSessionID(
			getAPIKeyIDFromContext(c),
			codexAccountIdentitySource(c, account),
			sessionSeed,
		)),
	)
	request.Header.Set("X-Client-Request-Id", uuid.NewString())
	applyCodexAccountIdentityHeaders(request.Header, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
	enforceCodexIdentityHeadersWithUA(request.Header, s.codexIdentityOverrideUA(account))
	setOpenAICodexRoutingHintFromBody(request.Header, account, body)
	account.ApplyHeaderOverrides(request.Header)
	return request, nil
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
		replay := *resp
		replay.Body = io.NopCloser(bytes.NewReader(body))
		if parsed.Stream && isOpenAICodexNativeImageSSE(body) {
			return s.handleOpenAIImagesStreamingResponse(
				&replay,
				c,
				time.Now(),
				parsed,
			)
		}
		if parsed.Stream {
			return s.handleOpenAIImagesOAuthStreamingResponseWithValidation(
				&replay,
				c,
				time.Now(),
				parsed.ResponseFormat,
				openAIImagesStreamPrefix(parsed),
				fallbackModel,
				parsed,
			)
		}
		usage, count, sizes, err := s.handleOpenAIImagesOAuthNonStreamingResponseWithValidation(
			&replay,
			c,
			parsed.ResponseFormat,
			fallbackModel,
			parsed,
		)
		return usage, count, sizes, nil, err
	}

	if upstreamErr := openAICodexImagesUpstreamErrorFromJSON(resp, body); upstreamErr != nil {
		setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
		if !IsOpenAIImagesRetryableUpstreamError(upstreamErr) {
			writeOpenAIImagesUpstreamErrorResponse(c, upstreamErr)
		}
		return OpenAIUsage{}, 0, nil, nil, upstreamErr
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("upstream returned invalid image JSON")
	}

	root := gjson.ParseBytes(body)
	observeOpenAICodexImagesResponseModel(c, root)
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
	results, firstMeta, err := collectOpenAICodexImagesDirectJSON(ctx, upstreamHeaders, proxyURL, root, fallbackMeta)
	if err != nil {
		return OpenAIUsage{}, 0, nil, nil, err
	}
	if len(results) == 0 {
		return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("upstream did not return image output")
	}
	if mismatchErr := validateOpenAICodexImagesResponse(
		parsed,
		results,
		collectOpenAICodexImagesObservedMeta(root),
	); mismatchErr != nil {
		reportOpenAICodexImagesResponseMismatch(c, mismatchErr)
		return OpenAIUsage{}, 0, nil, nil, mismatchErr
	}
	for i := range results {
		normalizeOpenAIResponsesImageClientModel(&results[i], fallbackModel)
	}
	firstMeta.Model = strings.TrimSpace(fallbackModel)
	normalizeOpenAIResponsesImageClientModel(&firstMeta, fallbackModel)

	usage, ok := codexDirectImagesUsage(body)
	if !ok {
		usage, _ = extractOpenAIUsageFromJSONBytes(body)
	}
	var usageRaw []byte
	if rawUsage := root.Get("usage"); rawUsage.Exists() && rawUsage.IsObject() {
		usageRaw = []byte(rawUsage.Raw)
	}
	createdAt := root.Get("created").Int()
	if createdAt <= 0 {
		createdAt = root.Get("created_at").Int()
	}
	if parsed.Stream {
		firstTokenMs, streamErr := s.writeOpenAICodexImagesSyntheticStreamResponse(
			resp,
			c,
			parsed.ResponseFormat,
			openAIImagesStreamPrefix(parsed),
			results,
			createdAt,
			usageRaw,
		)
		return usage, len(results), openAIResponsesImageResultSizes(results), firstTokenMs, streamErr
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

func isOpenAICodexNativeImageSSE(body []byte) bool {
	return bytes.Contains(body, []byte(`"type":"image_generation.`)) ||
		bytes.Contains(body, []byte(`"type":"image_edit.`))
}

func observeOpenAICodexImagesResponseModel(c *gin.Context, root gjson.Result) {
	model := strings.TrimSpace(root.Get("model").String())
	if model == "" {
		model = strings.TrimSpace(root.Get("response.model").String())
	}
	if model == "" {
		model = strings.TrimSpace(root.Get("data.0.model").String())
	}
	if observer := upstreamResponseModelObserverFromContext(c); observer != nil {
		observer.Observe(model, true)
	}
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
	if upstreamErr != nil && upstreamErr.StatusCode < http.StatusBadRequest {
		upstreamErr.StatusCode = http.StatusBadGateway
	}
	return upstreamErr
}

func collectOpenAICodexImagesDirectJSON(
	ctx context.Context,
	headers http.Header,
	proxyURL string,
	root gjson.Result,
	fallbackMeta openAIResponsesImageResult,
) ([]openAIResponsesImageResult, openAIResponsesImageResult, error) {
	firstMeta := openAIResponsesImageResult{
		OutputFormat: strings.TrimSpace(root.Get("output_format").String()),
		Size:         strings.TrimSpace(root.Get("size").String()),
		Background:   strings.TrimSpace(root.Get("background").String()),
		Quality:      strings.TrimSpace(root.Get("quality").String()),
		Model:        strings.TrimSpace(root.Get("model").String()),
	}
	fillOpenAICodexImagesMeta(&firstMeta, fallbackMeta)

	var downloader *req.Client
	results := make([]openAIResponsesImageResult, 0)
	seen := make(map[string]struct{})
	var collectErr error
	appendItems := func(items gjson.Result) {
		if !items.IsArray() || collectErr != nil {
			return
		}
		for _, item := range items.Array() {
			result, detectedFormat, err := openAICodexImagesResultB64FromItem(
				ctx,
				&downloader,
				headers,
				proxyURL,
				item,
			)
			if err != nil {
				collectErr = err
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
	appendItems(root.Get("data"))
	appendItems(root.Get("output"))
	appendItems(root.Get("response.output"))
	if collectErr != nil {
		return nil, openAIResponsesImageResult{}, collectErr
	}
	return results, firstMeta, nil
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

func collectOpenAICodexImagesObservedMeta(root gjson.Result) []openAIResponsesImageResult {
	observed := make([]openAIResponsesImageResult, 0, 2)
	appendMeta := func(value gjson.Result) {
		if !value.Exists() {
			return
		}
		meta := openAIResponsesImageResult{
			OutputFormat: strings.TrimSpace(value.Get("output_format").String()),
			Size:         strings.TrimSpace(value.Get("size").String()),
			Background:   strings.TrimSpace(value.Get("background").String()),
			Quality:      strings.TrimSpace(value.Get("quality").String()),
			Model:        strings.TrimSpace(value.Get("model").String()),
		}
		if meta.OutputFormat != "" || meta.Size != "" || meta.Background != "" || meta.Quality != "" || meta.Model != "" {
			observed = append(observed, meta)
		}
	}
	appendItems := func(items gjson.Result) {
		if !items.IsArray() {
			return
		}
		for _, item := range items.Array() {
			appendMeta(item)
		}
	}

	appendMeta(root)
	appendItems(root.Get("data"))
	appendItems(root.Get("output"))
	appendItems(root.Get("response.output"))
	return observed
}

func validateOpenAICodexImagesResponse(
	parsed *OpenAIImagesRequest,
	results []openAIResponsesImageResult,
	observed []openAIResponsesImageResult,
) *OpenAIImagesUpstreamError {
	if parsed == nil {
		return nil
	}

	requestedSize := strings.TrimSpace(parsed.Size)
	if requestedSize != "" && !strings.EqualFold(requestedSize, "auto") {
		for _, result := range results {
			actualSize := detectOpenAIImageResultSize(result.Result)
			if actualSize == "" {
				continue
			}
			if !strings.EqualFold(actualSize, requestedSize) {
				return newOpenAICodexImagesResponseMismatchError("size", requestedSize, actualSize)
			}
		}
	}

	requestedOptions := []struct {
		param    string
		request  string
		observed func(openAIResponsesImageResult) string
	}{
		{
			param:    "quality",
			request:  parsed.Quality,
			observed: func(meta openAIResponsesImageResult) string { return meta.Quality },
		},
		{
			param:    "background",
			request:  parsed.Background,
			observed: func(meta openAIResponsesImageResult) string { return meta.Background },
		},
		{
			param:   "output_format",
			request: parsed.OutputFormat,
			observed: func(meta openAIResponsesImageResult) string {
				return meta.OutputFormat
			},
		},
	}
	for _, option := range requestedOptions {
		requested := strings.TrimSpace(option.request)
		if requested == "" {
			continue
		}
		if strings.EqualFold(requested, "auto") {
			continue
		}
		for _, meta := range observed {
			seen := strings.TrimSpace(option.observed(meta))
			if seen == "" {
				continue
			}
			if !openAICodexImagesOptionsEqual(option.param, requested, seen) {
				return newOpenAICodexImagesResponseMismatchError(option.param, requested, seen)
			}
		}
	}
	return nil
}

func openAICodexImagesOptionsEqual(param, requested, observed string) bool {
	requested = strings.ToLower(strings.TrimSpace(requested))
	observed = strings.ToLower(strings.TrimSpace(observed))
	if param == "output_format" {
		if requested == "jpg" {
			requested = "jpeg"
		}
		if observed == "jpg" {
			observed = "jpeg"
		}
	}
	return requested == observed
}

func newOpenAICodexImagesResponseMismatchError(param, requested, observed string) *OpenAIImagesUpstreamError {
	return &OpenAIImagesUpstreamError{
		StatusCode:   http.StatusBadGateway,
		ErrorType:    "upstream_response_mismatch",
		Code:         "image_generation_parameters_mismatch",
		Message:      fmt.Sprintf("upstream image response did not honor requested %s %q; observed %q", param, requested, observed),
		Param:        param,
		NonRetryable: true,
	}
}

func reportOpenAICodexImagesResponseMismatch(c *gin.Context, err *OpenAIImagesUpstreamError) {
	if err == nil {
		return
	}
	setOpsUpstreamError(c, err.clientStatusCode(), err.clientMessage(), "")
	writeOpenAIImagesUpstreamErrorResponse(c, err)
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

func openAICodexImagesResultB64FromItem(
	ctx context.Context,
	downloadClient **req.Client,
	headers http.Header,
	proxyURL string,
	item gjson.Result,
) (string, string, error) {
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
		if !openAICodexImagesIsHTTPURL(value) {
			continue
		}
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
