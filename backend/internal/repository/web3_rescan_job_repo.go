package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3RescanJobRepository struct{ db *sql.DB }

var _ web3deposit.RescanJobStore = (*Web3RescanJobRepository)(nil)

func NewWeb3RescanJobRepository(db *sql.DB) *Web3RescanJobRepository {
	return &Web3RescanJobRepository{db: db}
}

func (r *Web3RescanJobRepository) CreateRescanJob(ctx context.Context, job web3deposit.RescanJob) (web3deposit.RescanJob, error) {
	row := r.db.QueryRowContext(ctx, `INSERT INTO web3_rescan_jobs
	 (network_key,asset_key,from_block,to_block,status,requested_by,created_at,updated_at)
	 VALUES ($1,$2,$3,$4,'pending',$5,NOW(),NOW())
	 RETURNING id,network_key,asset_key,from_block,to_block,status,requested_by,attempt_count,
	 event_count,matched_count,deposit_count,error_message,lease_expires_at,started_at,completed_at,created_at,updated_at`,
		job.NetworkKey, job.AssetKey, job.FromBlock, job.ToBlock, job.RequestedBy)
	created, err := scanRescanJob(row)
	if err != nil {
		return web3deposit.RescanJob{}, fmt.Errorf("create web3 rescan job: %w", err)
	}
	return created, nil
}

func (r *Web3RescanJobRepository) ClaimRescanJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]web3deposit.RescanJob, error) {
	rows, err := r.db.QueryContext(ctx, `WITH candidates AS (
	 SELECT id FROM web3_rescan_jobs
	 WHERE status='pending' OR (status='running' AND lease_expires_at <= $1)
	 ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED)
	 UPDATE web3_rescan_jobs j SET status='running',attempt_count=attempt_count+1,
	 started_at=COALESCE(started_at,$1),lease_expires_at=$3,error_message=NULL,updated_at=$1
	 FROM candidates c WHERE j.id=c.id
	 RETURNING j.id,j.network_key,j.asset_key,j.from_block,j.to_block,j.status,j.requested_by,j.attempt_count,
	 j.event_count,j.matched_count,j.deposit_count,j.error_message,j.lease_expires_at,j.started_at,j.completed_at,j.created_at,j.updated_at`,
		now, limit, now.Add(lease))
	if err != nil {
		return nil, fmt.Errorf("claim web3 rescan jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]web3deposit.RescanJob, 0, limit)
	for rows.Next() {
		job, err := scanRescanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Web3RescanJobRepository) RenewRescanJob(ctx context.Context, job web3deposit.RescanJob, now time.Time, lease time.Duration) error {
	res, err := r.db.ExecContext(ctx, `UPDATE web3_rescan_jobs SET lease_expires_at=$3,updated_at=$2
	 WHERE id=$1 AND status='running' AND attempt_count=$4`, job.ID, now, now.Add(lease), job.AttemptCount)
	return rescanJobClaimUpdateResult(res, err)
}

func (r *Web3RescanJobRepository) CompleteRescanJob(ctx context.Context, job web3deposit.RescanJob, result web3deposit.BoundedRescanResult, now time.Time) error {
	res, err := r.db.ExecContext(ctx, `UPDATE web3_rescan_jobs SET status='succeeded',event_count=$2,
	 matched_count=$3,deposit_count=$4,error_message=NULL,lease_expires_at=NULL,completed_at=$5,updated_at=$5
	 WHERE id=$1 AND status='running' AND attempt_count=$6`, job.ID, result.EventCount, result.MatchedCount, result.DepositCount, now, job.AttemptCount)
	return rescanJobClaimUpdateResult(res, err)
}

func (r *Web3RescanJobRepository) FailRescanJob(ctx context.Context, job web3deposit.RescanJob, cause error, now time.Time) error {
	message := cause.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	res, err := r.db.ExecContext(ctx, `UPDATE web3_rescan_jobs SET status='failed',error_message=$2,
	 lease_expires_at=NULL,completed_at=$3,updated_at=$3 WHERE id=$1 AND status='running' AND attempt_count=$4`, job.ID, message, now, job.AttemptCount)
	return rescanJobClaimUpdateResult(res, err)
}

func (r *Web3RescanJobRepository) ListRescanJobs(ctx context.Context, limit int) ([]web3deposit.RescanJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,network_key,asset_key,from_block,to_block,status,requested_by,
	 attempt_count,event_count,matched_count,deposit_count,error_message,lease_expires_at,started_at,completed_at,created_at,updated_at
	 FROM web3_rescan_jobs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list web3 rescan jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]web3deposit.RescanJob, 0, limit)
	for rows.Next() {
		job, err := scanRescanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Web3RescanJobRepository) GetRescanJob(ctx context.Context, id int64) (web3deposit.RescanJob, error) {
	job, err := scanRescanJob(r.db.QueryRowContext(ctx, `SELECT id,network_key,asset_key,from_block,to_block,status,requested_by,
	 attempt_count,event_count,matched_count,deposit_count,error_message,lease_expires_at,started_at,completed_at,created_at,updated_at
	 FROM web3_rescan_jobs WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return web3deposit.RescanJob{}, web3deposit.ErrRescanJobNotFound
	}
	if err != nil {
		return web3deposit.RescanJob{}, fmt.Errorf("get web3 rescan job: %w", err)
	}
	return job, nil
}

type rescanJobScanner interface{ Scan(...any) error }

func scanRescanJob(scanner rescanJobScanner) (web3deposit.RescanJob, error) {
	var job web3deposit.RescanJob
	var errorMessage sql.NullString
	var leaseExpiresAt, startedAt, completedAt sql.NullTime
	err := scanner.Scan(&job.ID, &job.NetworkKey, &job.AssetKey, &job.FromBlock, &job.ToBlock, &job.Status,
		&job.RequestedBy, &job.AttemptCount, &job.EventCount, &job.MatchedCount, &job.DepositCount, &errorMessage,
		&leaseExpiresAt, &startedAt, &completedAt, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return web3deposit.RescanJob{}, err
	}
	job.ErrorMessage = errorMessage.String
	if leaseExpiresAt.Valid {
		job.LeaseExpiresAt = &leaseExpiresAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	return job, nil
}

func rescanJobClaimUpdateResult(result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("update web3 rescan job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return web3deposit.ErrRescanJobClaimLost
	}
	return nil
}
