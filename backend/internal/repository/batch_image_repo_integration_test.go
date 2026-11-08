//go:build integration

package repository

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newBatchImageRepositoryWithSQL(sqlq batchImageSQLExecutor) *batchImageRepository {
	return &batchImageRepository{sql: sqlq}
}

func TestBatchImageRepository_CreateJobAndDuplicates(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "create")

	job, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:       batchID,
		UserID:        1001,
		Provider:      service.BatchImageProviderGeminiAPI,
		Model:         "gemini-2.5-flash-image",
		ItemCount:     2,
		EstimatedCost: 0.02,
	})
	require.NoError(t, err)
	require.Equal(t, batchID, job.BatchID)
	require.Equal(t, service.BatchImageJobStatusCreated, job.Status)
	require.Equal(t, "USD", job.Currency)

	_, err = repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageJobExists))
}

func TestBatchImageRepository_InvalidProvider(t *testing.T) {
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)

	_, err := repo.CreateBatchImageJob(context.Background(), service.CreateBatchImageJobParams{
		BatchID:   batchImageTestID(t, "provider"),
		UserID:    1001,
		Provider:  "unknown",
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageInvalidProvider))
}

func TestBatchImageRepository_TransitionIncrementsVersionAndEvents(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "transition")
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderVertex,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusUploading, service.BatchImageTransitionOptions{
		EventType:    "status_changed",
		EventPayload: map[string]any{"to": service.BatchImageJobStatusUploading},
		Now:          &now,
	})
	require.NoError(t, err)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusUploading, job.Status)
	require.Equal(t, 1, job.Version)

	var eventCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM batch_image_events WHERE job_id = $1 AND event_type = 'status_changed'`, batchID).Scan(&eventCount)
	require.NoError(t, err)
	require.Equal(t, 1, eventCount)
}

func TestBatchImageRepository_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "invalid-transition")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusRunning, service.BatchImageTransitionOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageInvalidTransition))
}

func TestBatchImageRepository_TerminalStatusCannotMoveBack(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "terminal")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		Status:    service.BatchImageJobStatusCompleted,
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusRunning, service.BatchImageTransitionOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageInvalidTransition))
}

func TestBatchImageRepository_ItemCustomIDUniqueness(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	firstBatchID := batchImageTestID(t, "items-a")
	secondBatchID := batchImageTestID(t, "items-b")

	for _, batchID := range []string{firstBatchID, secondBatchID} {
		_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
			BatchID:   batchID,
			UserID:    1001,
			Provider:  service.BatchImageProviderGeminiAPI,
			Model:     "gemini-2.5-flash-image",
			ItemCount: 1,
		})
		require.NoError(t, err)
	}

	_, err := repo.CreateBatchImageItem(ctx, service.CreateBatchImageItemParams{
		JobID:      firstBatchID,
		CustomID:   "line-1",
		Status:     service.BatchImageItemStatusSuccess,
		ImageCount: 1,
	})
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `SAVEPOINT batch_image_duplicate_item`)
	require.NoError(t, err)
	_, err = repo.CreateBatchImageItem(ctx, service.CreateBatchImageItemParams{
		JobID:    firstBatchID,
		CustomID: "line-1",
		Status:   service.BatchImageItemStatusFailed,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageItemExists))
	_, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT batch_image_duplicate_item`)
	require.NoError(t, rollbackErr)

	_, err = repo.CreateBatchImageItem(ctx, service.CreateBatchImageItemParams{
		JobID:      secondBatchID,
		CustomID:   "line-1",
		Status:     service.BatchImageItemStatusSuccess,
		ImageCount: 1,
	})
	require.NoError(t, err)

	items, err := repo.ListBatchImageItems(ctx, firstBatchID, service.BatchImageItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestBatchImageRepository_ReplaceBatchImageItemsForJob(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "replace-items")
	lineOne := 1
	lineTwo := 2

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 2,
	})
	require.NoError(t, err)

	// 非 indexing 状态不允许重建 item 表：防止锁过期后掉队的 worker
	// 重写已完成/已结算 job 的条目。
	err = repo.ReplaceBatchImageItemsForJob(ctx, batchID, []service.CreateBatchImageItemParams{
		{CustomID: "old", Status: service.BatchImageItemStatusSuccess, SourceLineNumber: &lineOne, ImageCount: 1},
	}, service.BatchImageCounts{SuccessCount: 1})
	require.ErrorIs(t, err, service.ErrBatchImageIndexStateConflict)

	require.NoError(t, repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusSubmitted, service.BatchImageTransitionOptions{}))
	require.NoError(t, repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusIndexing, service.BatchImageTransitionOptions{}))

	err = repo.ReplaceBatchImageItemsForJob(ctx, batchID, []service.CreateBatchImageItemParams{
		{CustomID: "old", Status: service.BatchImageItemStatusSuccess, SourceLineNumber: &lineOne, ImageCount: 1},
	}, service.BatchImageCounts{SuccessCount: 1})
	require.NoError(t, err)

	err = repo.ReplaceBatchImageItemsForJob(ctx, batchID, []service.CreateBatchImageItemParams{
		{CustomID: "new-ok", Status: service.BatchImageItemStatusSuccess, SourceLineNumber: &lineOne, ImageCount: 1},
		{CustomID: "new-fail", Status: service.BatchImageItemStatusFailed, SourceLineNumber: &lineTwo, ErrorCode: batchImageTestStringPtr("SAFETY_BLOCKED")},
	}, service.BatchImageCounts{SuccessCount: 1, FailCount: 1})
	require.NoError(t, err)

	items, err := repo.ListBatchImageItems(ctx, batchID, service.BatchImageItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "new-ok", items[0].CustomID)
	require.Equal(t, "new-fail", items[1].CustomID)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, 1, job.SuccessCount)
	require.Equal(t, 1, job.FailCount)
}

func TestBatchImageRepository_MarkBatchImageJobSettled(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "settled")
	apiKeyID := int64(2001)
	accountID := int64(3001)
	providerJob := "providers/job"
	outputRef := "files/output"
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:           batchID,
		UserID:            1001,
		APIKeyID:          &apiKeyID,
		AccountID:         &accountID,
		Provider:          service.BatchImageProviderGeminiAPI,
		Model:             "gemini-image",
		Status:            service.BatchImageJobStatusSettling,
		ProviderJobName:   &providerJob,
		ProviderOutputRef: &outputRef,
		ItemCount:         3,
		SuccessCount:      2,
		FailCount:         1,
	})
	require.NoError(t, err)

	err = repo.MarkBatchImageJobSettled(ctx, service.MarkBatchImageJobSettledParams{
		BatchID:      batchID,
		ActualCost:   0.5,
		ManifestHash: "manifest-hash",
		EventPayload: map[string]any{"request_id": "batch_image_settlement:" + batchID},
		Now:          &now,
	})
	require.NoError(t, err)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusCompleted, job.Status)
	require.NotNil(t, job.ActualCost)
	require.Equal(t, 0.5, *job.ActualCost)
	require.Equal(t, "manifest-hash", batchImageDerefTest(job.ManifestHash))
	require.NotNil(t, job.SettledAt)
	require.Equal(t, now, *job.SettledAt)

	var eventCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM batch_image_events WHERE job_id = $1 AND event_type = 'settlement_completed'`, batchID).Scan(&eventCount)
	require.NoError(t, err)
	require.Equal(t, 1, eventCount)
}

func TestBatchImageRepository_SetBatchImageJobSettlementFailed(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "settlement-failed")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:      batchID,
		UserID:       1001,
		Provider:     service.BatchImageProviderGeminiAPI,
		Model:        "gemini-image",
		Status:       service.BatchImageJobStatusSettling,
		ItemCount:    1,
		SuccessCount: 1,
	})
	require.NoError(t, err)

	retryCount, err := repo.SetBatchImageJobSettlementFailed(ctx, batchID, "SETTLEMENT_BILLING_FAILED", "temporary")
	require.NoError(t, err)
	require.Equal(t, 1, retryCount)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusSettling, job.Status)
	require.Equal(t, "SETTLEMENT_BILLING_FAILED", batchImageDerefTest(job.LastErrorCode))
	require.Equal(t, "temporary", batchImageDerefTest(job.LastErrorMessage))
	require.Equal(t, 1, job.RetryCount)
}

func TestBatchImageRepository_AppendEvent(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "event")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderVertex,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.AppendBatchImageEvent(ctx, batchID, "job_created", map[string]any{"batch_id": batchID})
	require.NoError(t, err)

	var payload string
	err = tx.QueryRowContext(ctx, `SELECT payload::text FROM batch_image_events WHERE job_id = $1 AND event_type = 'job_created'`, batchID).Scan(&payload)
	require.NoError(t, err)
	require.Contains(t, payload, batchID)
}

func batchImageTestID(t *testing.T, prefix string) string {
	t.Helper()
	safePrefix := batchImageSafeTestIDSegment(prefix, 20)
	sum := sha1.Sum([]byte(t.Name()))
	return "imgbatch_" + safePrefix + "_" + hex.EncodeToString(sum[:])[:16]
}

func batchImageSafeTestIDSegment(v string, maxLen int) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(v, "-")
	v = strings.Trim(v, "-_")
	if v == "" {
		v = "job"
	}
	if len(v) > maxLen {
		v = v[:maxLen]
		v = strings.Trim(v, "-_")
	}
	if v == "" {
		return "job"
	}
	return v
}

func batchImageTestStringPtr(v string) *string {
	return &v
}

func batchImageDerefTest(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
