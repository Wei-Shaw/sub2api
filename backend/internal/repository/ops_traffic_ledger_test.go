package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type trafficJSONArg struct{}

func (trafficJSONArg) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var usage traffic.Usage
	return json.Unmarshal([]byte(text), &usage) == nil && usage.Downstream != nil && usage.Downstream.Response != nil && usage.Downstream.Response.ActualBytes != nil && *usage.Downstream.Response.ActualBytes == 0
}

func TestOpsTrafficLedgerSQLArgsAndDuplicateID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &opsRepository{db: db}
	zero, group, account := int64(0), int64(7), int64(8)
	event := &service.OpsTrafficEvent{EventID: uuid.NewString(), RequestID: "client-id", GroupID: &group, AccountID: &account, Platform: " OpenAI ", CreatedAt: time.Now().UTC(), Traffic: &traffic.Usage{Downstream: &traffic.Link{Response: &traffic.Measurement{ActualBytes: &zero}}}}
	for _, affected := range []int64{1, 0} {
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO ops_traffic_events .*ON CONFLICT \(event_id\) DO NOTHING`).WithArgs(event.EventID, "client-id", group, account, "openai", event.CreatedAt, trafficJSONArg{}).WillReturnResult(sqlmock.NewResult(0, affected))
		mock.ExpectCommit()
		require.NoError(t, repo.BatchInsertTrafficEvents(context.Background(), []*service.OpsTrafficEvent{event}))
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsTrafficLedgerSQLChunksAtomicAndUnknown(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &opsRepository{db: db}
	events := make([]*service.OpsTrafficEvent, opsTrafficInsertChunkSize+1)
	for i := range events {
		events[i] = &service.OpsTrafficEvent{EventID: uuid.NewString(), CreatedAt: time.Now(), Traffic: &traffic.Usage{}}
	}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ops_traffic_events`).WillReturnResult(sqlmock.NewResult(0, opsTrafficInsertChunkSize))
	mock.ExpectExec(`INSERT INTO ops_traffic_events`).WithArgs(events[opsTrafficInsertChunkSize].EventID, "", nil, nil, nil, events[opsTrafficInsertChunkSize].CreatedAt, `{"upstream":null,"downstream":null}`).WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	require.Error(t, repo.BatchInsertTrafficEvents(context.Background(), events))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsTrafficLedgerRejectsInvalidBeforeTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &opsRepository{db: db}
	for _, event := range []*service.OpsTrafficEvent{nil, {}, {EventID: strings.Repeat("x", 36), CreatedAt: time.Now(), Traffic: &traffic.Usage{}}} {
		require.Error(t, repo.BatchInsertTrafficEvents(context.Background(), []*service.OpsTrafficEvent{event}))
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
