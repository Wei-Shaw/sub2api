package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

var rescanJobColumns = []string{
	"id", "network_key", "asset_key", "from_block", "to_block", "status", "requested_by", "attempt_count",
	"event_count", "matched_count", "deposit_count", "error_message", "lease_expires_at", "started_at", "completed_at", "created_at", "updated_at",
}

func TestWeb3RescanJobRepositoryCreatesAuditablePendingJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO web3_rescan_jobs")).
		WithArgs("conflux", "usdt0", uint64(100), uint64(120), int64(77)).
		WillReturnRows(sqlmock.NewRows(rescanJobColumns).AddRow(
			int64(9), "conflux", "usdt0", int64(100), int64(120), "pending", int64(77), 0,
			0, 0, 0, nil, nil, nil, nil, now, now,
		))

	job, err := NewWeb3RescanJobRepository(db).CreateRescanJob(context.Background(), web3deposit.RescanJob{
		NetworkKey: "conflux", AssetKey: "usdt0", FromBlock: 100, ToBlock: 120, RequestedBy: 77,
	})

	require.NoError(t, err)
	require.Equal(t, int64(9), job.ID)
	require.Equal(t, web3deposit.RescanJobStatusPending, job.Status)
	require.Equal(t, int64(77), job.RequestedBy)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3RescanJobRepositoryReclaimsExpiredRunningJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	lease := 10 * time.Minute
	mock.ExpectQuery("status='pending' OR \\(status='running' AND lease_expires_at <= \\$1\\)").
		WithArgs(now, 1, now.Add(lease)).
		WillReturnRows(sqlmock.NewRows(rescanJobColumns).AddRow(
			int64(10), "conflux", "usdt0", int64(100), int64(120), "running", int64(77), 2,
			0, 0, 0, nil, now.Add(lease), now.Add(-lease), nil, now.Add(-time.Hour), now,
		))

	jobs, err := NewWeb3RescanJobRepository(db).ClaimRescanJobs(context.Background(), now, lease, 1)

	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, 2, jobs[0].AttemptCount)
	require.Equal(t, web3deposit.RescanJobStatusRunning, jobs[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3RescanJobRepositoryFencesStaleLeaseRenewal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	lease := 10 * time.Minute
	mock.ExpectExec("attempt_count=\\$4").
		WithArgs(int64(10), now, now.Add(lease), 1).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = NewWeb3RescanJobRepository(db).RenewRescanJob(context.Background(), web3deposit.RescanJob{ID: 10, AttemptCount: 1}, now, lease)

	require.ErrorIs(t, err, web3deposit.ErrRescanJobClaimLost)
	require.NoError(t, mock.ExpectationsWereMet())
}
