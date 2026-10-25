//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stepFunMonitorHandlerRepoStub struct {
	service.ChannelMonitorRepository
	monitor     *service.ChannelMonitor
	createCalls int
	updateCalls int
}

func (r *stepFunMonitorHandlerRepoStub) Create(_ context.Context, monitor *service.ChannelMonitor) error {
	r.createCalls++
	monitor.ID = 101
	return nil
}

func (r *stepFunMonitorHandlerRepoStub) GetByID(_ context.Context, id int64) (*service.ChannelMonitor, error) {
	if r.monitor == nil || r.monitor.ID != id {
		return nil, service.ErrChannelMonitorNotFound
	}
	clone := *r.monitor
	return &clone, nil
}

func (r *stepFunMonitorHandlerRepoStub) Update(_ context.Context, monitor *service.ChannelMonitor) error {
	r.updateCalls++
	clone := *monitor
	r.monitor = &clone
	return nil
}

type stepFunTemplateHandlerRepoStub struct {
	service.ChannelMonitorRequestTemplateRepository
	createCalls int
}

func (r *stepFunTemplateHandlerRepoStub) Create(_ context.Context, template *service.ChannelMonitorRequestTemplate) error {
	r.createCalls++
	template.ID = 201
	template.CreatedAt = time.Now()
	template.UpdatedAt = template.CreatedAt
	return nil
}

func (r *stepFunTemplateHandlerRepoStub) CountAssociatedMonitors(context.Context, int64) (int64, error) {
	return 0, nil
}

func newStepFunHandlerRouter(t *testing.T) (*gin.Engine, *stepFunMonitorHandlerRepoStub, *stepFunTemplateHandlerRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	monitorRepo := &stepFunMonitorHandlerRepoStub{monitor: &service.ChannelMonitor{
		ID:               42,
		Name:             "stepfun",
		Provider:         service.MonitorProviderStepfun,
		APIMode:          service.MonitorAPIModeChatCompletions,
		Endpoint:         "https://api.stepfun.com/step_plan/v1",
		APIKey:           "ENC:sk-stepfun",
		PrimaryModel:     "step-3.5-flash",
		Enabled:          true,
		IntervalSeconds:  60,
		BodyOverrideMode: service.MonitorBodyOverrideModeOff,
		CheckMode:        service.MonitorCheckModeProbe,
	}}
	monitorService := service.NewChannelMonitorService(monitorRepo, duplicateChannelMonitorHandlerEncryptor{})
	monitorHandler := NewChannelMonitorHandler(monitorService)

	templateRepo := &stepFunTemplateHandlerRepoStub{}
	templateService := service.NewChannelMonitorRequestTemplateService(templateRepo)
	templateHandler := NewChannelMonitorRequestTemplateHandler(templateService)

	router := gin.New()
	router.POST("/api/v1/admin/channel-monitors", monitorHandler.Create)
	router.PUT("/api/v1/admin/channel-monitors/:id", monitorHandler.Update)
	router.POST("/api/v1/admin/channel-monitor-templates", templateHandler.Create)
	return router, monitorRepo, templateRepo
}

func performJSONRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestChannelMonitorHandlerCreatesStepFunMonitor(t *testing.T) {
	router, repo, _ := newStepFunHandlerRouter(t)

	recorder := performJSONRequest(router, http.MethodPost, "/api/v1/admin/channel-monitors", `{
		"name":"stepfun",
		"provider":"stepfun",
		"endpoint":"https://api.stepfun.com/step_plan/v1",
		"api_key":"sk-stepfun",
		"primary_model":"step-3.5-flash",
		"interval_seconds":60,
		"check_mode":"probe"
	}`)

	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.createCalls)
	require.Contains(t, recorder.Body.String(), `"provider":"stepfun"`)
}

func TestChannelMonitorHandlerUpdatesStepFunMonitor(t *testing.T) {
	router, repo, _ := newStepFunHandlerRouter(t)

	recorder := performJSONRequest(router, http.MethodPut, "/api/v1/admin/channel-monitors/42", `{"provider":"stepfun"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.updateCalls)
}

func TestChannelMonitorTemplateHandlerCreatesStepFunTemplate(t *testing.T) {
	router, _, repo := newStepFunHandlerRouter(t)

	recorder := performJSONRequest(router, http.MethodPost, "/api/v1/admin/channel-monitor-templates", `{
		"name":"stepfun template",
		"provider":"stepfun"
	}`)

	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.createCalls)
	require.Contains(t, recorder.Body.String(), `"provider":"stepfun"`)
}
