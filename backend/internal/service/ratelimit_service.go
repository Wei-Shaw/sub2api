package service

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

// RateLimitService 处理限流和过载状态管理
type RateLimitService struct {
	accountRepo ports.AccountRepository
	cfg         *config.Config
}

// NewRateLimitService 创建RateLimitService实例
func NewRateLimitService(accountRepo ports.AccountRepository, cfg *config.Config) *RateLimitService {
	return &RateLimitService{
		accountRepo: accountRepo,
		cfg:         cfg,
	}
}

// HandleUpstreamError 处理上游错误响应，标记账号状态
// 返回是否应该停止该账号的调度
func (s *RateLimitService) HandleUpstreamError(ctx context.Context, account *model.Account, statusCode int, headers http.Header, responseBody []byte) (shouldDisable bool) {
	// apikey 类型账号：检查自定义错误码配置
	// 如果启用且错误码不在列表中，则不处理（不停止调度、不标记限流/过载）
	if !account.ShouldHandleErrorCode(statusCode) {
		log.Printf("Account %d: error %d skipped (not in custom error codes)", account.ID, statusCode)
		return false
	}

	switch statusCode {
	case 401:
		// 认证失败：停止调度，记录错误
		s.handleAuthError(ctx, account, "Authentication failed (401): invalid or expired credentials")
		return true
	case 403:
		// 禁止访问：停止调度，记录错误
		s.handleAuthError(ctx, account, "Access forbidden (403): account may be suspended or lack permissions")
		return true
	case 429:
		s.handle429(ctx, account, headers)
		return false
	case 529:
		s.handle529(ctx, account)
		return false
	default:
		// 其他5xx错误：记录但不停止调度
		if statusCode >= 500 {
			log.Printf("Account %d received upstream error %d", account.ID, statusCode)
		}
		return false
	}
}

// handleAuthError 处理认证类错误(401/403)，停止账号调度
func (s *RateLimitService) handleAuthError(ctx context.Context, account *model.Account, errorMsg string) {
	if err := s.accountRepo.SetError(ctx, account.ID, errorMsg); err != nil {
		log.Printf("SetError failed for account %d: %v", account.ID, err)
		return
	}
	log.Printf("Account %d disabled due to auth error: %s", account.ID, errorMsg)
}

// handle429 处理429限流错误
// 解析响应头获取重置时间，标记账号为限流状态
func (s *RateLimitService) handle429(ctx context.Context, account *model.Account, headers http.Header) {
	// 解析重置时间戳
	resetTimestamp := headers.Get("anthropic-ratelimit-unified-reset")
	if resetTimestamp == "" {
		// 没有重置时间，使用默认5分钟
		resetAt := time.Now().Add(5 * time.Minute)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			log.Printf("SetRateLimited failed for account %d: %v", account.ID, err)
		}
		return
	}

	// 解析Unix时间戳
	ts, err := strconv.ParseInt(resetTimestamp, 10, 64)
	if err != nil {
		log.Printf("Parse reset timestamp failed: %v", err)
		resetAt := time.Now().Add(5 * time.Minute)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			log.Printf("SetRateLimited failed for account %d: %v", account.ID, err)
		}
		return
	}

	resetAt := time.Unix(ts, 0)

	// 标记限流状态
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		log.Printf("SetRateLimited failed for account %d: %v", account.ID, err)
		return
	}

	// 根据重置时间反推5h窗口
	windowEnd := resetAt
	windowStart := resetAt.Add(-5 * time.Hour)
	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, &windowStart, &windowEnd, "rejected"); err != nil {
		log.Printf("UpdateSessionWindow failed for account %d: %v", account.ID, err)
	}

	log.Printf("Account %d rate limited until %v", account.ID, resetAt)
}

// handle529 处理529过载错误
// 根据配置设置过载冷却时间
func (s *RateLimitService) handle529(ctx context.Context, account *model.Account) {
	cooldownMinutes := s.cfg.RateLimit.OverloadCooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = 10 // 默认10分钟
	}

	until := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)
	if err := s.accountRepo.SetOverloaded(ctx, account.ID, until); err != nil {
		log.Printf("SetOverloaded failed for account %d: %v", account.ID, err)
		return
	}

	log.Printf("Account %d overloaded until %v", account.ID, until)
}

// UpdateSessionWindow 从成功响应更新5h窗口状态
func (s *RateLimitService) UpdateSessionWindow(ctx context.Context, account *model.Account, headers http.Header) {
	status := headers.Get("anthropic-ratelimit-unified-5h-status")
	if status == "" {
		return
	}

	// 检查是否需要初始化时间窗口
	// 对于 Setup Token 账号，首次成功请求时需要预测时间窗口
	var windowStart, windowEnd *time.Time
	needInitWindow := account.SessionWindowEnd == nil || time.Now().After(*account.SessionWindowEnd)

	if needInitWindow && (status == "allowed" || status == "allowed_warning") {
		// 预测时间窗口：从当前时间的整点开始，+5小时为结束
		// 例如：现在是 14:30，窗口为 14:00 ~ 19:00
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		end := start.Add(5 * time.Hour)
		windowStart = &start
		windowEnd = &end
		log.Printf("Account %d: initializing 5h window from %v to %v (status: %s)", account.ID, start, end, status)
	}

	if err := s.accountRepo.UpdateSessionWindow(ctx, account.ID, windowStart, windowEnd, status); err != nil {
		log.Printf("UpdateSessionWindow failed for account %d: %v", account.ID, err)
	}

	// 如果状态为allowed且之前有限流，说明窗口已重置，清除限流状态
	if status == "allowed" && account.IsRateLimited() {
		if err := s.accountRepo.ClearRateLimit(ctx, account.ID); err != nil {
			log.Printf("ClearRateLimit failed for account %d: %v", account.ID, err)
		}
	}
}

// ClearRateLimit 清除账号的限流状态
func (s *RateLimitService) ClearRateLimit(ctx context.Context, accountID int64) error {
	return s.accountRepo.ClearRateLimit(ctx, accountID)
}
