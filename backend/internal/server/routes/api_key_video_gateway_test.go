package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type apiKeyVideoGatewayNoopEncryptor struct{}

func (apiKeyVideoGatewayNoopEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (apiKeyVideoGatewayNoopEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

type apiKeyVideoGatewayMemoryRepo struct {
	nextProviderID int64
	nextTaskID     int64
	nextEventID    int64
	providers      map[int64]*service.VideoProviderAccount
	tasks          map[int64]*service.VideoTask
	events         map[int64][]*service.VideoTaskEvent
	usage          []*service.VideoTask
	balanceClaims  map[int64]time.Time
	dailyTrials    map[string]struct{}
}

func newAPIKeyVideoGatewayMemoryRepo() *apiKeyVideoGatewayMemoryRepo {
	return &apiKeyVideoGatewayMemoryRepo{
		nextProviderID: 1,
		nextTaskID:     1,
		nextEventID:    1,
		providers:      make(map[int64]*service.VideoProviderAccount),
		tasks:          make(map[int64]*service.VideoTask),
		events:         make(map[int64][]*service.VideoTaskEvent),
		balanceClaims:  make(map[int64]time.Time),
		dailyTrials:    make(map[string]struct{}),
	}
}

func (r *apiKeyVideoGatewayMemoryRepo) seedProvider(provider string, enabled bool, metadata map[string]any) int64 {
	id := r.nextProviderID
	r.nextProviderID++
	now := time.Now().UTC()
	r.providers[id] = &service.VideoProviderAccount{
		ID:                 id,
		Provider:           provider,
		DisplayName:        provider + " test provider",
		Enabled:            enabled,
		EncryptedAPIKey:    "",
		MaskedKey:          "",
		BaseURL:            "mock://" + provider,
		DefaultModel:       provider + "-model",
		RateLimitPerMinute: 60,
		Metadata:           cloneAPIKeyVideoMetadata(metadata),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return id
}

func (r *apiKeyVideoGatewayMemoryRepo) CreateProviderAccount(_ context.Context, account *service.VideoProviderAccount) error {
	account.ID = r.nextProviderID
	r.nextProviderID++
	now := time.Now().UTC()
	account.CreatedAt = now
	account.UpdatedAt = now
	r.providers[account.ID] = cloneAPIKeyVideoProvider(account)
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) GetProviderAccount(_ context.Context, id int64) (*service.VideoProviderAccount, error) {
	item, ok := r.providers[id]
	if !ok {
		return nil, service.ErrVideoProviderNotFound
	}
	return cloneAPIKeyVideoProvider(item), nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListProviderAccounts(_ context.Context) ([]*service.VideoProviderAccount, error) {
	out := make([]*service.VideoProviderAccount, 0, len(r.providers))
	for _, item := range r.providers {
		out = append(out, cloneAPIKeyVideoProvider(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) UpdateProviderAccount(_ context.Context, account *service.VideoProviderAccount) error {
	account.UpdatedAt = time.Now().UTC()
	r.providers[account.ID] = cloneAPIKeyVideoProvider(account)
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) CreateTask(_ context.Context, task *service.VideoTask) error {
	task.ID = r.nextTaskID
	r.nextTaskID++
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	if provider, ok := r.providers[task.ProviderAccountID]; ok {
		task.ProviderAccountName = provider.DisplayName
	}
	r.tasks[task.ID] = cloneAPIKeyVideoTask(task)
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) CreateWithReservation(context.Context, service.VideoTaskCreationInput) (*service.VideoTaskCreationResult, error) {
	return nil, errors.New("atomic video task creation is not configured in route memory repository")
}

func (r *apiKeyVideoGatewayMemoryRepo) ReplayExisting(context.Context, service.VideoTaskCreationReplayInput) (*service.VideoTaskCreationResult, bool, error) {
	return nil, false, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) CreateDailyTrialTask(ctx context.Context, task *service.VideoTask, provider string, createdBy int64, trialDate time.Time) (bool, error) {
	key := fmt.Sprintf("%s:%d:%s", provider, createdBy, trialDate.Format("2006-01-02"))
	if _, ok := r.dailyTrials[key]; ok {
		return false, nil
	}
	r.dailyTrials[key] = struct{}{}
	if err := r.CreateTask(ctx, task); err != nil {
		delete(r.dailyTrials, key)
		return false, err
	}
	return true, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) GetTask(_ context.Context, id int64) (*service.VideoTask, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, service.ErrVideoTaskNotFound
	}
	return cloneAPIKeyVideoTask(task), nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListTasks(_ context.Context, params service.VideoTaskListParams) ([]*service.VideoTask, int64, error) {
	out := make([]*service.VideoTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if params.Provider != "" && task.Provider != params.Provider {
			continue
		}
		if params.Status != "" && task.Status != params.Status {
			continue
		}
		if !params.IsAdmin && task.CreatedBy != params.CreatedBy {
			continue
		}
		out = append(out, cloneAPIKeyVideoTask(task))
	}
	return out, int64(len(out)), nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListDramaTasks(ctx context.Context, params service.VideoTaskListParams, _ map[string]string) ([]*service.VideoTask, int64, error) {
	return r.ListTasks(ctx, params)
}

func (r *apiKeyVideoGatewayMemoryRepo) ListRunnableTasks(_ context.Context, limit int) ([]*service.VideoTask, error) {
	out := make([]*service.VideoTask, 0, limit)
	for _, task := range r.tasks {
		if service.IsTerminalVideoStatus(task.Status) {
			continue
		}
		out = append(out, cloneAPIKeyVideoTask(task))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListUnchargedSucceededVideoTasks(_ context.Context, limit int) ([]*service.VideoTask, error) {
	out := make([]*service.VideoTask, 0, limit)
	for _, task := range r.tasks {
		if task.Status != service.VideoStatusSucceeded {
			continue
		}
		if task.CostEstimate <= 0 && (task.UsageTotalTokens == nil || *task.UsageTotalTokens <= 0) {
			continue
		}
		if _, ok := r.balanceClaims[task.ID]; ok {
			continue
		}
		out = append(out, cloneAPIKeyVideoTask(task))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ClaimTaskForSubmit(_ context.Context, taskID int64) (bool, error) {
	task, ok := r.tasks[taskID]
	if !ok || task.Status != service.VideoStatusQueued {
		return false, nil
	}
	updated := cloneAPIKeyVideoTask(task)
	updated.Status = service.VideoStatusSubmitted
	updated.UpdatedAt = time.Now().UTC()
	r.tasks[taskID] = updated
	return true, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) UpdateTask(_ context.Context, task *service.VideoTask) error {
	if _, ok := r.tasks[task.ID]; !ok {
		return service.ErrVideoTaskNotFound
	}
	task.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = cloneAPIKeyVideoTask(task)
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) SetTaskLocalAsset(_ context.Context, taskID int64, path string, savedAt time.Time) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return service.ErrVideoTaskNotFound
	}
	task.LocalAssetPath = path
	t := savedAt
	task.LocalAssetSavedAt = &t
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ClearTaskLocalAsset(_ context.Context, taskID int64) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return nil
	}
	task.LocalAssetPath = ""
	task.LocalAssetSavedAt = nil
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListExpiredLocalAssets(_ context.Context, olderThan time.Time, limit int) ([]*service.VideoTask, error) {
	out := make([]*service.VideoTask, 0)
	for _, task := range r.tasks {
		if task.LocalAssetPath == "" || task.LocalAssetSavedAt == nil {
			continue
		}
		if task.LocalAssetSavedAt.Before(olderThan) {
			out = append(out, task)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) AddTaskEvent(_ context.Context, event *service.VideoTaskEvent) error {
	event.ID = r.nextEventID
	r.nextEventID++
	event.CreatedAt = time.Now().UTC()
	r.events[event.VideoTaskID] = append(r.events[event.VideoTaskID], cloneAPIKeyVideoEvent(event))
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListTaskEvents(_ context.Context, taskID int64, limit int) ([]*service.VideoTaskEvent, error) {
	items := r.events[taskID]
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	out := make([]*service.VideoTaskEvent, 0, len(items))
	for _, item := range items {
		out = append(out, cloneAPIKeyVideoEvent(item))
	}
	return out, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) InsertUsageLog(_ context.Context, task *service.VideoTask) error {
	r.usage = append(r.usage, cloneAPIKeyVideoTask(task))
	return nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ClaimVideoBalanceCharge(_ context.Context, taskID int64) (time.Time, bool, error) {
	if _, ok := r.balanceClaims[taskID]; ok {
		return time.Time{}, false, nil
	}
	if _, ok := r.tasks[taskID]; !ok {
		return time.Time{}, false, nil
	}
	claimedAt := time.Now().UTC()
	r.balanceClaims[taskID] = claimedAt
	return claimedAt, true, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ClearVideoBalanceChargeIfClaimedAt(_ context.Context, taskID int64, claimedAt time.Time) (bool, error) {
	existing, ok := r.balanceClaims[taskID]
	if !ok || !existing.Equal(claimedAt) {
		return false, nil
	}
	delete(r.balanceClaims, taskID)
	return true, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) CountTasksSince(context.Context, time.Time) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) CountProviderTasksSince(context.Context, time.Time) (map[string]map[string]int64, error) {
	return map[string]map[string]int64{}, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ProviderAccountTaskStatsSince(context.Context, time.Time) (map[int64]service.VideoProviderRuntimeStats, error) {
	stats := make(map[int64]service.VideoProviderRuntimeStats)
	for _, task := range r.tasks {
		item := stats[task.ProviderAccountID]
		item.TodayTasks++
		if task.Status == service.VideoStatusQueued || task.Status == service.VideoStatusSubmitted || task.Status == service.VideoStatusRunning {
			item.CurrentInflight++
		}
		if task.Status == service.VideoStatusFailed {
			item.TodayFailures++
			item.LastError = task.ErrorMessage
			t := task.UpdatedAt
			item.LastErrorAt = &t
		}
		stats[task.ProviderAccountID] = item
	}
	return stats, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) ListRecentTasksByStatus(_ context.Context, status string, limit int) ([]*service.VideoTask, error) {
	out := make([]*service.VideoTask, 0, limit)
	for _, task := range r.tasks {
		if task.Status == status {
			out = append(out, cloneAPIKeyVideoTask(task))
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) UsageSummarySince(context.Context, time.Time) ([]service.VideoUsageSummary, error) {
	return []service.VideoUsageSummary{}, nil
}

func (r *apiKeyVideoGatewayMemoryRepo) realProviderTaskCount() int {
	count := 0
	for _, task := range r.tasks {
		if task.Provider != service.VideoProviderMock {
			count++
		}
	}
	return count
}

func newAPIKeyVideoGatewayTestRouter(repo *apiKeyVideoGatewayMemoryRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{}
	cfg.Gateway.MaxBodySize = 1 << 20
	videoSvc := service.NewVideoGatewayService(repo, apiKeyVideoGatewayNoopEncryptor{}, cfg)
	videoHandler := handler.NewVideoHandler(videoSvc)
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			Video:         videoHandler,
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

func apiKeyVideoGatewayAuthMiddleware() middleware.APIKeyAuthMiddleware {
	return middleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			middleware.AbortWithError(c, http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required")
			return
		}
		groupID := int64(1)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
			ID:      100,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
		})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7, Concurrency: 1})
		c.Set(string(middleware.ContextKeyUserRole), "user")
		c.Next()
	})
}

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old := os.Getenv(key)
	require.NoError(t, os.Setenv(key, value))
	t.Cleanup(func() {
		if old == "" {
			require.NoError(t, os.Unsetenv(key))
		} else {
			require.NoError(t, os.Setenv(key, old))
		}
	})
}

func TestAPIKeyVideoGatewayRequiresAPIKey(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/v1/video/tasks", strings.NewReader(`{"task_type":"text_to_video","prompt":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "API_KEY_REQUIRED")
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewayMockCreateGetCancel(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"priority": 10, "key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	createBody := []byte(`{
		"provider":"mock",
		"task_type":"reference_to_video",
		"model":"mock-video-v1",
		"prompt":"QCanvas mock-only video task",
		"negative_prompt":"no artifacts",
		"reference_image_url":"https://example.invalid/ref.png",
		"reference_video_url":"https://example.invalid/ref.mp4",
		"aspect_ratio":"16:9",
		"duration":5,
		"resolution":"720p"
	}`)
	createRec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", createBody)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	require.Equal(t, 0, created.Code)
	require.Equal(t, service.VideoProviderMock, created.Data.Provider)
	require.Equal(t, service.VideoStatusQueued, created.Data.Status)
	require.True(t, created.Data.MockOnly)
	require.Equal(t, 0, created.Data.RealProviderDispatchCount)
	require.Equal(t, "https://example.invalid/ref.mp4", created.Data.ReferenceVideoURL)
	require.Equal(t, service.VideoDispatchStatePending, created.Data.DispatchState)
	require.Equal(t, service.VideoSettlementStatusPending, created.Data.SettlementStatus)
	require.Equal(t, service.VideoSideEffectStatusPending, created.Data.ArchiveStatus)
	require.Equal(t, service.VideoSideEffectStatusPending, created.Data.CaptureStatus)

	getRec := apiKeyVideoGatewayRequest(router, http.MethodGet, "/v1/video/tasks/1", nil)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
	require.Contains(t, getRec.Body.String(), "mock_only_gateway")

	cancelRec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks/1/cancel", nil)
	require.Equal(t, http.StatusOK, cancelRec.Code, cancelRec.Body.String())
	var canceled apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(cancelRec.Body.Bytes(), &canceled))
	require.Equal(t, service.VideoStatusCancelled, canceled.Data.Status)
	require.Equal(t, service.VideoProviderMock, canceled.Data.Provider)
	require.Equal(t, "api-key-video-mock-only", canceled.Data.ProviderBoundary)
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewayCancelPreservesSeedanceProvenanceBoundaries(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	seedanceID := repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":    "normal",
		"health_status": "healthy",
	})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	production := &service.VideoTask{
		ID:                11,
		ProviderAccountID: seedanceID,
		Provider:          service.VideoProviderSeedance,
		Status:            service.VideoStatusCancelled,
		CreatedBy:         7,
		Version:           1,
	}
	repo.tasks[production.ID] = production

	tiny := &service.VideoTask{
		ID:                12,
		ProviderAccountID: seedanceID,
		Provider:          service.VideoProviderSeedance,
		Status:            service.VideoStatusCancelled,
		CreatedBy:         7,
		Version:           1,
	}
	repo.tasks[tiny.ID] = tiny
	require.NoError(t, repo.AddTaskEvent(context.Background(), &service.VideoTaskEvent{
		VideoTaskID: tiny.ID,
		EventType:   "trial_gate",
		Message:     "tiny trial gate passed",
		Payload: map[string]any{
			"trial_mode":  "tiny_real",
			"gate_result": "passed",
		},
	}))

	productionRec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks/11/cancel", nil)
	require.Equal(t, http.StatusOK, productionRec.Code, productionRec.Body.String())
	var productionBody apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(productionRec.Body.Bytes(), &productionBody))
	require.Equal(t, service.VideoStatusCancelled, productionBody.Data.Status)
	require.Equal(t, "api-key-video-seedance-production", productionBody.Data.ProviderBoundary)
	require.Empty(t, productionBody.Data.TrialMode)

	tinyRec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks/12/cancel", nil)
	require.Equal(t, http.StatusOK, tinyRec.Code, tinyRec.Body.String())
	var tinyBody apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(tinyRec.Body.Bytes(), &tinyBody))
	require.Equal(t, service.VideoStatusCancelled, tinyBody.Data.Status)
	require.Equal(t, "api-key-video-seedance-tiny-trial", tinyBody.Data.ProviderBoundary)
	require.Equal(t, "tiny_real", tinyBody.Data.TrialMode)
	require.Equal(t, "passed", tinyBody.Data.TrialGateResult)
}

func TestAPIKeyVideoGatewayCreateAttributesCredentialAndReturnsStableIdempotencyKey(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"priority": 10, "key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	body := []byte(`{"provider":"mock","task_type":"text_to_video","prompt":"credential attribution"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/video/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "  qcanvas-create-42  ")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, "qcanvas-create-42", rec.Header().Get("Idempotency-Key"))

	var created apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "qcanvas-create-42", created.Data.IdempotencyKey)
	stored := repo.tasks[created.Data.ID]
	require.NotNil(t, stored)
	require.NotNil(t, stored.APIKeyID)
	require.Equal(t, int64(100), *stored.APIKeyID)
	require.Nil(t, stored.ReservationID, "mock tasks must not reserve balance")
}

func TestAPIKeyVideoGatewayCreateGeneratesIdempotencyKeyWhenHeaderMissing(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"priority": 10, "key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{"provider":"mock","task_type":"text_to_video","prompt":"generated key"}`))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rawKey := rec.Header().Get("Idempotency-Key")
	require.NotEmpty(t, rawKey)
	require.NotEqual(t, service.HashIdempotencyKey(rawKey), rawKey, "the response returns the raw retry key, never its stored hash")

	var created apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, rawKey, created.Data.IdempotencyKey)
}

func TestAPIKeyVideoGatewayBlocksSeedanceWithoutTrialMode(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(
		router,
		http.MethodPost,
		"/v1/video/tasks",
		[]byte(`{"provider":"seedance","task_type":"text_to_video","prompt":"must stay blocked"}`),
	)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Message  string            `json:"message"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, http.StatusForbidden, body.Code)
	require.Equal(t, "VIDEO_PRODUCTION_NOT_AUTHORIZED", body.Reason)
	require.Equal(t, service.VideoProviderSeedance, body.Metadata["provider"])
	require.Equal(t, "0", body.Metadata["real_provider_dispatch_count"])
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewayBlocksKling(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderKling, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(
		router,
		http.MethodPost,
		"/v1/video/tasks",
		[]byte(`{"provider":"kling","task_type":"text_to_video","prompt":"must stay blocked"}`),
	)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Message  string            `json:"message"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, http.StatusForbidden, body.Code)
	require.Equal(t, "VIDEO_PROVIDER_DISABLED", body.Reason)
	require.Equal(t, service.VideoProviderKling, body.Metadata["provider"])
	require.Equal(t, "0", body.Metadata["real_provider_dispatch_count"])
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewaySeedanceTrialBlockedWithoutGate(t *testing.T) {
	setEnv(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "")
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":              "normal",
		"health_status":           "healthy",
		"single_smoke_authorized": true,
	})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"must stay blocked",
		"duration":5
	}`))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Message  string            `json:"message"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "VIDEO_TRIAL_BLOCKED", body.Reason)
	require.Contains(t, body.Metadata["blocked_reasons"], "redacted event log path is missing")
	require.Contains(t, body.Metadata["blocked_reasons"], "media url allowlist (SUB2API_MEDIA_URL_ALLOWLIST or SUB2API_VIDEO_URL_ALLOWLIST) is missing")
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewaySeedanceTrialBlockedDurationTooLong(t *testing.T) {
	setEnv(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	setEnv(t, "SUB2API_VIDEO_REDACTED_EVENT_LOG", "/tmp/redacted.log")
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":              "normal",
		"health_status":           "healthy",
		"single_smoke_authorized": true,
	})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"duration too long",
		"duration":16
	}`))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Message  string            `json:"message"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "VIDEO_TRIAL_BLOCKED", body.Reason)
	require.Contains(t, body.Metadata["blocked_reasons"], "duration")
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewaySeedanceTrialBlockedMissingAuthorization(t *testing.T) {
	setEnv(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	setEnv(t, "SUB2API_VIDEO_REDACTED_EVENT_LOG", "/tmp/redacted.log")
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":    "normal",
		"health_status": "healthy",
	})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"missing auth",
		"duration":5
	}`))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Message  string            `json:"message"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "VIDEO_TRIAL_BLOCKED", body.Reason)
	require.Contains(t, body.Metadata["blocked_reasons"], "single smoke authorization")
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewaySeedanceTrialBlockedMissingEventLog(t *testing.T) {
	setEnv(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":              "normal",
		"health_status":           "healthy",
		"single_smoke_authorized": true,
	})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"missing event log",
		"duration":5
	}`))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Message  string            `json:"message"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "VIDEO_TRIAL_BLOCKED", body.Reason)
	require.Contains(t, body.Metadata["blocked_reasons"], "redacted event log path is missing")
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewaySeedanceTrialBlocksRouteUnavailableAccountWithKey(t *testing.T) {
	setEnv(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	setEnv(t, "SUB2API_VIDEO_REDACTED_EVENT_LOG", "/tmp/redacted.log")
	setEnv(t, "SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	seedanceID := repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":              "rate_limited",
		"health_status":           "healthy",
		"single_smoke_authorized": true,
	})
	repo.providers[seedanceID].EncryptedAPIKey = "enc:test-only-placeholder"
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"route unavailable account must stay blocked",
		"duration":5
	}`))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code     int               `json:"code"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "VIDEO_PROVIDER_DISABLED", body.Reason)
	require.Equal(t, service.VideoProviderSeedance, body.Metadata["provider"])
	require.Equal(t, "0", body.Metadata["real_provider_dispatch_count"])
	require.Equal(t, 0, repo.realProviderTaskCount())
}

func TestAPIKeyVideoGatewaySeedanceTrialSuccess(t *testing.T) {
	setEnv(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	setEnv(t, "SUB2API_VIDEO_REDACTED_EVENT_LOG", "/tmp/redacted.log")
	// Fail-closed SSRF posture now requires the media URL allowlist before the
	// real trial path is permitted (see seedanceSmokeGateBlockedReasons).
	setEnv(t, "SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{
		"key_status":              "normal",
		"health_status":           "healthy",
		"single_smoke_authorized": true,
	})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"successful tiny real trial",
		"duration":5
	}`))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var body apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, service.VideoProviderSeedance, body.Data.Provider)
	require.False(t, body.Data.MockOnly)
	require.Equal(t, "api-key-video-seedance-tiny-trial", body.Data.ProviderBoundary)
	require.Equal(t, 0, body.Data.RealProviderDispatchCount)
	require.Equal(t, 1, repo.realProviderTaskCount())

	secondRec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"seedance",
		"trial_mode":"tiny_real",
		"task_type":"text_to_video",
		"prompt":"second tiny real trial must stay blocked",
		"duration":5
	}`))
	require.Equal(t, http.StatusForbidden, secondRec.Code, secondRec.Body.String())
	var secondBody struct {
		Code     int               `json:"code"`
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(secondRec.Body.Bytes(), &secondBody))
	require.Equal(t, "VIDEO_TRIAL_LIMIT_EXCEEDED", secondBody.Reason)
	require.Equal(t, "0", secondBody.Metadata["real_provider_dispatch_count"])
	require.Equal(t, 1, repo.realProviderTaskCount())

	getRec := apiKeyVideoGatewayRequest(router, http.MethodGet, fmt.Sprintf("/v1/video/tasks/%d", body.Data.ID), nil)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
	require.Contains(t, getRec.Body.String(), "trial_gate")

	var getBody apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getBody))
	require.Equal(t, 0, getBody.Data.RealProviderDispatchCount)
	require.Equal(t, "tiny_real", getBody.Data.TrialMode)
	require.Equal(t, "passed", getBody.Data.TrialGateResult)

	require.NoError(t, repo.AddTaskEvent(context.Background(), &service.VideoTaskEvent{
		VideoTaskID: body.Data.ID,
		EventType:   service.VideoStatusSubmitted,
		Message:     "video task submitted to provider",
	}))

	submittedRec := apiKeyVideoGatewayRequest(router, http.MethodGet, fmt.Sprintf("/v1/video/tasks/%d", body.Data.ID), nil)
	require.Equal(t, http.StatusOK, submittedRec.Code, submittedRec.Body.String())
	var submittedBody apiKeyVideoGatewayTaskEnvelope
	require.NoError(t, json.Unmarshal(submittedRec.Body.Bytes(), &submittedBody))
	require.Equal(t, 1, submittedBody.Data.RealProviderDispatchCount)
}

func TestAPIKeyVideoGatewayProvidersReturnSeedanceReadiness(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodGet, "/v1/video/providers", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body apiKeyVideoGatewayProvidersEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.False(t, body.Data.MockOnly)
	require.True(t, body.Data.TrialModeSupported)
	providers := map[string]apiKeyVideoGatewayProvider{}
	for _, item := range body.Data.Items {
		providers[item.Provider] = item
	}
	require.True(t, providers[service.VideoProviderMock].RouteAvailable)
	require.False(t, providers[service.VideoProviderSeedance].RouteAvailable)
}

func TestAPIKeyVideoGatewayProvidersDoNotExposeSecrets(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	seedanceID := repo.seedProvider(service.VideoProviderSeedance, true, map[string]any{"key_status": "normal", "health_status": "healthy"})
	repo.providers[seedanceID].EncryptedAPIKey = "enc:test-only-placeholder"
	repo.providers[seedanceID].MaskedKey = "sdnc***demo"
	repo.seedProvider(service.VideoProviderKling, false, map[string]any{"key_status": "disabled", "health_status": "disabled"})
	router := newAPIKeyVideoGatewayTestRouter(repo)

	rec := apiKeyVideoGatewayRequest(router, http.MethodGet, "/v1/video/providers", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "test-only-placeholder")
	require.NotContains(t, rec.Body.String(), "sdnc***demo")
	var body apiKeyVideoGatewayProvidersEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.False(t, body.Data.MockOnly)
	require.Equal(t, 0, body.Data.RealProviderDispatchCount)
	require.True(t, body.Data.TrialModeSupported)
	require.Len(t, body.Data.Items, 3)
	providers := map[string]apiKeyVideoGatewayProvider{}
	for _, item := range body.Data.Items {
		providers[item.Provider] = item
	}
	require.True(t, providers[service.VideoProviderMock].RouteAvailable)
	require.False(t, providers[service.VideoProviderSeedance].RouteAvailable)
	require.False(t, providers[service.VideoProviderKling].RouteAvailable)
	require.Equal(t, "", providers[service.VideoProviderSeedance].MaskedKey)
}

func apiKeyVideoGatewayRequest(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer test-api-key")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type apiKeyVideoGatewayTaskEnvelope struct {
	Code int `json:"code"`
	Data struct {
		ID                        int64  `json:"id"`
		Provider                  string `json:"provider"`
		Model                     string `json:"model"`
		TaskType                  string `json:"task_type"`
		Status                    string `json:"status"`
		ResultURL                 string `json:"result_url"`
		ErrorMessage              string `json:"error_message"`
		UpstreamTaskID            string `json:"upstream_task_id"`
		ReferenceVideoURL         string `json:"reference_video_url"`
		MockOnly                  bool   `json:"mock_only"`
		ProviderBoundary          string `json:"provider_boundary"`
		RealProviderDispatchCount int    `json:"real_provider_dispatch_count"`
		TrialMode                 string `json:"trial_mode"`
		BlockedReason             string `json:"blocked_reason"`
		TrialGateResult           string `json:"trial_gate_result"`
		IdempotencyKey            string `json:"idempotency_key"`
		DispatchState             string `json:"dispatch_state"`
		SettlementStatus          string `json:"settlement_status"`
		ArchiveStatus             string `json:"archive_status"`
		CaptureStatus             string `json:"capture_status"`
		CreatedAt                 string `json:"created_at"`
		UpdatedAt                 string `json:"updated_at"`
	} `json:"data"`
}

type apiKeyVideoGatewayProvidersEnvelope struct {
	Code int `json:"code"`
	Data struct {
		Items                     []apiKeyVideoGatewayProvider `json:"items"`
		MockOnly                  bool                         `json:"mock_only"`
		RealProviderDispatchCount int                          `json:"real_provider_dispatch_count"`
		TrialModeSupported        bool                         `json:"trial_mode_supported"`
	} `json:"data"`
}

type apiKeyVideoGatewayProvider struct {
	Provider        string `json:"provider"`
	MaskedKey       string `json:"masked_key"`
	RouteAvailable  bool   `json:"route_available"`
	RouteSkipReason string `json:"route_skip_reason"`
}

func cloneAPIKeyVideoProvider(in *service.VideoProviderAccount) *service.VideoProviderAccount {
	if in == nil {
		return nil
	}
	out := *in
	out.Metadata = cloneAPIKeyVideoMetadata(in.Metadata)
	if in.LastTestAt != nil {
		t := *in.LastTestAt
		out.LastTestAt = &t
	}
	return &out
}

func cloneAPIKeyVideoTask(in *service.VideoTask) *service.VideoTask {
	if in == nil {
		return nil
	}
	out := *in
	if in.Content != nil {
		out.Content = append([]service.VideoTaskContentItem(nil), in.Content...)
	}
	if in.GenerateAudio != nil {
		v := *in.GenerateAudio
		out.GenerateAudio = &v
	}
	if in.Watermark != nil {
		v := *in.Watermark
		out.Watermark = &v
	}
	if in.CameraFixed != nil {
		v := *in.CameraFixed
		out.CameraFixed = &v
	}
	if in.ReturnLastFrame != nil {
		v := *in.ReturnLastFrame
		out.ReturnLastFrame = &v
	}
	if in.UsageTotalTokens != nil {
		v := *in.UsageTotalTokens
		out.UsageTotalTokens = &v
	}
	if in.ActualDuration != nil {
		v := *in.ActualDuration
		out.ActualDuration = &v
	}
	if in.CompletedAt != nil {
		t := *in.CompletedAt
		out.CompletedAt = &t
	}
	return &out
}

func cloneAPIKeyVideoEvent(in *service.VideoTaskEvent) *service.VideoTaskEvent {
	if in == nil {
		return nil
	}
	out := *in
	out.Payload = cloneAPIKeyVideoMetadata(in.Payload)
	return &out
}

func cloneAPIKeyVideoMetadata(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var _ service.VideoGatewayRepository = (*apiKeyVideoGatewayMemoryRepo)(nil)
