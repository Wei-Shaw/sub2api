//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type asyncVideoReconcilerTaskRepo struct {
	AsyncVideoTaskRepository
	listCalled           chan struct{}
	markSucceeded        bool
	markFailed           bool
	failedReason         string
	completeRefundErrors []error
	completeRefundCalls  int
	schedules            []asyncVideoRefundSchedule
	markRefundFailed     bool
}

type asyncVideoRefundSchedule struct {
	attempts int
	next     time.Time
	err      string
}

func (r *asyncVideoReconcilerTaskRepo) ListUnfinished(context.Context, int) ([]*AsyncVideoTask, error) {
	if r.listCalled != nil {
		select {
		case r.listCalled <- struct{}{}:
		default:
		}
	}
	return nil, nil
}

func (r *asyncVideoReconcilerTaskRepo) ListPendingRefunds(context.Context, int) ([]*AsyncVideoTask, error) {
	return nil, nil
}

func (r *asyncVideoReconcilerTaskRepo) MarkSucceeded(
	context.Context,
	int64,
	[]string,
	[]string,
	map[string]any,
	float64,
	int,
	float64,
) (bool, error) {
	r.markSucceeded = true
	return true, nil
}

func (r *asyncVideoReconcilerTaskRepo) MarkFailed(_ context.Context, _ int64, _ string, reason string, _ time.Time) (bool, error) {
	r.markFailed = true
	r.failedReason = reason
	return true, nil
}

func (r *asyncVideoReconcilerTaskRepo) CompleteRefund(context.Context, int64, string) (bool, int64, error) {
	index := r.completeRefundCalls
	r.completeRefundCalls++
	if index < len(r.completeRefundErrors) && r.completeRefundErrors[index] != nil {
		return false, 0, r.completeRefundErrors[index]
	}
	return true, 99, nil
}

func (r *asyncVideoReconcilerTaskRepo) ScheduleRefundRetry(_ context.Context, _ int64, attempts int, next time.Time, refundError string) (bool, error) {
	r.schedules = append(r.schedules, asyncVideoRefundSchedule{attempts: attempts, next: next, err: refundError})
	return true, nil
}

func (r *asyncVideoReconcilerTaskRepo) ClaimRefundRetry(context.Context, int64, int) (bool, error) {
	return true, nil
}

func (r *asyncVideoReconcilerTaskRepo) MarkRefundFailed(context.Context, int64, int, string) (bool, error) {
	r.markRefundFailed = true
	return true, nil
}

func (r *asyncVideoReconcilerTaskRepo) InsertTerminalUsageLog(context.Context, *VideoTerminalUsageLogInput) (bool, error) {
	return true, nil
}

func TestAsyncVideoReconcilerScansImmediatelyOnStart(t *testing.T) {
	repo := &asyncVideoReconcilerTaskRepo{listCalled: make(chan struct{}, 1)}
	exec := NewAsyncVideoService(repo, nil, nil)
	reconciler := NewAsyncVideoReconciler(repo, exec, nil)
	reconciler.SetInterval(time.Hour)
	reconciler.Start()
	defer reconciler.Stop()

	select {
	case <-repo.listCalled:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not scan unfinished tasks immediately after start")
	}
}

func TestAsyncVideoReconcileChecksUpstreamBeforeExpiredDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":200,"data":{"id":"prediction-1","status":"completed","outputs":["https://cdn.example.test/video.mp4"]}}`)
	}))
	defer server.Close()

	repo := &asyncVideoReconcilerTaskRepo{}
	exec := NewAsyncVideoService(repo, nil, nil)
	deadline := time.Now().Add(-time.Minute)
	statusURL := server.URL + "/api/v1/model/prediction/prediction-1"
	accountID := int64(57)
	task := &AsyncVideoTask{
		ID:                1,
		InternalRequestID: "request-1",
		AccountID:         &accountID,
		Status:            AsyncVideoStatusRunning,
		RequestedModel:    "bytedance/seedance-2.5/text-to-video",
		StatusURL:         &statusURL,
		FailDeadlineAt:    &deadline,
		HeldCost:          1,
		FinalCost:         1,
		RateMultiplier:    1,
	}
	account := &Account{
		ID:       accountID,
		Platform: PlatformAtlasCloud,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": server.URL,
		},
	}

	require.NoError(t, exec.ReconcileTask(context.Background(), task, account))
	require.True(t, repo.markSucceeded)
	require.Equal(t, AsyncVideoStatusSucceeded, task.Status)
}

func TestAsyncVideoReconcileStoresOnlyProviderErrorFromFailedStatus(t *testing.T) {
	const providerError = "The requested video duration is not supported by this model."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{
			"code":400,
			"data":{
				"id":"prediction-1",
				"status":"failed",
				"error":"`+providerError+`",
				"urls":{"get":"https://api.atlascloud.ai/api/v1/model/prediction/prediction-1"}
			}
		}`)
	}))
	defer server.Close()

	repo := &asyncVideoReconcilerTaskRepo{}
	exec := NewAsyncVideoService(repo, nil, nil)
	statusURL := server.URL + "/api/v1/model/prediction/prediction-1"
	accountID := int64(57)
	task := &AsyncVideoTask{
		ID:                2,
		InternalRequestID: "request-2",
		AccountID:         &accountID,
		Status:            AsyncVideoStatusRunning,
		RequestedModel:    "bytedance/seedance-2.5/image-to-video",
		StatusURL:         &statusURL,
		HeldCost:          1,
		BillingType:       BillingTypeBalance,
	}
	account := &Account{
		ID:       accountID,
		Platform: PlatformAtlasCloud,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": server.URL,
		},
	}

	require.NoError(t, exec.ReconcileTask(context.Background(), task, account))
	require.True(t, repo.markFailed)
	require.Equal(t, "status 400: "+providerError, repo.failedReason)
	require.NotContains(t, strings.ToLower(repo.failedReason), "atlascloud")
	require.NotContains(t, repo.failedReason, "http")
	require.NotContains(t, repo.failedReason, "prediction-1")
}

func TestAsyncVideoFailurePersistsBeforeRefundAndRetriesAtConfiguredDelays(t *testing.T) {
	refundErr := errors.New("database temporarily unavailable")
	repo := &asyncVideoReconcilerTaskRepo{
		completeRefundErrors: []error{refundErr, refundErr, refundErr, refundErr},
	}
	exec := NewAsyncVideoService(repo, nil, nil)
	task := &AsyncVideoTask{
		ID:          49,
		Status:      AsyncVideoStatusRunning,
		HeldCost:    1.25,
		BillingType: BillingTypeBalance,
	}
	startedAt := time.Now()

	exec.markFailedAndRefund(context.Background(), task, BillingTypeBalance, strings.Repeat("错", 600))

	require.True(t, repo.markFailed, "generation failure must be persisted before refund")
	require.Len(t, []rune(repo.failedReason), maxAsyncVideoErrorReasonRunes)
	require.True(t, strings.HasSuffix(repo.failedReason, "..."))
	require.Equal(t, AsyncVideoStatusFailed, task.Status)
	require.Equal(t, AsyncVideoRefundStatusPending, task.RefundStatus)
	require.Len(t, repo.schedules, 1)
	require.Zero(t, repo.schedules[0].attempts)
	require.WithinDuration(t, startedAt.Add(time.Minute), repo.schedules[0].next, 2*time.Second)

	for attempt := 1; attempt <= len(asyncVideoRefundRetryDelays); attempt++ {
		require.NoError(t, exec.RetryPendingRefund(context.Background(), task))
		if attempt < len(asyncVideoRefundRetryDelays) {
			require.Equal(t, AsyncVideoRefundStatusPending, task.RefundStatus)
		}
	}

	require.Len(t, repo.schedules, 3)
	require.Equal(t, 1, repo.schedules[1].attempts)
	require.WithinDuration(t, time.Now().Add(3*time.Minute), repo.schedules[1].next, 2*time.Second)
	require.Equal(t, 2, repo.schedules[2].attempts)
	require.WithinDuration(t, time.Now().Add(9*time.Minute), repo.schedules[2].next, 2*time.Second)
	require.True(t, repo.markRefundFailed)
	require.Equal(t, AsyncVideoStatusRefundFailed, task.Status)
	require.Equal(t, AsyncVideoRefundStatusFailed, task.RefundStatus)
	require.Equal(t, len(asyncVideoRefundRetryDelays), task.RefundAttempts)
}
