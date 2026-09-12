package securityaudit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

const (
	promptRecordQueueCapacity     = 256
	promptRecordOverflowCapacity  = 2048
	promptRecordWorkerCount       = 4
	promptResponseWorkerCount     = 1
	promptRecordPersistTimeout    = 5 * time.Second
	promptResponsePersistAttempts = 10
	promptResponsePersistDelay    = 50 * time.Millisecond
)

// PromptRecord is the retained user prompt. Risk fields are intentionally
// independent from the prompt audit event model so a future risk service can
// update them without changing request handling.
type PromptRecord struct {
	ID                 int64      `json:"id"`
	RequestID          string     `json:"request_id"`
	TurnNo             int        `json:"turn_no"`
	Stage              string     `json:"stage"`
	UserID             int64      `json:"user_id"`
	Username           string     `json:"username"`
	UserEmail          string     `json:"user_email"`
	APIKeyID           int64      `json:"api_key_id"`
	APIKeyName         string     `json:"api_key_name"`
	GroupID            *int64     `json:"group_id,omitempty"`
	GroupName          string     `json:"group_name"`
	Provider           string     `json:"provider"`
	Endpoint           string     `json:"endpoint"`
	Protocol           string     `json:"protocol"`
	Model              string     `json:"model"`
	PromptHash         string     `json:"prompt_hash"`
	PromptText         string     `json:"prompt_text"`
	RequestBody        string     `json:"request_body"`
	RequestHeaders     string     `json:"request_headers"`
	PromptLength       int        `json:"prompt_length"`
	MessageCount       int        `json:"message_count"`
	RiskStatus         string     `json:"risk_status"`
	RiskResult         string     `json:"risk_result"`
	RiskCheckedAt      *time.Time `json:"risk_checked_at,omitempty"`
	ResponseText       string     `json:"response_text"`
	ResponseLength     int        `json:"response_length"`
	ResponseTruncated  bool       `json:"response_truncated"`
	ResponseCapturedAt *time.Time `json:"response_captured_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
}

// PromptRecordSummary contains only call metadata. Full prompt text and risk
// results are loaded through GetPromptRecord when an administrator opens a row.
type PromptRecordSummary struct {
	ID            int64      `json:"id"`
	RequestID     string     `json:"request_id"`
	TurnNo        int        `json:"turn_no"`
	Stage         string     `json:"stage"`
	UserID        int64      `json:"user_id"`
	Username      string     `json:"username"`
	UserEmail     string     `json:"user_email"`
	APIKeyID      int64      `json:"api_key_id"`
	APIKeyName    string     `json:"api_key_name"`
	GroupID       *int64     `json:"group_id,omitempty"`
	GroupName     string     `json:"group_name"`
	Provider      string     `json:"provider"`
	Endpoint      string     `json:"endpoint"`
	Protocol      string     `json:"protocol"`
	Model         string     `json:"model"`
	PromptHash    string     `json:"prompt_hash"`
	PromptLength  int        `json:"prompt_length"`
	MessageCount  int        `json:"message_count"`
	RiskStatus    string     `json:"risk_status"`
	RiskCheckedAt *time.Time `json:"risk_checked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type PromptRecordFilter struct {
	CursorMode      bool
	CursorCreatedAt *time.Time
	CursorID        int64
	RequestID       string
	UserID          *int64
	APIKeyID        *int64
	Model           string
	Stage           string
	StartAt         *time.Time
	EndAt           *time.Time
}

type PromptRecordPage struct {
	HasMore    bool                   `json:"has_more"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	Items      []*PromptRecordSummary `json:"items"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	Total      int64                  `json:"total"`
	TotalPages int                    `json:"total_pages"`
	Queue      PromptRecordQueueStats `json:"queue"`
}

type PromptRecordQueueStats struct {
	QueueLength      int        `json:"queue_length"`
	QueueCapacity    int        `json:"queue_capacity"`
	OverflowLength   int        `json:"overflow_length"`
	OverflowCapacity int        `json:"overflow_capacity"`
	WorkerCount      int        `json:"worker_count"`
	DroppedTotal     int64      `json:"dropped_total"`
	PersistFailed    int64      `json:"persist_failed_total"`
	LastDroppedAt    *time.Time `json:"last_dropped_at,omitempty"`
	InFlightBytes    int64      `json:"in_flight_bytes"`
	ByteCapacity     int64      `json:"byte_capacity"`
	ItemByteLimit    int64      `json:"item_byte_limit"`
	PendingResponses int        `json:"pending_responses"`
	RequestDropped   int64      `json:"request_dropped_total"`
	ResponseDropped  int64      `json:"response_dropped_total"`
	RequestFailed    int64      `json:"request_failed_total"`
	ResponseFailed   int64      `json:"response_failed_total"`
	ExpiredDeleted   int64      `json:"expired_deleted_total"`
	CleanupFailed    int64      `json:"cleanup_failed_total"`
	CleanupBacklog   bool       `json:"cleanup_backlog"`
}

type PromptRecordRepository interface {
	InsertPromptRecord(ctx context.Context, record *PromptRecord) error
	UpdatePromptRecordResponse(ctx context.Context, key PromptRecordKey, response PromptResponse) (bool, error)
	ListPromptRecords(ctx context.Context, filter PromptRecordFilter, page, pageSize int) (*PromptRecordPage, error)
	GetPromptRecord(ctx context.Context, id int64) (*PromptRecord, error)
	DeletePromptRecord(ctx context.Context, id int64) error
	DeletePromptRecords(ctx context.Context, ids []int64) (int64, error)
}

type PromptResponse struct {
	Text       string
	Length     int
	Truncated  bool
	CapturedAt time.Time
}

func (r *PostgreSQLRepository) InsertPromptRecord(ctx context.Context, record *PromptRecord) error {
	if r == nil || r.db == nil {
		return errors.New("prompt record database unavailable")
	}
	if record == nil {
		return errors.New("prompt record is empty")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO prompt_records (
			request_id, turn_no, stage, user_id, username_snapshot, user_email_snapshot,
			api_key_id, api_key_name_snapshot, group_id, group_name, provider, endpoint,
			protocol, model, prompt_hash, prompt_text, prompt_length, message_count,
			risk_status, created_at, expires_at, request_body, request_headers
		) VALUES ($1,$2,$3,NULLIF($4,0),$5,$6,NULLIF($7,0),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		ON CONFLICT DO NOTHING`,
		record.RequestID, record.TurnNo, record.Stage, record.UserID, record.Username, record.UserEmail,
		record.APIKeyID, record.APIKeyName, record.GroupID, record.GroupName, record.Provider, record.Endpoint,
		record.Protocol, record.Model, record.PromptHash, record.PromptText, record.PromptLength, record.MessageCount,
		ifEmpty(record.RiskStatus, "pending"), record.CreatedAt, record.ExpiresAt, record.RequestBody, record.RequestHeaders)
	return err
}

func (r *PostgreSQLRepository) UpdatePromptRecordResponse(ctx context.Context, key PromptRecordKey, captured PromptResponse) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("prompt record database unavailable")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE prompt_records
		SET response_text=$1, response_length=$2, response_truncated=$3, response_captured_at=$4
		WHERE request_id=$5 AND stage=$6 AND turn_no=$7 AND COALESCE(api_key_id,0)=$8 AND prompt_hash=$9`,
		captured.Text, captured.Length, captured.Truncated, captured.CapturedAt,
		key.RequestID, ifEmpty(key.Stage, "http"), key.TurnNo, key.APIKeyID, key.PromptHash)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *PostgreSQLRepository) ListPromptRecords(ctx context.Context, filter PromptRecordFilter, page, pageSize int) (*PromptRecordPage, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("prompt record database unavailable")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	where := []string{"(expires_at IS NULL OR expires_at > NOW())"}
	args := make([]any, 0, 5)
	add := func(sqlText string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(sqlText, len(args)))
	}
	if filter.RequestID != "" {
		add("request_id = $%d", filter.RequestID)
	}
	if filter.UserID != nil {
		add("user_id = $%d", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add("api_key_id = $%d", *filter.APIKeyID)
	}
	if strings.TrimSpace(filter.Model) != "" {
		add("model ILIKE $%d", "%"+strings.TrimSpace(filter.Model)+"%")
	}
	if strings.TrimSpace(filter.Stage) != "" {
		add("stage = $%d", filter.Stage)
	}
	if filter.StartAt != nil {
		add("created_at >= $%d", filter.StartAt.UTC())
	}
	if filter.EndAt != nil {
		add("created_at <= $%d", filter.EndAt.UTC())
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	var pagingSQL string
	if filter.CursorMode {
		if filter.CursorCreatedAt != nil {
			args = append(args, filter.CursorCreatedAt.UTC(), filter.CursorID)
			whereSQL += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
		}
		args = append(args, pageSize+1)
		pagingSQL = fmt.Sprintf(" LIMIT $%d", len(args))
	} else {
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prompt_records WHERE "+whereSQL, args...).Scan(&total); err != nil {
			return nil, err
		}
		args = append(args, pageSize, int64(page-1)*int64(pageSize))
		pagingSQL = fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, request_id, turn_no, stage, COALESCE(user_id,0), username_snapshot,
		user_email_snapshot, COALESCE(api_key_id,0), api_key_name_snapshot, group_id, group_name, provider, endpoint,
		protocol, model, prompt_hash, prompt_length, message_count, risk_status,
		risk_checked_at, created_at, expires_at FROM prompt_records WHERE `+whereSQL+` ORDER BY created_at DESC, id DESC`+pagingSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*PromptRecordSummary, 0, pageSize)
	for rows.Next() {
		item := new(PromptRecordSummary)
		var groupID sql.NullInt64
		var riskCheckedAt, expiresAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.RequestID, &item.TurnNo, &item.Stage, &item.UserID, &item.Username, &item.UserEmail, &item.APIKeyID, &item.APIKeyName, &groupID, &item.GroupName, &item.Provider, &item.Endpoint, &item.Protocol, &item.Model, &item.PromptHash, &item.PromptLength, &item.MessageCount, &item.RiskStatus, &riskCheckedAt, &item.CreatedAt, &expiresAt); err != nil {
			return nil, err
		}
		if groupID.Valid {
			item.GroupID = &groupID.Int64
		}
		if riskCheckedAt.Valid {
			item.RiskCheckedAt = &riskCheckedAt.Time
		}
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	result := &PromptRecordPage{Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}
	if filter.CursorMode && len(items) > pageSize {
		result.Items = items[:pageSize]
		last := result.Items[pageSize-1]
		result.HasMore = true
		result.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + fmt.Sprint(last.ID)))
	}
	return result, nil
}

func (r *PostgreSQLRepository) GetPromptRecord(ctx context.Context, id int64) (*PromptRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("prompt record database unavailable")
	}
	item := new(PromptRecord)
	var groupID sql.NullInt64
	var responseCapturedAt, riskCheckedAt, expiresAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id, request_id, turn_no, stage, COALESCE(user_id,0), username_snapshot,
		user_email_snapshot, COALESCE(api_key_id,0), api_key_name_snapshot, group_id, group_name, provider, endpoint,
		protocol, model, prompt_hash, prompt_text, prompt_length, message_count, risk_status, risk_result::text,
		response_text, response_length, response_truncated, response_captured_at,
		risk_checked_at, created_at, expires_at, request_body, request_headers FROM prompt_records WHERE id=$1`, id).Scan(&item.ID, &item.RequestID, &item.TurnNo, &item.Stage, &item.UserID, &item.Username, &item.UserEmail, &item.APIKeyID, &item.APIKeyName, &groupID, &item.GroupName, &item.Provider, &item.Endpoint, &item.Protocol, &item.Model, &item.PromptHash, &item.PromptText, &item.PromptLength, &item.MessageCount, &item.RiskStatus, &item.RiskResult, &item.ResponseText, &item.ResponseLength, &item.ResponseTruncated, &responseCapturedAt, &riskCheckedAt, &item.CreatedAt, &expiresAt, &item.RequestBody, &item.RequestHeaders)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPromptRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	if groupID.Valid {
		item.GroupID = &groupID.Int64
	}
	if riskCheckedAt.Valid {
		item.RiskCheckedAt = &riskCheckedAt.Time
	}
	if responseCapturedAt.Valid {
		item.ResponseCapturedAt = &responseCapturedAt.Time
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
		if !expiresAt.Time.After(time.Now()) {
			return nil, ErrPromptRecordNotFound
		}
	}
	return item, nil
}

// A bounded batch and SKIP LOCKED allow independent instances to clean up
// expired records without holding locks on the entire retained history.
func (r *PostgreSQLRepository) DeleteExpiredPromptRecords(ctx context.Context, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("prompt record database unavailable")
	}
	if limit < 1 || limit > 1000 {
		limit = 1000
	}
	result, err := r.db.ExecContext(ctx, `WITH expired AS (
		SELECT id FROM prompt_records WHERE expires_at <= NOW()
		ORDER BY expires_at, id LIMIT $1 FOR UPDATE SKIP LOCKED
	) DELETE FROM prompt_records p USING expired e WHERE p.id=e.id`, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PostgreSQLRepository) DeletePromptRecord(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("prompt record database unavailable")
	}
	result, err := r.db.ExecContext(ctx, "DELETE FROM prompt_records WHERE id=$1", id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrPromptRecordNotFound
	}
	return nil
}

func (r *PostgreSQLRepository) DeletePromptRecords(ctx context.Context, ids []int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("prompt record database unavailable")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result, err := r.db.ExecContext(ctx, "DELETE FROM prompt_records WHERE id = ANY($1)", pq.Array(ids))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var ErrPromptRecordNotFound = errors.New("prompt record not found")

type PromptRecordRecorder interface {
	RecordPrompt(context.Context, Request)
}

type PromptResponseRecorder interface {
	RecordResponse(context.Context, Request, PromptResponse)
}

type promptRecordJob struct {
	request  Request
	response *PromptResponse
	bytes    int64
}

func NewPromptRecordService(repo PromptRecordRepository) *PromptRecordService {
	return newPromptRecordService(repo, promptRecordQueueCapacity, promptRecordOverflowCapacity, promptRecordWorkerCount)
}

func newPromptRecordService(repo PromptRecordRepository, queueCapacity, overflowCapacity, workers int) *PromptRecordService {
	if queueCapacity < 1 {
		queueCapacity = promptRecordQueueCapacity
	}
	if overflowCapacity < 1 {
		overflowCapacity = promptRecordOverflowCapacity
	}
	if workers < 1 {
		workers = promptRecordWorkerCount
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &PromptRecordService{
		repo: repo, queue: make(chan promptRecordJob, queueCapacity), overflow: make(chan promptRecordJob, overflowCapacity),
		responseQueue: make(chan promptRecordJob, queueCapacity), responseOverflow: make(chan promptRecordJob, overflowCapacity),
		workers: workers, requestSlots: make(chan struct{}, queueCapacity+overflowCapacity),
		responseSlots: make(chan struct{}, queueCapacity+overflowCapacity), pendingResponses: make(map[*promptRecordCorrelation]promptRecordJob),
		ctx: ctx, cancel: cancel, done: make(chan struct{}), cleanupStop: make(chan struct{}), byteLimit: promptRecordByteBudget, itemByteLimit: promptRecordItemByteLimit,
	}
}

func (s *PromptRecordService) persistJobSafely(job promptRecordJob) {
	defer func() {
		if recover() != nil {
			job.request.recordingCorrelation.complete(PromptRecordKey{}, false)
			code := "prompt_record_worker_panic"
			if job.response != nil {
				code = "prompt_record_response_worker_panic"
			}
			s.recordPersistFailure(job.request, code)
		}
	}()
	if job.response != nil {
		s.persistResponse(job.request, *job.response)
		return
	}
	s.persist(job.request)
}

func (s *PromptRecordService) RecordPrompt(_ context.Context, req Request) {
	if s == nil || s.repo == nil {
		req.recordingCorrelation.complete(PromptRecordKey{}, false)
		return
	}
	size := promptRecordRequestBytes(req)
	if !s.reserve(size, false, req) {
		req.recordingCorrelation.complete(PromptRecordKey{}, false)
		return
	}
	defer s.producers.Done()
	submitted := false
	defer func() {
		if !submitted {
			s.releaseJob(promptRecordJob{bytes: size}, true)
			req.recordingCorrelation.complete(PromptRecordKey{}, false)
		}
	}()
	if req.recordingSkipPrompt {
		sum := sha256.Sum256(req.Body)
		req.recordingIdentity = hex.EncodeToString(sum[:])
		req.Body = nil
	}
	if req.recordingSkipHeaders {
		req.Headers = nil
	}
	job := promptRecordJob{request: req.Clone(), bytes: size}
	submitted = s.enqueue(job, req, s.queue, s.overflow)
}

func (s *PromptRecordService) RecordResponse(_ context.Context, req Request, captured PromptResponse) {
	if s == nil || s.repo == nil {
		return
	}
	if req.recordingCorrelation == nil {
		s.recordPersistFailure(req, "prompt_record_response_identity_missing")
		return
	}
	requestReference := req
	requestReference.Body = nil
	requestReference.Headers = nil
	size := int64(len(captured.Text)) + promptRecordMetadataBytes(req)
	if !s.reserve(size, true, req) {
		return
	}
	defer s.producers.Done()
	// A truncated string may still retain a much larger backing allocation.
	// Own exactly the text covered by the reservation.
	captured.Text = strings.Clone(captured.Text)
	job := promptRecordJob{request: requestReference, response: &captured, bytes: size}
	s.pendingMu.Lock()
	if _, exists := s.pendingResponses[req.recordingCorrelation]; exists {
		s.pendingMu.Unlock()
		s.releaseJob(job, true)
		return
	}
	s.pendingResponses[req.recordingCorrelation] = job
	s.pendingMu.Unlock()
	if !req.recordingCorrelation.whenReady(func(_ PromptRecordKey, persisted bool) {
		s.pendingMu.Lock()
		pending, exists := s.pendingResponses[req.recordingCorrelation]
		delete(s.pendingResponses, req.recordingCorrelation)
		s.pendingMu.Unlock()
		if !exists {
			return
		}
		if !persisted {
			s.releaseJob(pending, true)
			s.recordPersistFailure(req, "prompt_record_response_request_failed")
			return
		}
		if !s.enqueue(pending, req, s.responseQueue, s.responseOverflow) {
			s.releaseJob(pending, true)
		}
	}) {
		s.pendingMu.Lock()
		delete(s.pendingResponses, req.recordingCorrelation)
		s.pendingMu.Unlock()
		s.releaseJob(job, true)
	}
}

func (s *PromptRecordService) enqueue(job promptRecordJob, req Request, queue, overflow chan<- promptRecordJob) bool {
	select {
	case queue <- job:
		return true
	default:
	}
	select {
	case overflow <- job:
		return true
	default:
		s.recordDrop(req, job.response != nil, "prompt_record_queue_full")
		return false
	}
}

func (s *PromptRecordService) persist(req Request) {
	prepared, err := preparePromptRecord(req)
	if err != nil {
		req.recordingCorrelation.complete(PromptRecordKey{}, false)
		s.recordPersistFailure(req, "prompt_record_prepare_failed")
		return
	}
	snapshot := prepared.StoredSnapshot
	key := promptRecordKey(req, prepared.OriginalPromptHash)
	record := &PromptRecord{RequestID: snapshot.RequestID, Stage: ifEmpty(snapshot.Stage, req.Stage), UserID: snapshot.UserID, Username: snapshot.UsernameSnapshot, UserEmail: snapshot.UserEmailSnapshot, APIKeyID: snapshot.APIKeyID, APIKeyName: snapshot.APIKeyNameSnapshot, GroupID: snapshot.GroupID, GroupName: snapshot.GroupName, Provider: snapshot.Provider, Endpoint: snapshot.Endpoint, Protocol: snapshot.Protocol, Model: snapshot.Model, PromptHash: key.PromptHash, PromptText: snapshot.FullPrompt, PromptLength: snapshot.PromptLength, MessageCount: snapshot.MessageCount, RiskStatus: "pending", CreatedAt: time.Now()}
	record.TurnNo = req.TurnNo
	if req.recordingRetentionDays > 0 {
		expiresAt := record.CreatedAt.AddDate(0, 0, req.recordingRetentionDays)
		record.ExpiresAt = &expiresAt
	}
	if !req.recordingSkipPrompt {
		record.RequestBody = string(prepared.StoredBody)
		// The body is the canonical stored document. Legacy prompt_text remains
		// readable; new detail responses derive it lazily when needed.
		record.PromptText = ""
	} else {
		record.PromptText = ""
		record.PromptLength = 0
		record.MessageCount = 0
	}
	if !req.recordingSkipHeaders {
		headers := req.Headers
		if headers == nil {
			headers = make(map[string][]string)
		}
		encoded, err := json.Marshal(headers)
		if err != nil {
			req.recordingCorrelation.complete(key, false)
			s.recordPersistFailure(req, "prompt_record_headers_encode_failed")
			return
		}
		record.RequestHeaders = string(encoded)
	}
	ctx, cancel := context.WithTimeout(s.ctx, promptRecordPersistTimeout)
	defer cancel()
	if err := s.repo.InsertPromptRecord(ctx, record); err != nil {
		req.recordingCorrelation.complete(key, false)
		s.recordPersistFailure(req, "prompt_record_insert_failed")
		return
	}
	req.recordingCorrelation.complete(key, true)
}

func (s *PromptRecordService) persistResponse(req Request, captured PromptResponse) {
	key, persisted := req.recordingCorrelation.result()
	if !persisted {
		s.recordPersistFailure(req, "prompt_record_response_identity_unavailable")
		return
	}
	for attempt := 0; attempt < promptResponsePersistAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(s.ctx, promptRecordPersistTimeout)
		updated, updateErr := s.repo.UpdatePromptRecordResponse(ctx, key, captured)
		cancel()
		if updateErr != nil {
			s.recordPersistFailure(req, "prompt_record_response_update_failed")
			return
		}
		if updated {
			return
		}
		if attempt+1 < promptResponsePersistAttempts {
			select {
			case <-time.After(promptResponsePersistDelay):
			case <-s.ctx.Done():
				return
			}
		}
	}
	s.recordPersistFailure(req, "prompt_record_response_not_found")
}

// Requests without extractable text still create a record and use the original
// request identity for the response update. Multimodal payloads are not retained.
func promptRecordSnapshot(req Request) PromptSnapshot {
	snapshot, err := ExtractPromptSnapshot(req)
	if err == nil {
		return snapshot
	}
	return promptRecordFallbackSnapshot(req, req.Body)
}

func (s *PromptRecordService) recordPersistFailure(req Request, code string) {
	if strings.Contains(code, "response") {
		s.responseFailed.Add(1)
	} else {
		s.requestFailed.Add(1)
	}
	failed := s.persistFailed.Add(1)
	if isPowerOfTwo(failed) {
		LogError(EventPromptRecordPersistFailed, mergeLogFields(requestLogFields(req), map[string]any{
			"status": "failed", "error_code": code,
		}))
	}
}

func (s *PromptRecordService) ListPromptRecords(ctx context.Context, filter PromptRecordFilter, page, pageSize int) (*PromptRecordPage, error) {
	result, err := s.repo.ListPromptRecords(ctx, filter, page, pageSize)
	if err != nil || result == nil {
		return result, err
	}
	result.Queue = s.QueueStats()
	return result, nil
}
func (s *PromptRecordService) GetPromptRecord(ctx context.Context, id int64) (*PromptRecord, error) {
	item, err := s.repo.GetPromptRecord(ctx, id)
	if err == nil && item != nil && item.PromptText == "" && item.RequestBody != "" {
		if document, decodeErr := decodePromptDocument([]byte(item.RequestBody)); decodeErr == nil {
			item.PromptText = promptRecordSnapshotDocument(Request{Protocol: item.Protocol}, document, nil).FullPrompt
		}
	}
	return item, err
}
func (s *PromptRecordService) DeletePromptRecord(ctx context.Context, id int64) error {
	return s.repo.DeletePromptRecord(ctx, id)
}
func (s *PromptRecordService) DeletePromptRecords(ctx context.Context, ids []int64) (int64, error) {
	return s.repo.DeletePromptRecords(ctx, ids)
}

func (s *PromptRecordService) QueueStats() PromptRecordQueueStats {
	if s == nil {
		return PromptRecordQueueStats{}
	}
	stats := PromptRecordQueueStats{
		QueueLength: len(s.queue) + len(s.responseQueue), QueueCapacity: cap(s.queue) + cap(s.responseQueue),
		OverflowLength: len(s.overflow) + len(s.responseOverflow), OverflowCapacity: cap(s.overflow) + cap(s.responseOverflow),
		WorkerCount: s.workers + promptResponseWorkerCount, DroppedTotal: s.dropped.Load(), PersistFailed: s.persistFailed.Load(),
		InFlightBytes: s.inFlightBytes.Load(), ByteCapacity: s.byteLimit, ItemByteLimit: s.itemByteLimit,
		RequestDropped: s.requestDropped.Load(), ResponseDropped: s.responseDropped.Load(),
		RequestFailed: s.requestFailed.Load(), ResponseFailed: s.responseFailed.Load(),
		ExpiredDeleted: s.expiredDeleted.Load(), CleanupFailed: s.cleanupFailed.Load(), CleanupBacklog: s.cleanupBacklog.Load(),
	}
	s.pendingMu.Lock()
	stats.PendingResponses = len(s.pendingResponses)
	s.pendingMu.Unlock()
	if value := s.lastDroppedAt.Load(); value != nil {
		lastDroppedAt := *value
		stats.LastDroppedAt = &lastDroppedAt
	}
	return stats
}

func isPowerOfTwo(value int64) bool {
	return value > 0 && value&(value-1) == 0
}

func ifEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
