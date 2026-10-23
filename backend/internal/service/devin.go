package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Devin uses a CLI session token, not a public Devin REST API key. The upstream
// login contract has no refresh grant; see docs/DEVIN.md and THIRD_PARTY_NOTICES.md.
func (a *Account) IsDevin() bool { return a != nil && a.Platform == PlatformDevin }

func DefaultDevinModelIDs() []string {
	models := devin.DefaultModels()
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

// NormalizeDevinCredentials operates on a complete, merged credentials map.
// JWT expiry is a scheduling hint only: upstream remains the authority for token
// validity. Never use an unverified JWT claim as an authorization decision.
func NormalizeDevinCredentials(platform, accountType string, credentials map[string]any) error {
	if platform != PlatformDevin {
		return nil
	}
	invalid := func(message string) error { return infraerrors.BadRequest("INVALID_DEVIN_CREDENTIALS", message) }
	if accountType != AccountTypeOAuth {
		return invalid("Devin subscriptions require oauth accounts with a CLI session token")
	}
	token, _ := credentials["access_token"].(string)
	token = strings.TrimSpace(token)
	if strings.TrimPrefix(token, "devin-session-token$") == "" || len(token) > 64*1024 || strings.ContainsAny(token, " \r\n\t") {
		return invalid("A valid Devin CLI session token is required (not a REST API key)")
	}
	for _, key := range []string{"refresh_token", "api_key", "base_url", "api_base_urls", "api_protocol"} {
		if value, exists := credentials[key]; exists && value != nil && value != "" {
			return invalid("Devin does not support " + key + "; use a CLI session token and sign in again when it expires")
		}
		delete(credentials, key)
	}
	credentials["access_token"] = token
	// Always recalculate from the current token, including on replacement. Keeping
	// the previous token's expiry would immediately expire a freshly imported one.
	delete(credentials, "expires_at")
	if expiry := devinTokenExpiry(token); !expiry.IsZero() {
		credentials["expires_at"] = expiry.UTC().Format(time.RFC3339)
	}
	return nil
}

func devinTokenExpiry(token string) time.Time {
	parts := strings.Split(strings.TrimPrefix(token, "devin-session-token$"), ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp <= 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func devinAccountToken(account *Account) (string, error) {
	if !account.IsDevin() || account.Type != AccountTypeOAuth {
		return "", &devin.Error{StatusCode: http.StatusBadRequest, Code: "invalid_request_error", Message: "Devin requires an OAuth subscription account"}
	}
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return "", &devin.Error{StatusCode: http.StatusUnauthorized, Code: "authentication_error", Message: "Devin CLI session token is missing; sign in again and edit the account"}
	}
	if expiry := devinTokenExpiry(token); !expiry.IsZero() && !time.Now().Before(expiry) {
		return "", &devin.Error{StatusCode: http.StatusUnauthorized, Code: "authentication_error", Message: "Devin CLI session token has expired; sign in again and replace it"}
	}
	return token, nil
}

// Auth may select a regional server. Credentials may only follow it to a trusted
// provider domain, never to a private address or an arbitrary URL in a response.
// The operator's existing outbound allowlist is applied in addition to this rule.
func validateDevinBaseURL(raw string, validate func(string) (string, error)) (string, error) {
	value, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     []string{"codeium.com", "*.codeium.com", "windsurf.com", "*.windsurf.com", "devin.ai", "*.devin.ai"},
		RequireAllowlist: true,
	})
	if err != nil {
		return "", errors.New("Devin returned an untrusted API server URL")
	}
	u, err := url.Parse(value)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Port() != "" && u.Port() != "443") {
		return "", errors.New("Devin API server URL must be an HTTPS provider origin")
	}
	if validate != nil {
		if _, err := validate(value); err != nil {
			return "", errors.New("Devin API server is blocked by the outbound URL policy")
		}
	}
	return strings.TrimRight(value, "/"), nil
}

func newDevinClient(account *Account, upstream HTTPUpstream, validate func(string) (string, error)) *devin.Client {
	validator := func(raw string) (string, error) { return validateDevinBaseURL(raw, validate) }
	return &devin.Client{
		ValidateBaseURL: validator,
		Do: func(req *http.Request) (*http.Response, error) {
			if upstream == nil {
				return nil, errors.New("Devin upstream transport is not configured")
			}
			origin := req.URL.Scheme + "://" + req.URL.Host
			if _, err := validator(origin); err != nil {
				return nil, err
			}
			proxyURL := ""
			if account.ProxyID != nil && account.Proxy != nil {
				proxyURL = account.Proxy.URL()
			}
			// The session token is inside the protobuf body. Go can replay that
			// body across 307/308 redirects even when Authorization is stripped.
			// Auth's explicit customApiServerUrl is the only supported reroute.
			ctx := WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamPublicHostsOnly(req.Context()))
			req = req.WithContext(ctx)
			return upstream.Do(req, proxyURL, account.ID, account.Concurrency)
		},
	}
}

type devinModelCacheEntry struct {
	credential [32]byte
	expires    time.Time
	models     []devin.Model
}

func (s *OpenAIGatewayService) devinModels(ctx context.Context, account *Account, token string) ([]devin.Model, error) {
	fingerprint := sha256.Sum256([]byte(token))
	if raw, ok := s.devinModelCache.Load(account.ID); ok {
		entry := raw.(devinModelCacheEntry)
		if entry.credential == fingerprint && time.Now().Before(entry.expires) {
			return entry.models, nil
		}
	}
	models, err := newDevinClient(account, s.httpUpstream, s.validateUpstreamBaseURL).Models(ctx, token)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		models = devin.DefaultModels()
	}
	s.devinModelCache.Store(account.ID, devinModelCacheEntry{credential: fingerprint, expires: time.Now().Add(5 * time.Minute), models: models})
	return models, nil
}

func devinCascadeID(c *gin.Context, account *Account, body []byte) string {
	seed := ""
	if c != nil && c.Request != nil {
		for _, name := range []string{"session_id", "session-id", "conversation_id", "x-session-id"} {
			if seed = strings.TrimSpace(c.GetHeader(name)); seed != "" {
				break
			}
		}
	}
	if seed == "" {
		seed = gjson.GetBytes(body, "prompt_cache_key").String()
	}
	if seed == "" {
		seed = deriveOpenAIContentSessionSeed(body)
	}
	if seed == "" {
		seed = uuid.NewString()
	}
	return isolateDevinCascadeID(c, account, seed)
}

func isolateDevinCascadeID(c *gin.Context, account *Account, seed string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("devin:%d:%d:%s", account.ID, getAPIKeyIDFromContext(c), seed))).String()
}

func devinErrorResponse(err error) *http.Response {
	status, code, message := http.StatusBadGateway, "upstream_error", "Devin upstream request failed"
	var upstream *devin.Error
	if errors.As(err, &upstream) {
		status, code, message = upstream.StatusCode, upstream.Code, upstream.Message
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"type": code, "code": code, "message": message}})
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}
}

// Adapt only at the shared CC transport boundary: all three public protocols
// retain the existing model mapping, policy, SSE conversion, usage and failover.
func (s *OpenAIGatewayService) sendDevinRequest(ctx context.Context, c *gin.Context, account *Account, body []byte) (*http.Response, error) {
	SetActualOpenAIUpstreamEndpoint(c, devin.ChatPath)
	token, err := devinAccountToken(account)
	if err != nil {
		return devinErrorResponse(err), nil
	}
	models, err := s.devinModels(ctx, account, token)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Discovery failures must not hide revoked credentials or rate limits.
		return devinErrorResponse(err), nil
	}
	requested := gjson.GetBytes(body, "model").String()
	var selected *devin.Model
	for i := range models {
		if models[i].ID == requested {
			selected = &models[i]
			break
		}
	}
	if selected == nil {
		return nil, &devin.Error{StatusCode: http.StatusBadRequest, Code: "model_not_supported", Message: "Model is not available in this Devin account's catalog; synchronize models first"}
	}
	cascadeID := c.GetString(devinCascadeContextKey)
	if cascadeID == "" {
		cascadeID = devinCascadeID(c, account, body)
	}
	// This client field has already been consumed for the isolated cascade ID;
	// it is not a native Devin generation parameter.
	if gjson.GetBytes(body, "prompt_cache_key").Exists() {
		body, err = sjson.DeleteBytes(body, "prompt_cache_key")
		if err != nil {
			return nil, err
		}
	}
	response, err := newDevinClient(account, s.httpUpstream, s.validateUpstreamBaseURL).Chat(ctx, token, cascadeID, *selected, body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var requestErr *devin.Error
		if errors.As(err, &requestErr) && (requestErr.StatusCode == http.StatusBadRequest || requestErr.StatusCode == http.StatusUnprocessableEntity) {
			return nil, err
		}
		return devinErrorResponse(err), nil
	}
	return response, nil
}

func (s *AccountTestService) fetchDevinUpstreamModels(ctx context.Context, account *Account) ([]string, []byte, error) {
	token, err := devinAccountToken(account)
	if err != nil {
		return nil, nil, newUpstreamModelSyncConfigError("Devin requires a valid CLI session token", err)
	}
	models, err := newDevinClient(account, s.httpUpstream, s.validateUpstreamBaseURL).Models(ctx, token)
	if err != nil {
		return nil, nil, newUpstreamModelSyncUpstreamError("Failed to discover Devin models", err)
	}
	if len(models) == 0 {
		models = devin.DefaultModels()
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	body, err := json.Marshal(map[string]any{"data": models})
	return ids, body, err
}

func (s *AccountTestService) testDevinAccountConnection(c *gin.Context, account *Account, modelID, prompt string) error {
	if modelID == "" {
		modelID = "swe-1-6"
	}
	modelID = account.GetMappedModel(modelID)
	if prompt == "" {
		prompt = "Reply with OK."
	}
	body, _ := json.Marshal(map[string]any{"model": modelID, "stream": true, "max_tokens": 64, "messages": []map[string]string{{"role": "user", "content": prompt}}})
	gateway := s.openaiGatewayService
	if gateway == nil {
		gateway = &OpenAIGatewayService{cfg: s.cfg, httpUpstream: s.httpUpstream}
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})
	resp, err := gateway.sendDevinRequest(c.Request.Context(), c, account, body)
	if err != nil {
		return s.sendErrorAndEnd(c, "Devin connection test failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Devin connection test failed (HTTP %d); verify the CLI token and model access", resp.StatusCode))
	}
	return s.processOpenAIChatCompletionsStream(c, resp.Body)
}
