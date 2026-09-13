//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type monitorTemplateHandlerRepo struct {
	service.ChannelMonitorRequestTemplateRepository
	template *service.ChannelMonitorRequestTemplate
}

func (r *monitorTemplateHandlerRepo) Create(_ context.Context, template *service.ChannelMonitorRequestTemplate) error {
	template.ID = 1
	stored := *template
	r.template = &stored
	return nil
}

func (r *monitorTemplateHandlerRepo) GetByID(_ context.Context, id int64) (*service.ChannelMonitorRequestTemplate, error) {
	if r.template == nil || r.template.ID != id {
		return nil, service.ErrChannelMonitorTemplateNotFound
	}
	return r.template, nil
}

func (r *monitorTemplateHandlerRepo) CountAssociatedMonitors(context.Context, int64) (int64, error) {
	return 0, nil
}

func TestChannelMonitorTemplateHandlerAgentProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, provider := range []string{service.MonitorProviderCursor, service.MonitorProviderDevin} {
		t.Run(provider, func(t *testing.T) {
			repo := &monitorTemplateHandlerRepo{}
			handler := NewChannelMonitorRequestTemplateHandler(service.NewChannelMonitorRequestTemplateService(repo))
			router := gin.New()
			router.POST("/templates", handler.Create)
			router.GET("/templates/:id", handler.Get)

			request := httptest.NewRequest(http.MethodPost, "/templates", strings.NewReader(`{"name":"agent template","provider":"`+provider+`","api_mode":"chat_completions"}`))
			request.Header.Set("Content-Type", "application/json")
			created := httptest.NewRecorder()
			router.ServeHTTP(created, request)
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

			loaded := httptest.NewRecorder()
			router.ServeHTTP(loaded, httptest.NewRequest(http.MethodGet, "/templates/1", nil))
			require.Equal(t, http.StatusOK, loaded.Code, loaded.Body.String())
			require.Contains(t, loaded.Body.String(), `"provider":"`+provider+`"`)
		})
	}
}

func TestChannelMonitorTemplateHandlerRejectsNonProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &monitorTemplateHandlerRepo{}
	handler := NewChannelMonitorRequestTemplateHandler(service.NewChannelMonitorRequestTemplateService(repo))
	router := gin.New()
	router.POST("/templates", handler.Create)
	request := httptest.NewRequest(http.MethodPost, "/templates", strings.NewReader(`{"name":"invalid","provider":"composite"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Nil(t, repo.template)
}
