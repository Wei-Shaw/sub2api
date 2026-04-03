package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// tokenRefreshTempUnschedDuration token 刷新重试耗尽后临时不可调度的持续时间
const tokenRefreshTempUnschedDuration = 10 * time.Minute

// TokenRefreshService OAuth token自动刷新服务
// 定期检查并刷新即将过期的token
type TokenRefreshService struct {
	accountRepo      AccountRepository
	refreshers       []TokenRefresher
	executors        []OAuthRefreshExecutor // 与 refreshers 一一对应的 executor（带 CacheKey）
	refreshPolicy    BackgroundRefreshPolicy
	cfg              *config.TokenRefreshConfig
	cacheInvalidator TokenCacheInvalidator
	schedulerCache   SchedulerCache   // 用于同步更新调度器缓存，解决 token 刷新后缓存不一致问题
	tempUnschedCache TempUnschedCache // 用于清除 Redis 中的临时不可调度缓存
	refreshAPI       *OAuthRefreshAPI // 统一刷新 API

	// OpenAI privacy: 刷新成功后检查并设置 training opt-out
	privacyClientFactory PrivacyClientFactory
	proxyRepo            ProxyRepository

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewTokenRefreshService 创建token刷新服务
func NewTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	antigravityOAuthService *AntigravityOAuthService,
	cacheInvalidator TokenCacheInvalidator,
	schedulerCache SchedulerCache,
	cfg *config.Config,
	tempUnschedCache TempUnschedCache,
) *TokenRefreshService {
	s := &TokenRefreshService{
		accountRepo:      accountRepo,
		refreshPolicy:    DefaultBackgroundRefreshPolicy(),
		cfg:              &cfg.TokenRefresh,
		cacheInvalidator: cacheInvalidator,
		schedulerCache:   schedulerCache,
		tempUnschedCache: tempUnschedCache,
		stopCh:           make(chan struct{}),
	}

	openAIRefresher := NewOpenAITokenRefresher(openaiOAuthService, accountRepo)
	openAIRefresher.SetSyncLinkedSoraAccounts(cfg.TokenRefresh.SyncLinkedSoraAccounts)

	claudeRefresher := NewClaudeTokenRefresher(oauthService)
	geminiRefresher := NewGeminiTokenRefresher(geminiOAuthService)
	agRefresher := NewAntigravityTokenRefresher(antigravityOAuthService)

	// 注册平台特定的刷新器（TokenRefresher 接口）
	s.refreshers = []TokenRefresher{
		claudeRefresher,
		openAIRefresher,
		geminiRefresher,
		agRefresher,
	}

	// 注册对应的 OAuthRefreshExecutor（带 CacheKey 方法）
	s.executors = []OAuthRefreshExecutor{
		claudeRefresher,
		openAIRefresher,
		geminiRefresher,
		agRefresher,
	}

	return s
}

// SetSoraAccountRepo 设置 Sora 账号扩展表仓储
// 用于在 OpenAI Token 刷新时同步更新 sora_accounts 表
// 需要在 Start() 之前调用
func (s *TokenRefreshService) SetSoraAccountRepo(repo SoraAccountRepository) {
	// 将 soraAccountRepo 注入到 OpenAITokenRefresher
	for _, refresher := range s.refreshers {
		if openaiRefresher, ok := refresher.(*OpenAITokenRefresher); ok {
			openaiRefresher.SetSoraAccountRepo(repo)
		}
	}
}

// SetPrivacyDeps 注入 OpenAI privacy opt-out 所需依赖
func (s *TokenRefreshService) SetPrivacyDeps(factory PrivacyClientFactory, proxyRepo ProxyRepository) {
	s.privacyClientFactory = factory
	s.proxyRepo = proxyRepo
}

// SetRefreshAPI 注入统一的 OAuth 刷新 API
func (s *TokenRefreshService) SetRefreshAPI(api *OAuthRefreshAPI) {
	s.refreshAPI = api
}

// SetRefreshPolicy 注入后台刷新调用侧策略（用于显式化平台/场景差异行为）。
func (s *TokenRefreshService) SetRefreshPolicy(policy BackgroundRefreshPolicy) {
	s.refreshPolicy = policy
}

// Start 启动后台刷新服务
func (s *TokenRefreshService) Start() {
	if !s.cfg.Enabled {
		slog.Info("token_refresh.service_disabled")
		return
	}

	s.wg.Add(1)
	go s.refreshLoop()

	slog.Info("token_refresh.service_started",
		"check_interval_minutes", s.cfg.CheckIntervalMinutes,
		"refresh_before_expiry_hours", s.cfg.RefreshBeforeExpiryHours,
	)
}

// Stop 停止刷新服务（可安全多次调用）
func (s *TokenRefreshService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
	slog.Info("token_refresh.service_stopped")
}

// refreshLoop 刷新循环
func (s *TokenRefreshService) refreshLoop() {
	defer s.wg.Done()

	// 计算检查间隔
	checkInterval := time.Duration(s.cfg.CheckIntervalMinutes) * time.Minute
	if checkInterval < time.Minute {
		checkInterval = 5 * time.Minute
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 启动时立即执行一次检查
	s.processRefresh()

	for {
		select {
		case <-ticker.C:
			s.processRefresh()
		case <-s.stopCh:
			return
		}
	}
}

// processRefresh 执行一次刷新检查
func (s *TokenRefreshService) processRefresh() {
	ctx := context.Background()

	// 计算刷新窗口
	refreshWindow := time.Duration(s.cfg.RefreshBeforeExpiryHours * float64(time.Hour))

	// 获取所有active状态的账号
	accounts, err := s.listActiveAccounts(ctx)
	if err != nil {
		slog.Error("token_refresh.list_accounts_failed", "error", err)
		return
	}

	totalAccounts := len(accounts)
	oauthAccounts := 0 // 可刷新的OAuth账号数
	needsRefresh := 0  // 需要刷新的账号数
	refreshed, failed, skipped := 0, 0, 0

	for i := range accounts {
		account := &accounts[i]

		// 遍历所有刷新器，找到能处理此账号的
		for idx, refresher := range s.refreshers {
			if !refresher.CanRefresh(account) {
				continue
			}

			oauthAccounts++

			// 检查是否需要刷新
			if !refresher.NeedsRefresh(account, refreshWindow) {
				break // 不需要刷新，跳过
			}

			needsRefresh++

			// 获取对应的 executor
			var executor OAuthRefreshExecutor
			if idx < len(s.executors) {
				executor = s.executors[idx]
			}

			// 执行刷新
			if err := s.refreshWithRetry(ctx, account, refresher, executor, refreshWindow); err != nil {
				if errors.Is(err, errRefreshSkipped) {
					skipped++
				} else {
					slog.Warn("token_refresh.account_refresh_failed",
						"account_id", account.ID,
						"account_name", account.Name,
						"error", err,
					)
					failed++
				}
			} else {
				slog.Info("token_refresh.account_refreshed",
					"account_id", account.ID,
					"account_name", account.Name,
				)
				refreshed++
			}

			// 每个账号只由一个refresher处理
			break
		}
	}

	// 无刷新活动时降级为 Debug，有实际刷新活动时保持 Info
	if needsRefresh == 0 && failed == 0 {
		slog.Debug("token_refresh.cycle_completed",
			"total", totalAccounts, "oauth", oauthAccounts,
			"needs_refresh", needsRefresh, "refreshed", refreshed, "skipped", skipped, "failed", failed)
	} else {
		slog.Info("token_refresh.cycle_completed",
			"total", totalAccounts,
			"oauth", oauthAccounts,
			"needs_refresh", needsRefresh,
			"refreshed", refreshed,
			"skipped", skipped,
			"failed", failed,
		)
	}
}

// listActiveAccounts 获取所有active状态的账号
// 使用ListActive确保刷新所有活跃账号的token（包括临时禁用的）
func (s *TokenRefreshService) listActiveAccounts(ctx context.Context) ([]Account, error) {
	return s.accountRepo.ListActive(ctx)
}

// refreshWithRetry 带重试的刷新
func (s *TokenRefreshService) refreshWithRetry(ctx context.Context, account *Account, refresher TokenRefresher, executor OAuthRefreshExecutor, refreshWindow time.Duration) error {
	var lastErr error

	for attempt := 1; attempt <= s.cfg.MaxRetries; attempt++ {
		var newCredentials map[string]any
		var err error

		// 优先使用统一 API（带分布式锁 + DB 重读保护）
		if s.refreshAPI != nil && executor != nil {
			result, refreshErr := s.refreshAPI.RefreshIfNeeded(ctx, account, executor, refreshWindow)
			if refreshErr != nil {
				err = refreshErr
			} else if result.LockHeld {
				// 锁被其他 worker 持有，由调用侧策略决定如何计数
				return s.refreshPolicy.handleLockHeld()
			} else if !result.Refreshed {
				// 已被其他路径刷新，由调用侧策略决定如何计数
				return s.refreshPolicy.handleAlreadyRefreshed()
			} else {
				account = result.Account
				_ = result.NewCredentials // 统一 API 已设置 _token_version 并更新 DB，无需重复操作
			}
		} else {
			// 降级：直接调用 refresher（兼容旧路径）
			newCredentials, err = refresher.Refresh(ctx, account)
			if newCredentials != nil {
				newCredentials["_token_version"] = time.Now().UnixMilli()
				if saveErr := persistAccountCredentials(ctx, s.accountRepo, account, newCredentials); saveErr != nil {
					return fmt.Errorf("failed to save credentials: %w", saveErr)
				}
			}
		}

		if err == nil {
			s.postRefreshActions(ctx, account)
			return nil
		}

		// 不可重试错误（invalid_grant/invalid_client 等）直接标记 error 状态并返回
		if isNonRetryableRefreshError(err) {
			errorMsg := fmt.Sprintf("Token refresh failed (non-retryable): %v", err)
			if setErr := s.accountRepo.SetError(ctx, account.ID, errorMsg); setErr != nil {
				slog.Error("token_refresh.set_error_status_failed",
					"account_id", account.ID,
					"error", setErr,
				)
			}
			// 刷新失败但 access_token 可能仍有效，尝试设置隐私
			s.ensureOpenAIPrivacy(ctx, account)
			s.ensureAntigravityPrivacy(ctx, account)
			return err
		}

		lastErr = err
		slog.Warn("token_refresh.retry_attempt_failed",
			"account_id", account.ID,
			"attempt", attempt,
			"max_retries", s.cfg.MaxRetries,
			"error", err,
		)

		// 如果还有重试机会，等待后重试
		if attempt < s.cfg.MaxRetries {
			// 指数退避：2^(attempt-1) * baseSeconds
			backoff := time.Duration(s.cfg.RetryBackoffSeconds) * time.Second * time.Duration(1<<(attempt-1))
			time.Sleep(backoff)
		}
	}

	// 可重试错误耗尽：临时标记账号不可调度，避免请求路径反复命中已知失败的账号
	slog.Warn("token_refresh.retry_exhausted",
		"account_id", account.ID,
		"platform", account.Platform,
		"max_retries", s.cfg.MaxRetries,
		"error", lastErr,
	)

	// 刷新失败但 access_token 可能仍有效，尝试设置隐私
	s.ensureOpenAIPrivacy(ctx, account)
	s.ensureAntigravityPrivacy(ctx, account)

	// 设置临时不可调度 10 分钟（不标记 error，保持 status=active 让下个刷新周期能继续尝试）
	until := time.Now().Add(tokenRefreshTempUnschedDuration)
	reason := fmt.Sprintf("token refresh retry exhausted: %v", lastErr)
	if setErr := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); setErr != nil {
		slog.Warn("token_refresh.set_temp_unschedulable_failed",
			"account_id", account.ID,
			"error", setErr,
		)
	} else {
		slog.Info("token_refresh.temp_unschedulable_set",
			"account_id", account.ID,
			"until", until.Format(time.RFC3339),
		)
	}

	return lastErr
}

// postRefreshActions 刷新成功后的后续动作（清除错误状态、缓存失效、调度器同步等）
func (s *TokenRefreshService) postRefreshActions(ctx context.Context, account *Account) {
	// Antigravity 账户：如果之前是因为缺少 project_id 而标记为 error，现在成功获取到了，清除错误状态
	if account.Platform == PlatformAntigravity &&
		account.Status == StatusError &&
		strings.Contains(account.ErrorMessage, "missing_project_id:") {
		if clearErr := s.accountRepo.ClearError(ctx, account.ID); clearErr != nil {
			slog.Warn("token_refresh.clear_account_error_failed",
				"account_id", account.ID,
				"error", clearErr,
			)
		} else {
			slog.Info("token_refresh.cleared_missing_project_id_error", "account_id", account.ID)
		}
	}
	// 刷新成功后清除临时不可调度状态（处理 OAuth 401 恢复场景）
	if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
		if clearErr := s.accountRepo.ClearTempUnschedulable(ctx, account.ID); clearErr != nil {
			slog.Warn("token_refresh.clear_temp_unschedulable_failed",
				"account_id", account.ID,
				"error", clearErr,
			)
		} else {
			slog.Info("token_refresh.cleared_temp_unschedulable", "account_id", account.ID)
		}
		// 同步清除 Redis 缓存，避免调度器读到过期的临时不可调度状态
		if s.tempUnschedCache != nil {
			if clearErr := s.tempUnschedCache.DeleteTempUnsched(ctx, account.ID); clearErr != nil {
				slog.Warn("token_refresh.clear_temp_unsched_cache_failed",
					"account_id", account.ID,
					"error", clearErr,
				)
			}
		}
	}
	// 对所有 OAuth 账号调用缓存失效（InvalidateToken 内部根据平台判断是否需要处理）
	if s.cacheInvalidator != nil && account.Type == AccountTypeOAuth {
		if err := s.cacheInvalidator.InvalidateToken(ctx, account); err != nil {
			slog.Warn("token_refresh.invalidate_token_cache_failed",
				"account_id", account.ID,
				"error", err,
			)
		} else {
			slog.Debug("token_refresh.token_cache_invalidated", "account_id", account.ID)
		}
	}
	// 同步更新调度器缓存，确保调度获取的 Account 对象包含最新的 credentials
	if s.schedulerCache != nil {
		if err := s.schedulerCache.SetAccount(ctx, account); err != nil {
			slog.Warn("token_refresh.sync_scheduler_cache_failed",
				"account_id", account.ID,
				"error", err,
			)
		} else {
			slog.Debug("token_refresh.scheduler_cache_synced", "account_id", account.ID)
		}
	}
	// OpenAI OAuth: 刷新成功后，检查是否已设置 privacy_mode，未设置则尝试关闭训练数据共享
	s.ensureOpenAIPrivacy(ctx, account)
	// Antigravity OAuth: 刷新成功后，检查是否已设置 privacy_mode，未设置则调用 setUserSettings
	s.ensureAntigravityPrivacy(ctx, account)
}

// errRefreshSkipped 表示刷新被跳过（锁竞争或已被其他路径刷新），不计入 failed 或 refreshed
var errRefreshSkipped = fmt.Errorf("refresh skipped")

// isNonRetryableRefreshError 判断是否为不可重试的刷新错误
// 这些错误通常表示凭证已失效或配置确实缺失，需要用户重新授权
// 注意：missing_project_id 错误只在真正缺失（从未获取过）时返回，临时获取失败不会返回此错误
func isNonRetryableRefreshError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	nonRetryable := []string{
		"invalid_grant",       // refresh_token 已失效
		"invalid_client",      // 客户端配置错误
		"unauthorized_client", // 客户端未授权
		"access_denied",       // 访问被拒绝
		"missing_project_id",  // 缺少 project_id
		"no refresh token available",
	}
	for _, needle := range nonRetryable {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// ensureOpenAIPrivacy 检查 OpenAI OAuth 账号是否已设置 privacy_mode，
// 未设置则调用 disableOpenAITraining 并持久化结果到 Extra。
func (s *TokenRefreshService) ensureOpenAIPrivacy(ctx context.Context, account *Account) {
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return
	}
	if s.privacyClientFactory == nil {
		return
	}
	if shouldSkipOpenAIPrivacyEnsure(account.Extra) {
		return
	}

	token, _ := account.Credentials["access_token"].(string)
	if token == "" {
		return
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}

	mode := disableOpenAITraining(ctx, s.privacyClientFactory, token, proxyURL)
	if mode == "" {
		return
	}

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": mode}); err != nil {
		slog.Warn("token_refresh.update_privacy_mode_failed",
			"account_id", account.ID,
			"error", err,
		)
	} else {
		slog.Info("token_refresh.privacy_mode_set",
			"account_id", account.ID,
			"privacy_mode", mode,
		)
	}
}

// ensureAntigravityPrivacy 后台刷新中检查 Antigravity OAuth 账号隐私状态。
// 仅当 privacy_mode 已成功设置（"privacy_set"）时跳过；
// 未设置或之前失败（"privacy_set_failed"）均会重试。
func (s *TokenRefreshService) ensureAntigravityPrivacy(ctx context.Context, account *Account) {
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return
	}
	if account.Extra != nil {
		if mode, ok := account.Extra["privacy_mode"].(string); ok && mode == AntigravityPrivacySet {
			return
		}
	}

	token, _ := account.Credentials["access_token"].(string)
	if token == "" {
		return
	}

	projectID, _ := account.Credentials["project_id"].(string)

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}

	mode := setAntigravityPrivacy(ctx, token, projectID, proxyURL)
	if mode == "" {
		return
	}

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": mode}); err != nil {
		slog.Warn("token_refresh.update_antigravity_privacy_mode_failed",
			"account_id", account.ID,
			"error", err,
		)
	} else {
		applyAntigravityPrivacyMode(account, mode)
		slog.Info("token_refresh.antigravity_privacy_mode_set",
			"account_id", account.ID,
			"privacy_mode", mode,
		)
	}
}
