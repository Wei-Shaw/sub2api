package securityaudit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type EventFilter struct {
	Decision   string     `json:"decision,omitempty"`
	RiskLevel  string     `json:"risk_level,omitempty"`
	Endpoint   string     `json:"endpoint,omitempty"`
	GroupID    *int64     `json:"group_id,omitempty"`
	UserID     *int64     `json:"user_id,omitempty"`
	APIKeyID   *int64     `json:"api_key_id,omitempty"`
	RequestID  string     `json:"request_id,omitempty"`
	PromptHash string     `json:"prompt_hash,omitempty"`
	Keyword    string     `json:"keyword,omitempty"`
	StartAt    *time.Time `json:"start_at,omitempty"`
	EndAt      *time.Time `json:"end_at,omitempty"`
}

type EventPage struct {
	Items    []*Event `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	Pages    int      `json:"pages"`
}

type DeletePreview struct {
	MatchedCount      int64       `json:"matched_count"`
	FilterSummary     EventFilter `json:"filter_summary"`
	SnapshotMaxID     int64       `json:"snapshot_max_id"`
	FilterHash        string      `json:"filter_hash"`
	ConfirmationToken string      `json:"confirmation_token,omitempty"`
	ExpiresAt         time.Time   `json:"expires_at,omitempty"`
}

type DeleteResult struct {
	DeletedEvents int64   `json:"deleted_events"`
	DeletedJobs   int64   `json:"deleted_jobs"`
	JobIDs        []int64 `json:"-"`
}

type EventRepository interface {
	ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error)
	GetEvent(ctx context.Context, id int64) (*Event, error)
	DeleteEvent(ctx context.Context, id int64) (*DeleteResult, error)
	DeleteEventsByIDs(ctx context.Context, ids []int64) (*DeleteResult, error)
	PreviewDelete(ctx context.Context, filter EventFilter) (*DeletePreview, error)
	DeleteEventsByFilter(ctx context.Context, filter EventFilter, snapshotMaxID int64, batchSize int) (*DeleteResult, error)
}

func (r *PostgreSQLRepository) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if r.isMySQLDB() {
		return r.listEventsMySQL(ctx, filter, page, pageSize)
	}
	where, args := buildEventWhere(filter, 1)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM prompt_audit_events e`+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append([]any(nil), args...)
	limitIndex := len(queryArgs) + 1
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+eventColumns("e")+` FROM prompt_audit_events e`+where+
		fmt.Sprintf(` ORDER BY e.created_at DESC, e.id DESC LIMIT $%d OFFSET $%d`, limitIndex, limitIndex+1), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*Event, 0, pageSize)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &EventPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *PostgreSQLRepository) GetEvent(ctx context.Context, id int64) (*Event, error) {
	eventQuery := `SELECT ` + eventDetailColumns("e") + ` FROM prompt_audit_events e WHERE e.id=$1`
	if r.isMySQLDB() {
		eventQuery = `SELECT ` + eventDetailColumns("e") + ` FROM prompt_audit_events e WHERE e.id=?`
	}
	event, err := scanEvent(r.db.QueryRowContext(ctx, eventQuery, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEventNotFound
	}
	return event, err
}

func (r *PostgreSQLRepository) DeleteEvent(ctx context.Context, id int64) (*DeleteResult, error) {
	return r.DeleteEventsByIDs(ctx, []int64{id})
}

func (r *PostgreSQLRepository) DeleteEventsByIDs(ctx context.Context, ids []int64) (*DeleteResult, error) {
	ids = canonicalInt64s(ids)
	if len(ids) == 0 {
		return &DeleteResult{}, nil
	}
	if len(ids) > 500 {
		return nil, errors.New("prompt audit delete batch exceeds 500 events")
	}
	if r.isMySQLDB() {
		return r.deleteEventsByIDsMySQL(ctx, ids)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `DELETE FROM prompt_audit_events WHERE id=ANY($1) RETURNING job_id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	jobIDs, err := scanReturnedJobIDs(rows)
	if err != nil {
		return nil, err
	}
	deletedJobs, err := r.deleteOrphanJobs(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DeleteResult{DeletedEvents: int64(len(jobIDs)), DeletedJobs: deletedJobs, JobIDs: canonicalInt64s(jobIDs)}, nil
}

func (r *PostgreSQLRepository) PreviewDelete(ctx context.Context, filter EventFilter) (*DeletePreview, error) {
	if err := validateDeleteFilter(filter); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	where, args := buildEventWhere(filter, 1)
	if r.isMySQLDB() {
		where, args = buildEventWhereMySQL(filter)
	}
	var count, maxID int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(e.id),0) FROM prompt_audit_events e`+where, args...).Scan(&count, &maxID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	canonical := canonicalEventFilter(filter)
	return &DeletePreview{MatchedCount: count, FilterSummary: canonical, SnapshotMaxID: maxID, FilterHash: FilterHash(canonical, maxID)}, nil
}

func (r *PostgreSQLRepository) DeleteEventsByFilter(ctx context.Context, filter EventFilter, snapshotMaxID int64, batchSize int) (*DeleteResult, error) {
	if err := validateDeleteFilter(filter); err != nil {
		return nil, err
	}
	if snapshotMaxID <= 0 {
		return &DeleteResult{}, nil
	}
	if batchSize < 1 || batchSize > 1000 {
		batchSize = 200
	}
	if r.isMySQLDB() {
		return r.deleteEventsByFilterMySQL(ctx, filter, snapshotMaxID, batchSize)
	}
	total := &DeleteResult{}
	jobSet := map[int64]struct{}{}
	for {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		where, args := buildEventWhere(filter, 1)
		maxIndex := len(args) + 1
		limitIndex := maxIndex + 1
		args = append(args, snapshotMaxID, batchSize)
		rows, err := tx.QueryContext(ctx, `
			WITH selected AS (
				SELECT e.id FROM prompt_audit_events e`+where+
			fmt.Sprintf(` AND e.id <= $%d ORDER BY e.id LIMIT $%d FOR UPDATE SKIP LOCKED`, maxIndex, limitIndex)+`
			), deleted AS (
				DELETE FROM prompt_audit_events e USING selected s WHERE e.id=s.id RETURNING e.job_id
			) SELECT job_id FROM deleted`, args...)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		jobIDs, err := scanReturnedJobIDs(rows)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		deletedJobs, err := r.deleteOrphanJobs(ctx, tx, jobIDs)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		total.DeletedEvents += int64(len(jobIDs))
		total.DeletedJobs += deletedJobs
		for _, id := range jobIDs {
			jobSet[id] = struct{}{}
		}
		if len(jobIDs) < batchSize {
			break
		}
	}
	for id := range jobSet {
		total.JobIDs = append(total.JobIDs, id)
	}
	total.JobIDs = canonicalInt64s(total.JobIDs)
	return total, nil
}

func FilterHash(filter EventFilter, snapshotMaxID int64) string {
	payload := struct {
		Filter        EventFilter `json:"filter"`
		SnapshotMaxID int64       `json:"snapshot_max_id"`
	}{canonicalEventFilter(filter), snapshotMaxID}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validateDeleteFilter(filter EventFilter) error {
	if filter.StartAt == nil || filter.EndAt == nil || !filter.StartAt.Before(*filter.EndAt) {
		return errors.New("prompt audit filter delete requires a valid explicit time range")
	}
	return nil
}

func canonicalEventFilter(filter EventFilter) EventFilter {
	filter.Decision = strings.TrimSpace(strings.ToLower(filter.Decision))
	filter.RiskLevel = strings.TrimSpace(strings.ToLower(filter.RiskLevel))
	filter.Endpoint = strings.TrimSpace(filter.Endpoint)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	filter.PromptHash = strings.ToLower(strings.TrimSpace(filter.PromptHash))
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.StartAt != nil {
		value := filter.StartAt.UTC()
		filter.StartAt = &value
	}
	if filter.EndAt != nil {
		value := filter.EndAt.UTC()
		filter.EndAt = &value
	}
	return filter
}

func buildEventWhere(filter EventFilter, firstIndex int) (string, []any) {
	filter = canonicalEventFilter(filter)
	clauses := []string{" WHERE TRUE"}
	args := make([]any, 0, 12)
	add := func(clause string, value any) {
		clauses = append(clauses, fmt.Sprintf(clause, firstIndex+len(args)))
		args = append(args, value)
	}
	if filter.Decision != "" {
		add(" AND e.decision=$%d", filter.Decision)
	}
	if filter.RiskLevel != "" {
		add(" AND e.risk_level=$%d", filter.RiskLevel)
	}
	if filter.Endpoint != "" {
		add(" AND e.endpoint=$%d", filter.Endpoint)
	}
	if filter.GroupID != nil {
		add(" AND e.group_id=$%d", *filter.GroupID)
	}
	if filter.UserID != nil {
		add(" AND e.user_id=$%d", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add(" AND e.api_key_id=$%d", *filter.APIKeyID)
	}
	if filter.RequestID != "" {
		add(" AND e.request_id=$%d", filter.RequestID)
	}
	if filter.PromptHash != "" {
		add(" AND e.prompt_hash=$%d", filter.PromptHash)
	}
	if filter.Keyword != "" {
		add(` AND (e.request_id ILIKE $%d OR e.prompt_hash ILIKE $%d OR e.redacted_preview ILIKE $%d
			OR e.username_snapshot ILIKE $%d OR e.user_email_snapshot ILIKE $%d OR e.api_key_name_snapshot ILIKE $%d)`, "%"+TrimRunes(filter.Keyword, 128)+"%")
		// The clause has six placeholders but add only supplied one. Rebuild it with one shared placeholder.
		clauses[len(clauses)-1] = fmt.Sprintf(` AND (e.request_id ILIKE $%[1]d OR e.prompt_hash ILIKE $%[1]d OR e.redacted_preview ILIKE $%[1]d
			OR e.username_snapshot ILIKE $%[1]d OR e.user_email_snapshot ILIKE $%[1]d OR e.api_key_name_snapshot ILIKE $%[1]d)`, firstIndex+len(args)-1)
	}
	if filter.StartAt != nil {
		add(" AND e.created_at >= $%d", filter.StartAt.UTC())
	}
	if filter.EndAt != nil {
		add(" AND e.created_at <= $%d", filter.EndAt.UTC())
	}
	return strings.Join(clauses, ""), args
}

func eventColumns(alias string) string {
	return fmt.Sprintf(`%[1]s.id,%[1]s.job_id,%[1]s.request_id,%[1]s.user_id,%[1]s.username_snapshot,
		%[1]s.user_email_snapshot,%[1]s.api_key_id,%[1]s.api_key_name_snapshot,%[1]s.group_id,%[1]s.group_name,
		%[1]s.provider,%[1]s.endpoint,%[1]s.protocol,%[1]s.model,%[1]s.prompt_hash,%[1]s.redacted_preview,
		%[1]s.stage,%[1]s.decision,%[1]s.risk_level,%[1]s.action,%[1]s.categories,%[1]s.matched_scanners,
		%[1]s.scanner_scores,%[1]s.scanner_evidence,%[1]s.scanner_backend,%[1]s.scanner_version,
		%[1]s.guard_endpoint_id,%[1]s.policy_id,%[1]s.policy_version,%[1]s.config_version,
		%[1]s.chunk_total,%[1]s.latency_ms,%[1]s.created_at`, alias)
}

// eventDetailColumns adds the full prompt, which can be large, so it is only
// loaded for single-event detail reads and never for list pages.
func eventDetailColumns(alias string) string {
	return eventColumns(alias) + fmt.Sprintf(",%[1]s.full_prompt", alias)
}

func scanEvent(row rowScanner, withFullPrompt ...bool) (*Event, error) {
	event := &Event{}
	var userID, apiKeyID, groupID sql.NullInt64
	var categories, matched, scores, evidence []byte
	dest := []any{&event.ID, &event.JobID, &event.Snapshot.RequestID, &userID,
		&event.Snapshot.UsernameSnapshot, &event.Snapshot.UserEmailSnapshot, &apiKeyID,
		&event.Snapshot.APIKeyNameSnapshot, &groupID, &event.Snapshot.GroupName,
		&event.Snapshot.Provider, &event.Snapshot.Endpoint, &event.Snapshot.Protocol, &event.Snapshot.Model,
		&event.Snapshot.PromptHash, &event.Snapshot.RedactedPreview, &event.Snapshot.Stage, &event.Decision,
		&event.RiskLevel, &event.Action, &categories, &matched, &scores, &evidence, &event.ScannerBackend,
		&event.ScannerVersion, &event.GuardEndpointID, &event.PolicyID, &event.PolicyVersion,
		&event.ConfigVersion, &event.ChunkTotal, &event.LatencyMS, &event.CreatedAt}
	if len(withFullPrompt) > 0 && withFullPrompt[0] {
		dest = append(dest, &event.Snapshot.FullPrompt)
	}
	err := row.Scan(dest...)
	if err != nil {
		return nil, err
	}
	event.Snapshot.UserID = nullableInt64Value(userID)
	event.Snapshot.APIKeyID = nullableInt64Value(apiKeyID)
	event.Snapshot.GroupID = nullableInt64Ptr(groupID)
	_ = json.Unmarshal(categories, &event.Categories)
	_ = json.Unmarshal(matched, &event.MatchedScanners)
	_ = json.Unmarshal(scores, &event.ScannerScores)
	_ = json.Unmarshal(evidence, &event.ScannerEvidence)
	result := NormalizedResult{Decision: event.Decision, RiskLevel: event.RiskLevel, Action: event.Action,
		Categories: event.Categories, MatchedScanners: event.MatchedScanners, ScannerScores: event.ScannerScores,
		ScannerEvidence: event.ScannerEvidence}
	event.IssueSummaries = BuildIssueSummaries(result)
	return event, nil
}

func scanReturnedJobIDs(rows *sql.Rows) ([]int64, error) {
	defer func() { _ = rows.Close() }()
	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// deleteOrphanJobs 删除已无事件关联且不在处理中的 job。
// MySQL 版：`DELETE FROM t alias` 需改写为多表删除语法 `DELETE j FROM ...`；
// `id=ANY($1)` + pq.Array → IN 展开。
func (r *PostgreSQLRepository) deleteOrphanJobs(ctx context.Context, tx *sql.Tx, jobIDs []int64) (int64, error) {
	jobIDs = canonicalInt64s(jobIDs)
	if len(jobIDs) == 0 {
		return 0, nil
	}
	query := `DELETE FROM prompt_audit_jobs j
		WHERE j.id=ANY($1) AND j.status <> 'processing'
		AND NOT EXISTS (SELECT 1 FROM prompt_audit_events e WHERE e.job_id=j.id)`
	args := []any{pq.Array(jobIDs)}
	if r.isMySQLDB() {
		query = `DELETE j FROM prompt_audit_jobs j
			WHERE j.id IN (` + sqlPlaceholders(len(jobIDs)) + `) AND j.status <> 'processing'
			AND NOT EXISTS (SELECT 1 FROM prompt_audit_events e WHERE e.job_id=j.id)`
		args = int64Args(jobIDs)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 internal/repository/DIALECT_MYSQL_GUIDE.md）。
// ---------------------------------------------------------------------------

// listEventsMySQL 是 ListEvents 的 MySQL 实现：WHERE 构造改用 ? 占位符与 LIKE
// （utf8mb4_0900_ai_ci 排序规则天然大小写不敏感，对齐 PG 的 ILIKE），
// LIMIT/OFFSET 占位符同理。
func (r *PostgreSQLRepository) listEventsMySQL(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	where, args := buildEventWhereMySQL(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM prompt_audit_events e`+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+eventColumns("e")+` FROM prompt_audit_events e`+where+
		` ORDER BY e.created_at DESC, e.id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*Event, 0, pageSize)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &EventPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

// deleteEventsByIDsMySQL 是 DeleteEventsByIDs 的 MySQL 实现。PG 版
// `DELETE ... WHERE id=ANY($1) RETURNING job_id` 拆为同事务两步：
// 先 SELECT ... FOR UPDATE 锁定目标行并取得 job_id 集合，再按已锁定 id 集合删除，
// 返回先前读到的 job_id（对齐 RETURNING 语义）。
func (r *PostgreSQLRepository) deleteEventsByIDsMySQL(ctx context.Context, ids []int64) (*DeleteResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx,
		`SELECT id, job_id FROM prompt_audit_events WHERE id IN (`+sqlPlaceholders(len(ids))+`) ORDER BY id FOR UPDATE`,
		int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	lockedIDs := make([]int64, 0, len(ids))
	jobIDs := make([]int64, 0, len(ids))
	for rows.Next() {
		var id, jobID int64
		if err := rows.Scan(&id, &jobID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		lockedIDs = append(lockedIDs, id)
		jobIDs = append(jobIDs, jobID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(lockedIDs) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM prompt_audit_events WHERE id IN (`+sqlPlaceholders(len(lockedIDs))+`)`,
			int64Args(lockedIDs)...); err != nil {
			return nil, err
		}
	}
	deletedJobs, err := r.deleteOrphanJobs(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DeleteResult{DeletedEvents: int64(len(jobIDs)), DeletedJobs: deletedJobs, JobIDs: canonicalInt64s(jobIDs)}, nil
}

// deleteEventsByFilterMySQL 是 DeleteEventsByFilter 的 MySQL 实现。PG 版可写 CTE
// （selected FOR UPDATE SKIP LOCKED + DELETE..USING..RETURNING job_id）拆为同事务
// 「锁定读行（拿 job_id）→ 按已锁定 id 集合删除」两步；MySQL 要求 LIMIT 位于
// FOR UPDATE 之前。
// REVIEW: 行集合已由同事务 FOR UPDATE 锁定，按 id 集合删除与
// `DELETE e FROM prompt_audit_events e JOIN (selected) s ON e.id=s.id`
// （DIALECT_MYSQL_GUIDE 的 USING→JOIN 映射）作用于同一批已锁定行。
func (r *PostgreSQLRepository) deleteEventsByFilterMySQL(ctx context.Context, filter EventFilter, snapshotMaxID int64, batchSize int) (*DeleteResult, error) {
	total := &DeleteResult{}
	jobSet := map[int64]struct{}{}
	where, baseArgs := buildEventWhereMySQL(filter)
	for {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		selectArgs := append(append([]any(nil), baseArgs...), snapshotMaxID, batchSize)
		rows, err := tx.QueryContext(ctx, `
			SELECT e.id, e.job_id FROM prompt_audit_events e`+where+
			` AND e.id <= ? ORDER BY e.id LIMIT ? FOR UPDATE SKIP LOCKED`, selectArgs...)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		ids := make([]int64, 0, batchSize)
		jobIDs := make([]int64, 0, batchSize)
		for rows.Next() {
			var id, jobID int64
			if err := rows.Scan(&id, &jobID); err != nil {
				_ = rows.Close()
				_ = tx.Rollback()
				return nil, err
			}
			ids = append(ids, id)
			jobIDs = append(jobIDs, jobID)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if len(ids) > 0 {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM prompt_audit_events WHERE id IN (`+sqlPlaceholders(len(ids))+`)`,
				int64Args(ids)...); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
		}
		deletedJobs, err := r.deleteOrphanJobs(ctx, tx, jobIDs)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		total.DeletedEvents += int64(len(jobIDs))
		total.DeletedJobs += deletedJobs
		for _, id := range jobIDs {
			jobSet[id] = struct{}{}
		}
		if len(jobIDs) < batchSize {
			break
		}
	}
	for id := range jobSet {
		total.JobIDs = append(total.JobIDs, id)
	}
	total.JobIDs = canonicalInt64s(total.JobIDs)
	return total, nil
}

// buildEventWhereMySQL 是 buildEventWhere 的 MySQL 版：$n → ?；ILIKE → LIKE
// （utf8mb4_0900_ai_ci 天然大小写不敏感）。PG 版六个 ILIKE 共享同一个 $n 占位符，
// MySQL 的 ? 按位置绑定，同一模式需重复提供六次参数。
func buildEventWhereMySQL(filter EventFilter) (string, []any) {
	filter = canonicalEventFilter(filter)
	clauses := []string{" WHERE TRUE"}
	args := make([]any, 0, 12)
	add := func(clause string, value any) {
		clauses = append(clauses, clause)
		args = append(args, value)
	}
	if filter.Decision != "" {
		add(" AND e.decision=?", filter.Decision)
	}
	if filter.RiskLevel != "" {
		add(" AND e.risk_level=?", filter.RiskLevel)
	}
	if filter.Endpoint != "" {
		add(" AND e.endpoint=?", filter.Endpoint)
	}
	if filter.GroupID != nil {
		add(" AND e.group_id=?", *filter.GroupID)
	}
	if filter.UserID != nil {
		add(" AND e.user_id=?", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add(" AND e.api_key_id=?", *filter.APIKeyID)
	}
	if filter.RequestID != "" {
		add(" AND e.request_id=?", filter.RequestID)
	}
	if filter.PromptHash != "" {
		add(" AND e.prompt_hash=?", filter.PromptHash)
	}
	if filter.Keyword != "" {
		pattern := "%" + TrimRunes(filter.Keyword, 128) + "%"
		clauses = append(clauses, ` AND (e.request_id LIKE ? OR e.prompt_hash LIKE ? OR e.redacted_preview LIKE ?
			OR e.username_snapshot LIKE ? OR e.user_email_snapshot LIKE ? OR e.api_key_name_snapshot LIKE ?)`)
		for i := 0; i < 6; i++ {
			args = append(args, pattern)
		}
	}
	if filter.StartAt != nil {
		add(" AND e.created_at >= ?", filter.StartAt.UTC())
	}
	if filter.EndAt != nil {
		add(" AND e.created_at <= ?", filter.EndAt.UTC())
	}
	return strings.Join(clauses, ""), args
}
