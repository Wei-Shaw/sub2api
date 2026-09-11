package securityaudit

import (
	"context"
	"encoding/json"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type blockingPromptRecordRepository struct {
	started  chan struct{}
	release  chan struct{}
	inserted atomic.Int64
}

func (r *blockingPromptRecordRepository) InsertPromptRecord(context.Context, *PromptRecord) error {
	r.inserted.Add(1)
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.release
	return nil
}
func (*blockingPromptRecordRepository) UpdatePromptRecordResponse(context.Context, Request, string, PromptResponse) (bool, error) {
	return true, nil
}

func (*blockingPromptRecordRepository) ListPromptRecords(context.Context, PromptRecordFilter, int, int) (*PromptRecordPage, error) {
	return nil, nil
}
func (*blockingPromptRecordRepository) GetPromptRecord(context.Context, int64) (*PromptRecord, error) {
	return nil, ErrPromptRecordNotFound
}
func (*blockingPromptRecordRepository) DeletePromptRecord(context.Context, int64) error { return nil }
func (*blockingPromptRecordRepository) DeletePromptRecords(context.Context, []int64) (int64, error) {
	return 0, nil
}

func TestPromptRecordServiceUsesBoundedQueuesWhenSaturated(t *testing.T) {
	repo := &blockingPromptRecordRepository{started: make(chan struct{}, 1), release: make(chan struct{})}
	service := newPromptRecordService(repo, 1, 1, 1)
	request := Request{
		RequestID: "bounded-queue", Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	}

	service.RecordPrompt(context.Background(), request)
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("record worker did not start")
	}

	// The worker is blocked on the first insert. The next two jobs occupy the
	// primary and overflow queues; subsequent jobs must be dropped immediately.
	service.RecordPrompt(context.Background(), request)
	service.RecordPrompt(context.Background(), request)
	service.RecordPrompt(context.Background(), request)
	stats := service.QueueStats()
	require.Equal(t, int64(1), stats.DroppedTotal)
	require.Equal(t, 1, stats.QueueLength)
	require.Equal(t, 1, stats.OverflowLength)
	require.NotNil(t, stats.LastDroppedAt)

	close(repo.release)
	deadline := time.Now().Add(time.Second)
	for repo.inserted.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, int64(3), repo.inserted.Load())
}

func TestListPromptRecordsReturnsCallMetadataWithoutPromptText(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repository := NewPostgreSQLRepository(db)
	createdAt := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM prompt_records WHERE 1=1")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, request_id, turn_no, stage.*FROM prompt_records WHERE 1=1 ORDER BY created_at DESC, id DESC").
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "turn_no", "stage", "user_id", "username_snapshot", "user_email_snapshot",
			"api_key_id", "api_key_name_snapshot", "group_id", "group_name", "provider", "endpoint", "protocol",
			"model", "prompt_hash", "prompt_length", "message_count", "risk_status", "risk_checked_at", "created_at", "expires_at",
		}).AddRow(
			7, "req-7", 1, "http", 2, "alice", "alice@example.com", 3, "primary", nil, "", "openai",
			"/v1/chat/completions", "openai_chat", "gpt-test", "hash", 128, 4, "pending", nil, createdAt, nil,
		))

	page, err := repository.ListPromptRecords(context.Background(), PromptRecordFilter{}, 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "req-7", page.Items[0].RequestID)
	require.Equal(t, 128, page.Items[0].PromptLength)

	payload, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "prompt_text")
	require.NotContains(t, string(payload), "risk_result")
	require.NoError(t, mock.ExpectationsWereMet())
}

var _ PromptRecordRepository = (*blockingPromptRecordRepository)(nil)
