package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// userPlatformQuotaServiceAdapter 将 repository 层的 userPlatformQuotaRepository
// 适配为 service.UserPlatformQuotaRepository 接口（返回 *service.UserPlatformQuotaRecord）。
type userPlatformQuotaServiceAdapter struct {
	inner *userPlatformQuotaRepository
}

// NewUserPlatformQuotaServiceAdapter 将 UserPlatformQuotaRepository 实现包装为
// 满足 service.UserPlatformQuotaRepository 接口的适配器。
func NewUserPlatformQuotaServiceAdapter(repo UserPlatformQuotaRepository) service.UserPlatformQuotaRepository {
	impl, ok := repo.(*userPlatformQuotaRepository)
	if !ok {
		// 非标准实现（如测试 fake），通过通用适配器包装
		return &genericUserPlatformQuotaAdapter{inner: repo}
	}
	return &userPlatformQuotaServiceAdapter{inner: impl}
}

func (a *userPlatformQuotaServiceAdapter) GetByUserPlatform(ctx context.Context, userID int64, platform string) (*service.UserPlatformQuotaRecord, error) {
	rec, err := a.inner.GetByUserPlatform(ctx, userID, platform)
	if err != nil || rec == nil {
		return nil, err
	}
	return toServiceRecord(rec), nil
}

// IncrementUsageWithReset 原子累加 cost 到 (user, platform) 三个窗口的用量。
func (a *userPlatformQuotaServiceAdapter) IncrementUsageWithReset(ctx context.Context, userID int64, platform string, cost float64, now time.Time) error {
	return a.inner.IncrementUsageWithReset(ctx, userID, platform, cost, now)
}

// ListByUser 查询用户的所有平台配额记录。
func (a *userPlatformQuotaServiceAdapter) ListByUser(ctx context.Context, userID int64) ([]service.UserPlatformQuotaRecord, error) {
	rows, err := a.inner.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]service.UserPlatformQuotaRecord, len(rows))
	for i, r := range rows {
		out[i] = service.UserPlatformQuotaRecord{
			UserID:             r.UserID,
			Platform:           r.Platform,
			DailyLimitUSD:      r.DailyLimitUSD,
			WeeklyLimitUSD:     r.WeeklyLimitUSD,
			MonthlyLimitUSD:    r.MonthlyLimitUSD,
			DailyUsageUSD:      r.DailyUsageUSD,
			WeeklyUsageUSD:     r.WeeklyUsageUSD,
			MonthlyUsageUSD:    r.MonthlyUsageUSD,
			DailyWindowStart:   r.DailyWindowStart,
			WeeklyWindowStart:  r.WeeklyWindowStart,
			MonthlyWindowStart: r.MonthlyWindowStart,
		}
	}
	return out, nil
}

// BulkInsertInitial 将 service.UserPlatformQuotaRecord 切片转换后调用底层 repo。
func (a *userPlatformQuotaServiceAdapter) BulkInsertInitial(ctx context.Context, records []service.UserPlatformQuotaRecord) error {
	repoRecords := make([]UserPlatformQuotaRecord, len(records))
	for i, r := range records {
		repoRecords[i] = UserPlatformQuotaRecord{
			UserID:          r.UserID,
			Platform:        r.Platform,
			DailyLimitUSD:   r.DailyLimitUSD,
			WeeklyLimitUSD:  r.WeeklyLimitUSD,
			MonthlyLimitUSD: r.MonthlyLimitUSD,
		}
	}
	return a.inner.BulkInsertInitial(ctx, repoRecords)
}

// UpsertForUser 全量替换该用户所有平台限额。
func (a *userPlatformQuotaServiceAdapter) UpsertForUser(ctx context.Context, userID int64, records []service.UserPlatformQuotaRecord) error {
	repoRecords := toRepoRecords(records)
	return a.inner.UpsertForUser(ctx, userID, repoRecords)
}

// ResetExpiredWindow 转发至 repository.ResetExpiredWindow，并将 repository sentinel 包装为 service sentinel。
func (a *userPlatformQuotaServiceAdapter) ResetExpiredWindow(ctx context.Context, userID int64, platform string, window string, newStart time.Time) error {
	err := a.inner.ResetExpiredWindow(ctx, userID, platform, window, newStart)
	if errors.Is(err, ErrUserPlatformQuotaNotFound) {
		return fmt.Errorf("%w: %w", service.ErrUserPlatformQuotaNotFound, err)
	}
	return err
}

// genericUserPlatformQuotaAdapter 通过通用接口适配（用于测试 fake 或非标准实现）。
type genericUserPlatformQuotaAdapter struct {
	inner UserPlatformQuotaRepository
}

func (a *genericUserPlatformQuotaAdapter) GetByUserPlatform(ctx context.Context, userID int64, platform string) (*service.UserPlatformQuotaRecord, error) {
	rec, err := a.inner.GetByUserPlatform(ctx, userID, platform)
	if err != nil || rec == nil {
		return nil, err
	}
	return toServiceRecord(rec), nil
}

// IncrementUsageWithReset 原子累加 cost（通用 adapter 实现）。
func (a *genericUserPlatformQuotaAdapter) IncrementUsageWithReset(ctx context.Context, userID int64, platform string, cost float64, now time.Time) error {
	return a.inner.IncrementUsageWithReset(ctx, userID, platform, cost, now)
}

// ListByUser 查询用户的所有平台配额记录（通用 adapter 实现）。
func (a *genericUserPlatformQuotaAdapter) ListByUser(ctx context.Context, userID int64) ([]service.UserPlatformQuotaRecord, error) {
	rows, err := a.inner.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]service.UserPlatformQuotaRecord, len(rows))
	for i, r := range rows {
		out[i] = service.UserPlatformQuotaRecord{
			UserID:             r.UserID,
			Platform:           r.Platform,
			DailyLimitUSD:      r.DailyLimitUSD,
			WeeklyLimitUSD:     r.WeeklyLimitUSD,
			MonthlyLimitUSD:    r.MonthlyLimitUSD,
			DailyUsageUSD:      r.DailyUsageUSD,
			WeeklyUsageUSD:     r.WeeklyUsageUSD,
			MonthlyUsageUSD:    r.MonthlyUsageUSD,
			DailyWindowStart:   r.DailyWindowStart,
			WeeklyWindowStart:  r.WeeklyWindowStart,
			MonthlyWindowStart: r.MonthlyWindowStart,
		}
	}
	return out, nil
}

// BulkInsertInitial 将 service.UserPlatformQuotaRecord 切片转换后调用底层 generic repo。
func (a *genericUserPlatformQuotaAdapter) BulkInsertInitial(ctx context.Context, records []service.UserPlatformQuotaRecord) error {
	repoRecords := make([]UserPlatformQuotaRecord, len(records))
	for i, r := range records {
		repoRecords[i] = UserPlatformQuotaRecord{
			UserID:          r.UserID,
			Platform:        r.Platform,
			DailyLimitUSD:   r.DailyLimitUSD,
			WeeklyLimitUSD:  r.WeeklyLimitUSD,
			MonthlyLimitUSD: r.MonthlyLimitUSD,
		}
	}
	return a.inner.BulkInsertInitial(ctx, repoRecords)
}

// UpsertForUser 全量替换（通用 adapter 实现）。
func (a *genericUserPlatformQuotaAdapter) UpsertForUser(ctx context.Context, userID int64, records []service.UserPlatformQuotaRecord) error {
	repoRecords := toRepoRecords(records)
	return a.inner.UpsertForUser(ctx, userID, repoRecords)
}

// ResetExpiredWindow 转发至 repository.ResetExpiredWindow（通用 adapter），并包装 sentinel。
func (a *genericUserPlatformQuotaAdapter) ResetExpiredWindow(ctx context.Context, userID int64, platform string, window string, newStart time.Time) error {
	err := a.inner.ResetExpiredWindow(ctx, userID, platform, window, newStart)
	if errors.Is(err, ErrUserPlatformQuotaNotFound) {
		return fmt.Errorf("%w: %w", service.ErrUserPlatformQuotaNotFound, err)
	}
	return err
}

// toServiceRecord 将 repository.UserPlatformQuotaRecord 转换为 service.UserPlatformQuotaRecord。
func toServiceRecord(rec *UserPlatformQuotaRecord) *service.UserPlatformQuotaRecord {
	return &service.UserPlatformQuotaRecord{
		UserID:             rec.UserID,
		Platform:           rec.Platform,
		DailyLimitUSD:      rec.DailyLimitUSD,
		WeeklyLimitUSD:     rec.WeeklyLimitUSD,
		MonthlyLimitUSD:    rec.MonthlyLimitUSD,
		DailyUsageUSD:      rec.DailyUsageUSD,
		WeeklyUsageUSD:     rec.WeeklyUsageUSD,
		MonthlyUsageUSD:    rec.MonthlyUsageUSD,
		DailyWindowStart:   rec.DailyWindowStart,
		WeeklyWindowStart:  rec.WeeklyWindowStart,
		MonthlyWindowStart: rec.MonthlyWindowStart,
	}
}

// toRepoRecords 将 service.UserPlatformQuotaRecord 切片转换为 repository.UserPlatformQuotaRecord（含 limit 字段，含 usage/window_start）。
func toRepoRecords(records []service.UserPlatformQuotaRecord) []UserPlatformQuotaRecord {
	out := make([]UserPlatformQuotaRecord, len(records))
	for i, r := range records {
		out[i] = UserPlatformQuotaRecord{
			UserID:             r.UserID,
			Platform:           r.Platform,
			DailyLimitUSD:      r.DailyLimitUSD,
			WeeklyLimitUSD:     r.WeeklyLimitUSD,
			MonthlyLimitUSD:    r.MonthlyLimitUSD,
			DailyUsageUSD:      r.DailyUsageUSD,
			WeeklyUsageUSD:     r.WeeklyUsageUSD,
			MonthlyUsageUSD:    r.MonthlyUsageUSD,
			DailyWindowStart:   r.DailyWindowStart,
			WeeklyWindowStart:  r.WeeklyWindowStart,
			MonthlyWindowStart: r.MonthlyWindowStart,
		}
	}
	return out
}
