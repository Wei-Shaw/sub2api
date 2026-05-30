package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/gin-gonic/gin"
)

type geminiOpenAIImageGenerationRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              *int   `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type geminiOpenAIImagesResponse struct {
	Created int64                          `json:"created"`
	Data    []geminiOpenAIImagesDataObject `json:"data"`
}

type geminiOpenAIImagesDataObject struct {
	B64JSON       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ForwardOpenAICompatibleImagesGenerations serves
// /v1beta/openai/images/generations through Gemini native generateContent.
func (s *GeminiMessagesCompatService) ForwardOpenAICompatibleImagesGenerations(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	req, err := parseGeminiOpenAIImageGenerationRequest(body)
	if err != nil {
		writeGeminiOpenAIImagesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}

	originalModel := strings.TrimSpace(req.Model)
	mappedModel := originalModel
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(originalModel)
	}
	upstreamBody := buildGeminiImageGenerateContentRequest(req.Prompt, req.Size)

	upstreamReq, requestIDHeader, err := s.buildGeminiOpenAIImagesRequest(ctx, account, mappedModel, upstreamBody)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		writeGeminiOpenAIImagesError(c, http.StatusBadGateway, "upstream_error", err.Error())
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeGeminiOpenAIImagesError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("Gemini upstream error: %d", resp.StatusCode)
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
		writeGeminiOpenAIImagesError(c, resp.StatusCode, "upstream_error", upstreamMsg)
		return nil, fmt.Errorf("gemini upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeGeminiOpenAIImagesError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	images, revisedPrompt, usage, err := collectGeminiOpenAIImages(respBody)
	if err != nil {
		writeGeminiOpenAIImagesError(c, http.StatusBadGateway, "api_error", "Failed to parse upstream response")
		return nil, err
	}

	if !c.Writer.Written() {
		c.Data(http.StatusOK, "application/json", buildGeminiOpenAIImagesResponse(time.Now().Unix(), images, revisedPrompt))
	}

	imageInputSize := strings.TrimSpace(req.Size)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	return &ForwardResult{
		RequestID:      requestID,
		Usage:          usage,
		Model:          originalModel,
		UpstreamModel:  mappedModel,
		Stream:         false,
		Duration:       time.Since(startTime),
		ImageCount:     len(images),
		ImageSize:      imageSize,
		ImageInputSize: imageInputSize,
	}, nil
}

func parseGeminiOpenAIImageGenerationRequest(body []byte) (*geminiOpenAIImageGenerationRequest, error) {
	var req geminiOpenAIImageGenerationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, errors.New("Failed to parse request body")
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.Size = strings.TrimSpace(req.Size)
	req.ResponseFormat = strings.ToLower(strings.TrimSpace(req.ResponseFormat))
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if req.Prompt == "" {
		return nil, errors.New("prompt is required")
	}
	if !isImageGenerationModel(req.Model) {
		return nil, fmt.Errorf("images/generations requires an image generation model, got %q", req.Model)
	}
	switch req.ResponseFormat {
	case "", "b64_json":
	case "url":
		return nil, errors.New("response_format=url is not supported")
	default:
		return nil, fmt.Errorf("unsupported response_format %q", req.ResponseFormat)
	}
	if req.N != nil {
		switch {
		case *req.N == 1:
		case *req.N > 1:
			return nil, errors.New("n greater than 1 is not supported")
		default:
			return nil, errors.New("n must be 1")
		}
	}
	return &req, nil
}

func buildGeminiImageGenerateContentRequest(prompt string, size string) []byte {
	generationConfig := map[string]any{
		"responseModalities": []string{"TEXT", "IMAGE"},
	}
	if strings.TrimSpace(size) != "" {
		generationConfig["imageConfig"] = map[string]any{
			"imageSize": normalizeOpenAIImageSizeTier(size),
		}
	}
	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": generationConfig,
	})
	return body
}

func (s *GeminiMessagesCompatService) buildGeminiOpenAIImagesRequest(
	ctx context.Context,
	account *Account,
	model string,
	body []byte,
) (*http.Request, string, error) {
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, "", err
	}
	targetURL := geminiImageGenerateContentURL(normalizedBaseURL, model)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	releaseUpstreamCtx()
	if err != nil {
		return nil, "", err
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/json")

	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, "", errors.New("gemini api_key not configured")
		}
		upstreamReq.Header.Set("x-goog-api-key", apiKey)
	case AccountTypeOAuth, AccountTypeServiceAccount:
		if s.tokenProvider == nil {
			return nil, "", errors.New("gemini token provider not configured")
		}
		accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, "", fmt.Errorf("unsupported account type: %s", account.Type)
	}

	return upstreamReq, "x-request-id", nil
}

func geminiImageGenerateContentURL(baseURL string, model string) string {
	model = strings.TrimSpace(model)
	model = strings.TrimPrefix(model, "models/")
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(baseURL, "/"), model)
}

func collectGeminiOpenAIImages(raw []byte) ([]string, string, ClaudeUsage, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", ClaudeUsage{}, err
	}

	var images []string
	revisedPrompt := ""
	if candidates, ok := payload["candidates"].([]any); ok {
		for _, candidate := range candidates {
			cm, ok := candidate.(map[string]any)
			if !ok {
				continue
			}
			content, ok := cm["content"].(map[string]any)
			if !ok {
				continue
			}
			parts, ok := content["parts"].([]any)
			if !ok {
				continue
			}
			for _, part := range parts {
				pm, ok := part.(map[string]any)
				if !ok {
					continue
				}
				if revisedPrompt == "" {
					if text, _ := pm["text"].(string); strings.TrimSpace(text) != "" {
						revisedPrompt = text
					}
				}
				if data := geminiInlineImageData(pm["inlineData"]); data != "" {
					images = append(images, data)
					continue
				}
				if data := geminiInlineImageData(pm["inline_data"]); data != "" {
					images = append(images, data)
				}
			}
		}
	}
	if len(images) == 0 {
		return nil, revisedPrompt, ClaudeUsage{}, errors.New("gemini image response did not include image data")
	}

	usage := ClaudeUsage{}
	if usageMetadata, ok := payload["usageMetadata"].(map[string]any); ok {
		usage.InputTokens = intNumberFromAny(usageMetadata["promptTokenCount"])
		usage.OutputTokens = intNumberFromAny(usageMetadata["candidatesTokenCount"])
		usage.ImageOutputTokens = intNumberFromAny(usageMetadata["imageOutputTokenCount"])
	}
	return images, revisedPrompt, usage, nil
}

func geminiInlineImageData(v any) string {
	inline, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	data, _ := inline["data"].(string)
	if strings.TrimSpace(data) == "" {
		return ""
	}
	if mimeType, _ := inline["mimeType"].(string); mimeType != "" && !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return ""
	}
	if mimeType, _ := inline["mime_type"].(string); mimeType != "" && !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return ""
	}
	return data
}

func intNumberFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func buildGeminiOpenAIImagesResponse(created int64, images []string, revisedPrompt string) []byte {
	data := make([]geminiOpenAIImagesDataObject, 0, len(images))
	for _, image := range images {
		item := geminiOpenAIImagesDataObject{B64JSON: image}
		if strings.TrimSpace(revisedPrompt) != "" {
			item.RevisedPrompt = revisedPrompt
		}
		data = append(data, item)
	}
	body, _ := json.Marshal(geminiOpenAIImagesResponse{
		Created: created,
		Data:    data,
	})
	return body
}

func writeGeminiOpenAIImagesError(c *gin.Context, statusCode int, errType string, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
