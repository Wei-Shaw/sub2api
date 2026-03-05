package repository

import (
	"context"
	"database/sql"
	"hash/fnv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// --- Plan Repository ---

type scheduledTestPlanRepository struct {
	db *sql.DB
REDACTED

func NewScheduledTestPlanRepository(db *sql.DB) service.ScheduledTestPlanRepository {
	return &scheduledTestPlanRepository{db: dbREDACTED
REDACTED

func (r *scheduledTestPlanRepository) Create(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_test_plans (account_id, model_id, cron_expression, enabled, max_results, next_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, account_id, model_id, cron_expression, enabled, max_results, last_run_at, next_run_at, created_at, updated_at
	`, plan.AccountID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.NextRunAt)
	return scanPlan(row)
REDACTED

func (r *scheduledTestPlanRepository) GetByID(ctx context.Context, id int64) (*service.ScheduledTestPlan, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans WHERE id = $1
	`, id)
	return scanPlan(row)
REDACTED

func (r *scheduledTestPlanRepository) ListByAccountID(ctx context.Context, accountID int64) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	return scanPlans(rows)
REDACTED

func (r *scheduledTestPlanRepository) ListDue(ctx context.Context, now time.Time) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans
		WHERE enabled = true AND next_run_at <= $1
		ORDER BY next_run_at ASC
	`, now)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	return scanPlans(rows)
REDACTED

func (r *scheduledTestPlanRepository) Update(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE scheduled_test_plans
		SET model_id = $2, cron_expression = $3, enabled = $4, max_results = $5, next_run_at = $6, updated_at = NOW()
		WHERE id = $1
		RETURNING id, account_id, model_id, cron_expression, enabled, max_results, last_run_at, next_run_at, created_at, updated_at
	`, plan.ID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.NextRunAt)
	return scanPlan(row)
REDACTED

func (r *scheduledTestPlanRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_test_plans WHERE id = $1`, id)
	return err
REDACTED

func (r *scheduledTestPlanRepository) UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_test_plans SET last_run_at = $2, next_run_at = $3, updated_at = NOW() WHERE id = $1
	`, id, lastRunAt, nextRunAt)
	return err
REDACTED

// --- Result Repository ---

type scheduledTestResultRepository struct {
	db *sql.DB
REDACTED

func NewScheduledTestResultRepository(db *sql.DB) service.ScheduledTestResultRepository {
	return &scheduledTestResultRepository{db: dbREDACTED
REDACTED

func (r *scheduledTestResultRepository) Create(ctx context.Context, result *service.ScheduledTestResult) (*service.ScheduledTestResult, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_test_results (plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id, plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at
	`, result.PlanID, result.Status, result.ResponseText, result.ErrorMessage, result.LatencyMs, result.StartedAt, result.FinishedAt)

	out := &service.ScheduledTestResult{REDACTED
	if err := row.Scan(
		&out.ID, &out.PlanID, &out.Status, &out.ResponseText, &out.ErrorMessage,
		&out.LatencyMs, &out.StartedAt, &out.FinishedAt, &out.CreatedAt,
	); err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

func (r *scheduledTestResultRepository) ListByPlanID(ctx context.Context, planID int64, limit int) ([]*service.ScheduledTestResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at
		FROM scheduled_test_results
		WHERE plan_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, planID, limit)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	var results []*service.ScheduledTestResult
	for rows.Next() {
		r := &service.ScheduledTestResult{REDACTED
		if err := rows.Scan(
			&r.ID, &r.PlanID, &r.Status, &r.ResponseText, &r.ErrorMessage,
			&r.LatencyMs, &r.StartedAt, &r.FinishedAt, &r.CreatedAt,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, r)
REDACTED
	return results, rows.Err()
REDACTED

func (r *scheduledTestResultRepository) PruneOldResults(ctx context.Context, planID int64, keepCount int) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM scheduled_test_results
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY created_at DESC) AS rn
				FROM scheduled_test_results
				WHERE plan_id = $1
			) ranked
			WHERE rn > $2
		)
	`, planID, keepCount)
	return err
REDACTED

// --- scan helpers ---

type scannable interface {
	Scan(dest ...any) error
REDACTED

func scanPlan(row scannable) (*service.ScheduledTestPlan, error) {
	p := &service.ScheduledTestPlan{REDACTED
	if err := row.Scan(
		&p.ID, &p.AccountID, &p.ModelID, &p.CronExpression, &p.Enabled, &p.MaxResults,
		&p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
REDACTED
	return p, nil
REDACTED

func scanPlans(rows *sql.Rows) ([]*service.ScheduledTestPlan, error) {
	var plans []*service.ScheduledTestPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
	REDACTED
		plans = append(plans, p)
REDACTED
	return plans, rows.Err()
REDACTED

// --- LeaderLocker ---

var redisReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type leaderLocker struct {
	db              *sql.DB
	redisClient     *redis.Client
	instanceID      string
	warnNoRedisOnce sync.Once
REDACTED

// NewLeaderLocker creates a LeaderLocker that tries Redis first, then falls back to pg advisory lock.
func NewLeaderLocker(db *sql.DB, redisClient *redis.Client) service.LeaderLocker {
	return &leaderLocker{
		db:          db,
		redisClient: redisClient,
		instanceID:  uuid.NewString(),
REDACTED
REDACTED

func (l *leaderLocker) TryAcquire(ctx context.Context, key string, ttl time.Duration) (func(), bool) {
	if l.redisClient != nil {
		ok, err := l.redisClient.SetNX(ctx, key, l.instanceID, ttl).Result()
		if err == nil {
			if !ok {
				return nil, false
		REDACTED
			return func() {
				_, _ = redisReleaseScript.Run(ctx, l.redisClient, []string{keyREDACTED, l.instanceID).Result()
		REDACTED, true
	REDACTED
		l.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("repository.leader_locker", "[LeaderLocker] Redis SetNX failed; falling back to DB advisory lock: %v", err)
	REDACTED)
REDACTED else {
		l.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("repository.leader_locker", "[LeaderLocker] Redis not configured; using DB advisory lock")
	REDACTED)
REDACTED

	// Fallback: pg_try_advisory_lock
	return tryAdvisoryLock(ctx, l.db, hashLockID(key))
REDACTED

func tryAdvisoryLock(ctx context.Context, db *sql.DB, lockID int64) (func(), bool) {
	if db == nil {
		return nil, false
REDACTED
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false
REDACTED
	acquired := false
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false
REDACTED
	if !acquired {
		_ = conn.Close()
		return nil, false
REDACTED
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", lockID)
		_ = conn.Close()
REDACTED, true
REDACTED

func hashLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
REDACTED
