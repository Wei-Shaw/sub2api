package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type generationContentRepoStub struct {
	adoptionInput  service.GenerationContentAdoptionInput
	adoptionErr    error
	adoptionResult *service.GenerationContentAdoption
	weeklyReport   *service.GenerationContentWeeklyReport
	recentRows     []service.GenerationContentSample
}

func (s *generationContentRepoStub) Create(context.Context, *service.GenerationContent) error {
	panic("unexpected Create call")
}

func (s *generationContentRepoStub) CreateVideoTaskContent(context.Context, *service.GenerationContent) error {
	panic("unexpected CreateVideoTaskContent call")
}

func (s *generationContentRepoStub) GetCaptureStats(context.Context) (*service.GenerationContentStats, error) {
	return &service.GenerationContentStats{}, nil
}

func (s *generationContentRepoStub) GetRecent(context.Context, int) ([]service.GenerationContentSample, error) {
	return s.recentRows, nil
}

func (s *generationContentRepoStub) PurgeExpiredContent(context.Context, time.Time, int, bool) (int64, error) {
	panic("unexpected PurgeExpiredContent call")
}

func (s *generationContentRepoStub) UpdateVideoTaskAdoption(_ context.Context, input service.GenerationContentAdoptionInput) (*service.GenerationContentAdoption, error) {
	s.adoptionInput = input
	if s.adoptionErr != nil {
		return nil, s.adoptionErr
	}
	if s.adoptionResult != nil {
		return s.adoptionResult, nil
	}
	return &service.GenerationContentAdoption{
		TaskID:         input.TaskID,
		AdoptionStatus: input.AdoptionStatus,
		QualityScore:   input.QualityScore,
		Notes:          input.Notes,
		Saved:          true,
	}, nil
}

func TestGenerationContentHandlerUpdateAdoptionReportsTaskNotFound(t *testing.T) {
	repo := &generationContentRepoStub{adoptionResult: &service.GenerationContentAdoption{
		TaskID:         42,
		AdoptionStatus: "pending",
		Saved:          false,
	}}
	router := setupGenerationContentRouter(repo, &config.Config{
		Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/42/adoption", bytes.NewBufferString(`{"adoption_status":"pending"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), repo.adoptionInput.TaskID)
	require.Contains(t, rec.Body.String(), `"saved":false`)
	require.Contains(t, rec.Body.String(), `"reason":"task_not_found"`)
}

func (s *generationContentRepoStub) GetWeeklyReport(context.Context, time.Time, time.Time) (*service.GenerationContentWeeklyReport, error) {
	return s.weeklyReport, nil
}

func setupGenerationContentRouter(repo *generationContentRepoStub, cfg *config.Config, settingService ...*service.SettingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewGenerationContentHandler(repo, cfg, settingService...)
	router.POST("/api/v1/admin/generation-content/:task_id/adoption", h.UpdateAdoption)
	router.GET("/api/v1/admin/generation-content/samples", h.GetSamples)
	router.GET("/api/v1/admin/generation-content/weekly-report", h.GetWeeklyReport)
	return router
}

func TestGenerationContentHandlerSamplesIncludesCurrency(t *testing.T) {
	taskID := int64(42)
	repo := &generationContentRepoStub{recentRows: []service.GenerationContentSample{{
		TaskID:       &taskID,
		Model:        "seedance-v1",
		CreatedAt:    time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC),
		Username:     "operator",
		VideoStatus:  "succeeded",
		CostEstimate: 5.0094,
		Currency:     "CNY",
	}}}
	router := setupGenerationContentRouter(repo, &config.Config{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/generation-content/samples", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"currency":"CNY"`)
	require.Contains(t, rec.Body.String(), `"cost_estimate":5.0094`)
}

func TestGenerationContentHandlerSamplesIncludesUSDCNYRate(t *testing.T) {
	repo := &generationContentRepoStub{}
	settingSvc := service.NewSettingService(&dashboardSettingRepoStub{value: "7.41"}, nil)
	router := setupGenerationContentRouter(repo, &config.Config{}, settingSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/generation-content/samples", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"usd_cny_rate":7.41`)
}

func TestGenerationContentHandlerUpdateAdoptionMapsPayload(t *testing.T) {
	repo := &generationContentRepoStub{}
	router := setupGenerationContentRouter(repo, &config.Config{
		Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}},
	})

	body, err := json.Marshal(map[string]any{
		"adoption_status": "adopted",
		"quality_score":   0.875,
		"notes":           "picked for episode cut",
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/42/adoption", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), repo.adoptionInput.TaskID)
	require.Equal(t, "adopted", repo.adoptionInput.AdoptionStatus)
	require.NotNil(t, repo.adoptionInput.QualityScore)
	require.Equal(t, 0.875, *repo.adoptionInput.QualityScore)
	require.Equal(t, "picked for episode cut", repo.adoptionInput.Notes)
}

func TestGenerationContentHandlerUpdateAdoptionRejectsInvalidTaskID(t *testing.T) {
	repo := &generationContentRepoStub{}
	router := setupGenerationContentRouter(repo, &config.Config{
		Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/0/adoption", bytes.NewBufferString(`{"adoption_status":"adopted"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, repo.adoptionInput.TaskID)
	require.Contains(t, rec.Body.String(), "Invalid task_id")
}

func TestGenerationContentHandlerUpdateAdoptionRejectsInvalidStatus(t *testing.T) {
	repo := &generationContentRepoStub{}
	router := setupGenerationContentRouter(repo, &config.Config{
		Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/42/adoption", bytes.NewBufferString(`{"adoption_status":"done"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, repo.adoptionInput.TaskID)
	require.Contains(t, rec.Body.String(), "Invalid adoption_status")
}

func TestGenerationContentHandlerUpdateAdoptionRejectsOutOfRangeQualityScore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "above one", body: `{"adoption_status":"pending","quality_score":1.01}`},
		{name: "below zero", body: `{"adoption_status":"pending","quality_score":-0.01}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &generationContentRepoStub{}
			router := setupGenerationContentRouter(repo, &config.Config{
				Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}},
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/42/adoption", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, repo.adoptionInput.TaskID)
			require.Contains(t, rec.Body.String(), "quality_score must be between 0 and 1")
		})
	}
}

func TestGenerationContentHandlerUpdateAdoptionFlagOffFailOpen(t *testing.T) {
	repo := &generationContentRepoStub{}
	router := setupGenerationContentRouter(repo, &config.Config{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/42/adoption", bytes.NewBufferString(`{"adoption_status":"adopted"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, repo.adoptionInput.TaskID)
	require.Contains(t, rec.Body.String(), `"enabled":false`)
	require.Contains(t, rec.Body.String(), `"saved":false`)
}

func TestGenerationContentHandlerUpdateAdoptionRepoErrorReturns500(t *testing.T) {
	repo := &generationContentRepoStub{adoptionErr: errors.New("db temporarily unavailable")}
	router := setupGenerationContentRouter(repo, &config.Config{
		Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/generation-content/42/adoption", bytes.NewBufferString(`{"adoption_status":"pending"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestGenerationContentHandlerWeeklyReportReturnsMarkdown(t *testing.T) {
	repo := &generationContentRepoStub{weeklyReport: &service.GenerationContentWeeklyReport{
		PeriodStart:       time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:         time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		Entries:           10,
		VideoTasks:        4,
		TotalCostEstimate: 1.25,
		AdoptedCount:      3,
		PendingCount:      2,
		UnreviewedCount:   4,
		AdoptionRate:      0.3,
	}}
	router := setupGenerationContentRouter(repo, &config.Config{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/generation-content/weekly-report", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"entries":10`)
	require.Contains(t, rec.Body.String(), `"markdown"`)
	require.Contains(t, rec.Body.String(), "Weekly Production Ledger")
}

func TestGenerationContentHandlerWeeklyReportIncludesUSDCNYRate(t *testing.T) {
	repo := &generationContentRepoStub{weeklyReport: &service.GenerationContentWeeklyReport{}}
	settingSvc := service.NewSettingService(&dashboardSettingRepoStub{value: "7.41"}, nil)
	router := setupGenerationContentRouter(repo, &config.Config{}, settingSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/generation-content/weekly-report", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"usd_cny_rate":7.41`)
}

func TestGenerationContentHandlerWeeklyReportRejectsInvalidWindow(t *testing.T) {
	repo := &generationContentRepoStub{}
	router := setupGenerationContentRouter(repo, &config.Config{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/generation-content/weekly-report?start=2026-07-08&end=2026-07-01", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid weekly report window")
}
