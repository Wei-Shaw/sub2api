package service

import (
	"context"
	"encoding/json"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 账号维度的追溯与台账：
//   - GetAccountTrace     按账号 + 时间窗看「这个号最近怎么了」（attempts / 调度态事件 / 台账）
//   - GetAccountLedger    按账号看完整订阅台账（开通、续费、到期、凭据/套餐/额度变化）
//   - ListExpiringAccounts 看即将到期 / 已到期的账号，用于续费巡检
//
// 台账由 migrations/252 的数据库触发器写入，覆盖所有写 accounts 的路径。

const (
	accountTraceDefaultHours = 24
	accountTraceMaxHours     = 24 * 7
	accountTraceDefaultLimit = 50
	accountTraceMaxLimit     = 500

	accountLedgerDefaultLimit = 100

	expiringAccountsDefaultDays = 7
	expiringAccountsMaxDays     = 90
	expiringAccountsDefaultLim  = 100

	// 失败率告警的最小样本量：避免「1 次失败 = 100%」误报。
	accountFailureRateMinAttempts = 5
)

// OpsAccountLifecycleEvent 是 account_lifecycle_events 的一行。
type OpsAccountLifecycleEvent struct {
	ID                 int64           `json:"id"`
	AccountID          int64           `json:"account_id"`
	EventType          string          `json:"event_type"`
	Changes            json.RawMessage `json:"changes"`
	StatusAfter        *string         `json:"status_after,omitempty"`
	ExpiresAtBefore    *time.Time      `json:"expires_at_before,omitempty"`
	ExpiresAtAfter     *time.Time      `json:"expires_at_after,omitempty"`
	CredentialFPBefore *string         `json:"credential_fp_before,omitempty"`
	CredentialFPAfter  *string         `json:"credential_fp_after,omitempty"`
	Source             string          `json:"source"`
	OperatorID         *int64          `json:"operator_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

// OpsAccountTrace 是按账号反查的结果。
type OpsAccountTrace struct {
	AccountID       int64                       `json:"account_id"`
	Since           time.Time                   `json:"since"`
	Attempts        []*OpsTraceAttempt          `json:"attempts"`
	StatusEvents    []*OpsTraceStatusEvent      `json:"status_events"`
	LifecycleEvents []*OpsAccountLifecycleEvent `json:"lifecycle_events"`
}

// OpsAccountLedger 是一个账号的完整订阅台账。
type OpsAccountLedger struct {
	AccountID int64                       `json:"account_id"`
	Events    []*OpsAccountLifecycleEvent `json:"events"`
}

// OpsExpiringAccount 是到期巡检列表的一行。
type OpsExpiringAccount struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Platform           string    `json:"platform"`
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Schedulable        bool      `json:"schedulable"`
	ExpiresAt          time.Time `json:"expires_at"`
	AutoPauseOnExpired bool      `json:"auto_pause_on_expired"`
	Expired            bool      `json:"expired"`
	HoursLeft          float64   `json:"hours_left"`
}

// AccountAttemptStat 是一个账号在时间窗内的失败率统计。
//
// 分母 = 失败尝试（upstream_attempts）+ 成功请求（usage_logs，全量）。
// 不能只看 upstream_attempts：失败全量落库而成功只有采样，单看它失败率恒接近 100%。
type AccountAttemptStat struct {
	AccountID  int64   `json:"account_id"`
	Attempts   int64   `json:"attempts"`
	Failures   int64   `json:"failures"`
	FailurePct float64 `json:"failure_pct"`
	TopKind    string  `json:"top_kind"`
}

func clampInt(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

// GetAccountTrace 按账号返回时间窗内的追溯数据。空结果返回空数组而不是 404：
// 「这个号最近没出事」本身就是有效答案。
func (s *OpsService) GetAccountTrace(ctx context.Context, accountID int64, hours, limit int) (*OpsAccountTrace, error) {
	if accountID <= 0 {
		return nil, infraerrors.BadRequest("TRACE_ACCOUNT_ID_INVALID", "account id must be positive")
	}
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	hours = clampInt(hours, accountTraceDefaultHours, accountTraceMaxHours)
	limit = clampInt(limit, accountTraceDefaultLimit, accountTraceMaxLimit)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	empty := &OpsAccountTrace{
		AccountID:       accountID,
		Since:           since,
		Attempts:        []*OpsTraceAttempt{},
		StatusEvents:    []*OpsTraceStatusEvent{},
		LifecycleEvents: []*OpsAccountLifecycleEvent{},
	}
	if s.opsRepo == nil {
		return empty, nil
	}
	trace, err := s.opsRepo.GetAccountTrace(ctx, accountID, since, limit)
	if err != nil {
		return nil, infraerrors.InternalServer("TRACE_LOAD_FAILED", "Failed to load account trace").WithCause(err)
	}
	if trace == nil {
		return empty, nil
	}
	trace.AccountID = accountID
	trace.Since = since
	if trace.Attempts == nil {
		trace.Attempts = []*OpsTraceAttempt{}
	}
	if trace.StatusEvents == nil {
		trace.StatusEvents = []*OpsTraceStatusEvent{}
	}
	if trace.LifecycleEvents == nil {
		trace.LifecycleEvents = []*OpsAccountLifecycleEvent{}
	}
	return trace, nil
}

// GetAccountLedger 返回账号的完整订阅台账（新 → 旧）。台账不随 ops 清理删除。
func (s *OpsService) GetAccountLedger(ctx context.Context, accountID int64, limit int) (*OpsAccountLedger, error) {
	if accountID <= 0 {
		return nil, infraerrors.BadRequest("TRACE_ACCOUNT_ID_INVALID", "account id must be positive")
	}
	limit = clampInt(limit, accountLedgerDefaultLimit, accountTraceMaxLimit)
	out := &OpsAccountLedger{AccountID: accountID, Events: []*OpsAccountLifecycleEvent{}}
	if s.opsRepo == nil {
		return out, nil
	}
	events, err := s.opsRepo.ListAccountLifecycleEvents(ctx, accountID, time.Time{}, limit)
	if err != nil {
		return nil, infraerrors.InternalServer("LEDGER_LOAD_FAILED", "Failed to load account ledger").WithCause(err)
	}
	if events != nil {
		out.Events = events
	}
	return out, nil
}

// ListExpiringAccounts 返回 days 天内到期（含已到期）的账号，按到期时间升序。
func (s *OpsService) ListExpiringAccounts(ctx context.Context, days, limit int) ([]*OpsExpiringAccount, error) {
	days = clampInt(days, expiringAccountsDefaultDays, expiringAccountsMaxDays)
	limit = clampInt(limit, expiringAccountsDefaultLim, accountTraceMaxLimit)
	if s.opsRepo == nil {
		return []*OpsExpiringAccount{}, nil
	}
	now := time.Now()
	items, err := s.opsRepo.ListExpiringAccounts(ctx, now.Add(time.Duration(days)*24*time.Hour), limit)
	if err != nil {
		return nil, infraerrors.InternalServer("EXPIRING_LOAD_FAILED", "Failed to load expiring accounts").WithCause(err)
	}
	if items == nil {
		return []*OpsExpiringAccount{}, nil
	}
	for _, it := range items {
		if it == nil {
			continue
		}
		left := it.ExpiresAt.Sub(now).Hours()
		it.Expired = left <= 0
		it.HoursLeft = float64(int64(left*10)) / 10
	}
	return items, nil
}
