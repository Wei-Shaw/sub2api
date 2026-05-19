package kiro

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// kiroOAuthScopes is the scope set Kiro requests on the upstream OIDC
// client. Matches the values in the Kiro-Go reference.
var kiroOAuthScopes = []string{
	"codewhisperer:completions",
	"codewhisperer:analysis",
	"codewhisperer:conversations",
	"codewhisperer:transformations",
	"codewhisperer:taskassist",
}

// idcRedirectURI is the loopback URI Kiro registers for the IdC PKCE flow.
// The admin pastes the full redirected URL back, so we never actually run
// a callback server.
const idcRedirectURI = "http://127.0.0.1/oauth/callback"

// idcSessionTTL bounds how long the admin has to complete the IdC login.
const idcSessionTTL = 10 * time.Minute

// IdCLoginStarted is the result of StartIdCLogin: the session id (passed
// back to CompleteIdCLogin) and the URL the admin opens in their browser.
type IdCLoginStarted struct {
	SessionID     string
	AuthorizeURL  string
	ExpiresInSecs int
}

// StartIdCLogin registers an OIDC client at the user-supplied startUrl /
// region, generates PKCE material, and returns the authorize URL.
//
// The caller stores the session in the package-level SessionStore via the
// returned IdCLoginStarted.SessionID (set by the caller).
func StartIdCLogin(store *SessionStore, startURL, region, proxyURL string) (*IdCLoginStarted, *IdCSession, error) {
	if strings.TrimSpace(startURL) == "" {
		return nil, nil, fmt.Errorf("kiro idc: start_url required")
	}
	if region == "" {
		region = "us-east-1"
	}
	client := HTTPClient(proxyURL)
	oidcBase := fmt.Sprintf("https://oidc.%s.amazonaws.com", region)

	clientID, clientSecret, err := registerOIDCClient(client, oidcBase, startURL, []string{idcRedirectURI}, []string{"authorization_code", "refresh_token"})
	if err != nil {
		return nil, nil, fmt.Errorf("kiro idc: register client: %w", err)
	}

	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, nil, fmt.Errorf("kiro idc: pkce: %w", err)
	}
	challenge := generateCodeChallenge(verifier)
	state := uuid.New().String()
	sessionID := uuid.New().String()

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", idcRedirectURI)
	params.Set("scopes", strings.Join(kiroOAuthScopes, ","))
	params.Set("state", state)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	authorizeURL := fmt.Sprintf("%s/authorize?%s", oidcBase, params.Encode())

	sess := &IdCSession{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CodeVerifier: verifier,
		State:        state,
		Region:       region,
		StartURL:     startURL,
		RedirectURI:  idcRedirectURI,
		ProxyURL:     proxyURL,
		ExpiresAt:    time.Now().Add(idcSessionTTL),
	}
	if store != nil {
		store.SetIdC(sessionID, sess)
	}

	return &IdCLoginStarted{
		SessionID:     sessionID,
		AuthorizeURL:  authorizeURL,
		ExpiresInSecs: int(idcSessionTTL.Seconds()),
	}, sess, nil
}

// CompleteIdCLogin parses the callback URL, validates state, and exchanges
// the code for tokens. On success it returns a populated TokenInfo with
// AuthMethod=IdC and the persistent client_id/secret/region/start_url
// (needed for subsequent refreshes).
func CompleteIdCLogin(store *SessionStore, sessionID, callbackURL string) (*TokenInfo, error) {
	if store == nil {
		return nil, fmt.Errorf("kiro idc: session store required")
	}
	sess, ok := store.GetIdC(sessionID)
	if !ok {
		return nil, fmt.Errorf("kiro idc: session not found or expired")
	}
	if time.Now().After(sess.ExpiresAt) {
		store.DeleteIdC(sessionID)
		return nil, fmt.Errorf("kiro idc: session expired")
	}

	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return nil, fmt.Errorf("kiro idc: invalid callback url: %w", err)
	}
	q := parsed.Query()
	if e := q.Get("error"); e != "" {
		return nil, fmt.Errorf("kiro idc: authorization error: %s", e)
	}
	state := q.Get("state")
	code := q.Get("code")
	if state == "" || code == "" {
		return nil, fmt.Errorf("kiro idc: callback missing state/code")
	}
	if state != sess.State {
		return nil, fmt.Errorf("kiro idc: state mismatch")
	}

	oidcBase := fmt.Sprintf("https://oidc.%s.amazonaws.com", sess.Region)
	info, err := exchangeAuthorizationCode(HTTPClient(sess.ProxyURL), oidcBase,
		sess.ClientID, sess.ClientSecret, code, sess.CodeVerifier, sess.RedirectURI)
	if err != nil {
		return nil, err
	}

	store.DeleteIdC(sessionID)

	info.AuthMethod = AuthMethodIdC
	info.ClientID = sess.ClientID
	info.ClientSecret = sess.ClientSecret
	info.Region = sess.Region
	info.StartURL = sess.StartURL
	return info, nil
}

// registerOIDCClient registers a fresh OIDC client at oidc.{region}.amazonaws.com.
// Shared by IdC and Builder ID flows (they differ only in grant types and
// redirect URIs).
func registerOIDCClient(client *http.Client, oidcBase, issuerURL string, redirectURIs, grantTypes []string) (string, string, error) {
	body := map[string]any{
		"clientName": "sub2api Kiro",
		"clientType": "public",
		"scopes":     kiroOAuthScopes,
		"grantTypes": grantTypes,
		"issuerUrl":  issuerURL,
	}
	if len(redirectURIs) > 0 {
		body["redirectUris"] = redirectURIs
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, oidcBase+"/client/register", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", err
	}
	if parsed.ClientID == "" || parsed.ClientSecret == "" {
		return "", "", fmt.Errorf("empty client_id or client_secret in response")
	}
	return parsed.ClientID, parsed.ClientSecret, nil
}

// exchangeAuthorizationCode posts to /token with grant_type=authorization_code
// and decodes the standard response shape. Used by CompleteIdCLogin.
func exchangeAuthorizationCode(client *http.Client, oidcBase, clientID, clientSecret, code, verifier, redirectURI string) (*TokenInfo, error) {
	payload, _ := json.Marshal(map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"grantType":    "authorization_code",
		"redirectUri":  redirectURI,
		"code":         code,
		"codeVerifier": verifier,
	})
	req, err := http.NewRequest(http.MethodPost, oidcBase+"/token", bytes.NewReader(payload))
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
		return nil, fmt.Errorf("kiro idc: token exchange HTTP %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
		ProfileArn   string `json:"profileArn"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("kiro idc: decode token: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("kiro idc: empty access token")
	}
	return &TokenInfo{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(parsed.ExpiresIn),
		ProfileARN:   parsed.ProfileArn,
	}, nil
}

// generateCodeVerifier produces a 32-byte URL-safe random verifier per RFC 7636.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge returns the S256-encoded challenge for a verifier.
func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
