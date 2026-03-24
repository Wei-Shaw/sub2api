package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

var openAISoraSessionAuthURL = "https://sora.chatgpt.com/api/auth/session"

var soraSessionCookiePattern = regexp.MustCompile(`(?i)(?:^|[\n\r;])\s*(?:(?:set-cookie|cookie)\s*:\s*)?__Secure-(?:next-auth|authjs)\.session-token(?:\.(\d+))?=([^;\r\n]+)`)

type soraSessionChunk struct {
	index int
	value string
REDACTED

// OpenAIOAuthService handles OpenAI OAuth authentication flows
type OpenAIOAuthService struct {
	sessionStore         *openai.SessionStore
	proxyRepo            ProxyRepository
	oauthClient          OpenAIOAuthClient
	privacyClientFactory PrivacyClientFactory // 用于调用 chatgpt.com/backend-api（ImpersonateChrome）
REDACTED

// NewOpenAIOAuthService creates a new OpenAI OAuth service
func NewOpenAIOAuthService(proxyRepo ProxyRepository, oauthClient OpenAIOAuthClient) *OpenAIOAuthService {
	return &OpenAIOAuthService{
		sessionStore: openai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
REDACTED
REDACTED

// SetPrivacyClientFactory 注入 ImpersonateChrome 客户端工厂，
// 用于调用 chatgpt.com/backend-api 获取账号信息（plan_type 等）。
func (s *OpenAIOAuthService) SetPrivacyClientFactory(factory PrivacyClientFactory) {
	s.privacyClientFactory = factory
REDACTED

// OpenAIAuthURLResult contains the authorization URL and session info
type OpenAIAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
REDACTED

// GenerateAuthURL generates an OpenAI OAuth authorization URL
func (s *OpenAIOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI, platform string) (*OpenAIAuthURLResult, error) {
	// Generate PKCE values
	state, err := openai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
REDACTED

	codeVerifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
REDACTED

	codeChallenge := openai.GenerateCodeChallenge(codeVerifier)

	// Generate session ID
	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
REDACTED

	// Get proxy URL if specified
	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	REDACTED
		if proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Use default redirect URI if not specified
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
REDACTED
	normalizedPlatform := normalizeOpenAIOAuthPlatform(platform)
	clientID, _ := openai.OAuthClientConfigByPlatform(normalizedPlatform)

	// Store session
	session := &openai.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
REDACTED
	s.sessionStore.Set(sessionID, session)

	// Build authorization URL
	authURL := openai.BuildAuthorizationURLForPlatform(state, codeChallenge, redirectURI, normalizedPlatform)

	return &OpenAIAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
REDACTED, nil
REDACTED

// OpenAIExchangeCodeInput represents the input for code exchange
type OpenAIExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
REDACTED

// OpenAITokenInfo represents the token information for OpenAI
type OpenAITokenInfo struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token,omitempty"`
	ExpiresIn        int64  `json:"expires_in"`
	ExpiresAt        int64  `json:"expires_at"`
	ClientID         string `json:"client_id,omitempty"`
	Email            string `json:"email,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	ChatGPTUserID    string `json:"chatgpt_user_id,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	PlanType         string `json:"plan_type,omitempty"`
	PrivacyMode      string `json:"privacy_mode,omitempty"`
REDACTED

// ExchangeCode exchanges authorization code for tokens
func (s *OpenAIOAuthService) ExchangeCode(ctx context.Context, input *OpenAIExchangeCodeInput) (*OpenAITokenInfo, error) {
	// Get session
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
REDACTED
	if input.State == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_STATE_REQUIRED", "oauth state is required")
REDACTED
	if subtle.ConstantTimeCompare([]byte(input.State), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_STATE", "invalid oauth state")
REDACTED

	// Get proxy URL: prefer input.ProxyID, fallback to session.ProxyURL
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	REDACTED
		if proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Use redirect URI from session or input
	redirectURI := session.RedirectURI
	if input.RedirectURI != "" {
		redirectURI = input.RedirectURI
REDACTED
	clientID := strings.TrimSpace(session.ClientID)
	if clientID == "" {
		clientID = openai.ClientID
REDACTED

	// Exchange code for token
	tokenResp, err := s.oauthClient.ExchangeCode(ctx, input.Code, session.CodeVerifier, redirectURI, proxyURL, clientID)
	if err != nil {
		return nil, err
REDACTED

	// Parse ID token to get user info
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, parseErr := openai.ParseIDToken(tokenResp.IDToken)
		if parseErr != nil {
			slog.Warn("openai_oauth_id_token_parse_failed", "error", parseErr)
	REDACTED else {
			userInfo = claims.GetUserInfo()
	REDACTED
REDACTED

	// Delete session after successful exchange
	s.sessionStore.Delete(input.SessionID)

	tokenInfo := &OpenAITokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresIn:    int64(tokenResp.ExpiresIn),
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
		ClientID:     clientID,
REDACTED

	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
		tokenInfo.PlanType = userInfo.PlanType
REDACTED

	return tokenInfo, nil
REDACTED

// RefreshToken refreshes an OpenAI OAuth token
func (s *OpenAIOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*OpenAITokenInfo, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
REDACTED

// RefreshTokenWithClientID refreshes an OpenAI/Sora OAuth token with optional client_id.
func (s *OpenAIOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken string, proxyURL string, clientID string) (*OpenAITokenInfo, error) {
	tokenResp, err := s.oauthClient.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
REDACTED

	// Parse ID token to get user info
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, parseErr := openai.ParseIDToken(tokenResp.IDToken)
		if parseErr != nil {
			slog.Warn("openai_oauth_id_token_parse_failed", "error", parseErr)
	REDACTED else {
			userInfo = claims.GetUserInfo()
	REDACTED
REDACTED

	tokenInfo := &OpenAITokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresIn:    int64(tokenResp.ExpiresIn),
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
REDACTED
	if trimmed := strings.TrimSpace(clientID); trimmed != "" {
		tokenInfo.ClientID = trimmed
REDACTED

	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
		tokenInfo.PlanType = userInfo.PlanType
REDACTED

	// id_token 中缺少 plan_type 时（如 Mobile RT），尝试通过 ChatGPT backend-api 补全
	if tokenInfo.PlanType == "" && tokenInfo.AccessToken != "" && s.privacyClientFactory != nil {
		// 从 access_token JWT 中提取 orgID（poid），用于匹配正确的账号
		orgID := tokenInfo.OrganizationID
		if orgID == "" {
			if atClaims, err := openai.DecodeIDToken(tokenInfo.AccessToken); err == nil && atClaims.OpenAIAuth != nil {
				orgID = atClaims.OpenAIAuth.POID
		REDACTED
	REDACTED
		if info := fetchChatGPTAccountInfo(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL, orgID); info != nil {
			if tokenInfo.PlanType == "" && info.PlanType != "" {
				tokenInfo.PlanType = info.PlanType
		REDACTED
			if tokenInfo.Email == "" && info.Email != "" {
				tokenInfo.Email = info.Email
		REDACTED
	REDACTED
REDACTED

	// 尝试设置隐私（关闭训练数据共享），best-effort
	if tokenInfo.AccessToken != "" && s.privacyClientFactory != nil {
		tokenInfo.PrivacyMode = disableOpenAITraining(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL)
REDACTED

	return tokenInfo, nil
REDACTED

// ExchangeSoraSessionToken exchanges Sora session_token to access_token.
func (s *OpenAIOAuthService) ExchangeSoraSessionToken(ctx context.Context, sessionToken string, proxyID *int64) (*OpenAITokenInfo, error) {
	sessionToken = normalizeSoraSessionTokenInput(sessionToken)
	if strings.TrimSpace(sessionToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "SORA_SESSION_TOKEN_REQUIRED", "session_token is required")
REDACTED

	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAISoraSessionAuthURL, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "SORA_SESSION_REQUEST_BUILD_FAILED", "failed to build request: %v", err)
REDACTED
	req.Header.Set("Cookie", "__Secure-next-auth.session-token="+strings.TrimSpace(sessionToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")
	req.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
REDACTED)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_CLIENT_FAILED", "create http client failed: %v", err)
REDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_REQUEST_FAILED", "request failed: %v", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_EXCHANGE_FAILED", "status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
REDACTED

	var sessionResp struct {
		AccessToken string `json:"accessToken"`
		Expires     string `json:"expires"`
		User        struct {
			Email string `json:"email"`
			Name  string `json:"name"`
	REDACTED `json:"user"`
REDACTED
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_PARSE_FAILED", "failed to parse response: %v", err)
REDACTED
	if strings.TrimSpace(sessionResp.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "SORA_SESSION_ACCESS_TOKEN_MISSING", "session exchange response missing access token")
REDACTED

	expiresAt := time.Now().Add(time.Hour).Unix()
	if strings.TrimSpace(sessionResp.Expires) != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, sessionResp.Expires); parseErr == nil {
			expiresAt = parsed.Unix()
	REDACTED
REDACTED
	expiresIn := expiresAt - time.Now().Unix()
	if expiresIn < 0 {
		expiresIn = 0
REDACTED

	return &OpenAITokenInfo{
		AccessToken: strings.TrimSpace(sessionResp.AccessToken),
		ExpiresIn:   expiresIn,
		ExpiresAt:   expiresAt,
		ClientID:    openai.SoraClientID,
		Email:       strings.TrimSpace(sessionResp.User.Email),
REDACTED, nil
REDACTED

func normalizeSoraSessionTokenInput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
REDACTED

	matches := soraSessionCookiePattern.FindAllStringSubmatch(trimmed, -1)
	if len(matches) == 0 {
		return sanitizeSessionToken(trimmed)
REDACTED

	chunkMatches := make([]soraSessionChunk, 0, len(matches))
	singleValues := make([]string, 0, len(matches))

	for _, match := range matches {
		if len(match) < 3 {
			continue
	REDACTED

		value := sanitizeSessionToken(match[2])
		if value == "" {
			continue
	REDACTED

		if strings.TrimSpace(match[1]) == "" {
			singleValues = append(singleValues, value)
			continue
	REDACTED

		idx, err := strconv.Atoi(strings.TrimSpace(match[1]))
		if err != nil || idx < 0 {
			continue
	REDACTED
		chunkMatches = append(chunkMatches, soraSessionChunk{
			index: idx,
			value: value,
	REDACTED)
REDACTED

	if merged := mergeLatestSoraSessionChunks(chunkMatches); merged != "" {
		return merged
REDACTED

	if len(singleValues) > 0 {
		return singleValues[len(singleValues)-1]
REDACTED

	return ""
REDACTED

func mergeSoraSessionChunkSegment(chunks []soraSessionChunk, requiredMaxIndex int, requireComplete bool) string {
	if len(chunks) == 0 {
		return ""
REDACTED

	byIndex := make(map[int]string, len(chunks))
	for _, chunk := range chunks {
		byIndex[chunk.index] = chunk.value
REDACTED

	if _, ok := byIndex[0]; !ok {
		return ""
REDACTED
	if requireComplete {
		for idx := 0; idx <= requiredMaxIndex; idx++ {
			if _, ok := byIndex[idx]; !ok {
				return ""
		REDACTED
	REDACTED
REDACTED

	orderedIndexes := make([]int, 0, len(byIndex))
	for idx := range byIndex {
		orderedIndexes = append(orderedIndexes, idx)
REDACTED
	sort.Ints(orderedIndexes)

	var builder strings.Builder
	for _, idx := range orderedIndexes {
		if _, err := builder.WriteString(byIndex[idx]); err != nil {
			return ""
	REDACTED
REDACTED
	return sanitizeSessionToken(builder.String())
REDACTED

func mergeLatestSoraSessionChunks(chunks []soraSessionChunk) string {
	if len(chunks) == 0 {
		return ""
REDACTED

	requiredMaxIndex := 0
	for _, chunk := range chunks {
		if chunk.index > requiredMaxIndex {
			requiredMaxIndex = chunk.index
	REDACTED
REDACTED

	groupStarts := make([]int, 0, len(chunks))
	for idx, chunk := range chunks {
		if chunk.index == 0 {
			groupStarts = append(groupStarts, idx)
	REDACTED
REDACTED

	if len(groupStarts) == 0 {
		return mergeSoraSessionChunkSegment(chunks, requiredMaxIndex, false)
REDACTED

	for i := len(groupStarts) - 1; i >= 0; i-- {
		start := groupStarts[i]
		end := len(chunks)
		if i+1 < len(groupStarts) {
			end = groupStarts[i+1]
	REDACTED
		if merged := mergeSoraSessionChunkSegment(chunks[start:end], requiredMaxIndex, true); merged != "" {
			return merged
	REDACTED
REDACTED

	return mergeSoraSessionChunkSegment(chunks, requiredMaxIndex, false)
REDACTED

func sanitizeSessionToken(raw string) string {
	token := strings.TrimSpace(raw)
	token = strings.Trim(token, "\"'`")
	token = strings.TrimSuffix(token, ";")
	return strings.TrimSpace(token)
REDACTED

// RefreshAccountToken refreshes token for an OpenAI/Sora OAuth account
func (s *OpenAIOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*OpenAITokenInfo, error) {
	if account.Platform != PlatformOpenAI && account.Platform != PlatformSora {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT", "account is not an OpenAI/Sora account")
REDACTED
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
REDACTED

	refreshToken := account.GetCredential("refresh_token")
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
REDACTED

	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	clientID := account.GetCredential("client_id")
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
REDACTED

// BuildAccountCredentials builds credentials map from token info
func (s *OpenAIOAuthService) BuildAccountCredentials(tokenInfo *OpenAITokenInfo) map[string]any {
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)

	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   expiresAt,
REDACTED
	// 仅在刷新响应返回了新的 refresh_token 时才更新，防止用空值覆盖已有令牌
	if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
REDACTED

	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
REDACTED
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
REDACTED
	if tokenInfo.ChatGPTAccountID != "" {
		creds["chatgpt_account_id"] = tokenInfo.ChatGPTAccountID
REDACTED
	if tokenInfo.ChatGPTUserID != "" {
		creds["chatgpt_user_id"] = tokenInfo.ChatGPTUserID
REDACTED
	if tokenInfo.OrganizationID != "" {
		creds["organization_id"] = tokenInfo.OrganizationID
REDACTED
	if tokenInfo.PlanType != "" {
		creds["plan_type"] = tokenInfo.PlanType
REDACTED
	if strings.TrimSpace(tokenInfo.ClientID) != "" {
		creds["client_id"] = strings.TrimSpace(tokenInfo.ClientID)
REDACTED

	return creds
REDACTED

// Stop stops the session store cleanup goroutine
func (s *OpenAIOAuthService) Stop() {
	s.sessionStore.Stop()
REDACTED

func (s *OpenAIOAuthService) resolveProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
REDACTED
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
REDACTED
	if proxy == nil {
		return "", nil
REDACTED
	return proxy.URL(), nil
REDACTED

func normalizeOpenAIOAuthPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformSora:
		return openai.OAuthPlatformSora
	default:
		return openai.OAuthPlatformOpenAI
REDACTED
REDACTED
