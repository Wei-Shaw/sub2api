//go:build integration

package openaiasync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/testutil/asyncdb"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(asyncdb.Open(t), 1<<20, 1<<20, 16384, 5)
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
			if mode == "quota" {
				require.ErrorIs(t, s.Finish(ctx, j, 1<<20), ErrQuota)
			} else {
				require.NoError(t, s.Finish(ctx, j, 1<<20))
			}
			b, err := s.GetBatch(ctx, 1, j.ID)
			require.NoError(t, err)
			require.Equal(t, map[string]string{"cancel": "cancelled", "expire": "expired", "quota": "finalizing"}[mode], b.Status)
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

// Exercise multiple Store instances sharing one database: admission and uploads
// must honor the same reservation even when concurrent workers use other replicas.
func TestStoreConcurrentStorageReservations(t *testing.T) {
	db := asyncdb.Open(t)
	const budget = int64(16 << 10)
	const count = 32
	s := NewStore(db, 1<<20, 3*budget+1024, budget, count)
	ctx := context.Background()
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for n := 0; n < count; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replica := NewStore(db, s.quota, s.tempQuota, budget, count)
			j := &Job{Batch: Batch{ID: NewID("batch_"), Status: "validating", ExpiresAt: time.Now().Add(time.Hour).Unix()}, UserID: 1, APIKeyID: 1, GroupID: 1}
			err := replica.CreateBatch(ctx, j, []Item{{CustomID: "request", Body: json.RawMessage(`{"model":"m","input":"hello"}`)}})
			if err == nil {
				admitted.Add(1)
			} else {
				require.ErrorIs(t, err, ErrQuota)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 3, admitted.Load())
	var temporary int64
	require.NoError(t, db.QueryRow(`SELECT SUM(input_bytes+reserved_output_bytes) FROM openai_batches WHERE user_id=1`).Scan(&temporary))
	require.LessOrEqual(t, temporary, s.tempQuota)
	// Temporary copies and reservations also reduce the shared file quota.
	_, err := s.PutFile(ctx, 1, "too-large.jsonl", "batch", make([]byte, s.quota-temporary+1), time.Hour)
	require.ErrorIs(t, err, ErrQuota)
	fill, err := s.PutFile(ctx, 1, "fill.jsonl", "batch", make([]byte, s.quota-temporary), time.Hour)
	require.NoError(t, err)
	_, err = s.PutFile(ctx, 1, "one-byte.jsonl", "batch", []byte("x"), time.Hour)
	require.ErrorIs(t, err, ErrQuota)
	require.NoError(t, s.DeleteFile(ctx, 1, fill.ID))
}

func TestStoreReservedOutputSurvivesConcurrentUploads(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	original := testJob(t, s, 2)
	var temporary int64
	require.NoError(t, s.db.QueryRow(`SELECT input_bytes+reserved_output_bytes FROM openai_batches WHERE id=$1`, original.ID).Scan(&temporary))
	// Fill all unreserved space while inference is running.
	_, err := s.PutFile(ctx, 1, "fill.jsonl", "batch", make([]byte, s.quota-temporary), time.Hour)
	require.NoError(t, err)
	j, err := s.Claim(ctx)
	require.NoError(t, err)
	for n := 0; n < 2; n++ {
		item, e := s.Next(ctx, j)
		require.NoError(t, e)
		result := Result{ID: NewID("batch_req_"), CustomID: item.CustomID, Response: &Response{StatusCode: 200, Body: json.RawMessage(`{"answer":"paid-result"}`)}}
		require.NoError(t, s.SaveResult(ctx, j, item, result, s.maxOutput))
	}
	require.NoError(t, s.Finish(ctx, j, s.maxOutput))
	b, err := s.GetBatch(ctx, 1, j.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", b.Status)
	require.NotNil(t, b.OutputFileID)
	_, body, err := s.GetFile(ctx, 1, *b.OutputFileID)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(body), "paid-result"))
	var remaining int64
	require.NoError(t, s.db.QueryRow(`SELECT input_bytes+reserved_output_bytes+result_bytes FROM openai_batches WHERE id=$1`, j.ID).Scan(&remaining))
	require.Zero(t, remaining)
}

func TestStoreFinalizationQuotaRecoveryDoesNotExecuteAgain(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	testJob(t, s, 2)
	j, err := s.Claim(ctx)
	require.NoError(t, err)
	for n := 0; n < 2; n++ {
		item, e := s.Next(ctx, j)
		require.NoError(t, e)
		var result Result
		if n == 0 {
			result = Result{ID: NewID("batch_req_"), CustomID: item.CustomID, Response: &Response{StatusCode: 200, Body: json.RawMessage(`{"answer":"paid-result"}`)}}
		} else {
			result = Failure(item.CustomID, "upstream_rejected", "original failure")
		}
		require.NoError(t, s.SaveResult(ctx, j, item, result, s.maxOutput))
	}
	// A later administrator quota reduction must not destroy either result.
	s.quota = 1
	require.ErrorIs(t, s.Finish(ctx, j, s.maxOutput), ErrQuota)
	b, err := s.GetBatch(ctx, 1, j.ID)
	require.NoError(t, err)
	require.Equal(t, "finalizing", b.Status)
	require.Nil(t, b.OutputFileID)
	var rows int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM openai_batch_items WHERE batch_id=$1 AND result IS NOT NULL`, j.ID).Scan(&rows))
	require.Equal(t, 2, rows)
	require.ErrorIs(t, s.Finish(ctx, j, s.maxOutput), ErrQuota)
	// A new replica/process retries export after capacity returns, not inference.
	restarted := NewStore(s.db, 1<<20, s.tempQuota, s.maxOutput, 5)
	var executed atomic.Int64
	_, err = s.db.Exec(`UPDATE openai_batches SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, j.ID)
	require.NoError(t, err)
	worker := NewWorker(restarted, func(context.Context, *Job, *Item) Result {
		executed.Add(1)
		return Failure("unexpected", "test", "must not execute")
	}, 1, s.maxOutput, time.Second)
	worker.Start()
	defer worker.Stop()
	require.Eventually(t, func() bool { b, err = restarted.GetBatch(ctx, 1, j.ID); return err == nil && b.Status == "completed" }, 5*time.Second, 20*time.Millisecond)
	worker.Stop()
	require.Zero(t, executed.Load())
	require.Equal(t, Counts{Total: 2, Completed: 1, Failed: 1}, b.RequestCounts)
	require.Nil(t, b.Errors)
	require.NotNil(t, b.OutputFileID)
	require.NotNil(t, b.ErrorFileID)
	_, body, err := restarted.GetFile(ctx, 1, *b.OutputFileID)
	require.NoError(t, err)
	require.Contains(t, string(body), "paid-result")
	_, body, err = restarted.GetFile(ctx, 1, *b.ErrorFileID)
	require.NoError(t, err)
	require.Contains(t, string(body), "original failure")
}

func TestStoreLegacyReservationWaitFencesOldWorker(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	testJob(t, s, 2)
	old, err := s.Claim(ctx)
	require.NoError(t, err)
	running, err := s.Next(ctx, old)
	require.NoError(t, err)
	// Simulate a migrated queue entry that never had an output reservation.
	_, err = s.db.Exec(`UPDATE openai_batches SET reserved_output_bytes=0,lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, old.ID)
	require.NoError(t, err)
	s.quota = 1
	waiting, err := s.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, waiting)
	require.ErrorIs(t, s.Renew(ctx, old), ErrLeaseLost)
	require.ErrorIs(t, s.SaveResult(ctx, old, running, Failure(running.CustomID, "old", "old"), s.maxOutput), ErrLeaseLost)
	s.quota = 1 << 20
	_, err = s.db.Exec(`UPDATE openai_batches SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, old.ID)
	require.NoError(t, err)
	resumed, err := s.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, resumed)
	require.Equal(t, s.maxOutput, resumed.OutputBudget)
	require.Equal(t, 1, resumed.RequestCounts.Failed)
	pending, err := s.Next(ctx, resumed)
	require.NoError(t, err)
	require.NotEqual(t, running.Ordinal, pending.Ordinal)
}

func TestStoreCancellingFinalizationKeepsSkippedItemsSkipped(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	original := testJob(t, s, 2)
	j, err := s.Claim(ctx)
	require.NoError(t, err)
	item, err := s.Next(ctx, j)
	require.NoError(t, err)
	require.NoError(t, s.SaveResult(ctx, j, item, Failure(item.CustomID, "original", "original error"), s.maxOutput))
	_, err = s.Cancel(ctx, 1, original.ID)
	require.NoError(t, err)
	s.quota = 1
	require.ErrorIs(t, s.Finish(ctx, j, s.maxOutput), ErrQuota)
	// Output pressure can last past the completion window; cancelled pending
	// items must not later be converted into expiry errors on an export retry.
	_, err = s.db.Exec(`UPDATE openai_batches SET payload=jsonb_set(payload,'{expires_at}','1') WHERE id=$1`, j.ID)
	require.NoError(t, err)
	s.quota = 1 << 20
	require.NoError(t, s.Finish(ctx, j, s.maxOutput))
	b, err := s.GetBatch(ctx, 1, j.ID)
	require.NoError(t, err)
	require.Equal(t, "cancelled", b.Status)
	require.Equal(t, Counts{Total: 2, Failed: 1}, b.RequestCounts)
	_, body, err := s.GetFile(ctx, 1, *b.ErrorFileID)
	require.NoError(t, err)
	require.Contains(t, string(body), "original error")
	require.NotContains(t, string(body), "batch_expired")
}
