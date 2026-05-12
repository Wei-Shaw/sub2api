package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- Vertex token constants ---

const (
	vertexDefaultTokenURL    = "https://oauth2.googleapis.com/token"
	vertexCloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
	vertexTokenTimeout       = 15 * time.Second
	vertexMaxTokenBody       = 1 << 20 // 1 MB
)

// --- Service account key parsing ---

// vertexServiceAccountKey holds the relevant fields from a Google Service
// Account JSON key file.
type vertexServiceAccountKey struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
}

// parseVertexServiceAccountKey parses and validates a service account JSON key.
func parseVertexServiceAccountKey(saJSON string) (*vertexServiceAccountKey, error) {
	saJSON = strings.TrimSpace(saJSON)
	if saJSON == "" {
		return nil, fmt.Errorf("service_account_json is empty")
	}
	var key vertexServiceAccountKey
	if err := json.Unmarshal([]byte(saJSON), &key); err != nil {
		return nil, fmt.Errorf("invalid service account json: %w", err)
	}
	if strings.TrimSpace(key.ClientEmail) == "" {
		return nil, fmt.Errorf("service account json missing client_email")
	}
	if strings.TrimSpace(key.PrivateKey) == "" {
		return nil, fmt.Errorf("service account json missing private_key")
	}
	if strings.TrimSpace(key.ProjectID) == "" {
		return nil, fmt.Errorf("service account json missing project_id")
	}
	return &key, nil
}

// --- Token exchange ---

// vertexTokenResponse mirrors the Google OAuth2 token endpoint response.
type vertexTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// exchangeVertexServiceAccountToken creates a JWT assertion from the service
// account key and exchanges it for a Google OAuth2 access token.
//
// This is a simplified version of the core's token exchange without Redis
// caching or distributed locking. Each call makes a fresh token request.
func exchangeVertexServiceAccountToken(ctx context.Context, client *http.Client, saJSON string) (string, error) {
	key, err := parseVertexServiceAccountKey(saJSON)
	if err != nil {
		return "", err
	}
	return exchangeWithKey(ctx, client, key)
}

// exchangeWithKey performs the JWT -> access token exchange.
func exchangeWithKey(ctx context.Context, client *http.Client, key *vertexServiceAccountKey) (string, error) {
	assertion, err := buildServiceAccountJWT(key)
	if err != nil {
		return "", err
	}

	values := url.Values{}
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	values.Set("assertion", assertion)

	reqCtx, cancel := context.WithTimeout(ctx, vertexTokenTimeout)
	defer cancel()

	// Always use the well-known Google token endpoint to prevent SSRF.
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, vertexDefaultTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, vertexMaxTokenBody))

	var parsed vertexTokenResponse
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(parsed.ErrorDesc)
		if msg == "" {
			msg = strings.TrimSpace(parsed.Error)
		}
		if msg == "" {
			msg = truncateBody(body)
		}
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, msg)
	}

	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("token response missing access_token")
	}
	return parsed.AccessToken, nil
}

// buildServiceAccountJWT creates a signed JWT assertion for the Google
// OAuth2 token endpoint using RS256.
func buildServiceAccountJWT(key *vertexServiceAccountKey) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   key.ClientEmail,
		"scope": vertexCloudPlatformScope,
		"aud":   vertexDefaultTokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if strings.TrimSpace(key.PrivateKeyID) != "" {
		token.Header["kid"] = key.PrivateKeyID
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(key.PrivateKey))
	if err != nil {
		return "", fmt.Errorf("parse service account private key: %w", err)
	}

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign service account assertion: %w", err)
	}
	return signed, nil
}
