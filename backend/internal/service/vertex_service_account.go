package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	vertexDefaultLocation         = "us-central1"
	vertexDefaultTokenURL         = "https://oauth2.googleapis.com/token"
	vertexCloudPlatformScope      = "https://www.googleapis.com/auth/cloud-platform"
	vertexServiceAccountCacheSkew = 5 * time.Minute
	vertexLockWaitTime            = 200 * time.Millisecond
	vertexAnthropicVersion        = "vertex-2023-10-16"
)

var (
	vertexLocationPattern                = regexp.MustCompile(`^[a-z0-9-]+$`)
	vertexAnthropicDatedModelIDPattern   = regexp.MustCompile(`^(.+)-([0-9]{8})$`)
	vertexAnthropicAlreadyDatedIDPattern = regexp.MustCompile(`^.+@[0-9]{8}$`)
)

type vertexServiceAccountKey struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`
}

type vertexTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func (a *Account) IsVertexServiceAccount() bool {
	return a != nil && a.Type == AccountTypeServiceAccount
}

func (a *Account) VertexProjectID() string {
	if a == nil {
		return ""
	}
	if v := strings.TrimSpace(a.GetCredential("project_id")); v != "" {
		return v
	}
	key, err := parseVertexServiceAccountKey(a)
	if err == nil {
		return strings.TrimSpace(key.ProjectID)
	}
	return ""
}

func (a *Account) VertexLocation(model string) string {
	if a == nil {
		return vertexDefaultLocation
	}
	if model != "" && a.Credentials != nil {
		if raw, ok := a.Credentials["vertex_model_locations"].(map[string]any); ok {
			if loc, ok := raw[model].(string); ok && strings.TrimSpace(loc) != "" {
				return strings.TrimSpace(loc)
			}
		}
	}
	if v := strings.TrimSpace(a.GetCredential("location")); v != "" {
		return v
	}
	if v := strings.TrimSpace(a.GetCredential("vertex_location")); v != "" {
		return v
	}
	return vertexDefaultLocation
}

func parseVertexServiceAccountKey(account *Account) (*vertexServiceAccountKey, error) {
	if account == nil || account.Credentials == nil {
		return nil, errors.New("service account credentials not configured")
	}

	if raw := strings.TrimSpace(account.GetCredential("service_account_json")); raw != "" {
		return parseVertexServiceAccountJSON([]byte(raw))
	}
	if raw := strings.TrimSpace(account.GetCredential("service_account")); raw != "" {
		return parseVertexServiceAccountJSON([]byte(raw))
	}
	if nested, ok := account.Credentials["service_account_json"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		return parseVertexServiceAccountJSON(b)
	}
	if nested, ok := account.Credentials["service_account"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		return parseVertexServiceAccountJSON(b)
	}
	return nil, errors.New("service_account_json not found in credentials")
}

func parseVertexServiceAccountJSON(raw []byte) (*vertexServiceAccountKey, error) {
	var key vertexServiceAccountKey
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, fmt.Errorf("invalid service account json: %w", err)
	}
	if strings.TrimSpace(key.ClientEmail) == "" {
		return nil, errors.New("service account json missing client_email")
	}
	if strings.TrimSpace(key.PrivateKey) == "" {
		return nil, errors.New("service account json missing private_key")
	}
	if strings.TrimSpace(key.ProjectID) == "" {
		return nil, errors.New("service account json missing project_id")
	}
	// Always use the well-known Google token endpoint to prevent SSRF via crafted token_uri.
	key.TokenURI = vertexDefaultTokenURL
	return &key, nil
}

func vertexServiceAccountCacheKey(account *Account, key *vertexServiceAccountKey) string {
	fingerprint := ""
	if key != nil {
		sum := sha256.Sum256([]byte(key.ClientEmail + "\x00" + key.PrivateKeyID))
		fingerprint = hex.EncodeToString(sum[:8])
	}
	if fingerprint == "" && account != nil {
		fingerprint = fmt.Sprintf("account:%d", account.ID)
	}
	return "vertex:service_account:" + fingerprint
}

// getVertexServiceAccountAccessToken obtains an access token for a Vertex service account,
// using the shared cache and distributed lock to avoid redundant exchanges.
func getVertexServiceAccountAccessToken(ctx context.Context, cache GeminiTokenCache, account *Account) (string, error) {
	key, err := parseVertexServiceAccountKey(account)
	if err != nil {
		return "", err
	}
	cacheKey := vertexServiceAccountCacheKey(account, key)

	if cache != nil {
		if token, err := cache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	locked := false
	if cache != nil {
		var lockErr error
		locked, lockErr = cache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if lockErr == nil && locked {
			defer func() { _ = cache.ReleaseRefreshLock(ctx, cacheKey) }()
		} else if lockErr != nil {
			slog.Warn("vertex_service_account_token_lock_failed", "account_id", account.ID, "error", lockErr)
		} else {
			time.Sleep(vertexLockWaitTime)
			if token, err := cache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
				return token, nil
			}
		}
	}

	accessToken, ttl, err := exchangeVertexServiceAccountToken(ctx, key)
	if err != nil {
		return "", err
	}
	if cache != nil {
		_ = cache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	}
	return accessToken, nil
}

func exchangeVertexServiceAccountToken(ctx context.Context, key *vertexServiceAccountKey) (string, time.Duration, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   key.ClientEmail,
		"scope": vertexCloudPlatformScope,
		"aud":   key.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if strings.TrimSpace(key.PrivateKeyID) != "" {
		token.Header["kid"] = key.PrivateKeyID
	}
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(key.PrivateKey))
	if err != nil {
		return "", 0, fmt.Errorf("parse service account private key: %w", err)
	}
	assertion, err := token.SignedString(privateKey)
	if err != nil {
		return "", 0, fmt.Errorf("sign service account assertion: %w", err)
	}

	values := url.Values{}
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	values.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, key.TokenURI, strings.NewReader(values.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("service account token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed vertexTokenResponse
	_ = json.Unmarshal(body, &parsed)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(parsed.ErrorDesc)
		if msg == "" {
			msg = strings.TrimSpace(parsed.Error)
		}
		if msg == "" {
			msg = string(bytes.TrimSpace(body))
		}
		return "", 0, fmt.Errorf("service account token request returned %d: %s", resp.StatusCode, msg)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", 0, errors.New("service account token response missing access_token")
	}
	ttl := time.Duration(parsed.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	if ttl > vertexServiceAccountCacheSkew {
		ttl -= vertexServiceAccountCacheSkew
	}
	return parsed.AccessToken, ttl, nil
}

func buildVertexGeminiURL(projectID, location, model, action string, stream bool) (string, error) {
	projectID = strings.TrimSpace(projectID)
	location = strings.TrimSpace(location)
	model = strings.TrimSpace(model)
	action = strings.TrimSpace(action)
	if projectID == "" {
		return "", errors.New("vertex project_id is required")
	}
	if location == "" {
		location = vertexDefaultLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}
	if model == "" {
		return "", errors.New("vertex model is required")
	}
	switch action {
	case "generateContent", "streamGenerateContent", "countTokens":
	default:
		return "", fmt.Errorf("unsupported vertex gemini action: %s", action)
	}
	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
	}
	u := fmt.Sprintf(
		"https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:%s",
		host,
		url.PathEscape(projectID),
		url.PathEscape(location),
		url.PathEscape(model),
		action,
	)
	if stream {
		u += "?alt=sse"
	}
	return u, nil
}

func buildVertexAnthropicURL(projectID, location, model string, stream bool) (string, error) {
	projectID = strings.TrimSpace(projectID)
	location = strings.TrimSpace(location)
	model = strings.TrimSpace(model)
	if projectID == "" {
		return "", errors.New("vertex project_id is required")
	}
	if location == "" {
		location = vertexDefaultLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}
	if model == "" {
		return "", errors.New("vertex model is required")
	}
	action := "rawPredict"
	if stream {
		action = "streamRawPredict"
	}
	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
	}
	escapedModel := strings.ReplaceAll(url.PathEscape(model), "%40", "@")
	return fmt.Sprintf(
		"https://%s/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
		host,
		url.PathEscape(projectID),
		url.PathEscape(location),
		escapedModel,
		action,
	), nil
}

func normalizeVertexAnthropicModelID(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || vertexAnthropicAlreadyDatedIDPattern.MatchString(model) {
		return model
	}
	if m := vertexAnthropicDatedModelIDPattern.FindStringSubmatch(model); len(m) == 3 {
		return m[1] + "@" + m[2]
	}
	return model
}

func buildVertexAnthropicRequestBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse anthropic vertex request body: %w", err)
	}
	delete(payload, "model")
	payload["anthropic_version"] = vertexAnthropicVersion
	return json.Marshal(payload)
}
