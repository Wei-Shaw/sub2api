package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// MaybeCooldownSlowFirstToken 在请求完成后检查首 Token 延迟是否超过阈值，
// 若超过则自动将该账号置入临时冷却（temp_unschedulable），保护后续请求
// 不再选择该慢速账号。
//
// 调用时机：failover 循环结束、请求已成功返回、尚未提交使用记录之前。
// 该冷却不影响当前请求，仅影响后续调度。
//
// 与外部脚本 slow-account-kicker.sh 的语义一致，
// 但从轮询改为事件驱动（请求完成即判断），延迟更低。
func (s *OpenAIGatewayService) MaybeCooldownSlowFirstToken(ctx context.Context, accountID int64, firstTokenMs *int) {
	if s.cfg == nil {
		return
	}
	thresholdMs := s.cfg.Gateway.SlowFirstTokenCooldownMs
	if thresholdMs <= 0 {
		return
	}
	if firstTokenMs == nil {
		return
	}
	if *firstTokenMs <= thresholdMs {
		return
	}

	cooldownSeconds := s.cfg.Gateway.SlowFirstTokenCooldownSeconds
	if cooldownSeconds <= 0 {
		cooldownSeconds = 60
	}

	until := time.Now().Add(time.Duration(cooldownSeconds) * time.Second)
	reason := fmt.Sprintf("Auto-cooled: first_token_ms=%d > threshold=%dms", *firstTokenMs, thresholdMs)

	bgCtx := context.Background()
	if err := s.accountRepo.SetTempUnschedulable(bgCtx, accountID, until, reason); err != nil {
		logger.L().With(
			zap.String("component", "service.slow_first_token_cooldown"),
			zap.Int64("account_id", accountID),
		).Warn("slow_first_token_cooldown.set_temp_unschedulable_failed", zap.Error(err))
		return
	}

	logger.L().With(
		zap.String("component", "service.slow_first_token_cooldown"),
		zap.Int64("account_id", accountID),
		zap.Int("first_token_ms", *firstTokenMs),
		zap.Int("threshold_ms", thresholdMs),
		zap.Int("cooldown_seconds", cooldownSeconds),
		zap.Time("until", until),
	).Info("slow_first_token_cooldown.applied")
}
