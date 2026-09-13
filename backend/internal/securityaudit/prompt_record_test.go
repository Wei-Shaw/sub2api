package securityaudit

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type blockingPromptRecordRepository struct {
	started         chan struct{}
	release         chan struct{}
	responseUpdated chan PromptRecordKey
	inserted        atomic.Int64
}

func (r *blockingPromptRecordRepository) InsertPromptRecord(context.Context, *PromptRecord) (int64, error) {
	id := r.inserted.Add(1)
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.release
	return id, nil
}
func (r *blockingPromptRecordRepository) UpdatePromptRecordResponse(_ context.Context, key PromptRecordKey, _ PromptResponse) (bool, error) {
	if r.responseUpdated != nil {
		r.responseUpdated <- key
	}
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
func (*blockingPromptRecordRepository) DeleteAllPromptRecords(context.Context) (int64, error) {
	return 0, nil
}

func TestPromptRecordPolicyAllowsOnlyUserFacingEndpointsAndTurns(t *testing.T) {
	for _, endpoint := range []string{"/v1/responses", "/v1/messages", "/v1/chat/completions"} {
		require.True(t, ShouldRecordPromptRequest(Request{Endpoint: endpoint}), endpoint)
	}
	for _, endpoint := range []string{"", "/v1/alpha/search", "/v1/embeddings", "/v1/responses/"} {
		require.False(t, ShouldRecordPromptRequest(Request{Endpoint: endpoint}), endpoint)
	}

	for _, metadata := range []string{
		`{"request_kind":"memory","thread_source":"memory_consolidation"}`,
		`{"request_kind":"compaction"}`,
		`{"request_kind":"turn","thread_source":"thread_title"}`,
		`{"request_kind":"turn","thread_source":"system"}`,
		`{"request_kind":"turn","turn_trigger":"thread_title"}`,
	} {
		require.False(t, ShouldRecordPromptRequest(Request{
			Endpoint: "/v1/responses", Headers: map[string][]string{"X-Codex-Turn-Metadata": {metadata}},
		}), metadata)
	}
	require.True(t, ShouldRecordPromptRequest(Request{
		Endpoint: "/v1/responses", Headers: map[string][]string{"X-Codex-Turn-Metadata": {`{"request_kind":"turn","thread_source":"user"}`}},
	}))
	require.True(t, ShouldRecordPromptRequest(Request{
		Endpoint: "/v1/responses", Headers: map[string][]string{"X-Codex-Turn-Metadata": {`invalid-json`}},
	}))
}

func TestPromptRecordSessionIDUsesProviderSessionHeaderWithoutRequestIDFallback(t *testing.T) {
	require.Equal(t, "codex-session", promptRecordSessionID(map[string][]string{
		"Session-Id": {"codex-session"}, "X-Claude-Code-Session-Id": {"claude-session"},
	}))
	require.Equal(t, "claude-session", promptRecordSessionID(map[string][]string{
		"X-Claude-Code-Session-Id": {"claude-session"},
	}))
	require.Empty(t, promptRecordSessionID(nil))
	require.Len(t, []byte(promptRecordSessionID(map[string][]string{
		"Session-Id": {strings.Repeat("会", 100)},
	})), 126)
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

func TestPromptRecordResponseQueueIsIndependentFromBlockedRequestWrites(t *testing.T) {
	repo := &blockingPromptRecordRepository{
		started: make(chan struct{}, 1), release: make(chan struct{}), responseUpdated: make(chan PromptRecordKey, 1),
	}
	service := newPromptRecordService(repo, 1, 1, 1)
	service.RecordPrompt(context.Background(), Request{
		RequestID: "blocked-write", Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	})
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("record worker did not start")
	}

	_, responseReference := newPromptRecordingRequestPair(Request{RequestID: "response", Stage: "http", APIKeyID: 7})
	key := PromptRecordKey{ID: 42}
	responseReference.recordingCorrelation.complete(key, true)
	service.RecordResponse(context.Background(), responseReference, PromptResponse{Text: "ok", CapturedAt: time.Now()})

	select {
	case updated := <-repo.responseUpdated:
		require.Equal(t, key, updated)
	case <-time.After(time.Second):
		t.Fatal("response update was blocked by the request record worker")
	}
	close(repo.release)
}

type recoveringPromptRecordRepository struct {
	blockingPromptRecordRepository
	calls atomic.Int64
	done  chan struct{}
}

func (r *recoveringPromptRecordRepository) InsertPromptRecord(context.Context, *PromptRecord) (int64, error) {
	if r.calls.Add(1) == 1 {
		panic("record storage panic")
	}
	r.done <- struct{}{}
	return r.calls.Load(), nil
}

func TestPromptRecordWorkerRecoversFromStoragePanic(t *testing.T) {
	repo := &recoveringPromptRecordRepository{done: make(chan struct{}, 1)}
	service := newPromptRecordService(repo, 2, 1, 1)
	request := Request{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"hello"}]}`)}
	service.RecordPrompt(context.Background(), request)
	service.RecordPrompt(context.Background(), request)
	select {
	case <-repo.done:
	case <-time.After(time.Second):
		t.Fatal("record worker stopped after a storage panic")
	}
	require.GreaterOrEqual(t, service.QueueStats().PersistFailed, int64(1))
}

func TestListPromptRecordsReturnsCallMetadataWithoutPromptText(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repository := NewPostgreSQLRepository(db)
	createdAt := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM prompt_records WHERE (expires_at IS NULL OR expires_at > NOW())")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, session_id, turn_no, stage.*FROM prompt_records WHERE .*expires_at.* ORDER BY created_at DESC, id DESC").
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "turn_no", "stage", "user_id", "username_snapshot", "user_email_snapshot",
			"api_key_id", "api_key_name_snapshot", "group_id", "group_name", "provider", "endpoint", "protocol",
			"model", "prompt_hash", "prompt_length", "message_count", "risk_status", "risk_checked_at", "created_at", "expires_at",
		}).AddRow(
			7, "session-7", 1, "http", 2, "alice", "alice@example.com", 3, "primary", nil, "", "openai",
			"/v1/chat/completions", "openai_chat", "gpt-test", "hash", 128, 4, "pending", nil, createdAt, nil,
		))

	page, err := repository.ListPromptRecords(context.Background(), PromptRecordFilter{}, 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "session-7", page.Items[0].SessionID)
	require.Equal(t, 128, page.Items[0].PromptLength)

	payload, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "prompt_text")
	require.NotContains(t, string(payload), "risk_result")
	require.NoError(t, mock.ExpectationsWereMet())
}

var _ PromptRecordRepository = (*blockingPromptRecordRepository)(nil)
