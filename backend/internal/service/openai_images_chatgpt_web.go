package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	AccountExtraChatGPTWebImageEnabled     = "chatgpt_web_image_enabled"
	AccountExtraChatGPTWebImageRegularChat = "chatgpt_web_image_regular_chat"
	chatGPTWebImageProvider                = "chatgpt_web"
	chatGPTWebImageRequestContextKey       = "sub2api.internal.chatgpt_web_image_request"
)

// IsChatGPTWebImageRequest checks a marker written only by the server's Images
// dispatcher. A client-supplied provider header must not influence scheduling.
func IsChatGPTWebImageRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, found := c.Get(chatGPTWebImageRequestContextKey)
	flag, _ := value.(bool)
	return found && flag
}

// These strict boolean flags opt into creating regular ChatGPT web history.
func chatGPTWebImageEnabled(account *Account) bool {
	return account != nil && account.getExtraBool(AccountExtraChatGPTWebImageEnabled)
}

func chatGPTWebImageRequestError(code, message, param string) *OpenAIImagesUpstreamError {
	return &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "invalid_request_error", Code: code, Message: message, Param: param}
}

func validateChatGPTWebImageAccount(account *Account) *OpenAIImagesUpstreamError {
	if !chatGPTWebImageEnabled(account) {
		return chatGPTWebImageRequestError("chatgpt_web_image_disabled", "ChatGPT web image generation is not enabled for this account", "")
	}
	if !account.IsOpenAIOAuth() || !strings.EqualFold(strings.TrimSpace(account.GetCredential("plan_type")), "free") || account.IsShadow() {
		return chatGPTWebImageRequestError("chatgpt_web_image_account_unsupported", "ChatGPT web image generation requires a Free OpenAI OAuth account", "")
	}
	if !account.getExtraBool(AccountExtraChatGPTWebImageRegularChat) {
		return chatGPTWebImageRequestError("chatgpt_web_image_history_consent_required", "ChatGPT web image generation requires explicit permission to create a regular web conversation", "")
	}
	return nil
}

func validateChatGPTWebImagesRequest(parsed *OpenAIImagesRequest) *OpenAIImagesUpstreamError {
	if parsed == nil {
		return chatGPTWebImageRequestError("invalid_request", "An image request is required", "")
	}
	unsupported := func(param string) *OpenAIImagesUpstreamError {
		return chatGPTWebImageRequestError("chatgpt_web_image_option_unsupported", "ChatGPT web image generation does not support the requested "+param+" option", param)
	}
	if parsed.Endpoint != openAIImagesGenerationsEndpoint || parsed.Multipart || len(parsed.Uploads) > 0 || len(parsed.InputImageURLs) > 0 || parsed.HasMask || parsed.MaskUpload != nil || parsed.MaskImageURL != "" {
		return unsupported("image")
	}
	if strings.TrimSpace(parsed.Prompt) == "" {
		return chatGPTWebImageRequestError("invalid_request", "prompt is required", "prompt")
	}
	if len(parsed.Prompt) > 64<<10 {
		return chatGPTWebImageRequestError("invalid_request", "prompt exceeds the ChatGPT web image size limit", "prompt")
	}
	// The shared parser preserves fields it does not understand. Reject them
	// here so privacy options such as store=false cannot silently become a
	// retained web conversation, and unsupported controls are never ignored.
	if len(parsed.Body) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(parsed.Body, &fields) != nil || fields == nil {
			return chatGPTWebImageRequestError("invalid_request", "An image JSON object is required", "")
		}
		for field := range fields {
			switch field {
			case "model", "prompt", "n", "stream", "response_format", "output_format", "size", "quality", "background", "moderation", "input_fidelity", "style", "output_compression", "partial_images":
			default:
				return unsupported(field)
			}
		}
	}
	if parsed.N != 1 {
		return unsupported("n")
	}
	if parsed.Stream {
		return unsupported("stream")
	}
	if _, err := chatGPTWebImagePromptForSize(parsed.Prompt, parsed.Size); err != nil {
		return err
	}
	for _, field := range []struct{ key, value, allowed string }{
		{"response_format", parsed.ResponseFormat, "b64_json"},
		{"output_format", parsed.OutputFormat, "png"},
		{"quality", parsed.Quality, "auto"},
		{"background", parsed.Background, "auto"},
		{"moderation", parsed.Moderation, "auto"},
		{"input_fidelity", parsed.InputFidelity, ""},
		{"style", parsed.Style, ""},
	} {
		if value := strings.TrimSpace(field.value); value != "" && value != field.allowed {
			return unsupported(field.key)
		}
	}
	if parsed.OutputCompression != nil {
		return unsupported("output_compression")
	}
	if parsed.PartialImages != nil {
		return unsupported("partial_images")
	}
	return nil
}

// generateWebImage is shared with the Responses adapter. It owns no response
// writer and never retries a generation or changes account status.
func (s *OpenAIGatewayService) generateWebImage(ctx context.Context, account *Account, prompt string) (chatgptwebimage.Result, error) {
	if err := validateChatGPTWebImageAccount(account); err != nil {
		return chatgptwebimage.Result{}, err
	}
	if strings.TrimSpace(prompt) == "" {
		return chatgptwebimage.Result{}, chatGPTWebImageRequestError("invalid_request", "prompt is required", "prompt")
	}
	if len(prompt) > 64<<10 {
		return chatgptwebimage.Result{}, chatGPTWebImageRequestError("invalid_request", "prompt exceeds the ChatGPT web image size limit", "prompt")
	}
	logFailure := func(err error) {
		var providerErr *chatgptwebimage.Error
		if !errors.As(err, &providerErr) {
			return
		}
		diagnostic := providerErr.Diagnostic()
		// The request-scoped logger carries the request ID into detached jobs.
		// Never attach err/cause, the request body, headers, or credentials.
		logger.FromContext(ctx).Error("chatgpt_web_image_failed",
			zap.Int64("account_id", account.ID),
			zap.String("provider_stage", diagnostic.Stage),
			zap.String("provider_error_kind", diagnostic.Kind),
			zap.Int("upstream_status_code", diagnostic.StatusCode),
			zap.Bool("generation_may_have_started", diagnostic.Accepted))
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		providerErr := &chatgptwebimage.Error{Stage: "credentials", Kind: "credentials_unavailable"}
		logFailure(providerErr)
		return chatgptwebimage.Result{}, providerErr
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := chatgptwebimage.New(chatgptwebimage.Options{
		ProxyURL: proxyURL, Timeout: 4 * time.Minute, PollInterval: 3 * time.Second,
		MaxPolls: 60, MaxImageBytes: openAIImageMaxDownloadBytes,
	})
	if err != nil {
		providerErr := &chatgptwebimage.Error{Stage: "initialize", Kind: "invalid_configuration"}
		logFailure(providerErr)
		return chatgptwebimage.Result{}, providerErr
	}
	defer client.CloseIdleConnections()
	result, err := client.Generate(ctx, chatgptwebimage.GenerateRequest{AccessToken: token, Prompt: prompt, AllowRegularChat: true})
	if err != nil {
		logFailure(err)
		// The job store needs the original acceptance state to distinguish a
		// safe later retry from an unknown generation outcome. Convert only at
		// the HTTP/persistence boundary.
		return result, err
	}
	return result, nil
}

func chatGPTWebImagePublicError(err error) *OpenAIImagesUpstreamError {
	var already *OpenAIImagesUpstreamError
	if errors.As(err, &already) {
		return already
	}
	result := &OpenAIImagesUpstreamError{StatusCode: http.StatusBadGateway, ErrorType: "upstream_error", Code: "chatgpt_web_image_failed", Message: "ChatGPT web image generation failed"}
	var providerErr *chatgptwebimage.Error
	if errors.As(err, &providerErr) {
		diagnostic := providerErr.Diagnostic()
		if diagnostic.Kind == "credentials_unavailable" {
			result.Code = "chatgpt_web_image_credentials_unavailable"
			result.Message = "Could not obtain account credentials for ChatGPT web image generation"
		}
		if diagnostic.Kind == "model_not_found" && diagnostic.Stage == "conversation" && diagnostic.StatusCode == http.StatusNotFound && !diagnostic.Accepted {
			result.Code = "chatgpt_web_image_model_unavailable"
			result.Message = "ChatGPT web rejected the conversation model as unavailable; no image generation started and no alternate model was submitted"
		}
		if providerErr.StatusCode == http.StatusTooManyRequests {
			result.StatusCode = http.StatusTooManyRequests
			result.ErrorType = "rate_limit_error"
			result.Code = "chatgpt_web_image_rate_limited"
			result.Message = "ChatGPT web image generation is rate limited"
		}
		if providerErr.Accepted {
			// The tested Codex client retries 422 as well as 429/5xx responses.
			// Once generation may have started, use 400 to avoid duplicate jobs.
			result.StatusCode = http.StatusBadRequest
			result.ErrorType = "image_generation_error"
			result.Code = "image_generation_outcome_unknown"
			result.Message = "The image request may have started but its result is unavailable; a new generation was not submitted"
		}
		result.Message += fmt.Sprintf(" (stage=%s, reason=%s, upstream_status=%d)", diagnostic.Stage, diagnostic.Kind, diagnostic.StatusCode)
		if !diagnostic.Accepted {
			switch diagnostic.Kind {
			case "authentication_or_challenge", "interactive_challenge":
				result.Message += "; check the account's ChatGPT web session and outbound proxy; no image generation started"
			case "transport", "timeout_or_canceled":
				result.Message += "; check outbound connectivity and retry; no image generation started"
			}
		}
	}
	// Not UpstreamFailoverError: the Images handler must not submit another job.
	return result
}

func (s *OpenAIGatewayService) forwardChatGPTWebImages(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIImagesRequest, channelMappedModel string) (*OpenAIForwardResult, error) {
	start := time.Now()
	c.Header("X-Sub2API-Image-Provider", chatGPTWebImageProvider)
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" && !IsGPTImageGenerationModel(requestModel) {
		requestModel = mapped
	}
	imageModel, err := resolveChatGPTWebImageModel(account, requestModel)
	if err != nil {
		writeOpenAIImagesUpstreamErrorResponse(c, chatGPTWebImagePublicError(err))
		return nil, err
	}
	if err := validateChatGPTWebImagesRequest(parsed); err != nil {
		writeOpenAIImagesUpstreamErrorResponse(c, err)
		return nil, err
	}
	prompt, sizeErr := chatGPTWebImagePromptForSize(parsed.Prompt, parsed.Size)
	if sizeErr != nil {
		writeOpenAIImagesUpstreamErrorResponse(c, sizeErr)
		return nil, sizeErr
	}
	result, err := s.generateWebImageOnce(ctx, c, account, parsed.Body, prompt, imageModel.RequestedModel)
	if err != nil {
		publicErr := chatGPTWebImagePublicError(err)
		writeOpenAIImagesUpstreamErrorResponse(c, publicErr)
		return nil, publicErr
	}
	forward, err := writeChatGPTWebImagesResult(c, parsed, result, start)
	if err == nil {
		applyChatGPTWebImageJobBilling(c, forward)
	}
	return forward, err
}

func writeChatGPTWebImagesResult(c *gin.Context, parsed *OpenAIImagesRequest, result chatgptwebimage.Result, start time.Time) (*OpenAIForwardResult, error) {
	fail := func(message string) (*OpenAIForwardResult, error) {
		err := &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "image_generation_error", Code: "image_generation_outcome_unknown", Message: message}
		writeOpenAIImagesUpstreamErrorResponse(c, err)
		return nil, err
	}
	if len(result.Images) != 1 {
		return fail("ChatGPT web image generation did not return exactly one image")
	}
	img := result.Images[0]
	if len(img.Bytes) == 0 || int64(len(img.Bytes)) > openAIImageMaxDownloadBytes {
		return fail("ChatGPT web image generation returned invalid image data")
	}
	config, err := png.DecodeConfig(bytes.NewReader(img.Bytes))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 32*1024*1024 {
		return fail("ChatGPT web image generation returned invalid PNG dimensions")
	}
	decoded, err := png.Decode(bytes.NewReader(img.Bytes))
	if err != nil {
		return fail("ChatGPT web image generation did not return a valid PNG")
	}
	width, height, sizeErr := parseChatGPTWebImageSize(parsed.Size)
	if sizeErr != nil {
		writeOpenAIImagesUpstreamErrorResponse(c, sizeErr)
		return nil, sizeErr
	}
	if width > 0 && (config.Width != width || config.Height != height) {
		img.Bytes, err = resizeChatGPTWebImage(decoded, width, height)
		if err != nil {
			return fail("Could not resize the generated PNG to the requested dimensions; the image was generated and a new generation was not submitted")
		}
		c.Header("X-Sub2API-Image-Original-Size", fmt.Sprintf("%dx%d", config.Width, config.Height))
		c.Header("X-Sub2API-Image-Size-Adaptation", "resize-center-crop")
		config.Width, config.Height = width, height
	}
	size := fmt.Sprintf("%dx%d", config.Width, config.Height)
	meta := openAIResponsesImageResult{OutputFormat: "png", Size: size, Model: strings.TrimSpace(result.ObservedImageModel)}
	imageResult := meta
	imageResult.Result = base64.StdEncoding.EncodeToString(img.Bytes)
	body, err := buildOpenAIImagesAPIResponse([]openAIResponsesImageResult{imageResult}, time.Now().Unix(), nil, meta, "b64_json")
	if err != nil {
		return fail("Could not format the ChatGPT web image result")
	}
	StopOpenAIImagesJSONKeepaliveCommitted(c)
	c.Header("X-Sub2API-Image-Provider", chatGPTWebImageProvider)
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return &OpenAIForwardResult{
		Model: parsed.Model, UpstreamModel: result.WebDriver,
		UpstreamResponseModel: result.ObservedImageModel,
		UpstreamEndpoint:      "/backend-api/f/conversation", Duration: time.Since(start),
		ImageCount: 1, ImageSize: normalizeOpenAIImageSizeTier(size), ImageInputSize: parsed.Size,
		ImageOutputSize: size, ImageOutputSizes: []string{size}, ImageSizeSource: "decoded",
	}, nil
}
