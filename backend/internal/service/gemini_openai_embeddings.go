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

type geminiOpenAIEmbeddingsRequest struct {
	Model string          `json:"model"`
	Input json.RawMessage `json:"input"`
}

type geminiOpenAIEmbeddingsResponse struct {
	Object string                      `json:"object"`
	Data   []geminiOpenAIEmbedding     `json:"data"`
	Model  string                      `json:"model"`
	Usage  geminiOpenAIEmbeddingsUsage `json:"usage"`
}

type geminiOpenAIEmbedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type geminiOpenAIEmbeddingsUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type geminiEmbeddingAPIResponse struct {
	Embedding *struct {
		Values []float64 `json:"values"`
	} `json:"embedding,omitempty"`
	Embeddings []struct {
		Values []float64 `json:"values"`
	} `json:"embeddings,omitempty"`
	UsageMetadata struct {
		PromptTokenCount int `json:"promptTokenCount"`
	} `json:"usageMetadata,omitempty"`
}

// ForwardOpenAICompatibleEmbeddings serves /v1beta/openai/embeddings through
// Gemini native embedContent and batchEmbedContents while returning OpenAI shape.
func (s *GeminiMessagesCompatService) ForwardOpenAICompatibleEmbeddings(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	var req geminiOpenAIEmbeddingsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeGeminiOpenAIEmbeddingsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, err
	}
	originalModel := strings.TrimSpace(req.Model)
	if originalModel == "" {
		writeGeminiOpenAIEmbeddingsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, errors.New("model is required")
	}
	inputs, err := parseGeminiOpenAIEmbeddingInputs(req.Input)
	if err != nil {
		writeGeminiOpenAIEmbeddingsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}

	mappedModel := originalModel
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(originalModel)
	}
	batch := len(inputs) > 1
	var upstreamBody []byte
	if batch {
		upstreamBody = buildGeminiBatchEmbedContentsRequest(mappedModel, inputs)
	} else {
		upstreamBody = buildGeminiEmbedContentRequest(inputs[0])
	}

	upstreamReq, requestIDHeader, err := s.buildGeminiOpenAIEmbeddingsRequest(ctx, account, mappedModel, batch, upstreamBody)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		writeGeminiOpenAIEmbeddingsError(c, http.StatusBadGateway, "upstream_error", err.Error())
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
		writeGeminiOpenAIEmbeddingsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
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
		writeGeminiOpenAIEmbeddingsError(c, resp.StatusCode, "upstream_error", upstreamMsg)
		return nil, fmt.Errorf("gemini upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeGeminiOpenAIEmbeddingsError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	openAIBody, usage, err := convertGeminiEmbeddingResponseToOpenAI(respBody, originalModel, len(inputs))
	if err != nil {
		writeGeminiOpenAIEmbeddingsError(c, http.StatusBadGateway, "api_error", "Failed to parse upstream response")
		return nil, err
	}

	if !c.Writer.Written() {
		c.Data(http.StatusOK, "application/json", openAIBody)
	}

	return &ForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		UpstreamModel: mappedModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *GeminiMessagesCompatService) buildGeminiOpenAIEmbeddingsRequest(
	ctx context.Context,
	account *Account,
	model string,
	batch bool,
	body []byte,
) (*http.Request, string, error) {
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, "", err
	}
	targetURL := geminiEmbeddingURL(normalizedBaseURL, model, batch)

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

func parseGeminiOpenAIEmbeddingInputs(raw json.RawMessage) ([]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, errors.New("input is required")
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return nil, errors.New("input cannot be empty")
		}
		return []string{single}, nil
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		if len(many) == 0 {
			return nil, errors.New("input cannot be empty")
		}
		for _, input := range many {
			if strings.TrimSpace(input) == "" {
				return nil, errors.New("input cannot contain empty strings")
			}
		}
		return many, nil
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return nil, errors.New("token array inputs are not supported")
	}
	return nil, errors.New("input must be a string or array of strings")
}

func buildGeminiEmbedContentRequest(input string) []byte {
	body, _ := json.Marshal(map[string]any{
		"content": geminiEmbeddingContent(input),
	})
	return body
}

func buildGeminiBatchEmbedContentsRequest(model string, inputs []string) []byte {
	requests := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		requests = append(requests, map[string]any{
			"model":   geminiEmbeddingBodyModel(model),
			"content": geminiEmbeddingContent(input),
		})
	}
	body, _ := json.Marshal(map[string]any{"requests": requests})
	return body
}

func geminiEmbeddingContent(input string) map[string]any {
	return map[string]any{
		"parts": []map[string]string{{"text": input}},
	}
}

func geminiEmbeddingBodyModel(model string) string {
	model = strings.TrimSpace(model)
	model = strings.TrimPrefix(model, "models/")
	return "models/" + model
}

func geminiEmbeddingURL(baseURL string, model string, batch bool) string {
	action := "embedContent"
	if batch {
		action = "batchEmbedContents"
	}
	model = strings.TrimSpace(model)
	model = strings.TrimPrefix(model, "models/")
	return fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), model, action)
}

func convertGeminiEmbeddingResponseToOpenAI(body []byte, model string, inputCount int) ([]byte, ClaudeUsage, error) {
	var parsed geminiEmbeddingAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, ClaudeUsage{}, err
	}

	vectors := make([][]float64, 0, inputCount)
	if parsed.Embedding != nil {
		vectors = append(vectors, parsed.Embedding.Values)
	}
	for _, embedding := range parsed.Embeddings {
		vectors = append(vectors, embedding.Values)
	}
	if len(vectors) == 0 {
		return nil, ClaudeUsage{}, errors.New("gemini embedding response did not include embeddings")
	}

	data := make([]geminiOpenAIEmbedding, 0, len(vectors))
	for i, values := range vectors {
		data = append(data, geminiOpenAIEmbedding{
			Object:    "embedding",
			Embedding: values,
			Index:     i,
		})
	}

	promptTokens := parsed.UsageMetadata.PromptTokenCount
	out := geminiOpenAIEmbeddingsResponse{
		Object: "list",
		Data:   data,
		Model:  model,
		Usage: geminiOpenAIEmbeddingsUsage{
			PromptTokens: promptTokens,
			TotalTokens:  promptTokens,
		},
	}
	outBody, err := json.Marshal(out)
	if err != nil {
		return nil, ClaudeUsage{}, err
	}
	return outBody, ClaudeUsage{InputTokens: promptTokens}, nil
}

func writeGeminiOpenAIEmbeddingsError(c *gin.Context, statusCode int, errType string, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
