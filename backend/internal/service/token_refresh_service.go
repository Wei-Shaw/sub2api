package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// TokenRefreshService OAuth token自动刷新服务
// 定期检查并刷新即将过期的token
type TokenRefreshService struct {
	accountRepo      AccountRepository
	refreshers       []TokenRefresher
	cfg              *config.TokenRefreshConfig
	cacheInvalidator TokenCacheInvalidator
	schedulerCache   SchedulerCache   // 用于同步更新调度器缓存，解决 token 刷新后缓存不一致问题
	tempUnschedCache TempUnschedCache // 用于清除 Redis 中的临时不可调度缓存

	stopCh chan struct{REDACTED
	wg     sync.WaitGroup
REDACTED

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
		cfg:              &cfg.TokenRefresh,
		cacheInvalidator: cacheInvalidator,
		schedulerCache:   schedulerCache,
		tempUnschedCache: tempUnschedCache,
		stopCh:           make(chan struct{REDACTED),
REDACTED

	openAIRefresher := NewOpenAITokenRefresher(openaiOAuthService, accountRepo)
	openAIRefresher.SetSyncLinkedSoraAccounts(cfg.TokenRefresh.SyncLinkedSoraAccounts)

	// 注册平台特定的刷新器
	s.refreshers = []TokenRefresher{
		NewClaudeTokenRefresher(oauthService),
		openAIRefresher,
		NewGeminiTokenRefresher(geminiOAuthService),
		NewAntigravityTokenRefresher(antigravityOAuthService),
REDACTED

	return s
REDACTED

// SetSoraAccountRepo 设置 Sora 账号扩展表仓储
// 用于在 OpenAI Token 刷新时同步更新 sora_accounts 表
// 需要在 Start() 之前调用
func (s *TokenRefreshService) SetSoraAccountRepo(repo SoraAccountRepository) {
	// 将 soraAccountRepo 注入到 OpenAITokenRefresher
	for _, refresher := range s.refreshers {
		if openaiRefresher, ok := refresher.(*OpenAITokenRefresher); ok {
			openaiRefresher.SetSoraAccountRepo(repo)
	REDACTED
REDACTED
REDACTED

// Start 启动后台刷新服务
func (s *TokenRefreshService) Start() {
	if !s.cfg.Enabled {
		slog.Info("token_refresh.service_disabled")
		return
REDACTED

	s.wg.Add(1)
	go s.refreshLoop()

	slog.Info("token_refresh.service_started",
		"check_interval_minutes", s.cfg.CheckIntervalMinutes,
		"refresh_before_expiry_hours", s.cfg.RefreshBeforeExpiryHours,
	)
REDACTED

// Stop 停止刷新服务
func (s *TokenRefreshService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	slog.Info("token_refresh.service_stopped")
REDACTED

// refreshLoop 刷新循环
func (s *TokenRefreshService) refreshLoop() {
	defer s.wg.Done()

	// 计算检查间隔
	checkInterval := time.Duration(s.cfg.CheckIntervalMinutes) * time.Minute
	if checkInterval < time.Minute {
		checkInterval = 5 * time.Minute
REDACTED

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
	REDACTED
REDACTED
REDACTED

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
REDACTED

	totalAccounts := len(accounts)
	oauthAccounts := 0 // 可刷新的OAuth账号数
	needsRefresh := 0  // 需要刷新的账号数
	refreshed, failed := 0, 0

	for i := range accounts {
		account := &accounts[i]

		// 遍历所有刷新器，找到能处理此账号的
		for _, refresher := range s.refreshers {
			if !refresher.CanRefresh(account) {
				continue
		REDACTED

			oauthAccounts++

			// 检查是否需要刷新
			if !refresher.NeedsRefresh(account, refreshWindow) {
				break // 不需要刷新，跳过
		REDACTED

			needsRefresh++

			// 执行刷新
			if err := s.refreshWithRetry(ctx, account, refresher); err != nil {
				slog.Warn("token_refresh.account_refresh_failed",
					"account_id", account.ID,
					"account_name", account.Name,
					"error", err,
				)
				failed++
		REDACTED else {
				slog.Info("token_refresh.account_refreshed",
					"account_id", account.ID,
					"account_name", account.Name,
				)
				refreshed++
		REDACTED

			// 每个账号只由一个refresher处理
			break
	REDACTED
REDACTED

	// 无刷新活动时降级为 Debug，有实际刷新活动时保持 Info
	if needsRefresh == 0 && failed == 0 {
		slog.Debug("token_refresh.cycle_completed",
			"total", totalAccounts, "oauth", oauthAccounts,
			"needs_refresh", needsRefresh, "refreshed", refreshed, "failed", failed)
REDACTED else {
		slog.Info("token_refresh.cycle_completed",
			"total", totalAccounts,
			"oauth", oauthAccounts,
			"needs_refresh", needsRefresh,
			"refreshed", refreshed,
			"failed", failed,
		)
REDACTED
REDACTED

// listActiveAccounts 获取所有active状态的账号
// 使用ListActive确保刷新所有活跃账号的token（包括临时禁用的）
func (s *TokenRefreshService) listActiveAccounts(ctx context.Context) ([]Account, error) {
	return s.accountRepo.ListActive(ctx)
REDACTED

// refreshWithRetry 带重试的刷新
func (s *TokenRefreshService) refreshWithRetry(ctx context.Context, account *Account, refresher TokenRefresher) error {
	var lastErr error

	for attempt := 1; attempt <= s.cfg.MaxRetries; attempt++ {
		newCredentials, err := refresher.Refresh(ctx, account)

		// 如果有新凭证，先更新（即使有错误也要保存 token）
		if newCredentials != nil {
			// 记录刷新版本时间戳，用于解决缓存一致性问题
			// TokenProvider 写入缓存前会检查此版本，如果版本已更新则跳过写入
			newCredentials["_token_version"] = time.Now().UnixMilli()

			account.Credentials = newCredentials
			if saveErr := s.accountRepo.Update(ctx, account); saveErr != nil {
				return fmt.Errorf("failed to save credentials: %w", saveErr)
		REDACTED
	REDACTED

		if err == nil {
			// Antigravity 账户：如果之前是因为缺少 project_id 而标记为 error，现在成功获取到了，清除错误状态
			if account.Platform == PlatformAntigravity &&
				account.Status == StatusError &&
				strings.Contains(account.ErrorMessage, "missing_project_id:") {
				if clearErr := s.accountRepo.ClearError(ctx, account.ID); clearErr != nil {
					slog.Warn("token_refresh.clear_account_error_failed",
						"account_id", account.ID,
						"error", clearErr,
					)
			REDACTED else {
					slog.Info("token_refresh.cleared_missing_project_id_error", "account_id", account.ID)
			REDACTED
		REDACTED
			// 刷新成功后清除临时不可调度状态（处理 OAuth 401 恢复场景）
			if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
				if clearErr := s.accountRepo.ClearTempUnschedulable(ctx, account.ID); clearErr != nil {
					slog.Warn("token_refresh.clear_temp_unschedulable_failed",
						"account_id", account.ID,
						"error", clearErr,
					)
			REDACTED else {
					slog.Info("token_refresh.cleared_temp_unschedulable", "account_id", account.ID)
			REDACTED
				// 同步清除 Redis 缓存，避免调度器读到过期的临时不可调度状态
				if s.tempUnschedCache != nil {
					if clearErr := s.tempUnschedCache.DeleteTempUnsched(ctx, account.ID); clearErr != nil {
						slog.Warn("token_refresh.clear_temp_unsched_cache_failed",
							"account_id", account.ID,
							"error", clearErr,
						)
				REDACTED
			REDACTED
		REDACTED
			// 对所有 OAuth 账号调用缓存失效（InvalidateToken 内部根据平台判断是否需要处理）
			if s.cacheInvalidator != nil && account.Type == AccountTypeOAuth {
				if err := s.cacheInvalidator.InvalidateToken(ctx, account); err != nil {
					slog.Warn("token_refresh.invalidate_token_cache_failed",
						"account_id", account.ID,
						"error", err,
					)
			REDACTED else {
					slog.Debug("token_refresh.token_cache_invalidated", "account_id", account.ID)
			REDACTED
		REDACTED
			// 同步更新调度器缓存，确保调度获取的 Account 对象包含最新的 credentials
			// 这解决了 token 刷新后调度器缓存数据不一致的问题（#445）
			if s.schedulerCache != nil {
				if err := s.schedulerCache.SetAccount(ctx, account); err != nil {
					slog.Warn("token_refresh.sync_scheduler_cache_failed",
						"account_id", account.ID,
						"error", err,
					)
			REDACTED else {
					slog.Debug("token_refresh.scheduler_cache_synced", "account_id", account.ID)
			REDACTED
		REDACTED
			return nil
	REDACTED

		// 不可重试错误（invalid_grant/invalid_client 等）直接标记 error 状态并返回
		if isNonRetryableRefreshError(err) {
			errorMsg := fmt.Sprintf("Token refresh failed (non-retryable): %v", err)
			if setErr := s.accountRepo.SetError(ctx, account.ID, errorMsg); setErr != nil {
				slog.Error("token_refresh.set_error_status_failed",
					"account_id", account.ID,
					"error", setErr,
				)
		REDACTED
			return err
	REDACTED

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
	REDACTED
REDACTED

	// 可重试错误耗尽：仅记录日志，不标记 error（可能是临时网络问题，下个周期继续重试）
	slog.Warn("token_refresh.retry_exhausted",
		"account_id", account.ID,
		"platform", account.Platform,
		"max_retries", s.cfg.MaxRetries,
		"error", lastErr,
	)

	return lastErr
REDACTED

// isNonRetryableRefreshError 判断是否为不可重试的刷新错误
// 这些错误通常表示凭证已失效或配置确实缺失，需要用户重新授权
// 注意：missing_project_id 错误只在真正缺失（从未获取过）时返回，临时获取失败不会返回此错误
func isNonRetryableRefreshError(err error) bool {
	if err == nil {
		return false
REDACTED
	msg := strings.ToLower(err.Error())
	nonRetryable := []string{
		"invalid_grant",       // refresh_token 已失效
		"invalid_client",      // 客户端配置错误
		"unauthorized_client", // 客户端未授权
		"access_denied",       // 访问被拒绝
		"missing_project_id",  // 缺少 project_id
REDACTED
	for _, needle := range nonRetryable {
		if strings.Contains(msg, needle) {
			return true
	REDACTED
REDACTED
	return false
REDACTED
