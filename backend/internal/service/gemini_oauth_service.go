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
)

const (
	TierAIPremium          = "AI_PREMIUM"
	TierGoogleOneStandard  = "GOOGLE_ONE_STANDARD"
	TierGoogleOneBasic     = "GOOGLE_ONE_BASIC"
	TierFree               = "FREE"
	TierGoogleOneUnknown   = "GOOGLE_ONE_UNKNOWN"
	TierGoogleOneUnlimited = "GOOGLE_ONE_UNLIMITED"
)

type GeminiOAuthService struct {
	sessionStore *geminicli.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  GeminiOAuthClient
	codeAssist   GeminiCliCodeAssistClient
	cfg          *config.Config
REDACTED

type GeminiOAuthCapabilities struct {
	AIStudioOAuthEnabled bool     `json:"ai_studio_oauth_enabled"`
	RequiredRedirectURIs []string `json:"required_redirect_uris"`
REDACTED

func NewGeminiOAuthService(
	proxyRepo ProxyRepository,
	oauthClient GeminiOAuthClient,
	codeAssist GeminiCliCodeAssistClient,
	cfg *config.Config,
) *GeminiOAuthService {
	return &GeminiOAuthService{
		sessionStore: geminicli.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
		codeAssist:   codeAssist,
		cfg:          cfg,
REDACTED
REDACTED

func (s *GeminiOAuthService) GetOAuthConfig() *GeminiOAuthCapabilities {
	// AI Studio OAuth is only enabled when the operator configures a custom OAuth client.
	clientID := strings.TrimSpace(s.cfg.Gemini.OAuth.ClientID)
	clientSecret := strings.TrimSpace(s.cfg.Gemini.OAuth.ClientSecret)
	enabled := clientID != "" && clientSecret != "" &&
		(clientID != geminicli.GeminiCLIOAuthClientID || clientSecret != geminicli.GeminiCLIOAuthClientSecret)

	return &GeminiOAuthCapabilities{
		AIStudioOAuthEnabled: enabled,
		RequiredRedirectURIs: []string{geminicli.AIStudioOAuthRedirectURIREDACTED,
REDACTED
REDACTED

type GeminiAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
REDACTED

func (s *GeminiOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI, projectID, oauthType string) (*GeminiAuthURLResult, error) {
	state, err := geminicli.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
REDACTED
	codeVerifier, err := geminicli.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
REDACTED
	codeChallenge := geminicli.GenerateCodeChallenge(codeVerifier)
	sessionID, err := geminicli.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
REDACTED

	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// OAuth client selection:
	// - code_assist: always use built-in Gemini CLI OAuth client (public), regardless of configured client_id/secret.
	// - google_one: same as code_assist, uses built-in client for personal Google accounts.
	// - ai_studio: requires a user-provided OAuth client.
	oauthCfg := geminicli.OAuthConfig{
		ClientID:     s.cfg.Gemini.OAuth.ClientID,
		ClientSecret: s.cfg.Gemini.OAuth.ClientSecret,
		Scopes:       s.cfg.Gemini.OAuth.Scopes,
REDACTED
	if oauthType == "code_assist" || oauthType == "google_one" {
		oauthCfg.ClientID = ""
		oauthCfg.ClientSecret = ""
REDACTED

	session := &geminicli.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		RedirectURI:  redirectURI,
		ProjectID:    strings.TrimSpace(projectID),
		OAuthType:    oauthType,
		CreatedAt:    time.Now(),
REDACTED
	s.sessionStore.Set(sessionID, session)

	effectiveCfg, err := geminicli.EffectiveOAuthConfig(oauthCfg, oauthType)
	if err != nil {
		return nil, err
REDACTED

	isBuiltinClient := effectiveCfg.ClientID == geminicli.GeminiCLIOAuthClientID &&
		effectiveCfg.ClientSecret == geminicli.GeminiCLIOAuthClientSecret

	// AI Studio OAuth requires a user-provided OAuth client (built-in Gemini CLI client is scope-restricted).
	if oauthType == "ai_studio" && isBuiltinClient {
		return nil, fmt.Errorf("AI Studio OAuth requires a custom OAuth Client (GEMINI_OAUTH_CLIENT_ID / GEMINI_OAUTH_CLIENT_SECRET). If you don't want to configure an OAuth client, please use an AI Studio API Key account instead")
REDACTED

	// Redirect URI strategy:
	// - code_assist: use Gemini CLI redirect URI (codeassist.google.com/authcode)
	// - ai_studio: use localhost callback for manual copy/paste flow
	if oauthType == "code_assist" {
		redirectURI = geminicli.GeminiCLIRedirectURI
REDACTED else {
		redirectURI = geminicli.AIStudioOAuthRedirectURI
REDACTED
	session.RedirectURI = redirectURI
	s.sessionStore.Set(sessionID, session)

	authURL, err := geminicli.BuildAuthorizationURL(effectiveCfg, state, codeChallenge, redirectURI, session.ProjectID, oauthType)
	if err != nil {
		return nil, err
REDACTED

	return &GeminiAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
		State:     state,
REDACTED, nil
REDACTED

type GeminiExchangeCodeInput struct {
	SessionID string
	State     string
	Code      string
	ProxyID   *int64
	OAuthType string // "code_assist" 或 "ai_studio"
REDACTED

type GeminiTokenInfo struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int64          `json:"expires_in"`
	ExpiresAt    int64          `json:"expires_at"`
	TokenType    string         `json:"token_type"`
	Scope        string         `json:"scope,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
	OAuthType    string         `json:"oauth_type,omitempty"` // "code_assist" 或 "ai_studio"
	TierID       string         `json:"tier_id,omitempty"`    // Gemini Code Assist tier: LEGACY/PRO/ULTRA
	Extra        map[string]any `json:"extra,omitempty"`      // Drive metadata
REDACTED

// validateTierID validates tier_id format and length
func validateTierID(tierID string) error {
	if tierID == "" {
		return nil // Empty is allowed
REDACTED
	if len(tierID) > 64 {
		return fmt.Errorf("tier_id exceeds maximum length of 64 characters")
REDACTED
	// Allow alphanumeric, underscore, hyphen, and slash (for tier paths)
	if !regexp.MustCompile(`^[a-zA-Z0-9_/-]+$`).MatchString(tierID) {
		return fmt.Errorf("tier_id contains invalid characters")
REDACTED
	return nil
REDACTED

// extractTierIDFromAllowedTiers extracts tierID from LoadCodeAssist response
// Prioritizes IsDefault tier, falls back to first non-empty tier
func extractTierIDFromAllowedTiers(allowedTiers []geminicli.AllowedTier) string {
	tierID := "LEGACY"
	// First pass: look for default tier
	for _, tier := range allowedTiers {
		if tier.IsDefault && strings.TrimSpace(tier.ID) != "" {
			tierID = strings.TrimSpace(tier.ID)
			break
	REDACTED
REDACTED
	// Second pass: if still LEGACY, take first non-empty tier
	if tierID == "LEGACY" {
		for _, tier := range allowedTiers {
			if strings.TrimSpace(tier.ID) != "" {
				tierID = strings.TrimSpace(tier.ID)
				break
		REDACTED
	REDACTED
REDACTED
	return tierID
REDACTED

// inferGoogleOneTier infers Google One tier from Drive storage limit
func inferGoogleOneTier(storageBytes int64) string {
	if storageBytes <= 0 {
		return TierGoogleOneUnknown
REDACTED

	// Unlimited storage (G Suite legacy)
	if storageBytes > 100*1024*1024*1024*1024 { // > 100TB
		return TierGoogleOneUnlimited
REDACTED

	// AI Premium (2TB+)
	if storageBytes >= 2*1024*1024*1024*1024 { // >= 2TB
		return TierAIPremium
REDACTED

	// Google One Standard (200GB)
	if storageBytes >= 200*1024*1024*1024 { // >= 200GB
		return TierGoogleOneStandard
REDACTED

	// Google One Basic (100GB)
	if storageBytes >= 100*1024*1024*1024 { // >= 100GB
		return TierGoogleOneBasic
REDACTED

	// Free (15GB)
	if storageBytes >= 15*1024*1024*1024 { // >= 15GB
		return TierFree
REDACTED

	return TierGoogleOneUnknown
REDACTED

// fetchGoogleOneTier fetches Google One tier from Drive API
func (s *GeminiOAuthService) FetchGoogleOneTier(ctx context.Context, accessToken, proxyURL string) (string, *geminicli.DriveStorageInfo, error) {
	driveClient := geminicli.NewDriveClient()

	storageInfo, err := driveClient.GetStorageQuota(ctx, accessToken, proxyURL)
	if err != nil {
		// Check if it's a 403 (scope not granted)
		if strings.Contains(err.Error(), "status 403") {
			fmt.Printf("[GeminiOAuth] Drive API scope not available: %v\n", err)
			return TierGoogleOneUnknown, nil, err
	REDACTED
		// Other errors
		fmt.Printf("[GeminiOAuth] Failed to fetch Drive storage: %v\n", err)
		return TierGoogleOneUnknown, nil, err
REDACTED

	tierID := inferGoogleOneTier(storageInfo.Limit)
	return tierID, storageInfo, nil
REDACTED

func (s *GeminiOAuthService) ExchangeCode(ctx context.Context, input *GeminiExchangeCodeInput) (*GeminiTokenInfo, error) {
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
REDACTED
	if strings.TrimSpace(input.State) == "" || input.State != session.State {
		return nil, fmt.Errorf("invalid state")
REDACTED

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	redirectURI := session.RedirectURI

	// Resolve oauth_type early (defaults to code_assist for backward compatibility).
	oauthType := session.OAuthType
	if oauthType == "" {
		oauthType = "code_assist"
REDACTED

	// If the session was created for AI Studio OAuth, ensure a custom OAuth client is configured.
	if oauthType == "ai_studio" {
		effectiveCfg, err := geminicli.EffectiveOAuthConfig(geminicli.OAuthConfig{
			ClientID:     s.cfg.Gemini.OAuth.ClientID,
			ClientSecret: s.cfg.Gemini.OAuth.ClientSecret,
			Scopes:       s.cfg.Gemini.OAuth.Scopes,
	REDACTED, "ai_studio")
		if err != nil {
			return nil, err
	REDACTED
		isBuiltinClient := effectiveCfg.ClientID == geminicli.GeminiCLIOAuthClientID &&
			effectiveCfg.ClientSecret == geminicli.GeminiCLIOAuthClientSecret
		if isBuiltinClient {
			return nil, fmt.Errorf("AI Studio OAuth requires a custom OAuth Client. Please use an AI Studio API Key account, or configure GEMINI_OAUTH_CLIENT_ID / GEMINI_OAUTH_CLIENT_SECRET and re-authorize")
	REDACTED
REDACTED

	// code_assist always uses the built-in client and its fixed redirect URI.
	if oauthType == "code_assist" {
		redirectURI = geminicli.GeminiCLIRedirectURI
REDACTED

	tokenResp, err := s.oauthClient.ExchangeCode(ctx, oauthType, input.Code, session.CodeVerifier, redirectURI, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
REDACTED
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
REDACTED

	projectID := sessionProjectID
	var tierID string

	// 对于 code_assist 模式，project_id 是必需的，需要调用 Code Assist API
	// 对于 google_one 模式，使用个人 Google 账号，不需要 project_id，配额由 Google 网关自动识别
	// 对于 ai_studio 模式，project_id 是可选的（不影响使用 AI Studio API）
	switch oauthType {
	case "code_assist":
		if projectID == "" {
			var err error
			projectID, tierID, err = s.fetchProjectID(ctx, tokenResp.AccessToken, proxyURL)
			if err != nil {
				// 记录警告但不阻断流程，允许后续补充 project_id
				fmt.Printf("[GeminiOAuth] Warning: Failed to fetch project_id during token exchange: %v\n", err)
		REDACTED
	REDACTED else {
			// 用户手动填了 project_id，仍需调用 LoadCodeAssist 获取 tierID
			_, fetchedTierID, err := s.fetchProjectID(ctx, tokenResp.AccessToken, proxyURL)
			if err != nil {
				fmt.Printf("[GeminiOAuth] Warning: Failed to fetch tierID: %v\n", err)
		REDACTED else {
				tierID = fetchedTierID
		REDACTED
	REDACTED
		if strings.TrimSpace(projectID) == "" {
			return nil, fmt.Errorf("missing project_id for Code Assist OAuth: please fill Project ID (optional field) and regenerate the auth URL, or ensure your Google account has an ACTIVE GCP project")
	REDACTED
		// tierID 缺失时使用默认值
		if tierID == "" {
			tierID = "LEGACY"
	REDACTED
	case "google_one":
		// Attempt to fetch Drive storage tier
		tierID, storageInfo, err := s.FetchGoogleOneTier(ctx, tokenResp.AccessToken, proxyURL)
		if err != nil {
			// Log warning but don't block - use fallback
			fmt.Printf("[GeminiOAuth] Warning: Failed to fetch Drive tier: %v\n", err)
			tierID = TierGoogleOneUnknown
	REDACTED

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
			REDACTED,
		REDACTED
			return tokenInfo, nil
	REDACTED
REDACTED
	// ai_studio 模式不设置 tierID，保持为空

	return &GeminiTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
		ProjectID:    projectID,
		TierID:       tierID,
		OAuthType:    oauthType,
REDACTED, nil
REDACTED

func (s *GeminiOAuthService) RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*GeminiTokenInfo, error) {
	var lastErr error

	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
		REDACTED
			time.Sleep(backoff)
	REDACTED

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
		REDACTED
			return &GeminiTokenInfo{
				AccessToken:  tokenResp.AccessToken,
				RefreshToken: tokenResp.RefreshToken,
				TokenType:    tokenResp.TokenType,
				ExpiresIn:    tokenResp.ExpiresIn,
				ExpiresAt:    expiresAt,
				Scope:        tokenResp.Scope,
		REDACTED, nil
	REDACTED

		if isNonRetryableGeminiOAuthError(err) {
			return nil, err
	REDACTED
		lastErr = err
REDACTED

	return nil, fmt.Errorf("token refresh failed after retries: %w", lastErr)
REDACTED

func isNonRetryableGeminiOAuthError(err error) bool {
	msg := err.Error()
	nonRetryable := []string{
		"invalid_grant",
		"invalid_client",
		"unauthorized_client",
		"access_denied",
REDACTED
	for _, needle := range nonRetryable {
		if strings.Contains(msg, needle) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *GeminiOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*GeminiTokenInfo, error) {
	if account.Platform != PlatformGemini || account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("account is not a Gemini OAuth account")
REDACTED

	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("no refresh token available")
REDACTED

	// Preserve oauth_type from the account (defaults to code_assist for backward compatibility).
	oauthType := strings.TrimSpace(account.GetCredential("oauth_type"))
	if oauthType == "" {
		oauthType = "code_assist"
REDACTED

	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	tokenInfo, err := s.RefreshToken(ctx, oauthType, refreshToken, proxyURL)
	// Backward compatibility:
	// Older versions could refresh Code Assist tokens using a user-provided OAuth client when configured.
	// If the refresh token was originally issued to that custom client, forcing the built-in client will
	// fail with "unauthorized_client". In that case, retry with the custom client (ai_studio path) when available.
	if err != nil && oauthType == "code_assist" && strings.Contains(err.Error(), "unauthorized_client") && s.GetOAuthConfig().AIStudioOAuthEnabled {
		if alt, altErr := s.RefreshToken(ctx, "ai_studio", refreshToken, proxyURL); altErr == nil {
			tokenInfo = alt
			err = nil
	REDACTED
REDACTED
	if err != nil {
		// Provide a more actionable error for common OAuth client mismatch issues.
		if strings.Contains(err.Error(), "unauthorized_client") {
			return nil, fmt.Errorf("%w (OAuth client mismatch: the refresh_token is bound to the OAuth client used during authorization; please re-authorize this account or restore the original GEMINI_OAUTH_CLIENT_ID/SECRET)", err)
	REDACTED
		return nil, err
REDACTED

	tokenInfo.OAuthType = oauthType

	// Preserve account's project_id when present.
	existingProjectID := strings.TrimSpace(account.GetCredential("project_id"))
	if existingProjectID != "" {
		tokenInfo.ProjectID = existingProjectID
REDACTED

	// 尝试从账号凭证获取 tierID（向后兼容）
	existingTierID := strings.TrimSpace(account.GetCredential("tier_id"))

	// For Code Assist, project_id is required. Auto-detect if missing.
	// For AI Studio OAuth, project_id is optional and should not block refresh.
	switch oauthType {
	case "code_assist":
		// 先设置默认值或保留旧值，确保 tier_id 始终有值
		if existingTierID != "" {
			tokenInfo.TierID = existingTierID
	REDACTED else {
			tokenInfo.TierID = "LEGACY" // 默认值
	REDACTED

		// 尝试自动探测 project_id 和 tier_id
		needDetect := strings.TrimSpace(tokenInfo.ProjectID) == "" || existingTierID == ""
		if needDetect {
			projectID, tierID, err := s.fetchProjectID(ctx, tokenInfo.AccessToken, proxyURL)
			if err != nil {
				fmt.Printf("[GeminiOAuth] Warning: failed to auto-detect project/tier: %v\n", err)
		REDACTED else {
				if strings.TrimSpace(tokenInfo.ProjectID) == "" && projectID != "" {
					tokenInfo.ProjectID = projectID
			REDACTED
				// 只有当原来没有 tier_id 且探测成功时才更新
				if existingTierID == "" && tierID != "" {
					tokenInfo.TierID = tierID
			REDACTED
		REDACTED
	REDACTED

		if strings.TrimSpace(tokenInfo.ProjectID) == "" {
			return nil, fmt.Errorf("failed to auto-detect project_id: empty result")
	REDACTED
	case "google_one":
		// Check if tier cache is stale (> 24 hours)
		needsRefresh := true
		if account.Extra != nil {
			if updatedAtStr, ok := account.Extra["drive_tier_updated_at"].(string); ok {
				if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
					if time.Since(updatedAt) <= 24*time.Hour {
						needsRefresh = false
						// Use cached tier
						if existingTierID != "" {
							tokenInfo.TierID = existingTierID
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED

		if needsRefresh {
			tierID, storageInfo, err := s.FetchGoogleOneTier(ctx, tokenInfo.AccessToken, proxyURL)
			if err == nil && storageInfo != nil {
				tokenInfo.TierID = tierID
				tokenInfo.Extra = map[string]any{
					"drive_storage_limit":   storageInfo.Limit,
					"drive_storage_usage":   storageInfo.Usage,
					"drive_tier_updated_at": time.Now().Format(time.RFC3339),
			REDACTED
		REDACTED else {
				// Fallback to cached or unknown
				if existingTierID != "" {
					tokenInfo.TierID = existingTierID
			REDACTED else {
					tokenInfo.TierID = TierGoogleOneUnknown
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return tokenInfo, nil
REDACTED

func (s *GeminiOAuthService) BuildAccountCredentials(tokenInfo *GeminiTokenInfo) map[string]any {
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   strconv.FormatInt(tokenInfo.ExpiresAt, 10),
REDACTED
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
REDACTED
	if tokenInfo.TokenType != "" {
		creds["token_type"] = tokenInfo.TokenType
REDACTED
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
REDACTED
	if tokenInfo.ProjectID != "" {
		creds["project_id"] = tokenInfo.ProjectID
REDACTED
	if tokenInfo.TierID != "" {
		// Validate tier_id before storing
		if err := validateTierID(tokenInfo.TierID); err == nil {
			creds["tier_id"] = tokenInfo.TierID
	REDACTED
		// Silently skip invalid tier_id (don't block account creation)
REDACTED
	if tokenInfo.OAuthType != "" {
		creds["oauth_type"] = tokenInfo.OAuthType
REDACTED
	// Store extra metadata (Drive info) if present
	if len(tokenInfo.Extra) > 0 {
		for k, v := range tokenInfo.Extra {
			creds[k] = v
	REDACTED
REDACTED
	return creds
REDACTED

func (s *GeminiOAuthService) Stop() {
	s.sessionStore.Stop()
REDACTED

func (s *GeminiOAuthService) fetchProjectID(ctx context.Context, accessToken, proxyURL string) (string, string, error) {
	if s.codeAssist == nil {
		return "", "", errors.New("code assist client not configured")
REDACTED

	loadResp, loadErr := s.codeAssist.LoadCodeAssist(ctx, accessToken, proxyURL, nil)

	// Extract tierID from response (works whether CloudAICompanionProject is set or not)
	tierID := "LEGACY"
	if loadResp != nil {
		tierID = extractTierIDFromAllowedTiers(loadResp.AllowedTiers)
REDACTED

	// If LoadCodeAssist returned a project, use it
	if loadErr == nil && loadResp != nil && strings.TrimSpace(loadResp.CloudAICompanionProject) != "" {
		return strings.TrimSpace(loadResp.CloudAICompanionProject), tierID, nil
REDACTED

	req := &geminicli.OnboardUserRequest{
		TierID: tierID,
		Metadata: geminicli.LoadCodeAssistMetadata{
			IDEType:    "ANTIGRAVITY",
			Platform:   "PLATFORM_UNSPECIFIED",
			PluginType: "GEMINI",
	REDACTED,
REDACTED

	maxAttempts := 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := s.codeAssist.OnboardUser(ctx, accessToken, proxyURL, req)
		if err != nil {
			// If Code Assist onboarding fails (e.g. INVALID_ARGUMENT), fallback to Cloud Resource Manager projects.
			fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
			if fbErr == nil && strings.TrimSpace(fallback) != "" {
				return strings.TrimSpace(fallback), tierID, nil
		REDACTED
			return "", tierID, err
	REDACTED
		if resp.Done {
			if resp.Response != nil && resp.Response.CloudAICompanionProject != nil {
				switch v := resp.Response.CloudAICompanionProject.(type) {
				case string:
					return strings.TrimSpace(v), tierID, nil
				case map[string]any:
					if id, ok := v["id"].(string); ok {
						return strings.TrimSpace(id), tierID, nil
				REDACTED
			REDACTED
		REDACTED

			fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
			if fbErr == nil && strings.TrimSpace(fallback) != "" {
				return strings.TrimSpace(fallback), tierID, nil
		REDACTED
			return "", tierID, errors.New("onboardUser completed but no project_id returned")
	REDACTED
		time.Sleep(2 * time.Second)
REDACTED

	fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
	if fbErr == nil && strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback), tierID, nil
REDACTED
	if loadErr != nil {
		return "", tierID, fmt.Errorf("loadCodeAssist failed (%v) and onboardUser timeout after %d attempts", loadErr, maxAttempts)
REDACTED
	return "", tierID, fmt.Errorf("onboardUser timeout after %d attempts", maxAttempts)
REDACTED

type googleCloudProject struct {
	ProjectID      string `json:"projectId"`
	DisplayName    string `json:"name"`
	LifecycleState string `json:"lifecycleState"`
REDACTED

type googleCloudProjectsResponse struct {
	Projects []googleCloudProject `json:"projects"`
REDACTED

func fetchProjectIDFromResourceManager(ctx context.Context, accessToken, proxyURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cloudresourcemanager.googleapis.com/v1/projects", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create resource manager request: %w", err)
REDACTED

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL: strings.TrimSpace(proxyURL),
		Timeout:  30 * time.Second,
REDACTED)
	if err != nil {
		client = &http.Client{Timeout: 30 * time.SecondREDACTED
REDACTED

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resource manager request failed: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read resource manager response: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resource manager HTTP %d: %s", resp.StatusCode, string(bodyBytes))
REDACTED

	var projectsResp googleCloudProjectsResponse
	if err := json.Unmarshal(bodyBytes, &projectsResp); err != nil {
		return "", fmt.Errorf("failed to parse resource manager response: %w", err)
REDACTED

	active := make([]googleCloudProject, 0, len(projectsResp.Projects))
	for _, p := range projectsResp.Projects {
		if p.LifecycleState == "ACTIVE" && strings.TrimSpace(p.ProjectID) != "" {
			active = append(active, p)
	REDACTED
REDACTED
	if len(active) == 0 {
		return "", errors.New("no ACTIVE projects found from resource manager")
REDACTED

	// Prefer likely companion projects first.
	for _, p := range active {
		id := strings.ToLower(strings.TrimSpace(p.ProjectID))
		name := strings.ToLower(strings.TrimSpace(p.DisplayName))
		if strings.Contains(id, "cloud-ai-companion") || strings.Contains(name, "cloud ai companion") || strings.Contains(name, "code assist") {
			return strings.TrimSpace(p.ProjectID), nil
	REDACTED
REDACTED
	// Then prefer "default".
	for _, p := range active {
		id := strings.ToLower(strings.TrimSpace(p.ProjectID))
		name := strings.ToLower(strings.TrimSpace(p.DisplayName))
		if strings.Contains(id, "default") || strings.Contains(name, "default") {
			return strings.TrimSpace(p.ProjectID), nil
	REDACTED
REDACTED

	return strings.TrimSpace(active[0].ProjectID), nil
REDACTED
