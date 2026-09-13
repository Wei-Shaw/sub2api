package securityaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakePromptAdminService struct {
	config        PublicConfig
	save          func(context.Context, UpdateConfigRequest, int64) (PublicConfig, error)
	probe         func(context.Context, ProbeRequest) ProbeResult
	runtime       RuntimeSnapshot
	list          func(context.Context, EventFilter, int, int) (*EventPage, error)
	get           func(context.Context, int64) (*Event, error)
	deleteOne     func(context.Context, int64) (*DeleteResult, error)
	deleteIDs     func(context.Context, []int64) (*DeleteResult, error)
	preview       func(context.Context, EventFilter, int64) (*DeletePreview, error)
	deleteFilter  func(context.Context, DeleteByFilterRequest, int64) (*DeleteResult, error)
	listRecords   func(context.Context, PromptRecordFilter, int, int) (*PromptRecordPage, error)
	getRecord     func(context.Context, int64) (*PromptRecord, error)
	deleteRecord  func(context.Context, int64) error
	deleteRecords func(context.Context, []int64) (int64, error)
	deleteAll     func(context.Context) (int64, error)
	recording     PromptRecordingConfig
	saveRecording func(context.Context, bool) (PromptRecordingConfig, error)
}

func (s *fakePromptAdminService) GetConfig() (PublicConfig, error) {
	return s.config, nil
}
func (s *fakePromptAdminService) SaveConfig(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
	if s.save == nil {
		return PublicConfig{}, errors.New("unexpected SaveConfig call")
	}
	return s.save(ctx, req, actorID)
}
func (s *fakePromptAdminService) Probe(ctx context.Context, req ProbeRequest) ProbeResult {
	if s.probe == nil {
		return ProbeResult{}
	}
	return s.probe(ctx, req)
}
func (s *fakePromptAdminService) Runtime(context.Context) RuntimeSnapshot { return s.runtime }
func (s *fakePromptAdminService) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	if s.list == nil {
		return &EventPage{}, nil
	}
	return s.list(ctx, filter, page, pageSize)
}
func (s *fakePromptAdminService) GetEvent(ctx context.Context, id int64) (*Event, error) {
	if s.get == nil {
		return nil, ErrEventNotFound
	}
	return s.get(ctx, id)
}
func (s *fakePromptAdminService) DeleteEvent(ctx context.Context, id int64) (*DeleteResult, error) {
	if s.deleteOne == nil {
		return &DeleteResult{}, nil
	}
	return s.deleteOne(ctx, id)
}
func (s *fakePromptAdminService) DeleteEventsByIDs(ctx context.Context, ids []int64) (*DeleteResult, error) {
	if s.deleteIDs == nil {
		return &DeleteResult{}, nil
	}
	return s.deleteIDs(ctx, ids)
}
func (s *fakePromptAdminService) PreviewDelete(ctx context.Context, filter EventFilter, actorID int64) (*DeletePreview, error) {
	if s.preview == nil {
		return &DeletePreview{}, nil
	}
	return s.preview(ctx, filter, actorID)
}
func (s *fakePromptAdminService) DeleteByFilter(ctx context.Context, req DeleteByFilterRequest, actorID int64) (*DeleteResult, error) {
	if s.deleteFilter == nil {
		return &DeleteResult{}, nil
	}
	return s.deleteFilter(ctx, req, actorID)
}
func (s *fakePromptAdminService) ListPromptRecords(ctx context.Context, filter PromptRecordFilter, page, pageSize int) (*PromptRecordPage, error) {
	if s.listRecords == nil {
		return &PromptRecordPage{}, nil
	}
	return s.listRecords(ctx, filter, page, pageSize)
}
func (s *fakePromptAdminService) GetPromptRecord(ctx context.Context, id int64) (*PromptRecord, error) {
	if s.getRecord == nil {
		return nil, ErrPromptRecordNotFound
	}
	return s.getRecord(ctx, id)
}
func (s *fakePromptAdminService) DeletePromptRecord(ctx context.Context, id int64) error {
	if s.deleteRecord == nil {
		return nil
	}
	return s.deleteRecord(ctx, id)
}
func (s *fakePromptAdminService) DeletePromptRecords(ctx context.Context, ids []int64) (int64, error) {
	if s.deleteRecords == nil {
		return int64(len(ids)), nil
	}
	return s.deleteRecords(ctx, ids)
}
func (s *fakePromptAdminService) DeleteAllPromptRecords(ctx context.Context) (int64, error) {
	if s.deleteAll == nil {
		return 0, nil
	}
	return s.deleteAll(ctx)
}
func (s *fakePromptAdminService) GetPromptRecordingConfig() PromptRecordingConfig {
	return s.recording
}

func (s *fakePromptAdminService) SavePromptRecordingSettings(_ context.Context, update PromptRecordingSettingsUpdate) (PromptRecordingConfig, error) {
	if update.Enabled != nil {
		s.recording.Enabled = *update.Enabled
	}
	if update.HeadersEnabled != nil {
		s.recording.HeadersEnabled = *update.HeadersEnabled
	}
	if update.PromptEnabled != nil {
		s.recording.PromptEnabled = *update.PromptEnabled
	}
	if update.ResponseEnabled != nil {
		s.recording.ResponseEnabled = *update.ResponseEnabled
	}
	if update.FilterPreset != nil {
		s.recording.FilterPreset = *update.FilterPreset
	}
	if update.FilterAgentPreset != nil {
		s.recording.FilterAgentPreset = *update.FilterAgentPreset
	}
	if update.FilterSkills != nil {
		s.recording.FilterSkills = *update.FilterSkills
	}
	return s.recording, nil
}

func TestPromptRecordingContentEndpoints(t *testing.T) {
	service := &fakePromptAdminService{recording: PromptRecordingConfig{
		Enabled: true, HeadersEnabled: true, PromptEnabled: true, ResponseEnabled: true,
		FilterAgentPreset: true, FilterSkills: true,
	}}
	router := promptAdminRouter(service)
	for _, key := range []string{"headers_enabled", "prompt_enabled", "response_enabled", "filter_agent_preset", "filter_skills"} {
		result := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-records/recording", map[string]any{key: false})
		require.Equal(t, http.StatusOK, result.Code)
		require.Contains(t, result.Body.String(), `"`+key+`":false`)
		require.Contains(t, result.Body.String(), `"enabled":true`)
	}
	combined := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-records/recording", map[string]any{
		"enabled": false, "headers_enabled": true, "prompt_enabled": true, "response_enabled": true,
		"filter_preset": true, "filter_agent_preset": true, "filter_skills": true,
	})
	require.Equal(t, http.StatusOK, combined.Code)
	require.Equal(t, PromptRecordingConfig{
		Enabled: false, HeadersEnabled: true, PromptEnabled: true, ResponseEnabled: true,
		FilterPreset: true, FilterAgentPreset: true, FilterSkills: true,
	}, service.recording)
	for _, payload := range []map[string]any{{"headers_enabled": "false"}, {"prompt_enabled": nil}, {"response_enabled": "false"}, {"filter_skills": nil}} {
		result := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-records/recording", payload)
		require.Equal(t, http.StatusBadRequest, result.Code)
	}
}
func (s *fakePromptAdminService) SavePromptRecordingConfig(ctx context.Context, enabled bool) (PromptRecordingConfig, error) {
	if s.saveRecording == nil {
		return PromptRecordingConfig{Enabled: enabled}, nil
	}
	return s.saveRecording(ctx, enabled)
}

func promptAdminRouter(service PromptAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Set(string(servermiddleware.ContextKeyUserRole), "admin")
		c.Next()
	})
	handler := NewPromptAdminHandler(service)
	group := router.Group("/admin/prompt-audit")
	group.GET("/config", handler.GetConfig)
	group.PUT("/config", handler.UpdateConfig)
	group.POST("/endpoints/probe", handler.ProbeEndpoint)
	group.GET("/runtime", handler.GetRuntime)
	group.GET("/events", handler.ListEvents)
	group.GET("/events/:id", handler.GetEvent)
	group.DELETE("/events/:id", handler.DeleteEvent)
	group.POST("/events/batch-delete", handler.BatchDelete)
	group.POST("/events/delete-preview", handler.DeletePreview)
	group.POST("/events/delete-by-filter", handler.DeleteByFilter)
	records := router.Group("/admin/prompt-records")
	records.GET("", handler.ListPromptRecords)
	records.GET("/recording", handler.GetPromptRecordingConfig)
	records.PUT("/recording", handler.UpdatePromptRecordingConfig)
	records.DELETE("/all", handler.DeleteAllPromptRecords)
	records.GET("/:id", handler.GetPromptRecord)
	records.DELETE("/:id", handler.DeletePromptRecord)
	records.POST("/batch-delete", handler.BatchDeletePromptRecords)
	return router
}

func TestPromptRecordingConfigEndpoints(t *testing.T) {
	service := &fakePromptAdminService{
		recording: PromptRecordingConfig{Enabled: true},
		saveRecording: func(_ context.Context, enabled bool) (PromptRecordingConfig, error) {
			require.False(t, enabled)
			return PromptRecordingConfig{Enabled: false}, nil
		},
	}
	router := promptAdminRouter(service)

	getResponse := promptAdminRequest(t, router, http.MethodGet, "/admin/prompt-records/recording", nil)
	require.Equal(t, http.StatusOK, getResponse.Code)
	require.Contains(t, getResponse.Body.String(), `"enabled":true`)

	updateResponse := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-records/recording", map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, updateResponse.Code)
	require.Contains(t, updateResponse.Body.String(), `"enabled":false`)

	invalidResponse := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-records/recording", map[string]any{})
	require.Equal(t, http.StatusBadRequest, invalidResponse.Code)
	require.Contains(t, invalidResponse.Body.String(), "prompt_recording_invalid_request")
}

func promptAdminRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestPromptAdminConfigRequiresVersionMapsConflictAndNeverEchoesToken(t *testing.T) {
	const canary = "prompt-admin-token-canary"

	t.Run("missing expected version", func(t *testing.T) {
		router := promptAdminRouter(&fakePromptAdminService{})
		response := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-audit/config", map[string]any{})
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, response.Body.String(), "prompt_audit_invalid_config_request")
	})

	t.Run("CAS conflict", func(t *testing.T) {
		service := &fakePromptAdminService{save: func(context.Context, UpdateConfigRequest, int64) (PublicConfig, error) {
			return PublicConfig{}, infraerrors.Conflict(ErrorCodeConfigConflict, "配置已被更新")
		}}
		response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPut, "/admin/prompt-audit/config", validHandlerUpdateRequest(canary))
		require.Equal(t, http.StatusConflict, response.Code)
		require.Contains(t, response.Body.String(), ErrorCodeConfigConflict)
		require.NotContains(t, response.Body.String(), canary)
	})

	t.Run("success public DTO", func(t *testing.T) {
		service := &fakePromptAdminService{save: func(_ context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
			require.Equal(t, int64(42), actorID)
			require.Equal(t, canary, req.Endpoints[0].Token)
			return PublicConfig{ConfigVersion: 8, Endpoints: []PublicEndpoint{{ID: "guard-1", HasToken: true, TokenStatus: "configured"}}}, nil
		}}
		response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPut, "/admin/prompt-audit/config", validHandlerUpdateRequest(canary))
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		require.NotContains(t, body, canary)
		require.NotContains(t, body, "token_ciphertext")
		require.NotContains(t, body, `"token":`)
		require.Contains(t, body, `"has_token":true`)
	})
}

func TestPromptAdminGetConfigReturnsSecretFreeUnavailableError(t *testing.T) {
	const canary = "persisted-config-secret-canary"
	repository := &switchableSettingRepository{loadErr: errors.New("failed to load token " + canary)}
	manager := NewConfigManager(nil, repository, nil, prefixEncryptor{}, testTotpKeyConfig())
	require.Error(t, manager.Reload(context.Background()))
	service := &PromptService{config: manager}

	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodGet, "/admin/prompt-audit/config", nil)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), ErrorCodeConfigUnavailable)
	require.NotContains(t, response.Body.String(), canary)
	require.NotContains(t, response.Body.String(), `"config_version"`)
	require.NotContains(t, response.Body.String(), `"token"`)
}

func TestPromptAdminProbeSupportsTemporaryOrSavedTokenWithoutEcho(t *testing.T) {
	const canary = "probe-token-canary"
	for _, tc := range []struct {
		name         string
		token        string
		tokenApplied bool
	}{
		{name: "temporary token", token: canary, tokenApplied: true},
		{name: "saved token", token: "", tokenApplied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakePromptAdminService{probe: func(_ context.Context, req ProbeRequest) ProbeResult {
				require.Equal(t, tc.token, req.Endpoint.Token)
				return ProbeResult{OK: true, Status: "healthy", Message: "ok", TokenApplied: tc.tokenApplied}
			}}
			endpoint := validHandlerUpdateRequest(tc.token).Endpoints[0]
			response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPost, "/admin/prompt-audit/endpoints/probe", ProbeRequest{Endpoint: endpoint})
			require.Equal(t, http.StatusOK, response.Code)
			require.NotContains(t, response.Body.String(), canary)
			require.NotContains(t, response.Body.String(), `"token":`)
			require.Contains(t, response.Body.String(), `"token_applied":true`)
		})
	}
}

func TestPromptAdminRejectsInvalidEventIDsTimesAndPagination(t *testing.T) {
	router := promptAdminRouter(&fakePromptAdminService{})
	for _, tc := range []struct {
		method string
		path   string
		body   any
		reason string
	}{
		{http.MethodGet, "/admin/prompt-audit/events/not-a-number", nil, "prompt_audit_invalid_event_id"},
		{http.MethodDelete, "/admin/prompt-audit/events/-1", nil, "prompt_audit_invalid_event_id"},
		{http.MethodGet, "/admin/prompt-audit/events?group_id=bad", nil, "prompt_audit_invalid_filter_id"},
		{http.MethodGet, "/admin/prompt-audit/events?start_at=not-time", nil, "prompt_audit_invalid_time"},
		{http.MethodGet, "/admin/prompt-audit/events?page=0", nil, "prompt_audit_invalid_pagination"},
		{http.MethodPost, "/admin/prompt-audit/events/batch-delete", map[string]any{"ids": []int64{1, -2}}, "prompt_audit_invalid_event_id"},
	} {
		response := promptAdminRequest(t, router, tc.method, tc.path, tc.body)
		require.Equalf(t, http.StatusBadRequest, response.Code, "%s %s", tc.method, tc.path)
		require.Contains(t, response.Body.String(), tc.reason)
	}
}

func validHandlerUpdateRequest(token string) UpdateConfigRequest {
	return UpdateConfigRequest{
		ExpectedConfigVersion: 7,
		Strategy:              "priority",
		WorkerCount:           1,
		QueueCapacity:         10,
		Scanners:              []string{"pii"},
		AllGroups:             true,
		Endpoints: []UpdateEndpoint{{
			ID: "guard-1", Name: "Guard One", Protocol: "openai_compatible",
			BaseURL: "http://127.0.0.1:18080", Model: DefaultGuardModel, Token: token,
			TimeoutMS: 1000, InputLimit: 1024, Enabled: true,
		}},
	}
}

func TestPromptAdminDeleteConfirmationErrorsStayGeneric(t *testing.T) {
	service := &fakePromptAdminService{deleteFilter: func(context.Context, DeleteByFilterRequest, int64) (*DeleteResult, error) {
		return nil, errors.New("sensitive-token-or-filter-detail")
	}}
	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPost, "/admin/prompt-audit/events/delete-by-filter", DeleteByFilterRequest{
		SnapshotMaxID: 3, FilterHash: strings.Repeat("a", 64), ConfirmationToken: "secret-confirmation", Confirm: true,
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "prompt_audit_delete_confirmation_invalid")
	require.NotContains(t, response.Body.String(), "sensitive-token")
	require.NotContains(t, response.Body.String(), "secret-confirmation")
}

func TestPromptRecordListParsesUserAndTimeFilters(t *testing.T) {
	service := &fakePromptAdminService{listRecords: func(_ context.Context, filter PromptRecordFilter, page, pageSize int) (*PromptRecordPage, error) {
		require.Equal(t, int64(7), *filter.UserID)
		require.Equal(t, "gpt-test", filter.Model)
		require.Equal(t, "2026-09-11T10:00:00Z", filter.StartAt.UTC().Format(time.RFC3339))
		require.Equal(t, "2026-09-11T11:00:00Z", filter.EndAt.UTC().Format(time.RFC3339))
		require.Equal(t, 2, page)
		require.Equal(t, 25, pageSize)
		return &PromptRecordPage{}, nil
	}}
	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodGet,
		"/admin/prompt-records?user_id=7&model=gpt-test&start_at=2026-09-11T10:00:00Z&end_at=2026-09-11T11:00:00Z&page=2&page_size=25", nil)
	require.Equal(t, http.StatusOK, response.Code)
}

func TestPromptRecordListRejectsInvalidTimeRange(t *testing.T) {
	response := promptAdminRequest(t, promptAdminRouter(&fakePromptAdminService{}), http.MethodGet,
		"/admin/prompt-records?start_at=2026-09-11T12:00:00Z&end_at=2026-09-11T11:00:00Z", nil)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "prompt_record_invalid_time_range")
}

func TestPromptRecordBatchDeleteDeduplicatesIDs(t *testing.T) {
	service := &fakePromptAdminService{deleteRecords: func(_ context.Context, ids []int64) (int64, error) {
		require.Equal(t, []int64{4, 8}, ids)
		return 2, nil
	}}
	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPost,
		"/admin/prompt-records/batch-delete", map[string]any{"ids": []int64{4, 4, 8}})
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"deleted":2`)
}

func TestPromptRecordDeleteAllReturnsDeletedCount(t *testing.T) {
	service := &fakePromptAdminService{deleteAll: func(context.Context) (int64, error) {
		return 983, nil
	}}
	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodDelete, "/admin/prompt-records/all", nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"deleted":983`)
}
