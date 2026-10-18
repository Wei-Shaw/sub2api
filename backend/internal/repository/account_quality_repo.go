package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qualitytrace"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"strings"
	"time"
)

type accountQualityRepo struct{ db *sql.DB }

func NewAccountQualityRepository(db *sql.DB) service.AccountQualityRepository {
	return &accountQualityRepo{db: db}
}
func (r *accountQualityRepo) Policy(ctx context.Context, id int64) (service.AccountQualityPolicy, error) {
	p := service.DefaultAccountQualityPolicy()
	var raw []byte
	err := r.db.QueryRowContext(ctx, "SELECT config FROM account_quality_policies WHERE group_id=$1", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p)
	return p, err
}
func (r *accountQualityRepo) SavePolicy(ctx context.Context, id int64, p service.AccountQualityPolicy) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO account_quality_policies(group_id,config,next_run_at) VALUES($1,$2,NOW()+$3*interval '1 minute') ON CONFLICT(group_id) DO UPDATE SET config=EXCLUDED.config,next_run_at=EXCLUDED.next_run_at,updated_at=NOW()", id, raw, p.IntervalMinutes)
	return err
}
func (r *accountQualityRepo) Groups(ctx context.Context) ([]service.AccountQualityGroup, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT g.id,g.name,g.platform,COUNT(a.id),COUNT(a.id) FILTER(WHERE a.status='active' AND a.schedulable),p.config,p.next_run_at FROM groups g LEFT JOIN account_groups ag ON ag.group_id=g.id LEFT JOIN accounts a ON a.id=ag.account_id AND a.deleted_at IS NULL LEFT JOIN account_quality_policies p ON p.group_id=g.id WHERE g.deleted_at IS NULL GROUP BY g.id,p.config,p.next_run_at ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.AccountQualityGroup{}
	for rows.Next() {
		var g service.AccountQualityGroup
		var raw []byte
		g.Policy = service.DefaultAccountQualityPolicy()
		if err = rows.Scan(&g.ID, &g.Name, &g.Platform, &g.Total, &g.Available, &raw, &g.NextRunAt); err != nil {
			return nil, err
		}
		switch g.Platform {
		case "anthropic":
			g.Policy.Endpoint = "messages"
			g.Policy.Effort = "unspecified"
		case "gemini":
			g.Policy.Endpoint = "generate_content"
			g.Policy.Effort = "unspecified"
		case "openai", "composite":
		default:
			g.Policy.Endpoint = "chat_completions"
			g.Policy.Effort = "unspecified"
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &g.Policy); err != nil {
				return nil, err
			}
		}
		out = append(out, g)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var g service.AccountQualityGroup
	g.ID = 0
	g.Name = "ungrouped"
	g.Platform = "mixed"
	g.Policy = service.DefaultAccountQualityPolicy()
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE status='active' AND schedulable) FROM accounts a WHERE a.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM account_groups ag JOIN groups g ON g.id=ag.group_id AND g.deleted_at IS NULL WHERE ag.account_id=a.id)`).Scan(&g.Total, &g.Available)
	if err != nil {
		return nil, err
	}
	if g.Total > 0 {
		out = append(out, g)
	}
	return out, nil
}
func (r *accountQualityRepo) Rows(ctx context.Context, id int64, p service.AccountQualityPolicy) ([]service.AccountQualityRow, error) {
	rows, err := r.db.QueryContext(ctx, accountQualityRowsSQL, id, p.Model, p.Effort, p.ServiceTier, p.Endpoint, p.ReferenceModel, qualitytrace.Revision())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.AccountQualityRow{}
	for rows.Next() {
		var a service.AccountQualityRow
		if err = rows.Scan(&a.ID, &a.Name, &a.Platform, &a.Type, &a.Status, &a.Schedulable, &a.Priority, &a.Concurrency, &a.Requests, &a.TTFTSamples, &a.P50, &a.P95, &a.CacheRate, &a.LastRequestAt, &a.CandyStatus, &a.QualitySamples, &a.QualityPassed, &a.LastTestAt, &a.Failures, &a.UnknownFailureScope, &a.TraceStatus, &a.TraceAt, &a.TraceCandidate); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *accountQualityRepo) StartRun(ctx context.Context, g int64, p service.AccountQualityPolicy, n int, benchmark string, scheduled bool) (*service.AccountQualityRun, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var currentRaw []byte
	if err = tx.QueryRowContext(ctx, "SELECT config FROM account_quality_policies WHERE group_id=$1 FOR UPDATE", g).Scan(&currentRaw); err != nil {
		return nil, err
	}
	current := service.DefaultAccountQualityPolicy()
	if err = json.Unmarshal(currentRaw, &current); err != nil {
		return nil, err
	}
	if scheduled && !current.Enabled {
		return nil, service.ErrAccountQualityDisabled
	}
	oldRaw, _ := json.Marshal(p)
	canonical, _ := json.Marshal(current)
	if string(oldRaw) != string(canonical) {
		return nil, service.ErrAccountQualityPolicyChanged
	}
	var used int
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(planned),0) FROM account_quality_runs WHERE group_id=$1 AND created_at>=date_trunc('day',NOW() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai'", g).Scan(&used); err != nil {
		return nil, err
	}
	if n < 1 || used+n > current.DailyBudget {
		return nil, service.ErrAccountQualityBudget
	}
	run := &service.AccountQualityRun{GroupID: g, Status: "queued", Planned: n, Benchmark: benchmark, Scheduled: scheduled}
	if err = tx.QueryRowContext(ctx, "INSERT INTO account_quality_runs(group_id,spec,planned,benchmark,scheduled) VALUES($1,$2,$3,$4,$5) RETURNING id,created_at", g, oldRaw, n, benchmark, scheduled).Scan(&run.ID, &run.CreatedAt); err != nil {
		var pg *pq.Error
		if errors.As(err, &pg) && pg.Code == "23505" && (pg.Constraint == "account_quality_one_active_group" || pg.Constraint == "account_quality_one_active_round") {
			return nil, service.ErrAccountQualityBusy
		}
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *accountQualityRepo) Runs(ctx context.Context, g int64) ([]service.AccountQualityRun, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,group_id,status,planned,completed,created_at,finished_at,cancel_requested,benchmark,scheduled FROM account_quality_runs WHERE ($1=0 OR group_id=$1) ORDER BY id DESC LIMIT 20", g)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.AccountQualityRun{}
	for rows.Next() {
		var x service.AccountQualityRun
		if err = rows.Scan(&x.ID, &x.GroupID, &x.Status, &x.Planned, &x.Completed, &x.CreatedAt, &x.FinishedAt, &x.CancelRequested, &x.Benchmark, &x.Scheduled); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *accountQualityRepo) Results(ctx context.Context, g, a int64) ([]service.AccountQualityResult, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,run_id,group_id,account_id,model,effort,service_tier,benchmark,verdict,answer,detail,duration_ms,created_at,evidence,endpoint,requests_used FROM account_quality_results WHERE group_id=$1 AND ($2=0 OR account_id=$2) ORDER BY id DESC LIMIT 100", g, a)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.AccountQualityResult{}
	for rows.Next() {
		var x service.AccountQualityResult
		if err = rows.Scan(&x.ID, &x.RunID, &x.GroupID, &x.AccountID, &x.Model, &x.Effort, &x.ServiceTier, &x.Benchmark, &x.Verdict, &x.Answer, &x.Detail, &x.DurationMs, &x.CreatedAt, &x.Evidence, &x.Endpoint, &x.RequestsUsed); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *accountQualityRepo) AddResult(ctx context.Context, x *service.AccountQualityResult) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.QueryRowContext(ctx, "SELECT NOT cancel_requested AND status IN ('queued','running') AND lease_until>NOW() FROM account_quality_runs WHERE id=$1 FOR UPDATE", x.RunID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return context.Canceled
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO account_quality_results(run_id,group_id,account_id,model,effort,service_tier,benchmark,verdict,answer,detail,duration_ms,evidence,endpoint,requests_used) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)", x.RunID, x.GroupID, x.AccountID, x.Model, x.Effort, x.ServiceTier, x.Benchmark, x.Verdict, x.Answer, x.Detail, x.DurationMs, qualityEvidenceJSON(x.Evidence), qualityResultEndpoint(x.Endpoint), qualityResultRequests(x))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE account_quality_runs SET completed=completed+$2,status='running' WHERE id=$1", x.RunID, qualityResultRequests(x))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *accountQualityRepo) FinishRun(ctx context.Context, id int64, s string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE account_quality_runs SET status=$2,planned=CASE WHEN $2='completed' THEN completed ELSE planned END,finished_at=NOW() WHERE id=$1 AND status IN ('queued','running')", id, s)
	return err
}
func (r *accountQualityRepo) CancelRun(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE account_quality_runs SET cancel_requested=true WHERE id=$1 AND status IN ('queued','running')", id)
	return err
}
func (r *accountQualityRepo) Canceled(ctx context.Context, id int64) bool {
	var b bool
	err := r.db.QueryRowContext(ctx, "SELECT cancel_requested OR status NOT IN ('queued','running') OR lease_until<NOW() FROM account_quality_runs WHERE id=$1", id).Scan(&b)
	return err != nil || b
}
func (r *accountQualityRepo) UsedToday(ctx context.Context, g int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(planned),0) FROM account_quality_runs WHERE group_id=$1 AND created_at>=date_trunc('day',NOW() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai'", g).Scan(&n)
	return n, err
}
func (r *accountQualityRepo) RecoverRuns(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE account_quality_runs SET status='interrupted',finished_at=NOW() WHERE status IN ('queued','running') AND lease_until<NOW()")
	return err
}

func (r *accountQualityRepo) Heartbeat(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "UPDATE account_quality_runs SET lease_until=NOW()+interval '3 minutes',status='running' WHERE id=$1 AND status IN ('queued','running') AND NOT cancel_requested AND lease_until>NOW()", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *accountQualityRepo) RecentResult(ctx context.Context, id int64, p service.AccountQualityPolicy, benchmark string) (*service.AccountQualityResult, error) {
	var x service.AccountQualityResult
	err := r.db.QueryRowContext(ctx, "SELECT id,verdict,answer,detail,duration_ms FROM account_quality_results WHERE account_id=$1 AND model=$2 AND effort=$3 AND service_tier=$4 AND endpoint=$5 AND benchmark=$6 AND detail NOT LIKE 'reused_result:%' AND verdict IN ('passed','incorrect') AND created_at>NOW()-interval '15 minutes' ORDER BY created_at DESC LIMIT 1", id, p.Model, p.Effort, p.ServiceTier, p.Endpoint, benchmark).Scan(&x.ID, &x.Verdict, &x.Answer, &x.Detail, &x.DurationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &x, err
}

func qualityEvidenceJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return []byte(raw)
}

func qualityResultEndpoint(endpoint string) string {
	if endpoint == "" {
		return "responses"
	}
	return endpoint
}
func qualityResultRequests(r *service.AccountQualityResult) int {
	if r.RequestsUsed == 0 && !strings.HasPrefix(r.Detail, "reused_result:") {
		return 1
	}
	return r.RequestsUsed
}

func (r *accountQualityRepo) LoadWeights(ctx context.Context, g int64, key string) (map[int64]service.AccountQualityStoredWeight, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT account_id,multiplier,updated_at FROM account_quality_weights WHERE group_id=$1 AND scope_key=$2", g, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]service.AccountQualityStoredWeight{}
	for rows.Next() {
		var id int64
		var w service.AccountQualityStoredWeight
		if err = rows.Scan(&id, &w.Multiplier, &w.UpdatedAt); err != nil {
			return nil, err
		}
		out[id] = w
	}
	return out, rows.Err()
}
func (r *accountQualityRepo) SaveWeights(ctx context.Context, g int64, key string, weights map[int64]service.AccountQualityStoredWeight) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, w := range weights {
		if _, err = tx.ExecContext(ctx, "INSERT INTO account_quality_weights(group_id,account_id,scope_key,multiplier,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(group_id,account_id,scope_key) DO UPDATE SET multiplier=EXCLUDED.multiplier,updated_at=EXCLUDED.updated_at", g, id, key, w.Multiplier, w.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *accountQualityRepo) AdvanceSchedule(ctx context.Context, g int64, next time.Time) error {
	_, err := r.db.ExecContext(ctx, "UPDATE account_quality_policies SET next_run_at=$2 WHERE group_id=$1 AND next_run_at<=NOW() AND config->>'enabled'='true'", g, next)
	return err
}
