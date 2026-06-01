package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"net/url"
	"strings"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Antigravity OAuth constants -- mirrored from backend/internal/pkg/antigravity/oauth.go.
// The plugin cannot import host internal packages, so these are duplicated.
const (
	antigravityAuthorizeURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	antigravityOAuthTokenURL = "https://oauth2.googleapis.com/token"
	antigravityUserInfoURL   = "https://www.googleapis.com/oauth2/v2/userinfo"
	antigravityRedirectURI   = "http://localhost:8085/callback"
	antigravityOAuthScopes   = "https://www.googleapis.com/auth/cloud-platform " +
		"https://www.googleapis.com/auth/userinfo.email " +
		"https://www.googleapis.com/auth/userinfo.profile " +
		"https://www.googleapis.com/auth/cclog " +
		"https://www.googleapis.com/auth/experimentsandconfigs"

	// antigravityDefaultClientSecret is the default OAuth client_secret.
	antigravityDefaultClientSecret = "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"

	// oauthSessionTTL is the maximum lifetime for a pending OAuth session.
	oauthSessionTTL = 30 * time.Minute

	// Privacy API constants.
	privacyAPIBaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	privacyModeSet    = "privacy_set"
	privacyModeFailed = "privacy_set_failed"
)

// ---------- Session store ----------

// oauthSession stores the state needed between GenerateAuthURL and ExchangeOAuthCode.
type oauthSession struct {
	state        string
	codeVerifier string
	createdAt    time.Time
}

// oauthSessionStore manages pending OAuth sessions in plugin memory.
type oauthSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*oauthSession
}

var sessionStore = &oauthSessionStore{
	sessions: make(map[string]*oauthSession),
}

func init() {
	go sessionStore.cleanupLoop()
}

func (s *oauthSessionStore) set(id string, sess *oauthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = sess
}

func (s *oauthSessionStore) get(id string) (*oauthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok || time.Since(sess.createdAt) > oauthSessionTTL {
		return nil, false
	}
	return sess, true
}

func (s *oauthSessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *oauthSessionStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			if time.Since(sess.createdAt) > oauthSessionTTL {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// ---------- GenerateAuthURL ----------

// GenerateAuthURL generates a Google OAuth authorization URL for Antigravity.
func (s *accountPlatformServer) GenerateAuthURL(
	_ context.Context,
	_ *pb.GenerateAuthURLRequest,
) (*pb.GenerateAuthURLResponse, error) {
	state, err := generateRandomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	codeVerifier, err := generateRandomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}
	codeChallenge := computeCodeChallenge(codeVerifier)

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	sessionStore.set(sessionID, &oauthSession{
		state:        state,
		codeVerifier: codeVerifier,
		createdAt:    time.Now(),
	})

	authURL := buildAntigravityAuthURL(state, codeChallenge)

	return &pb.GenerateAuthURLResponse{
		AuthUrl:   authURL,
		SessionId: sessionID,
	}, nil
}

// ---------- ExchangeOAuthCode ----------

// ExchangeOAuthCode exchanges an OAuth authorization code for Antigravity credentials.
func (s *accountPlatformServer) ExchangeOAuthCode(
	ctx context.Context,
	req *pb.ExchangeOAuthCodeRequest,
) (*pb.ExchangeOAuthCodeResponse, error) {
	sess, ok := sessionStore.get(req.GetSessionId())
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
	}
	defer sessionStore.delete(req.GetSessionId())

	tokenResp, err := exchangeAntigravityCode(ctx, s.httpClient, req.GetCode(), sess.codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - 300
	creds := map[string]any{
		"access_token":  tokenResp.AccessToken,
		"refresh_token": tokenResp.RefreshToken,
		"expires_at":    fmt.Sprintf("%d", expiresAt),
		"token_type":    tokenResp.TokenType,
	}

	accountName := fetchAntigravityEmail(ctx, s.httpClient, tokenResp.AccessToken)
	if accountName != "" {
		creds["email"] = accountName
	}

	// Attempt to load project_id and plan_type.
	projectID, planType := loadAntigravityProjectInfo(ctx, s.httpClient, tokenResp.AccessToken)
	if projectID != "" {
		creds["project_id"] = projectID
	}
	if planType != "" {
		creds["plan_type"] = planType
	}

	// Set privacy immediately after token exchange.
	privacyMode := setAntigravityPrivacyViaAPI(ctx, s.httpClient, tokenResp.AccessToken, projectID)

	var extraJSON []byte
	if privacyMode != "" {
		extra := map[string]any{"privacy_mode": privacyMode}
		extraJSON, _ = json.Marshal(extra)
	}

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ExchangeOAuthCodeResponse{
		CredentialsJson: credsJSON,
		ExtraJson:       extraJSON,
		AccountName:     accountName,
	}, nil
}

// ---------- ValidateRefreshToken ----------

// ValidateRefreshToken validates an Antigravity refresh token by refreshing it
// and fetching full account info (email, project_id, plan_type, privacy).
func (s *accountPlatformServer) ValidateRefreshToken(
	ctx context.Context,
	req *pb.ValidateRefreshTokenRequest,
) (*pb.ValidateRefreshTokenResponse, error) {
	refreshToken := strings.TrimSpace(req.GetRefreshToken())
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}

	tokenResp, err := refreshAntigravityToken(ctx, s.httpClient, refreshToken, "")
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
	}

	expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - 300
	creds := map[string]any{
		"access_token":  tokenResp.AccessToken,
		"refresh_token": refreshToken,
		"expires_at":    fmt.Sprintf("%d", expiresAt),
		"token_type":    tokenResp.TokenType,
	}

	// Preserve new refresh_token if returned.
	if strings.TrimSpace(tokenResp.RefreshToken) != "" {
		creds["refresh_token"] = tokenResp.RefreshToken
	}

	email := fetchAntigravityEmail(ctx, s.httpClient, tokenResp.AccessToken)
	if email != "" {
		creds["email"] = email
	}

	projectID, planType := loadAntigravityProjectInfo(ctx, s.httpClient, tokenResp.AccessToken)
	if projectID != "" {
		creds["project_id"] = projectID
	}
	if planType != "" {
		creds["plan_type"] = planType
	}

	privacyMode := setAntigravityPrivacyViaAPI(ctx, s.httpClient, tokenResp.AccessToken, projectID)

	var extraJSON []byte
	if privacyMode != "" {
		extra := map[string]any{"privacy_mode": privacyMode}
		extraJSON, _ = json.Marshal(extra)
	}

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ValidateRefreshTokenResponse{
		CredentialsJson: credsJSON,
		ExtraJson:       extraJSON,
		AccountName:     email,
	}, nil
}

// ---------- SetPrivacy ----------

// SetPrivacy sets the privacy/data-collection mode for an Antigravity account.
// It calls the Antigravity setUserSettings API to clear telemetry settings,
// then verifies via fetchUserInfo.
func (s *accountPlatformServer) SetPrivacy(
	ctx context.Context,
	req *pb.SetPrivacyRequest,
) (*pb.SetPrivacyResponse, error) {
	var creds map[string]any
	if err := json.Unmarshal(req.GetCredentialsJson(), &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	accessToken := gatewayutil.CredStr(creds, "access_token")
	if accessToken == "" {
		return &pb.SetPrivacyResponse{
			Success: false,
			Error:   "no access_token available",
		}, nil
	}

	projectID := gatewayutil.CredStr(creds, "project_id")

	mode := setAntigravityPrivacyViaAPI(ctx, s.httpClient, accessToken, projectID)
	if mode == privacyModeSet {
		return &pb.SetPrivacyResponse{
			Success:     true,
			PrivacyMode: privacyModeSet,
		}, nil
	}

	return &pb.SetPrivacyResponse{
		Success:     false,
		PrivacyMode: privacyModeFailed,
		Error:       "privacy setting failed or could not be verified",
	}, nil
}

// ---------- PostAccountCreate ----------

// PostAccountCreate is called by the host after persisting a new Antigravity account.
// It attempts to set privacy for the new account.
func (s *accountPlatformServer) PostAccountCreate(
	ctx context.Context,
	req *pb.PostAccountCreateRequest,
) (*pb.PostAccountCreateResponse, error) {
	if req.GetAccountType() != accountTypeOAuth {
		return &pb.PostAccountCreateResponse{}, nil
	}

	var creds map[string]any
	if err := json.Unmarshal(req.GetCredentialsJson(), &creds); err != nil {
		return &pb.PostAccountCreateResponse{}, nil
	}

	accessToken := gatewayutil.CredStr(creds, "access_token")
	projectID := gatewayutil.CredStr(creds, "project_id")

	if accessToken == "" {
		return &pb.PostAccountCreateResponse{}, nil
	}

	mode := setAntigravityPrivacyViaAPI(ctx, s.httpClient, accessToken, projectID)
	if mode != "" {
		extra := map[string]any{"privacy_mode": mode}
		extraJSON, _ := json.Marshal(extra)
		return &pb.PostAccountCreateResponse{
			UpdatedExtraJson: extraJSON,
		}, nil
	}

	return &pb.PostAccountCreateResponse{}, nil
}

// ---------- PKCE / crypto helpers ----------

func generateRandomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "="), nil
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func computeCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")
}

func buildAntigravityAuthURL(state, codeChallenge string) string {
	params := url.Values{}
	params.Set("client_id", googleClientID)
	params.Set("redirect_uri", antigravityRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", antigravityOAuthScopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("include_granted_scopes", "true")

	return fmt.Sprintf("%s?%s", antigravityAuthorizeURL, params.Encode())
}
