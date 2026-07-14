package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	usageBillingPendingPrefix       = "billing:usage:pending:"
	usageBillingMutationPrefix      = "billing:usage:mutation:"
	usageBillingOverlayPrefix       = "billing:usage:overlay:"
	usageBillingEnqueueBatchMaxSize = 256
	usageBillingEnqueueBatchWindow  = 3 * time.Millisecond
	usageBillingEnqueueQueueSize    = 32768
	usageBillingMutationTTL         = 24 * time.Hour
)

var errUsageBillingJobPayloadInvalid = errors.New("usage billing job payload is invalid")

const usageBillingEnqueueBatchSQL = `
	WITH input AS (
		SELECT request_id, api_key_id, request_fingerprint, payload
		FROM jsonb_to_recordset($1::jsonb) AS x(
			request_id text,
			api_key_id bigint,
			request_fingerprint text,
			payload jsonb
		)
	),
	eligible AS (
		SELECT i.*
		FROM input i
		LEFT JOIN usage_billing_dedup d
			ON d.request_id = i.request_id AND d.api_key_id = i.api_key_id
		LEFT JOIN usage_billing_dedup_archive a
			ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
		WHERE d.id IS NULL AND a.request_id IS NULL
	),
	inserted AS (
		INSERT INTO usage_billing_jobs (
			request_id, api_key_id, request_fingerprint, payload
		)
		SELECT request_id, api_key_id, request_fingerprint, payload
		FROM eligible
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id, request_id, api_key_id, request_fingerprint
	)
	SELECT
		i.request_id,
		i.api_key_id,
		COALESCE(ins.id, j.id, 0) AS job_id,
		CASE
			WHEN ins.id IS NOT NULL THEN 'inserted'
			WHEN j.id IS NOT NULL AND j.request_fingerprint = i.request_fingerprint THEN 'pending'
			WHEN j.id IS NOT NULL THEN 'conflict'
			WHEN d.id IS NOT NULL AND d.request_fingerprint = i.request_fingerprint THEN 'applied'
			WHEN d.id IS NOT NULL THEN 'conflict'
			WHEN a.request_id IS NOT NULL AND a.request_fingerprint = i.request_fingerprint THEN 'applied'
			WHEN a.request_id IS NOT NULL THEN 'conflict'
			ELSE 'conflict'
		END AS status
	FROM input i
	LEFT JOIN inserted ins
		ON ins.request_id = i.request_id AND ins.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_jobs j
		ON j.request_id = i.request_id AND j.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup d
		ON d.request_id = i.request_id AND d.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup_archive a
		ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
`

const usageBillingClaimBatchSQL = `
	WITH input AS (
		SELECT job_id, request_id, api_key_id, request_fingerprint
		FROM jsonb_to_recordset($1::jsonb) AS x(
			job_id bigint,
			request_id text,
			api_key_id bigint,
			request_fingerprint text
		)
	),
	eligible AS (
		SELECT i.*
		FROM input i
		LEFT JOIN usage_billing_dedup_archive a
			ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
		WHERE a.request_id IS NULL
	),
	inserted AS (
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		SELECT request_id, api_key_id, request_fingerprint
		FROM eligible
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING request_id, api_key_id, request_fingerprint
	)
	SELECT
		i.job_id,
		CASE
			WHEN ins.request_id IS NOT NULL THEN 'inserted'
			WHEN d.id IS NOT NULL AND d.request_fingerprint = i.request_fingerprint THEN 'applied'
			WHEN d.id IS NOT NULL THEN 'conflict'
			WHEN a.request_id IS NOT NULL AND a.request_fingerprint = i.request_fingerprint THEN 'applied'
			ELSE 'conflict'
		END AS status
	FROM input i
	LEFT JOIN inserted ins
		ON ins.request_id = i.request_id AND ins.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup d
		ON d.request_id = i.request_id AND d.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup_archive a
		ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
`

var usageBillingOverlayScript = redis.NewScript(`
	local existing = redis.call('GET', KEYS[13])
	if existing then
		if existing == ARGV[6] then
			return 0
		end
		return redis.error_reply('usage billing overlay fingerprint conflict')
	end

	redis.call('SET', KEYS[13], ARGV[6])
	local balance_cost = tonumber(ARGV[1]) or 0
	local subscription_cost = tonumber(ARGV[2]) or 0
	local api_quota_cost = tonumber(ARGV[3]) or 0
	local rate_limit_cost = tonumber(ARGV[4]) or 0

	if balance_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[3], balance_cost)
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], ARGV[5])
	end
	if subscription_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[4], subscription_cost)
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], ARGV[5])
	end
	if api_quota_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[5], api_quota_cost)
	end
	if rate_limit_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[6], rate_limit_cost)
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], ARGV[5])
	end
	return 1
`)

var usageBillingCompleteOverlayScript = redis.NewScript(`
	local function subtract_pending(key, amount)
		if amount <= 0 then
			return
		end
		local current = tonumber(redis.call('GET', key) or '0')
		local updated = current - amount
		if updated <= 0.0000000001 then
			redis.call('DEL', key)
		else
			redis.call('SET', key, tostring(updated))
		end
	end

	local balance_cost = tonumber(ARGV[1]) or 0
	local subscription_cost = tonumber(ARGV[2]) or 0
	local api_quota_cost = tonumber(ARGV[3]) or 0
	local rate_limit_cost = tonumber(ARGV[4]) or 0
	local marker = redis.call('GET', KEYS[13])
	if marker == ARGV[6] then
		subtract_pending(KEYS[3], balance_cost)
		subtract_pending(KEYS[4], subscription_cost)
		subtract_pending(KEYS[5], api_quota_cost)
		subtract_pending(KEYS[6], rate_limit_cost)
		redis.call('DEL', KEYS[13])
	end

	if balance_cost > 0 then
		redis.call('DEL', KEYS[7])
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], ARGV[5])
	end
	if subscription_cost > 0 then
		redis.call('DEL', KEYS[8])
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], ARGV[5])
	end
	if rate_limit_cost > 0 then
		redis.call('DEL', KEYS[9])
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], ARGV[5])
	end
	return 1
`)

type usageBillingEnqueueStatus string

const (
	usageBillingEnqueueInserted usageBillingEnqueueStatus = "inserted"
	usageBillingEnqueuePending  usageBillingEnqueueStatus = "pending"
	usageBillingEnqueueApplied  usageBillingEnqueueStatus = "applied"
	usageBillingEnqueueConflict usageBillingEnqueueStatus = "conflict"
)

type usageBillingEnqueueRequest struct {
	cmd      service.UsageBillingCommand
	payload  json.RawMessage
	resultCh chan usageBillingEnqueueResult
}

type usageBillingEnqueueResult struct {
	status usageBillingEnqueueStatus
	jobID  int64
	err    error
}

type usageBillingBatchInput struct {
	RequestID          string          `json:"request_id"`
	APIKeyID           int64           `json:"api_key_id"`
	RequestFingerprint string          `json:"request_fingerprint"`
	Payload            json.RawMessage `json:"payload"`
}

type usageBillingClaimInput struct {
	JobID              int64  `json:"job_id"`
	RequestID          string `json:"request_id"`
	APIKeyID           int64  `json:"api_key_id"`
	RequestFingerprint string `json:"request_fingerprint"`
}

type usageBillingJob struct {
	id                 int64
	requestID          string
	apiKeyID           int64
	requestFingerprint string
	payload            []byte
	attempts           int
	createdAt          time.Time
}

type usageBillingCompletion struct {
	cmd service.UsageBillingCommand
}

type usageBillingPlatformQuotaAggregate struct {
	userID   int64
	platform string
	amount   float64
}

type queuedUsageBillingRepository struct {
	direct         *usageBillingRepository
	db             *sql.DB
	rdb            *redis.Client
	consumerCount  int
	readBatchSize  int
	pollInterval   time.Duration
	commandTimeout time.Duration
	maxRetryDelay  time.Duration

	enqueueCh chan usageBillingEnqueueRequest
	wakeCh    chan struct{}
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	stopped   atomic.Bool
	lifecycle sync.RWMutex
}

// ProvideUsageBillingRepository uses a PostgreSQL WAL-backed queue in
// production. Redis is optional and only accelerates pending-usage visibility.
func ProvideUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB, rdb *redis.Client, cfg *config.Config) service.UsageBillingRepository {
	direct := &usageBillingRepository{db: sqlDB}
	if cfg == nil || !cfg.Billing.Queue.Enabled || sqlDB == nil {
		return direct
	}
	queueCfg := cfg.Billing.Queue
	consumerCount := max(1, queueCfg.ConsumerCount)
	if queueCfg.MaxConsumerCount > 0 {
		consumerCount = min(consumerCount, queueCfg.MaxConsumerCount)
	}
	repo := &queuedUsageBillingRepository{
		direct:         direct,
		db:             sqlDB,
		rdb:            rdb,
		consumerCount:  consumerCount,
		readBatchSize:  max(1, queueCfg.ReadBatchSize),
		pollInterval:   time.Duration(max(1, queueCfg.ReadBlockMilliseconds)) * time.Millisecond,
		commandTimeout: time.Duration(max(1, queueCfg.CommandTimeoutSeconds)) * time.Second,
		maxRetryDelay:  time.Duration(max(1, queueCfg.MaxRetryDelaySeconds)) * time.Second,
		enqueueCh:      make(chan usageBillingEnqueueRequest, usageBillingEnqueueQueueSize),
		wakeCh:         make(chan struct{}, consumerCount),
	}
	repo.recoverPendingOverlays()
	repo.start()
	return repo
}

func (r *queuedUsageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("durable usage billing queue db is nil")
	}
	cloned := cloneUsageBillingCommand(cmd)
	cloned.Normalize()
	if cloned.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}
	payload, err := json.Marshal(&cloned)
	if err != nil {
		return nil, fmt.Errorf("marshal usage billing command: %w", err)
	}
	req := usageBillingEnqueueRequest{
		cmd:      cloned,
		payload:  payload,
		resultCh: make(chan usageBillingEnqueueResult, 1),
	}

	r.lifecycle.RLock()
	if r.stopped.Load() {
		r.lifecycle.RUnlock()
		return nil, errors.New("durable usage billing queue is stopped")
	}
	select {
	case r.enqueueCh <- req:
		r.lifecycle.RUnlock()
	case <-ctx.Done():
		r.lifecycle.RUnlock()
		return nil, ctx.Err()
	}

	select {
	case result := <-req.resultCh:
		if result.err != nil {
			return nil, result.err
		}
		switch result.status {
		case usageBillingEnqueueInserted:
			r.reconcilePendingOverlay(&cloned)
			r.wakeConsumers()
			return &service.UsageBillingApplyResult{Applied: true, Deferred: true}, nil
		case usageBillingEnqueuePending:
			// Rebuild the Redis overlay after a Redis restart without double-counting.
			r.reconcilePendingOverlay(&cloned)
			return &service.UsageBillingApplyResult{Applied: false, Deferred: true}, nil
		case usageBillingEnqueueApplied:
			return &service.UsageBillingApplyResult{Applied: false}, nil
		default:
			return nil, service.ErrUsageBillingRequestConflict
		}
	case <-ctx.Done():
		// The batcher uses its own context and will still durably persist this
		// accepted request. A retry is safe because request_id is idempotent.
		return nil, ctx.Err()
	}
}

func cloneUsageBillingCommand(cmd *service.UsageBillingCommand) service.UsageBillingCommand {
	cloned := *cmd
	if cmd.SubscriptionID != nil {
		subscriptionID := *cmd.SubscriptionID
		cloned.SubscriptionID = &subscriptionID
	}
	return cloned
}

func (r *queuedUsageBillingRepository) ReserveBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.ReserveBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) CaptureBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.CaptureBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) ReleaseBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.ReleaseBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go r.runEnqueueBatcher(ctx)
	for i := 0; i < r.consumerCount; i++ {
		r.wg.Add(1)
		go r.runConsumer(ctx, i)
	}
}

func (r *queuedUsageBillingRepository) Stop() {
	if r == nil {
		return
	}
	r.lifecycle.Lock()
	if !r.stopped.CompareAndSwap(false, true) {
		r.lifecycle.Unlock()
		return
	}
	r.lifecycle.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *queuedUsageBillingRepository) runEnqueueBatcher(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case first := <-r.enqueueCh:
			batch := r.collectEnqueueBatch(ctx, first)
			r.flushEnqueueBatch(batch)
		case <-ctx.Done():
			for {
				select {
				case first := <-r.enqueueCh:
					batch := r.collectEnqueueBatch(context.Background(), first)
					r.flushEnqueueBatch(batch)
				default:
					return
				}
			}
		}
	}
}

func (r *queuedUsageBillingRepository) collectEnqueueBatch(ctx context.Context, first usageBillingEnqueueRequest) []usageBillingEnqueueRequest {
	batch := make([]usageBillingEnqueueRequest, 0, usageBillingEnqueueBatchMaxSize)
	batch = append(batch, first)
	timer := time.NewTimer(usageBillingEnqueueBatchWindow)
	defer timer.Stop()
	for len(batch) < usageBillingEnqueueBatchMaxSize {
		select {
		case req := <-r.enqueueCh:
			batch = append(batch, req)
		case <-timer.C:
			return batch
		case <-ctx.Done():
			return batch
		}
	}
	return batch
}

func (r *queuedUsageBillingRepository) flushEnqueueBatch(batch []usageBillingEnqueueRequest) {
	if len(batch) == 0 {
		return
	}
	unique := make([]usageBillingEnqueueRequest, 0, len(batch))
	byKey := make(map[string]int, len(batch))
	conflicted := make(map[int]struct{})
	for i, req := range batch {
		key := usageBillingRequestKey(req.cmd.RequestID, req.cmd.APIKeyID)
		idx, exists := byKey[key]
		if !exists {
			byKey[key] = len(unique)
			unique = append(unique, req)
			continue
		}
		if unique[idx].cmd.RequestFingerprint != req.cmd.RequestFingerprint {
			conflicted[i] = struct{}{}
		}
	}

	inputs := make([]usageBillingBatchInput, 0, len(unique))
	for _, req := range unique {
		inputs = append(inputs, usageBillingBatchInput{
			RequestID:          req.cmd.RequestID,
			APIKeyID:           req.cmd.APIKeyID,
			RequestFingerprint: req.cmd.RequestFingerprint,
			Payload:            req.payload,
		})
	}
	payload, err := json.Marshal(inputs)
	results := make(map[string]usageBillingEnqueueResult, len(unique))
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), r.commandTimeout)
		results, err = r.insertEnqueueBatch(ctx, payload)
		cancel()
	}
	if err != nil {
		for _, req := range batch {
			req.resultCh <- usageBillingEnqueueResult{err: err}
		}
		return
	}

	seen := make(map[string]bool, len(unique))
	for i, req := range batch {
		if _, conflict := conflicted[i]; conflict {
			req.resultCh <- usageBillingEnqueueResult{status: usageBillingEnqueueConflict}
			continue
		}
		key := usageBillingRequestKey(req.cmd.RequestID, req.cmd.APIKeyID)
		result, ok := results[key]
		if !ok {
			result.err = errors.New("durable usage billing enqueue result missing")
		}
		if seen[key] && result.status == usageBillingEnqueueInserted {
			result.status = usageBillingEnqueuePending
		}
		seen[key] = true
		req.resultCh <- result
	}
	r.wakeConsumers()
}

func (r *queuedUsageBillingRepository) insertEnqueueBatch(ctx context.Context, payload []byte) (map[string]usageBillingEnqueueResult, error) {
	rows, err := r.db.QueryContext(ctx, usageBillingEnqueueBatchSQL, payload)
	if err != nil {
		return nil, fmt.Errorf("insert durable usage billing batch: %w", err)
	}
	defer rows.Close()
	results := make(map[string]usageBillingEnqueueResult)
	for rows.Next() {
		var requestID, status string
		var apiKeyID, jobID int64
		if err := rows.Scan(&requestID, &apiKeyID, &jobID, &status); err != nil {
			return nil, err
		}
		results[usageBillingRequestKey(requestID, apiKeyID)] = usageBillingEnqueueResult{
			status: usageBillingEnqueueStatus(status),
			jobID:  jobID,
		}
	}
	return results, rows.Err()
}

func (r *queuedUsageBillingRepository) runConsumer(ctx context.Context, workerID int) {
	defer r.wg.Done()
	for ctx.Err() == nil {
		processed, err := r.processJobBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("durable usage billing consumer failed", "worker", workerID, "error", err)
		}
		if processed > 0 {
			continue
		}
		timer := time.NewTimer(r.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-r.wakeCh:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (r *queuedUsageBillingRepository) processJobBatch(parent context.Context) (_ int, err error) {
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE available_at <= NOW()
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, r.readBatchSize)
	if err != nil {
		return 0, err
	}
	jobs := make([]usageBillingJob, 0, r.readBatchSize)
	for rows.Next() {
		var job usageBillingJob
		if err := rows.Scan(&job.id, &job.requestID, &job.apiKeyID, &job.requestFingerprint, &job.payload, &job.attempts, &job.createdAt); err != nil {
			_ = rows.Close()
			return 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}

	completions, fastErr := r.applyJobBatchFast(ctx, tx, jobs)
	if fastErr != nil {
		_ = tx.Rollback()
		tx = nil
		// Isolate an invalid or concurrently deleted entity without degrading the
		// normal batch path. The next loop retries the remaining healthy jobs.
		return r.processSingleJob(parent)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	for _, completion := range completions {
		r.completePendingOverlay(&completion.cmd)
	}
	return len(jobs), nil
}

func (r *queuedUsageBillingRepository) processSingleJob(parent context.Context) (_ int, err error) {
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE available_at <= NOW()
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`)
	if err != nil {
		return 0, err
	}
	var job usageBillingJob
	if !rows.Next() {
		_ = rows.Close()
		return 0, nil
	}
	if err := rows.Scan(&job.id, &job.requestID, &job.apiKeyID, &job.requestFingerprint, &job.payload, &job.attempts, &job.createdAt); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	cmd, _, err := r.applyJobWithSavepoint(ctx, tx, job)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	if cmd != nil {
		r.completePendingOverlay(cmd)
	}
	return 1, nil
}

func (r *queuedUsageBillingRepository) applyJobBatchFast(ctx context.Context, tx *sql.Tx, jobs []usageBillingJob) ([]usageBillingCompletion, error) {
	commands := make(map[int64]*service.UsageBillingCommand, len(jobs))
	jobsByID := make(map[int64]usageBillingJob, len(jobs))
	claimInputs := make([]usageBillingClaimInput, 0, len(jobs))
	completions := make([]usageBillingCompletion, 0, len(jobs))
	for _, job := range jobs {
		jobsByID[job.id] = job
		var cmd service.UsageBillingCommand
		if err := json.Unmarshal(job.payload, &cmd); err != nil {
			if deadErr := deadLetterUsageBillingJob(ctx, tx, job, fmt.Sprintf("%v: %v", errUsageBillingJobPayloadInvalid, err)); deadErr != nil {
				return nil, deadErr
			}
			continue
		}
		cmd.Normalize()
		if cmd.RequestID != job.requestID || cmd.APIKeyID != job.apiKeyID || cmd.RequestFingerprint != job.requestFingerprint {
			if deadErr := deadLetterUsageBillingJob(ctx, tx, job, errUsageBillingJobPayloadInvalid.Error()+": identity mismatch"); deadErr != nil {
				return nil, deadErr
			}
			continue
		}
		commands[job.id] = &cmd
		claimInputs = append(claimInputs, usageBillingClaimInput{
			JobID:              job.id,
			RequestID:          job.requestID,
			APIKeyID:           job.apiKeyID,
			RequestFingerprint: job.requestFingerprint,
		})
	}
	if len(claimInputs) == 0 {
		return completions, nil
	}
	payload, err := json.Marshal(claimInputs)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, usageBillingClaimBatchSQL, payload)
	if err != nil {
		return nil, err
	}
	claimStatus := make(map[int64]usageBillingEnqueueStatus, len(claimInputs))
	for rows.Next() {
		var jobID int64
		var status string
		if err := rows.Scan(&jobID, &status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		claimStatus[jobID] = usageBillingEnqueueStatus(status)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	inserted := make([]*service.UsageBillingCommand, 0, len(commands))
	terminalIDs := make([]int64, 0, len(commands))
	for jobID, cmd := range commands {
		status, ok := claimStatus[jobID]
		if !ok {
			return nil, errors.New("durable usage billing claim result missing")
		}
		switch status {
		case usageBillingEnqueueInserted:
			inserted = append(inserted, cmd)
			terminalIDs = append(terminalIDs, jobID)
			completions = append(completions, usageBillingCompletion{cmd: *cmd})
		case usageBillingEnqueueApplied:
			terminalIDs = append(terminalIDs, jobID)
			completions = append(completions, usageBillingCompletion{cmd: *cmd})
		default:
			if err := deadLetterUsageBillingJob(ctx, tx, jobsByID[jobID], service.ErrUsageBillingRequestConflict.Error()); err != nil {
				return nil, err
			}
			completions = append(completions, usageBillingCompletion{cmd: *cmd})
		}
	}
	if err := applyAggregatedUsageBillingEffects(ctx, tx, inserted); err != nil {
		return nil, err
	}
	if len(terminalIDs) > 0 {
		idsJSON, err := json.Marshal(terminalIDs)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM usage_billing_jobs
			WHERE id IN (
				SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb)
			)
		`, idsJSON); err != nil {
			return nil, err
		}
	}
	return completions, nil
}

func applyAggregatedUsageBillingEffects(ctx context.Context, tx *sql.Tx, commands []*service.UsageBillingCommand) error {
	balances := make(map[int64]float64)
	subscriptions := make(map[int64]float64)
	apiKeyQuotas := make(map[int64]float64)
	apiKeyRateLimits := make(map[int64]float64)
	accountQuotas := make(map[int64]float64)
	platformQuotas := make(map[string]*usageBillingPlatformQuotaAggregate)
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		if cmd.BalanceCost > 0 {
			balances[cmd.UserID] += cmd.BalanceCost
		}
		if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
			subscriptions[*cmd.SubscriptionID] += cmd.SubscriptionCost
		}
		if cmd.APIKeyQuotaCost > 0 {
			apiKeyQuotas[cmd.APIKeyID] += cmd.APIKeyQuotaCost
		}
		if cmd.APIKeyRateLimitCost > 0 {
			apiKeyRateLimits[cmd.APIKeyID] += cmd.APIKeyRateLimitCost
		}
		if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) || strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
			accountQuotas[cmd.AccountID] += cmd.AccountQuotaCost
		}
		if cmd.UserPlatformQuotaCost > 0 && strings.TrimSpace(cmd.QuotaPlatform) != "" {
			platform := strings.TrimSpace(cmd.QuotaPlatform)
			key := strconv.FormatInt(cmd.UserID, 10) + "\x00" + platform
			aggregate := platformQuotas[key]
			if aggregate == nil {
				aggregate = &usageBillingPlatformQuotaAggregate{userID: cmd.UserID, platform: platform}
				platformQuotas[key] = aggregate
			}
			aggregate.amount += cmd.UserPlatformQuotaCost
		}
	}
	for subscriptionID, amount := range subscriptions {
		if err := incrementUsageBillingSubscription(ctx, tx, subscriptionID, amount); err != nil {
			return err
		}
	}
	for userID, amount := range balances {
		if _, _, err := deductUsageBillingBalance(ctx, tx, userID, amount); err != nil {
			return err
		}
	}
	for apiKeyID, amount := range apiKeyQuotas {
		if _, err := incrementUsageBillingAPIKeyQuota(ctx, tx, apiKeyID, amount); err != nil {
			return err
		}
	}
	for apiKeyID, amount := range apiKeyRateLimits {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, apiKeyID, amount); err != nil {
			return err
		}
	}
	for accountID, amount := range accountQuotas {
		if _, err := incrementUsageBillingAccountQuota(ctx, tx, accountID, amount); err != nil {
			return err
		}
	}
	for _, quota := range platformQuotas {
		if err := incrementUsageBillingUserPlatformQuota(ctx, tx, quota.userID, quota.platform, quota.amount); err != nil {
			return err
		}
	}
	return nil
}

func (r *queuedUsageBillingRepository) applyJobWithSavepoint(ctx context.Context, tx *sql.Tx, job usageBillingJob) (*service.UsageBillingCommand, bool, error) {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT usage_billing_job"); err != nil {
		return nil, false, err
	}
	var cmd service.UsageBillingCommand
	err := json.Unmarshal(job.payload, &cmd)
	if err != nil {
		err = fmt.Errorf("%w: %v", errUsageBillingJobPayloadInvalid, err)
	}
	if err == nil {
		cmd.Normalize()
		if cmd.RequestID != job.requestID || cmd.APIKeyID != job.apiKeyID || cmd.RequestFingerprint != job.requestFingerprint {
			err = fmt.Errorf("%w: identity mismatch", errUsageBillingJobPayloadInvalid)
		}
	}
	if err == nil {
		var applied bool
		applied, err = r.direct.claimUsageBillingKey(ctx, tx, &cmd)
		if err == nil && applied {
			result := &service.UsageBillingApplyResult{Applied: true}
			err = r.direct.applyUsageBillingEffects(ctx, tx, &cmd, result)
		}
		if err == nil && !applied {
			if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT usage_billing_job"); rollbackErr != nil {
				return nil, false, rollbackErr
			}
		}
		if err == nil {
			if _, deleteErr := tx.ExecContext(ctx, "DELETE FROM usage_billing_jobs WHERE id = $1", job.id); deleteErr != nil {
				return nil, false, deleteErr
			}
			if _, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT usage_billing_job"); releaseErr != nil {
				return nil, false, releaseErr
			}
			return &cmd, applied, nil
		}
	}

	if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT usage_billing_job"); rollbackErr != nil {
		return nil, false, rollbackErr
	}
	if isPermanentUsageBillingError(err) {
		if deadErr := deadLetterUsageBillingJob(ctx, tx, job, err.Error()); deadErr != nil {
			return nil, false, deadErr
		}
		if _, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT usage_billing_job"); releaseErr != nil {
			return nil, false, releaseErr
		}
		if cmd.RequestID == "" {
			return nil, false, nil
		}
		return &cmd, false, nil
	}

	delay := usageBillingRetryDelay(job.attempts+1, r.maxRetryDelay)
	if _, updateErr := tx.ExecContext(ctx, `
		UPDATE usage_billing_jobs
		SET attempts = attempts + 1,
			last_error = $2,
			available_at = NOW() + ($3 * INTERVAL '1 millisecond'),
			updated_at = NOW()
		WHERE id = $1
	`, job.id, truncateUsageBillingError(err), delay.Milliseconds()); updateErr != nil {
		return nil, false, updateErr
	}
	if _, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT usage_billing_job"); releaseErr != nil {
		return nil, false, releaseErr
	}
	return nil, false, nil
}

func deadLetterUsageBillingJob(ctx context.Context, tx *sql.Tx, job usageBillingJob, reason string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_billing_dead_letters (
			source_job_id, request_id, api_key_id, request_fingerprint,
			payload, attempts, reason, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (request_id, api_key_id) DO UPDATE SET
			reason = EXCLUDED.reason,
			attempts = EXCLUDED.attempts,
			failed_at = NOW()
	`, job.id, job.requestID, job.apiKeyID, job.requestFingerprint, job.payload, job.attempts+1, truncateUsageBillingError(errors.New(reason)), job.createdAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM usage_billing_jobs WHERE id = $1", job.id)
	return err
}

func usageBillingRetryDelay(attempt int, maxDelay time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 10)
	delay := time.Second * time.Duration(1<<shift)
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	return delay
}

func truncateUsageBillingError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	return message
}

func (r *queuedUsageBillingRepository) wakeConsumers() {
	for i := 0; i < r.consumerCount; i++ {
		select {
		case r.wakeCh <- struct{}{}:
		default:
			return
		}
	}
}

func (r *queuedUsageBillingRepository) reconcilePendingOverlay(cmd *service.UsageBillingCommand) {
	if r == nil || r.rdb == nil || cmd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	keys := usageBillingRedisKeys(cmd)
	if _, err := usageBillingOverlayScript.Run(ctx, r.rdb, keys,
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		int64(usageBillingMutationTTL/time.Second),
		cmd.RequestFingerprint,
	).Result(); err != nil {
		slog.Warn("durable usage billing Redis overlay failed", "request_id", cmd.RequestID, "error", err)
	}
}

func (r *queuedUsageBillingRepository) recoverPendingOverlays() {
	if r == nil || r.rdb == nil || r.db == nil {
		return
	}
	timeout := max(30*time.Second, r.commandTimeout*2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := r.rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("skip durable usage billing Redis overlay recovery", "error", err)
		return
	}
	if _, err := usageBillingOverlayScript.Load(ctx, r.rdb).Result(); err != nil {
		slog.Warn("load durable usage billing Redis overlay script failed", "error", err)
		return
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT payload
		FROM usage_billing_jobs
		ORDER BY id
	`)
	if err != nil {
		slog.Warn("query durable usage billing overlays failed", "error", err)
		return
	}
	defer rows.Close()

	const pipelineSize = 256
	pipe := r.rdb.Pipeline()
	queued := 0
	recovered := 0
	flush := func() bool {
		if queued == 0 {
			return true
		}
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			slog.Warn("recover durable usage billing Redis overlays failed", "error", execErr)
			return false
		}
		recovered += queued
		queued = 0
		pipe = r.rdb.Pipeline()
		return true
	}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			slog.Warn("scan durable usage billing overlay failed", "error", err)
			return
		}
		var cmd service.UsageBillingCommand
		if err := json.Unmarshal(payload, &cmd); err != nil {
			continue
		}
		cmd.Normalize()
		pipe.EvalSha(ctx, usageBillingOverlayScript.Hash(), usageBillingRedisKeys(&cmd),
			cmd.BalanceCost,
			cmd.SubscriptionCost,
			cmd.APIKeyQuotaCost,
			cmd.APIKeyRateLimitCost,
			int64(usageBillingMutationTTL/time.Second),
			cmd.RequestFingerprint,
		)
		queued++
		if queued >= pipelineSize && !flush() {
			return
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("iterate durable usage billing overlays failed", "error", err)
		return
	}
	if !flush() {
		return
	}
	if recovered > 0 {
		slog.Info("durable usage billing Redis overlays recovered", "jobs", recovered)
	}
}

func (r *queuedUsageBillingRepository) completePendingOverlay(cmd *service.UsageBillingCommand) {
	if r == nil || r.rdb == nil || cmd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	keys := usageBillingRedisKeys(cmd)
	if _, err := usageBillingCompleteOverlayScript.Run(ctx, r.rdb, keys,
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		int64(usageBillingMutationTTL/time.Second),
		cmd.RequestFingerprint,
	).Result(); err != nil {
		slog.Warn("durable usage billing Redis overlay completion failed", "request_id", cmd.RequestID, "error", err)
	}
	if cmd.APIKeyQuotaCost > 0 && cmd.APIKeyAuthCacheKey != "" {
		pipe := r.rdb.Pipeline()
		pipe.Del(ctx, apiKeyAuthCacheKey(cmd.APIKeyAuthCacheKey))
		pipe.Publish(ctx, authCacheInvalidateChannel, cmd.APIKeyAuthCacheKey)
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Warn("durable usage billing API key cache invalidation failed", "request_id", cmd.RequestID, "error", err)
		}
	}
}

func usageBillingRedisKeys(cmd *service.UsageBillingCommand) []string {
	userID, groupID, apiKeyID := int64(0), int64(0), int64(0)
	requestID := "invalid"
	if cmd != nil {
		userID, groupID, apiKeyID = cmd.UserID, cmd.GroupID, cmd.APIKeyID
		requestID = cmd.RequestID
	}
	return []string{
		"billing:usage:durable",
		usageBillingOverlayKey(requestID, apiKeyID),
		usageBillingPendingBalanceKey(userID),
		usageBillingPendingSubscriptionKey(userID, groupID),
		usageBillingPendingAPIKeyQuotaKey(apiKeyID),
		usageBillingPendingAPIKeyRateLimitKey(apiKeyID),
		billingBalanceKey(userID),
		billingSubKey(userID, groupID),
		billingRateLimitKey(apiKeyID),
		usageBillingBalanceMutationKey(userID),
		usageBillingSubscriptionMutationKey(userID, groupID),
		usageBillingAPIKeyRateLimitMutationKey(apiKeyID),
		usageBillingOverlayKey(requestID, apiKeyID),
	}
}

func usageBillingRequestKey(requestID string, apiKeyID int64) string {
	return strings.TrimSpace(requestID) + "\x00" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingOverlayKey(requestID string, apiKeyID int64) string {
	sum := sha256.Sum256([]byte(usageBillingRequestKey(requestID, apiKeyID)))
	return usageBillingOverlayPrefix + hex.EncodeToString(sum[:])
}

func usageBillingPendingBalanceKey(userID int64) string {
	return usageBillingPendingPrefix + "balance:" + strconv.FormatInt(userID, 10)
}

func usageBillingPendingSubscriptionKey(userID, groupID int64) string {
	return usageBillingPendingPrefix + "subscription:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func usageBillingPendingAPIKeyQuotaKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-quota:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingPendingAPIKeyRateLimitKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-rate:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingBalanceMutationKey(userID int64) string {
	return usageBillingMutationPrefix + "balance:" + strconv.FormatInt(userID, 10)
}

func usageBillingSubscriptionMutationKey(userID, groupID int64) string {
	return usageBillingMutationPrefix + "subscription:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func usageBillingAPIKeyRateLimitMutationKey(apiKeyID int64) string {
	return usageBillingMutationPrefix + "api-rate:" + strconv.FormatInt(apiKeyID, 10)
}

func isPermanentUsageBillingError(err error) bool {
	return errors.Is(err, errUsageBillingJobPayloadInvalid) ||
		errors.Is(err, service.ErrUsageBillingRequestConflict) ||
		errors.Is(err, service.ErrUsageBillingRequestIDRequired) ||
		errors.Is(err, service.ErrUserNotFound) ||
		errors.Is(err, service.ErrAPIKeyNotFound) ||
		errors.Is(err, service.ErrAccountNotFound) ||
		errors.Is(err, service.ErrSubscriptionNotFound)
}
