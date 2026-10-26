//go:build integration

package openaiasync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/testutil/asyncdb"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(asyncdb.Open(t), 1<<20, 5)
}

func testJob(t *testing.T, s *Store, n int) *Job {
	t.Helper()
	j := &Job{Batch: Batch{ID: NewID("batch_"), Object: "batch", Status: "validating", Endpoint: "/v1/responses", CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(), RequestCounts: Counts{Total: n}}, UserID: 1, APIKeyID: 1, GroupID: 1, RemoteIP: "127.0.0.1", OutputTTL: time.Hour}
	items := make([]Item, n)
	for idx := range items {
		items[idx] = Item{CustomID: NewID("custom_"), Body: json.RawMessage(`{"model":"m","input":"hi\u0000","large":9007199254740993,"huge":1e1000000}`)}
	}
	require.NoError(t, s.CreateBatch(context.Background(), j, items))
	return j
}

func TestStoreFilesIsolationQuotaPagination(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.quota = 10
	f, err := s.PutFile(ctx, 1, "a.txt", "user_data", []byte("12345"), time.Hour)
	require.NoError(t, err)
	_, _, err = s.GetFile(ctx, 2, f.ID)
	require.ErrorIs(t, err, ErrNotFound)
	metadata, err := s.FileMetadata(ctx, 1, f.ID)
	require.NoError(t, err)
	require.Equal(t, f, metadata)
	_, err = s.FileMetadata(ctx, 2, f.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.PutFile(ctx, 1, "b.txt", "user_data", []byte("123456"), time.Hour)
	require.ErrorIs(t, err, ErrQuota)
	_, err = s.PutFile(ctx, 2, "b.txt", "user_data", []byte("123456"), time.Hour)
	require.NoError(t, err)
	_, err = s.PutFile(ctx, 1, "c.txt", "user_data", []byte("12345"), time.Hour)
	require.NoError(t, err)
	files, err := s.ListFiles(ctx, 1, "user_data", "", false, 1)
	require.NoError(t, err)
	require.Len(t, files, 1)
	next, err := s.ListFiles(ctx, 1, "user_data", files[0].ID, false, 10)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.NotEqual(t, files[0].ID, next[0].ID)
	require.ErrorIs(t, s.DeleteFile(ctx, 2, f.ID), ErrNotFound)
	require.NoError(t, s.DeleteFile(ctx, 1, f.ID))
}

func TestStoreBatchLifecycleAndOwnership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	original := testJob(t, s, 2)
	_, err := s.GetBatch(ctx, 2, original.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Cancel(ctx, 2, original.ID)
	require.ErrorIs(t, err, ErrNotFound)
	j, err := s.Claim(ctx)
	require.NoError(t, err)
	require.Equal(t, "in_progress", j.Status)
	other, err := s.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, other)
	i, err := s.Next(ctx, j)
	require.NoError(t, err)
	require.Contains(t, string(i.Body), `"input":"hi\u0000"`)
	require.Contains(t, string(i.Body), "9007199254740993")
	require.Contains(t, string(i.Body), "1e1000000")
	r := Result{ID: NewID("batch_req_"), CustomID: i.CustomID, Response: &Response{StatusCode: 200, Body: json.RawMessage(`{"text":"ok","large":9007199254740993}`)}}
	require.NoError(t, s.SaveResult(ctx, j, i, r, 1<<20))
	require.ErrorIs(t, s.SaveResult(ctx, j, i, r, 1<<20), ErrLeaseLost)
	i, err = s.Next(ctx, j)
	require.NoError(t, err)
	require.NoError(t, s.SaveResult(ctx, j, i, Failure(i.CustomID, "upstream_error", "failed"), 1<<20))
	i, err = s.Next(ctx, j)
	require.NoError(t, err)
	require.Nil(t, i)
	require.NoError(t, s.Finish(ctx, j, 1<<20))
	b, err := s.GetBatch(ctx, 1, j.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", b.Status)
	require.Equal(t, Counts{Total: 2, Completed: 1, Failed: 1}, b.RequestCounts)
	require.NotNil(t, b.OutputFileID)
	require.NotNil(t, b.ErrorFileID)
	_, body, err := s.GetFile(ctx, 1, *b.OutputFileID)
	require.NoError(t, err)
	require.Contains(t, string(body), "9007199254740993")
	var count int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM openai_batch_items WHERE batch_id=$1`, j.ID).Scan(&count))
	require.Zero(t, count)
}

func TestStoreRecoveryFencesAndNeverReplaysRunningItem(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	testJob(t, s, 2)
	j, err := s.Claim(ctx)
	require.NoError(t, err)
	i, err := s.Next(ctx, j)
	require.NoError(t, err)
	_, err = s.db.Exec(`UPDATE openai_batches SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, j.ID)
	require.NoError(t, err)
	recovered, err := s.Claim(ctx)
	require.NoError(t, err)
	require.NotEqual(t, j.Lease, recovered.Lease)
	require.Equal(t, 1, recovered.RequestCounts.Failed)
	var used, actual int64
	require.NoError(t, s.db.QueryRow(`SELECT result_bytes FROM openai_batches WHERE id=$1`, j.ID).Scan(&used))
	require.NoError(t, s.db.QueryRow(`SELECT SUM(octet_length(result::text)+1) FROM openai_batch_items WHERE batch_id=$1 AND result IS NOT NULL`, j.ID).Scan(&actual))
	require.Equal(t, actual, used)
	require.ErrorIs(t, s.SaveResult(ctx, j, i, Failure(i.CustomID, "old_worker", "old"), 1<<20), ErrLeaseLost)
	require.ErrorIs(t, s.Renew(ctx, j), ErrLeaseLost)
	next, err := s.Next(ctx, recovered)
	require.NoError(t, err)
	require.NotEqual(t, i.CustomID, next.CustomID)
	require.NoError(t, s.SaveResult(ctx, recovered, next, Failure(next.CustomID, "test", "test"), 1<<20))
	require.NoError(t, s.Finish(ctx, recovered, 1<<20))
	b, err := s.GetBatch(ctx, 1, j.ID)
	require.NoError(t, err)
	_, body, err := s.GetFile(ctx, 1, *b.ErrorFileID)
	require.NoError(t, err)
	require.Contains(t, string(body), "batch_execution_interrupted")
}

func TestStoreCancellationExpiryAndOutputLimits(t *testing.T) {
	for _, mode := range []string{"cancel", "expire", "quota"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			original := testJob(t, s, 1)
			if mode == "cancel" {
				_, err := s.Cancel(ctx, 1, original.ID)
				require.NoError(t, err)
			}
			j, err := s.Claim(ctx)
			require.NoError(t, err)
			if mode == "expire" {
				j.ExpiresAt = time.Now().Add(-time.Hour).Unix()
				raw, _ := json.Marshal(j.Batch)
				_, err = s.db.Exec(`UPDATE openai_batches SET payload=$2 WHERE id=$1`, j.ID, raw)
				require.NoError(t, err)
			}
			i, err := s.Next(ctx, j)
			require.NoError(t, err)
			if mode == "quota" {
				require.NotNil(t, i)
				s.quota = 1
				require.NoError(t, s.SaveResult(ctx, j, i, Failure(i.CustomID, "test", "test"), 1<<20))
			} else {
				require.Nil(t, i)
			}
			require.NoError(t, s.Finish(ctx, j, 1<<20))
			b, err := s.GetBatch(ctx, 1, j.ID)
			require.NoError(t, err)
			require.Equal(t, map[string]string{"cancel": "cancelled", "expire": "expired", "quota": "failed"}[mode], b.Status)
		})
	}
}

func TestStoreOutputBudgetReservesEscapedFailureIDs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	original := testJob(t, s, 2)
	for ordinal := 0; ordinal < 2; ordinal++ {
		// Each control byte expands to six bytes in the persisted JSONL.
		customID := fmt.Sprintf("%d", ordinal) + strings.Repeat("\x01", 511)
		_, err := s.db.Exec(`UPDATE openai_batch_items SET custom_id=$3 WHERE batch_id=$1 AND ordinal=$2`, original.ID, ordinal, customID)
		require.NoError(t, err)
	}
	j, err := s.Claim(ctx)
	require.NoError(t, err)
	for ordinal := 0; ordinal < 2; ordinal++ {
		i, err := s.Next(ctx, j)
		require.NoError(t, err)
		require.NotNil(t, i)
		body, err := json.Marshal(map[string]string{"text": strings.Repeat("x", 8192)})
		require.NoError(t, err)
		r := Result{ID: NewID("batch_req_"), CustomID: i.CustomID, Response: &Response{StatusCode: 200, Body: body}}
		require.NoError(t, s.SaveResult(ctx, j, i, r, 8192))
	}
	require.NoError(t, s.Finish(ctx, j, 8192))
	b, err := s.GetBatch(ctx, 1, j.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", b.Status)
	require.Equal(t, Counts{Total: 2, Failed: 2}, b.RequestCounts)
	require.NotNil(t, b.ErrorFileID)
	_, body, err := s.GetFile(ctx, 1, *b.ErrorFileID)
	require.NoError(t, err)
	require.LessOrEqual(t, len(body), 8192)
	require.Equal(t, 2, strings.Count(string(body), "response_too_large"))
}

func TestStoreConcurrentClaimsAndAdmission(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.maxActive = 1
	testJob(t, s, 1)
	var wg sync.WaitGroup
	claims := make(chan *Job, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := s.Claim(ctx)
			require.NoError(t, err)
			if j != nil {
				claims <- j
			}
		}()
	}
	wg.Wait()
	close(claims)
	require.Len(t, claims, 1)
	j := &Job{Batch: Batch{ID: NewID("batch_"), Status: "validating"}, UserID: 1, APIKeyID: 1, GroupID: 1}
	require.ErrorIs(t, s.CreateBatch(ctx, j, nil), ErrQuota)
}

func TestWorkerRenewsLeaseDuringLongExecution(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	original := testJob(t, s, 2)
	started := make(chan struct{})
	release := make(chan struct{})
	worker := NewWorker(s, func(ctx context.Context, _ *Job, item *Item) Result {
		if item.Ordinal == 0 {
			close(started)
			select {
			case <-ctx.Done():
			case <-release:
			}
		}
		return Failure(item.CustomID, "test_result", "test result")
	}, 1, 1<<20, 30*time.Second)
	worker.Start()
	t.Cleanup(worker.Stop)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	var initial time.Time
	require.NoError(t, s.db.QueryRow(`SELECT lease_until FROM openai_batches WHERE id=$1`, original.ID).Scan(&initial))
	require.Eventually(t, func() bool {
		var renewed time.Time
		return s.db.QueryRow(`SELECT lease_until FROM openai_batches WHERE id=$1`, original.ID).Scan(&renewed) == nil && renewed.After(initial.Add(5*time.Second))
	}, 15*time.Second, 100*time.Millisecond)
	close(release)
	require.Eventually(t, func() bool {
		b, err := s.GetBatch(ctx, 1, original.ID)
		return err == nil && b.Status == "completed" && b.RequestCounts.Failed == 2
	}, 5*time.Second, 20*time.Millisecond)
}
