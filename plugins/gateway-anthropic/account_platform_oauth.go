package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Claude OAuth constants -- mirrored from backend/internal/pkg/oauth/oauth.go.
// The plugin cannot import host internal packages, so these are duplicated.
const (
	claudeAuthorizeURL = "https://claude.ai/oauth/authorize"
	claudeRedirectURI  = "https://platform.claude.com/oauth/code/callback"
	claudeBaseURL      = "https://claude.ai"

	// scopeOAuth is the full OAuth scope for browser-based authorization.
	scopeOAuth = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// scopeAPI is the scope for internal API calls (org:create_api_key not supported).
	scopeAPI = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// scopeInference is the minimal scope for setup-token accounts.
	scopeInference = "user:inference"

	// oauthSessionTTL is the maximum lifetime for a pending OAuth session.
	oauthSessionTTL = 30 * time.Minute

	// setupTokenExpiresIn is 1 year in seconds (365 * 24 * 60 * 60).
	setupTokenExpiresIn = 31536000
)

// oauthSession stores the state needed between GenerateAuthURL and ExchangeOAuthCode.
type oauthSession struct {
	state        string
	codeVerifier string
	scope        string
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

// GenerateAuthURL generates a Claude OAuth authorization URL with PKCE.
// The oauth_type param determines the scope: "setup-token" -> inference only,
// anything else -> full scope.
func (s *accountPlatformServer) GenerateAuthURL(
	_ context.Context,
	req *pb.GenerateAuthURLRequest,
) (*pb.GenerateAuthURLResponse, error) {
	scope := scopeOAuth
	if req.GetOauthType() == accountTypeSetupToken {
		scope = scopeInference
	}

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
		scope:        scope,
		createdAt:    time.Now(),
	})

	authURL := buildAuthorizationURL(state, codeChallenge, scope)

	return &pb.GenerateAuthURLResponse{
		AuthUrl:   authURL,
		SessionId: sessionID,
	}, nil
}

// ---------- ExchangeOAuthCode ----------

// ExchangeOAuthCode exchanges an OAuth authorization code for Claude credentials.
func (s *accountPlatformServer) ExchangeOAuthCode(
	ctx context.Context,
	req *pb.ExchangeOAuthCodeRequest,
) (*pb.ExchangeOAuthCodeResponse, error) {
	sess, ok := sessionStore.get(req.GetSessionId())
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
	}
	defer sessionStore.delete(req.GetSessionId())

	isSetupToken := sess.scope == scopeInference
	tokenResp, err := exchangeCodeForToken(
		ctx, s.httpClient,
		req.GetCode(), sess.codeVerifier, sess.state, isSetupToken,
	)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	creds := buildTokenCredentials(tokenResp)
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ExchangeOAuthCodeResponse{
		CredentialsJson: credsJSON,
		AccountName:     deriveAccountName(tokenResp),
	}, nil
}

// ---------- ValidateRefreshToken ----------

// ValidateRefreshToken validates a refresh token by exchanging it for a new
// access token.
func (s *accountPlatformServer) ValidateRefreshToken(
	ctx context.Context,
	req *pb.ValidateRefreshTokenRequest,
) (*pb.ValidateRefreshTokenResponse, error) {
	refreshToken := strings.TrimSpace(req.GetRefreshToken())
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}

	tokenResp, err := callClaudeTokenEndpoint(ctx, s.httpClient, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
	}

	creds := buildRefreshCredentials(refreshToken, tokenResp)
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ValidateRefreshTokenResponse{
		CredentialsJson: credsJSON,
	}, nil
}

// ---------- CookieAuth ----------

// CookieAuth performs the full cookie-based OAuth flow for Claude:
//  1. Get organization UUID via session cookie
//  2. Generate PKCE values
//  3. Get authorization code via session cookie
//  4. Exchange code for token
func (s *accountPlatformServer) CookieAuth(
	ctx context.Context,
	req *pb.CookieAuthRequest,
) (*pb.CookieAuthResponse, error) {
	sessionKey := strings.TrimSpace(req.GetSessionKey())
	if sessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}

	scope, isSetupToken := resolveCookieAuthScope(req.GetScope())

	tokenInfo, err := performCookieOAuthFlow(
		ctx, s.httpClient, sessionKey, scope, isSetupToken,
	)
	if err != nil {
		return nil, err
	}

	credsJSON, err := json.Marshal(tokenInfo.creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.CookieAuthResponse{
		CredentialsJson: credsJSON,
		AccountName:     tokenInfo.accountName,
	}, nil
}

// ---------- SetPrivacy ----------

// SetPrivacy for Claude/Anthropic platform.
// Claude does not expose a user-facing privacy-toggle API (unlike OpenAI or
// Antigravity), so we return success with a "not_applicable" mode. The host
// will persist this to indicate the RPC was called.
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

// PostAccountCreate is called by the host after persisting a new Anthropic
// account. Currently a no-op that returns empty (no updates needed).
func (s *accountPlatformServer) PostAccountCreate(
	_ context.Context,
	_ *pb.PostAccountCreateRequest,
) (*pb.PostAccountCreateResponse, error) {
	return &pb.PostAccountCreateResponse{}, nil
}
