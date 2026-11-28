package service

import (
	"context"
	"log/slog"
	"time"
)

// resumeSchedulingAfterRecovery 在观测到恢复信号后，恢复被系统因错误自动暂停
// （带 scheduling_auto_disabled 标记）的账号调度。手动暂停的账号无标记，此调用
// 对其为无操作，不会违背管理员意图。signal 仅用于日志区分触发来源。
func resumeSchedulingAfterRecovery(ctx context.Context, repo AccountRepository, accountID int64, signal string) {
	resumeRepo, ok := repo.(SchedulingAutoResumeRepository)
	if !ok || resumeRepo == nil {
		return
	}
	resumed, err := resumeRepo.ResumeSchedulableIfAutoDisabled(ctx, accountID, time.Now())
	if err != nil {
		slog.Warn("account_scheduling_auto_resume_failed",
			"account_id", accountID,
			"signal", signal,
			"error", err,
		)
		return
	}
	if resumed {
		slog.Info("account_scheduling_auto_resumed",
			"account_id", accountID,
			"signal", signal,
		)
	}
}
