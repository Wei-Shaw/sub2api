package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

// TokenRefreshService OAuth token自动刷新服务
// 定期检查并刷新即将过期的token
type TokenRefreshService struct {
	accountRepo ports.AccountRepository
	refreshers  []TokenRefresher
	cfg         *config.TokenRefreshConfig

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewTokenRefreshService 创建token刷新服务
func NewTokenRefreshService(
	accountRepo ports.AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	cfg *config.Config,
) *TokenRefreshService {
	s := &TokenRefreshService{
		accountRepo: accountRepo,
		cfg:         &cfg.TokenRefresh,
		stopCh:      make(chan struct{}),
	}

	// 注册平台特定的刷新器
	s.refreshers = []TokenRefresher{
		NewClaudeTokenRefresher(oauthService),
		NewOpenAITokenRefresher(openaiOAuthService),
	}

	return s
}

// Start 启动后台刷新服务
func (s *TokenRefreshService) Start() {
	if !s.cfg.Enabled {
		log.Println("[TokenRefresh] Service disabled by configuration")
		return
	}

	s.wg.Add(1)
	go s.refreshLoop()

	log.Printf("[TokenRefresh] Service started (check every %d minutes, refresh %v hours before expiry)",
		s.cfg.CheckIntervalMinutes, s.cfg.RefreshBeforeExpiryHours)
}

// Stop 停止刷新服务
func (s *TokenRefreshService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("[TokenRefresh] Service stopped")
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
		log.Printf("[TokenRefresh] Failed to list accounts: %v", err)
		return
	}

	refreshed, failed := 0, 0

	for i := range accounts {
		account := &accounts[i]

		// 遍历所有刷新器，找到能处理此账号的
		for _, refresher := range s.refreshers {
			if !refresher.CanRefresh(account) {
				continue
			}

			// 检查是否需要刷新
			if !refresher.NeedsRefresh(account, refreshWindow) {
				continue
			}

			// 执行刷新
			if err := s.refreshWithRetry(ctx, account, refresher); err != nil {
				log.Printf("[TokenRefresh] Account %d (%s) failed: %v", account.ID, account.Name, err)
				failed++
			} else {
				log.Printf("[TokenRefresh] Account %d (%s) refreshed successfully", account.ID, account.Name)
				refreshed++
			}

			// 每个账号只由一个refresher处理
			break
		}
	}

	if refreshed > 0 || failed > 0 {
		log.Printf("[TokenRefresh] Cycle complete: %d refreshed, %d failed", refreshed, failed)
	}
}

// listActiveAccounts 获取所有active状态的账号
// 使用ListActive确保刷新所有活跃账号的token（包括临时禁用的）
func (s *TokenRefreshService) listActiveAccounts(ctx context.Context) ([]model.Account, error) {
	return s.accountRepo.ListActive(ctx)
}

// refreshWithRetry 带重试的刷新
func (s *TokenRefreshService) refreshWithRetry(ctx context.Context, account *model.Account, refresher TokenRefresher) error {
	var lastErr error

	for attempt := 1; attempt <= s.cfg.MaxRetries; attempt++ {
		newCredentials, err := refresher.Refresh(ctx, account)
		if err == nil {
			// 刷新成功，更新账号credentials
			account.Credentials = model.JSONB(newCredentials)
			if err := s.accountRepo.Update(ctx, account); err != nil {
				return fmt.Errorf("failed to save credentials: %w", err)
			}
			return nil
		}

		lastErr = err
		log.Printf("[TokenRefresh] Account %d attempt %d/%d failed: %v",
			account.ID, attempt, s.cfg.MaxRetries, err)

		// 如果还有重试机会，等待后重试
		if attempt < s.cfg.MaxRetries {
			// 指数退避：2^(attempt-1) * baseSeconds
			backoff := time.Duration(s.cfg.RetryBackoffSeconds) * time.Second * time.Duration(1<<(attempt-1))
			time.Sleep(backoff)
		}
	}

	// 所有重试都失败，标记账号为error状态
	errorMsg := fmt.Sprintf("Token refresh failed after %d retries: %v", s.cfg.MaxRetries, lastErr)
	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		log.Printf("[TokenRefresh] Failed to set error status for account %d: %v", account.ID, err)
	}

	return lastErr
}
