package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func effectiveAuthorizeURL() string {
	return firstNonEmpty(os.Getenv("XAI_OAUTH_AUTHORIZE_URL"), DefaultAuthorizeURL)
}

func effectiveTokenURL() string {
	return firstNonEmpty(os.Getenv("XAI_OAUTH_TOKEN_URL"), DefaultTokenURL)
}

func effectiveClientID(value string) string {
	return firstNonEmpty(value, os.Getenv("XAI_OAUTH_CLIENT_ID"), DefaultClientID)
}

func effectiveScope() string {
	return firstNonEmpty(os.Getenv("XAI_OAUTH_SCOPE"), DefaultScope)
}

func effectiveRedirectURI(value string) string {
	return firstNonEmpty(value, os.Getenv("XAI_OAUTH_REDIRECT_URI"), DefaultRedirectURI)
}

func normalizeBaseURL(raw string) string {
	raw = firstNonEmpty(raw, os.Getenv("XAI_BASE_URL"), DefaultBaseURL)
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if strings.TrimRight(parsed.Path, "/") == "" {
		parsed.Path = "/v1"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func endpointURL(baseURL, suffix string) string {
	base := normalizeBaseURL(baseURL)
	return strings.TrimRight(base, "/") + suffix
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func randomBase64URL(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "="), nil
}

func randomHexString(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return strings.TrimRight(base64.URLEncoding.EncodeToString(sum[:]), "=")
}

type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ClientID     string    `json:"client_id"`
	Scope        string    `json:"scope"`
	RedirectURI  string    `json:"redirect_uri"`
	ProxyURL     string    `json:"proxy_url"`
	CreatedAt    time.Time `json:"created_at"`
}

type OAuthSessionStore struct {
	sessions map[string]*OAuthSession
}

func NewOAuthSessionStore() *OAuthSessionStore {
	return &OAuthSessionStore{sessions: map[string]*OAuthSession{}}
}

func (s *OAuthSessionStore) Set(id string, session *OAuthSession) {
	s.sessions[id] = session
}

func (s *OAuthSessionStore) Get(id string) (*OAuthSession, bool) {
	session := s.sessions[strings.TrimSpace(id)]
	if session == nil {
		return nil, false
	}
	if time.Since(session.CreatedAt) > 30*time.Minute {
		delete(s.sessions, id)
		return nil, false
	}
	return session, true
}

func (s *OAuthSessionStore) Delete(id string) {
	delete(s.sessions, strings.TrimSpace(id))
}

type AuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (a *App) generateAuthURL(redirectURI, proxyURL string) (*AuthURLResult, error) {
	state, err := randomHexString(32)
	if err != nil {
		return nil, err
	}
	nonce, err := randomHexString(16)
	if err != nil {
		return nil, err
	}
	verifier, err := randomBase64URL(32)
	if err != nil {
		return nil, err
	}
	sessionID, err := randomHexString(16)
	if err != nil {
		return nil, err
	}
	redirectURI = effectiveRedirectURI(redirectURI)
	clientID := effectiveClientID("")
	scope := effectiveScope()

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", scope)
	values.Set("state", state)
	values.Set("nonce", nonce)
	values.Set("code_challenge", codeChallenge(verifier))
	values.Set("code_challenge_method", "S256")
	values.Set("plan", "generic")
	values.Set("referrer", "grok-only-gateway")

	authURL := strings.TrimRight(effectiveAuthorizeURL(), "?") + "?" + values.Encode()
	a.sessions.Set(sessionID, &OAuthSession{
		State:        state,
		CodeVerifier: verifier,
		ClientID:     clientID,
		Scope:        scope,
		RedirectURI:  redirectURI,
		ProxyURL:     strings.TrimSpace(proxyURL),
		CreatedAt:    time.Now().UTC(),
	})
	return &AuthURLResult{AuthURL: authURL, SessionID: sessionID, State: state}, nil
}

type AuthorizationInput struct {
	Code          string
	State         string
	RequiresState bool
}

func parseAuthorizationInput(raw string) AuthorizationInput {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AuthorizationInput{}
	}
	if parsed, err := url.Parse(raw); err == nil {
		values := parsed.Query()
		if code := strings.TrimSpace(values.Get("code")); code != "" {
			return AuthorizationInput{Code: code, State: strings.TrimSpace(values.Get("state")), RequiresState: true}
		}
	}
	query := strings.TrimPrefix(raw, "?")
	if strings.Contains(query, "=") {
		if values, err := url.ParseQuery(query); err == nil {
			if code := strings.TrimSpace(values.Get("code")); code != "" {
				return AuthorizationInput{Code: code, State: strings.TrimSpace(values.Get("state")), RequiresState: true}
			}
		}
	}
	return AuthorizationInput{Code: raw}
}

func (a *App) exchangeOAuthCode(ctx context.Context, sessionID, codeInput, state, redirectURI, proxyURL string) (*TokenInfo, error) {
	session, ok := a.sessions.Get(sessionID)
	if !ok {
		return nil, errors.New("oauth session not found or expired")
	}
	defer a.sessions.Delete(sessionID)

	parsed := parseAuthorizationInput(codeInput)
	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		return nil, errors.New("authorization code is required")
	}
	if state == "" {
		state = parsed.State
	}
	if parsed.RequiresState && state == "" {
		return nil, errors.New("oauth state is required")
	}
	if state != "" && state != session.State {
		return nil, errors.New("invalid oauth state")
	}
	if strings.TrimSpace(proxyURL) == "" {
		proxyURL = session.ProxyURL
	}
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = session.RedirectURI
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", effectiveClientID(session.ClientID))
	form.Set("code", code)
	form.Set("redirect_uri", effectiveRedirectURI(redirectURI))
	form.Set("code_verifier", session.CodeVerifier)
	return a.postTokenForm(ctx, form, proxyURL, session.ClientID)
}

func (a *App) refreshOAuthToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*TokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errors.New("refresh_token is required")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", effectiveClientID(clientID))
	form.Set("refresh_token", refreshToken)
	info, err := a.postTokenForm(ctx, form, proxyURL, clientID)
	if err != nil {
		return nil, err
	}
	if info.RefreshToken == "" {
		info.RefreshToken = refreshToken
	}
	return info, nil
}

func (a *App) postTokenForm(ctx context.Context, form url.Values, proxyURL, clientID string) (*TokenInfo, error) {
	client, err := a.httpClient.Client(proxyURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := withTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, effectiveTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "grok-only-gateway-oauth/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token request failed: status %d body %s", resp.StatusCode, sanitizeLogBody(body))
	}
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, errors.New("token response missing access_token")
	}
	return tokenInfoFromResponse(&tokenResp, clientID), nil
}

func tokenInfoFromResponse(resp *TokenResponse, clientID string) *TokenInfo {
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64((6 * time.Hour).Seconds())
	}
	info := &TokenInfo{
		AccessToken:  strings.TrimSpace(resp.AccessToken),
		RefreshToken: strings.TrimSpace(resp.RefreshToken),
		IDToken:      strings.TrimSpace(resp.IDToken),
		TokenType:    firstNonEmpty(resp.TokenType, "Bearer"),
		ExpiresIn:    expiresIn,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(expiresIn) * time.Second).Unix(),
		ClientID:     effectiveClientID(clientID),
		Scope:        strings.TrimSpace(resp.Scope),
	}
	info.Email = parseJWTEmail(resp.IDToken)
	return info
}

func parseJWTEmail(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.Email)
}

func (a *App) accessTokenForAccount(ctx context.Context, account *Account) (string, *Account, error) {
	if account == nil {
		return "", nil, errors.New("account is nil")
	}
	if strings.TrimSpace(account.RefreshToken) != "" {
		needsRefresh := account.ExpiresAt == nil || time.Until(*account.ExpiresAt) <= time.Hour
		if needsRefresh {
			info, err := a.refreshOAuthToken(ctx, account.RefreshToken, account.ProxyURL, account.ClientID)
			if err != nil {
				if strings.TrimSpace(account.AccessToken) == "" || account.ExpiresAt == nil || time.Now().After(*account.ExpiresAt) {
					return "", account, err
				}
			} else {
				updated, err := a.store.UpdateAccountToken(account.ID, info)
				if err == nil {
					account = updated
				}
			}
		}
	}
	token := strings.TrimSpace(account.AccessToken)
	if token == "" {
		return "", account, errors.New("account missing access_token")
	}
	if account.ExpiresAt != nil && time.Now().After(*account.ExpiresAt) && strings.TrimSpace(account.RefreshToken) == "" {
		return "", account, errors.New("access_token expired and refresh_token is missing")
	}
	return token, account, nil
}

func quotaFromHeaders(headers http.Header, statusCode int, source string) map[string]any {
	allow := []string{
		"x-ratelimit-limit-requests",
		"x-ratelimit-remaining-requests",
		"x-ratelimit-reset-requests",
		"x-ratelimit-limit-tokens",
		"x-ratelimit-remaining-tokens",
		"x-ratelimit-reset-tokens",
		"retry-after",
		"x-subscription-tier",
		"xai-subscription-tier",
		"x-entitlement-status",
		"xai-entitlement-status",
		"xai-monthly-credits",
		"xai-monthly-credit-balance",
		"xai-monthly-credits-remaining",
		"xai-monthly-credits-limit",
		"xai-payg-credits",
		"xai-payg-credits-remaining",
	}
	out := map[string]any{
		"status_code":        statusCode,
		"observation_source": source,
		"updated_at":         time.Now().UTC().Format(time.RFC3339),
		"headers":            map[string]string{},
	}
	headerMap := out["headers"].(map[string]string)
	for _, key := range allow {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			headerMap[key] = value
		}
	}
	if len(headerMap) == 0 {
		return nil
	}
	return out
}

func retryAfter(headers http.Header) time.Duration {
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		return time.Until(t)
	}
	return 0
}

func sanitizeLogBody(body []byte) string {
	text := string(body)
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		text = redactJSONValue(text, key)
	}
	if len(text) > 2048 {
		text = text[:2048]
	}
	return text
}

func redactJSONValue(text, key string) string {
	needle := `"` + key + `":"`
	idx := strings.Index(text, needle)
	for idx >= 0 {
		start := idx + len(needle)
		end := strings.Index(text[start:], `"`)
		if end < 0 {
			return text
		}
		text = text[:start] + "[REDACTED]" + text[start+end:]
		idx = strings.Index(text[start+10:], needle)
		if idx >= 0 {
			idx += start + 10
		}
	}
	return text
}

type multipartUpload struct {
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
}

func uploadToDataURL(upload multipartUpload) string {
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(upload.Data)
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(upload.Data)
}

func readMultipartPart(part *multipart.Part, max int64) ([]byte, error) {
	defer part.Close()
	var buf bytes.Buffer
	_, err := io.Copy(&buf, io.LimitReader(part, max))
	return buf.Bytes(), err
}
