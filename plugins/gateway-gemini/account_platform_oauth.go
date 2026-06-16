package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Gemini OAuth constants -- mirrored from backend/internal/pkg/geminicli/.
// The plugin cannot import host internal packages, so these are duplicated.
const (
	geminiAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiTokenURL     = "https://oauth2.googleapis.com/token"
	geminiUserInfoURL  = "https://www.googleapis.com/oauth2/v2/userinfo"

	// Built-in Gemini CLI OAuth client credentials (public).
	geminiCLIClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiCLIClientSecret = "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"

	// Redirect URIs per OAuth type.
	geminiCLIRedirectURI      = "https://codeassist.google.com/authcode"
	geminiAIStudioRedirectURI = "http://localhost:1455/auth/callback"

	// Default scopes for code_assist / google_one (cloud-platform + userinfo).
	defaultCodeAssistScopes = "https://www.googleapis.com/auth/cloud-platform " +
		"https://www.googleapis.com/auth/userinfo.email " +
		"https://www.googleapis.com/auth/userinfo.profile"

	// Default scopes for ai_studio (generative-language + cloud-platform).
	defaultAIStudioScopes = "https://www.googleapis.com/auth/cloud-platform " +
		"https://www.googleapis.com/auth/generative-language.retriever"

	// geminiSessionTTL is the maximum lifetime for a pending OAuth session.
	geminiSessionTTL = 30 * time.Minute
)

// ---------- Session store ----------

// geminiOAuthSession stores the state needed between GenerateAuthURL and ExchangeOAuthCode.
type geminiOAuthSession struct {
	state        string
	codeVerifier string
	redirectURI  string
	oauthType    string // "code_assist" / "google_one" / "ai_studio"
	projectID    string
	tierID       string
	clientID     string
	clientSecret string
	createdAt    time.Time
}

type geminiSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*geminiOAuthSession
}

var geminiSessions = &geminiSessionStore{
	sessions: make(map[string]*geminiOAuthSession),
}

func init() {
	go geminiSessions.cleanupLoop()
}

func (s *geminiSessionStore) set(id string, sess *geminiOAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = sess
}

func (s *geminiSessionStore) get(id string) (*geminiOAuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok || time.Since(sess.createdAt) > geminiSessionTTL {
		return nil, false
	}
	return sess, true
}

func (s *geminiSessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *geminiSessionStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			if time.Since(sess.createdAt) > geminiSessionTTL {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// ---------- GenerateAuthURL ----------

// GenerateAuthURL generates a Google OAuth authorization URL for Gemini.
// Supports oauth_type: "code_assist" (default), "google_one", "ai_studio".
// - code_assist / google_one use the built-in Gemini CLI OAuth client.
// - ai_studio requires a custom OAuth client (passed via params).
func (s *accountPlatformServer) GenerateAuthURL(
	_ context.Context,
	req *pb.GenerateAuthURLRequest,
) (*pb.GenerateAuthURLResponse, error) {
	oauthType := strings.TrimSpace(req.GetOauthType())
	if oauthType == "" {
		oauthType = "code_assist"
	}

	params := req.GetParams()
	projectID := strings.TrimSpace(params["project_id"])
	tierID := strings.TrimSpace(params["tier_id"])

	clientID, clientSecret, scopes, redirectURI := resolveGeminiOAuthConfig(oauthType, params)

	state, err := geminiGenerateRandomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	codeVerifier, err := geminiGenerateRandomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}
	codeChallenge := geminiComputeCodeChallenge(codeVerifier)

	sessionID, err := geminiGenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	// If a redirect_uri was passed by the host, prefer it for custom clients.
	if hostRedirect := strings.TrimSpace(req.GetRedirectUri()); hostRedirect != "" {
		if clientID != geminiCLIClientID {
			redirectURI = hostRedirect
		}
	}

	geminiSessions.set(sessionID, &geminiOAuthSession{
		state:        state,
		codeVerifier: codeVerifier,
		redirectURI:  redirectURI,
		oauthType:    oauthType,
		projectID:    projectID,
		tierID:       tierID,
		clientID:     clientID,
		clientSecret: clientSecret,
		createdAt:    time.Now(),
	})

	authURL := buildGeminiAuthURL(clientID, scopes, redirectURI, state, codeChallenge, projectID)

	return &pb.GenerateAuthURLResponse{
		AuthUrl:   authURL,
		SessionId: sessionID,
	}, nil
}

// ---------- ExchangeOAuthCode ----------

// ExchangeOAuthCode exchanges a Google OAuth authorization code for Gemini credentials.
func (s *accountPlatformServer) ExchangeOAuthCode(
	ctx context.Context,
	req *pb.ExchangeOAuthCodeRequest,
) (*pb.ExchangeOAuthCodeResponse, error) {
	sess, ok := geminiSessions.get(req.GetSessionId())
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
	}
	defer geminiSessions.delete(req.GetSessionId())

	tokenResp, err := exchangeGeminiCode(
		ctx, s.httpClient,
		req.GetCode(), sess.codeVerifier,
		sess.clientID, sess.clientSecret, sess.redirectURI,
	)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	expiresAt := computeTokenExpiresAt(tokenResp.ExpiresIn)
	creds := buildGeminiBaseCreds(tokenResp, expiresAt, sess)
	projectID, tierID := resolveGeminiProjectAndTier(ctx, s.httpClient, sess, tokenResp.AccessToken)
	applyGeminiProjectAndTier(creds, projectID, tierID)

	accountName := fetchGeminiEmail(ctx, s.httpClient, tokenResp.AccessToken)
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ExchangeOAuthCodeResponse{
		CredentialsJson: credsJSON,
		AccountName:     accountName,
		TierId:          tierID,
	}, nil
}

// computeTokenExpiresAt calculates the token expiration timestamp with a
// safety window subtracted from the raw TTL.
func computeTokenExpiresAt(expiresIn int64) int64 {
	const safetyWindow = 300
	const minTTL = 30
	expiresAt := time.Now().Unix() + expiresIn - safetyWindow
	minExpiresAt := time.Now().Unix() + minTTL
	if expiresAt < minExpiresAt {
		expiresAt = minExpiresAt
	}
	return expiresAt
}

// buildGeminiBaseCreds assembles the base credentials map from a token response.
func buildGeminiBaseCreds(tokenResp *geminiTokenResponse, expiresAt int64, sess *geminiOAuthSession) map[string]any {
	creds := map[string]any{
		"access_token": tokenResp.AccessToken,
		"expires_at":   time.Unix(expiresAt, 0).Format(time.RFC3339),
		"oauth_type":   sess.oauthType,
	}
	if tokenResp.RefreshToken != "" {
		creds["refresh_token"] = tokenResp.RefreshToken
	}
	if tokenResp.TokenType != "" {
		creds["token_type"] = tokenResp.TokenType
	}
	if tokenResp.Scope != "" {
		creds["scope"] = tokenResp.Scope
	}
	if sess.clientID != geminiCLIClientID {
		creds["client_id"] = sess.clientID
		creds["client_secret"] = sess.clientSecret
	}
	return creds
}

// resolveGeminiProjectAndTier determines the project ID and tier based on
// the OAuth type. code_assist/google_one fetch from the API; ai_studio
// defaults to "aistudio_free".
func resolveGeminiProjectAndTier(
	ctx context.Context, client *http.Client,
	sess *geminiOAuthSession, accessToken string,
) (projectID, tierID string) {
	projectID = sess.projectID
	tierID = sess.tierID
	switch sess.oauthType {
	case "code_assist", "google_one":
		fetchedPID, fetchedTier := loadGeminiProjectInfo(ctx, client, accessToken)
		if fetchedPID != "" {
			projectID = fetchedPID
		}
		if fetchedTier != "" {
			tierID = fetchedTier
		}
	case "ai_studio":
		if tierID == "" {
			tierID = "aistudio_free"
		}
	}
	return projectID, tierID
}

// applyGeminiProjectAndTier sets project_id and tier_id in the credentials
// map when they are non-empty.
func applyGeminiProjectAndTier(creds map[string]any, projectID, tierID string) {
	if projectID != "" {
		creds["project_id"] = projectID
	}
	if tierID != "" {
		creds["tier_id"] = tierID
	}
}

// ---------- ValidateRefreshToken ----------

// ValidateRefreshToken validates a Gemini refresh token by exchanging it
// for a new access token and fetching account info.
func (s *accountPlatformServer) ValidateRefreshToken(
	ctx context.Context,
	req *pb.ValidateRefreshTokenRequest,
) (*pb.ValidateRefreshTokenResponse, error) {
	refreshToken := strings.TrimSpace(req.GetRefreshToken())
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}

	// Use the default Gemini CLI client for refresh.
	tokenResp, err := refreshGeminiToken(
		ctx, s.httpClient, refreshToken,
		geminiCLIClientID, geminiCLIClientSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
	}

	const safetyWindow = 300
	const minTTL = 30
	expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - safetyWindow
	minExpiresAt := time.Now().Unix() + minTTL
	if expiresAt < minExpiresAt {
		expiresAt = minExpiresAt
	}

	creds := map[string]any{
		"access_token":  tokenResp.AccessToken,
		"refresh_token": refreshToken,
		"expires_at":    time.Unix(expiresAt, 0).Format(time.RFC3339),
	}
	if strings.TrimSpace(tokenResp.RefreshToken) != "" {
		creds["refresh_token"] = tokenResp.RefreshToken
	}

	// Fetch email and project info (best-effort).
	email := fetchGeminiEmail(ctx, s.httpClient, tokenResp.AccessToken)
	projectID, tierID := loadGeminiProjectInfo(ctx, s.httpClient, tokenResp.AccessToken)
	if projectID != "" {
		creds["project_id"] = projectID
	}
	if tierID != "" {
		creds["tier_id"] = tierID
	}

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ValidateRefreshTokenResponse{
		CredentialsJson: credsJSON,
		AccountName:     email,
		TierId:          tierID,
	}, nil
}

// ---------- SetPrivacy ----------

// SetPrivacy for Gemini platform.
// Gemini does not expose a user-facing privacy-toggle API, so we return
// success with "not_applicable" mode.
func (s *accountPlatformServer) SetPrivacy(
	_ context.Context,
	_ *pb.SetPrivacyRequest,
) (*pb.SetPrivacyResponse, error) {
	return &pb.SetPrivacyResponse{
		Success:     true,
		PrivacyMode: "not_applicable",
	}, nil
}

// ---------- PostAccountCreate ----------

// PostAccountCreate is called by the host after persisting a new Gemini account.
// For Gemini, this is a no-op.
func (s *accountPlatformServer) PostAccountCreate(
	_ context.Context,
	_ *pb.PostAccountCreateRequest,
) (*pb.PostAccountCreateResponse, error) {
	return &pb.PostAccountCreateResponse{}, nil
}

// ---------- OAuth config resolution ----------

// resolveGeminiOAuthConfig determines the OAuth client, scopes, and redirect URI
// based on the oauth_type and optional params.
func resolveGeminiOAuthConfig(
	oauthType string, params map[string]string,
) (clientID, clientSecret, scopes, redirectURI string) {
	switch oauthType {
	case "ai_studio":
		// AI Studio requires a custom OAuth client. If not provided in params,
		// fall back to the built-in client (which may fail for restricted scopes).
		clientID = strings.TrimSpace(params["client_id"])
		clientSecret = strings.TrimSpace(params["client_secret"])
		if clientID == "" || clientSecret == "" {
			clientID = geminiCLIClientID
			clientSecret = geminiCLIClientSecret
			scopes = defaultCodeAssistScopes
		} else {
			scopes = defaultAIStudioScopes
		}
		redirectURI = geminiAIStudioRedirectURI
	default:
		// code_assist / google_one: always use built-in Gemini CLI client.
		clientID = geminiCLIClientID
		clientSecret = geminiCLIClientSecret
		scopes = defaultCodeAssistScopes
		redirectURI = geminiCLIRedirectURI
	}
	return
}

// ---------- PKCE / crypto helpers ----------

func geminiGenerateRandomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "="), nil
}

func geminiGenerateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func geminiComputeCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")
}

func buildGeminiAuthURL(
	clientID, scopes, redirectURI, state, codeChallenge, projectID string,
) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("include_granted_scopes", "true")
	if strings.TrimSpace(projectID) != "" {
		params.Set("project_id", strings.TrimSpace(projectID))
	}

	return fmt.Sprintf("%s?%s", geminiAuthorizeURL, params.Encode())
}
