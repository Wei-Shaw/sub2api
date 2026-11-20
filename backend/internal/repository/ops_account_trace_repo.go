package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 账号维度追溯 / 台账 / 到期巡检 / 失败率统计的查询。
// 扫描逻辑与按 request_id 反查共用（queryTraceAttempts / queryTraceStatusEvents）。

const traceAttemptColumns = `attempt_no, account_id, retry_from, platform, model_req, model_upstream,
		       upstream_url, upstream_status, failure_kind, error_code, error_msg,
		       account_state, cooling_until, credential_fp, proxy_id, request_id, created_at`

const traceStatusEventColumns = `account_id, from_state, to_state, reason, error_code, cooling_until,
		       source, request_id, created_at`

const traceLifecycleColumns = `id, account_id, event_type, changes, status_after,
		       expires_at_before, expires_at_after, credential_fp_before, credential_fp_after,
		       source, operator_id, created_at`

type traceQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryTraceAttempts(ctx context.Context, db traceQuerier, query string, args ...any) ([]*service.OpsTraceAttempt, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load trace attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsTraceAttempt, 0)
	for rows.Next() {
		var (
			item         service.OpsTraceAttempt
			accountID    sql.NullInt64
			retryFrom    sql.NullInt64
			modelUp      sql.NullString
			upstreamURL  sql.NullString
			upstreamCode sql.NullInt64
			errorCode    sql.NullString
			errorMsg     sql.NullString
			accountState sql.NullString
			coolingUntil sql.NullTime
			credentialFP sql.NullString
			proxyID      sql.NullInt64
		)
		if err := rows.Scan(
			&item.AttemptNo, &accountID, &retryFrom, &item.Platform, &item.ModelReq, &modelUp,
			&upstreamURL, &upstreamCode, &item.FailureKind, &errorCode, &errorMsg,
			&accountState, &coolingUntil, &credentialFP, &proxyID, &item.RequestID, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trace attempt: %w", err)
		}
		item.AccountID = traceNullInt64Ptr(accountID)
		item.RetryFrom = traceNullInt64Ptr(retryFrom)
		item.ModelUpstream = traceNullStringPtr(modelUp)
		item.UpstreamURL = traceNullStringPtr(upstreamURL)
		item.UpstreamStatus = traceNullInt64ToIntPtr(upstreamCode)
		item.ErrorCode = traceNullStringPtr(errorCode)
		item.ErrorMsg = traceNullStringPtr(errorMsg)
		item.AccountState = traceNullStringPtr(accountState)
		item.CoolingUntil = traceNullTimePtr(coolingUntil)
		item.CredentialFP = traceNullStringPtr(credentialFP)
		item.ProxyID = traceNullInt64Ptr(proxyID)
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace attempts: %w", err)
	}
	return out, nil
}

func queryTraceStatusEvents(ctx context.Context, db traceQuerier, query string, args ...any) ([]*service.OpsTraceStatusEvent, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load trace status events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsTraceStatusEvent, 0)
	for rows.Next() {
		var (
			item         service.OpsTraceStatusEvent
			fromState    sql.NullString
			reason       sql.NullString
			errorCode    sql.NullString
			coolingUntil sql.NullTime
			requestID    sql.NullString
		)
		if err := rows.Scan(
			&item.AccountID, &fromState, &item.ToState, &reason, &errorCode,
			&coolingUntil, &item.Source, &requestID, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trace status event: %w", err)
		}
		item.FromState = traceNullStringPtr(fromState)
		item.Reason = traceNullStringPtr(reason)
		item.ErrorCode = traceNullStringPtr(errorCode)
		item.CoolingUntil = traceNullTimePtr(coolingUntil)
		item.RequestID = traceNullStringPtr(requestID)
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace status events: %w", err)
	}
	return out, nil
}

func queryLifecycleEvents(ctx context.Context, db traceQuerier, query string, args ...any) ([]*service.OpsAccountLifecycleEvent, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load lifecycle events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsAccountLifecycleEvent, 0)
	for rows.Next() {
		var (
			item        service.OpsAccountLifecycleEvent
			changes     []byte
			statusAfter sql.NullString
			expBefore   sql.NullTime
			expAfter    sql.NullTime
			fpBefore    sql.NullString
			fpAfter     sql.NullString
			operatorID  sql.NullInt64
		)
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.EventType, &changes, &statusAfter,
			&expBefore, &expAfter, &fpBefore, &fpAfter,
			&item.Source, &operatorID, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan lifecycle event: %w", err)
		}
		if len(changes) == 0 {
			changes = []byte("{}")
		}
		item.Changes = changes
		item.StatusAfter = traceNullStringPtr(statusAfter)
		item.ExpiresAtBefore = traceNullTimePtr(expBefore)
		item.ExpiresAtAfter = traceNullTimePtr(expAfter)
		item.CredentialFPBefore = traceNullStringPtr(fpBefore)
		item.CredentialFPAfter = traceNullStringPtr(fpAfter)
		item.OperatorID = traceNullInt64Ptr(operatorID)
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle events: %w", err)
	}
	return out, nil
}

// GetAccountTrace 按账号聚合时间窗内的 attempts / 状态事件 / 台账。
func (r *opsRepository) GetAccountTrace(ctx context.Context, accountID int64, since time.Time, limit int) (*service.OpsAccountTrace, error) {
	const attemptsQuery = `
		SELECT ` + traceAttemptColumns + `
		FROM upstream_attempts
		WHERE account_id = $1 AND created_at >= $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`
	const statusQuery = `
		SELECT ` + traceStatusEventColumns + `
		FROM account_status_events
		WHERE account_id = $1 AND created_at >= $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`

	attempts, err := queryTraceAttempts(ctx, r.db, attemptsQuery, accountID, since, limit)
	if err != nil {
		return nil, err
	}
	statusEvents, err := queryTraceStatusEvents(ctx, r.db, statusQuery, accountID, since, limit)
	if err != nil {
		return nil, err
	}
	lifecycle, err := r.ListAccountLifecycleEvents(ctx, accountID, since, limit)
	if err != nil {
		return nil, err
	}
	return &service.OpsAccountTrace{
		AccountID:       accountID,
		Since:           since,
		Attempts:        attempts,
		StatusEvents:    statusEvents,
		LifecycleEvents: lifecycle,
	}, nil
}

// ListAccountLifecycleEvents 返回账号台账（新 → 旧）。since 为零值表示不限时间。
func (r *opsRepository) ListAccountLifecycleEvents(ctx context.Context, accountID int64, since time.Time, limit int) ([]*service.OpsAccountLifecycleEvent, error) {
	const query = `
		SELECT ` + traceLifecycleColumns + `
		FROM account_lifecycle_events
		WHERE account_id = $1 AND created_at >= $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`
	if since.IsZero() {
		since = time.Unix(0, 0)
	}
	return queryLifecycleEvents(ctx, r.db, query, accountID, since, limit)
}

// ListExpiringAccounts 返回 before 之前到期（含已到期）且未删除的账号。
func (r *opsRepository) ListExpiringAccounts(ctx context.Context, before time.Time, limit int) ([]*service.OpsExpiringAccount, error) {
	const query = `
		SELECT id, name, platform, type, status, schedulable, expires_at,
		       COALESCE(auto_pause_on_expired, FALSE)
		FROM accounts
		WHERE deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at <= $1
		ORDER BY expires_at ASC, id ASC
		LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, before, limit)
	if err != nil {
		return nil, fmt.Errorf("load expiring accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsExpiringAccount, 0)
	for rows.Next() {
		var item service.OpsExpiringAccount
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Platform, &item.Type, &item.Status,
			&item.Schedulable, &item.ExpiresAt, &item.AutoPauseOnExpired,
		); err != nil {
			return nil, fmt.Errorf("scan expiring account: %w", err)
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expiring accounts: %w", err)
	}
	return out, nil
}

// GetAccountAttemptStats 统计 since 以来每个账号的失败率，最差的排在前面。
//
// 失败来自 upstream_attempts（failure_kind <> 'none'），成功来自 usage_logs（全量）。
// 只返回至少有一次失败、且样本量 >= minAttempts 的账号。
func (r *opsRepository) GetAccountAttemptStats(ctx context.Context, since time.Time, minAttempts int) ([]*service.AccountAttemptStat, error) {
	const query = `
		WITH f AS (
			SELECT account_id,
			       count(*) AS failures,
			       mode() WITHIN GROUP (ORDER BY failure_kind) AS top_kind
			FROM upstream_attempts
			WHERE created_at >= $1 AND account_id IS NOT NULL AND failure_kind <> 'none'
			GROUP BY account_id
		), s AS (
			SELECT account_id, count(*) AS successes
			FROM usage_logs
			WHERE created_at >= $1 AND account_id IN (SELECT account_id FROM f)
			GROUP BY account_id
		)
		SELECT f.account_id,
		       f.failures + COALESCE(s.successes, 0) AS attempts,
		       f.failures,
		       100.0 * f.failures / NULLIF(f.failures + COALESCE(s.successes, 0), 0) AS failure_pct,
		       COALESCE(f.top_kind, '') AS top_kind
		FROM f LEFT JOIN s USING (account_id)
		WHERE f.failures + COALESCE(s.successes, 0) >= $2
		ORDER BY failure_pct DESC, attempts DESC
		LIMIT 50`

	rows, err := r.db.QueryContext(ctx, query, since, minAttempts)
	if err != nil {
		return nil, fmt.Errorf("load account attempt stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.AccountAttemptStat, 0)
	for rows.Next() {
		var item service.AccountAttemptStat
		if err := rows.Scan(&item.AccountID, &item.Attempts, &item.Failures, &item.FailurePct, &item.TopKind); err != nil {
			return nil, fmt.Errorf("scan account attempt stat: %w", err)
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account attempt stats: %w", err)
	}
	return out, nil
}
