package service

import (
	"context"
	"log"
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
}

// ClaudeTokenRefresher 处理Anthropic/Claude OAuth token刷新
type ClaudeTokenRefresher struct {
	oauthService *OAuthService
}

// NewClaudeTokenRefresher 创建Claude token刷新器
func NewClaudeTokenRefresher(oauthService *OAuthService) *ClaudeTokenRefresher {
	return &ClaudeTokenRefresher{
		oauthService: oauthService,
	}
}

// CacheKey 返回用于分布式锁的缓存键
func (r *ClaudeTokenRefresher) CacheKey(account *Account) string {
	return ClaudeTokenCacheKey(account)
}

// CanRefresh 检查是否能处理此账号
// 只处理 anthropic 平台的 oauth 类型账号
// setup-token 虽然也是OAuth，但有效期1年，不需要频繁刷新
func (r *ClaudeTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformAnthropic &&
		account.Type == AccountTypeOAuth
}

// NeedsRefresh 检查token是否需要刷新
// 基于 expires_at 字段判断是否在刷新窗口内
func (r *ClaudeTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}
	return time.Until(*expiresAt) < refreshWindow
}

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
func (r *ClaudeTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	newCredentials := BuildClaudeAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	return newCredentials, nil
}

// OpenAITokenRefresher 处理 OpenAI OAuth token刷新
type OpenAITokenRefresher struct {
	openaiOAuthService *OpenAIOAuthService
	accountRepo        AccountRepository
	soraAccountRepo    SoraAccountRepository // Sora 扩展表仓储，用于双表同步
	syncLinkedSora     bool
}

// NewOpenAITokenRefresher 创建 OpenAI token刷新器
func NewOpenAITokenRefresher(openaiOAuthService *OpenAIOAuthService, accountRepo AccountRepository) *OpenAITokenRefresher {
	return &OpenAITokenRefresher{
		openaiOAuthService: openaiOAuthService,
		accountRepo:        accountRepo,
	}
}

// CacheKey 返回用于分布式锁的缓存键
func (r *OpenAITokenRefresher) CacheKey(account *Account) string {
	return OpenAITokenCacheKey(account)
}

// SetSoraAccountRepo 设置 Sora 账号扩展表仓储
// 用于在 Token 刷新时同步更新 sora_accounts 表
// 如果未设置，syncLinkedSoraAccounts 只会更新 accounts.credentials
func (r *OpenAITokenRefresher) SetSoraAccountRepo(repo SoraAccountRepository) {
	r.soraAccountRepo = repo
}

// SetSyncLinkedSoraAccounts 控制是否同步覆盖关联的 Sora 账号 token。
func (r *OpenAITokenRefresher) SetSyncLinkedSoraAccounts(enabled bool) {
	r.syncLinkedSora = enabled
}

// CanRefresh 检查是否能处理此账号
// 只处理 openai 平台的 oauth 类型账号（不直接刷新 sora 平台账号）
func (r *OpenAITokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth
}

// NeedsRefresh 检查token是否需要刷新
// 基于 expires_at 字段判断是否在刷新窗口内
func (r *OpenAITokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}

	return time.Until(*expiresAt) < refreshWindow
}

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
// 刷新成功后，异步同步关联的 Sora 账号
func (r *OpenAITokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.openaiOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 使用服务提供的方法构建新凭证，并保留原有字段
	newCredentials := r.openaiOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	// 异步同步关联的 Sora 账号（不阻塞主流程）
	if r.accountRepo != nil && r.syncLinkedSora {
		go r.syncLinkedSoraAccounts(context.Background(), account.ID, newCredentials)
	}

	return newCredentials, nil
}

// syncLinkedSoraAccounts 同步关联的 Sora 账号的 token（双表同步）
// 该方法异步执行，失败只记录日志，不影响主流程
//
// 同步策略：
//  1. 更新 accounts.credentials（主表）
//  2. 更新 sora_accounts 扩展表（如果 soraAccountRepo 已设置）
//
// 超时控制：30 秒，防止数据库阻塞导致 goroutine 泄漏
func (r *OpenAITokenRefresher) syncLinkedSoraAccounts(ctx context.Context, openaiAccountID int64, newCredentials map[string]any) {
	// 添加超时控制，防止 goroutine 泄漏
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 查找所有关联的 Sora 账号（限定 platform='sora'）
	soraAccounts, err := r.accountRepo.FindByExtraField(ctx, "linked_openai_account_id", openaiAccountID)
	if err != nil {
		log.Printf("[TokenSync] 查找关联 Sora 账号失败: openai_account_id=%d err=%v", openaiAccountID, err)
		return
	}

	if len(soraAccounts) == 0 {
		// 没有关联的 Sora 账号，直接返回
		return
	}

	// 2. 同步更新每个 Sora 账号的双表数据
	for _, soraAccount := range soraAccounts {
		// 2.1 更新 accounts.credentials（主表）
		soraAccount.Credentials["access_token"] = newCredentials["access_token"]
		soraAccount.Credentials["refresh_token"] = newCredentials["refresh_token"]
		if expiresAt, ok := newCredentials["expires_at"]; ok {
			soraAccount.Credentials["expires_at"] = expiresAt
		}

		if err := r.accountRepo.Update(ctx, &soraAccount); err != nil {
			log.Printf("[TokenSync] 更新 Sora accounts 表失败: sora_account_id=%d openai_account_id=%d err=%v",
				soraAccount.ID, openaiAccountID, err)
			continue
		}

		// 2.2 更新 sora_accounts 扩展表（如果仓储已设置）
		if r.soraAccountRepo != nil {
			soraUpdates := map[string]any{
				"access_token":  newCredentials["access_token"],
				"refresh_token": newCredentials["refresh_token"],
			}
			if err := r.soraAccountRepo.Upsert(ctx, soraAccount.ID, soraUpdates); err != nil {
				log.Printf("[TokenSync] 更新 sora_accounts 表失败: account_id=%d openai_account_id=%d err=%v",
					soraAccount.ID, openaiAccountID, err)
				// 继续处理其他账号，不中断
			}
		}

		log.Printf("[TokenSync] 成功同步 Sora 账号 token: sora_account_id=%d openai_account_id=%d dual_table=%v",
			soraAccount.ID, openaiAccountID, r.soraAccountRepo != nil)
	}
}
