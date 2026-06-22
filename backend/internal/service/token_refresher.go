package service

import (
	"context"
	"strings"
	"time"
)

// TokenRefresher 定义平台特定的token刷新策略接口
// 通过此接口可以扩展支持不同平台（Anthropic/OpenAI/Gemini）
type TokenRefresher interface {
	// CanRefresh 检查此刷新器是否能处理指定账号
	CanRefresh(account *Account) bool

	// NeedsRefresh 检查账号的token是否需要刷新
	NeedsRefresh(account *Account, refreshWindow time.Duration) bool

	// Refresh 执行token刷新，返回更新后的credentials
	// 注意：返回的map应该保留原有credentials中的所有字段，只更新token相关字段
	Refresh(ctx context.Context, account *Account) (map[string]any, error)
REDACTED

// ClaudeTokenRefresher 处理Anthropic/Claude OAuth token刷新
type ClaudeTokenRefresher struct {
	oauthService *OAuthService
REDACTED

// NewClaudeTokenRefresher 创建Claude token刷新器
func NewClaudeTokenRefresher(oauthService *OAuthService) *ClaudeTokenRefresher {
	return &ClaudeTokenRefresher{
		oauthService: oauthService,
REDACTED
REDACTED

// CacheKey 返回用于分布式锁的缓存键
func (r *ClaudeTokenRefresher) CacheKey(account *Account) string {
	return ClaudeTokenCacheKey(account)
REDACTED

// CanRefresh 检查是否能处理此账号
// 只处理 anthropic 平台的 oauth 类型账号
// setup-token 虽然也是OAuth，但有效期1年，不需要频繁刷新
func (r *ClaudeTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformAnthropic &&
		account.Type == AccountTypeOAuth
REDACTED

// NeedsRefresh 检查token是否需要刷新
// 基于 expires_at 字段判断是否在刷新窗口内
func (r *ClaudeTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
REDACTED
	return time.Until(*expiresAt) < refreshWindow
REDACTED

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
func (r *ClaudeTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	newCredentials := BuildClaudeAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	return newCredentials, nil
REDACTED

// OpenAITokenRefresher 处理 OpenAI OAuth token刷新
type OpenAITokenRefresher struct {
	openaiOAuthService *OpenAIOAuthService
	accountRepo        AccountRepository
REDACTED

// NewOpenAITokenRefresher 创建 OpenAI token刷新器
func NewOpenAITokenRefresher(openaiOAuthService *OpenAIOAuthService, accountRepo AccountRepository) *OpenAITokenRefresher {
	return &OpenAITokenRefresher{
		openaiOAuthService: openaiOAuthService,
		accountRepo:        accountRepo,
REDACTED
REDACTED

// CacheKey 返回用于分布式锁的缓存键
func (r *OpenAITokenRefresher) CacheKey(account *Account) string {
	return OpenAITokenCacheKey(account)
REDACTED

// CanRefresh 检查是否能处理此账号
func (r *OpenAITokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth
REDACTED

// NeedsRefresh 检查token是否需要刷新
// expires_at 缺失且处于限流状态时需要刷新，防止限流期间 token 静默过期
func (r *OpenAITokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account.IsOpenAIPersonalAccessToken() {
		return false
REDACTED
	if strings.TrimSpace(account.GetOpenAIRefreshToken()) == "" {
		return false
REDACTED
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return account.IsRateLimited()
REDACTED

	return time.Until(*expiresAt) < refreshWindow
REDACTED

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
func (r *OpenAITokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.openaiOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	// 使用服务提供的方法构建新凭证，并保留原有字段
	newCredentials := r.openaiOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	newCredentials = NormalizeOpenAIPersonalAccessTokenCredentials(account, tokenInfo, newCredentials)

	return newCredentials, nil
REDACTED
