package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// socialRefreshURL is the endpoint used by the Kiro desktop app to swap a
// long-lived Social (GitHub/Google/etc.) refresh token for a fresh
// access+refresh pair.
const socialRefreshURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

// RefreshSocial swaps a Social refresh token for a fresh access+refresh pair.
// Both tokens may rotate — the caller must persist both fields.
//
// AuthMethod is set to AuthMethodSocial on the returned TokenInfo so a
// generic caller can dispatch a follow-up refresh without re-detecting the
// auth method.
func RefreshSocial(refreshToken, proxyURL string) (*TokenInfo, error) {
	return refreshSocialAt(socialRefreshURL, refreshToken, HTTPClient(proxyURL))
}

// refreshSocialAt is the testable form: takes an explicit URL and HTTP
// client so the unit test can point at httptest.Server.
func refreshSocialAt(url, refreshToken string, client *http.Client) (*TokenInfo, error) {
	body, _ := json.Marshal(map[string]string{"refreshToken": refreshToken})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro social refresh: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
		ProfileArn   string `json:"profileArn"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("kiro social refresh: decode: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("kiro social refresh: empty access token")
	}
	return &TokenInfo{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(parsed.ExpiresIn),
		ProfileARN:   parsed.ProfileArn,
		AuthMethod:   AuthMethodSocial,
	}, nil
}
