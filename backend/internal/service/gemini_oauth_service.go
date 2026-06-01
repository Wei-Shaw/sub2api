package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	// Canonical tier IDs used by sub2api (2026-aligned).
	GeminiTierGoogleOneFree    = "google_one_free"
	GeminiTierGoogleAIPro      = "google_ai_pro"
	GeminiTierGoogleAIUltra    = "google_ai_ultra"
	GeminiTierGCPStandard      = "gcp_standard"
	GeminiTierGCPEnterprise    = "gcp_enterprise"
	GeminiTierAIStudioFree     = "aistudio_free"
	GeminiTierAIStudioPaid     = "aistudio_paid"
	GeminiTierGoogleOneUnknown = "google_one_unknown"

	// Legacy/compat tier IDs that may exist in historical data or upstream responses.
	legacyTierAIPremium          = "AI_PREMIUM"
	legacyTierGoogleOneStandard  = "GOOGLE_ONE_STANDARD"
	legacyTierGoogleOneBasic     = "GOOGLE_ONE_BASIC"
	legacyTierFree               = "FREE"
	legacyTierGoogleOneUnknown   = "GOOGLE_ONE_UNKNOWN"
	legacyTierGoogleOneUnlimited = "GOOGLE_ONE_UNLIMITED"
)

const (
	GB = 1024 * 1024 * 1024
	TB = 1024 * GB

	StorageTierUnlimited = 100 * TB // 100TB
	StorageTierAIPremium = 2 * TB   // 2TB
	StorageTierStandard  = 200 * GB // 200GB
	StorageTierBasic     = 100 * GB // 100GB
	StorageTierFree      = 15 * GB  // 15GB
)

type GeminiOAuthService struct {
	sessionStore *geminicli.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  GeminiOAuthClient
	codeAssist   GeminiCliCodeAssistClient
	driveClient  geminicli.DriveClient
	cfg          *config.Config
}

type GeminiOAuthCapabilities struct {
	AIStudioOAuthEnabled bool     `json:"ai_studio_oauth_enabled"`
	RequiredRedirectURIs []string `json:"required_redirect_uris"`
}

func NewGeminiOAuthService(
	proxyRepo ProxyRepository,
	oauthClient GeminiOAuthClient,
	codeAssist GeminiCliCodeAssistClient,
	driveClient geminicli.DriveClient,
	cfg *config.Config,
) *GeminiOAuthService {
	return &GeminiOAuthService{
		sessionStore: geminicli.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
		codeAssist:   codeAssist,
		driveClient:  driveClient,
		cfg:          cfg,
	}
}

func (s *GeminiOAuthService) GetOAuthConfig() *GeminiOAuthCapabilities {
	// AI Studio OAuth is only enabled when the operator configures a custom OAuth client.
	clientID := strings.TrimSpace(s.cfg.Gemini.OAuth.ClientID)
	clientSecret := strings.TrimSpace(s.cfg.Gemini.OAuth.ClientSecret)
	enabled := clientID != "" && clientSecret != "" && clientID != geminicli.GeminiCLIOAuthClientID

	return &GeminiOAuthCapabilities{
		AIStudioOAuthEnabled: enabled,
		RequiredRedirectURIs: []string{geminicli.AIStudioOAuthRedirectURI},
	}
}

type GeminiAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (s *GeminiOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI, projectID, oauthType, tierID string) (*GeminiAuthURLResult, error) {
	state, err := geminicli.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	codeVerifier, err := geminicli.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeChallenge := geminicli.GenerateCodeChallenge(codeVerifier)
	sessionID, err := geminicli.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	// OAuth client selection:
	// - code_assist: always use built-in Gemini CLI OAuth client (public)
	// - google_one: always use built-in Gemini CLI OAuth client (public)
	// - ai_studio: requires a user-provided OAuth client
	oauthCfg := geminicli.OAuthConfig{
		ClientID:     s.cfg.Gemini.OAuth.ClientID,
		ClientSecret: s.cfg.Gemini.OAuth.ClientSecret,
		Scopes:       s.cfg.Gemini.OAuth.Scopes,
	}
	if oauthType == "code_assist" || oauthType == "google_one" {
		// Force use of built-in Gemini CLI OAuth client
		oauthCfg.ClientID = ""
		oauthCfg.ClientSecret = ""
	}

	session := &geminicli.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		RedirectURI:  redirectURI,
		ProjectID:    strings.TrimSpace(projectID),
		TierID:       canonicalGeminiTierIDForOAuthType(oauthType, tierID),
		OAuthType:    oauthType,
		CreatedAt:    time.Now(),
	}
	s.sessionStore.Set(sessionID, session)

	effectiveCfg, err := geminicli.EffectiveOAuthConfig(oauthCfg, oauthType)
	if err != nil {
		return nil, err
	}

	isBuiltinClient := effectiveCfg.ClientID == geminicli.GeminiCLIOAuthClientID

	// AI Studio OAuth requires a user-provided OAuth client (built-in Gemini CLI client is scope-restricted).
	if oauthType == "ai_studio" && isBuiltinClient {
		return nil, fmt.Errorf("AI Studio OAuth requires a custom OAuth Client (GEMINI_OAUTH_CLIENT_ID / GEMINI_OAUTH_CLIENT_SECRET). If you don't want to configure an OAuth client, please use an AI Studio API Key account instead")
	}

	// Redirect URI strategy:
	// - built-in Gemini CLI OAuth client: use upstream redirect URI (codeassist.google.com/authcode)
	// - custom OAuth client: use localhost callback for manual copy/paste flow
	if isBuiltinClient {
		redirectURI = geminicli.GeminiCLIRedirectURI
	} else {
		redirectURI = geminicli.AIStudioOAuthRedirectURI
	}
	session.RedirectURI = redirectURI
	s.sessionStore.Set(sessionID, session)

	authURL, err := geminicli.BuildAuthorizationURL(effectiveCfg, state, codeChallenge, redirectURI, session.ProjectID, oauthType)
	if err != nil {
		return nil, err
	}

	return &GeminiAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
		State:     state,
	}, nil
}

type GeminiExchangeCodeInput struct {
	SessionID string
	State     string
	Code      string
	ProxyID   *int64
	OAuthType string // "code_assist" 或 "ai_studio"
	// TierID is a user-selected tier to be used when auto detection is unavailable or fails.
	// If empty, the service will fall back to the tier stored in the OAuth session (if any).
	TierID string
}

type GeminiTokenInfo struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int64          `json:"expires_in"`
	ExpiresAt    int64          `json:"expires_at"`
	TokenType    string         `json:"token_type"`
	Scope        string         `json:"scope,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
	OAuthType    string         `json:"oauth_type,omitempty"` // "code_assist" 或 "ai_studio"
	TierID       string         `json:"tier_id,omitempty"`    // Canonical tier id (e.g. google_one_free, gcp_standard, aistudio_free)
	Extra        map[string]any `json:"extra,omitempty"`      // Drive metadata
}

// validateTierID validates tier_id format and length
func validateTierID(tierID string) error {
	if tierID == "" {
		return nil // Empty is allowed
	}
	if len(tierID) > 64 {
		return fmt.Errorf("tier_id exceeds maximum length of 64 characters")
	}
	// Allow alphanumeric, underscore, hyphen, and slash (for tier paths)
	if !regexp.MustCompile(`^[a-zA-Z0-9_/-]+$`).MatchString(tierID) {
		return fmt.Errorf("tier_id contains invalid characters")
	}
	return nil
}

func canonicalGeminiTierID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	switch lower {
	case GeminiTierGoogleOneFree,
		GeminiTierGoogleAIPro,
		GeminiTierGoogleAIUltra,
		GeminiTierGCPStandard,
		GeminiTierGCPEnterprise,
		GeminiTierAIStudioFree,
		GeminiTierAIStudioPaid,
		GeminiTierGoogleOneUnknown:
		return lower
	}

	upper := strings.ToUpper(raw)
	switch upper {
	// Google One legacy tiers
	case legacyTierAIPremium:
		return GeminiTierGoogleAIPro
	case legacyTierGoogleOneUnlimited:
		return GeminiTierGoogleAIUltra
	case legacyTierFree, legacyTierGoogleOneBasic, legacyTierGoogleOneStandard:
		return GeminiTierGoogleOneFree
	case legacyTierGoogleOneUnknown:
		return GeminiTierGoogleOneUnknown

	// Code Assist legacy tiers
	case "STANDARD", "PRO", "LEGACY":
		return GeminiTierGCPStandard
	case "ENTERPRISE", "ULTRA":
		return GeminiTierGCPEnterprise
	}

	// Some Code Assist responses use kebab-case tier identifiers.
	switch lower {
	case "standard-tier", "pro-tier":
		return GeminiTierGCPStandard
	case "ultra-tier":
		return GeminiTierGCPEnterprise
	}

	return ""
}

func canonicalGeminiTierIDForOAuthType(oauthType, tierID string) string {
	oauthType = strings.ToLower(strings.TrimSpace(oauthType))
	canonical := canonicalGeminiTierID(tierID)
	if canonical == "" {
		return ""
	}

	switch oauthType {
	case "google_one":
		switch canonical {
		case GeminiTierGoogleOneFree, GeminiTierGoogleAIPro, GeminiTierGoogleAIUltra:
			return canonical
		default:
			return ""
		}
	case "code_assist":
		switch canonical {
		case GeminiTierGCPStandard, GeminiTierGCPEnterprise:
			return canonical
		default:
			return ""
		}
	case "ai_studio":
		switch canonical {
		case GeminiTierAIStudioFree, GeminiTierAIStudioPaid:
			return canonical
		default:
			return ""
		}
	default:
		// Unknown oauth type: accept canonical tier.
		return canonical
	}
}

// extractTierIDFromAllowedTiers extracts tierID from LoadCodeAssist response
// Prioritizes IsDefault tier, falls back to first non-empty tier
func extractTierIDFromAllowedTiers(allowedTiers []geminicli.AllowedTier) string {
	tierID := "LEGACY"
	// First pass: look for default tier
	for _, tier := range allowedTiers {
		if tier.IsDefault && strings.TrimSpace(tier.ID) != "" {
			tierID = strings.TrimSpace(tier.ID)
			break
		}
	}
	// Second pass: if still LEGACY, take first non-empty tier
	if tierID == "LEGACY" {
		for _, tier := range allowedTiers {
			if strings.TrimSpace(tier.ID) != "" {
				tierID = strings.TrimSpace(tier.ID)
				break
			}
		}
	}
	return tierID
}

// inferGoogleOneTier infers Google One tier from Drive storage limit
func inferGoogleOneTier(storageBytes int64) string {
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] inferGoogleOneTier - input: %d bytes (%.2f TB)", storageBytes, float64(storageBytes)/float64(TB))

	if storageBytes <= 0 {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] inferGoogleOneTier - storageBytes <= 0, returning UNKNOWN")
		return GeminiTierGoogleOneUnknown
	}

	if storageBytes > StorageTierUnlimited {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] inferGoogleOneTier - > %d bytes (100TB), returning UNLIMITED", int64(StorageTierUnlimited))
		return GeminiTierGoogleAIUltra
	}
	if storageBytes >= StorageTierAIPremium {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] inferGoogleOneTier - >= %d bytes (2TB), returning google_ai_pro", int64(StorageTierAIPremium))
		return GeminiTierGoogleAIPro
	}
	if storageBytes >= StorageTierFree {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] inferGoogleOneTier - >= %d bytes (15GB), returning FREE", int64(StorageTierFree))
		return GeminiTierGoogleOneFree
	}

	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] inferGoogleOneTier - < %d bytes (15GB), returning UNKNOWN", int64(StorageTierFree))
	return GeminiTierGoogleOneUnknown
}

// FetchGoogleOneTier fetches Google One tier from Drive API.
// Note: LoadCodeAssist API is NOT called for Google One accounts because:
// 1. It's designed for GCP IAM (enterprise), not personal Google accounts
// 2. Personal accounts will get 403/404 from cloudaicompanion.googleapis.com
// 3. Google consumer (Google One) and enterprise (GCP) systems are physically isolated
func (s *GeminiOAuthService) FetchGoogleOneTier(ctx context.Context, accessToken, proxyURL string) (string, *geminicli.DriveStorageInfo, error) {
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Starting FetchGoogleOneTier (Google One personal account)")

	// Use Drive API to infer tier from storage quota (requires drive.readonly scope)
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Calling Drive API for storage quota...")

	storageInfo, err := s.driveClient.GetStorageQuota(ctx, accessToken, proxyURL)
	if err != nil {
		// Check if it's a 403 (scope not granted)
		if strings.Contains(err.Error(), "status 403") {
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Drive API scope not available (403): %v", err)
			return GeminiTierGoogleOneUnknown, nil, err
		}
		// Other errors
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Failed to fetch Drive storage: %v", err)
		return GeminiTierGoogleOneUnknown, nil, err
	}

	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Drive API response - Limit: %d bytes (%.2f TB), Usage: %d bytes (%.2f GB)",
		storageInfo.Limit, float64(storageInfo.Limit)/float64(TB),
		storageInfo.Usage, float64(storageInfo.Usage)/float64(GB))

	tierID := inferGoogleOneTier(storageInfo.Limit)
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Inferred tier from storage: %s", tierID)

	return tierID, storageInfo, nil
}

// RefreshAccountGoogleOneTier 刷新单个账号的 Google One Tier
func (s *GeminiOAuthService) RefreshAccountGoogleOneTier(
	ctx context.Context,
	account *Account,
) (tierID string, extra map[string]any, credentials map[string]any, err error) {
	if account == nil {
		return "", nil, nil, fmt.Errorf("account is nil")
	}

	// 验证账号类型
	oauthType, ok := account.Credentials["oauth_type"].(string)
	if !ok || oauthType != "google_one" {
		return "", nil, nil, fmt.Errorf("not a google_one OAuth account")
	}

	// 获取 access_token
	accessToken, ok := account.Credentials["access_token"].(string)
	if !ok || accessToken == "" {
		return "", nil, nil, fmt.Errorf("missing access_token")
	}

	// 获取 proxy URL
	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 调用 Drive API
	tierID, storageInfo, err := s.FetchGoogleOneTier(ctx, accessToken, proxyURL)
	if err != nil {
		return "", nil, nil, err
	}

	// 构建 extra 数据（保留原有 extra 字段）
	extra = make(map[string]any)
	for k, v := range account.Extra {
		extra[k] = v
	}
	if storageInfo != nil {
		extra["drive_storage_limit"] = storageInfo.Limit
		extra["drive_storage_usage"] = storageInfo.Usage
		extra["drive_tier_updated_at"] = time.Now().Format(time.RFC3339)
	}

	// 构建 credentials 数据
	credentials = make(map[string]any)
	for k, v := range account.Credentials {
		credentials[k] = v
	}
	credentials["tier_id"] = tierID

	return tierID, extra, credentials, nil
}

func (s *GeminiOAuthService) ExchangeCode(ctx context.Context, input *GeminiExchangeCodeInput) (*GeminiTokenInfo, error) {
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ========== ExchangeCode START ==========")
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] SessionID: %s", input.SessionID)

	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ERROR: Session not found or expired")
		return nil, fmt.Errorf("session not found or expired")
	}
	if strings.TrimSpace(input.State) == "" || input.State != session.State {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ERROR: Invalid state")
		return nil, fmt.Errorf("invalid state")
	}

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ProxyURL: %s", proxyURL)

	redirectURI := session.RedirectURI

	// Resolve oauth_type early (defaults to code_assist for backward compatibility).
	oauthType := session.OAuthType
	if oauthType == "" {
		oauthType = "code_assist"
	}
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] OAuth Type: %s", oauthType)
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Project ID from session: %s", session.ProjectID)

	// If the session was created for AI Studio OAuth, ensure a custom OAuth client is configured.
	if oauthType == "ai_studio" {
		effectiveCfg, err := geminicli.EffectiveOAuthConfig(geminicli.OAuthConfig{
			ClientID:     s.cfg.Gemini.OAuth.ClientID,
			ClientSecret: s.cfg.Gemini.OAuth.ClientSecret,
			Scopes:       s.cfg.Gemini.OAuth.Scopes,
		}, "ai_studio")
		if err != nil {
			return nil, err
		}
		isBuiltinClient := effectiveCfg.ClientID == geminicli.GeminiCLIOAuthClientID
		if isBuiltinClient {
			return nil, fmt.Errorf("AI Studio OAuth requires a custom OAuth Client. Please use an AI Studio API Key account, or configure GEMINI_OAUTH_CLIENT_ID / GEMINI_OAUTH_CLIENT_SECRET and re-authorize")
		}
	}

	// code_assist/google_one always uses the built-in client and its fixed redirect URI.
	if oauthType == "code_assist" || oauthType == "google_one" {
		redirectURI = geminicli.GeminiCLIRedirectURI
	}

	tokenResp, err := s.oauthClient.ExchangeCode(ctx, oauthType, input.Code, session.CodeVerifier, redirectURI, proxyURL)
	if err != nil {
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ERROR: Failed to exchange code: %v", err)
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Token exchange successful")
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Token scope: %s", tokenResp.Scope)
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Token expires_in: %d seconds", tokenResp.ExpiresIn)

	sessionProjectID := strings.TrimSpace(session.ProjectID)
	s.sessionStore.Delete(input.SessionID)

	// 计算过期时间：减去 5 分钟安全时间窗口（考虑网络延迟和时钟偏差）
	// 同时设置下界保护，防止 expires_in 过小导致过去时间（引发刷新风暴）
	const safetyWindow = 300 // 5 minutes
	const minTTL = 30        // minimum 30 seconds
	expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - safetyWindow
	minExpiresAt := time.Now().Unix() + minTTL
	if expiresAt < minExpiresAt {
		expiresAt = minExpiresAt
	}

	projectID := sessionProjectID
	var tierID string
	fallbackTierID := canonicalGeminiTierIDForOAuthType(oauthType, input.TierID)
	if fallbackTierID == "" {
		fallbackTierID = canonicalGeminiTierIDForOAuthType(oauthType, session.TierID)
	}

	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ========== Account Type Detection START ==========")
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] OAuth Type: %s", oauthType)

	// 对于 code_assist 模式，project_id 是必需的，需要调用 Code Assist API
	// 对于 google_one 模式，使用个人 Google 账号，不需要 project_id，配额由 Google 网关自动识别
	// 对于 ai_studio 模式，project_id 是可选的（不影响使用 AI Studio API）
	switch oauthType {
	case "code_assist":
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Processing code_assist OAuth type")
		if projectID == "" {
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] No project_id provided, attempting to fetch from LoadCodeAssist API...")
			var err error
			projectID, tierID, err = s.fetchProjectID(ctx, tokenResp.AccessToken, proxyURL)
			if err != nil {
				// 记录警告但不阻断流程，允许后续补充 project_id
				fmt.Printf("[GeminiOAuth] Warning: Failed to fetch project_id during token exchange: %v\n", err)
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] WARNING: Failed to fetch project_id: %v", err)
			} else {
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Successfully fetched project_id: %s, tier_id: %s", projectID, tierID)
			}
		} else {
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] User provided project_id: %s, fetching tier_id...", projectID)
			// 用户手动填了 project_id，仍需调用 LoadCodeAssist 获取 tierID
			_, fetchedTierID, err := s.fetchProjectID(ctx, tokenResp.AccessToken, proxyURL)
			if err != nil {
				fmt.Printf("[GeminiOAuth] Warning: Failed to fetch tierID: %v\n", err)
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] WARNING: Failed to fetch tier_id: %v", err)
			} else {
				tierID = fetchedTierID
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Successfully fetched tier_id: %s", tierID)
			}
		}
		if strings.TrimSpace(projectID) == "" {
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ERROR: Missing project_id for Code Assist OAuth")
			return nil, fmt.Errorf("missing project_id for Code Assist OAuth: please fill Project ID (optional field) and regenerate the auth URL, or ensure your Google account has an ACTIVE GCP project")
		}
		// Prefer auto-detected tier; fall back to user-selected tier.
		tierID = canonicalGeminiTierIDForOAuthType(oauthType, tierID)
		if tierID == "" {
			if fallbackTierID != "" {
				tierID = fallbackTierID
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Using fallback tier_id from user/session: %s", tierID)
			} else {
				tierID = GeminiTierGCPStandard
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Using default tier_id: %s", tierID)
			}
		}
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Final code_assist result - project_id: %s, tier_id: %s", projectID, tierID)

	case "google_one":
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Processing google_one OAuth type")

		// Google One accounts use cloudaicompanion API, which requires a project_id.
		// For personal accounts, Google auto-assigns a project_id via the LoadCodeAssist API.
		if projectID == "" {
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] No project_id provided, attempting to fetch from LoadCodeAssist API...")
			var err error
			projectID, _, err = s.fetchProjectID(ctx, tokenResp.AccessToken, proxyURL)
			if err != nil {
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ERROR: Failed to fetch project_id: %v", err)
				return nil, fmt.Errorf("google One accounts require a project_id, failed to auto-detect: %w", err)
			}
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Successfully fetched project_id: %s", projectID)
		}

		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Attempting to fetch Google One tier from Drive API...")
		// Attempt to fetch Drive storage tier
		var storageInfo *geminicli.DriveStorageInfo
		var err error
		tierID, storageInfo, err = s.FetchGoogleOneTier(ctx, tokenResp.AccessToken, proxyURL)
		if err != nil {
			// Log warning but don't block - use fallback
			fmt.Printf("[GeminiOAuth] Warning: Failed to fetch Drive tier: %v\n", err)
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] WARNING: Failed to fetch Drive tier: %v", err)
			tierID = ""
		} else {
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Successfully fetched Drive tier: %s", tierID)
			if storageInfo != nil {
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Drive storage - Limit: %d bytes (%.2f TB), Usage: %d bytes (%.2f GB)",
					storageInfo.Limit, float64(storageInfo.Limit)/float64(TB),
					storageInfo.Usage, float64(storageInfo.Usage)/float64(GB))
			}
		}
		tierID = canonicalGeminiTierIDForOAuthType(oauthType, tierID)
		if tierID == "" || tierID == GeminiTierGoogleOneUnknown {
			if fallbackTierID != "" {
				tierID = fallbackTierID
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Using fallback tier_id from user/session: %s", tierID)
			} else {
				tierID = GeminiTierGoogleOneFree
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Using default tier_id: %s", tierID)
			}
		}
		fmt.Printf("[GeminiOAuth] Google One tierID after normalization: %s\n", tierID)

		// Store Drive info in extra field for caching
		if storageInfo != nil {
			tokenInfo := &GeminiTokenInfo{
				AccessToken:  tokenResp.AccessToken,
				RefreshToken: tokenResp.RefreshToken,
				TokenType:    tokenResp.TokenType,
				ExpiresIn:    tokenResp.ExpiresIn,
				ExpiresAt:    expiresAt,
				Scope:        tokenResp.Scope,
				ProjectID:    projectID,
				TierID:       tierID,
				OAuthType:    oauthType,
				Extra: map[string]any{
					"drive_storage_limit":   storageInfo.Limit,
					"drive_storage_usage":   storageInfo.Usage,
					"drive_tier_updated_at": time.Now().Format(time.RFC3339),
				},
			}
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ========== ExchangeCode END (google_one with storage info) ==========")
			return tokenInfo, nil
		}

	case "ai_studio":
		// No automatic tier detection for AI Studio OAuth; rely on user selection.
		if fallbackTierID != "" {
			tierID = fallbackTierID
		} else {
			tierID = GeminiTierAIStudioFree
		}

	default:
		logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Processing %s OAuth type (no tier detection)", oauthType)
	}

	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ========== Account Type Detection END ==========")

	result := &GeminiTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
		ProjectID:    projectID,
		TierID:       tierID,
		OAuthType:    oauthType,
	}
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Final result - OAuth Type: %s, Project ID: %s, Tier ID: %s", result.OAuthType, result.ProjectID, result.TierID)
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] ========== ExchangeCode END ==========")
	return result, nil
}

func (s *GeminiOAuthService) RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*GeminiTokenInfo, error) {
	var lastErr error

	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			time.Sleep(backoff)
		}

		tokenResp, err := s.oauthClient.RefreshToken(ctx, oauthType, refreshToken, proxyURL)
		if err == nil {
			// 计算过期时间：减去 5 分钟安全时间窗口（考虑网络延迟和时钟偏差）
			// 同时设置下界保护，防止 expires_in 过小导致过去时间（引发刷新风暴）
			const safetyWindow = 300 // 5 minutes
			const minTTL = 30        // minimum 30 seconds
			expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - safetyWindow
			minExpiresAt := time.Now().Unix() + minTTL
			if expiresAt < minExpiresAt {
				expiresAt = minExpiresAt
			}
			return &GeminiTokenInfo{
				AccessToken:  tokenResp.AccessToken,
				RefreshToken: tokenResp.RefreshToken,
				TokenType:    tokenResp.TokenType,
				ExpiresIn:    tokenResp.ExpiresIn,
				ExpiresAt:    expiresAt,
				Scope:        tokenResp.Scope,
			}, nil
		}

		if isNonRetryableGeminiOAuthError(err) {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("token refresh failed after retries: %w", lastErr)
}

func isNonRetryableGeminiOAuthError(err error) bool {
	msg := err.Error()
	nonRetryable := []string{
		"invalid_grant",
		"invalid_client",
		"unauthorized_client",
		"access_denied",
	}
	for _, needle := range nonRetryable {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func (s *GeminiOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*GeminiTokenInfo, error) {
	if account.Platform != PlatformGemini || account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("account is not a Gemini OAuth account")
	}

	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// Preserve oauth_type from the account (defaults to code_assist for backward compatibility).
	oauthType := strings.TrimSpace(account.GetCredential("oauth_type"))
	if oauthType == "" {
		oauthType = "code_assist"
	}

	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	tokenInfo, err := s.RefreshToken(ctx, oauthType, refreshToken, proxyURL)
	// Backward compatibility:
	// Older versions could refresh Code Assist tokens using a user-provided OAuth client when configured.
	// If the refresh token was originally issued to that custom client, forcing the built-in client will
	// fail with "unauthorized_client". In that case, retry with the custom client (ai_studio path) when available.
	if err != nil && oauthType == "code_assist" && strings.Contains(err.Error(), "unauthorized_client") && s.GetOAuthConfig().AIStudioOAuthEnabled {
		if alt, altErr := s.RefreshToken(ctx, "ai_studio", refreshToken, proxyURL); altErr == nil {
			tokenInfo = alt
			err = nil
		}
	}
	// Backward compatibility for google_one:
	// - New behavior: when a custom OAuth client is configured, google_one will use it.
	// - Old behavior: google_one always used the built-in Gemini CLI OAuth client.
	// If an existing account was authorized with the built-in client, refreshing with the custom client
	// will fail with "unauthorized_client". Retry with the built-in client (code_assist path forces it).
	if err != nil && oauthType == "google_one" && strings.Contains(err.Error(), "unauthorized_client") && s.GetOAuthConfig().AIStudioOAuthEnabled {
		if alt, altErr := s.RefreshToken(ctx, "code_assist", refreshToken, proxyURL); altErr == nil {
			tokenInfo = alt
			err = nil
		}
	}
	if err != nil {
		// Provide a more actionable error for common OAuth client mismatch issues.
		if strings.Contains(err.Error(), "unauthorized_client") {
			return nil, fmt.Errorf("%w (OAuth client mismatch: the refresh_token is bound to the OAuth client used during authorization; please re-authorize this account or restore the original GEMINI_OAUTH_CLIENT_ID/SECRET)", err)
		}
		return nil, err
	}

	tokenInfo.OAuthType = oauthType

	// Preserve account's project_id when present.
	existingProjectID := strings.TrimSpace(account.GetCredential("project_id"))
	if existingProjectID != "" {
		tokenInfo.ProjectID = existingProjectID
	}

	// 尝试从账号凭证获取 tierID（向后兼容）
	existingTierID := strings.TrimSpace(account.GetCredential("tier_id"))

	// For Code Assist, project_id is required. Auto-detect if missing.
	// For AI Studio OAuth, project_id is optional and should not block refresh.
	switch oauthType {
	case "code_assist":
		// 先设置默认值或保留旧值，确保 tier_id 始终有值
		if existingTierID != "" {
			tokenInfo.TierID = canonicalGeminiTierIDForOAuthType(oauthType, existingTierID)
		}
		if tokenInfo.TierID == "" {
			tokenInfo.TierID = GeminiTierGCPStandard
		}

		// 尝试自动探测 project_id 和 tier_id
		needDetect := strings.TrimSpace(tokenInfo.ProjectID) == "" || tokenInfo.TierID == ""
		if needDetect {
			projectID, tierID, err := s.fetchProjectID(ctx, tokenInfo.AccessToken, proxyURL)
			if err != nil {
				fmt.Printf("[GeminiOAuth] Warning: failed to auto-detect project/tier: %v\n", err)
			} else {
				if strings.TrimSpace(tokenInfo.ProjectID) == "" && projectID != "" {
					tokenInfo.ProjectID = projectID
				}
				if tierID != "" {
					if canonical := canonicalGeminiTierIDForOAuthType(oauthType, tierID); canonical != "" {
						tokenInfo.TierID = canonical
					}
				}
			}
		}

		if strings.TrimSpace(tokenInfo.ProjectID) == "" {
			return nil, fmt.Errorf("failed to auto-detect project_id: empty result")
		}
	case "google_one":
		canonicalExistingTier := canonicalGeminiTierIDForOAuthType(oauthType, existingTierID)
		// Check if tier cache is stale (> 24 hours)
		needsRefresh := true
		if account.Extra != nil {
			if updatedAtStr, ok := account.Extra["drive_tier_updated_at"].(string); ok {
				if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
					if time.Since(updatedAt) <= 24*time.Hour {
						needsRefresh = false
						// Use cached tier
						tokenInfo.TierID = canonicalExistingTier
					}
				}
			}
		}

		if tokenInfo.TierID == "" {
			tokenInfo.TierID = canonicalExistingTier
		}

		if needsRefresh {
			tierID, storageInfo, err := s.FetchGoogleOneTier(ctx, tokenInfo.AccessToken, proxyURL)
			if err == nil {
				if canonical := canonicalGeminiTierIDForOAuthType(oauthType, tierID); canonical != "" && canonical != GeminiTierGoogleOneUnknown {
					tokenInfo.TierID = canonical
				}
				if storageInfo != nil {
					tokenInfo.Extra = map[string]any{
						"drive_storage_limit":   storageInfo.Limit,
						"drive_storage_usage":   storageInfo.Usage,
						"drive_tier_updated_at": time.Now().Format(time.RFC3339),
					}
				}
			}
		}

		if tokenInfo.TierID == "" || tokenInfo.TierID == GeminiTierGoogleOneUnknown {
			if canonicalExistingTier != "" {
				tokenInfo.TierID = canonicalExistingTier
			} else {
				tokenInfo.TierID = GeminiTierGoogleOneFree
			}
		}
	}

	return tokenInfo, nil
}

func (s *GeminiOAuthService) BuildAccountCredentials(tokenInfo *GeminiTokenInfo) map[string]any {
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   strconv.FormatInt(tokenInfo.ExpiresAt, 10),
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.TokenType != "" {
		creds["token_type"] = tokenInfo.TokenType
	}
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
	}
	if tokenInfo.ProjectID != "" {
		creds["project_id"] = tokenInfo.ProjectID
	}
	if tokenInfo.TierID != "" {
		// Validate tier_id before storing
		if err := validateTierID(tokenInfo.TierID); err == nil {
			creds["tier_id"] = tokenInfo.TierID
			fmt.Printf("[GeminiOAuth] Storing tier_id: %s\n", tokenInfo.TierID)
		} else {
			fmt.Printf("[GeminiOAuth] Invalid tier_id %s: %v\n", tokenInfo.TierID, err)
		}
		// Silently skip invalid tier_id (don't block account creation)
	}
	if tokenInfo.OAuthType != "" {
		creds["oauth_type"] = tokenInfo.OAuthType
	}
	// Store extra metadata (Drive info) if present
	if len(tokenInfo.Extra) > 0 {
		for k, v := range tokenInfo.Extra {
			creds[k] = v
		}
	}
	return creds
}

func (s *GeminiOAuthService) Stop() {
	s.sessionStore.Stop()
}

func (s *GeminiOAuthService) fetchProjectID(ctx context.Context, accessToken, proxyURL string) (string, string, error) {
	if s.codeAssist == nil {
		return "", "", errors.New("code assist client not configured")
	}

	loadResp, loadErr := s.codeAssist.LoadCodeAssist(ctx, accessToken, proxyURL, nil)

	// Extract tierID from response (works whether CloudAICompanionProject is set or not)
	tierID := "LEGACY"
	if loadResp != nil {
		// First try to get tier from currentTier/paidTier fields
		if tier := loadResp.GetTier(); tier != "" {
			tierID = tier
		} else {
			// Fallback to extracting from allowedTiers
			tierID = extractTierIDFromAllowedTiers(loadResp.AllowedTiers)
		}
	}

	// If LoadCodeAssist returned a project, use it
	if loadErr == nil && loadResp != nil && strings.TrimSpace(loadResp.CloudAICompanionProject) != "" {
		return strings.TrimSpace(loadResp.CloudAICompanionProject), tierID, nil
	}

	// 关键逻辑：对齐 Gemini CLI 对“已注册用户”的处理方式。
	// 当 LoadCodeAssist 返回了 currentTier / paidTier（表示账号已注册）但没有返回 cloudaicompanionProject 时：
	// - 不要再调用 onboardUser（通常不会再分配 project_id，且可能触发 INVALID_ARGUMENT）
	// - 先尝试从 Cloud Resource Manager 获取可用项目；仍失败则提示用户手动填写 project_id
	if loadResp != nil {
		registeredTierID := strings.TrimSpace(loadResp.GetTier())
		if registeredTierID != "" {
			// 已注册但未返回 cloudaicompanionProject，这在 Google One 用户中较常见：需要用户自行提供 project_id。
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] User has tier (%s) but no cloudaicompanionProject, trying Cloud Resource Manager...", registeredTierID)

			// Try to get project from Cloud Resource Manager
			fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
			if fbErr == nil && strings.TrimSpace(fallback) != "" {
				logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] Found project from Cloud Resource Manager: %s", fallback)
				return strings.TrimSpace(fallback), tierID, nil
			}

			// No project found - user must provide project_id manually
			logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] No project found from Cloud Resource Manager, user must provide project_id manually")
			return "", tierID, fmt.Errorf("user is registered (tier: %s) but no project_id available. Please provide Project ID manually in the authorization form, or create a project at https://console.cloud.google.com", registeredTierID)
		}
	}

	// 未检测到 currentTier/paidTier，视为新用户，继续调用 onboardUser
	logger.LegacyPrintf("service.gemini_oauth", "[GeminiOAuth] No currentTier/paidTier found, proceeding with onboardUser (tierID: %s)", tierID)

	req := &geminicli.OnboardUserRequest{
		TierID: tierID,
		Metadata: geminicli.LoadCodeAssistMetadata{
			IDEType:    "ANTIGRAVITY",
			Platform:   "PLATFORM_UNSPECIFIED",
			PluginType: "GEMINI",
		},
	}

	maxAttempts := 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := s.codeAssist.OnboardUser(ctx, accessToken, proxyURL, req)
		if err != nil {
			// If Code Assist onboarding fails (e.g. INVALID_ARGUMENT), fallback to Cloud Resource Manager projects.
			fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
			if fbErr == nil && strings.TrimSpace(fallback) != "" {
				return strings.TrimSpace(fallback), tierID, nil
			}
			return "", tierID, err
		}
		if resp.Done {
			if resp.Response != nil && resp.Response.CloudAICompanionProject != nil {
				switch v := resp.Response.CloudAICompanionProject.(type) {
				case string:
					return strings.TrimSpace(v), tierID, nil
				case map[string]any:
					if id, ok := v["id"].(string); ok {
						return strings.TrimSpace(id), tierID, nil
					}
				}
			}

			fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
			if fbErr == nil && strings.TrimSpace(fallback) != "" {
				return strings.TrimSpace(fallback), tierID, nil
			}
			return "", tierID, errors.New("onboardUser completed but no project_id returned")
		}
		time.Sleep(2 * time.Second)
	}

	fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
	if fbErr == nil && strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback), tierID, nil
	}
	if loadErr != nil {
		return "", tierID, fmt.Errorf("loadCodeAssist failed (%v) and onboardUser timeout after %d attempts", loadErr, maxAttempts)
	}
	return "", tierID, fmt.Errorf("onboardUser timeout after %d attempts", maxAttempts)
}

type googleCloudProject struct {
	ProjectID      string `json:"projectId"`
	DisplayName    string `json:"name"`
	LifecycleState string `json:"lifecycleState"`
}

type googleCloudProjectsResponse struct {
	Projects []googleCloudProject `json:"projects"`
}

func fetchProjectIDFromResourceManager(ctx context.Context, accessToken, proxyURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cloudresourcemanager.googleapis.com/v1/projects", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create resource manager request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:           strings.TrimSpace(proxyURL),
		Timeout:            30 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		return "", fmt.Errorf("create http client failed: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resource manager request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read resource manager response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resource manager HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var projectsResp googleCloudProjectsResponse
	if err := json.Unmarshal(bodyBytes, &projectsResp); err != nil {
		return "", fmt.Errorf("failed to parse resource manager response: %w", err)
	}

	active := make([]googleCloudProject, 0, len(projectsResp.Projects))
	for _, p := range projectsResp.Projects {
		if p.LifecycleState == "ACTIVE" && strings.TrimSpace(p.ProjectID) != "" {
			active = append(active, p)
		}
	}
	if len(active) == 0 {
		return "", errors.New("no ACTIVE projects found from resource manager")
	}

	// Prefer likely companion projects first.
	for _, p := range active {
		id := strings.ToLower(strings.TrimSpace(p.ProjectID))
		name := strings.ToLower(strings.TrimSpace(p.DisplayName))
		if strings.Contains(id, "cloud-ai-companion") || strings.Contains(name, "cloud ai companion") || strings.Contains(name, "code assist") {
			return strings.TrimSpace(p.ProjectID), nil
		}
	}
	// Then prefer "default".
	for _, p := range active {
		id := strings.ToLower(strings.TrimSpace(p.ProjectID))
		name := strings.ToLower(strings.TrimSpace(p.DisplayName))
		if strings.Contains(id, "default") || strings.Contains(name, "default") {
			return strings.TrimSpace(p.ProjectID), nil
		}
	}

	return strings.TrimSpace(active[0].ProjectID), nil
}
