package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	promptAuditAdmissionLockKey int64 = 579147893221901921
	promptAuditConfigLockKey    int64 = 579147893221901922
)

var (
	ErrQueueFull          = errors.New("prompt audit queue full")
	ErrQueueAdmissionBusy = errors.New("prompt audit queue admission busy")
	ErrLeaseLost          = errors.New("prompt audit worker lease lost")
	ErrEventNotFound      = errors.New("prompt audit event not found")
)

type Job struct {
	ID                  int64
	Snapshot            PromptSnapshot
	ExecutionMode       Mode
	ConfigVersion       int64
	Status              string
	Attempts            int
	MaxAttempts         int
	ClaimVersion        int64
	NextAttemptAt       time.Time
	ProcessingStartedAt *time.Time
	ProcessedAt         *time.Time
	LastErrorCode       string
	LastErrorMessage    string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Event struct {
	ID              int64              `json:"id"`
	JobID           int64              `json:"job_id"`
	Snapshot        PromptSnapshot     `json:"snapshot"`
	Decision        EventDecision      `json:"decision"`
	RiskLevel       RiskLevel          `json:"risk_level"`
	Action          Action             `json:"action"`
	Categories      []string           `json:"categories"`
	MatchedScanners []string           `json:"matched_scanners"`
	ScannerScores   map[string]float64 `json:"scanner_scores"`
	ScannerEvidence map[string]string  `json:"scanner_evidence"`
	ScannerBackend  string             `json:"scanner_backend"`
	ScannerVersion  string             `json:"scanner_version"`
	GuardEndpointID string             `json:"guard_endpoint_id"`
	PolicyID        string             `json:"policy_id"`
	PolicyVersion   int                `json:"policy_version"`
	ConfigVersion   int64              `json:"config_version"`
	ChunkTotal      int                `json:"chunk_total"`
	LatencyMS       int                `json:"latency_ms"`
	IssueSummaries  []IssueSummary     `json:"issue_summaries"`
	CreatedAt       time.Time          `json:"created_at"`
}

type JobRepository interface {
	CreateStagingWithCapacity(ctx context.Context, snapshot PromptSnapshot, configVersion int64, maxAttempts, capacity int) (*Job, error)
	PublishQueued(ctx context.Context, jobID int64) error
	MarkStagingFailed(ctx context.Context, jobID int64, code, message string) error
	ClaimNextJob(ctx context.Context, now time.Time) (*Job, bool, error)
	RefreshLease(ctx context.Context, jobID, claimVersion int64, now time.Time) error
	Complete(ctx context.Context, job *Job, result *NormalizedResult, storePassEvents bool) (*Event, error)
	Retry(ctx context.Context, jobID, claimVersion int64, next time.Time, code, message string) error
	Fail(ctx context.Context, jobID, claimVersion int64, code, message string) error
	ReclaimStale(ctx context.Context, stagingBefore, processingBefore time.Time, limit int) (int64, error)
	QueueStats(ctx context.Context) (QueueStats, error)
	RecordBlocking(ctx context.Context, snapshot PromptSnapshot, configVersion int64, result *NormalizedResult, storePassEvents bool) (*Event, error)
}

type PostgreSQLRepository struct {
	db    *sql.DB
	clock Clock
}

func NewPostgreSQLRepository(db *sql.DB) *PostgreSQLRepository {
	return &PostgreSQLRepository{db: db, clock: realClock{}}
}

func (r *PostgreSQLRepository) CreateStagingWithCapacity(ctx context.Context, snapshot PromptSnapshot, configVersion int64, maxAttempts, capacity int) (*Job, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("prompt audit database unavailable")
	}
	if r.isMySQLDB() {
		return r.createStagingWithCapacityMySQL(ctx, snapshot, configVersion, maxAttempts, capacity)
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, promptAuditAdmissionLockKey).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrQueueAdmissionBusy
	}
	var active int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM prompt_audit_jobs
		WHERE status IN ('staging','queued','processing','retry')`).Scan(&active); err != nil {
		return nil, err
	}
	if capacity <= 0 || active >= capacity {
		return nil, ErrQueueFull
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	job, err := r.insertJobInTx(ctx, tx, snapshot.Redacted(), ModeAsync, configVersion, "staging", maxAttempts)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

func (r *PostgreSQLRepository) PublishQueued(ctx context.Context, jobID int64) error {
	if r.isMySQLDB() {
		return r.publishQueuedMySQL(ctx, jobID)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET status='queued', next_attempt_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status='staging'`, jobID)
	return requireOneRow(result, err, ErrLeaseLost)
}

func (r *PostgreSQLRepository) MarkStagingFailed(ctx context.Context, jobID int64, code, _ string) error {
	if r.isMySQLDB() {
		return r.markStagingFailedMySQL(ctx, jobID, code)
	}
	code, message := sanitizeStoredError(code)
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs
		SET status='failed', processed_at=NOW(), updated_at=NOW(), last_error_code=$2, last_error_message=$3
		WHERE id=$1 AND status='staging'`, jobID, code, message)
	return requireOneRow(result, err, ErrLeaseLost)
}

func (r *PostgreSQLRepository) ClaimNextJob(ctx context.Context, now time.Time) (*Job, bool, error) {
	if r.isMySQLDB() {
		return r.claimNextJobMySQL(ctx, now)
	}
	row := r.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM prompt_audit_jobs
			WHERE status IN ('queued','retry') AND next_attempt_at <= $1
			ORDER BY next_attempt_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE prompt_audit_jobs AS j
		SET status='processing', attempts=j.attempts+1, claim_version=j.claim_version+1,
			processing_started_at=$1, updated_at=$1
		FROM candidate
		WHERE j.id=candidate.id
		RETURNING `+jobColumns("j"), now.UTC())
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return job, err == nil, err
}

func (r *PostgreSQLRepository) RefreshLease(ctx context.Context, jobID, claimVersion int64, now time.Time) error {
	if r.isMySQLDB() {
		return r.refreshLeaseMySQL(ctx, jobID, claimVersion, now)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET processing_started_at=$3, updated_at=$3
		WHERE id=$1 AND status='processing' AND claim_version=$2`, jobID, claimVersion, now.UTC())
	return requireOneRow(result, err, ErrLeaseLost)
}

func (r *PostgreSQLRepository) Complete(ctx context.Context, job *Job, result *NormalizedResult, storePassEvents bool) (*Event, error) {
	if job == nil || result == nil {
		return nil, errors.New("prompt audit completion requires job and result")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// MySQL 分支仅替换语句文本（$n→?、NOW()→NOW(3)），事务编排与 PG 版共用。
	updateSQL := `
		UPDATE prompt_audit_jobs SET status='done', processed_at=NOW(), updated_at=NOW(),
			last_error_code='', last_error_message=''
		WHERE id=$1 AND status='processing' AND claim_version=$2`
	if r.isMySQLDB() {
		updateSQL = `
		UPDATE prompt_audit_jobs SET status='done', processed_at=NOW(3), updated_at=NOW(3),
			last_error_code='', last_error_message=''
		WHERE id=? AND status='processing' AND claim_version=?`
	}
	updateResult, err := tx.ExecContext(ctx, updateSQL, job.ID, job.ClaimVersion)
	if err := requireOneRow(updateResult, err, ErrLeaseLost); err != nil {
		return nil, err
	}
	var event *Event
	if shouldStorePromptAuditEvent(result.Decision, storePassEvents) {
		event, err = r.insertEventInTx(ctx, tx, job.ID, job.Snapshot.Redacted(), job.ConfigVersion, result)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return event, nil
}

func (r *PostgreSQLRepository) Retry(ctx context.Context, jobID, claimVersion int64, next time.Time, code, _ string) error {
	if r.isMySQLDB() {
		return r.retryMySQL(ctx, jobID, claimVersion, next, code)
	}
	code, message := sanitizeStoredError(code)
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET status='retry', next_attempt_at=$3, processing_started_at=NULL,
			updated_at=NOW(), last_error_code=$4, last_error_message=$5
		WHERE id=$1 AND status='processing' AND claim_version=$2`,
		jobID, claimVersion, next.UTC(), code, message)
	return requireOneRow(result, err, ErrLeaseLost)
}

func (r *PostgreSQLRepository) Fail(ctx context.Context, jobID, claimVersion int64, code, _ string) error {
	if r.isMySQLDB() {
		return r.failMySQL(ctx, jobID, claimVersion, code)
	}
	code, message := sanitizeStoredError(code)
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET status='failed', processed_at=NOW(), processing_started_at=NULL,
			updated_at=NOW(), last_error_code=$3, last_error_message=$4
		WHERE id=$1 AND status='processing' AND claim_version=$2`,
		jobID, claimVersion, code, message)
	return requireOneRow(result, err, ErrLeaseLost)
}

func (r *PostgreSQLRepository) ReclaimStale(ctx context.Context, stagingBefore, processingBefore time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if r.isMySQLDB() {
		return r.reclaimStaleMySQL(ctx, stagingBefore, processingBefore, limit)
	}
	result, err := r.db.ExecContext(ctx, `
		WITH stale AS (
			SELECT id FROM prompt_audit_jobs
			WHERE (status='staging' AND updated_at < $1)
			   OR (status='processing' AND processing_started_at < $2)
			ORDER BY updated_at, id FOR UPDATE SKIP LOCKED LIMIT $3
		)
		UPDATE prompt_audit_jobs AS j
		SET status=CASE
			WHEN j.status='staging' THEN 'failed'
			WHEN j.attempts < j.max_attempts THEN 'retry'
			ELSE 'failed' END,
			next_attempt_at=CASE WHEN j.status='processing' AND j.attempts < j.max_attempts THEN NOW() ELSE j.next_attempt_at END,
			processing_started_at=NULL,
			processed_at=CASE WHEN j.status='staging' OR j.attempts >= j.max_attempts THEN NOW() ELSE NULL END,
			last_error_code=CASE WHEN j.status='staging' THEN 'staging_timeout' ELSE 'processing_lease_expired' END,
			last_error_message='', updated_at=NOW()
		FROM stale WHERE j.id=stale.id`, stagingBefore.UTC(), processingBefore.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PostgreSQLRepository) QueueStats(ctx context.Context) (QueueStats, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM prompt_audit_jobs GROUP BY status`)
	if err != nil {
		return QueueStats{}, err
	}
	defer func() { _ = rows.Close() }()
	var stats QueueStats
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return QueueStats{}, err
		}
		switch status {
		case "staging":
			stats.Staging = count
		case "queued":
			stats.Queued = count
		case "processing":
			stats.Processing = count
		case "retry":
			stats.Retry = count
		case "done":
			stats.Done = count
		case "failed":
			stats.Failed = count
		}
	}
	stats.Active = stats.Staging + stats.Queued + stats.Processing + stats.Retry
	return stats, rows.Err()
}

func (r *PostgreSQLRepository) RecordBlocking(ctx context.Context, snapshot PromptSnapshot, configVersion int64, result *NormalizedResult, storePassEvents bool) (*Event, error) {
	if result == nil {
		return nil, errors.New("prompt guard result required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := r.insertJobInTx(ctx, tx, snapshot.Redacted(), ModeBlocking, configVersion, "done", 1)
	if err != nil {
		return nil, err
	}
	var event *Event
	if shouldStorePromptAuditEvent(result.Decision, storePassEvents) {
		event, err = r.insertEventInTx(ctx, tx, job.ID, snapshot.Redacted(), configVersion, result)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return event, nil
}

// shouldStorePromptAuditEvent keeps store_pass_events scoped to safe results.
// Risk events are always persisted while prompt auditing itself is enabled.
func shouldStorePromptAuditEvent(decision EventDecision, storePassEvents bool) bool {
	return decision != EventPass || storePassEvents
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func insertJob(ctx context.Context, queryer sqlQueryer, snapshot PromptSnapshot, mode Mode, configVersion int64, status string, maxAttempts int) (*Job, error) {
	processedExpr := "NULL"
	if status == "done" || status == "failed" {
		processedExpr = "NOW()"
	}
	row := queryer.QueryRowContext(ctx, `
		INSERT INTO prompt_audit_jobs (
			request_id,user_id,username_snapshot,user_email_snapshot,api_key_id,api_key_name_snapshot,
			group_id,group_name,provider,endpoint,protocol,model,prompt_hash,redacted_preview,
			prompt_length,message_count,stage,execution_mode,config_version,status,max_attempts,processed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,`+processedExpr+`)
		RETURNING `+jobColumns("prompt_audit_jobs"),
		snapshot.RequestID, nullableID(snapshot.UserID), snapshot.UsernameSnapshot, snapshot.UserEmailSnapshot,
		nullableID(snapshot.APIKeyID), snapshot.APIKeyNameSnapshot, snapshot.GroupID, snapshot.GroupName,
		snapshot.Provider, snapshot.Endpoint, snapshot.Protocol, snapshot.Model, snapshot.PromptHash,
		snapshot.RedactedPreview, snapshot.PromptLength, snapshot.MessageCount, normalizeStage(snapshot.Stage),
		string(mode), configVersion, status, maxAttempts)
	return scanJob(row)
}

func insertEvent(ctx context.Context, queryer sqlQueryer, jobID int64, snapshot PromptSnapshot, configVersion int64, result *NormalizedResult) (*Event, error) {
	categories, _ := json.Marshal(result.Categories)
	matched, _ := json.Marshal(result.MatchedScanners)
	scores, _ := json.Marshal(result.ScannerScores)
	evidence := make(map[string]string, len(result.ScannerEvidence))
	for key, value := range result.ScannerEvidence {
		evidence[key] = RedactPreview(value, 160)
	}
	evidenceJSON, _ := json.Marshal(evidence)
	row := queryer.QueryRowContext(ctx, `
		INSERT INTO prompt_audit_events (
			job_id,request_id,user_id,username_snapshot,user_email_snapshot,api_key_id,api_key_name_snapshot,
			group_id,group_name,provider,endpoint,protocol,model,prompt_hash,redacted_preview,stage,
			decision,risk_level,action,categories,matched_scanners,scanner_scores,scanner_evidence,
			scanner_backend,scanner_version,guard_endpoint_id,policy_id,policy_version,config_version,chunk_total,latency_ms,
			full_prompt
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
			$20::jsonb,$21::jsonb,$22::jsonb,$23::jsonb,$24,$25,$26,$27,$28,$29,$30,$31,$32)
		RETURNING `+eventDetailColumns("prompt_audit_events"),
		jobID, snapshot.RequestID, nullableID(snapshot.UserID), snapshot.UsernameSnapshot, snapshot.UserEmailSnapshot,
		nullableID(snapshot.APIKeyID), snapshot.APIKeyNameSnapshot, snapshot.GroupID, snapshot.GroupName,
		snapshot.Provider, snapshot.Endpoint, snapshot.Protocol, snapshot.Model, snapshot.PromptHash,
		snapshot.RedactedPreview, normalizeStage(snapshot.Stage), string(result.Decision), string(result.RiskLevel),
		string(result.Action), categories, matched, scores, evidenceJSON, result.ScannerBackend, result.ScannerVersion,
		result.GuardEndpointID, result.PolicyID, result.PolicyVersion, configVersion, result.ChunkTotal, result.LatencyMS,
		snapshot.FullPrompt)
	return scanEvent(row, true)
}

type rowScanner interface{ Scan(...any) error }

func scanJob(row rowScanner) (*Job, error) {
	job := &Job{}
	var userID, apiKeyID, groupID sql.NullInt64
	var processingStarted, processed sql.NullTime
	err := row.Scan(
		&job.ID, &job.Snapshot.RequestID, &userID, &job.Snapshot.UsernameSnapshot, &job.Snapshot.UserEmailSnapshot,
		&apiKeyID, &job.Snapshot.APIKeyNameSnapshot, &groupID, &job.Snapshot.GroupName, &job.Snapshot.Provider,
		&job.Snapshot.Endpoint, &job.Snapshot.Protocol, &job.Snapshot.Model, &job.Snapshot.PromptHash,
		&job.Snapshot.RedactedPreview, &job.Snapshot.PromptLength, &job.Snapshot.MessageCount, &job.Snapshot.Stage,
		&job.ExecutionMode, &job.ConfigVersion, &job.Status, &job.Attempts, &job.MaxAttempts, &job.ClaimVersion,
		&job.NextAttemptAt, &processingStarted, &processed, &job.LastErrorCode, &job.LastErrorMessage,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	job.Snapshot.UserID = nullableInt64Value(userID)
	job.Snapshot.APIKeyID = nullableInt64Value(apiKeyID)
	job.Snapshot.GroupID = nullableInt64Ptr(groupID)
	if processingStarted.Valid {
		value := processingStarted.Time
		job.ProcessingStartedAt = &value
	}
	if processed.Valid {
		value := processed.Time
		job.ProcessedAt = &value
	}
	return job, nil
}

func jobColumns(alias string) string {
	return fmt.Sprintf(`%[1]s.id,%[1]s.request_id,%[1]s.user_id,%[1]s.username_snapshot,%[1]s.user_email_snapshot,
		%[1]s.api_key_id,%[1]s.api_key_name_snapshot,%[1]s.group_id,%[1]s.group_name,%[1]s.provider,
		%[1]s.endpoint,%[1]s.protocol,%[1]s.model,%[1]s.prompt_hash,%[1]s.redacted_preview,
		%[1]s.prompt_length,%[1]s.message_count,%[1]s.stage,%[1]s.execution_mode,%[1]s.config_version,%[1]s.status,
		%[1]s.attempts,%[1]s.max_attempts,%[1]s.claim_version,%[1]s.next_attempt_at,
		%[1]s.processing_started_at,%[1]s.processed_at,%[1]s.last_error_code,%[1]s.last_error_message,
		%[1]s.created_at,%[1]s.updated_at`, alias)
}

func normalizeStage(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return "http"
	}
	return stage
}

func requireOneRow(result sql.Result, err error, missing error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return missing
	}
	return nil
}

func nullableID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt64Value(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 internal/repository/DIALECT_MYSQL_GUIDE.md）。
// 本包不 import internal/repository（依赖方向约束，repository 的测试依赖本包），
// 方言判定按 *sql.DB 驱动类型探测（先例：repository 包 dashboard_aggregation_repo.go
// 的 isPostgresDriver / service 包 payment_fulfillment.go 的 paymentAuditDialect）。
// ---------------------------------------------------------------------------

// auditDBIsMySQL 报告 *sql.DB 底层驱动是否为 go-sql-driver/mysql。
func auditDBIsMySQL(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*mysql.MySQLDriver)
	return ok
}

// isMySQLDB 报告当前仓库连接是否为 MySQL。
func (r *PostgreSQLRepository) isMySQLDB() bool {
	return r != nil && auditDBIsMySQL(r.db)
}

// sqlPlaceholders 生成 n 个 "?" 占位符列表（MySQL IN 展开，
// 对应 repository 包的 Placeholders helper）。
func sqlPlaceholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// int64Args 把 []int64 展开为 []any 传参（MySQL IN 展开）。
func int64Args(values []int64) []any {
	args := make([]any, 0, len(values))
	for _, v := range values {
		args = append(args, v)
	}
	return args
}

// MySQL 命名锁名，分别对应 PG 版 int64 咨询锁键 promptAuditAdmissionLockKey /
// promptAuditConfigLockKey。命名锁按名字互斥，只需保证全局唯一且与 PG 侧用途一一对应。
const (
	promptAuditAdmissionLockNameMySQL = "prompt_audit:admission"
	promptAuditConfigLockNameMySQL    = "prompt_audit:config"
)

// acquireMySQLNamedLock 在独立连接上获取 MySQL 命名锁（GET_LOCK，连接级锁，
// 参考 migrations_runner.go 的 mysqlGetLock / scheduler_outbox_repo.go 先例）。
// timeoutSeconds 为 0 时对应 pg_try_advisory_lock 的非阻塞语义（返回 1=成功，0=被占用）；
// 为 -1 时无限等待（对应 pg_advisory_xact_lock 的阻塞语义，受 ctx 取消约束）。
// 返回的 release 恒非 nil（未获锁时为 no-op），持锁期间必须最终调用以
// RELEASE_LOCK 并归还连接。
func acquireMySQLNamedLock(ctx context.Context, db *sql.DB, name string, timeoutSeconds int) (release func(), acquired bool, err error) {
	if db == nil {
		return nil, false, errors.New("prompt audit database unavailable")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, timeoutSeconds).Scan(&locked); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		_ = conn.Close()
		return func() {}, false, nil
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(releaseCtx, "SELECT RELEASE_LOCK(?)", name)
		_ = conn.Close()
	}, true, nil
}

// createStagingWithCapacityMySQL 是 CreateStagingWithCapacity 的 MySQL 实现：
// pg_try_advisory_xact_lock（事务级咨询锁）→ GET_LOCK（连接级命名锁，独立连接持有，
// timeout 0 = 非阻塞尝试，语义对齐 try 锁），容量检查与插入仍在同一事务内完成。
func (r *PostgreSQLRepository) createStagingWithCapacityMySQL(ctx context.Context, snapshot PromptSnapshot, configVersion int64, maxAttempts, capacity int) (*Job, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	releaseAdmission, locked, err := acquireMySQLNamedLock(ctx, r.db, promptAuditAdmissionLockNameMySQL, 0)
	if err != nil {
		return nil, err
	}
	defer releaseAdmission()
	if !locked {
		return nil, ErrQueueAdmissionBusy
	}
	var active int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM prompt_audit_jobs
		WHERE status IN ('staging','queued','processing','retry')`).Scan(&active); err != nil {
		return nil, err
	}
	if capacity <= 0 || active >= capacity {
		return nil, ErrQueueFull
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	job, err := insertJobMySQL(ctx, tx, snapshot.Redacted(), ModeAsync, configVersion, "staging", maxAttempts)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

// publishQueuedMySQL 是 PublishQueued 的 MySQL 实现（$1→?、NOW()→NOW(3)）。
func (r *PostgreSQLRepository) publishQueuedMySQL(ctx context.Context, jobID int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET status='queued', next_attempt_at=NOW(3), updated_at=NOW(3)
		WHERE id=? AND status='staging'`, jobID)
	return requireOneRow(result, err, ErrLeaseLost)
}

// markStagingFailedMySQL 是 MarkStagingFailed 的 MySQL 实现（$n→?、NOW()→NOW(3)）。
func (r *PostgreSQLRepository) markStagingFailedMySQL(ctx context.Context, jobID int64, code string) error {
	code, message := sanitizeStoredError(code)
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs
		SET status='failed', processed_at=NOW(3), updated_at=NOW(3), last_error_code=?, last_error_message=?
		WHERE id=? AND status='staging'`, code, message, jobID)
	return requireOneRow(result, err, ErrLeaseLost)
}

// claimNextJobMySQL 是 ClaimNextJob 的 MySQL 实现。PG 版是可写 CTE
// （candidate FOR UPDATE SKIP LOCKED 选行 + UPDATE..FROM..RETURNING 整行），
// MySQL 无可写 CTE，拆为同事务三步：
//  1. SELECT id ... LIMIT 1 FOR UPDATE SKIP LOCKED（MySQL 要求 LIMIT 在 FOR UPDATE 之前）；
//  2. UPDATE 该行（attempts/claim_version 自增）；
//  3. 回查整行（对齐 RETURNING 的更新后值），提交后返回。
func (r *PostgreSQLRepository) claimNextJobMySQL(ctx context.Context, now time.Time) (*Job, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var candidateID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM prompt_audit_jobs
		WHERE status IN ('queued','retry') AND next_attempt_at <= ?
		ORDER BY next_attempt_at, id
		LIMIT 1
		FOR UPDATE SKIP LOCKED`, now.UTC()).Scan(&candidateID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE prompt_audit_jobs
		SET status='processing', attempts=attempts+1, claim_version=claim_version+1,
			processing_started_at=?, updated_at=?
		WHERE id=?`, now.UTC(), now.UTC(), candidateID); err != nil {
		return nil, false, err
	}
	job, err := scanJob(tx.QueryRowContext(ctx,
		`SELECT `+jobColumns("prompt_audit_jobs")+` FROM prompt_audit_jobs WHERE id=?`, candidateID))
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return job, true, nil
}

// refreshLeaseMySQL 是 RefreshLease 的 MySQL 实现（$n→?）。
func (r *PostgreSQLRepository) refreshLeaseMySQL(ctx context.Context, jobID, claimVersion int64, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET processing_started_at=?, updated_at=?
		WHERE id=? AND status='processing' AND claim_version=?`, now.UTC(), now.UTC(), jobID, claimVersion)
	return requireOneRow(result, err, ErrLeaseLost)
}

// retryMySQL 是 Retry 的 MySQL 实现（$n→?、NOW()→NOW(3)）。
func (r *PostgreSQLRepository) retryMySQL(ctx context.Context, jobID, claimVersion int64, next time.Time, code string) error {
	code, message := sanitizeStoredError(code)
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET status='retry', next_attempt_at=?, processing_started_at=NULL,
			updated_at=NOW(3), last_error_code=?, last_error_message=?
		WHERE id=? AND status='processing' AND claim_version=?`,
		next.UTC(), code, message, jobID, claimVersion)
	return requireOneRow(result, err, ErrLeaseLost)
}

// failMySQL 是 Fail 的 MySQL 实现（$n→?、NOW()→NOW(3)）。
func (r *PostgreSQLRepository) failMySQL(ctx context.Context, jobID, claimVersion int64, code string) error {
	code, message := sanitizeStoredError(code)
	result, err := r.db.ExecContext(ctx, `
		UPDATE prompt_audit_jobs SET status='failed', processed_at=NOW(3), processing_started_at=NULL,
			updated_at=NOW(3), last_error_code=?, last_error_message=?
		WHERE id=? AND status='processing' AND claim_version=?`,
		code, message, jobID, claimVersion)
	return requireOneRow(result, err, ErrLeaseLost)
}

// reclaimStaleMySQL 是 ReclaimStale 的 MySQL 实现。PG 版可写 CTE（stale 选行
// FOR UPDATE SKIP LOCKED + UPDATE..FROM）拆为同事务两步；MySQL 要求 LIMIT 位于
// FOR UPDATE 之前。SET 的 CASE 表达式与 PG 版逐字对应（MySQL 允许 SET 右侧
// 引用同行旧值）。
func (r *PostgreSQLRepository) reclaimStaleMySQL(ctx context.Context, stagingBefore, processingBefore time.Time, limit int) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM prompt_audit_jobs
		WHERE (status='staging' AND updated_at < ?)
		   OR (status='processing' AND processing_started_at < ?)
		ORDER BY updated_at, id LIMIT ? FOR UPDATE SKIP LOCKED`, stagingBefore.UTC(), processingBefore.UTC(), limit)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE prompt_audit_jobs
		SET status=CASE
			WHEN status='staging' THEN 'failed'
			WHEN attempts < max_attempts THEN 'retry'
			ELSE 'failed' END,
			next_attempt_at=CASE WHEN status='processing' AND attempts < max_attempts THEN NOW(3) ELSE next_attempt_at END,
			processing_started_at=NULL,
			processed_at=CASE WHEN status='staging' OR attempts >= max_attempts THEN NOW(3) ELSE NULL END,
			last_error_code=CASE WHEN status='staging' THEN 'staging_timeout' ELSE 'processing_lease_expired' END,
			last_error_message='', updated_at=NOW(3)
		WHERE id IN (`+sqlPlaceholders(len(ids))+`)`, int64Args(ids)...)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// insertJobInTx / insertEventInTx 按当前连接方言分派插入实现
// （MySQL 版见下；PG 版为原有 insertJob/insertEvent，保持不动）。
func (r *PostgreSQLRepository) insertJobInTx(ctx context.Context, tx *sql.Tx, snapshot PromptSnapshot, mode Mode, configVersion int64, status string, maxAttempts int) (*Job, error) {
	if r.isMySQLDB() {
		return insertJobMySQL(ctx, tx, snapshot, mode, configVersion, status, maxAttempts)
	}
	return insertJob(ctx, tx, snapshot, mode, configVersion, status, maxAttempts)
}

func (r *PostgreSQLRepository) insertEventInTx(ctx context.Context, tx *sql.Tx, jobID int64, snapshot PromptSnapshot, configVersion int64, result *NormalizedResult) (*Event, error) {
	if r.isMySQLDB() {
		return insertEventMySQL(ctx, tx, jobID, snapshot, configVersion, result)
	}
	return insertEvent(ctx, tx, jobID, snapshot, configVersion, result)
}

// insertJobMySQL 是 insertJob 的 MySQL 实现：PG 版 `INSERT ... RETURNING <job 全列>`
// 拆为 ExecContext + LastInsertId（AUTO_INCREMENT 主键）+ 同事务按 id 回查全列
// （对齐 RETURNING 语义；created_at/updated_at 等由数据库 CURRENT_TIMESTAMP(3) 生成）。
func insertJobMySQL(ctx context.Context, tx *sql.Tx, snapshot PromptSnapshot, mode Mode, configVersion int64, status string, maxAttempts int) (*Job, error) {
	processedExpr := "NULL"
	if status == "done" || status == "failed" {
		processedExpr = "NOW(3)"
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO prompt_audit_jobs (
			request_id,user_id,username_snapshot,user_email_snapshot,api_key_id,api_key_name_snapshot,
			group_id,group_name,provider,endpoint,protocol,model,prompt_hash,redacted_preview,
			prompt_length,message_count,stage,execution_mode,config_version,status,max_attempts,processed_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,`+processedExpr+`)`,
		snapshot.RequestID, nullableID(snapshot.UserID), snapshot.UsernameSnapshot, snapshot.UserEmailSnapshot,
		nullableID(snapshot.APIKeyID), snapshot.APIKeyNameSnapshot, snapshot.GroupID, snapshot.GroupName,
		snapshot.Provider, snapshot.Endpoint, snapshot.Protocol, snapshot.Model, snapshot.PromptHash,
		snapshot.RedactedPreview, snapshot.PromptLength, snapshot.MessageCount, normalizeStage(snapshot.Stage),
		string(mode), configVersion, status, maxAttempts)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return scanJob(tx.QueryRowContext(ctx,
		`SELECT `+jobColumns("prompt_audit_jobs")+` FROM prompt_audit_jobs WHERE id=?`, id))
}

// insertEventMySQL 是 insertEvent 的 MySQL 实现：`$n::jsonb` → CAST(? AS JSON)，
// `INSERT ... RETURNING <event 全列含 full_prompt>` → ExecContext + LastInsertId +
// 同事务按 id 回查（对齐 RETURNING 语义）。
func insertEventMySQL(ctx context.Context, tx *sql.Tx, jobID int64, snapshot PromptSnapshot, configVersion int64, result *NormalizedResult) (*Event, error) {
	categories, _ := json.Marshal(result.Categories)
	matched, _ := json.Marshal(result.MatchedScanners)
	scores, _ := json.Marshal(result.ScannerScores)
	evidence := make(map[string]string, len(result.ScannerEvidence))
	for key, value := range result.ScannerEvidence {
		evidence[key] = RedactPreview(value, 160)
	}
	evidenceJSON, _ := json.Marshal(evidence)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO prompt_audit_events (
			job_id,request_id,user_id,username_snapshot,user_email_snapshot,api_key_id,api_key_name_snapshot,
			group_id,group_name,provider,endpoint,protocol,model,prompt_hash,redacted_preview,stage,
			decision,risk_level,action,categories,matched_scanners,scanner_scores,scanner_evidence,
			scanner_backend,scanner_version,guard_endpoint_id,policy_id,policy_version,config_version,chunk_total,latency_ms,
			full_prompt
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,
			CAST(? AS JSON),CAST(? AS JSON),CAST(? AS JSON),CAST(? AS JSON),?,?,?,?,?,?,?,?,?)`,
		jobID, snapshot.RequestID, nullableID(snapshot.UserID), snapshot.UsernameSnapshot, snapshot.UserEmailSnapshot,
		nullableID(snapshot.APIKeyID), snapshot.APIKeyNameSnapshot, snapshot.GroupID, snapshot.GroupName,
		snapshot.Provider, snapshot.Endpoint, snapshot.Protocol, snapshot.Model, snapshot.PromptHash,
		snapshot.RedactedPreview, normalizeStage(snapshot.Stage), string(result.Decision), string(result.RiskLevel),
		string(result.Action), categories, matched, scores, evidenceJSON, result.ScannerBackend, result.ScannerVersion,
		result.GuardEndpointID, result.PolicyID, result.PolicyVersion, configVersion, result.ChunkTotal, result.LatencyMS,
		snapshot.FullPrompt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return scanEvent(tx.QueryRowContext(ctx,
		`SELECT `+eventDetailColumns("prompt_audit_events")+` FROM prompt_audit_events WHERE id=?`, id), true)
}
