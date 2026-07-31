package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
		return "", errors.New("account is nil")
	}
	if validateBaseURL == nil {
		return "", errors.New("base URL validator is nil")
	}

	model = strings.TrimSpace(model)
	if model == "" {
		return "", errors.New("missing model")
	}
	switch action {
	case "generateContent", "streamGenerateContent", "countTokens":
	default:
		return "", fmt.Errorf("unsupported gemini action: %s", action)
	}

	baseURL := account.GetGeminiBaseURL(geminiAPIKeyDefaultBaseURL(account))
	normalizedBaseURL, err := validateBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	trimmedBaseURL := strings.TrimRight(normalizedBaseURL, "/")
	var fullURL string
	if account.IsGeminiVertexAPIKey() {
		model = normalizeGeminiAPIKeyModelID(model)
		if model == "" {
			return "", errors.New("missing model")
		}
		escapedModel := strings.ReplaceAll(url.PathEscape(model), "%40", "@")
		fullURL = fmt.Sprintf("%s/v1/publishers/google/models/%s:%s", trimmedBaseURL, escapedModel, action)
	} else {
		// Preserve legacy AI Studio model paths exactly as configured.
		fullURL = fmt.Sprintf("%s/v1beta/models/%s:%s", trimmedBaseURL, model, action)
	}
	if stream {
		fullURL += "?alt=sse"
	}
	return fullURL, nil
}

func normalizeGeminiAPIKeyModelID(model string) string {
	model = strings.Trim(strings.TrimSpace(model), "/")
	model = strings.TrimPrefix(model, "publishers/google/models/")
	model = strings.TrimPrefix(model, "models/")
	return strings.TrimSpace(model)
}

func buildGeminiVertexModelsFallback(path string) (*UpstreamHTTPResult, bool, error) {
	path = strings.TrimSpace(path)
	headers := http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}

	switch {
	case path == "/v1beta/models":
		body, err := json.Marshal(gemini.FallbackModelsList())
		if err != nil {
			return nil, true, err
		}
		return &UpstreamHTTPResult{StatusCode: http.StatusOK, Headers: headers, Body: body}, true, nil
	case strings.HasPrefix(path, "/v1beta/models/"):
		model := strings.TrimSpace(strings.TrimPrefix(path, "/v1beta/models/"))
		if model == "" {
			return nil, true, errors.New("invalid path")
		}
		body, err := json.Marshal(gemini.FallbackModel(model))
		if err != nil {
			return nil, true, err
		}
		return &UpstreamHTTPResult{StatusCode: http.StatusOK, Headers: headers, Body: body}, true, nil
	default:
		return nil, false, nil
	}
}
