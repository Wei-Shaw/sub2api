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

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	openai_decisions "github.com/Wei-Shaw/sub2api/internal/pkg/openai_decisions"
	"github.com/gin-gonic/gin"
)

const openAIDecisionsUpstreamEndpoint = openai_decisions.Endpoint

type OpenAIDecisionsForwardResult struct {
	OpenAIForwardResult
	StatusCode  int
	Body        []byte
	ContentType string
}

func (s *OpenAIGatewayService) ForwardDecisions(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIDecisionsForwardResult, error) {
	ClearActualOpenAIUpstreamEndpoint(c)
	if s == nil || account == nil || c == nil {
		return nil, errors.New("service, context, and account are required")
	}
	if account.Platform != PlatformOpenAI || (account.Type != AccountTypeAPIKey && account.Type != AccountTypeUpstream) {
		return nil, errors.New("OpenAI Decisions requires an API key or upstream account")
	}
	parsed, err := openai_decisions.ParseRequest(body)
	if err != nil {
		return nil, err
	}

	credAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
		}
		credAccount = resolved
	}
	if err := validateOpenAIDecisionsCredentials(credAccount.Platform, credAccount.Type, credAccount.Credentials); err != nil {
		return nil, err
	}
	protocol := credAccount.GetOpenAIDecisionsProtocol()
	if protocol != "openai" && protocol != "openrouter" {
		return nil, errors.New("invalid Decisions protocol")
	}
	upstreamModel, endpoint := openai_decisions.Model, openai_decisions.Endpoint
	originalBody := body
	if protocol == "openrouter" {
		if parsed.ImageCount > 1 {
			return nil, errors.New("OpenRouter Decisions supports at most one image")
		}
		body, err = openai_decisions.EncodeOpenRouterRequest(originalBody)
		if err != nil {
			return nil, err
		}
		upstreamModel, endpoint = openai_decisions.OpenRouterModel, openai_decisions.OpenRouterPath
	}
	baseURL := strings.TrimSpace(credAccount.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = "https://api.openai.com"
		if protocol == "openrouter" {
			baseURL = "https://openrouter.ai"
		}
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	targetURL := buildOpenAIEndpointURL(validatedURL, endpoint)
	if protocol == "openrouter" {
		targetURL, err = buildOpenRouterDecisionsURL(validatedURL)
		if err != nil {
			return nil, err
		}
	}
	key := strings.TrimSpace(credAccount.GetCredential("api_key"))
	if key == "" {
		return nil, errors.New("api_key not found in credentials")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, key)
	if err != nil {
		return nil, err
	}
	for name, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Request != nil {
		for _, name := range []string{"User-Agent", "Accept-Language"} {
			for _, value := range c.Request.Header.Values(name) {
				req.Header.Add(name, value)
			}
		}
	}
	credAccount.ApplyHeaderOverrides(req.Header)
	SetActualOpenAIUpstreamEndpoint(c, endpoint)
	SetOpsUpstreamModel(c, upstreamModel)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	started := time.Now()
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read Decisions response: %w", err)
	}
	if len(responseBody) > 4<<20 {
		return nil, errors.New("Decisions response exceeds the gateway size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, message, responseBody) && resp.StatusCode != 400 && resp.StatusCode != 413 && resp.StatusCode != 422 {
			shouldDisable := s.handleFailoverSideEffects(ctx, resp, account, responseBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, responseBody, message, retryableOnSameAccount)
		}
		return &OpenAIDecisionsForwardResult{
			StatusCode:  resp.StatusCode,
			Body:        responseBody,
			ContentType: decisionsContentType(resp.Header.Get("Content-Type")),
			OpenAIForwardResult: OpenAIForwardResult{
				RequestID:        strings.TrimSpace(resp.Header.Get("x-request-id")),
				UpstreamHeaders:  resp.Header.Clone(),
				Model:            openai_decisions.Model,
				UpstreamModel:    upstreamModel,
				UpstreamEndpoint: endpoint,
				Duration:         time.Since(started),
			},
		}, nil
	}
	observedModel := openai_decisions.Model
	if protocol == "openrouter" {
		var observed struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(responseBody, &observed)
		observedModel = observed.Model
		normalized, _, decodeErr := openai_decisions.DecodeOpenRouterResponse(originalBody, responseBody)
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid OpenRouter Decisions response: %w", decodeErr)
		}
		responseBody = normalized
	}
	usage, err := openai_decisions.DecodeResponse(originalBody, responseBody)
	if err != nil {
		return nil, fmt.Errorf("invalid Decisions response: %w", err)
	}
	return &OpenAIDecisionsForwardResult{
		StatusCode:  resp.StatusCode,
		Body:        responseBody,
		ContentType: decisionsContentType(resp.Header.Get("Content-Type")),
		OpenAIForwardResult: OpenAIForwardResult{
			RequestID:       strings.TrimSpace(resp.Header.Get("x-request-id")),
			UpstreamHeaders: resp.Header.Clone(),
			Usage: OpenAIUsage{
				InputTokens:              usage.InputTokens,
				OutputTokens:             usage.OutputTokens,
				CacheReadInputTokens:     usage.CachedTokens,
				CacheCreationInputTokens: usage.CacheWriteTokens,
			},
			Model:                 openai_decisions.Model,
			BillingModel:          upstreamModel,
			UpstreamResponseModel: observedModel,
			UpstreamModel:         upstreamModel,
			UpstreamEndpoint:      endpoint,
			Duration:              time.Since(started),
		},
	}, nil
}

func decisionsContentType(raw string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "application/json") {
		return raw
	}
	return "application/json"
}

func (a *Account) GetOpenAIDecisionsProtocol() string {
	if a == nil {
		return "openai"
	}
	protocol := strings.TrimSpace(a.GetCredential("openai_decisions_protocol"))
	if protocol == "" {
		return "openai"
	}
	return protocol
}

func validateOpenAIDecisionsCredentials(platform, accountType string, credentials map[string]any) error {
	raw, exists := credentials["openai_decisions_protocol"]
	if !exists || raw == nil {
		return nil
	}
	protocol, ok := raw.(string)
	if !ok || (protocol != "openai" && protocol != "openrouter") {
		return infraerrors.BadRequest("OPENAI_DECISIONS_PROTOCOL_INVALID", "openai_decisions_protocol must be openai or openrouter")
	}
	if platform != PlatformOpenAI || (accountType != AccountTypeAPIKey && accountType != AccountTypeUpstream) {
		return infraerrors.BadRequest("OPENAI_DECISIONS_PROTOCOL_INVALID", "Decisions protocol requires an OpenAI API-key or upstream account")
	}
	return nil
}

func buildOpenRouterDecisionsURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	for _, suffix := range []string{openai_decisions.OpenRouterPath, "/api/v1", "/api", "/v1"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	parsed.Path = path + openai_decisions.OpenRouterPath
	parsed.RawPath, parsed.Fragment = "", ""
	return parsed.String(), nil
}
