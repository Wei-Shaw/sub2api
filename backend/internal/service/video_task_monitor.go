package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// VideoTaskMonitorRepository only updates tracking records. It has no billing,
// balance, submission or task-creation operation. Usage is matched by the full
// task/user/key/account identity, never by model, time or amount.
type VideoTaskMonitorRepository interface {
	ClaimDueVideoTaskMonitor(context.Context, time.Duration) (*VideoTask, error)
	SyncVideoTaskMonitorUsage(context.Context, *VideoTask) (bool, error)
	FinishVideoTaskMonitor(context.Context, *VideoTask, string, string) error
}

type videoTaskMonitor struct {
	repo VideoTaskMonitorRepository
	poll func(context.Context, *VideoTask) (string, string)
}

// A slow monitoring GET must not delay the existing settlement worker.
func (m *videoTaskMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		for i := 0; i < 8 && ctx.Err() == nil; i++ {
			claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			task, err := m.repo.ClaimDueVideoTaskMonitor(claimCtx, videoTaskLeaseDuration)
			cancel()
			if err == nil && task != nil {
				err = m.process(ctx, task)
			}
			if err != nil && ctx.Err() == nil {
				logger.LegacyPrintf("service.video_task", "video progress monitor deferred")
			}
			if err != nil || task == nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *videoTaskMonitor) process(parent context.Context, t *VideoTask) error {
	if t == nil || !t.Pending.TrackingOnly || t.State != VideoTaskPending || strings.HasPrefix(t.TaskID, "submission:") {
		return errors.New("video monitor requires an accepted tracking-only task")
	}
	ctx, cancel := context.WithTimeout(parent, videoTaskAttemptTimeout)
	defer cancel()
	// A usage record already proves completion on the existing billing path.
	// Reading it cannot charge again, even if upstream retention has expired.
	done, err := m.repo.SyncVideoTaskMonitorUsage(ctx, t)
	if err != nil || done {
		return err
	}
	state, code := m.poll(ctx, t)
	if parent.Err() == nil && state == VideoTaskPending && code != "account_route_changed" && videoTaskPollingExhausted(t, time.Now()) {
		code = "poll_exhausted"
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	return m.repo.FinishVideoTaskMonitor(saveCtx, t, state, code)
}

// Stop automatic status checks after at least ten attempts and a full day.
// Retention loss, missing tasks and transport errors are NOT generation failure.
// Ready/billed tasks keep their existing settlement semantics.
func videoTaskPollingExhausted(t *VideoTask, now time.Time) bool {
	if t == nil || t.State != VideoTaskPending || t.Completion != nil || t.Attempts < 10 {
		return false
	}
	created, err := time.Parse(time.RFC3339Nano, t.Pending.CreatedAt)
	return err == nil && now.Sub(created) >= 24*time.Hour
}

func videoTaskMonitorRoutingFingerprint(account *Account) string {
	base := account.GetOpenAIBaseURL()
	if account.Platform == PlatformGrok {
		base = account.GetGrokMediaBaseURL()
	}
	protocol, _ := account.Extra["openai_video_protocol"].(string)
	body, _ := json.Marshal([]any{account.Platform, account.Type, base, protocol, account.ParentAccountID})
	return fmt.Sprintf("%x", sha256.Sum256(body))
}

// Poll the pinned upstream's GET endpoint directly. Do not invoke a public
// gateway handler (which can settle usage). Successful results may enqueue an
// independent read-only media download, never billing or generation.
func (s *OpenAIGatewayService) pollVideoTaskMonitor(ctx context.Context, t *VideoTask) (string, string) {
	if validateVideoTaskID(t.TaskID) != nil || s.accountRepo == nil {
		return VideoTaskPending, "poll_failed"
	}
	account, err := s.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil || account == nil {
		return VideoTaskPending, "poll_failed"
	}
	if account.ID != t.AccountID || account.Platform != t.Pending.Platform ||
		t.RoutingFingerprint != videoTaskMonitorRoutingFingerprint(account) {
		return VideoTaskPending, "account_route_changed"
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(videoPreviewCacheOwnerKey, t.APIKeyID)
	c.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/video-task-monitor", nil)
	var result *OpenAIForwardResult
	if strings.HasPrefix(t.TaskID, "seedance:") {
		result, err = s.ForwardSeedance(ctx, c, account, SeedanceEndpointStatus, t.TaskID, nil)
	} else {
		result, err = s.ForwardGrokMedia(ctx, c, account, GrokMediaEndpointVideoStatus, t.TaskID, nil, "")
	}
	if ctx.Err() != nil {
		return VideoTaskPending, "poll_timeout"
	}
	if recorder.Code == http.StatusNotFound || recorder.Code == http.StatusGone {
		return VideoTaskPending, "poll_not_found"
	}
	if err != nil || recorder.Code < 200 || recorder.Code >= 300 || !gjson.ValidBytes(recorder.Body.Bytes()) {
		return VideoTaskPending, "poll_failed"
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(recorder.Body.Bytes(), "status").String()))
	switch status {
	case "failed":
		return VideoTaskFailed, "upstream_failed"
	case "cancelled", "canceled":
		return VideoTaskFailed, "upstream_cancelled"
	case "expired":
		return VideoTaskPending, "upstream_expired"
	}
	if (result != nil && result.VideoCount > 0) || (strings.HasPrefix(t.TaskID, "seedance:") && status == "succeeded") {
		return VideoTaskCompleted, ""
	}
	switch status {
	case "queued", "pending", "in_progress", "processing", "running":
		return VideoTaskPending, ""
	default:
		return VideoTaskPending, "poll_failed"
	}
}
