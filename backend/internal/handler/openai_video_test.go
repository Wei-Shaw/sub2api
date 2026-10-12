package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIVideoSubmissionStub struct {
	taskID string
	err    error
	calls  int
}

func (s *openAIVideoSubmissionStub) BeginVideoSubmission(_ context.Context, keyID, groupID int64, key, contentType string, body []byte) (string, error) {
	s.calls++
	return s.taskID, s.err
}

func TestOpenAIVideoSubmissionGuard(t *testing.T) {
	for _, tc := range []struct {
		name, taskID string
		err          error
		status       int
		proceed      bool
	}{
		{"new", "", nil, 200, true},
		{"replay", "existing-1", nil, 202, false},
		{"conflict", "", service.ErrVideoSubmissionConflict, 409, false},
		{"pending", "", service.ErrVideoSubmissionPending, 409, false},
		{"unavailable", "", errors.New("store unavailable"), 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			c.Request.Header.Set("Idempotency-Key", "request-1")
			starter := &openAIVideoSubmissionStub{taskID: tc.taskID, err: tc.err}
			groupID := int64(112)
			proceed := beginOpenAIVideoSubmission(c, starter, &service.APIKey{ID: 7, GroupID: &groupID}, "application/json", []byte(`{"model":"video"}`))
			require.Equal(t, tc.proceed, proceed)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, 1, starter.calls)
			if tc.taskID != "" {
				require.JSONEq(t, `{"id":"existing-1","object":"video","status":"queued"}`, w.Body.String())
			}
			if tc.proceed {
				require.False(t, c.Writer.Written())
			}
		})
	}
}

type openAIVideoTaskStoreStub struct {
	pendingErrors int
	bindingError  error
	pendingCalls  int
	bindingCalls  int
	pending       service.GrokVideoPendingBilling
}

func (s *openAIVideoTaskStoreStub) StoreGrokVideoPendingBilling(_ context.Context, _ string, _, _ int64, pending service.GrokVideoPendingBilling) error {
	s.pendingCalls++
	if s.pendingCalls <= s.pendingErrors {
		return errors.New("pending unavailable")
	}
	s.pending = pending
	return nil
}

func (s *openAIVideoTaskStoreStub) BindGrokMediaVideoRequestAccount(_ context.Context, _ *int64, _ string, _, _, _ int64) error {
	s.bindingCalls++
	return s.bindingError
}

func TestOpenAIVideoTaskPersistenceFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name          string
		pendingErrors int
		bindingError  error
		wantError     bool
		wantBindings  int
	}{
		{"success", 0, nil, false, 1},
		{"pending retry", 1, nil, false, 1},
		{"pending unavailable", 2, nil, true, 0},
		{"binding unavailable", 0, errors.New("binding unavailable"), true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &openAIVideoTaskStoreStub{pendingErrors: tc.pendingErrors, bindingError: tc.bindingError}
			pending := service.GrokVideoPendingBilling{Model: "client-video", VideoDurationSeconds: 6}
			err := persistOpenAIVideoTask(context.Background(), store, nil, "task-1", 1, 2, 3, pending)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, pending, store.pending)
			}
			require.Equal(t, tc.wantBindings, store.bindingCalls)
			require.LessOrEqual(t, store.pendingCalls, 2)
		})
	}
}

type durableOpenAIVideoLookupStub struct {
	test         *testing.T
	owner        *service.VideoTask
	latest       *service.VideoTask
	reconcileErr error
	loadErr      error
	calls        int
}

func (s *durableOpenAIVideoLookupStub) ReconcileOwnedVideoTask(_ context.Context, id string, user, key int64) error {
	s.calls++
	require.Equal(s.test, s.owner.TaskID, id)
	require.Equal(s.test, s.owner.UserID, user)
	require.Equal(s.test, s.owner.APIKeyID, key)
	return s.reconcileErr
}

func (s *durableOpenAIVideoLookupStub) LoadDurableVideoTask(_ context.Context, id string, user, key int64) (*service.VideoTask, error) {
	require.Equal(s.test, s.owner.TaskID, id)
	require.Equal(s.test, s.owner.UserID, user)
	require.Equal(s.test, s.owner.APIKeyID, key)
	return s.latest, s.loadErr
}

func durableOpenAIVideoHandlerFixture(t *testing.T) (*service.VideoTask, *service.Account) {
	t.Helper()
	account := &service.Account{ID: 313, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example", "api_key": "private-token"}, Extra: map[string]any{}}
	body, err := json.Marshal([]any{account.Platform, account.Type, account.GetOpenAIBaseURL(), "", account.ParentAccountID})
	require.NoError(t, err)
	task := &service.VideoTask{TaskID: "video-1", UserID: 1, APIKeyID: 2, AccountID: account.ID, State: service.VideoTaskPending, RoutingFingerprint: fmt.Sprintf("%x", sha256.Sum256(body))}
	require.NoError(t, service.ValidateDurableVideoTaskAccount(task, account))
	return task, account
}

func TestDurableVideoPublicStatusAndBillingGuard(t *testing.T) {
	for _, tc := range []struct {
		name, state, lastError, code, detail string
		err                                  error
		status                               int
	}{
		{name: "generating", state: service.VideoTaskPending, detail: "generation_in_progress", status: 200},
		{name: "busy", state: service.VideoTaskPending, err: service.ErrVideoTaskBusy, detail: "lookup_busy", status: 200},
		{name: "poll timeout", state: service.VideoTaskPending, lastError: "poll_timeout", err: context.DeadlineExceeded, detail: "poll_timeout", code: "video_poll_timeout", status: 200},
		{name: "timeout backoff", state: service.VideoTaskPending, lastError: "poll_timeout", err: service.ErrVideoTaskBusy, detail: "poll_timeout", code: "video_poll_timeout", status: 200},
		{name: "temporary lookup failure", state: service.VideoTaskPending, lastError: "poll_failed", err: errors.New("private prompt https://secret.example"), detail: "poll_unavailable", code: "video_poll_unavailable", status: 200},
		{name: "expired backoff", state: service.VideoTaskPending, lastError: "upstream_expired", err: service.ErrVideoTaskBusy, detail: "outcome_unknown", code: "video_outcome_unknown", status: 200},
		{name: "paused automatic checks", state: service.VideoTaskPending, lastError: "poll_exhausted", err: service.ErrVideoTaskBusy, detail: "outcome_unknown", code: "video_outcome_unknown", status: 200},
		{name: "ready", state: service.VideoTaskReady, detail: "settlement_pending", code: "video_settlement_pending", status: 200},
		{name: "ready busy", state: service.VideoTaskReady, err: service.ErrVideoTaskBusy, detail: "settlement_pending", code: "video_settlement_pending", status: 200},
		{name: "billing error", state: service.VideoTaskReady, lastError: "billing_failed", err: errors.New("private billing data"), detail: "settlement_pending", code: "video_settlement_pending", status: 200},
		{name: "repository unavailable after timeout", state: service.VideoTaskPending, lastError: "poll_timeout", err: service.ErrVideoTaskRepositoryUnavailable, code: "video_task_unavailable", status: 503},
		{name: "repository unavailable ready", state: service.VideoTaskReady, err: service.ErrVideoTaskRepositoryUnavailable, code: "video_task_unavailable", status: 503},
		{name: "untrusted stored error", state: service.VideoTaskPending, lastError: "private prompt https://secret.example", err: errors.New("private token"), code: "video_task_unavailable", status: 503},
		{name: "failed", state: service.VideoTaskFailed, lastError: "upstream_failed", code: "upstream_failed", status: 200},
		{name: "cancelled", state: service.VideoTaskFailed, lastError: "upstream_cancelled", code: "upstream_cancelled", status: 200},
		{name: "old worker failed without evidence", state: service.VideoTaskFailed, code: "video_failed", status: 200},
	} {
		for _, endpoint := range []service.GrokMediaEndpoint{service.GrokMediaEndpointVideoStatus, service.GrokMediaEndpointVideoContent} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, endpoint), func(t *testing.T) {
				task, account := durableOpenAIVideoHandlerFixture(t)
				latest := *task
				latest.State, latest.LastError = tc.state, tc.lastError
				latest.NextAttemptAt = time.Now().Add(time.Hour) // public hint is capped
				if tc.state == service.VideoTaskReady {
					latest.Completion = &service.VideoTaskCompletion{}
				}
				lookup := &durableOpenAIVideoLookupStub{test: t, owner: task, latest: &latest, reconcileErr: tc.err}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video-1", nil)
				require.False(t, allowDurableOpenAIVideoForward(c, lookup, task, account, endpoint))
				wantStatus, wantCode := tc.status, tc.code
				if endpoint == service.GrokMediaEndpointVideoContent && tc.status == 200 {
					wantStatus = 503
					switch {
					case tc.state == service.VideoTaskFailed:
						wantStatus, wantCode = 409, "video_failed"
					case tc.detail == "lookup_busy":
						wantStatus, wantCode = 409, "video_task_busy"
					case tc.detail == "generation_in_progress":
						wantStatus, wantCode = 409, "video_not_ready"
					}
				}
				if endpoint == service.GrokMediaEndpointVideoStatus && tc.detail != "" {
					wantCode = ""
				}
				require.Equal(t, wantStatus, w.Code)
				body := w.Body.String()
				require.Equal(t, wantCode, gjson.Get(body, "error.code").String())
				if endpoint == service.GrokMediaEndpointVideoStatus && tc.detail != "" {
					require.False(t, gjson.Get(body, "error").Exists())
					require.Equal(t, "in_progress", gjson.Get(body, "status").String())
					require.Equal(t, tc.detail, gjson.Get(body, "status_detail").String())
				}
				if tc.state != service.VideoTaskFailed {
					require.Equal(t, "240", w.Header().Get("Retry-After"))
				} else {
					require.Empty(t, w.Header().Get("Retry-After"))
				}
				require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
				require.True(t, service.IsResponseCommitted(c))
				for _, secret := range []string{"private", "https://", "prompt", "token", "not charged", "refunded"} {
					require.NotContains(t, body, secret)
				}
			})
		}
	}
}

func TestDurableVideoCompletedForwardRequiresReloadedOwnerAndRoute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*service.VideoTask)
	}{
		{"completed", func(*service.VideoTask) {}},
		{"wrong task", func(task *service.VideoTask) { task.TaskID = "other-task" }},
		{"wrong owner", func(task *service.VideoTask) { task.UserID++ }},
		{"wrong key", func(task *service.VideoTask) { task.APIKeyID++ }},
		{"wrong account", func(task *service.VideoTask) { task.AccountID++ }},
		{"changed route", func(task *service.VideoTask) { task.RoutingFingerprint = "other-route" }},
		{"still ready", func(task *service.VideoTask) { task.State = service.VideoTaskReady }},
	} {
		for _, endpoint := range []service.GrokMediaEndpoint{service.GrokMediaEndpointVideoStatus, service.GrokMediaEndpointVideoContent} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, endpoint), func(t *testing.T) {
				task, account := durableOpenAIVideoHandlerFixture(t)
				latest := *task
				latest.State, latest.Completion = service.VideoTaskCompleted, &service.VideoTaskCompletion{}
				tc.mutate(&latest)
				// Reconciliation may return an error even though another worker has
				// committed settlement. The authoritative reload decides the gate.
				lookup := &durableOpenAIVideoLookupStub{test: t, owner: task, latest: &latest, reconcileErr: errors.New("private settlement error")}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video-1", nil)
				allowed := allowDurableOpenAIVideoForward(c, lookup, task, account, endpoint)
				require.Equal(t, tc.name == "completed", allowed)
				if allowed {
					require.False(t, c.Writer.Written())
					require.Empty(t, w.Header().Get("Retry-After"))
				} else if tc.name == "still ready" && endpoint == service.GrokMediaEndpointVideoStatus {
					require.Equal(t, 200, w.Code)
					require.Equal(t, "settlement_pending", gjson.Get(w.Body.String(), "status_detail").String())
					require.Equal(t, "15", w.Header().Get("Retry-After"))
				} else {
					require.Equal(t, 503, w.Code)
					require.NotContains(t, w.Body.String(), "private")
				}
			})
		}
	}
}

func TestOpenAIVideoSubmissionErrorsAreSafeAndDoNotRequestPOSTRetry(t *testing.T) {
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{service.ErrVideoSubmissionPending, "video_submission_pending"},
		{service.ErrVideoSubmissionConflict, "video_submission_conflict"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		starter := &openAIVideoSubmissionStub{err: fmt.Errorf("private prompt https://secret.example: %w", tc.cause)}
		require.False(t, beginOpenAIVideoSubmission(c, starter, &service.APIKey{ID: 7}, "application/json", nil))
		require.Equal(t, 409, w.Code)
		require.Equal(t, tc.code, gjson.Get(w.Body.String(), "error.code").String())
		require.NotContains(t, w.Body.String(), "private")
		require.NotContains(t, w.Body.String(), "https://")
		require.Empty(t, w.Header().Get("Retry-After"))
		require.Equal(t, 1, starter.calls)
	}
}

func TestDurableVideoLookupUnavailableFailsClosed(t *testing.T) {
	for _, name := range []string{"initial route mismatch", "missing reload", "reload error"} {
		t.Run(name, func(t *testing.T) {
			task, account := durableOpenAIVideoHandlerFixture(t)
			task.State = service.VideoTaskCompleted
			lookup := &durableOpenAIVideoLookupStub{test: t, owner: task}
			if name == "initial route mismatch" {
				task.RoutingFingerprint = "wrong-route"
			} else if name == "reload error" {
				lookup.latest = task
				lookup.loadErr = errors.New("private connection data")
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video-1/content", nil)
			require.False(t, allowDurableOpenAIVideoForward(c, lookup, task, account, service.GrokMediaEndpointVideoContent))
			require.Equal(t, 503, w.Code)
			require.NotContains(t, w.Body.String(), "private")
			if name == "initial route mismatch" {
				require.Zero(t, lookup.calls, "reject a changed route before reconciliation can bill")
			}
		})
	}
}
