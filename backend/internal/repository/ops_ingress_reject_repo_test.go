package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBatchUpsertIngressRejectsUsesFixedMultiRowChunks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &opsRepository{db: db}
	now := time.Now().UTC().Truncate(time.Minute)
	items := make([]*service.OpsIngressRejectAggregate, ingressRejectUpsertChunkSize+1)
	for i := range items {
		items[i] = &service.OpsIngressRejectAggregate{
			BucketStart: now, RejectReason: "invalid_api_key", RouteFamily: "messages",
			Protocol: "anthropic", ClientIP: "192.0.2.1", RequestCount: 1, FirstSeen: now, LastSeen: now,
		}
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ops_ingress_reject_aggregates").WillReturnResult(sqlmock.NewResult(0, int64(ingressRejectUpsertChunkSize)))
	mock.ExpectExec("INSERT INTO ops_ingress_reject_aggregates").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.BatchUpsertIngressRejects(context.Background(), items))
	require.NoError(t, mock.ExpectationsWereMet())
}
