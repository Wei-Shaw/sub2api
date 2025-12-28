package service

import (
	"context"
	"strconv"
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
	s := account.GetCredential("expires_at")
	if s == "" {
		return false
REDACTED

	expiresAt, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return false
REDACTED

	return time.Until(time.Unix(expiresAt, 0)) < refreshWindow
REDACTED

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
func (r *ClaudeTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	// 保留现有credentials中的所有字段
	newCredentials := make(map[string]any)
	for k, v := range account.Credentials {
		newCredentials[k] = v
REDACTED

	// 只更新token相关字段
	// 注意：expires_at 和 expires_in 必须存为字符串，因为 GetCredential 只返回 string 类型
	newCredentials["access_token"] = tokenInfo.AccessToken
	newCredentials["token_type"] = tokenInfo.TokenType
	newCredentials["expires_in"] = strconv.FormatInt(tokenInfo.ExpiresIn, 10)
	newCredentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
	if tokenInfo.RefreshToken != "" {
		newCredentials["refresh_token"] = tokenInfo.RefreshToken
REDACTED
	if tokenInfo.Scope != "" {
		newCredentials["scope"] = tokenInfo.Scope
REDACTED

	return newCredentials, nil
REDACTED

// OpenAITokenRefresher 处理 OpenAI OAuth token刷新
type OpenAITokenRefresher struct {
	openaiOAuthService *OpenAIOAuthService
REDACTED

// NewOpenAITokenRefresher 创建 OpenAI token刷新器
func NewOpenAITokenRefresher(openaiOAuthService *OpenAIOAuthService) *OpenAITokenRefresher {
	return &OpenAITokenRefresher{
		openaiOAuthService: openaiOAuthService,
REDACTED
REDACTED

// CanRefresh 检查是否能处理此账号
// 只处理 openai 平台的 oauth 类型账号
func (r *OpenAITokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformOpenAI &&
		account.Type == AccountTypeOAuth
REDACTED

// NeedsRefresh 检查token是否需要刷新
// 基于 expires_at 字段判断是否在刷新窗口内
func (r *OpenAITokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false
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

	// 保留原有credentials中非token相关字段
	for k, v := range account.Credentials {
		if _, exists := newCredentials[k]; !exists {
			newCredentials[k] = v
	REDACTED
REDACTED

	return newCredentials, nil
REDACTED
