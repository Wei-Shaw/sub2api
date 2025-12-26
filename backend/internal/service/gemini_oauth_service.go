package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

type GeminiOAuthService struct {
	sessionStore *geminicli.SessionStore
	proxyRepo    ports.ProxyRepository
	oauthClient  ports.GeminiOAuthClient
	codeAssist   ports.GeminiCliCodeAssistClient
	cfg          *config.Config
REDACTED

func NewGeminiOAuthService(
	proxyRepo ports.ProxyRepository,
	oauthClient ports.GeminiOAuthClient,
	codeAssist ports.GeminiCliCodeAssistClient,
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

	// 两种 OAuth 模式都使用相同的配置，只是 scopes 不同
	// scopes 会在 EffectiveOAuthConfig 中根据 oauthType 自动选择
	oauthCfg := geminicli.OAuthConfig{
		ClientID:     s.cfg.Gemini.OAuth.ClientID,
		ClientSecret: s.cfg.Gemini.OAuth.ClientSecret,
		Scopes:       s.cfg.Gemini.OAuth.Scopes,
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

	// For Code Assist with Gemini CLI credentials, use the CLI's redirect URI
	if oauthType == "code_assist" {
		redirectURI = geminicli.GeminiCLIRedirectURI
		session.RedirectURI = redirectURI
		s.sessionStore.Set(sessionID, session)
REDACTED

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
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	OAuthType    string `json:"oauth_type,omitempty"` // "code_assist" 或 "ai_studio"
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

	tokenResp, err := s.oauthClient.ExchangeCode(ctx, input.Code, session.CodeVerifier, redirectURI, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
REDACTED
	sessionProjectID := strings.TrimSpace(session.ProjectID)
	oauthType := session.OAuthType
	if oauthType == "" {
		oauthType = "code_assist" // 默认为 code_assist 以兼容旧 session
REDACTED
	s.sessionStore.Delete(input.SessionID)

	// 计算过期时间时减去 5 分钟安全时间窗口,考虑网络延迟和时钟偏差
	expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - 300

	projectID := sessionProjectID

	// 对于 code_assist 模式，project_id 是必需的
	// 对于 ai_studio 模式，project_id 是可选的（不影响使用 AI Studio API）
	if oauthType == "code_assist" {
		if projectID == "" {
			var err error
			projectID, err = s.fetchProjectID(ctx, tokenResp.AccessToken, proxyURL)
			if err != nil {
				// 记录警告但不阻断流程，允许后续补充 project_id
				fmt.Printf("[GeminiOAuth] Warning: Failed to fetch project_id during token exchange: %v\n", err)
		REDACTED
	REDACTED
		if strings.TrimSpace(projectID) == "" {
			return nil, fmt.Errorf("missing project_id for Code Assist OAuth: please fill Project ID (optional field) and regenerate the auth URL, or ensure your Google account has an ACTIVE GCP project")
	REDACTED
REDACTED

	return &GeminiTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
		ProjectID:    projectID,
		OAuthType:    oauthType,
REDACTED, nil
REDACTED

func (s *GeminiOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*GeminiTokenInfo, error) {
	var lastErr error

	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
		REDACTED
			time.Sleep(backoff)
	REDACTED

		tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL)
		if err == nil {
			// 计算过期时间时减去 5 分钟安全时间窗口,考虑网络延迟和时钟偏差
			expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - 300
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

func (s *GeminiOAuthService) RefreshAccountToken(ctx context.Context, account *model.Account) (*GeminiTokenInfo, error) {
	if account.Platform != model.PlatformGemini || account.Type != model.AccountTypeOAuth {
		return nil, fmt.Errorf("account is not a Gemini OAuth account")
REDACTED

	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("no refresh token available")
REDACTED

	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	tokenInfo, err := s.RefreshToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
REDACTED

	// Preserve oauth_type from the account (defaults to code_assist for backward compatibility).
	oauthType := strings.TrimSpace(account.GetCredential("oauth_type"))
	if oauthType == "" {
		oauthType = "code_assist"
REDACTED
	tokenInfo.OAuthType = oauthType

	// Preserve account's project_id when present.
	existingProjectID := strings.TrimSpace(account.GetCredential("project_id"))
	if existingProjectID != "" {
		tokenInfo.ProjectID = existingProjectID
REDACTED

	// For Code Assist, project_id is required. Auto-detect if missing.
	// For AI Studio OAuth, project_id is optional and should not block refresh.
	if oauthType == "code_assist" && strings.TrimSpace(tokenInfo.ProjectID) == "" {
		projectID, err := s.fetchProjectID(ctx, tokenInfo.AccessToken, proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to auto-detect project_id: %w", err)
	REDACTED
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			return nil, fmt.Errorf("failed to auto-detect project_id: empty result")
	REDACTED
		tokenInfo.ProjectID = projectID
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
	if tokenInfo.OAuthType != "" {
		creds["oauth_type"] = tokenInfo.OAuthType
REDACTED
	return creds
REDACTED

func (s *GeminiOAuthService) Stop() {
	s.sessionStore.Stop()
REDACTED

func (s *GeminiOAuthService) fetchProjectID(ctx context.Context, accessToken, proxyURL string) (string, error) {
	if s.codeAssist == nil {
		return "", errors.New("code assist client not configured")
REDACTED

	loadResp, loadErr := s.codeAssist.LoadCodeAssist(ctx, accessToken, proxyURL, nil)
	if loadErr == nil && loadResp != nil && strings.TrimSpace(loadResp.CloudAICompanionProject) != "" {
		return strings.TrimSpace(loadResp.CloudAICompanionProject), nil
REDACTED

	// Pick tier from allowedTiers; if no default tier is marked, pick the first non-empty tier ID.
	tierID := "LEGACY"
	if loadResp != nil {
		for _, tier := range loadResp.AllowedTiers {
			if tier.IsDefault && strings.TrimSpace(tier.ID) != "" {
				tierID = strings.TrimSpace(tier.ID)
				break
		REDACTED
	REDACTED
		if strings.TrimSpace(tierID) == "" || tierID == "LEGACY" {
			for _, tier := range loadResp.AllowedTiers {
				if strings.TrimSpace(tier.ID) != "" {
					tierID = strings.TrimSpace(tier.ID)
					break
			REDACTED
		REDACTED
	REDACTED
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
				return strings.TrimSpace(fallback), nil
		REDACTED
			return "", err
	REDACTED
		if resp.Done {
			if resp.Response != nil && resp.Response.CloudAICompanionProject != nil {
				switch v := resp.Response.CloudAICompanionProject.(type) {
				case string:
					return strings.TrimSpace(v), nil
				case map[string]any:
					if id, ok := v["id"].(string); ok {
						return strings.TrimSpace(id), nil
				REDACTED
			REDACTED
		REDACTED

			fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
			if fbErr == nil && strings.TrimSpace(fallback) != "" {
				return strings.TrimSpace(fallback), nil
		REDACTED
			return "", errors.New("onboardUser completed but no project_id returned")
	REDACTED
		time.Sleep(2 * time.Second)
REDACTED

	fallback, fbErr := fetchProjectIDFromResourceManager(ctx, accessToken, proxyURL)
	if fbErr == nil && strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback), nil
REDACTED
	if loadErr != nil {
		return "", fmt.Errorf("loadCodeAssist failed (%v) and onboardUser timeout after %d attempts", loadErr, maxAttempts)
REDACTED
	return "", fmt.Errorf("onboardUser timeout after %d attempts", maxAttempts)
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

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	if strings.TrimSpace(proxyURL) != "" {
		if proxyURLParsed, err := url.Parse(strings.TrimSpace(proxyURL)); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURLParsed)REDACTED
	REDACTED
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
