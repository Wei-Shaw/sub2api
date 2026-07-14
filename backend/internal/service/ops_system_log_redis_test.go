package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedisSystemLogSinkTest(t *testing.T, repo OpsRepository) (*OpsSystemLogSink, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sink := NewOpsSystemLogSink(repo)
	sink.ConfigureRedisOnlyStore(client, nil)
	sink.SetRedisOnly(true)
	return sink, client
}

func TestRedisSystemLogStoreRetainsLatest500(t *testing.T) {
	sink, client := newRedisSystemLogSinkTest(t, nil)
	base := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)
	inputs := make([]*OpsInsertSystemLogInput, 0, 600)
	for i := 0; i < 600; i++ {
		inputs = append(inputs, &OpsInsertSystemLogInput{
			CreatedAt: base.Add(time.Duration(i) * time.Millisecond),
			Host:      "api-node-1",
			Level:     "warn",
			Component: "app",
			Message:   fmt.Sprintf("log-%03d", i),
			RequestID: fmt.Sprintf("req-%03d", i),
			ExtraJSON: `{"status_code":500}`,
		})
	}

	written, err := sink.writeRedisSystemLogs(context.Background(), inputs)
	if err != nil {
		t.Fatalf("writeRedisSystemLogs() error: %v", err)
	}
	if written != 600 {
		t.Fatalf("written = %d, want 600", written)
	}
	if length := client.LLen(context.Background(), redisSystemLogKey).Val(); length != redisSystemLogLimit {
		t.Fatalf("redis list length = %d, want %d", length, redisSystemLogLimit)
	}

	result, err := sink.ListRedisSystemLogs(context.Background(), &OpsSystemLogFilter{Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("ListRedisSystemLogs() error: %v", err)
	}
	if result.Total != redisSystemLogLimit || len(result.Logs) != 200 {
		t.Fatalf("unexpected list result: total=%d len=%d", result.Total, len(result.Logs))
	}
	if result.Logs[0].Message != "log-599" {
		t.Fatalf("newest message = %q, want log-599", result.Logs[0].Message)
	}

	lastPage, err := sink.ListRedisSystemLogs(context.Background(), &OpsSystemLogFilter{Page: 3, PageSize: 200})
	if err != nil {
		t.Fatalf("ListRedisSystemLogs(page 3) error: %v", err)
	}
	if len(lastPage.Logs) != 100 {
		t.Fatalf("last page length = %d, want 100", len(lastPage.Logs))
	}
	if lastPage.Logs[99].Message != "log-100" {
		t.Fatalf("oldest retained log = %q, want log-100", lastPage.Logs[99].Message)
	}
}

func TestOpsServiceRedisOnlyListAndCleanupNeverUseDatabase(t *testing.T) {
	dbCalls := 0
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(context.Context, []*OpsInsertSystemLogInput) (int64, error) {
			dbCalls++
			return 0, errors.New("database insert must not be called")
		},
		ListSystemLogsFn: func(context.Context, *OpsSystemLogFilter) (*OpsSystemLogList, error) {
			dbCalls++
			return nil, errors.New("database list must not be called")
		},
		DeleteSystemLogsFn: func(context.Context, *OpsSystemLogCleanupFilter) (int64, error) {
			dbCalls++
			return 0, errors.New("database cleanup must not be called")
		},
		InsertSystemLogCleanupAuditFn: func(context.Context, *OpsSystemLogCleanupAudit) error {
			dbCalls++
			return errors.New("database audit must not be called")
		},
	}
	sink, _ := newRedisSystemLogSinkTest(t, repo)
	_, err := sink.flushBatch(context.Background(), []*logger.LogEvent{
		{
			Time: time.Now().UTC(), Level: "warn", Component: "app", Message: "timeout on upstream",
			Fields: map[string]any{"request_id": "req-match", "user_id": int64(12), "platform": "openai", "error": "timeout"},
		},
		{
			Time: time.Now().UTC().Add(-time.Second), Level: "error", Component: "app", Message: "other error",
			Fields: map[string]any{"request_id": "req-other", "platform": "gemini"},
		},
	})
	if err != nil {
		t.Fatalf("seed redis logs: %v", err)
	}

	svc := &OpsService{opsRepo: repo, systemLogSink: sink}
	listed, err := svc.ListSystemLogs(context.Background(), &OpsSystemLogFilter{
		Platform: "openai",
		Query:    "TIMEOUT",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListSystemLogs() error: %v", err)
	}
	if listed.Total != 1 || listed.Logs[0].RequestID != "req-match" {
		t.Fatalf("unexpected filtered logs: %+v", listed)
	}

	deleted, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{RequestID: "req-match"}, 99)
	if err != nil {
		t.Fatalf("CleanupSystemLogs() error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	remaining, err := svc.ListSystemLogs(context.Background(), &OpsSystemLogFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListSystemLogs() after cleanup error: %v", err)
	}
	if remaining.Total != 1 || remaining.Logs[0].RequestID != "req-other" {
		t.Fatalf("unexpected remaining logs: %+v", remaining)
	}
	deleted, err = svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{ClearAll: true}, 99)
	if err != nil {
		t.Fatalf("CleanupSystemLogs(clear all) error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("clear-all deleted = %d, want 1", deleted)
	}
	remaining, err = svc.ListSystemLogs(context.Background(), &OpsSystemLogFilter{Page: 1, PageSize: 20})
	if err != nil || remaining.Total != 0 {
		t.Fatalf("unexpected logs after clear all: result=%+v err=%v", remaining, err)
	}
	if dbCalls != 0 {
		t.Fatalf("database calls = %d, want 0", dbCalls)
	}
}

func TestOpsSystemLogSinkRedisOnlySettingRefreshIsStickyOnError(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.values[SettingKeyOpsRuntimeLogConfig] = `{"redis_only":true}`
	sink := NewOpsSystemLogSink(nil)
	sink.ConfigureRedisOnlyStore(nil, repo)
	if !sink.IsRedisOnly(context.Background()) {
		t.Fatalf("expected redis-only mode from setting")
	}

	repo.getValueFn = func(string) (string, error) {
		return "", errors.New("settings database unavailable")
	}
	if !sink.IsRedisOnly(context.Background()) {
		t.Fatalf("transient setting read failure should preserve the last known redis-only mode")
	}
}
