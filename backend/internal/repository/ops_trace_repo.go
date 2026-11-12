package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 账号追溯落库（migrations/251）。
//
// 用原生 SQL 而不是 ent：同包的 ops 日志本来就全走 database/sql 的批量插入，
// 保持一致可以让这两张表复用同一批连接池设置与超时语义，也避免把 ent.Client
// 再往 wire 里穿一层。
//
// 批量形状：单条 INSERT 多 VALUES，按列宽固定 200 行/语句分块。两张表都只增不改，
// 所以不需要 upsert；写入失败只上抛错误，由调用方降级为日志。

const opsTraceInsertChunk = 200

const opsInsertUpstreamAttemptsPrefix = `INSERT INTO upstream_attempts (
	request_id, attempt_no, account_id, retry_from, platform, model_req, model_upstream,
	upstream_url, upstream_status, failure_kind, error_code, error_msg, latency_ms,
	account_state, cooling_until, credential_fp, proxy_id, created_at
) VALUES `

const opsInsertAccountStatusEventsPrefix = `INSERT INTO account_status_events (
	account_id, from_state, to_state, reason, error_code, cooling_until,
	source, request_id, operator_id, created_at
) VALUES `

// InsertUpstreamAttempts 批量写入上游尝试记录。
func (r *opsRepository) InsertUpstreamAttempts(ctx context.Context, records []*service.OpsUpstreamAttemptRecord) (int, error) {
	if r == nil || r.db == nil || len(records) == 0 {
		return 0, nil
	}

	inserted := 0
	for start := 0; start < len(records); start += opsTraceInsertChunk {
		end := start + opsTraceInsertChunk
		if end > len(records) {
			end = len(records)
		}
		chunk := records[start:end]
		if err := r.insertAttemptChunk(ctx, chunk); err != nil {
			return inserted, err
		}
		inserted += len(chunk)
	}
	return inserted, nil
}

func (r *opsRepository) insertAttemptChunk(ctx context.Context, chunk []*service.OpsUpstreamAttemptRecord) error {
	var sb strings.Builder
	sb.WriteString(opsInsertUpstreamAttemptsPrefix)

	args := make([]any, 0, len(chunk)*18)
	for i, rec := range chunk {
		if rec == nil {
			continue
		}
		if i > 0 {
			sb.WriteString(",")
		}
		base := len(args)
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9,
			base+10, base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18))

		failureKind := strings.TrimSpace(rec.FailureKind)
		if failureKind == "" {
			failureKind = "none"
		}
		attemptNo := rec.AttemptNo
		if attemptNo <= 0 {
			attemptNo = 1
		}
		platform := strings.TrimSpace(rec.Platform)
		if platform == "" {
			platform = "unknown"
		}

		args = append(args,
			rec.RequestID,
			attemptNo,
			opsNullInt64(rec.AccountID),
			opsNullInt64(rec.RetryFrom),
			platform,
			rec.ModelReq,
			opsNullString(rec.ModelUpstream),
			opsNullString(rec.UpstreamURL),
			opsNullInt(rec.UpstreamStatus),
			failureKind,
			opsNullString(rec.ErrorCode),
			opsNullString(rec.ErrorMsg),
			opsNullInt(rec.LatencyMs),
			opsNullString(rec.AccountState),
			opsNullTime(rec.CoolingUntil),
			opsNullString(rec.CredentialFP),
			opsNullInt64(rec.ProxyID),
			rec.CreatedAt,
		)
	}

	if len(args) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return fmt.Errorf("insert upstream attempts: %w", err)
	}
	return nil
}

// InsertAccountStatusEvents 批量写入账号状态变更事件。
func (r *opsRepository) InsertAccountStatusEvents(ctx context.Context, records []*service.OpsAccountStatusEventRecord) (int, error) {
	if r == nil || r.db == nil || len(records) == 0 {
		return 0, nil
	}

	inserted := 0
	for start := 0; start < len(records); start += opsTraceInsertChunk {
		end := start + opsTraceInsertChunk
		if end > len(records) {
			end = len(records)
		}
		chunk := records[start:end]
		if err := r.insertStatusEventChunk(ctx, chunk); err != nil {
			return inserted, err
		}
		inserted += len(chunk)
	}
	return inserted, nil
}

func (r *opsRepository) insertStatusEventChunk(ctx context.Context, chunk []*service.OpsAccountStatusEventRecord) error {
	var sb strings.Builder
	sb.WriteString(opsInsertAccountStatusEventsPrefix)

	args := make([]any, 0, len(chunk)*10)
	row := 0
	for _, rec := range chunk {
		if rec == nil || rec.AccountID <= 0 {
			continue
		}
		if row > 0 {
			sb.WriteString(",")
		}
		row++
		base := len(args)
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10))

		toState := strings.TrimSpace(rec.ToState)
		if toState == "" {
			toState = "invalid"
		}
		source := strings.TrimSpace(rec.Source)
		if source == "" {
			source = "classify"
		}

		args = append(args,
			rec.AccountID,
			opsNullString(rec.FromState),
			toState,
			opsNullString(rec.Reason),
			opsNullString(rec.ErrorCode),
			opsNullTime(rec.CoolingUntil),
			source,
			opsNullString(rec.RequestID),
			opsNullInt64(rec.OperatorID),
			rec.CreatedAt,
		)
	}

	if len(args) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return fmt.Errorf("insert account status events: %w", err)
	}
	return nil
}
