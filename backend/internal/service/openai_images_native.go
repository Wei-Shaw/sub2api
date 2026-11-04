package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// The authentication remains OAuth. Only explicitly marked image-only accounts
// use the native Images endpoint; ordinary OAuth still uses Responses.
const OpenAIImageChannelExtraKey = "openai_image_channel"

// Share the verified CLI identity; the native endpoint still omits Version/OpenAI-Beta.
const nativeImageCLIUserAgent = codexCLIUserAgent

func (a *Account) UsesOpenAINativeImageChannel() bool {
	return a != nil && a.Platform == PlatformOpenAI && a.Type == AccountTypeOAuth && a.GetExtraString(OpenAIImageChannelExtraKey) == "native"
}

func validateOpenAIImageChannel(a *Account) error {
	if a == nil {
		return nil
	}
	value, present := a.Extra[OpenAIImageChannelExtraKey]
	if !present {
		return nil
	}
	channel, valid := value.(string)
	if !valid || channel != "native" || !a.UsesOpenAINativeImageChannel() || a.UsesChatGPTWebImageChannel() || a.IsOpenAIPersonalAccessToken() || a.IsOpenAIAgentIdentity() || a.IsCredentialShadow() {
		return infraerrors.BadRequest("INVALID_IMAGE_CHANNEL", "Native-Image requires a regular OpenAI OAuth account without the web image switch")
	}
	return nil
}

// The same option contract is used by scheduling and the final forwarding guard.
// All partial_images values are treated as zero on the Native-Image channel.
// Native Images currently returns final JSON only. Accept the option locally,
// omit it from the upstream JSON, and emit only the genuine completion event.
func supportsOpenAINativeImageOptions(parsed *OpenAIImagesRequest) bool {
	// input_fidelity is intentionally ignored on the native channel: the upstream
	// native endpoint does not accept it, and buildOpenAINativeImagesBody never
	// forwards it anyway. Dropping it here (instead of rejecting) lets clients
	// that send input_fidelity still reach a native account.
	return parsed != nil && parsed.N <= 1 && !parsed.HasMask && parsed.MaskUpload == nil && parsed.MaskImageURL == "" && parsed.OutputCompression == nil && (parsed.OutputFormat == "" || parsed.OutputFormat == "png") && parsed.Moderation == "" && parsed.Style == ""
}

type nativeImageOptionsContextKey struct{}

// WithOpenAIImagesRequestOptions records only compatibility, never image contents.
func WithOpenAIImagesRequestOptions(ctx context.Context, parsed *OpenAIImagesRequest) context.Context {
	return context.WithValue(ctx, nativeImageOptionsContextKey{}, !supportsOpenAINativeImageOptions(parsed))
}

func accountSupportsOpenAIImageRequestOptions(ctx context.Context, account *Account) bool {
	incompatible, _ := ctx.Value(nativeImageOptionsContextKey{}).(bool)
	return !incompatible || !account.UsesOpenAINativeImageChannel()
}

// OpenAIImagesOptionsUnsupported distinguishes a configured capability mismatch
// from temporarily unavailable compatible accounts. Only used after selection fails.
func (s *OpenAIGatewayService) OpenAIImagesOptionsUnsupported(ctx context.Context, groupID *int64, model string) bool {
	incompatible, _ := ctx.Value(nativeImageOptionsContextKey{}).(bool)
	if !incompatible || s == nil || s.accountRepo == nil {
		return false
	}
	includeGrouped := s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
	if includeGrouped {
		groupID = nil
	}
	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, groupID, []string{PlatformOpenAI}, includeGrouped)
	if err != nil {
		return false
	}
	found := false
	for i := range accounts {
		account := &accounts[i]
		if !account.SupportsOpenAIImageCapability(OpenAIImagesCapabilityNative) || !account.IsModelSupported(model) {
			continue
		}
		found = true
		if accountSupportsOpenAIImageRequestOptions(ctx, account) {
			return false
		}
	}
	return found
}

func buildOpenAINativeImagesBody(parsed *OpenAIImagesRequest, model string) ([]byte, error) {
	if !supportsOpenAINativeImageOptions(parsed) {
		return nil, infraerrors.BadRequest("UNSUPPORTED_NATIVE_IMAGE_OPTIONS", "Native-Image supports one PNG per request, without masks, compression, moderation, style or input_fidelity options")
	}
	payload := map[string]any{"model": model, "prompt": parsed.Prompt, "n": 1}
	for key, value := range map[string]string{"quality": parsed.Quality, "size": parsed.Size, "background": parsed.Background} {
		if value != "" {
			payload[key] = value
		}
	}
	if parsed.IsEdits() {
		images := make([]map[string]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
		for _, url := range parsed.InputImageURLs {
			images = append(images, map[string]string{"image_url": url})
		}
		for _, upload := range parsed.Uploads {
			url, err := openAIImageUploadToDataURL(upload)
			if err != nil {
				return nil, err
			}
			images = append(images, map[string]string{"image_url": url})
		}
		if len(images) == 0 {
			return nil, infraerrors.BadRequest("MISSING_IMAGE", "Native-Image edits require an input image")
		}
		payload["images"] = images
	}
	return json.Marshal(payload)
}

func (s *OpenAIGatewayService) buildOpenAINativeImagesRequest(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIImagesRequest, model, token string) (*http.Request, error) {
	body, err := buildOpenAINativeImagesBody(parsed, model)
	if err != nil {
		return nil, err
	}
	req, err := s.buildUpstreamRequest(withOpenAIImagesSelfBuiltRequest(ctx), c, account, body, token, false, "", false)
	if err != nil {
		return nil, err
	}
	req.URL.Path = "/backend-api/codex/images/generations"
	if parsed.IsEdits() {
		req.URL.Path = "/backend-api/codex/images/edits"
	}
	req.URL.RawPath = ""
	req.URL.RawQuery = ""
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", nativeImageCLIUserAgent)
	req.Header.Set("Originator", "codex_exec")
	req.Header.Del("Version")
	req.Header.Set("X-Codex-Image-Turn-Id", uuid.Must(uuid.NewV7()).String())
	req.Header.Del("OpenAI-Beta")
	return req, nil
}

func decodeOpenAINativeImages(body []byte) ([]openAIResponsesImageResult, openAIResponsesImageResult, []byte, int64, error) {
	if !gjson.ValidBytes(body) {
		return nil, openAIResponsesImageResult{}, nil, 0, fmt.Errorf("invalid native Images JSON response")
	}
	v := gjson.ParseBytes(body)
	meta := openAIResponsesImageResult{Quality: v.Get("quality").String(), Size: v.Get("size").String(), Background: v.Get("background").String(), OutputFormat: v.Get("output_format").String(), Model: v.Get("model").String()}
	var results []openAIResponsesImageResult
	for _, item := range v.Get("data").Array() {
		img := meta
		img.Result = item.Get("b64_json").String()
		img.RevisedPrompt = item.Get("revised_prompt").String()
		if img.Result != "" {
			results = append(results, img)
		}
	}
	if len(results) == 0 {
		return nil, meta, nil, 0, fmt.Errorf("native Images returned no image data")
	}
	var usage []byte
	if v.Get("usage").IsObject() {
		usage = []byte(v.Get("usage").Raw)
	}
	return results, meta, usage, v.Get("created").Int(), nil
}

func (s *OpenAIGatewayService) forwardOpenAIImagesNative(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIImagesRequest, mappedModel string) (*OpenAIForwardResult, error) {
	start := time.Now()
	model := strings.TrimSpace(mappedModel)
	if model == "" {
		model = strings.TrimSpace(parsed.Model)
	}
	if model == "" {
		model = "gpt-image-2"
	}
	if err := validateOpenAIImagesModel(model); err != nil {
		return nil, err
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	req, err := s.buildOpenAINativeImagesRequest(upstreamCtx, c, account, parsed, model, token)
	if err != nil {
		if infraerrors.Code(err) == http.StatusBadRequest {
			clientErr := &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "invalid_request_error", Code: infraerrors.Reason(err), Message: infraerrors.Message(err)}
			writeOpenAIImagesUpstreamErrorResponse(c, clientErr)
			return nil, clientErr
		}
		return nil, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("native Images request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
	}
	defer func(body io.ReadCloser) { _ = body.Close() }(resp.Body)
	if resp.StatusCode >= 400 {
		body := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, message, body) {
			disable := s.handleFailoverSideEffects(upstreamCtx, resp, account, body, model)
			return nil, s.newOpenAIAccountFailoverError(account, resp.StatusCode, resp.Header, body, message, disable, !disable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode))
		}
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, model)
	}
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	images, meta, usageRaw, created, err := decodeOpenAINativeImages(body)
	if err != nil {
		return nil, err
	}
	var outputSizes []string
	for _, img := range images {
		if size := detectOpenAIImageResultSize(img.Result); size != "" {
			outputSizes = append(outputSizes, size)
		} else if img.Size != "" {
			outputSizes = append(outputSizes, img.Size)
		}
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	output, err := buildOpenAIImagesAPIResponse(images, created, usageRaw, meta, parsed.ResponseFormat)
	if err != nil {
		return nil, err
	}
	if requestID := resp.Header.Get("x-request-id"); requestID != "" {
		c.Header("x-request-id", requestID)
	}
	if parsed.Stream {
		if parsed.PartialImages != nil {
			// Explicitly expose the final-only fallback; never fabricate previews.
			c.Header("X-Sub2api-Image-Preview", "final-only")
		}
		StopOpenAIImagesJSONKeepaliveCommitted(c)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming unavailable")
		}
		for _, img := range images {
			event := openAIImagesStreamPrefix(parsed) + ".completed"
			payload := buildOpenAIImagesStreamCompletedPayload(event, img, parsed.ResponseFormat, created, usageRaw)
			// Native Images returns formatted JSON. Raw usage retains its newlines,
			// so compact the complete payload before writing a single SSE data line.
			var compactPayload bytes.Buffer
			if err := json.Compact(&compactPayload, payload); err != nil {
				return nil, fmt.Errorf("encode native Images stream event: %w", err)
			}
			if writeErr := s.writeOpenAIImagesStreamEvent(c, flusher, event, compactPayload.Bytes()); writeErr != nil {
				return nil, writeErr
			}
		}
	} else {
		c.Data(resp.StatusCode, "application/json", output)
	}
	return &OpenAIForwardResult{RequestID: resp.Header.Get("x-request-id"), UpstreamHeaders: resp.Header.Clone(), ResponseHeaders: resp.Header.Clone(), Usage: usage, Model: parsed.Model, UpstreamModel: model, Stream: parsed.Stream, Duration: time.Since(start), ImageCount: len(images), ImageSize: parsed.SizeTier, ImageInputSize: parsed.Size, ImageOutputSizes: outputSizes}, nil
}

func (s *AccountTestService) testOpenAIImageNative(c *gin.Context, ctx context.Context, account *Account, model, prompt string) error {
	if err := validateOpenAIImageChannel(account); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	s.sendEvent(c, TestEvent{Type: "test_start", Model: model})
	s.sendEvent(c, TestEvent{Type: "content", Text: "Calling /backend-api/codex/images/generations (Native-Image)...\n"})
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: model, Prompt: prompt, N: 1}
	gateway := s.openaiGatewayService
	if gateway == nil {
		gateway = &OpenAIGatewayService{cfg: s.cfg, httpUpstream: s.httpUpstream, accountRepo: s.accountRepo}
	}
	token, _, err := gateway.GetAccessToken(ctx, account)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to obtain OAuth access token")
	}
	req, err := gateway.buildOpenAINativeImagesRequest(ctx, c, account, parsed, model, token)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIAccountTestUpstream(req, proxyURL, account, false)
	if err != nil {
		return s.sendErrorAndEnd(c, "Native-Image request failed")
	}
	defer func(body io.ReadCloser) { _ = body.Close() }(resp.Body)
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return s.sendErrorAndEnd(c, fmt.Sprintf("Native-Image HTTP %d: %s", resp.StatusCode, sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body))))
	}
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to read native Images response")
	}
	images, meta, usage, _, err := decodeOpenAINativeImages(body)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Actual quality: %s; size: %s; usage: %s\n", meta.Quality, meta.Size, usage)})
	for _, img := range images {
		mime := openAIImageOutputMIMEType(img.OutputFormat)
		s.sendEvent(c, TestEvent{Type: "image", ImageURL: "data:" + mime + ";base64," + img.Result, MimeType: mime})
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
