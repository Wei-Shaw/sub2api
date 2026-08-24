//go:build unit

package kiro

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseImportedExternalIdpTokenPreservesMetadata(t *testing.T) {
	token, err := ParseImportedToken(`{
		"accessToken": "access-token",
		"refreshToken": "refresh-token",
		"expiresAt": "2026-06-29T09:33:49Z",
		"authMethod": "external_idp",
		"provider": "ExternalIdp",
		"tokenEndpoint": "https://login.example.com/oauth2/v2.0/token",
		"issuerUrl": "https://login.example.com/tenant/v2.0",
		"clientId": "client-id",
		"scopes": "openid profile offline_access"
	}`, "")
	if err != nil {
		t.Fatalf("ParseImportedToken() error = %v", err)
	}

	if token.AuthMethod != "external_idp" {
		t.Fatalf("AuthMethod = %q, want external_idp", token.AuthMethod)
	}
	if token.Provider != ProviderExternalIdp {
		t.Fatalf("Provider = %q, want %q", token.Provider, ProviderExternalIdp)
	}
	if token.TokenEndpoint != "https://login.example.com/oauth2/v2.0/token" {
		t.Fatalf("TokenEndpoint = %q", token.TokenEndpoint)
	}
	if token.IssuerURL != "https://login.example.com/tenant/v2.0" {
		t.Fatalf("IssuerURL = %q", token.IssuerURL)
	}
	if token.ClientID != "client-id" {
		t.Fatalf("ClientID = %q", token.ClientID)
	}
	if token.Scopes != "openid profile offline_access" {
		t.Fatalf("Scopes = %q", token.Scopes)
	}
}

func TestParseImportedExternalIdpTokenRejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name      string
		tokenJSON string
	}{
		{
			name:      "missing refresh token",
			tokenJSON: `{"accessToken":"access-token","authMethod":"external_idp","provider":"ExternalIdp","tokenEndpoint":"https://login.example.com/token","clientId":"client-id"}`,
		},
		{
			name:      "missing client id",
			tokenJSON: `{"accessToken":"access-token","refreshToken":"refresh-token","authMethod":"external_idp","provider":"ExternalIdp","tokenEndpoint":"https://login.example.com/token"}`,
		},
		{
			name:      "missing token endpoint",
			tokenJSON: `{"accessToken":"access-token","refreshToken":"refresh-token","authMethod":"external_idp","provider":"ExternalIdp","clientId":"client-id"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseImportedToken(tc.tokenJSON, ""); err == nil {
				t.Fatalf("ParseImportedToken() expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestDiscoverExternalIdpRejectsDisallowedIssuerHost(t *testing.T) {
	_, err := DiscoverExternalIdp(context.Background(), "", "https://login.example.com/tenant/v2.0")

	if err == nil {
		t.Fatalf("DiscoverExternalIdp() expected allow-list error, got nil")
	}
	if !strings.Contains(err.Error(), "external IdP host") || !strings.Contains(err.Error(), "is not allow-listed") {
		t.Fatalf("error = %q, want allow-list rejection", err.Error())
	}
}

func TestValidateExternalIdpEndpointRejectsUnsafeURLs(t *testing.T) {
	cases := []string{
		"http://login.microsoftonline.com/tenant/v2.0",
		"https://127.0.0.1/tenant/v2.0",
		"https://login.example.com/tenant/v2.0",
	}

	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			if err := validateExternalIdpEndpoint(rawURL); err == nil {
				t.Fatalf("validateExternalIdpEndpoint(%q) expected error, got nil", rawURL)
			}
		})
	}
}

func TestValidateExternalIdpEndpointAcceptsMicrosoftHosts(t *testing.T) {
	cases := []string{
		"https://login.microsoftonline.com/tenant/v2.0",
		"https://login.microsoftonline.us/tenant/v2.0",
		"https://login.chinacloudapi.cn.microsoftonline.cn/tenant/v2.0",
	}

	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			if err := validateExternalIdpEndpoint(rawURL); err != nil {
				t.Fatalf("validateExternalIdpEndpoint(%q) error = %v", rawURL, err)
			}
		})
	}
}

func TestExchangeExternalIdpAuthCodeUsesFormPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q, want form", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "authorization_code" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.Form.Get("code"); got != "auth-code" {
			t.Fatalf("code = %q", got)
		}
		if got := r.Form.Get("code_verifier"); got != "verifier" {
			t.Fatalf("code_verifier = %q", got)
		}
		if got := r.Form.Get("redirect_uri"); got != "http://localhost:49153/oauth/callback" {
			t.Fatalf("redirect_uri = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","expires_in":3600}`))
	}))
	defer server.Close()

	token, err := ExchangeExternalIdpAuthCode(context.Background(), "", server.URL, "client-id", "auth-code", "verifier", "http://localhost:49153/oauth/callback", "openid profile", "https://issuer.example.com")
	if err != nil {
		t.Fatalf("ExchangeExternalIdpAuthCode() error = %v", err)
	}
	if token.AccessToken != "access-token" {
		t.Fatalf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "refresh-token" {
		t.Fatalf("RefreshToken = %q", token.RefreshToken)
	}
	if token.AuthMethod != "external_idp" {
		t.Fatalf("AuthMethod = %q", token.AuthMethod)
	}
}

func TestRefreshExternalIdpTokenUsesFormPostAndPreservesRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q, want form", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-token" {
			t.Fatalf("refresh_token = %q", got)
		}
		if got := r.Form.Get("client_id"); got != "client-id" {
			t.Fatalf("client_id = %q", got)
		}
		if got := r.Form.Get("scope"); got != "openid profile offline_access" {
			t.Fatalf("scope = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access-token","expires_in":7200}`))
	}))
	defer server.Close()

	token, err := RefreshExternalIdpToken(context.Background(), "", "client-id", "refresh-token", server.URL, "https://issuer.example.com", "openid profile offline_access")
	if err != nil {
		t.Fatalf("RefreshExternalIdpToken() error = %v", err)
	}
	if token.AccessToken != "new-access-token" {
		t.Fatalf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "refresh-token" {
		t.Fatalf("RefreshToken = %q, want original refresh token", token.RefreshToken)
	}
	if token.AuthMethod != "external_idp" {
		t.Fatalf("AuthMethod = %q", token.AuthMethod)
	}
	if token.Provider != ProviderExternalIdp {
		t.Fatalf("Provider = %q", token.Provider)
	}
	if token.TokenEndpoint != server.URL {
		t.Fatalf("TokenEndpoint = %q", token.TokenEndpoint)
	}
	if token.IssuerURL != "https://issuer.example.com" {
		t.Fatalf("IssuerURL = %q", token.IssuerURL)
	}
	if token.Scopes != "openid profile offline_access" {
		t.Fatalf("Scopes = %q", token.Scopes)
	}
}

func TestRefreshExternalIdpTokenInvalidGrantReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"revoked"}`))
	}))
	defer server.Close()

	_, err := RefreshExternalIdpToken(context.Background(), "", "client-id", "refresh-token", server.URL, "", "")
	if err == nil {
		t.Fatalf("RefreshExternalIdpToken() expected error, got nil")
	}
	var invalid *RefreshTokenInvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %T, want RefreshTokenInvalidError", err)
	}
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d", invalid.StatusCode)
	}
}
