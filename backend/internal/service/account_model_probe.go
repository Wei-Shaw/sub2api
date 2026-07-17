package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	ModelProbeModeOpenAIResponses       = "responses"
	ModelProbeModeOpenAIChatCompletions = "chat_completions"
	ModelProbeModeGeminiGenerateContent = "gemini_generate_content"
	ModelProbeModeAnthropicMessages     = "anthropic_messages"

	modelProbeTimeout       = 30 * time.Second
	modelProbeMaxModels     = 20
	modelProbeErrorBodySize = 2048
)

type ModelProbeListInput struct {
	Platform string
	BaseURL  string
	APIKey   string
}

type ModelProbeTestInput struct {
	Platform string
	BaseURL  string
	APIKey   string
	Mode     string
	Models   []string
}

type ModelProbeModel struct {
	ID          string `json:"id"`
	Object      string `json:"object,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
}

type ModelProbeListResult struct {
	Models []ModelProbeModel `json:"models"`
}

type ModelProbeTestResult struct {
	Results []ModelProbeSingleResult `json:"results"`
}

type ModelProbeSingleResult struct {
	Model  string `json:"model"`
	Mode   string `json:"mode"`
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (s *AccountTestService) ProbeModelList(ctx context.Context, input ModelProbeListInput) (ModelProbeListResult, error) {
	platform := normalizeModelProbePlatform(input.Platform)
	if platform == "" {
		return ModelProbeListResult{}, errors.New("unsupported platform")
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return ModelProbeListResult{}, errors.New("api key is required")
	}

	switch platform {
	case PlatformOpenAI:
		return s.probeOpenAIModelList(ctx, input.BaseURL, apiKey)
	case PlatformGemini:
		return s.probeGeminiModelList(ctx, input.BaseURL, apiKey)
	case PlatformAnthropic:
		return ModelProbeListResult{Models: defaultClaudeProbeModels()}, nil
	default:
		return ModelProbeListResult{}, errors.New("unsupported platform")
	}
}

func (s *AccountTestService) ProbeModels(ctx context.Context, input ModelProbeTestInput) (ModelProbeTestResult, error) {
	platform := normalizeModelProbePlatform(input.Platform)
	if platform == "" {
		return ModelProbeTestResult{}, errors.New("unsupported platform")
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return ModelProbeTestResult{}, errors.New("api key is required")
	}
	models := normalizeProbeModels(input.Models)
	if len(models) == 0 {
		return ModelProbeTestResult{}, errors.New("at least one model is required")
	}
	if len(models) > modelProbeMaxModels {
		return ModelProbeTestResult{}, fmt.Errorf("too many models: max %d", modelProbeMaxModels)
	}

	mode := normalizeModelProbeMode(platform, input.Mode)
	if mode == "" {
		return ModelProbeTestResult{}, errors.New("unsupported probe mode")
	}

	result := ModelProbeTestResult{Results: make([]ModelProbeSingleResult, 0, len(models))}
	for _, model := range models {
		result.Results = append(result.Results, s.probeSingleModel(ctx, platform, input.BaseURL, apiKey, mode, model))
	}
	return result, nil
}

func (s *AccountTestService) probeOpenAIModelList(ctx context.Context, baseURL, apiKey string) (ModelProbeListResult, error) {
	normalizedBaseURL, err := s.normalizeProbeBaseURL(baseURL, "https://api.openai.com")
	if err != nil {
		return ModelProbeListResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIProbeModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return ModelProbeListResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.doModelProbeRequest(req)
	if err != nil {
		return ModelProbeListResult{}, err
	}
	defer drainAndClose(resp.Body)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ModelProbeListResult{}, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, sanitizeProbeError(body, apiKey))
	}

	var parsed struct {
		Data []struct {
			ID          string `json:"id"`
			Object      string `json:"object"`
			OwnedBy     string `json:"owned_by"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ModelProbeListResult{}, fmt.Errorf("parse models response: %w", err)
	}

	models := make([]ModelProbeModel, 0, len(parsed.Data))
	seen := map[string]struct{}{}
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, ModelProbeModel{
			ID:          id,
			Object:      item.Object,
			DisplayName: firstNonEmptyProbeString(item.DisplayName, id),
			OwnedBy:     item.OwnedBy,
		})
	}

	return ModelProbeListResult{Models: models}, nil
}

func (s *AccountTestService) probeGeminiModelList(ctx context.Context, baseURL, apiKey string) (ModelProbeListResult, error) {
	normalizedBaseURL, err := s.normalizeProbeBaseURL(baseURL, geminicli.AIStudioBaseURL)
	if err != nil {
		return ModelProbeListResult{}, err
	}

	listURL := strings.TrimRight(normalizedBaseURL, "/") + "/v1beta/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return ModelProbeListResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := s.doModelProbeRequest(req)
	if err != nil {
		return ModelProbeListResult{}, err
	}
	defer drainAndClose(resp.Body)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ModelProbeListResult{}, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, sanitizeProbeError(body, apiKey))
	}

	var parsed struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ModelProbeListResult{}, fmt.Errorf("parse models response: %w", err)
	}

	models := make([]ModelProbeModel, 0, len(parsed.Models))
	seen := map[string]struct{}{}
	for _, item := range parsed.Models {
		id := strings.TrimPrefix(strings.TrimSpace(item.Name), "models/")
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, ModelProbeModel{
			ID:          id,
			Object:      "model",
			DisplayName: firstNonEmptyProbeString(item.DisplayName, id),
			OwnedBy:     "google",
		})
	}

	return ModelProbeListResult{Models: models}, nil
}

func (s *AccountTestService) probeSingleModel(ctx context.Context, platform, baseURL, apiKey, mode, model string) ModelProbeSingleResult {
	result := ModelProbeSingleResult{Model: model, Mode: mode}
	req, err := s.buildModelProbeRequest(ctx, platform, baseURL, apiKey, mode, model)
	if err != nil {
		result.Error = sanitizeProbeError([]byte(err.Error()), apiKey)
		return result
	}

	result = s.doSingleModelProbeRequest(req, apiKey, result)
	if shouldRetryOpenAIChatCompletionWithMaxTokens(platform, mode, result) {
		retryReq, retryErr := s.buildOpenAIChatCompletionsProbeRequest(ctx, baseURL, apiKey, openAIChatCompletionsLegacyMinimalProbePayload(model))
		if retryErr != nil {
			result.Error = sanitizeProbeError([]byte(retryErr.Error()), apiKey)
			return result
		}
		retryResult := ModelProbeSingleResult{Model: model, Mode: mode}
		return s.doSingleModelProbeRequest(retryReq, apiKey, retryResult)
	}
	return result
}

func (s *AccountTestService) doSingleModelProbeRequest(req *http.Request, apiKey string, result ModelProbeSingleResult) ModelProbeSingleResult {
	resp, err := s.doModelProbeRequest(req)
	if err != nil {
		result.Error = sanitizeProbeError([]byte(err.Error()), apiKey)
		return result
	}
	defer drainAndClose(resp.Body)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, modelProbeErrorBodySize))
	result.Status = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !result.OK {
		result.Error = fmt.Sprintf("upstream returned %d: %s", resp.StatusCode, sanitizeProbeError(body, apiKey))
	}
	return result
}

func (s *AccountTestService) buildModelProbeRequest(ctx context.Context, platform, baseURL, apiKey, mode, model string) (*http.Request, error) {
	switch {
	case platform == PlatformOpenAI && mode == ModelProbeModeOpenAIResponses:
		normalizedBaseURL, err := s.normalizeProbeBaseURL(baseURL, "https://api.openai.com")
		if err != nil {
			return nil, err
		}
		return newJSONProbeRequest(ctx, http.MethodPost, buildOpenAIResponsesURL(normalizedBaseURL), apiKey, openAIResponsesMinimalProbePayload(model))
	case platform == PlatformOpenAI && mode == ModelProbeModeOpenAIChatCompletions:
		return s.buildOpenAIChatCompletionsProbeRequest(ctx, baseURL, apiKey, openAIChatCompletionsMinimalProbePayload(model))
	case platform == PlatformGemini && mode == ModelProbeModeGeminiGenerateContent:
		normalizedBaseURL, err := s.normalizeProbeBaseURL(baseURL, geminicli.AIStudioBaseURL)
		if err != nil {
			return nil, err
		}
		return newGeminiProbeRequest(ctx, normalizedBaseURL, apiKey, model)
	case platform == PlatformAnthropic && mode == ModelProbeModeAnthropicMessages:
		normalizedBaseURL, err := s.normalizeProbeBaseURL(baseURL, "https://api.anthropic.com")
		if err != nil {
			return nil, err
		}
		return newAnthropicProbeRequest(ctx, normalizedBaseURL, apiKey, model)
	default:
		return nil, errors.New("unsupported probe mode")
	}
}

func (s *AccountTestService) buildOpenAIChatCompletionsProbeRequest(ctx context.Context, baseURL, apiKey string, payload []byte) (*http.Request, error) {
	normalizedBaseURL, err := s.normalizeProbeBaseURL(baseURL, "https://api.openai.com")
	if err != nil {
		return nil, err
	}
	return newJSONProbeRequest(ctx, http.MethodPost, buildOpenAIChatCompletionsURL(normalizedBaseURL), apiKey, payload)
}

func (s *AccountTestService) normalizeProbeBaseURL(baseURL, fallback string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = fallback
	}
	if s.cfg == nil {
		return "", errors.New("config is not available")
	}
	return s.validateUpstreamBaseURL(baseURL)
}

func (s *AccountTestService) doModelProbeRequest(req *http.Request) (*http.Response, error) {
	if s.httpUpstream == nil {
		return nil, errors.New("http upstream is not configured")
	}
	ctx, cancel := context.WithTimeout(req.Context(), modelProbeTimeout)
	req = req.WithContext(ctx)
	resp, err := s.httpUpstream.DoWithTLS(req, "", 0, 0, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelOnCloseReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

func newJSONProbeRequest(ctx context.Context, method, targetURL, apiKey string, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req, nil
}

func newGeminiProbeRequest(ctx context.Context, baseURL, apiKey, model string) (*http.Request, error) {
	targetURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(baseURL, "/"), url.PathEscape(model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geminiMinimalProbePayload()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)
	return req, nil
}

func newAnthropicProbeRequest(ctx context.Context, baseURL, apiKey, model string) (*http.Request, error) {
	targetURL := strings.TrimRight(baseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(anthropicMinimalProbePayload(model)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	return req, nil
}

func openAIResponsesMinimalProbePayload(model string) []byte {
	body, _ := json.Marshal(map[string]any{
		"model":             model,
		"input":             "ping",
		"max_output_tokens": 1,
	})
	return body
}

func openAIChatCompletionsMinimalProbePayload(model string) []byte {
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
		"max_completion_tokens": 1,
	})
	return body
}

func openAIChatCompletionsLegacyMinimalProbePayload(model string) []byte {
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
	})
	return body
}

func geminiMinimalProbePayload() []byte {
	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "ping"},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 1,
		},
	})
	return body
}

func anthropicMinimalProbePayload(model string) []byte {
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1,
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
	})
	return body
}

func buildOpenAIProbeModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

func normalizeModelProbePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformOpenAI:
		return PlatformOpenAI
	case PlatformGemini:
		return PlatformGemini
	case PlatformAnthropic, "claude":
		return PlatformAnthropic
	default:
		return ""
	}
}

func normalizeModelProbeMode(platform, mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch platform {
	case PlatformOpenAI:
		switch mode {
		case "", ModelProbeModeOpenAIResponses:
			return ModelProbeModeOpenAIResponses
		case ModelProbeModeOpenAIChatCompletions:
			return ModelProbeModeOpenAIChatCompletions
		}
	case PlatformGemini:
		if mode == "" || mode == ModelProbeModeGeminiGenerateContent {
			return ModelProbeModeGeminiGenerateContent
		}
	case PlatformAnthropic:
		if mode == "" || mode == ModelProbeModeAnthropicMessages {
			return ModelProbeModeAnthropicMessages
		}
	}
	return ""
}

func normalizeProbeModels(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		normalized = append(normalized, model)
	}
	return normalized
}

func sanitizeProbeError(body []byte, apiKey string) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = "empty response"
	}
	message = redactProbeAPIKeyFragments(message, apiKey)
	return logredact.RedactText(message, "api_key", "key", "token", "authorization", "x-api-key", "x-goog-api-key")
}

func redactProbeAPIKeyFragments(message, apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return message
	}
	message = strings.ReplaceAll(message, apiKey, "***")

	maxFragmentLen := len(apiKey) - 1
	if maxFragmentLen > 32 {
		maxFragmentLen = 32
	}
	for fragmentLen := maxFragmentLen; fragmentLen >= 8; fragmentLen-- {
		prefix := apiKey[:fragmentLen]
		suffix := apiKey[len(apiKey)-fragmentLen:]
		message = strings.ReplaceAll(message, prefix, "***")
		if suffix != prefix {
			message = strings.ReplaceAll(message, suffix, "***")
		}
	}
	return message
}

func shouldRetryOpenAIChatCompletionWithMaxTokens(platform, mode string, result ModelProbeSingleResult) bool {
	if platform != PlatformOpenAI || mode != ModelProbeModeOpenAIChatCompletions || result.OK {
		return false
	}
	return strings.Contains(strings.ToLower(result.Error), "max_completion_tokens")
}

func defaultClaudeProbeModels() []ModelProbeModel {
	models := make([]ModelProbeModel, 0, len(claude.DefaultModels))
	for _, model := range claude.DefaultModels {
		models = append(models, ModelProbeModel{
			ID:          model.ID,
			Object:      model.Type,
			DisplayName: model.DisplayName,
			OwnedBy:     "anthropic",
		})
	}
	return models
}

func firstNonEmptyProbeString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}
