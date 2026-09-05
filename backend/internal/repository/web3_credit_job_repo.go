package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3CreditJobRepository struct{ db *sql.DB }

var _ web3deposit.CreditJobStore = (*Web3CreditJobRepository)(nil)

func NewWeb3CreditJobRepository(db *sql.DB) *Web3CreditJobRepository {
	return &Web3CreditJobRepository{db: db}
}

func (r *Web3CreditJobRepository) ClaimCreditJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]web3deposit.CreditJob, error) {
	rows, err := r.db.QueryContext(ctx, `WITH candidates AS (
	 SELECT id FROM web3_deposits
	 WHERE (status='ready_to_credit' AND (next_retry_at IS NULL OR next_retry_at <= $1))
	    OR (status='crediting' AND next_retry_at <= $1)
	 ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED)
	 UPDATE web3_deposits d SET status='crediting', retry_count=retry_count+1,
	 next_retry_at=$3, failure_reason=NULL, updated_at=$1 FROM candidates c WHERE d.id=c.id
	 RETURNING d.id,d.retry_count`, now, limit, now.Add(lease))
	if err != nil {
		return nil, fmt.Errorf("claim web3 credit jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	jobs := make([]web3deposit.CreditJob, 0, limit)
	for rows.Next() {
		var job web3deposit.CreditJob
		if err := rows.Scan(&job.DepositID, &job.ClaimVersion); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Web3CreditJobRepository) RetryCreditJob(ctx context.Context, job web3deposit.CreditJob, now time.Time, cause error) error {
	backoff := time.Duration(1<<min(int(job.ClaimVersion), 8)) * time.Second
	result, err := r.db.ExecContext(ctx, `UPDATE web3_deposits SET status='ready_to_credit',next_retry_at=$3,
	 failure_reason=$4,updated_at=$2 WHERE id=$1 AND status='crediting' AND retry_count=$5`,
		job.DepositID, now, now.Add(backoff), cause.Error(), job.ClaimVersion)
	if err != nil {
		return fmt.Errorf("retry web3 credit job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return web3deposit.ErrCreditClaimLost
	}
	return nil
}
