package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OpenAI OAuth constants -- mirrored from backend/internal/pkg/openai/oauth.go.
const (
	openaiAuthorizeURL       = "https://auth.openai.com/oauth/authorize"
	openaiDefaultCIDOAuth    = "app_EMoamEEZ73f0CkXaXp7hrann"
	openaiDefaultRedirectURI = "http://localhost:1455/auth/callback"
	openaiDefaultScopes      = "openid profile email offline_access"

	// oauthSessionTTL is the maximum lifetime for a pending OAuth session.
	oauthSessionTTL = 30 * time.Minute

	// Privacy constants -- mirrored from backend/internal/service/openai_privacy_service.go.
	openAISettingsURL      = "https://chatgpt.com/backend-api/settings/account_user_setting"
	privacyModeTrainingOff = "training_off"
	privacyModeFailed      = "training_set_failed"

	// responsesProbeTimeout is the timeout for the Responses endpoint probe.
	responsesProbeTimeout = 8 * time.Second
)

// oauthSession stores the state needed between GenerateAuthURL and ExchangeOAuthCode.
type oauthSession struct {
	state        string
	codeVerifier string
	redirectURI  string
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

// GenerateAuthURL generates an OpenAI OAuth authorization URL with PKCE.
func (s *accountPlatformServer) GenerateAuthURL(
	_ context.Context,
	req *pb.GenerateAuthURLRequest,
) (*pb.GenerateAuthURLResponse, error) {
	state, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	codeVerifier, err := generateRandomHex(64)
	if err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}

	sessionID, err := generateRandomHex(16)
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = openaiDefaultRedirectURI
	}

	sessionStore.set(sessionID, &oauthSession{
		state:        state,
		codeVerifier: codeVerifier,
		redirectURI:  redirectURI,
		createdAt:    time.Now(),
	})

	codeChallenge := computeCodeChallenge(codeVerifier)
	authURL := buildOpenAIAuthorizationURL(state, codeChallenge, redirectURI)

	return &pb.GenerateAuthURLResponse{
		AuthUrl:   authURL,
		SessionId: sessionID,
	}, nil
}

// ---------- ExchangeOAuthCode ----------

// ExchangeOAuthCode exchanges an OpenAI OAuth authorization code for tokens.
func (s *accountPlatformServer) ExchangeOAuthCode(
	ctx context.Context,
	req *pb.ExchangeOAuthCodeRequest,
) (*pb.ExchangeOAuthCodeResponse, error) {
	sess, ok := sessionStore.get(req.GetSessionId())
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
	}
	defer sessionStore.delete(req.GetSessionId())

	redirectURI := sess.redirectURI
	if override := strings.TrimSpace(req.GetRedirectUri()); override != "" {
		redirectURI = override
	}

	tokenResp, err := exchangeOpenAICode(
		ctx, s.httpClient,
		req.GetCode(), sess.codeVerifier, redirectURI,
	)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	creds := buildOpenAITokenCredentials(tokenResp)
	accountName := deriveOpenAIAccountName(tokenResp)

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ExchangeOAuthCodeResponse{
		CredentialsJson: credsJSON,
		AccountName:     accountName,
	}, nil
}

// ---------- ValidateRefreshToken ----------

// ValidateRefreshToken validates a refresh token by exchanging it for a new
// access token via auth.openai.com.
func (s *accountPlatformServer) ValidateRefreshToken(
	ctx context.Context,
	req *pb.ValidateRefreshTokenRequest,
) (*pb.ValidateRefreshTokenResponse, error) {
	refreshToken := strings.TrimSpace(req.GetRefreshToken())
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}

	tokenResp, err := callOpenAITokenEndpoint(
		ctx, s.httpClient, refreshToken, openaiDefaultCIDOAuth,
	)
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
	}

	creds := buildValidateRefreshCredentials(refreshToken, tokenResp)
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	return &pb.ValidateRefreshTokenResponse{
		CredentialsJson: credsJSON,
	}, nil
}

// ---------- CookieAuth ----------

// CookieAuth is not applicable for OpenAI (no browser-cookie auth flow).
func (s *accountPlatformServer) CookieAuth(
	_ context.Context,
	_ *pb.CookieAuthRequest,
) (*pb.CookieAuthResponse, error) {
	return nil, status.Error(codes.Unimplemented, "gateway-openai: cookie auth not supported")
}

// ---------- SetPrivacy ----------

// SetPrivacy disables OpenAI's "Improve the model for everyone" training
// data sharing via the ChatGPT settings API.
func (s *accountPlatformServer) SetPrivacy(
	ctx context.Context,
	req *pb.SetPrivacyRequest,
) (*pb.SetPrivacyResponse, error) {
	var creds map[string]any
	if len(req.GetCredentialsJson()) > 0 {
		if err := json.Unmarshal(req.GetCredentialsJson(), &creds); err != nil {
			return &pb.SetPrivacyResponse{
				Success: false,
				Error:   "failed to parse credentials: " + err.Error(),
			}, nil
		}
	}

	accessToken := gatewayutil.CredStr(creds, "access_token")
	if accessToken == "" {
		return &pb.SetPrivacyResponse{
			Success: false,
			Error:   "no access_token in credentials",
		}, nil
	}

	if !req.GetForce() {
		if mode := extractPrivacyMode(req.GetExtraJson()); shouldSkipPrivacy(mode) {
			return &pb.SetPrivacyResponse{
				Success:     true,
				PrivacyMode: mode,
			}, nil
		}
	}

	mode := disableTraining(ctx, s.httpClient, accessToken)
	return &pb.SetPrivacyResponse{
		Success:     mode == privacyModeTrainingOff,
		PrivacyMode: mode,
		Error:       privacyErrorMessage(mode),
	}, nil
}

// ---------- PostAccountCreate ----------

// PostAccountCreate performs post-creation setup for OpenAI accounts:
// - For API key accounts: probes /v1/responses to detect endpoint support.
// - For OAuth accounts: attempts to disable training data sharing.
func (s *accountPlatformServer) PostAccountCreate(
	ctx context.Context,
	req *pb.PostAccountCreateRequest,
) (*pb.PostAccountCreateResponse, error) {
	var creds map[string]any
	if len(req.GetCredentialsJson()) > 0 {
		_ = json.Unmarshal(req.GetCredentialsJson(), &creds)
	}

	switch req.GetAccountType() {
	case accountTypeAPIKey:
		return s.postCreateAPIKeyProbe(ctx, creds)
	case accountTypeOAuth, accountTypeSetupToken:
		return s.postCreateOAuthPrivacy(ctx, creds)
	default:
		return &pb.PostAccountCreateResponse{}, nil
	}
}

// postCreateAPIKeyProbe probes the upstream /v1/responses endpoint.
func (s *accountPlatformServer) postCreateAPIKeyProbe(
	ctx context.Context, creds map[string]any,
) (*pb.PostAccountCreateResponse, error) {
	apiKey := gatewayutil.CredStr(creds, "api_key")
	if apiKey == "" {
		return &pb.PostAccountCreateResponse{}, nil
	}

	baseURL := gatewayutil.CredStr(creds, "base_url")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	responsesURL := strings.TrimSuffix(baseURL, "/") + "/v1/responses"

	supported := probeResponsesEndpoint(ctx, s.httpClient, responsesURL, apiKey)

	extraUpdate := map[string]any{
		"openai_responses_supported": supported,
	}
	extraJSON, _ := json.Marshal(extraUpdate)

	return &pb.PostAccountCreateResponse{
		UpdatedExtraJson: extraJSON,
	}, nil
}

// postCreateOAuthPrivacy attempts to disable training data sharing.
func (s *accountPlatformServer) postCreateOAuthPrivacy(
	ctx context.Context, creds map[string]any,
) (*pb.PostAccountCreateResponse, error) {
	accessToken := gatewayutil.CredStr(creds, "access_token")
	if accessToken == "" {
		return &pb.PostAccountCreateResponse{}, nil
	}

	mode := disableTraining(ctx, s.httpClient, accessToken)
	if mode == "" {
		return &pb.PostAccountCreateResponse{}, nil
	}

	extraUpdate := map[string]any{"privacy_mode": mode}
	updatedExtra, _ := json.Marshal(extraUpdate)

	return &pb.PostAccountCreateResponse{
		UpdatedExtraJson: updatedExtra,
	}, nil
}

// probeResponsesEndpoint sends a minimal request to /v1/responses and
// returns true if the endpoint exists (non-404/405 response).
func probeResponsesEndpoint(
	ctx context.Context, client *http.Client,
	responsesURL, apiKey string,
) bool {
	probeCtx, cancel := context.WithTimeout(ctx, responsesProbeTimeout)
	defer cancel()

	payload, _ := json.Marshal(map[string]any{
		"model": "gpt-4o",
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "hi"},
				},
			},
		},
		"instructions": "You are a helpful assistant.",
		"stream":       false,
	})

	req, err := http.NewRequestWithContext(
		probeCtx, http.MethodPost, responsesURL, bytes.NewReader(payload),
	)
	if err != nil {
		return true // assume supported on error
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return true // network error: assume supported
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	// Only 404/405 means endpoint doesn't exist
	return resp.StatusCode != http.StatusNotFound &&
		resp.StatusCode != http.StatusMethodNotAllowed
}
