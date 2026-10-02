package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

// This repository stages mutations against the Ent transaction from context and
// makes them visible only when the real Ent/sql transaction commits.
type transactionalBulkSubscriptionRepo struct {
	UserSubscriptionRepository
	committed       UserSubscription
	pending         map[*dbent.Tx]*UserSubscription
	reads           map[*dbent.Tx]int
	postReadFailure bool
	statusFailure   bool
}

func (r *transactionalBulkSubscriptionRepo) GetByID(ctx context.Context, _ int64) (*UserSubscription, error) {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return nil, errors.New("bulk mutation must use a transaction")
	}
	if r.pending[tx] == nil {
		copy := r.committed
		r.pending[tx] = &copy
		tx.OnCommit(func(next dbent.Committer) dbent.Committer {
			return dbent.CommitFunc(func(ctx context.Context, tx *dbent.Tx) error {
				if err := next.Commit(ctx, tx); err != nil {
					return err
				}
				r.committed = *r.pending[tx]
				return nil
			})
		})
	}
	r.reads[tx]++
	if r.reads[tx] > 1 && r.postReadFailure {
		return nil, errors.New("refresh failed after write")
	}
	copy := *r.pending[tx]
	return &copy, nil
}

func (r *transactionalBulkSubscriptionRepo) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return r.GetByID(ctx, id)
}

func (r *transactionalBulkSubscriptionRepo) ExtendExpiry(ctx context.Context, _ int64, expiry time.Time) error {
	r.pending[dbent.TxFromContext(ctx)].ExpiresAt = expiry
	return nil
}

func (r *transactionalBulkSubscriptionRepo) UpdateStatus(ctx context.Context, _ int64, status string) error {
	if r.statusFailure {
		return errors.New("status update failed after expiry write")
	}
	r.pending[dbent.TxFromContext(ctx)].Status = status
	return nil
}

func (r *transactionalBulkSubscriptionRepo) ResetUsageWindows(ctx context.Context, _ int64, daily, weekly, monthly bool, dailyStart, periodicStart time.Time) error {
	sub := r.pending[dbent.TxFromContext(ctx)]
	if daily {
		sub.DailyUsageUSD, sub.DailyWindowStart = 0, &dailyStart
	}
	if weekly {
		sub.WeeklyUsageUSD, sub.WeeklyWindowStart = 0, &periodicStart
	}
	if monthly {
		sub.MonthlyUsageUSD, sub.MonthlyWindowStart = 0, &periodicStart
	}
	return nil
}

func TestBulkSubscriptionAction_RollsBackPostWriteFailureBeforeRetry(t *testing.T) {
	for _, tc := range []struct {
		name            string
		action          string
		postReadFailure bool
		statusFailure   bool
	}{
		{name: "extend refresh failure", action: "extend", postReadFailure: true},
		{name: "extend status failure", action: "extend", statusFailure: true},
		{name: "quota refresh failure", action: "reset_quota", postReadFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
			t.Cleanup(func() { _ = client.Close() })
			expiresAt := time.Now().AddDate(0, 0, 30)
			status := SubscriptionStatusActive
			if tc.statusFailure {
				status = SubscriptionStatusExpired
			}
			repo := &transactionalBulkSubscriptionRepo{
				committed:       UserSubscription{ID: 1, UserID: 10, GroupID: 20, Status: status, ExpiresAt: expiresAt, DailyUsageUSD: 7},
				pending:         make(map[*dbent.Tx]*UserSubscription),
				reads:           make(map[*dbent.Tx]int),
				postReadFailure: tc.postReadFailure,
				statusFailure:   tc.statusFailure,
			}
			svc := NewSubscriptionService(nil, repo, nil, client, nil)
			t.Cleanup(svc.Stop)
			input := &BulkSubscriptionActionInput{SubscriptionIDs: []int64{1}, Action: tc.action, Days: 7, Daily: true}

			mock.ExpectBegin()
			mock.ExpectRollback()
			result, err := svc.BulkSubscriptionAction(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, 1, result.FailedCount)
			require.Equal(t, expiresAt, repo.committed.ExpiresAt, "failed operation must not commit its earlier expiry write")
			require.Equal(t, status, repo.committed.Status)
			require.Equal(t, float64(7), repo.committed.DailyUsageUSD)

			repo.postReadFailure, repo.statusFailure = false, false
			mock.ExpectBegin()
			mock.ExpectCommit()
			result, err = svc.BulkSubscriptionAction(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, 1, result.SuccessCount)
			if tc.action == "extend" {
				require.Equal(t, expiresAt.AddDate(0, 0, 7), repo.committed.ExpiresAt, "retry must extend only once")
				require.Equal(t, SubscriptionStatusActive, repo.committed.Status)
			} else {
				require.Zero(t, repo.committed.DailyUsageUSD)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
