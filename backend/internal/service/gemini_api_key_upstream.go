package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
)

func geminiAPIKeyDefaultBaseURL(account *Account) string {
	if account != nil && account.IsGeminiVertexAPIKey() {
		return geminicli.VertexExpressBaseURL
	}
	return geminicli.AIStudioBaseURL
}

func buildGeminiAPIKeyUpstreamURL(
	account *Account,
	validateBaseURL func(string) (string, error),
	model string,
	action string,
	stream bool,
) (string, error) {
	if account == nil {
		return "", fmt.Errorf("account is nil")
	}
	baseURL := account.GetGeminiBaseURL(geminiAPIKeyDefaultBaseURL(account))
	normalizedBaseURL, err := validateBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	trimmedBaseURL := strings.TrimRight(normalizedBaseURL, "/")
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		return "", fmt.Errorf("missing model")
	}

	var fullURL string
	if account.IsGeminiVertexAPIKey() {
		resource := geminiVertexModelResource(trimmedModel)
		fullURL = fmt.Sprintf("%s/v1beta1/%s:%s", trimmedBaseURL, resource, action)
	} else {
		fullURL = fmt.Sprintf("%s/v1beta/models/%s:%s", trimmedBaseURL, trimmedModel, action)
	}

	if stream {
		fullURL += "?alt=sse"
	}
	return fullURL, nil
}

func geminiVertexModelResource(model string) string {
	trimmed := strings.Trim(strings.TrimSpace(model), "/")
	switch {
	case strings.HasPrefix(trimmed, "publishers/"):
		return trimmed
	case strings.HasPrefix(trimmed, "models/"):
		trimmed = strings.TrimPrefix(trimmed, "models/")
	}
	return "publishers/google/models/" + trimmed
}

func buildGeminiVertexModelsFallback(path string) (*UpstreamHTTPResult, bool, error) {
	trimmedPath := strings.TrimSpace(path)
	headers := http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}

	switch {
	case trimmedPath == "/v1beta/models":
		body, err := json.Marshal(gemini.FallbackModelsList())
		if err != nil {
			return nil, true, err
		}
		return &UpstreamHTTPResult{
			StatusCode: http.StatusOK,
			Headers:    headers,
			Body:       body,
		}, true, nil
	case strings.HasPrefix(trimmedPath, "/v1beta/models/"):
		modelName := strings.TrimSpace(strings.TrimPrefix(trimmedPath, "/v1beta/models/"))
		if modelName == "" {
			return nil, true, fmt.Errorf("invalid path")
		}
		body, err := json.Marshal(gemini.FallbackModel(modelName))
		if err != nil {
			return nil, true, err
		}
		return &UpstreamHTTPResult{
			StatusCode: http.StatusOK,
			Headers:    headers,
			Body:       body,
		}, true, nil
	default:
		return nil, false, nil
	}
}
