package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
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
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIImagesGenerationsEndpoint = "/v1/images/generations"
	openAIImagesEditsEndpoint       = "/v1/images/edits"

	openAIImagesGenerationsURL = "https://api.openai.com/v1/images/generations"
	openAIImagesEditsURL       = "https://api.openai.com/v1/images/edits"

	openAIChatGPTStartURL          = "https://chatgpt.com/"
	openAIChatGPTFilesURL          = "https://chatgpt.com/backend-api/files"
	openAIImageBackendUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	openAIImageMaxDownloadBytes    = 20 << 20 // 20MB per image download
	openAIImageMaxUploadPartSize   = 20 << 20 // 20MB per multipart upload part
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

func (r *OpenAIImagesRequest) ModerationBody() []byte {
	if r == nil {
		return nil
	}
	payload := map[string]any{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}
	images := r.moderationImages()
	if len(images) > 0 {
		payload["images"] = images
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func (r *OpenAIImagesRequest) moderationImages() []map[string]string {
	if r == nil {
		return nil
	}
	images := make([]map[string]string, 0, len(r.InputImageURLs)+len(r.Uploads)+1)
	for _, imageURL := range r.InputImageURLs {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	for _, upload := range r.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if maskURL := strings.TrimSpace(r.MaskImageURL); maskURL != "" {
		images = append(images, map[string]string{"image_url": maskURL})
	}
	if r.MaskUpload != nil {
		if dataURL := r.MaskUpload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	return images
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
	for _, path := range []string{"image", "image_url", "url", "b64_json", "base64", "image_base64", "input_image"} {
		if err := appendOpenAIImagesJSONInputImages(req, gjson.GetBytes(body, path), path); err != nil {
			return err
		}
	}
	for _, path := range []string{"images", "input_images"} {
		images := gjson.GetBytes(body, path)
		if !images.Exists() {
			continue
		}
		if !images.IsArray() {
			return fmt.Errorf("invalid %s field type", path)
		}
		if err := appendOpenAIImagesJSONInputImages(req, images, path); err != nil {
			return err
		}
	}
	if req.IsEdits() {
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

func appendOpenAIImagesJSONInputImages(req *OpenAIImagesRequest, images gjson.Result, path string) error {
	if !images.Exists() {
		return nil
	}
	if images.IsArray() {
		for _, item := range images.Array() {
			if imageURL := normalizeOpenAIImagesJSONInputImage(item); imageURL != "" {
				req.InputImageURLs = append(req.InputImageURLs, imageURL)
				continue
			}
			if item.Get("file_id").Exists() {
				return fmt.Errorf("%s[].file_id is not supported (use %s[].image_url instead)", path, path)
			}
		}
		return nil
	}
	if imageURL := normalizeOpenAIImagesJSONInputImage(images); imageURL != "" {
		req.InputImageURLs = append(req.InputImageURLs, imageURL)
	}
	return nil
}

func normalizeOpenAIImagesJSONInputImage(item gjson.Result) string {
	if !item.Exists() {
		return ""
	}
	switch {
	case item.IsObject():
		for _, path := range []string{"image_url", "url", "b64_json", "base64", "image_base64"} {
			if imageURL := normalizeOpenAIImagesInputImageString(item.Get(path).String()); imageURL != "" {
				return imageURL
			}
		}
	case item.Type == gjson.String:
		return normalizeOpenAIImagesInputImageString(item.String())
	}
	return ""
}

func normalizeOpenAIImagesInputImageString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:image/") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") {
		return raw
	}
	return "data:image/png;base64," + raw
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
		if shouldForwardOpenAIImagesAPIKeyAsCompatibleAggregate(account, parsed) {
			return s.forwardOpenAIImagesAPIKeyCompatibleAggregate(ctx, c, account, body, parsed, channelMappedModel)
		}
		if shouldForwardOpenAIImagesAPIKeyAsResponses(account, parsed) {
			return s.forwardOpenAIImagesResponses(ctx, c, account, parsed, channelMappedModel)
		}
		if shouldForwardOpenAIImagesAPIKeyAsJSONEdit(account, parsed) {
			return s.forwardOpenAIImagesAPIKeyAsJSONEdit(ctx, c, account, parsed, channelMappedModel)
		}
		if shouldForwardOpenAIImagesAPIKeyAsMultipartEdit(account, parsed) {
			return s.forwardOpenAIImagesAPIKeyAsMultipartEdit(ctx, c, account, body, parsed, channelMappedModel)
		}
		return s.forwardOpenAIImagesAPIKey(ctx, c, account, body, parsed, channelMappedModel)
	case AccountTypeOAuth:
		return s.forwardOpenAIImagesResponses(ctx, c, account, parsed, channelMappedModel)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func shouldForwardOpenAIImagesAPIKeyAsCompatibleAggregate(account *Account, parsed *OpenAIImagesRequest) bool {
	if parsed == nil || parsed.Multipart || parsed.Endpoint != openAIImagesGenerationsEndpoint {
		return false
	}
	if !isCustomOpenAIImagesBaseURL(account) {
		return false
	}
	return hasOpenAIImagesInput(parsed)
}

func hasOpenAIImagesInput(parsed *OpenAIImagesRequest) bool {
	if parsed == nil {
		return false
	}
	return len(parsed.InputImageURLs) > 0 ||
		len(parsed.Uploads) > 0 ||
		strings.TrimSpace(parsed.MaskImageURL) != "" ||
		parsed.MaskUpload != nil
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKeyCompatibleAggregate(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	initialWriterSize := -1
	if c != nil && c.Writer != nil {
		initialWriterSize = c.Writer.Size()
	}

	attempts := []struct {
		name string
		run  func() (*OpenAIForwardResult, error)
	}{
		{
			name: "generations_json",
			run: func() (*OpenAIForwardResult, error) {
				return s.forwardOpenAIImagesAPIKeyGenerationProbe(ctx, c, account, body, parsed, channelMappedModel)
			},
		},
		{
			name: "responses_bridge",
			run: func() (*OpenAIForwardResult, error) {
				return s.forwardOpenAIImagesResponses(ctx, c, account, parsed, channelMappedModel)
			},
		},
		{
			name: "json_edits",
			run: func() (*OpenAIForwardResult, error) {
				return s.forwardOpenAIImagesAPIKeyAsJSONEdit(ctx, c, account, parsed, channelMappedModel)
			},
		},
		{
			name: "multipart_edits",
			run: func() (*OpenAIForwardResult, error) {
				return s.forwardOpenAIImagesAPIKeyAsMultipartEdit(ctx, c, account, body, parsed, channelMappedModel)
			},
		},
	}

	var lastErr error
	for index, attempt := range attempts {
		result, err := attempt.run()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if openAIImagesCompatibleAggregateWroteResponse(c, initialWriterSize) {
			return result, err
		}
		if index == len(attempts)-1 || !shouldContinueOpenAIImagesCompatibleAggregate(err) {
			return result, finalizeOpenAIImagesCompatibleAggregateError(err)
		}
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Images compatible aggregate fallback route=%s account=%d error=%s",
			attempt.name,
			account.ID,
			sanitizeUpstreamErrorMessage(err.Error()),
		)
	}
	return nil, lastErr
}

func finalizeOpenAIImagesCompatibleAggregateError(err error) error {
	return err
}

func openAIImagesCompatibleAggregateWroteResponse(c *gin.Context, initialWriterSize int) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	currentSize := c.Writer.Size()
	if currentSize <= 0 {
		return false
	}
	return initialWriterSize < 0 || currentSize > initialWriterSize
}

func shouldContinueOpenAIImagesCompatibleAggregate(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func shouldForwardOpenAIImagesAPIKeyAsResponses(account *Account, parsed *OpenAIImagesRequest) bool {
	if parsed == nil {
		return false
	}
	if parsed.Multipart {
		return false
	}
	if !isCustomOpenAIImagesBaseURL(account) {
		return false
	}
	if !hasOpenAIImagesInput(parsed) {
		return false
	}
	return true
}

func shouldForwardOpenAIImagesAPIKeyWithJSONEditFallback(account *Account, parsed *OpenAIImagesRequest) bool {
	if parsed == nil || parsed.Multipart || parsed.Endpoint != openAIImagesGenerationsEndpoint {
		return false
	}
	if len(parsed.InputImageURLs) == 0 && strings.TrimSpace(parsed.MaskImageURL) == "" {
		return false
	}
	return isCustomOpenAIImagesBaseURL(account)
}

func shouldForwardOpenAIImagesAPIKeyAsJSONEdit(account *Account, parsed *OpenAIImagesRequest) bool {
	if parsed == nil || parsed.Multipart || parsed.Endpoint != openAIImagesEditsEndpoint {
		return false
	}
	if len(parsed.InputImageURLs) == 0 && strings.TrimSpace(parsed.MaskImageURL) == "" {
		return false
	}
	return isCustomOpenAIImagesBaseURL(account)
}

func shouldForwardOpenAIImagesAPIKeyAsMultipartEdit(account *Account, parsed *OpenAIImagesRequest) bool {
	if parsed == nil {
		return false
	}
	if parsed.Multipart {
		return false
	}
	if len(parsed.InputImageURLs) == 0 && strings.TrimSpace(parsed.MaskImageURL) == "" {
		return false
	}
	return true
}

func isCustomOpenAIImagesBaseURL(account *Account) bool {
	if account == nil {
		return false
	}
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		return false
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return false
	}
	return !isOfficialOpenAIImagesBaseHost(host)
}

func isOfficialOpenAIImagesBaseHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	return host == "api.openai.com" ||
		strings.HasSuffix(host, ".openai.com") ||
		host == "chatgpt.com" ||
		strings.HasSuffix(host, ".chatgpt.com")
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKeyAsJSONEdit(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	body, err := buildOpenAIImagesAPIKeyJSONEditBody(ctx, parsed)
	if err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images custom JSON edit route endpoint=%s account=%d",
		parsed.Endpoint,
		account.ID,
	)
	rewritten := *parsed
	rewritten.Endpoint = openAIImagesEditsEndpoint
	rewritten.ContentType = "application/json"
	rewritten.Multipart = false
	rewritten.HasMask = rewritten.HasMask || strings.TrimSpace(rewritten.MaskImageURL) != ""
	return s.forwardOpenAIImagesAPIKeyInternal(ctx, c, account, body, &rewritten, channelMappedModel, &openAIImagesAPIKeyForwardOptions{
		CustomJSONEdit: true,
	})
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKeyWithJSONEditFallback(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	return s.forwardOpenAIImagesAPIKeyInternal(ctx, c, account, body, parsed, channelMappedModel, &openAIImagesAPIKeyForwardOptions{
		FallbackToJSONEdit: true,
	})
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKeyGenerationProbe(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	const maxIgnoredImageRetries = 0
	options := &openAIImagesAPIKeyForwardOptions{
		FallbackIgnoredGenerationToJSONEdit: true,
	}
	probeBody := body
	if hasOpenAIImagesInput(parsed) {
		var err error
		probeBody, err = buildOpenAIImagesAPIKeyGenerationInputBody(ctx, parsed)
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		result, err := s.forwardOpenAIImagesAPIKeyInternal(ctx, c, account, probeBody, parsed, channelMappedModel, options)
		if err == nil || attempt >= maxIgnoredImageRetries || !isOpenAIImagesIgnoredInputFailover(err) {
			return result, err
		}
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Images generation ignored input image, retrying original generations route account=%d attempt=%d",
			account.ID,
			attempt+1,
		)
	}
}

func buildOpenAIImagesAPIKeyGenerationInputBody(ctx context.Context, parsed *OpenAIImagesRequest) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	if len(parsed.InputImageURLs) == 0 {
		return nil, fmt.Errorf("image input is required")
	}
	payload := map[string]any{}
	setString := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			payload[name] = value
		}
	}
	setString("model", parsed.Model)
	setString("prompt", parsed.Prompt)
	setString("size", parsed.Size)
	setString("response_format", parsed.ResponseFormat)
	setString("quality", parsed.Quality)
	setString("background", parsed.Background)
	setString("output_format", parsed.OutputFormat)
	setString("moderation", parsed.Moderation)
	setString("input_fidelity", parsed.InputFidelity)
	setString("style", parsed.Style)
	if parsed.N > 1 {
		payload["n"] = parsed.N
	}
	if parsed.Stream {
		payload["stream"] = parsed.Stream
	}
	if parsed.OutputCompression != nil {
		payload["output_compression"] = *parsed.OutputCompression
	}
	if parsed.PartialImages != nil {
		payload["partial_images"] = *parsed.PartialImages
	}

	images := make([]string, 0, len(parsed.InputImageURLs))
	for index, imageURL := range parsed.InputImageURLs {
		resolved, err := resolveOpenAIImagesInputImageForJSONEdit(ctx, imageURL, index, false)
		if err != nil {
			return nil, err
		}
		if resolved != "" {
			images = append(images, resolved)
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("image input is required")
	}
	payload["image"] = images
	if maskURL := strings.TrimSpace(parsed.MaskImageURL); maskURL != "" {
		resolvedMaskURL, err := resolveOpenAIImagesInputImageForJSONEdit(ctx, maskURL, 0, true)
		if err != nil {
			return nil, err
		}
		payload["mask"] = resolvedMaskURL
	}
	return marshalOpenAIUpstreamJSON(payload)
}

func isOpenAIImagesIgnoredInputFailover(err error) bool {
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) || failoverErr == nil {
		return false
	}
	if failoverErr.StatusCode != http.StatusBadGateway {
		return false
	}
	imageTokens, ok := openAIImagesResponseInputImageTokens(failoverErr.ResponseBody)
	return ok && imageTokens == 0
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKeyAsMultipartEdit(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	originalBody []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	body, contentType, err := buildOpenAIImagesAPIKeyMultipartEditBody(ctx, parsed)
	if err != nil {
		return nil, err
	}
	rewritten := *parsed
	rewritten.Endpoint = openAIImagesEditsEndpoint
	rewritten.ContentType = contentType
	rewritten.Multipart = true
	rewritten.HasMask = rewritten.HasMask || strings.TrimSpace(rewritten.MaskImageURL) != ""
	return s.forwardOpenAIImagesAPIKeyInternal(ctx, c, account, body, &rewritten, channelMappedModel, &openAIImagesAPIKeyForwardOptions{
		FallbackBody:   originalBody,
		FallbackParsed: parsed,
	})
}

type openAIImagesAPIKeyForwardOptions struct {
	FallbackBody                        []byte
	FallbackParsed                      *OpenAIImagesRequest
	CustomJSONEdit                      bool
	FallbackToJSONEdit                  bool
	FallbackIgnoredGenerationToJSONEdit bool
}

func shouldFallbackOpenAIImagesAPIKeyMultipartEdit(
	parsed *OpenAIImagesRequest,
	options *openAIImagesAPIKeyForwardOptions,
	statusCode int,
	upstreamMsg string,
	upstreamBody []byte,
) bool {
	if parsed == nil || options == nil || options.FallbackParsed == nil || len(options.FallbackBody) == 0 {
		return false
	}
	if parsed.Endpoint != openAIImagesEditsEndpoint || !parsed.Multipart {
		return false
	}
	if options.FallbackParsed.Endpoint != openAIImagesGenerationsEndpoint {
		return false
	}

	combined := strings.ToLower(strings.Join([]string{
		upstreamMsg,
		extractUpstreamErrorCode(upstreamBody),
		gjson.GetBytes(upstreamBody, "error.type").String(),
		gjson.GetBytes(upstreamBody, "error.code").String(),
	}, " "))
	for _, marker := range []string{
		"content_policy",
		"safety",
		"moderation",
		"quota",
		"billing",
		"authentication",
		"unauthorized",
		"rate_limit",
		"rate limit",
	} {
		if strings.Contains(combined, marker) {
			return false
		}
	}

	switch statusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType, http.StatusNotImplemented:
		return true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		for _, marker := range []string{
			"unsupported",
			"not supported",
			"unknown endpoint",
			"invalid endpoint",
			"no route",
			"edits",
			"multipart",
		} {
			if strings.Contains(combined, marker) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func shouldFallbackOpenAIImagesAPIKeyGenerationToJSONEdit(
	parsed *OpenAIImagesRequest,
	options *openAIImagesAPIKeyForwardOptions,
	statusCode int,
	upstreamMsg string,
	upstreamBody []byte,
) bool {
	if parsed == nil || options == nil || !options.FallbackToJSONEdit {
		return false
	}
	if parsed.Multipart || parsed.Endpoint != openAIImagesGenerationsEndpoint {
		return false
	}
	if len(parsed.InputImageURLs) == 0 && strings.TrimSpace(parsed.MaskImageURL) == "" {
		return false
	}

	combined := strings.ToLower(strings.Join([]string{
		upstreamMsg,
		extractUpstreamErrorCode(upstreamBody),
		gjson.GetBytes(upstreamBody, "error.type").String(),
		gjson.GetBytes(upstreamBody, "error.code").String(),
	}, " "))
	for _, marker := range []string{
		"content_policy",
		"safety",
		"moderation",
		"quota",
		"billing",
		"authentication",
		"unauthorized",
		"invalid_api_key",
		"rate_limit",
		"rate limit",
	} {
		if strings.Contains(combined, marker) {
			return false
		}
	}

	switch statusCode {
	case http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType, http.StatusNotImplemented:
		return true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		for _, marker := range []string{
			"unsupported",
			"not supported",
			"unknown endpoint",
			"invalid endpoint",
			"no route",
			"image",
			"images",
			"edit",
			"edits",
			"multipart",
			"request format",
			"invalid field",
			"unknown parameter",
		} {
			if strings.Contains(combined, marker) {
				return true
			}
		}
		return false
	default:
		return statusCode >= http.StatusInternalServerError
	}
}

func buildOpenAIImagesAPIKeyMultipartEditBody(ctx context.Context, parsed *OpenAIImagesRequest) ([]byte, string, error) {
	if parsed == nil {
		return nil, "", fmt.Errorf("parsed images request is required")
	}
	if len(parsed.InputImageURLs) == 0 {
		return nil, "", fmt.Errorf("image input is required")
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	writeField := func(name, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "model", value: parsed.Model},
		{name: "prompt", value: parsed.Prompt},
		{name: "size", value: parsed.Size},
		{name: "response_format", value: parsed.ResponseFormat},
		{name: "quality", value: parsed.Quality},
		{name: "background", value: parsed.Background},
		{name: "output_format", value: parsed.OutputFormat},
		{name: "moderation", value: parsed.Moderation},
		{name: "input_fidelity", value: parsed.InputFidelity},
		{name: "style", value: parsed.Style},
	} {
		if err := writeField(field.name, field.value); err != nil {
			return nil, "", fmt.Errorf("write multipart field %s: %w", field.name, err)
		}
	}
	if parsed.N > 0 {
		if err := writer.WriteField("n", strconv.Itoa(parsed.N)); err != nil {
			return nil, "", fmt.Errorf("write multipart field n: %w", err)
		}
	}
	if parsed.Stream {
		if err := writer.WriteField("stream", strconv.FormatBool(parsed.Stream)); err != nil {
			return nil, "", fmt.Errorf("write multipart field stream: %w", err)
		}
	}
	if parsed.OutputCompression != nil {
		if err := writer.WriteField("output_compression", strconv.Itoa(*parsed.OutputCompression)); err != nil {
			return nil, "", fmt.Errorf("write multipart field output_compression: %w", err)
		}
	}
	if parsed.PartialImages != nil {
		if err := writer.WriteField("partial_images", strconv.Itoa(*parsed.PartialImages)); err != nil {
			return nil, "", fmt.Errorf("write multipart field partial_images: %w", err)
		}
	}

	for index, imageURL := range parsed.InputImageURLs {
		upload, err := resolveOpenAIImagesInputImageForMultipart(ctx, imageURL, index, false)
		if err != nil {
			return nil, "", err
		}
		// OpenAI's images edit examples use repeated `image[]` parts.
		// Using that exact field name avoids the reference image being silently ignored.
		fieldName := "image[]"
		if err := writeOpenAIImagesMultipartUpload(writer, fieldName, upload); err != nil {
			return nil, "", err
		}
	}
	if maskURL := strings.TrimSpace(parsed.MaskImageURL); maskURL != "" {
		upload, err := resolveOpenAIImagesInputImageForMultipart(ctx, maskURL, 0, true)
		if err != nil {
			return nil, "", err
		}
		if err := writeOpenAIImagesMultipartUpload(writer, "mask", upload); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func buildOpenAIImagesAPIKeyJSONEditBody(ctx context.Context, parsed *OpenAIImagesRequest) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	if len(parsed.InputImageURLs) == 0 {
		return nil, fmt.Errorf("image input is required")
	}
	payload := map[string]any{}
	setString := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			payload[name] = value
		}
	}
	setString("model", parsed.Model)
	setString("prompt", parsed.Prompt)
	setString("size", parsed.Size)
	setString("response_format", parsed.ResponseFormat)
	setString("quality", parsed.Quality)
	setString("background", parsed.Background)
	setString("output_format", parsed.OutputFormat)
	setString("moderation", parsed.Moderation)
	setString("input_fidelity", parsed.InputFidelity)
	setString("style", parsed.Style)
	if parsed.N > 1 {
		payload["n"] = parsed.N
	}
	if parsed.Stream {
		payload["stream"] = parsed.Stream
	}
	if parsed.OutputCompression != nil {
		payload["output_compression"] = *parsed.OutputCompression
	}
	if parsed.PartialImages != nil {
		payload["partial_images"] = *parsed.PartialImages
	}

	images := make([]map[string]string, 0, len(parsed.InputImageURLs))
	for index, imageURL := range parsed.InputImageURLs {
		imageURL, err := resolveOpenAIImagesInputImageForJSONEdit(ctx, imageURL, index, false)
		if err != nil {
			return nil, err
		}
		if imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("image input is required")
	}
	payload["images"] = images
	if maskURL := strings.TrimSpace(parsed.MaskImageURL); maskURL != "" {
		resolvedMaskURL, err := resolveOpenAIImagesInputImageForJSONEdit(ctx, maskURL, 0, true)
		if err != nil {
			return nil, err
		}
		maskURL = resolvedMaskURL
		payload["mask"] = map[string]string{"image_url": maskURL}
	}
	return marshalOpenAIUpstreamJSON(payload)
}

func resolveOpenAIImagesInputImageForJSONEdit(ctx context.Context, raw string, index int, mask bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return raw, nil
	}
	upload, err := openAIImagesJSONEditImageDownloader(ctx, raw, index, mask)
	if err != nil {
		return "", err
	}
	dataURL := upload.ModerationDataURL()
	if dataURL == "" {
		return "", fmt.Errorf("downloaded image input is not a valid image")
	}
	return dataURL, nil
}

var openAIImagesJSONEditImageDownloader = downloadOpenAIImagesInputImageForMultipart

func writeOpenAIImagesMultipartUpload(writer *multipart.Writer, fieldName string, upload OpenAIImagesUpload) error {
	fileName := strings.TrimSpace(upload.FileName)
	if fileName == "" {
		fileName = "image.png"
	}
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(upload.Data)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartQuote(fieldName), escapeMultipartQuote(fileName)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create multipart image part: %w", err)
	}
	if _, err := part.Write(upload.Data); err != nil {
		return fmt.Errorf("write multipart image part: %w", err)
	}
	return nil
}

func resolveOpenAIImagesInputImageForMultipart(ctx context.Context, raw string, index int, mask bool) (OpenAIImagesUpload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return OpenAIImagesUpload{}, fmt.Errorf("image input is required")
	}
	if mimeType, payload, ok := splitOpenAIImageDataURL(raw); ok {
		b64 := payload
		if normalized := normalizeOpenAIImageBase64(payload); normalized != "" {
			b64 = normalized
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return OpenAIImagesUpload{}, fmt.Errorf("decode image data url: %w", err)
		}
		return newOpenAIImagesUploadFromBytes(data, mimeType, generatedOpenAIImagesUploadFileName(index, mimeType, mask)), nil
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return downloadOpenAIImagesInputImageForMultipart(ctx, raw, index, mask)
	}
	b64 := raw
	if normalized := normalizeOpenAIImageBase64(raw); normalized != "" {
		b64 = normalized
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return OpenAIImagesUpload{}, fmt.Errorf("decode image base64: %w", err)
	}
	return newOpenAIImagesUploadFromBytes(data, "", generatedOpenAIImagesUploadFileName(index, "", mask)), nil
}

func downloadOpenAIImagesInputImageForMultipart(ctx context.Context, rawURL string, index int, mask bool) (OpenAIImagesUpload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return OpenAIImagesUpload{}, fmt.Errorf("build image download request: %w", err)
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	req.Header.Set("User-Agent", openAIImageBackendUserAgent)
	client := newSSRFSafeHTTPClientWithResponseHeaderTimeout(30*time.Second, 15*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return OpenAIImagesUpload{}, fmt.Errorf("download image input: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return OpenAIImagesUpload{}, fmt.Errorf("download image input: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes+1))
	if err != nil {
		return OpenAIImagesUpload{}, fmt.Errorf("read image input: %w", err)
	}
	if len(data) > openAIImageMaxDownloadBytes {
		return OpenAIImagesUpload{}, fmt.Errorf("image input too large")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	fileName := openAIImagesUploadFileNameFromURL(rawURL)
	if fileName == "" {
		fileName = generatedOpenAIImagesUploadFileName(index, contentType, mask)
	}
	return newOpenAIImagesUploadFromBytes(data, contentType, fileName), nil
}

func newOpenAIImagesUploadFromBytes(data []byte, contentType string, fileName string) OpenAIImagesUpload {
	if strings.TrimSpace(contentType) == "" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		contentType = "image/png"
	}
	return OpenAIImagesUpload{
		FileName:    fileName,
		ContentType: contentType,
		Data:        data,
	}
}

func generatedOpenAIImagesUploadFileName(index int, contentType string, mask bool) string {
	prefix := "image"
	if mask {
		prefix = "mask"
	}
	ext := openAIImageExtensionFromContentType(contentType)
	if ext == "" {
		ext = ".png"
	}
	return fmt.Sprintf("%s-%d%s", prefix, index+1, ext)
}

func openAIImagesUploadFileNameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	name := path.Base(u.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		return ""
	}
	return name
}

func openAIImageExtensionFromContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func escapeMultipartQuote(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKey(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	return s.forwardOpenAIImagesAPIKeyInternal(ctx, c, account, body, parsed, channelMappedModel, nil)
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKeyInternal(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
	options *openAIImagesAPIKeyForwardOptions,
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
	forwardBody, forwardContentType, err = sanitizeOpenAIImagesAPIKeyUpstreamBody(forwardBody, forwardContentType, parsed, upstreamModel, account)
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
	if options != nil && options.CustomJSONEdit {
		applyOpenAIImagesCustomJSONEditHeaders(upstreamReq, account)
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
		if shouldFallbackOpenAIImagesAPIKeyGenerationToJSONEdit(parsed, options, resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "fallback",
				Message:            upstreamMsg,
			})
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[OpenAI] Images generation JSON edit fallback status=%d endpoint=%s account=%d",
				resp.StatusCode,
				parsed.Endpoint,
				account.ID,
			)
			return s.forwardOpenAIImagesAPIKeyAsJSONEdit(ctx, c, account, parsed, channelMappedModel)
		}
		if shouldFallbackOpenAIImagesAPIKeyMultipartEdit(parsed, options, resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "fallback",
				Message:            upstreamMsg,
			})
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[OpenAI] Images multipart edit fallback status=%d endpoint=%s account=%d",
				resp.StatusCode,
				parsed.Endpoint,
				account.ID,
			)
			return s.forwardOpenAIImagesAPIKeyInternal(ctx, c, account, options.FallbackBody, options.FallbackParsed, channelMappedModel, nil)
		}
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
			s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: shouldRetryOpenAIImagesAPIKeySameAccount(account, resp.StatusCode, upstreamMsg, respBody),
			}
		}
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, upstreamModel)
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
		nonStreamResp, err := s.prepareOpenAIImagesNonStreamingResponse(resp, c, parsed.ResponseFormat, s.openAIImagesPublicBaseURL(c))
		if err != nil {
			return nil, err
		}
		if shouldFallbackOpenAIImagesAPIKeyGenerationToJSONEditAfterSuccess(parsed, options, nonStreamResp.Body) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "fallback",
				Message:            "upstream generation ignored image input",
			})
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[OpenAI] Images generation JSON edit fallback after zero image input tokens endpoint=%s account=%d",
				parsed.Endpoint,
				account.ID,
			)
			if options != nil && options.FallbackIgnoredGenerationToJSONEdit {
				return nil, &UpstreamFailoverError{
					StatusCode:             http.StatusBadGateway,
					ResponseBody:           nonStreamResp.Body,
					RetryableOnSameAccount: false,
				}
			}
			return s.forwardOpenAIImagesAPIKeyAsJSONEdit(ctx, c, account, parsed, channelMappedModel)
		}
		s.writeOpenAIImagesNonStreamingResponse(resp, c, nonStreamResp)
		usage = nonStreamResp.Usage
		if nonStreamResp.ImageCount > 0 {
			imageCount = nonStreamResp.ImageCount
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
			ImageOutputSizes: nonStreamResp.ImageOutputSizes,
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
	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func applyOpenAIImagesCustomJSONEditHeaders(req *http.Request, account *Account) {
	if req == nil {
		return
	}
	authorization := req.Header.Get("Authorization")
	req.Header = http.Header{}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if account != nil {
		customUA := strings.TrimSpace(account.GetOpenAIUserAgent())
		if customUA == "" {
			customUA = strings.TrimSpace(account.GetCredential("user_agent"))
		}
		if customUA == "" {
			return
		}
		req.Header.Set("User-Agent", customUA)
	}
}

func shouldRetryOpenAIImagesAPIKeySameAccount(account *Account, statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if account == nil || !account.IsPoolMode() {
		return false
	}
	if account.IsPoolModeRetryableStatus(statusCode) {
		return true
	}
	if statusCode >= http.StatusInternalServerError {
		return true
	}
	return isOpenAITransientProcessingError(statusCode, upstreamMsg, upstreamBody)
}

func shouldFallbackOpenAIImagesAPIKeyGenerationToJSONEditAfterSuccess(
	parsed *OpenAIImagesRequest,
	options *openAIImagesAPIKeyForwardOptions,
	responseBody []byte,
) bool {
	if parsed == nil || options == nil || (!options.FallbackToJSONEdit && !options.FallbackIgnoredGenerationToJSONEdit) {
		return false
	}
	if parsed.Multipart || parsed.Endpoint != openAIImagesGenerationsEndpoint {
		return false
	}
	if len(parsed.InputImageURLs) == 0 && strings.TrimSpace(parsed.MaskImageURL) == "" {
		return false
	}
	imageTokens, ok := openAIImagesResponseInputImageTokens(responseBody)
	return ok && imageTokens == 0
}

func openAIImagesResponseInputImageTokens(body []byte) (int64, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0, false
	}
	for _, path := range []string{
		"usage.input_tokens_details.image_tokens",
		"usage.prompt_tokens_details.image_tokens",
		"response.usage.input_tokens_details.image_tokens",
		"response.usage.prompt_tokens_details.image_tokens",
	} {
		result := gjson.GetBytes(body, path)
		if result.Exists() {
			return result.Int(), true
		}
	}
	return 0, false
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
		rewrittenBody, rewrittenType, rewriteErr := rewriteOpenAIImagesMultipartBody(body, contentType, model, nil)
		return rewrittenBody, rewrittenType, rewriteErr
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite image request model: %w", err)
	}
	return rewritten, contentType, nil
}

func rewriteOpenAIImagesMultipartModel(body []byte, contentType string, model string) ([]byte, string, error) {
	return rewriteOpenAIImagesMultipartBody(body, contentType, model, nil)
}

func sanitizeOpenAIImagesAPIKeyUpstreamBody(body []byte, contentType string, parsed *OpenAIImagesRequest, upstreamModel string, account *Account) ([]byte, string, error) {
	if parsed == nil {
		return body, contentType, nil
	}
	dropFields := unsupportedOpenAIImagesAPIKeyFields(parsed, upstreamModel, contentType, account)
	if len(dropFields) == 0 {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIImagesMultipartBody(body, contentType, "", dropFields)
	}

	rewritten := body
	for field := range dropFields {
		next, err := sjson.DeleteBytes(rewritten, field)
		if err != nil {
			return nil, "", fmt.Errorf("remove unsupported image request field %s: %w", field, err)
		}
		rewritten = next
	}
	return rewritten, contentType, nil
}

func unsupportedOpenAIImagesAPIKeyFields(parsed *OpenAIImagesRequest, upstreamModel string, contentType string, account *Account) map[string]struct{} {
	if parsed == nil {
		return nil
	}
	if preservesOpenAIImagesResponseFormatForCustomJSONEdit(parsed, contentType, account) {
		return nil
	}
	if parsed.Endpoint == openAIImagesEditsEndpoint && isOpenAIImageGenerationModel(upstreamModel) && strings.TrimSpace(parsed.ResponseFormat) != "" {
		return map[string]struct{}{
			"response_format": {},
		}
	}
	return nil
}

func preservesOpenAIImagesResponseFormatForCustomJSONEdit(parsed *OpenAIImagesRequest, contentType string, account *Account) bool {
	if parsed == nil || parsed.Endpoint != openAIImagesEditsEndpoint || !isCustomOpenAIImagesBaseURL(account) {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return true
	}
	return !strings.EqualFold(mediaType, "multipart/form-data")
}

func rewriteOpenAIImagesMultipartBody(body []byte, contentType string, model string, dropFields map[string]struct{}) ([]byte, string, error) {
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
		if part.FileName() == "" {
			if _, drop := dropFields[formName]; drop {
				_ = part.Close()
				continue
			}
		}
		partHeader := cloneMultipartHeader(part.Header)
		target, err := writer.CreatePart(partHeader)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}

		if model != "" && formName == "model" && part.FileName() == "" {
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

type openAIImagesNonStreamingResponse struct {
	Body             []byte
	ContentType      string
	Usage            OpenAIUsage
	ImageCount       int
	ImageOutputSizes []string
}

func (s *OpenAIGatewayService) handleOpenAIImagesNonStreamingResponse(resp *http.Response, c *gin.Context, responseFormat string, publicBaseURL string) (OpenAIUsage, int, []string, error) {
	result, err := s.prepareOpenAIImagesNonStreamingResponse(resp, c, responseFormat, publicBaseURL)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	s.writeOpenAIImagesNonStreamingResponse(resp, c, result)
	return result.Usage, result.ImageCount, result.ImageOutputSizes, nil
}

func (s *OpenAIGatewayService) prepareOpenAIImagesNonStreamingResponse(resp *http.Response, c *gin.Context, responseFormat string, publicBaseURL string) (*openAIImagesNonStreamingResponse, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	responseBody := body
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		if rewritten, changed := rewriteOpenAIImagesURLResponseBody(c, body, publicBaseURL); changed {
			responseBody = rewritten
		}
	}
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	return &openAIImagesNonStreamingResponse{
		Body:             responseBody,
		ContentType:      contentType,
		Usage:            usage,
		ImageCount:       extractOpenAIImageCountFromJSONBytes(responseBody),
		ImageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(responseBody),
	}, nil
}

func (s *OpenAIGatewayService) writeOpenAIImagesNonStreamingResponse(resp *http.Response, c *gin.Context, result *openAIImagesNonStreamingResponse) {
	if result == nil {
		return
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, result.ContentType, result.Body)
}

func rewriteOpenAIImagesURLResponseBody(c *gin.Context, body []byte, publicBaseURL string) ([]byte, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false
	}
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() {
		return body, false
	}

	out := append([]byte(nil), body...)
	changed := false
	for i, item := range data.Array() {
		itemPath := fmt.Sprintf("data.%d", i)
		outputFormat := firstNonEmptyString(
			item.Get("output_format").String(),
			item.Get("mime_type").String(),
			item.Get("content_type").String(),
			gjson.GetBytes(body, "output_format").String(),
		)

		rawURL := strings.TrimSpace(item.Get("url").String())
		b64JSON := strings.TrimSpace(item.Get("b64_json").String())
		nextURL := rawURL
		if b64JSON != "" {
			nextURL = openAIImageURLFromBase64(c, b64JSON, outputFormat, publicBaseURL)
		} else if isOpenAIImageDataURL(rawURL) {
			nextURL = openAIImageURLFromBase64(c, rawURL, outputFormat, publicBaseURL)
		}

		if nextURL == "" {
			continue
		}
		if nextURL != rawURL || !item.Get("url").Exists() {
			if rewritten, err := sjson.SetBytes(out, itemPath+".url", nextURL); err == nil {
				out = rewritten
				changed = true
			}
		}
		if item.Get("b64_json").Exists() {
			if rewritten, err := sjson.DeleteBytes(out, itemPath+".b64_json"); err == nil {
				out = rewritten
				changed = true
			}
		}
	}
	return out, changed
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

func extractOpenAIImagesBillableCountFromJSONBytes(body []byte) int {
	if count := extractOpenAIImageCountFromJSONBytes(body); count > 0 {
		return count
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0
	}
	if count := int(gjson.GetBytes(body, "usage.images").Int()); count > 0 {
		return count
	}
	if count := int(gjson.GetBytes(body, "tool_usage.image_gen.images").Int()); count > 0 {
		return count
	}
	eventType := strings.TrimSpace(gjson.GetBytes(body, "type").String())
	if eventType == "" || !strings.HasSuffix(eventType, ".completed") {
		return 0
	}
	if gjson.GetBytes(body, "b64_json").Exists() || gjson.GetBytes(body, "url").Exists() {
		return 1
	}
	return 0
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

type openAIImagePointerInfo struct {
	Pointer     string
	DownloadURL string
	B64JSON     string
	MimeType    string
	Prompt      string
}

func collectOpenAIImagePointers(body []byte) []openAIImagePointerInfo {
	if len(body) == 0 {
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
	matches := openAIImagePointerMatches(body)
	out := make([]openAIImagePointerInfo, 0, len(matches))
	for _, pointer := range matches {
		out = append(out, openAIImagePointerInfo{Pointer: pointer, Prompt: prompt})
	}
	return mergeOpenAIImagePointerInfos(out, collectOpenAIImageInlineAssets(body, prompt))
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
		if key := item.identityKey(); key != "" {
			seen[key] = item
		}
		out = append(out, item)
	}
	for _, item := range next {
		key := item.identityKey()
		if key == "" {
			continue
		}
		if existingItem, ok := seen[key]; ok {
			merged := mergeOpenAIImagePointerInfo(existingItem, item)
			if merged != existingItem {
				for i := range out {
					if out[i].identityKey() == key {
						out[i] = merged
						break
					}
				}
				seen[key] = merged
			}
			continue
		}
		seen[key] = item
		out = append(out, item)
	}
	return out
}

func (i openAIImagePointerInfo) identityKey() string {
	switch {
	case strings.TrimSpace(i.Pointer) != "":
		return "pointer:" + strings.TrimSpace(i.Pointer)
	case strings.TrimSpace(i.DownloadURL) != "":
		return "download:" + strings.TrimSpace(i.DownloadURL)
	case strings.TrimSpace(i.B64JSON) != "":
		b64 := strings.TrimSpace(i.B64JSON)
		if len(b64) > 64 {
			b64 = b64[:64]
		}
		return "b64:" + b64
	default:
		return ""
	}
}

func mergeOpenAIImagePointerInfo(existing, next openAIImagePointerInfo) openAIImagePointerInfo {
	merged := existing
	if strings.TrimSpace(merged.Pointer) == "" {
		merged.Pointer = next.Pointer
	}
	if strings.TrimSpace(merged.DownloadURL) == "" {
		merged.DownloadURL = next.DownloadURL
	}
	if strings.TrimSpace(merged.B64JSON) == "" {
		merged.B64JSON = next.B64JSON
	}
	if strings.TrimSpace(merged.MimeType) == "" {
		merged.MimeType = next.MimeType
	}
	if strings.TrimSpace(merged.Prompt) == "" {
		merged.Prompt = next.Prompt
	}
	return merged
}

func resolveOpenAIImageBytes(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer openAIImagePointerInfo,
	errorBodyReadLimit int64,
) ([]byte, error) {
	if normalized := normalizeOpenAIImageBase64(pointer.B64JSON); normalized != "" {
		return base64.StdEncoding.DecodeString(normalized)
	}
	if downloadURL := strings.TrimSpace(pointer.DownloadURL); downloadURL != "" {
		return downloadOpenAIImageBytes(ctx, client, headers, downloadURL, errorBodyReadLimit)
	}
	if strings.TrimSpace(pointer.Pointer) == "" {
		return nil, fmt.Errorf("image asset is missing pointer, url, and base64 data")
	}
	downloadURL, err := fetchOpenAIImageDownloadURL(ctx, client, headers, conversationID, pointer.Pointer, errorBodyReadLimit)
	if err != nil {
		return nil, err
	}
	return downloadOpenAIImageBytes(ctx, client, headers, downloadURL, errorBodyReadLimit)
}

func normalizeOpenAIImageBase64(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		if idx := strings.Index(raw, ","); idx >= 0 && idx+1 < len(raw) {
			raw = raw[idx+1:]
		}
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "=") + strings.Repeat("=", (4-len(raw)%4)%4)
	if raw == "" {
		return ""
	}
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
		return ""
	}
	return raw
}

func collectOpenAIImageInlineAssets(body []byte, fallbackPrompt string) []openAIImagePointerInfo {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	var out []openAIImagePointerInfo
	walkOpenAIImageInlineAssets(decoded, strings.TrimSpace(fallbackPrompt), &out)
	return out
}

func walkOpenAIImageInlineAssets(node any, prompt string, out *[]openAIImagePointerInfo) {
	switch value := node.(type) {
	case map[string]any:
		localPrompt := prompt
		for _, key := range []string{"revised_prompt", "image_gen_title", "prompt"} {
			if v, ok := value[key].(string); ok && strings.TrimSpace(v) != "" {
				localPrompt = strings.TrimSpace(v)
				break
			}
		}
		item := openAIImagePointerInfo{
			Prompt:      localPrompt,
			Pointer:     firstNonEmptyString(value["asset_pointer"], value["pointer"]),
			DownloadURL: firstNonEmptyString(value["download_url"], value["url"], value["image_url"]),
			B64JSON:     firstNonEmptyString(value["b64_json"], value["base64"], value["image_base64"]),
			MimeType:    firstNonEmptyString(value["mime_type"], value["mimeType"], value["content_type"]),
		}
		switch {
		case strings.HasPrefix(strings.TrimSpace(item.Pointer), "file-service://"),
			strings.HasPrefix(strings.TrimSpace(item.Pointer), "sediment://"),
			isLikelyOpenAIImageDownloadURL(item.DownloadURL),
			normalizeOpenAIImageBase64(item.B64JSON) != "":
			*out = append(*out, item)
		}
		for _, child := range value {
			walkOpenAIImageInlineAssets(child, localPrompt, out)
		}
	case []any:
		for _, child := range value {
			walkOpenAIImageInlineAssets(child, prompt, out)
		}
	}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func isLikelyOpenAIImageDownloadURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:image/") {
		return true
	}
	if !strings.HasPrefix(strings.ToLower(raw), "http://") && !strings.HasPrefix(strings.ToLower(raw), "https://") {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "/download") ||
		strings.Contains(lower, ".png") ||
		strings.Contains(lower, ".jpg") ||
		strings.Contains(lower, ".jpeg") ||
		strings.Contains(lower, ".webp")
}

func fetchOpenAIImageDownloadURL(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer string,
	errorBodyReadLimit int64,
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
			statusErr := newOpenAIImageStatusError(resp, "fetch image download url failed", errorBodyReadLimit)
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

func downloadOpenAIImageBytes(ctx context.Context, client *req.Client, headers http.Header, downloadURL string, errorBodyReadLimit int64) ([]byte, error) {
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
		return nil, newOpenAIImageStatusError(resp, "download image bytes failed", errorBodyReadLimit)
	}
	return io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
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

func newOpenAIImageStatusError(resp *req.Response, fallback string, errorBodyReadLimit int64) error {
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
			if errorBodyReadLimit <= 0 {
				errorBodyReadLimit = openAIUpstreamErrorBodyReadLimit
			}
			body, _ = io.ReadAll(io.LimitReader(resp.Body, errorBodyReadLimit))
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
