//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingBatchImageQueue struct {
	*fakeBatchImageQueue
	enqueued []string
REDACTED

func (q *recordingBatchImageQueue) Enqueue(_ context.Context, batchID string) error {
	q.enqueued = append(q.enqueued, batchID)
	return nil
REDACTED

func TestBatchImageBillingRecoveryService_ReleasesStaleUnsubmittedHold(t *testing.T) {
	repo := newFakeBatchImageRepository()
	apiKeyID := int64(22)
	holdAmount := 0.5
	stale := &BatchImageJob{
		BatchID:       "imgbatch_stale_created",
		UserID:        11,
		APIKeyID:      &apiKeyID,
		Status:        BatchImageJobStatusCreated,
		EstimatedCost: holdAmount,
		HoldAmount:    &holdAmount,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
REDACTED
	activeProviderName := "providers/job"
	active := &BatchImageJob{
		BatchID:         "imgbatch_has_provider",
		UserID:          11,
		APIKeyID:        &apiKeyID,
		Status:          BatchImageJobStatusSubmitted,
		ProviderJobName: &activeProviderName,
		EstimatedCost:   holdAmount,
		HoldAmount:      &holdAmount,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Hour),
REDACTED
	repo.jobs[stale.BatchID] = stale
	repo.jobs[active.BatchID] = active
	billing := &fakeBatchImageBillingRepo{REDACTED
	svc := &BatchImageBillingRecoveryService{Repo: repo, Billing: billing, StaleAfter: time.Minute, Limit: 10REDACTED

	released, err := svc.ReleaseStaleUnsubmittedOnce(context.Background())
REDACTED
	require.Equal(t, 1, released)
	require.Equal(t, BatchImageJobStatusFailed, repo.jobs[stale.BatchID].Status)
	require.Equal(t, "SUBMIT_STALE_BEFORE_PROVIDER", batchImageDerefString(repo.jobs[stale.BatchID].LastErrorCode))
	require.Len(t, billing.releases, 1)
	require.Equal(t, BatchImageReleaseRequestID(stale.BatchID), billing.releases[0].RequestID)
	require.Equal(t, BatchImageJobStatusSubmitted, repo.jobs[active.BatchID].Status)
REDACTED

func TestBatchImageBillingRecoveryService_SkipsJobRefreshedByHeartbeat(t *testing.T) {
	repo := newFakeBatchImageRepository()
	apiKeyID := int64(22)
	holdAmount := 0.5
	// updated_at 在 cutoff 之后（慢提交心跳持续续期）：不得误杀退款。
	fresh := &BatchImageJob{
		BatchID:       "imgbatch_fresh_uploading",
		UserID:        11,
		APIKeyID:      &apiKeyID,
		Status:        BatchImageJobStatusUploading,
		EstimatedCost: holdAmount,
		HoldAmount:    &holdAmount,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
REDACTED
	repo.jobs[fresh.BatchID] = fresh
	billing := &fakeBatchImageBillingRepo{REDACTED
	svc := &BatchImageBillingRecoveryService{Repo: repo, Billing: billing, StaleAfter: time.Minute, Limit: 10REDACTED

	released, err := svc.ReleaseStaleUnsubmittedOnce(context.Background())
REDACTED
	require.Equal(t, 0, released)
	require.Equal(t, BatchImageJobStatusUploading, repo.jobs[fresh.BatchID].Status)
	require.Empty(t, billing.releases)
REDACTED

func TestBatchImageBillingRecoveryService_EnqueuesRetryWhenReleaseFails(t *testing.T) {
	repo := newFakeBatchImageRepository()
	apiKeyID := int64(22)
	holdAmount := 0.5
	stale := &BatchImageJob{
		BatchID:       "imgbatch_stale_release_fail",
		UserID:        11,
		APIKeyID:      &apiKeyID,
		Status:        BatchImageJobStatusCreated,
		EstimatedCost: holdAmount,
		HoldAmount:    &holdAmount,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
REDACTED
	repo.jobs[stale.BatchID] = stale
	billing := &fakeBatchImageBillingRepo{releaseErr: errors.New("billing db down")REDACTED
	queue := &recordingBatchImageQueue{fakeBatchImageQueue: newFakeBatchImageQueue("")REDACTED
	svc := &BatchImageBillingRecoveryService{Repo: repo, Billing: billing, Queue: queue, StaleAfter: time.Minute, Limit: 10REDACTED

	released, err := svc.ReleaseStaleUnsubmittedOnce(context.Background())
	// job 已转 failed、不会再出现在 stale 列表：释放失败必须入队重试
	//（由 worker 的 releaseTerminalHold 兜底），否则冻结余额永久泄漏。
REDACTED
	require.Equal(t, 0, released)
	require.Equal(t, BatchImageJobStatusFailed, repo.jobs[stale.BatchID].Status)
	require.Equal(t, []string{stale.BatchIDREDACTED, queue.enqueued)
REDACTED
