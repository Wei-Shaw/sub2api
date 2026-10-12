package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAIVideoSubmissionStarter interface {
	BeginVideoSubmission(context.Context, int64, int64, string, string, []byte) (string, error)
}

func (h *OpenAIGatewayHandler) beginOpenAIVideoSubmission(c *gin.Context, key *service.APIKey, contentType string, body []byte) bool {
	return beginOpenAIVideoSubmission(c, h.gatewayService, key, contentType, body)
}

func beginOpenAIVideoSubmission(c *gin.Context, starter openAIVideoSubmissionStarter, key *service.APIKey, contentType string, body []byte) bool {
	var groupID int64
	if key.GroupID != nil {
		groupID = *key.GroupID
	}
	taskID, err := starter.BeginVideoSubmission(c.Request.Context(), key.ID, groupID, c.GetHeader("Idempotency-Key"), contentType, body)
	if err != nil {
		status, code, message := http.StatusServiceUnavailable, "video_submission_unavailable", "Video submission tracking is temporarily unavailable"
		if errors.Is(err, service.ErrVideoSubmissionConflict) {
			status, code, message = http.StatusConflict, "video_submission_conflict", "Video idempotency key was used with another request"
		} else if errors.Is(err, service.ErrVideoSubmissionPending) {
			status, code, message = http.StatusConflict, "video_submission_pending", "Video submission is in progress or its outcome is unknown; do not resubmit with a new key"
		}
		service.MarkResponseCommitted(c)
		c.JSON(status, gin.H{"error": gin.H{"type": "api_error", "code": code, "message": message}})
		return false
	}
	if taskID != "" {
		service.MarkResponseCommitted(c)
		if key.Group != nil && key.Group.Platform == service.PlatformGrok {
			c.JSON(http.StatusAccepted, gin.H{"request_id": taskID, "status": "pending"})
			return false
		}
		c.JSON(http.StatusAccepted, gin.H{"id": taskID, "object": "video", "status": "queued"})
		return false
	}
	return true
}

type openAIVideoTaskStore interface {
	StoreGrokVideoPendingBilling(context.Context, string, int64, int64, service.GrokVideoPendingBilling) error
	BindGrokMediaVideoRequestAccount(context.Context, *int64, string, int64, int64, int64) error
}

type durableOpenAIVideoLookup interface {
	ReconcileOwnedVideoTask(context.Context, string, int64, int64) error
	LoadDurableVideoTask(context.Context, string, int64, int64) (*service.VideoTask, error)
}

// Only a reloaded, durably completed task may reach the forwarding code.
// Reconcile can observe upstream completion yet fail settlement, or race a
// worker holding the lease; neither permits publishing an upstream URL/body.
func allowDurableOpenAIVideoForward(c *gin.Context, lookup durableOpenAIVideoLookup, task *service.VideoTask, account *service.Account, endpoint service.GrokMediaEndpoint) bool {
	fail := func(status int, code, message string) bool {
		service.MarkResponseCommitted(c)
		c.Header("Cache-Control", "private, no-store")
		c.JSON(status, gin.H{"error": gin.H{"type": "api_error", "code": code, "message": message}})
		return false
	}
	if err := service.ValidateDurableVideoTaskAccount(task, account); err != nil {
		return fail(http.StatusServiceUnavailable, "video_account_unavailable", "Video task account route is unavailable")
	}
	reconcileErr := lookup.ReconcileOwnedVideoTask(c.Request.Context(), task.TaskID, task.UserID, task.APIKeyID)
	latest, err := lookup.LoadDurableVideoTask(c.Request.Context(), task.TaskID, task.UserID, task.APIKeyID)
	if err != nil || latest == nil {
		return fail(http.StatusServiceUnavailable, "video_task_unavailable", "Video task tracking is temporarily unavailable")
	}
	if latest.TaskID != task.TaskID || latest.UserID != task.UserID || latest.APIKeyID != task.APIKeyID || service.ValidateDurableVideoTaskAccount(latest, account) != nil {
		return fail(http.StatusServiceUnavailable, "video_account_unavailable", "Video task account route is unavailable")
	}
	if latest.State == service.VideoTaskCompleted {
		return true
	}
	if latest.State == service.VideoTaskFailed {
		// An older worker stored no reason, including for ambiguous expiry.
		// Do not invent provider failure evidence for those historical rows.
		code, message := "video_failed", "Video task was marked failed"
		if latest.LastError == "upstream_failed" {
			code, message = "upstream_failed", "Video generation failed"
		}
		if latest.LastError == "upstream_cancelled" {
			code, message = "upstream_cancelled", "Video generation was cancelled by the provider"
		}
		if endpoint == service.GrokMediaEndpointVideoContent {
			return fail(http.StatusConflict, "video_failed", message)
		}
		service.MarkResponseCommitted(c)
		c.Header("Cache-Control", "private, no-store")
		c.JSON(http.StatusOK, gin.H{"id": latest.TaskID, "object": "video", "status": "failed", "error": gin.H{"code": code, "message": message}})
		return false
	}
	if latest.State != service.VideoTaskPending && latest.State != service.VideoTaskReady {
		return fail(http.StatusServiceUnavailable, "video_task_unavailable", "Video task state is unavailable")
	}
	// A ready task may already have a committed charge while its final state
	// update is being retried. Do not infer payment status from a lookup error.
	retryAfter := 15
	if remaining := time.Until(latest.NextAttemptAt); remaining > 0 {
		retryAfter = min(240, max(1, int((remaining+time.Second-1)/time.Second)))
	}
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	if errors.Is(reconcileErr, service.ErrVideoTaskRepositoryUnavailable) {
		return fail(http.StatusServiceUnavailable, "video_task_unavailable", "Video task tracking is temporarily unavailable")
	}
	progress := func(detail, message, contentCode string, contentStatus int) bool {
		if endpoint == service.GrokMediaEndpointVideoContent {
			return fail(contentStatus, contentCode, message)
		}
		// A recoverable lookup or settlement delay is not a generation failure.
		// Keep status pollers on the original task while the content gate stays closed.
		service.MarkResponseCommitted(c)
		c.Header("Cache-Control", "private, no-store")
		if account.Platform == service.PlatformGrok {
			c.JSON(http.StatusOK, gin.H{"request_id": latest.TaskID, "status": "pending", "status_detail": detail, "message": message})
		} else {
			c.JSON(http.StatusOK, gin.H{"id": latest.TaskID, "object": "video", "status": "in_progress", "status_detail": detail, "message": message})
		}
		return false
	}
	if latest.State == service.VideoTaskReady {
		return progress("settlement_pending", "Video generation completed; settlement confirmation is pending. Retry this lookup later", "video_settlement_pending", http.StatusServiceUnavailable)
	}
	// Only known codes influence public reporting; never copy stored or upstream
	// error text into a response. Retain the diagnostic while a retry is backoff-bound.
	switch latest.LastError {
	case "poll_exhausted":
		return progress("outcome_unknown", "Automatic checks are paused after repeated inconclusive lookups. This task needs upstream review; an explicit lookup can check again after four minutes", "video_outcome_unknown", http.StatusServiceUnavailable)
	case "poll_timeout":
		return progress("poll_timeout", "Video status lookup timed out; generation outcome is not yet confirmed. Retry this lookup later", "video_poll_timeout", http.StatusServiceUnavailable)
	case "poll_failed":
		return progress("poll_unavailable", "Video status is temporarily unavailable; generation outcome is not yet confirmed. Retry this lookup later", "video_poll_unavailable", http.StatusServiceUnavailable)
	case "upstream_expired":
		return progress("outcome_unknown", "The provider reports expiry; generation outcome is unknown. Retry this lookup later", "video_outcome_unknown", http.StatusServiceUnavailable)
	}
	if reconcileErr != nil && !errors.Is(reconcileErr, service.ErrVideoTaskBusy) {
		return fail(http.StatusServiceUnavailable, "video_task_unavailable", "Video task confirmation is temporarily unavailable; retry this lookup later")
	}
	if errors.Is(reconcileErr, service.ErrVideoTaskBusy) {
		return progress("lookup_busy", "Video status lookup is busy or awaiting retry; retry this lookup later", "video_task_busy", http.StatusConflict)
	}
	return progress("generation_in_progress", "Video generation is in progress; retry this lookup later", "video_not_ready", http.StatusConflict)
}

func persistOpenAIVideoTask(
	ctx context.Context, store openAIVideoTaskStore, groupID *int64,
	taskID string, userID, apiKeyID, accountID int64, pending service.GrokVideoPendingBilling,
) error {
	// Publish the owner binding last, so a readable task always has its pricing snapshot.
	if err := store.StoreGrokVideoPendingBilling(ctx, taskID, userID, apiKeyID, pending); err != nil {
		if err = store.StoreGrokVideoPendingBilling(ctx, taskID, userID, apiKeyID, pending); err != nil {
			return fmt.Errorf("save video billing snapshot: %w", err)
		}
	}
	if err := store.BindGrokMediaVideoRequestAccount(ctx, groupID, taskID, userID, apiKeyID, accountID); err != nil {
		return fmt.Errorf("save video owner binding: %w", err)
	}
	return nil
}
