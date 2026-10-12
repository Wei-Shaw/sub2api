//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Existing transport/slot fixtures now provide the required recording store.
type videoRecordRepoStub struct{ service.UsageBillingRepository }

func (*videoRecordRepoStub) BeginVideoTaskRecord(context.Context, *service.VideoTask) error {
	return nil
}
func (*videoRecordRepoStub) AcceptVideoTaskRecord(context.Context, *service.VideoTask, string, string) error {
	return nil
}
func (*videoRecordRepoStub) ObserveVideoTaskRecord(context.Context, string, int64, int64, int64, string) error {
	return nil
}

type recordingProbe struct {
	beginErr, acceptErr, observeErr error
	begun, accepted                 bool
	observed                        string
	beforeAccept                    func()
}

func (p *recordingProbe) BeginVideoTaskRecord(_ context.Context, key *service.APIKey, account *service.Account, _ *service.UserSubscription, pending service.GrokVideoPendingBilling) (*service.VideoTask, error) {
	p.begun = true
	pending.TrackingOnly = true
	return &service.VideoTask{TaskID: "submission:test", UserID: key.UserID, APIKeyID: key.ID, AccountID: account.ID, Pending: pending}, p.beginErr
}
func (p *recordingProbe) AcceptVideoTaskRecord(ctx context.Context, _ *service.VideoTask, _ string, _ string) error {
	if p.beforeAccept != nil {
		p.beforeAccept()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.accepted = true
	return p.acceptErr
}
func (p *recordingProbe) ObserveVideoTaskRecord(_ context.Context, _ string, _, _, _ int64, state string) error {
	p.observed = state
	return p.observeErr
}

func TestVideoRecordAllCreationEntrypointsSaveBeforePublishing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []service.GrokMediaEndpoint{service.GrokMediaEndpointVideosGenerations, service.GrokMediaEndpointVideosEdits, service.GrokMediaEndpointVideosExtensions, service.SeedanceEndpointCreate} {
		for _, platform := range []string{service.PlatformGrok, service.PlatformOpenAI} {
			t.Run(string(endpoint)+platform, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c.Request = httptest.NewRequest("POST", "/v1/videos", nil).WithContext(ctx)
				p := &recordingProbe{beforeAccept: func() { require.Empty(t, w.Body.String()); require.Empty(t, w.Header().Get("X-Task")) }}
				_, err := forwardRecordedVideo(c, p, endpoint, &service.APIKey{ID: 2, UserID: 1}, &service.Account{ID: 3, Platform: platform}, nil, service.GrokVideoPendingBilling{Model: "video"}, "", func() (*service.OpenAIForwardResult, error) {
					require.True(t, p.begun)
					c.Header("X-Task", "accepted")
					c.Data(200, "application/json", []byte(`{"request_id":"task-1"}`))
					cancel() // acceptance must survive client cancellation
					return &service.OpenAIForwardResult{ResponseID: "task-1"}, nil
				})
				require.NoError(t, err)
				require.True(t, p.accepted)
				require.JSONEq(t, `{"request_id":"task-1"}`, w.Body.String())
				require.Equal(t, "accepted", w.Header().Get("X-Task"))
			})
		}
	}
}

func TestVideoRecordFailuresNeverPublishUntrackedSuccess(t *testing.T) {
	for _, scenario := range []string{"begin", "accept", "upstream400", "missing_id"} {
		t.Run(scenario, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
			p := &recordingProbe{}
			called := false
			if scenario == "begin" {
				p.beginErr = errors.New("database offline")
			}
			if scenario == "accept" {
				p.acceptErr = errors.New("database offline")
			}
			_, err := forwardRecordedVideo(c, p, service.GrokMediaEndpointVideosGenerations, &service.APIKey{ID: 2, UserID: 1}, &service.Account{ID: 3}, nil, service.GrokVideoPendingBilling{}, "", func() (*service.OpenAIForwardResult, error) {
				called = true
				if scenario == "upstream400" {
					c.JSON(400, gin.H{"error": "req_info undefined"})
					return nil, errors.New("upstream400")
				}
				c.JSON(200, gin.H{"request_id": "task-1"})
				if scenario == "missing_id" {
					return &service.OpenAIForwardResult{}, nil
				}
				return &service.OpenAIForwardResult{ResponseID: "task-1"}, nil
			})
			require.Error(t, err)
			if scenario == "begin" {
				require.False(t, called)
				require.Equal(t, 503, w.Code)
			} else {
				var unknown *service.OpenAIVideoCreateUnknownError
				require.ErrorAs(t, err, &unknown)
				if scenario == "upstream400" {
					require.Equal(t, 400, w.Code)
					require.False(t, p.accepted)
				} else {
					require.Empty(t, w.Body.String())
				}
			}
		})
	}
}

func TestVideoRecordTracksStatusAndDoesNotBufferDownloads(t *testing.T) {
	for _, endpoint := range []service.GrokMediaEndpoint{service.GrokMediaEndpointVideoStatus, service.GrokMediaEndpointVideoContent, service.SeedanceEndpointStatus} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-1", nil)
		p := &recordingProbe{observeErr: errors.New("transient database error")}
		_, err := forwardRecordedVideo(c, p, endpoint, &service.APIKey{ID: 2, UserID: 1}, &service.Account{ID: 3}, nil, service.GrokVideoPendingBilling{}, "task-1", func() (*service.OpenAIForwardResult, error) {
			c.Data(200, "application/json", []byte(`{"status":"succeeded"}`))
			if endpoint == service.GrokMediaEndpointVideoContent {
				require.NotEmpty(t, w.Body.String())
			}
			return &service.OpenAIForwardResult{VideoCount: 1}, nil
		})
		require.NoError(t, err)
		require.Equal(t, service.VideoTaskCompleted, p.observed)
		require.False(t, p.begun)
	}
}
