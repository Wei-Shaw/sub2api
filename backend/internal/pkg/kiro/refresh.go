package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// socialRefresher is a package-level seam so tests can inject a fake
// without standing up an httptest.Server for every dispatch test.
var socialRefresher = RefreshSocial

// oidcRefresher is the seam for IdC / Builder ID refreshes. Both auth
// methods hit oidc.{region}.amazonaws.com/token with the same payload
// shape, so they share an implementation.
var oidcRefresher = RefreshOIDC

func withSocialRefresher(fn func(rt, proxy string) (*TokenInfo, error), body func()) {
	prev := socialRefresher
	socialRefresher = fn
	defer func() { socialRefresher = prev }()
	body()
}

func withOIDCRefresher(fn func(a *RefreshableAccount) (*TokenInfo, error), body func()) {
	prev := oidcRefresher
	oidcRefresher = fn
	defer func() { oidcRefresher = prev }()
	body()
}

// RefreshToken refreshes the access token for an account according to its
// auth_method. Social hits Kiro's desktop refresh endpoint; IdC and
// Builder ID share the AWS OIDC refresh endpoint.
func RefreshToken(a *RefreshableAccount) (*TokenInfo, error) {
	if a == nil {
		return nil, fmt.Errorf("kiro refresh: nil account")
	}
	switch a.AuthMethod {
	case AuthMethodSocial:
		return socialRefresher(a.RefreshToken, a.ProxyURL)
	case AuthMethodIdC, AuthMethodBuilderID:
		return oidcRefresher(a)
	default:
		return nil, fmt.Errorf("kiro refresh: unknown auth method %q", a.AuthMethod)
	}
}

// RefreshOIDC refreshes an IdC or Builder ID token against
// oidc.{region}.amazonaws.com/token. Both auth_methods use the same
// endpoint and payload; only the persisted credentials shape differs
// (which is the service layer's concern).
func RefreshOIDC(a *RefreshableAccount) (*TokenInfo, error) {
	if a.ClientID == "" || a.ClientSecret == "" {
		return nil, fmt.Errorf("kiro refresh: client_id/client_secret required for %q", a.AuthMethod)
	}
	region := a.Region
	if region == "" {
		region = "us-east-1"
	}
	url := fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)
	return refreshOIDCAt(url, a.ClientID, a.ClientSecret, a.RefreshToken, a.AuthMethod, HTTPClient(a.ProxyURL))
}

func refreshOIDCAt(url, clientID, clientSecret, refreshToken string, method AuthMethod, client *http.Client) (*TokenInfo, error) {
	payload, _ := json.Marshal(map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro oidc refresh: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
		ProfileArn   string `json:"profileArn"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("kiro oidc refresh: decode: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("kiro oidc refresh: empty access token")
	}
	return &TokenInfo{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(parsed.ExpiresIn),
		ProfileARN:   parsed.ProfileArn,
		AuthMethod:   method,
	}, nil
}
