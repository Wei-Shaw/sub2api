package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIImagesGenerationsEndpoint = "/v1/images/generations"
	openAIImagesEditsEndpoint       = "/v1/images/edits"

	openAIImagesGenerationsURL = "https://api.openai.com/v1/images/generations"
	openAIImagesEditsURL       = "https://api.openai.com/v1/images/edits"

	openAIImageMaxUploadPartSize = 20 << 20 // 20MB per multipart upload part
	openAIImagesResponsesMainModel = "gpt-5.4-mini"
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
	Quality            string
	Background         string
	OutputFormat       string
	Moderation         string
	InputFidelity      string
	Style              string
	OutputCompression  *int
	PartialImages      *int
	HasMask            bool
	HasNativeOptions   bool
	RequiredCapability OpenAIImagesCapability
	InputImageURLs     []string
	MaskImageURL       string
	Uploads            []OpenAIImagesUpload
	MaskUpload         *OpenAIImagesUpload
	Body               []byte
	bodyHash           string
}

func (u OpenAIImagesUpload) ModerationDataURL() string {
	if len(u.Data) == 0 {
		return ""
	}
	contentType := strings.TrimSpace(u.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(u.Data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return ""
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(u.Data))
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
	if err := validateOpenAIImagesModel(req.Model); err != nil {
		return nil, err
	}
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
	req.Quality = strings.TrimSpace(gjson.GetBytes(body, "quality").String())
	req.Background = strings.TrimSpace(gjson.GetBytes(body, "background").String())
	req.OutputFormat = strings.TrimSpace(gjson.GetBytes(body, "output_format").String())
	req.Moderation = strings.TrimSpace(gjson.GetBytes(body, "moderation").String())
	req.InputFidelity = strings.TrimSpace(gjson.GetBytes(body, "input_fidelity").String())
	req.Style = strings.TrimSpace(gjson.GetBytes(body, "style").String())
	req.HasMask = gjson.GetBytes(body, "mask").Exists()
	if outputCompression := gjson.GetBytes(body, "output_compression"); outputCompression.Exists() {
		if outputCompression.Type != gjson.Number {
			return fmt.Errorf("invalid output_compression field type")
		}
		v := int(outputCompression.Int())
		req.OutputCompression = &v
	}
	if partialImages := gjson.GetBytes(body, "partial_images"); partialImages.Exists() {
		if partialImages.Type != gjson.Number {
			return fmt.Errorf("invalid partial_images field type")
		}
		v := int(partialImages.Int())
		req.PartialImages = &v
	}
	if req.IsEdits() {
		images := gjson.GetBytes(body, "images")
		if images.Exists() {
			if !images.IsArray() {
				return fmt.Errorf("invalid images field type")
			}
			for _, item := range images.Array() {
				if imageURL := strings.TrimSpace(item.Get("image_url").String()); imageURL != "" {
					req.InputImageURLs = append(req.InputImageURLs, imageURL)
					continue
				}
				if item.Get("file_id").Exists() {
					return fmt.Errorf("images[].file_id is not supported (use images[].image_url instead)")
				}
			}
		}
		if maskImageURL := strings.TrimSpace(gjson.GetBytes(body, "mask.image_url").String()); maskImageURL != "" {
			req.MaskImageURL = maskImageURL
			req.HasMask = true
		}
		if gjson.GetBytes(body, "mask.file_id").Exists() {
			return fmt.Errorf("mask.file_id is not supported (use mask.image_url instead)")
		}
		if len(req.InputImageURLs) == 0 {
			return fmt.Errorf("images[].image_url is required")
		}
	}
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

		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if err != nil {
			return fmt.Errorf("read multipart field %s: %w", name, err)
		}

		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
			if name == "mask" && len(data) > 0 {
				req.HasMask = true
				width, height := parseOpenAIImageDimensions(part.Header)
				maskUpload := OpenAIImagesUpload{
					FieldName:   name,
					FileName:    fileName,
					ContentType: partContentType,
					Data:        data,
					Width:       width,
					Height:      height,
				}
				req.MaskUpload = &maskUpload
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
		case "quality":
			req.Quality = value
			req.HasNativeOptions = true
		case "background":
			req.Background = value
			req.HasNativeOptions = true
		case "output_format":
			req.OutputFormat = value
			req.HasNativeOptions = true
		case "moderation":
			req.Moderation = value
			req.HasNativeOptions = true
		case "input_fidelity":
			req.InputFidelity = value
			req.HasNativeOptions = true
		case "style":
			req.Style = value
			req.HasNativeOptions = true
		case "output_compression":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid output_compression field value")
			}
			req.OutputCompression = &n
			req.HasNativeOptions = true
		case "partial_images":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid partial_images field value")
			}
			req.PartialImages = &n
			req.HasNativeOptions = true
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

func isOpenAIImageGenerationModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-")
}

func validateOpenAIImagesModel(model string) error {
	model = strings.TrimSpace(model)
	if isOpenAIImageGenerationModel(model) {
		return nil
	}
	if model == "" {
		return fmt.Errorf("images endpoint requires an image model")
	}
	return fmt.Errorf("images endpoint requires an image model, got %q", model)
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
		"input_fidelity",
		"partial_images",
	} {
		if exists(path) {
			return true
		}
	}
	return false
}

func isOpenAINativeImageOption(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "background", "quality", "style", "output_format", "output_compression", "moderation", "input_fidelity", "partial_images":
		return true
	default:
		return false
	}
}

func normalizeOpenAIImageSizeTier(size string) string {
	return NormalizeImageBillingTierOrDefault(size)
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
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	upstreamModel := account.GetMappedModel(requestModel)
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s upstream_model=%s endpoint=%s account_type=%s",
		strings.TrimSpace(parsed.Model),
		upstreamModel,
		parsed.Endpoint,
		account.Type,
	)
	forwardBody, forwardContentType, err := rewriteOpenAIImagesModel(body, parsed.ContentType, upstreamModel)
	if err != nil {
		return nil, err
	}
	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, parsed.Stream)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(upstreamCtx, c, account, forwardBody, forwardContentType, token, parsed.Endpoint)
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
			s.handleFailoverSideEffects(upstreamCtx, resp, account, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(upstreamCtx, resp, c, account, forwardBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var usage OpenAIUsage
	imageCount := parsed.N
	var firstTokenMs *int
	if parsed.Stream && isEventStreamResponse(resp.Header) {
		streamUsage, streamCount, streamSizes, ttft, err := s.handleOpenAIImagesStreamingResponse(resp, c, startTime)
		if err != nil {
			if streamCount > 0 {
				return &OpenAIForwardResult{
					RequestID:        resp.Header.Get("x-request-id"),
					Usage:            streamUsage,
					Model:            requestModel,
					UpstreamModel:    upstreamModel,
					Stream:           parsed.Stream,
					ResponseHeaders:  resp.Header.Clone(),
					Duration:         time.Since(startTime),
					FirstTokenMs:     ttft,
					ImageCount:       streamCount,
					ImageSize:        parsed.SizeTier,
					ImageInputSize:   parsed.Size,
					ImageOutputSizes: streamSizes,
				}, err
			}
			return nil, err
		}
		usage = streamUsage
		imageCount = streamCount
		imageOutputSizes := streamSizes
		firstTokenMs = ttft
		return &OpenAIForwardResult{
			RequestID:        resp.Header.Get("x-request-id"),
			Usage:            usage,
			Model:            requestModel,
			UpstreamModel:    upstreamModel,
			Stream:           parsed.Stream,
			ResponseHeaders:  resp.Header.Clone(),
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ImageCount:       imageCount,
			ImageSize:        parsed.SizeTier,
			ImageInputSize:   parsed.Size,
			ImageOutputSizes: imageOutputSizes,
		}, nil
	} else {
		nonStreamUsage, nonStreamCount, nonStreamSizes, err := s.handleOpenAIImagesNonStreamingResponse(resp, c)
		if err != nil {
			return nil, err
		}
		usage = nonStreamUsage
		if nonStreamCount > 0 {
			imageCount = nonStreamCount
		}
		return &OpenAIForwardResult{
			RequestID:        resp.Header.Get("x-request-id"),
			Usage:            usage,
			Model:            requestModel,
			UpstreamModel:    upstreamModel,
			Stream:           parsed.Stream,
			ResponseHeaders:  resp.Header.Clone(),
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ImageCount:       imageCount,
			ImageSize:        parsed.SizeTier,
			ImageInputSize:   parsed.Size,
			ImageOutputSizes: nonStreamSizes,
		}, nil
	}
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
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
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
	return buildOpenAIEndpointURL(base, endpoint)
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

func (s *OpenAIGatewayService) handleOpenAIImagesNonStreamingResponse(resp *http.Response, c *gin.Context) (OpenAIUsage, int, []string, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
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
	return usage, extractOpenAIImageCountFromJSONBytes(body), collectOpenAIResponseImageOutputSizesFromJSONBytes(body), nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
) (OpenAIUsage, int, []string, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", contentType)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("streaming is not supported by response writer")
	}

	usage := OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	clientDisconnected := false
	lastDownstreamWriteAt := time.Now()
	var fallbackBody bytes.Buffer
	fallbackBytes := int64(0)
	fallbackLimit := resolveUpstreamResponseReadLimit(s.cfg)
	seenSSEData := false
	fallbackTooLarge := false
	var sseData openAISSEDataAccumulator

	processSSEData := func(dataBytes []byte) {
		seenSSEData = true
		fallbackBody.Reset()
		fallbackBytes = 0
		mergeOpenAIUsage(&usage, dataBytes)
		imageCounter.AddSSEData(dataBytes)
	}

	flushSSEEvent := func() {
		sseData.Flush(processSSEData)
	}

	processLine := func(line []byte) {
		if len(line) == 0 {
			return
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		if !clientDisconnected {
			if _, writeErr := c.Writer.Write(line); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected, continue draining upstream for billing")
			} else {
				flusher.Flush()
				lastDownstreamWriteAt = time.Now()
			}
		}

		trimmedLine := strings.TrimRight(string(line), "\r\n")
		if _, ok := extractOpenAISSEDataLine(trimmedLine); ok || strings.TrimSpace(trimmedLine) == "" {
			sseData.AddLine(trimmedLine, processSSEData)
			return
		}
		if !seenSSEData && !fallbackTooLarge {
			fallbackBytes += int64(len(line))
			if fallbackBytes <= fallbackLimit {
				_, _ = fallbackBody.Write(line)
			} else {
				fallbackTooLarge = true
				fallbackBody.Reset()
			}
		}
	}

	finalizeFallbackBody := func() {
		if seenSSEData || fallbackBody.Len() == 0 {
			return
		}
		body := bytes.TrimSpace(fallbackBody.Bytes())
		if len(body) == 0 {
			return
		}
		mergeOpenAIUsage(&usage, body)
		imageCounter.AddJSONResponse(body)
	}

	streamInterval := s.openAIImageStreamDataInterval()
	keepaliveInterval := s.openAIImageStreamKeepaliveInterval()
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			processLine(line)
			if err == io.EOF {
				break
			}
			if err != nil {
				flushSSEEvent()
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, err
			}
		}
		flushSSEEvent()
		finalizeFallbackBody()
		return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, nil
	}

	type readEvent struct {
		line []byte
		err  error
	}
	events := make(chan readEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev readEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			}
			if len(line) > 0 && !sendEvent(readEvent{line: line}) {
				return
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				_ = sendEvent(readEvent{err: err})
				return
			}
		}
	}()
	defer close(done)

	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				flushSSEEvent()
				finalizeFallbackBody()
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, nil
			}
			if ev.err != nil {
				flushSSEEvent()
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, ev.err
			}
			processLine(ev.line)
		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, fmt.Errorf("image stream incomplete after timeout")
			}
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream data interval timeout: interval=%s", streamInterval)
			_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(fmt.Sprintf("upstream image stream idle for %s", streamInterval)))
			return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, fmt.Errorf("image stream data interval timeout")
		case <-keepaliveCh:
			if clientDisconnected || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			if _, writeErr := io.WriteString(c.Writer, ":\n\n"); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected during keepalive, continue draining upstream for billing")
				continue
			}
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
		}
	}
}

func (s *OpenAIGatewayService) openAIImageStreamDataInterval() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.ImageStreamDataIntervalTimeout <= 0 {
		return 0
	}
	return time.Duration(s.cfg.Gateway.ImageStreamDataIntervalTimeout) * time.Second
}

func (s *OpenAIGatewayService) openAIImageStreamKeepaliveInterval() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.ImageStreamKeepaliveInterval <= 0 {
		return 0
	}
	return time.Duration(s.cfg.Gateway.ImageStreamKeepaliveInterval) * time.Second
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
	return countOpenAIResponseImageOutputsFromJSONBytes(body)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
