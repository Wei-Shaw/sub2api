package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type routeAdoptionGenContentRepo struct {
	lastInput service.GenerationContentAdoptionInput
}

func (r *routeAdoptionGenContentRepo) Create(context.Context, *service.GenerationContent) error {
	return nil
}
func (r *routeAdoptionGenContentRepo) CreateVideoTaskContent(context.Context, *service.GenerationContent) error {
	return nil
}
func (r *routeAdoptionGenContentRepo) UpdateVideoTaskAdoption(_ context.Context, input service.GenerationContentAdoptionInput) (*service.GenerationContentAdoption, error) {
	r.lastInput = input
	return &service.GenerationContentAdoption{
		TaskID:         input.TaskID,
		AdoptionStatus: input.AdoptionStatus,
		Saved:          true,
	}, nil
}
func (r *routeAdoptionGenContentRepo) GetWeeklyReport(context.Context, time.Time, time.Time) (*service.GenerationContentWeeklyReport, error) {
	return nil, nil
}
func (r *routeAdoptionGenContentRepo) GetCaptureStats(context.Context) (*service.GenerationContentStats, error) {
	return &service.GenerationContentStats{}, nil
}
func (r *routeAdoptionGenContentRepo) GetRecent(context.Context, int) ([]service.GenerationContentSample, error) {
	return nil, nil
}
func (r *routeAdoptionGenContentRepo) PurgeExpiredContent(context.Context, time.Time, int, bool) (int64, error) {
	return 0, nil
}

func newGenerationContentAdoptionTestRouter(repo *apiKeyVideoGatewayMemoryRepo, genRepo service.GenerationContentRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{}
	cfg.Gateway.MaxBodySize = 1 << 20
	cfg.Gateway.ContentCapture.Enabled = true
	videoSvc := service.NewVideoGatewayService(repo, apiKeyVideoGatewayNoopEncryptor{}, cfg)
	adoptionSvc := service.NewGenerationContentAdoptionService(videoSvc, genRepo, cfg)
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:           &handler.GatewayHandler{},
			OpenAIGateway:     &handler.OpenAIGatewayHandler{},
			Video:             handler.NewVideoHandler(videoSvc),
			GenerationContent: handler.NewGenerationContentHandler(adoptionSvc),
		},
		apiKeyVideoGatewayAuthMiddleware(),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	return router
}

func TestGenerationContentAdoptionRequiresAPIKey(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	genRepo := &routeAdoptionGenContentRepo{}
	router := newGenerationContentAdoptionTestRouter(repo, genRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/generation-content/1/adoption", strings.NewReader(`{"adoption_status":"adopted"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "API_KEY_REQUIRED")
}

func TestGenerationContentAdoptionOwnedTaskSucceeds(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	require.NoError(t, repo.CreateTask(context.Background(), &service.VideoTask{CreatedBy: 7}))
	genRepo := &routeAdoptionGenContentRepo{}
	router := newGenerationContentAdoptionTestRouter(repo, genRepo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/generation-content/1/adoption", []byte(`{"adoption_status":"adopted"}`))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Saved  bool `json:"saved"`
			TaskID int  `json:"task_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.True(t, body.Data.Saved)
	require.Equal(t, 1, body.Data.TaskID)
	require.Equal(t, "adopted", genRepo.lastInput.AdoptionStatus)
}

func TestGenerationContentAdoptionForeignTaskForbidden(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	require.NoError(t, repo.CreateTask(context.Background(), &service.VideoTask{CreatedBy: 99}))
	genRepo := &routeAdoptionGenContentRepo{}
	router := newGenerationContentAdoptionTestRouter(repo, genRepo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/generation-content/1/adoption", []byte(`{"adoption_status":"adopted"}`))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "VIDEO_TASK_FORBIDDEN")
}
