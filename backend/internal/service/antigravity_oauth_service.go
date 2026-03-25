package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

type AntigravityOAuthService struct {
	sessionStore *antigravity.SessionStore
	proxyRepo    ProxyRepository
REDACTED

func NewAntigravityOAuthService(proxyRepo ProxyRepository) *AntigravityOAuthService {
	return &AntigravityOAuthService{
		sessionStore: antigravity.NewSessionStore(),
		proxyRepo:    proxyRepo,
REDACTED
REDACTED

// AntigravityAuthURLResult is the result of generating an authorization URL
type AntigravityAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
REDACTED

// GenerateAuthURL 生成 Google OAuth 授权链接
func (s *AntigravityOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*AntigravityAuthURLResult, error) {
	state, err := antigravity.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("生成 state 失败: %w", err)
REDACTED

	codeVerifier, err := antigravity.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("生成 code_verifier 失败: %w", err)
REDACTED

	sessionID, err := antigravity.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("生成 session_id 失败: %w", err)
REDACTED

	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	session := &antigravity.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
REDACTED
	s.sessionStore.Set(sessionID, session)

	codeChallenge := antigravity.GenerateCodeChallenge(codeVerifier)
	authURL := antigravity.BuildAuthorizationURL(state, codeChallenge)

	return &AntigravityAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
		State:     state,
REDACTED, nil
REDACTED

// AntigravityExchangeCodeInput 交换 code 的输入
type AntigravityExchangeCodeInput struct {
	SessionID string
	State     string
	Code      string
	ProxyID   *int64
REDACTED

// AntigravityTokenInfo token 信息
type AntigravityTokenInfo struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	ExpiresAt        int64  `json:"expires_at"`
	TokenType        string `json:"token_type"`
	Email            string `json:"email,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	ProjectIDMissing bool   `json:"-"`
	PlanType         string `json:"-"`
REDACTED

// ExchangeCode 用 authorization code 交换 token
func (s *AntigravityOAuthService) ExchangeCode(ctx context.Context, input *AntigravityExchangeCodeInput) (*AntigravityTokenInfo, error) {
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, fmt.Errorf("session 不存在或已过期")
REDACTED

	if strings.TrimSpace(input.State) == "" || input.State != session.State {
		return nil, fmt.Errorf("state 无效")
REDACTED

	// 确定代理 URL
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create antigravity client failed: %w", err)
REDACTED

	// 交换 token
	tokenResp, err := client.ExchangeCode(ctx, input.Code, session.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("token 交换失败: %w", err)
REDACTED

	// 删除 session
	s.sessionStore.Delete(input.SessionID)

	// 计算过期时间（减去 5 分钟安全窗口）
	expiresAt := time.Now().Unix() + tokenResp.ExpiresIn - 300

	result := &AntigravityTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    expiresAt,
		TokenType:    tokenResp.TokenType,
REDACTED

	// 获取用户信息
	userInfo, err := client.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		fmt.Printf("[AntigravityOAuth] 警告: 获取用户信息失败: %v\n", err)
REDACTED else {
		result.Email = userInfo.Email
REDACTED

	// 获取 project_id + plan_type（部分账户类型可能没有），失败时重试
	loadResult, loadErr := s.loadProjectIDWithRetry(ctx, tokenResp.AccessToken, proxyURL, 3)
	if loadErr != nil {
		fmt.Printf("[AntigravityOAuth] 警告: 获取 project_id 失败（重试后）: %v\n", loadErr)
		result.ProjectIDMissing = true
REDACTED
	if loadResult != nil {
		result.ProjectID = loadResult.ProjectID
		if loadResult.Subscription != nil {
			result.PlanType = loadResult.Subscription.PlanType
	REDACTED
REDACTED

	return result, nil
REDACTED

// RefreshToken 刷新 token
func (s *AntigravityOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*AntigravityTokenInfo, error) {
	var lastErr error

	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
		REDACTED
			time.Sleep(backoff)
	REDACTED

		client, err := antigravity.NewClient(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("create antigravity client failed: %w", err)
	REDACTED
		tokenResp, err := client.RefreshToken(ctx, refreshToken)
		if err == nil {
			now := time.Now()
			expiresAt := now.Unix() + tokenResp.ExpiresIn - 300
			fmt.Printf("[AntigravityOAuth] Token refreshed: expires_in=%d, expires_at=%d (%s)\n",
				tokenResp.ExpiresIn, expiresAt, time.Unix(expiresAt, 0).Format("2006-01-02 15:04:05"))
			return &AntigravityTokenInfo{
				AccessToken:  tokenResp.AccessToken,
				RefreshToken: tokenResp.RefreshToken,
				ExpiresIn:    tokenResp.ExpiresIn,
				ExpiresAt:    expiresAt,
				TokenType:    tokenResp.TokenType,
		REDACTED, nil
	REDACTED

		if isNonRetryableAntigravityOAuthError(err) {
			return nil, err
	REDACTED
		// 代理连接错误（TCP 超时、连接拒绝、DNS 失败）不重试，立即返回
		if antigravity.IsConnectionError(err) {
			return nil, fmt.Errorf("proxy unavailable: %w", err)
	REDACTED
		lastErr = err
REDACTED

	return nil, fmt.Errorf("token 刷新失败 (重试后): %w", lastErr)
REDACTED

// ValidateRefreshToken 用 refresh token 验证并获取完整的 token 信息（含 email 和 project_id）
func (s *AntigravityOAuthService) ValidateRefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*AntigravityTokenInfo, error) {
	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// 刷新 token
	tokenInfo, err := s.RefreshToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
REDACTED

	// 获取用户信息（email）
	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create antigravity client failed: %w", err)
REDACTED
	userInfo, err := client.GetUserInfo(ctx, tokenInfo.AccessToken)
	if err != nil {
		fmt.Printf("[AntigravityOAuth] 警告: 获取用户信息失败: %v\n", err)
REDACTED else {
		tokenInfo.Email = userInfo.Email
REDACTED

	// 获取 project_id + plan_type（容错，失败不阻塞）
	loadResult, loadErr := s.loadProjectIDWithRetry(ctx, tokenInfo.AccessToken, proxyURL, 3)
	if loadErr != nil {
		fmt.Printf("[AntigravityOAuth] 警告: 获取 project_id 失败（重试后）: %v\n", loadErr)
		tokenInfo.ProjectIDMissing = true
REDACTED
	if loadResult != nil {
		tokenInfo.ProjectID = loadResult.ProjectID
		if loadResult.Subscription != nil {
			tokenInfo.PlanType = loadResult.Subscription.PlanType
	REDACTED
REDACTED

	return tokenInfo, nil
REDACTED

func isNonRetryableAntigravityOAuthError(err error) bool {
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

// RefreshAccountToken 刷新账户的 token
func (s *AntigravityOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*AntigravityTokenInfo, error) {
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("非 Antigravity OAuth 账户")
REDACTED

	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("无可用的 refresh_token")
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

	// 保留原有的 email
	existingEmail := strings.TrimSpace(account.GetCredential("email"))
	if existingEmail != "" {
		tokenInfo.Email = existingEmail
REDACTED

	// 每次刷新都调用 LoadCodeAssist 获取 project_id + plan_type，失败时重试
	existingProjectID := strings.TrimSpace(account.GetCredential("project_id"))
	loadResult, loadErr := s.loadProjectIDWithRetry(ctx, tokenInfo.AccessToken, proxyURL, 3)

	if loadErr != nil {
		tokenInfo.ProjectID = existingProjectID
		if existingProjectID == "" {
			tokenInfo.ProjectIDMissing = true
	REDACTED
REDACTED
	if loadResult != nil {
		if loadResult.ProjectID != "" {
			tokenInfo.ProjectID = loadResult.ProjectID
	REDACTED
		if loadResult.Subscription != nil {
			tokenInfo.PlanType = loadResult.Subscription.PlanType
	REDACTED
REDACTED

	return tokenInfo, nil
REDACTED

// loadCodeAssistResult 封装 loadProjectIDWithRetry 的返回结果，
// 同时携带从 LoadCodeAssist 响应中提取的 plan_type 信息。
type loadCodeAssistResult struct {
	ProjectID string
	Subscription *AntigravitySubscriptionResult
REDACTED

// loadProjectIDWithRetry 带重试机制获取 project_id，同时从响应中提取 plan_type。
func (s *AntigravityOAuthService) loadProjectIDWithRetry(ctx context.Context, accessToken, proxyURL string, maxRetries int) (*loadCodeAssistResult, error) {
	var lastErr error
	var lastSubscription *AntigravitySubscriptionResult

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 8*time.Second {
				backoff = 8 * time.Second
		REDACTED
			time.Sleep(backoff)
	REDACTED

		client, err := antigravity.NewClient(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("create antigravity client failed: %w", err)
	REDACTED
		loadResp, loadRaw, err := client.LoadCodeAssist(ctx, accessToken)

		if loadResp != nil {
			sub := NormalizeAntigravitySubscription(loadResp)
			lastSubscription = &sub
	REDACTED

		if err == nil && loadResp != nil && loadResp.CloudAICompanionProject != "" {
			return &loadCodeAssistResult{
				ProjectID:    loadResp.CloudAICompanionProject,
				Subscription: lastSubscription,
		REDACTED, nil
	REDACTED

		if err == nil {
			if projectID, onboardErr := tryOnboardProjectID(ctx, client, accessToken, loadRaw); onboardErr == nil && projectID != "" {
				return &loadCodeAssistResult{
					ProjectID:    projectID,
					Subscription: lastSubscription,
			REDACTED, nil
		REDACTED else if onboardErr != nil {
				lastErr = onboardErr
				continue
		REDACTED
	REDACTED

		if err != nil {
			lastErr = err
	REDACTED else if loadResp == nil {
			lastErr = fmt.Errorf("LoadCodeAssist 返回空响应")
	REDACTED else {
			lastErr = fmt.Errorf("LoadCodeAssist 返回空 project_id")
	REDACTED
REDACTED

	if lastSubscription != nil {
		return &loadCodeAssistResult{Subscription: lastSubscriptionREDACTED, fmt.Errorf("获取 project_id 失败 (重试 %d 次后): %w", maxRetries, lastErr)
REDACTED
	return nil, fmt.Errorf("获取 project_id 失败 (重试 %d 次后): %w", maxRetries, lastErr)
REDACTED

func tryOnboardProjectID(ctx context.Context, client *antigravity.Client, accessToken string, loadRaw map[string]any) (string, error) {
	tierID := resolveDefaultTierID(loadRaw)
	if tierID == "" {
		return "", fmt.Errorf("loadCodeAssist 未返回可用的默认 tier")
REDACTED

	projectID, err := client.OnboardUser(ctx, accessToken, tierID)
	if err != nil {
		return "", fmt.Errorf("onboardUser 失败 (tier=%s): %w", tierID, err)
REDACTED
	return projectID, nil
REDACTED

func resolveDefaultTierID(loadRaw map[string]any) string {
	if len(loadRaw) == 0 {
		return ""
REDACTED

	rawTiers, ok := loadRaw["allowedTiers"]
	if !ok {
		return ""
REDACTED

	tiers, ok := rawTiers.([]any)
	if !ok {
		return ""
REDACTED

	for _, rawTier := range tiers {
		tier, ok := rawTier.(map[string]any)
		if !ok {
			continue
	REDACTED
		if isDefault, _ := tier["isDefault"].(bool); !isDefault {
			continue
	REDACTED
		if id, ok := tier["id"].(string); ok {
			id = strings.TrimSpace(id)
			if id != "" {
				return id
		REDACTED
	REDACTED
REDACTED

	return ""
REDACTED

// FillProjectID 仅获取 project_id，不刷新 OAuth token
func (s *AntigravityOAuthService) FillProjectID(ctx context.Context, account *Account, accessToken string) (string, error) {
	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED
	result, err := s.loadProjectIDWithRetry(ctx, accessToken, proxyURL, 3)
	if result != nil {
		return result.ProjectID, err
REDACTED
	return "", err
REDACTED

// BuildAccountCredentials 构建账户凭证
func (s *AntigravityOAuthService) BuildAccountCredentials(tokenInfo *AntigravityTokenInfo) map[string]any {
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
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
REDACTED
	if tokenInfo.ProjectID != "" {
		creds["project_id"] = tokenInfo.ProjectID
REDACTED
	if tokenInfo.PlanType != "" {
		creds["plan_type"] = tokenInfo.PlanType
REDACTED
	return creds
REDACTED

// Stop 停止服务
func (s *AntigravityOAuthService) Stop() {
	s.sessionStore.Stop()
REDACTED
