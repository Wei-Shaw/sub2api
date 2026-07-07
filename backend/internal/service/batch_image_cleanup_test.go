//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestBatchImageCleanupService_DeleteOutputsForOwner(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes completed output and returns public dto", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()

		got, err := svc.DeleteOutputsForOwner(ctx, testBatchImageOwner(), "imgbatch_cleanup")
		require.NoError(t, err)
		require.Equal(t, "output_deleted", got.Status)
		require.NotNil(t, got.OutputDeletedAt)
		require.Equal(t, []CleanupTarget{CleanupTargetOutput}, provider.cleanupTargets)
		require.NotNil(t, repo.jobs["imgbatch_cleanup"].OutputDeletedAt)
		require.Equal(t, BatchImageJobStatusOutputDeleted, repo.jobs["imgbatch_cleanup"].Status)
		body := mustJSON(t, got)
		requireBatchImagePublicJSONHasNoInternals(t, body)
	})

	t.Run("repeated delete is idempotent", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()
		deletedAt := time.Now()
		repo.jobs["imgbatch_cleanup"].Status = BatchImageJobStatusOutputDeleted
		repo.jobs["imgbatch_cleanup"].OutputDeletedAt = &deletedAt

		got, err := svc.DeleteOutputsForOwner(ctx, testBatchImageOwner(), "imgbatch_cleanup")
		require.NoError(t, err)
		require.Equal(t, "output_deleted", got.Status)
		require.Empty(t, provider.cleanupTargets)
	})

	t.Run("not completed returns not ready", func(t *testing.T) {
		svc, repo, _ := newTestBatchImageCleanupService()
		repo.jobs["imgbatch_cleanup"].Status = BatchImageJobStatusRunning

		_, err := svc.DeleteOutputsForOwner(ctx, testBatchImageOwner(), "imgbatch_cleanup")
		require.ErrorIs(t, err, ErrBatchImageOutputDeleteNotReady)
	})

	t.Run("non owner returns not found", func(t *testing.T) {
		svc, _, _ := newTestBatchImageCleanupService()
		_, err := svc.DeleteOutputsForOwner(ctx, BatchImageOwner{UserID: 11, APIKeyID: 999}, "imgbatch_cleanup")
		require.ErrorIs(t, err, ErrBatchImageJobNotFound)
	})

	t.Run("provider not found is success", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()
		provider.cleanupErr = infraerrors.New(404, "PROVIDER_NOT_FOUND", "provider file not found: gs://hidden")

		got, err := svc.DeleteOutputsForOwner(ctx, testBatchImageOwner(), "imgbatch_cleanup")
		require.NoError(t, err)
		require.Equal(t, "output_deleted", got.Status)
		require.NotNil(t, repo.jobs["imgbatch_cleanup"].OutputDeletedAt)
	})

	t.Run("provider transient error is sanitized and records failure", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()
		provider.cleanupErr = errors.New("temporary cleanup failed for gs://secret-output")

		_, err := svc.DeleteOutputsForOwner(ctx, testBatchImageOwner(), "imgbatch_cleanup")
		require.ErrorIs(t, err, ErrBatchImageProviderCleanupFailed)
		require.Equal(t, "BATCH_IMAGE_PROVIDER_CLEANUP_FAILED", infraerrors.Reason(err))
		require.NotContains(t, infraerrors.Message(err), "gs://")
		require.Equal(t, "BATCH_IMAGE_PROVIDER_CLEANUP_FAILED", batchImageDerefString(repo.jobs["imgbatch_cleanup"].LastErrorCode))
		require.Equal(t, "upstream provider operation failed", batchImageDerefString(repo.jobs["imgbatch_cleanup"].LastErrorMessage))
	})

	t.Run("unsafe cleanup path is not swallowed", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()
		provider.cleanupErr = ErrBatchImageProviderUnsafeCleanupPath

		_, err := svc.DeleteOutputsForOwner(ctx, testBatchImageOwner(), "imgbatch_cleanup")
		require.ErrorIs(t, err, ErrBatchImageCleanupUnsafePath)
		require.Equal(t, "BATCH_IMAGE_CLEANUP_UNSAFE_PATH", batchImageDerefString(repo.jobs["imgbatch_cleanup"].LastErrorCode))
	})
}

func TestBatchImageCleanupService_InputOutputAndWorker(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("input cleanup marks input only", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()

		err := svc.CleanupInput(ctx, "imgbatch_cleanup")
		require.NoError(t, err)
		require.Equal(t, []CleanupTarget{CleanupTargetInput}, provider.cleanupTargets)
		require.NotNil(t, repo.jobs["imgbatch_cleanup"].InputDeletedAt)
		require.Equal(t, BatchImageJobStatusCompleted, repo.jobs["imgbatch_cleanup"].Status)

		err = svc.CleanupInput(ctx, "imgbatch_cleanup")
		require.NoError(t, err)
		require.Len(t, provider.cleanupTargets, 1)
	})

	t.Run("output cleanup for failed job keeps status", func(t *testing.T) {
		svc, repo, _ := newTestBatchImageCleanupService()
		repo.jobs["imgbatch_cleanup"].Status = BatchImageJobStatusFailed

		err := svc.CleanupOutput(ctx, "imgbatch_cleanup", "ttl")
		require.NoError(t, err)
		require.Equal(t, BatchImageJobStatusFailed, repo.jobs["imgbatch_cleanup"].Status)
		require.NotNil(t, repo.jobs["imgbatch_cleanup"].OutputDeletedAt)
	})

	t.Run("worker processes due jobs and continues after failure", func(t *testing.T) {
		svc, repo, provider := newTestBatchImageCleanupService()
		provider.cleanupErr = nil
		old := now.Add(-48 * time.Hour)
		expired := now.Add(-time.Minute)
		future := now.Add(time.Hour)
		repo.jobs["imgbatch_cleanup"].FinishedAt = &old
		repo.jobs["imgbatch_cleanup"].OutputExpiresAt = &expired
		repo.jobs["imgbatch_running"] = cleanupTestJob("imgbatch_running", BatchImageJobStatusRunning)
		repo.jobs["imgbatch_running"].FinishedAt = &old
		repo.jobs["imgbatch_running"].OutputExpiresAt = &expired
		repo.jobs["imgbatch_future"] = cleanupTestJob("imgbatch_future", BatchImageJobStatusCompleted)
		repo.jobs["imgbatch_future"].FinishedAt = &old
		repo.jobs["imgbatch_future"].OutputExpiresAt = &future

		result, err := svc.RunOnce(ctx, now)
		require.NoError(t, err)
		require.Equal(t, 2, result.InputCleaned)
		require.Equal(t, 1, result.OutputCleaned)
		require.Equal(t, BatchImageJobStatusRunning, repo.jobs["imgbatch_running"].Status)
		require.Nil(t, repo.jobs["imgbatch_future"].OutputDeletedAt)
		require.NotContains(t, strings.Join(repo.events["imgbatch_running"], ","), "cleanup")
	})
}

func TestBatchImageSettlementOutputExpiration(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_expire")
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{}
	svc := &BatchImageSettlementService{
		Repo:        repo,
		BillingRepo: billing,
		Pricing:     &fakeBatchImagePricingResolver{unitPrice: 0.25},
		Config:      &config.Config{BatchImage: config.BatchImageConfig{OutputRetentionAfterTerminalHours: 5}},
	}

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.NoError(t, err)
	require.NotNil(t, repo.jobs[job.BatchID].OutputExpiresAt)
	require.WithinDuration(t, time.Now().Add(5*time.Hour), *repo.jobs[job.BatchID].OutputExpiresAt, time.Minute)

	existing := time.Now().Add(time.Hour)
	second := testSettlingBatchImageJob("imgbatch_keep_expire")
	second.OutputExpiresAt = &existing
	repo.jobs[second.BatchID] = second
	_, err = svc.Settle(context.Background(), second.BatchID)
	require.NoError(t, err)
	require.Equal(t, existing, *repo.jobs[second.BatchID].OutputExpiresAt)
}

func TestBatchImageDownloadAfterOutputDeletedReturnsGone(t *testing.T) {
	svc, repo, _ := newTestBatchImageDownloadService()
	now := time.Now()
	repo.jobs["imgbatch_download"].Status = BatchImageJobStatusOutputDeleted
	repo.jobs["imgbatch_download"].OutputDeletedAt = &now

	stream, err := svc.OpenItemContent(context.Background(), testBatchImageOwner(), "imgbatch_download", "cover/../001", 0)
	require.Nil(t, stream)
	require.ErrorIs(t, err, ErrBatchImageOutputDeleted)

	var out strings.Builder
	result, err := svc.StreamZip(context.Background(), testBatchImageOwner(), "imgbatch_download", BatchImageZipOptions{}, &out)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrBatchImageOutputDeleted)
}

func newTestBatchImageCleanupService() (*BatchImageCleanupService, *fakeBatchImageRepository, *publicBatchImageProvider) {
	repo := newFakeBatchImageRepository()
	repo.jobs["imgbatch_cleanup"] = cleanupTestJob("imgbatch_cleanup", BatchImageJobStatusCompleted)
	provider := &publicBatchImageProvider{name: BatchImageProviderGeminiAPI}
	accountID := int64(101)
	svc := &BatchImageCleanupService{
		Repo:             repo,
		ProviderRegistry: NewBatchImageProviderRegistry(provider),
		AccountResolver:  &fakeBatchImageAccountResolver{account: &Account{ID: accountID, Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}},
		Config:           &config.Config{BatchImage: config.BatchImageConfig{CleanupBatchSize: 10, InputRetentionAfterTerminalHours: 24}},
	}
	return svc, repo, provider
}

func cleanupTestJob(batchID, status string) *BatchImageJob {
	apiKeyID := int64(22)
	accountID := int64(101)
	now := time.Now().Add(-48 * time.Hour)
	return &BatchImageJob{
		BatchID:           batchID,
		UserID:            11,
		APIKeyID:          &apiKeyID,
		AccountID:         &accountID,
		Provider:          BatchImageProviderGeminiAPI,
		Model:             "gemini-2.5-flash-image",
		Status:            status,
		ProviderJobName:   batchImageStringPtr("providers/internal/job"),
		ProviderInputRef:  batchImageStringPtr("files/internal/input"),
		ProviderOutputRef: batchImageStringPtr("files/internal/output"),
		ItemCount:         1,
		SuccessCount:      1,
		CreatedAt:         now,
		UpdatedAt:         now,
		FinishedAt:        &now,
		SettledAt:         &now,
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
