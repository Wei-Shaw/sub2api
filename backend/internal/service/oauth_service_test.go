//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// --- mock: ClaudeOAuthClient ---

type mockClaudeOAuthClient struct {
	getOrgUUIDFunc   func(ctx context.Context, sessionKey, proxyURL string) (string, error)
	getAuthCodeFunc  func(ctx context.Context, sessionKey, orgUUID, scope, codeChallenge, state, proxyURL string) (string, error)
	exchangeCodeFunc func(ctx context.Context, code, codeVerifier, state, proxyURL string, isSetupToken bool) (*oauth.TokenResponse, error)
	refreshTokenFunc func(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error)
REDACTED

func (m *mockClaudeOAuthClient) GetOrganizationUUID(ctx context.Context, sessionKey, proxyURL string) (string, error) {
	if m.getOrgUUIDFunc != nil {
		return m.getOrgUUIDFunc(ctx, sessionKey, proxyURL)
REDACTED
	panic("GetOrganizationUUID not implemented")
REDACTED

func (m *mockClaudeOAuthClient) GetAuthorizationCode(ctx context.Context, sessionKey, orgUUID, scope, codeChallenge, state, proxyURL string) (string, error) {
	if m.getAuthCodeFunc != nil {
		return m.getAuthCodeFunc(ctx, sessionKey, orgUUID, scope, codeChallenge, state, proxyURL)
REDACTED
	panic("GetAuthorizationCode not implemented")
REDACTED

func (m *mockClaudeOAuthClient) ExchangeCodeForToken(ctx context.Context, code, codeVerifier, state, proxyURL string, isSetupToken bool) (*oauth.TokenResponse, error) {
	if m.exchangeCodeFunc != nil {
		return m.exchangeCodeFunc(ctx, code, codeVerifier, state, proxyURL, isSetupToken)
REDACTED
	panic("ExchangeCodeForToken not implemented")
REDACTED

func (m *mockClaudeOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error) {
	if m.refreshTokenFunc != nil {
		return m.refreshTokenFunc(ctx, refreshToken, proxyURL)
REDACTED
	panic("RefreshToken not implemented")
REDACTED

// --- mock: ProxyRepository (最小实现，仅覆盖 OAuthService 依赖的方法) ---

type mockProxyRepoForOAuth struct {
	getByIDFunc func(ctx context.Context, id int64) (*Proxy, error)
REDACTED

func (m *mockProxyRepoForOAuth) Create(ctx context.Context, proxy *Proxy) error {
	panic("Create not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
REDACTED
	return nil, fmt.Errorf("proxy not found")
REDACTED
func (m *mockProxyRepoForOAuth) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	panic("ListByIDs not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) Update(ctx context.Context, proxy *Proxy) error {
	panic("Update not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) Delete(ctx context.Context, id int64) error {
	panic("Delete not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("List not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("ListWithFilters not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("ListWithFiltersAndAccountCount not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ListActive(ctx context.Context) ([]Proxy, error) {
	panic("ListActive not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	panic("ListActiveWithAccountCount not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	panic("ExistsByHostPortAuth not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	panic("CountAccountsByProxyID not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	panic("ListAccountSummariesByProxyID not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) SweepExpiredProxies(ctx context.Context, now time.Time) (int64, error) {
	panic("SweepExpiredProxies not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) ListAllForFallback(ctx context.Context) ([]Proxy, error) {
	panic("ListAllForFallback not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) CountExpired(ctx context.Context) (int64, error) {
	panic("CountExpired not implemented")
REDACTED
func (m *mockProxyRepoForOAuth) CountExpiringSoon(ctx context.Context, now time.Time) (int64, error) {
	panic("CountExpiringSoon not implemented")
REDACTED

// =====================
// 测试用例
// =====================

func TestNewOAuthService(t *testing.T) {
	t.Parallel()

	proxyRepo := &mockProxyRepoForOAuth{REDACTED
	client := &mockClaudeOAuthClient{REDACTED
	svc := NewOAuthService(proxyRepo, client)

	if svc == nil {
		t.Fatal("NewOAuthService 返回 nil")
REDACTED
	if svc.proxyRepo != proxyRepo {
		t.Fatal("proxyRepo 未正确设置")
REDACTED
	if svc.oauthClient != client {
		t.Fatal("oauthClient 未正确设置")
REDACTED
	if svc.sessionStore == nil {
		t.Fatal("sessionStore 应被自动初始化")
REDACTED

	// 清理
	svc.Stop()
REDACTED

func TestOAuthService_GenerateAuthURL(t *testing.T) {
	t.Parallel()

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, &mockClaudeOAuthClient{REDACTED)
	defer svc.Stop()

	result, err := svc.GenerateAuthURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateAuthURL 返回错误: %v", err)
REDACTED
	if result == nil {
		t.Fatal("GenerateAuthURL 返回 nil")
REDACTED
	if result.AuthURL == "" {
		t.Fatal("AuthURL 为空")
REDACTED
	if result.SessionID == "" {
		t.Fatal("SessionID 为空")
REDACTED

	// 验证 session 已存储
	session, ok := svc.sessionStore.Get(result.SessionID)
	if !ok {
		t.Fatal("session 未在 sessionStore 中找到")
REDACTED
	if session.Scope != oauth.ScopeOAuth {
		t.Fatalf("scope 不匹配: got=%q want=%q", session.Scope, oauth.ScopeOAuth)
REDACTED
REDACTED

func TestOAuthService_GenerateAuthURL_WithProxy(t *testing.T) {
	t.Parallel()

	proxyRepo := &mockProxyRepoForOAuth{
		getByIDFunc: func(ctx context.Context, id int64) (*Proxy, error) {
			return &Proxy{
				ID:       1,
				Protocol: "http",
				Host:     "proxy.example.com",
				Port:     8080,
		REDACTED, nil
	REDACTED,
REDACTED
	svc := NewOAuthService(proxyRepo, &mockClaudeOAuthClient{REDACTED)
	defer svc.Stop()

	proxyID := int64(1)
	result, err := svc.GenerateAuthURL(context.Background(), &proxyID)
	if err != nil {
		t.Fatalf("GenerateAuthURL 返回错误: %v", err)
REDACTED

	session, ok := svc.sessionStore.Get(result.SessionID)
	if !ok {
		t.Fatal("session 未在 sessionStore 中找到")
REDACTED
	if session.ProxyURL != "http://proxy.example.com:8080" {
		t.Fatalf("ProxyURL 不匹配: got=%q", session.ProxyURL)
REDACTED
REDACTED

func TestOAuthService_GenerateSetupTokenURL(t *testing.T) {
	t.Parallel()

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, &mockClaudeOAuthClient{REDACTED)
	defer svc.Stop()

	result, err := svc.GenerateSetupTokenURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateSetupTokenURL 返回错误: %v", err)
REDACTED
	if result == nil {
		t.Fatal("GenerateSetupTokenURL 返回 nil")
REDACTED

	// 验证 scope 是 inference
	session, ok := svc.sessionStore.Get(result.SessionID)
	if !ok {
		t.Fatal("session 未在 sessionStore 中找到")
REDACTED
	if session.Scope != oauth.ScopeInference {
		t.Fatalf("scope 不匹配: got=%q want=%q", session.Scope, oauth.ScopeInference)
REDACTED
REDACTED

func TestOAuthService_ExchangeCode_SessionNotFound(t *testing.T) {
	t.Parallel()

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, &mockClaudeOAuthClient{REDACTED)
	defer svc.Stop()

	_, err := svc.ExchangeCode(context.Background(), &ExchangeCodeInput{
		SessionID: "nonexistent-session",
		Code:      "test-code",
REDACTED)
	if err == nil {
		t.Fatal("ExchangeCode 应返回错误（session 不存在）")
REDACTED
	if err.Error() != "session not found or expired" {
		t.Fatalf("错误信息不匹配: got=%q", err.Error())
REDACTED
REDACTED

func TestOAuthService_ExchangeCode_Success(t *testing.T) {
	t.Parallel()

	exchangeCalled := false
	client := &mockClaudeOAuthClient{
		exchangeCodeFunc: func(ctx context.Context, code, codeVerifier, state, proxyURL string, isSetupToken bool) (*oauth.TokenResponse, error) {
			exchangeCalled = true
			if code != "auth-code-123" {
				t.Errorf("code 不匹配: got=%q", code)
		REDACTED
			if isSetupToken {
				t.Error("isSetupToken 应为 false（ScopeOAuth）")
		REDACTED
			return &oauth.TokenResponse{
				AccessToken:  "access-token-abc",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "refresh-token-xyz",
				Scope:        oauth.ScopeOAuth,
				Organization: &oauth.OrgInfo{UUID: "org-uuid-111"REDACTED,
				Account:      &oauth.AccountInfo{UUID: "acc-uuid-222", EmailAddress: "test@example.com"REDACTED,
		REDACTED, nil
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	// 先生成 URL 以创建 session
	result, err := svc.GenerateAuthURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateAuthURL 返回错误: %v", err)
REDACTED

	// 交换 code
	tokenInfo, err := svc.ExchangeCode(context.Background(), &ExchangeCodeInput{
		SessionID: result.SessionID,
		Code:      "auth-code-123",
REDACTED)
	if err != nil {
		t.Fatalf("ExchangeCode 返回错误: %v", err)
REDACTED

	if !exchangeCalled {
		t.Fatal("ExchangeCodeForToken 未被调用")
REDACTED
	if tokenInfo.AccessToken != "access-token-abc" {
		t.Fatalf("AccessToken 不匹配: got=%q", tokenInfo.AccessToken)
REDACTED
	if tokenInfo.TokenType != "Bearer" {
		t.Fatalf("TokenType 不匹配: got=%q", tokenInfo.TokenType)
REDACTED
	if tokenInfo.RefreshToken != "refresh-token-xyz" {
		t.Fatalf("RefreshToken 不匹配: got=%q", tokenInfo.RefreshToken)
REDACTED
	if tokenInfo.OrgUUID != "org-uuid-111" {
		t.Fatalf("OrgUUID 不匹配: got=%q", tokenInfo.OrgUUID)
REDACTED
	if tokenInfo.AccountUUID != "acc-uuid-222" {
		t.Fatalf("AccountUUID 不匹配: got=%q", tokenInfo.AccountUUID)
REDACTED
	if tokenInfo.EmailAddress != "test@example.com" {
		t.Fatalf("EmailAddress 不匹配: got=%q", tokenInfo.EmailAddress)
REDACTED
	if tokenInfo.ExpiresIn != 3600 {
		t.Fatalf("ExpiresIn 不匹配: got=%d", tokenInfo.ExpiresIn)
REDACTED
	if tokenInfo.ExpiresAt == 0 {
		t.Fatal("ExpiresAt 不应为 0")
REDACTED

	// 验证 session 已被删除
	_, ok := svc.sessionStore.Get(result.SessionID)
	if ok {
		t.Fatal("session 应在交换成功后被删除")
REDACTED
REDACTED

func TestOAuthService_ExchangeCode_SetupToken(t *testing.T) {
	t.Parallel()

	client := &mockClaudeOAuthClient{
		exchangeCodeFunc: func(ctx context.Context, code, codeVerifier, state, proxyURL string, isSetupToken bool) (*oauth.TokenResponse, error) {
			if !isSetupToken {
				t.Error("isSetupToken 应为 true（ScopeInference）")
		REDACTED
			return &oauth.TokenResponse{
				AccessToken: "setup-token",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
				Scope:       oauth.ScopeInference,
		REDACTED, nil
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	// 使用 SetupToken URL（inference scope）
	result, err := svc.GenerateSetupTokenURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateSetupTokenURL 返回错误: %v", err)
REDACTED

	tokenInfo, err := svc.ExchangeCode(context.Background(), &ExchangeCodeInput{
		SessionID: result.SessionID,
		Code:      "setup-code",
REDACTED)
	if err != nil {
		t.Fatalf("ExchangeCode 返回错误: %v", err)
REDACTED
	if tokenInfo.AccessToken != "setup-token" {
		t.Fatalf("AccessToken 不匹配: got=%q", tokenInfo.AccessToken)
REDACTED
REDACTED

func TestOAuthService_ExchangeCode_ClientError(t *testing.T) {
	t.Parallel()

	client := &mockClaudeOAuthClient{
		exchangeCodeFunc: func(ctx context.Context, code, codeVerifier, state, proxyURL string, isSetupToken bool) (*oauth.TokenResponse, error) {
			return nil, fmt.Errorf("upstream error: invalid code")
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	result, _ := svc.GenerateAuthURL(context.Background(), nil)
	_, err := svc.ExchangeCode(context.Background(), &ExchangeCodeInput{
		SessionID: result.SessionID,
		Code:      "bad-code",
REDACTED)
	if err == nil {
		t.Fatal("ExchangeCode 应返回错误")
REDACTED
	if err.Error() != "upstream error: invalid code" {
		t.Fatalf("错误信息不匹配: got=%q", err.Error())
REDACTED
REDACTED

func TestOAuthService_RefreshToken(t *testing.T) {
	t.Parallel()

	client := &mockClaudeOAuthClient{
		refreshTokenFunc: func(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error) {
			if refreshToken != "my-refresh-token" {
				t.Errorf("refreshToken 不匹配: got=%q", refreshToken)
		REDACTED
			if proxyURL != "" {
				t.Errorf("proxyURL 应为空: got=%q", proxyURL)
		REDACTED
			return &oauth.TokenResponse{
				AccessToken:  "new-access-token",
				TokenType:    "Bearer",
				ExpiresIn:    7200,
				RefreshToken: "new-refresh-token",
				Scope:        oauth.ScopeOAuth,
		REDACTED, nil
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	tokenInfo, err := svc.RefreshToken(context.Background(), "my-refresh-token", "")
	if err != nil {
		t.Fatalf("RefreshToken 返回错误: %v", err)
REDACTED
	if tokenInfo.AccessToken != "new-access-token" {
		t.Fatalf("AccessToken 不匹配: got=%q", tokenInfo.AccessToken)
REDACTED
	if tokenInfo.RefreshToken != "new-refresh-token" {
		t.Fatalf("RefreshToken 不匹配: got=%q", tokenInfo.RefreshToken)
REDACTED
	if tokenInfo.ExpiresIn != 7200 {
		t.Fatalf("ExpiresIn 不匹配: got=%d", tokenInfo.ExpiresIn)
REDACTED
	if tokenInfo.ExpiresAt == 0 {
		t.Fatal("ExpiresAt 不应为 0")
REDACTED
REDACTED

func TestOAuthService_RefreshToken_Error(t *testing.T) {
	t.Parallel()

	client := &mockClaudeOAuthClient{
		refreshTokenFunc: func(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error) {
			return nil, fmt.Errorf("invalid_grant: token expired")
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	_, err := svc.RefreshToken(context.Background(), "expired-token", "")
	if err == nil {
		t.Fatal("RefreshToken 应返回错误")
REDACTED
REDACTED

func TestOAuthService_RefreshAccountToken_NoRefreshToken(t *testing.T) {
	t.Parallel()

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, &mockClaudeOAuthClient{REDACTED)
	defer svc.Stop()

	// 无 refresh_token 的账号
	account := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "some-token",
	REDACTED,
REDACTED
	_, err := svc.RefreshAccountToken(context.Background(), account)
	if err == nil {
		t.Fatal("RefreshAccountToken 应返回错误（无 refresh_token）")
REDACTED
	if err.Error() != "no refresh token available" {
		t.Fatalf("错误信息不匹配: got=%q", err.Error())
REDACTED
REDACTED

func TestOAuthService_RefreshAccountToken_EmptyRefreshToken(t *testing.T) {
	t.Parallel()

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, &mockClaudeOAuthClient{REDACTED)
	defer svc.Stop()

	account := &Account{
		ID:       2,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "some-token",
			"refresh_token": "",
	REDACTED,
REDACTED
	_, err := svc.RefreshAccountToken(context.Background(), account)
	if err == nil {
		t.Fatal("RefreshAccountToken 应返回错误（refresh_token 为空）")
REDACTED
REDACTED

func TestOAuthService_RefreshAccountToken_Success(t *testing.T) {
	t.Parallel()

	client := &mockClaudeOAuthClient{
		refreshTokenFunc: func(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error) {
			if refreshToken != "account-refresh-token" {
				t.Errorf("refreshToken 不匹配: got=%q", refreshToken)
		REDACTED
			return &oauth.TokenResponse{
				AccessToken:  "refreshed-access",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "new-refresh",
		REDACTED, nil
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	account := &Account{
		ID:       3,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "old-access",
			"refresh_token": "account-refresh-token",
	REDACTED,
REDACTED

	tokenInfo, err := svc.RefreshAccountToken(context.Background(), account)
	if err != nil {
		t.Fatalf("RefreshAccountToken 返回错误: %v", err)
REDACTED
	if tokenInfo.AccessToken != "refreshed-access" {
		t.Fatalf("AccessToken 不匹配: got=%q", tokenInfo.AccessToken)
REDACTED
REDACTED

func TestOAuthService_RefreshAccountToken_WithProxy(t *testing.T) {
	t.Parallel()

	proxyRepo := &mockProxyRepoForOAuth{
		getByIDFunc: func(ctx context.Context, id int64) (*Proxy, error) {
			return &Proxy{
				Protocol: "socks5",
				Host:     "socks.example.com",
				Port:     1080,
				Username: "user",
				Password: "pass",
		REDACTED, nil
	REDACTED,
REDACTED

	client := &mockClaudeOAuthClient{
		refreshTokenFunc: func(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error) {
			if proxyURL != "socks5://user:pass@socks.example.com:1080" {
				t.Errorf("proxyURL 不匹配: got=%q", proxyURL)
		REDACTED
			return &oauth.TokenResponse{
				AccessToken: "refreshed",
				ExpiresIn:   3600,
		REDACTED, nil
	REDACTED,
REDACTED

	svc := NewOAuthService(proxyRepo, client)
	defer svc.Stop()

	proxyID := int64(10)
	account := &Account{
		ID:       4,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		ProxyID:  &proxyID,
REDACTED
			"refresh_token": "rt-with-proxy",
	REDACTED,
REDACTED

	_, err := svc.RefreshAccountToken(context.Background(), account)
	if err != nil {
		t.Fatalf("RefreshAccountToken 返回错误: %v", err)
REDACTED
REDACTED

func TestOAuthService_ExchangeCode_NilOrg(t *testing.T) {
	t.Parallel()

	client := &mockClaudeOAuthClient{
		exchangeCodeFunc: func(ctx context.Context, code, codeVerifier, state, proxyURL string, isSetupToken bool) (*oauth.TokenResponse, error) {
			return &oauth.TokenResponse{
				AccessToken:  "token-no-org",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				Organization: nil,
				Account:      nil,
		REDACTED, nil
	REDACTED,
REDACTED

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, client)
	defer svc.Stop()

	result, _ := svc.GenerateAuthURL(context.Background(), nil)
	tokenInfo, err := svc.ExchangeCode(context.Background(), &ExchangeCodeInput{
		SessionID: result.SessionID,
		Code:      "code",
REDACTED)
	if err != nil {
		t.Fatalf("ExchangeCode 返回错误: %v", err)
REDACTED
	if tokenInfo.OrgUUID != "" {
		t.Fatalf("OrgUUID 应为空: got=%q", tokenInfo.OrgUUID)
REDACTED
	if tokenInfo.AccountUUID != "" {
		t.Fatalf("AccountUUID 应为空: got=%q", tokenInfo.AccountUUID)
REDACTED
REDACTED

func TestOAuthService_Stop_NoPanic(t *testing.T) {
	t.Parallel()

	svc := NewOAuthService(&mockProxyRepoForOAuth{REDACTED, &mockClaudeOAuthClient{REDACTED)

	// 调用 Stop 不应 panic
	svc.Stop()

	// 多次调用也不应 panic
	svc.Stop()
REDACTED
