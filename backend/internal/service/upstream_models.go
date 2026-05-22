package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
)

const upstreamModelsBodyLimit int64 = 8 << 20

// UpstreamModelSyncErrorKind classifies model sync failures for safe HTTP mapping.
type UpstreamModelSyncErrorKind string

const (
	// UpstreamModelSyncErrorConfiguration means the account or server configuration cannot perform the sync.
	UpstreamModelSyncErrorConfiguration UpstreamModelSyncErrorKind = "configuration"
	// UpstreamModelSyncErrorUnsupported means the account format is intentionally unsupported for live model sync.
	UpstreamModelSyncErrorUnsupported UpstreamModelSyncErrorKind = "unsupported"
	// UpstreamModelSyncErrorUpstream means the configured upstream failed or returned an unusable response.
	UpstreamModelSyncErrorUpstream UpstreamModelSyncErrorKind = "upstream"
)

// UpstreamModelSyncError keeps internal failure details wrapped while exposing a safe client message.
type UpstreamModelSyncError struct {
	Kind    UpstreamModelSyncErrorKind
	Message string
	Err     error
}

func (e *UpstreamModelSyncError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *UpstreamModelSyncError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// SafeMessage returns the sanitized message that can be sent to API clients.
func (e *UpstreamModelSyncError) SafeMessage() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "Failed to sync upstream models"
	}
	return e.Message
}

func newUpstreamModelSyncConfigError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorConfiguration, Message: message, Err: err}
}

func newUpstreamModelSyncUnsupportedError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorUnsupported, Message: message, Err: err}
}

func newUpstreamModelSyncUpstreamError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorUpstream, Message: message, Err: err}
}

// FetchUpstreamSupportedModels fetches the live model list from the account's upstream API format.
func (s *AccountTestService) FetchUpstreamSupportedModels(ctx context.Context, account *Account) ([]string, error) {
	if s == nil {
		return nil, newUpstreamModelSyncConfigError("Account test service is not configured", nil)
	}
	if account == nil {
		return nil, newUpstreamModelSyncConfigError("Account is required", nil)
	}

	if account.Platform == PlatformAntigravity && account.Type != AccountTypeAPIKey {
		return s.fetchAntigravityOAuthUpstreamModels(ctx, account)
	}

	if s.httpUpstream == nil {
		return nil, newUpstreamModelSyncConfigError("Upstream HTTP client is not configured", nil)
	}

	req, err := s.buildUpstreamModelsRequest(ctx, account)
	if err != nil {
		return nil, err
	}

	proxyURL := upstreamModelsProxyURL(account)
	resp, err := s.doUpstreamModelsRequest(req, proxyURL, account)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to request upstream model list", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamModelsBodyLimit+1))
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to read upstream model list", err)
	}
	if int64(len(body)) > upstreamModelsBodyLimit {
		return nil, newUpstreamModelSyncUpstreamError("Upstream model list response is too large", fmt.Errorf("response exceeds %d bytes", upstreamModelsBodyLimit))
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newUpstreamModelSyncUpstreamError(
			fmt.Sprintf("Upstream model list request failed with HTTP %d", resp.StatusCode),
			fmt.Errorf("upstream model list returned HTTP %d", resp.StatusCode),
		)
	}

	models, err := extractUpstreamModelIDs(body)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Upstream model list response was not valid JSON", err)
	}
	if len(models) == 0 {
		return nil, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", nil)
	}

	return models, nil
}

func (s *AccountTestService) buildUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	switch {
	case account.Platform == PlatformAntigravity:
		return s.buildAntigravityAPIKeyModelsRequest(ctx, account)
	case account.IsOpenAI():
		return s.buildOpenAIUpstreamModelsRequest(ctx, account)
	case account.IsGemini():
		return s.buildGeminiUpstreamModelsRequest(ctx, account)
	case account.IsAnthropic():
		return s.buildAnthropicUpstreamModelsRequest(ctx, account)
	default:
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported platform for upstream model sync: %s", account.Platform), nil,
		)
	}
}

func (s *AccountTestService) buildAnthropicUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account.IsBedrock() || account.Type == AccountTypeServiceAccount {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Anthropic account type for upstream model sync: %s", account.Type), nil,
		)
	}

	baseURL := "https://api.anthropic.com"
	authHeaderName := ""
	authHeaderValue := ""
	betaHeader := ""

	if account.IsOAuth() {
		accessToken := strings.TrimSpace(account.GetCredential("access_token"))
		if accessToken == "" && s.claudeTokenProvider != nil {
			token, tokenErr := s.claudeTokenProvider.GetAccessToken(ctx, account)
			if tokenErr != nil {
				return nil, newUpstreamModelSyncUpstreamError("Failed to get Anthropic access token", tokenErr)
			}
			accessToken = strings.TrimSpace(token)
		}
		if accessToken == "" {
			return nil, newUpstreamModelSyncConfigError("No Anthropic access token is available", nil)
		}
		authHeaderName = "Authorization"
		authHeaderValue = "Bearer " + accessToken
		betaHeader = claude.DefaultBetaHeader
	} else if account.Type == AccountTypeAPIKey {
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, newUpstreamModelSyncConfigError("No Anthropic API key is available", nil)
		}
		baseURL = account.GetBaseURL()
		if strings.TrimSpace(baseURL) == "" {
			baseURL = "https://api.anthropic.com"
		}
		authHeaderName = "x-api-key"
		authHeaderValue = apiKey
		betaHeader = claude.APIKeyBetaHeader
	} else {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Anthropic account type for upstream model sync: %s", account.Type), nil,
		)
	}

	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Anthropic base URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildV1ModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Anthropic model list URL", err)
	}
	for key, value := range claude.DefaultHeaders {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set(authHeaderName, authHeaderValue)
	return req, nil
}

func (s *AccountTestService) buildAntigravityAPIKeyModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account.Type != AccountTypeAPIKey {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Antigravity account type for upstream model sync: %s", account.Type), nil,
		)
	}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, newUpstreamModelSyncConfigError("No Antigravity API key is available", nil)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(account.GetCredential("base_url")), "/")
	if baseURL == "" {
		return nil, newUpstreamModelSyncConfigError("Antigravity API-key base URL is required for upstream model sync", nil)
	}
	if !strings.HasSuffix(strings.ToLower(baseURL), "/antigravity") {
		return nil, newUpstreamModelSyncUnsupportedError(
			"Antigravity API-key upstream model sync requires a compatible gateway base URL ending in /antigravity; use Antigravity OAuth for official Cloud Code upstreams",
			nil,
		)
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Antigravity base URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildV1ModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Antigravity model list URL", err)
	}
	for key, value := range claude.DefaultHeaders {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
	req.Header.Set("x-api-key", apiKey)
	return req, nil
}

func (s *AccountTestService) buildOpenAIUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account.Type != AccountTypeAPIKey {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported OpenAI account type for upstream model sync: %s", account.Type), nil,
		)
	}
	apiKey := strings.TrimSpace(account.GetOpenAIApiKey())
	if apiKey == "" {
		return nil, newUpstreamModelSyncConfigError("No OpenAI API key is available", nil)
	}

	baseURL := account.GetOpenAIBaseURL()
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid OpenAI base URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid OpenAI model list URL", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req, nil
}

func (s *AccountTestService) buildGeminiUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = geminicli.AIStudioBaseURL
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Gemini base URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildGeminiModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Gemini model list URL", err)
	}
	req.Header.Set("Accept", "application/json")

	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, newUpstreamModelSyncConfigError("No Gemini API key is available", nil)
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case AccountTypeOAuth:
		if strings.TrimSpace(account.GetCredential("project_id")) != "" {
			return nil, newUpstreamModelSyncUnsupportedError("Gemini Code Assist model listing is not supported by this sync button", nil)
		}
		if s.geminiTokenProvider == nil {
			return nil, newUpstreamModelSyncConfigError("Gemini token provider is not configured", nil)
		}
		accessToken, tokenErr := s.geminiTokenProvider.GetAccessToken(ctx, account)
		if tokenErr != nil {
			return nil, newUpstreamModelSyncUpstreamError("Failed to get Gemini access token", tokenErr)
		}
		accessToken = strings.TrimSpace(accessToken)
		if accessToken == "" {
			return nil, newUpstreamModelSyncConfigError("No Gemini access token is available", nil)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Gemini account type for upstream model sync: %s", account.Type), nil,
		)
	}

	return req, nil
}

func (s *AccountTestService) fetchAntigravityOAuthUpstreamModels(ctx context.Context, account *Account) ([]string, error) {
	if s.antigravityGatewayService == nil || s.antigravityGatewayService.GetTokenProvider() == nil {
		return nil, newUpstreamModelSyncConfigError("Antigravity token provider is not configured", nil)
	}

	accessToken, err := s.antigravityGatewayService.GetTokenProvider().GetAccessToken(ctx, account)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to get Antigravity access token", err)
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, newUpstreamModelSyncConfigError("No Antigravity access token is available", nil)
	}

	client, err := antigravity.NewClient(upstreamModelsProxyURL(account))
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Failed to configure Antigravity client", err)
	}
	modelsResp, _, err := client.FetchAvailableModels(ctx, accessToken, strings.TrimSpace(account.GetCredential("project_id")))
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to fetch Antigravity available models", err)
	}
	if modelsResp == nil || len(modelsResp.Models) == 0 {
		return nil, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", nil)
	}

	models := make([]string, 0, len(modelsResp.Models))
	for modelID := range modelsResp.Models {
		models = append(models, strings.TrimSpace(modelID))
	}
	return dedupeAndSortModelIDs(models), nil
}

func (s *AccountTestService) doUpstreamModelsRequest(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if s.tlsFPProfileService == nil {
		return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, nil)
	}
	return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
}

func upstreamModelsProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

func buildV1ModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/v1/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

func buildOpenAIModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/v1/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

func buildGeminiModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/v1beta/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1beta") {
		return normalized + "/models"
	}
	return normalized + "/v1beta/models"
}

type upstreamModelEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func extractUpstreamModelIDs(body []byte) ([]string, error) {
	var response struct {
		Data   []upstreamModelEntry `json:"data"`
		Models []upstreamModelEntry `json:"models"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		var arrayResponse []upstreamModelEntry
		if arrayErr := json.Unmarshal(body, &arrayResponse); arrayErr != nil {
			return nil, fmt.Errorf("parse upstream model list: %w", err)
		}

		models := make([]string, 0, len(arrayResponse))
		for _, entry := range arrayResponse {
			models = append(models, upstreamModelEntryID(entry))
		}
		return dedupeAndSortModelIDs(models), nil
	}

	models := make([]string, 0, len(response.Data)+len(response.Models))
	for _, entry := range response.Data {
		models = append(models, upstreamModelEntryID(entry))
	}
	for _, entry := range response.Models {
		models = append(models, upstreamModelEntryID(entry))
	}

	if len(models) == 0 {
		var arrayResponse []upstreamModelEntry
		if err := json.Unmarshal(body, &arrayResponse); err == nil {
			for _, entry := range arrayResponse {
				models = append(models, upstreamModelEntryID(entry))
			}
		}
	}

	return dedupeAndSortModelIDs(models), nil
}

func upstreamModelEntryID(entry upstreamModelEntry) string {
	modelID := strings.TrimSpace(entry.ID)
	if modelID == "" {
		modelID = strings.TrimSpace(entry.Name)
	}
	return strings.TrimPrefix(modelID, "models/")
}

func dedupeAndSortModelIDs(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}
