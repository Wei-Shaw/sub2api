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

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakePromptAdminService struct {
	config       PublicConfig
	save         func(context.Context, UpdateConfigRequest, int64) (PublicConfig, error)
	probe        func(context.Context, ProbeRequest) ProbeResult
	runtime      RuntimeSnapshot
	list         func(context.Context, EventFilter, int, int) (*EventPage, error)
	get          func(context.Context, int64) (*Event, error)
	deleteOne    func(context.Context, int64) (*DeleteResult, error)
	deleteIDs    func(context.Context, []int64) (*DeleteResult, error)
	preview      func(context.Context, EventFilter, int64) (*DeletePreview, error)
	deleteFilter func(context.Context, DeleteByFilterRequest, int64) (*DeleteResult, error)
REDACTED

func (s *fakePromptAdminService) GetConfig() (PublicConfig, error) {
	return s.config, nil
REDACTED
func (s *fakePromptAdminService) SaveConfig(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
	if s.save == nil {
		return PublicConfig{REDACTED, errors.New("unexpected SaveConfig call")
REDACTED
	return s.save(ctx, req, actorID)
REDACTED
func (s *fakePromptAdminService) Probe(ctx context.Context, req ProbeRequest) ProbeResult {
	if s.probe == nil {
		return ProbeResult{REDACTED
REDACTED
	return s.probe(ctx, req)
REDACTED
func (s *fakePromptAdminService) Runtime(context.Context) RuntimeSnapshot { return s.runtime REDACTED
func (s *fakePromptAdminService) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	if s.list == nil {
		return &EventPage{REDACTED, nil
REDACTED
	return s.list(ctx, filter, page, pageSize)
REDACTED
func (s *fakePromptAdminService) GetEvent(ctx context.Context, id int64) (*Event, error) {
	if s.get == nil {
		return nil, ErrEventNotFound
REDACTED
	return s.get(ctx, id)
REDACTED
func (s *fakePromptAdminService) DeleteEvent(ctx context.Context, id int64) (*DeleteResult, error) {
	if s.deleteOne == nil {
		return &DeleteResult{REDACTED, nil
REDACTED
	return s.deleteOne(ctx, id)
REDACTED
func (s *fakePromptAdminService) DeleteEventsByIDs(ctx context.Context, ids []int64) (*DeleteResult, error) {
	if s.deleteIDs == nil {
		return &DeleteResult{REDACTED, nil
REDACTED
	return s.deleteIDs(ctx, ids)
REDACTED
func (s *fakePromptAdminService) PreviewDelete(ctx context.Context, filter EventFilter, actorID int64) (*DeletePreview, error) {
	if s.preview == nil {
		return &DeletePreview{REDACTED, nil
REDACTED
	return s.preview(ctx, filter, actorID)
REDACTED
func (s *fakePromptAdminService) DeleteByFilter(ctx context.Context, req DeleteByFilterRequest, actorID int64) (*DeleteResult, error) {
	if s.deleteFilter == nil {
		return &DeleteResult{REDACTED, nil
REDACTED
	return s.deleteFilter(ctx, req, actorID)
REDACTED

func promptAdminRouter(service PromptAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42REDACTED)
		c.Set(string(servermiddleware.ContextKeyUserRole), "admin")
		c.Next()
REDACTED)
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
	return router
REDACTED

func promptAdminRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
REDACTED
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
REDACTED else {
		raw, err := json.Marshal(body)
	REDACTED
		reader = bytes.NewReader(raw)
REDACTED
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
REDACTED

func TestPromptAdminConfigRequiresVersionMapsConflictAndNeverEchoesToken(t *testing.T) {
	const canary = "prompt-admin-token-canary"

	t.Run("missing expected version", func(t *testing.T) {
		router := promptAdminRouter(&fakePromptAdminService{REDACTED)
		response := promptAdminRequest(t, router, http.MethodPut, "/admin/prompt-audit/config", map[string]any{REDACTED)
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, response.Body.String(), "prompt_audit_invalid_config_request")
REDACTED)

	t.Run("CAS conflict", func(t *testing.T) {
		service := &fakePromptAdminService{save: func(context.Context, UpdateConfigRequest, int64) (PublicConfig, error) {
			return PublicConfig{REDACTED, infraerrors.Conflict(ErrorCodeConfigConflict, "配置已被更新")
	REDACTEDREDACTED
		response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPut, "/admin/prompt-audit/config", validHandlerUpdateRequest(canary))
		require.Equal(t, http.StatusConflict, response.Code)
		require.Contains(t, response.Body.String(), ErrorCodeConfigConflict)
		require.NotContains(t, response.Body.String(), canary)
REDACTED)

	t.Run("success public DTO", func(t *testing.T) {
		service := &fakePromptAdminService{save: func(_ context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
			require.Equal(t, int64(42), actorID)
			require.Equal(t, canary, req.Endpoints[0].Token)
			return PublicConfig{ConfigVersion: 8, Endpoints: []PublicEndpoint{{ID: "guard-1", HasToken: true, TokenStatus: "configured"REDACTEDREDACTEDREDACTED, nil
	REDACTEDREDACTED
		response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPut, "/admin/prompt-audit/config", validHandlerUpdateRequest(canary))
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		require.NotContains(t, body, canary)
		require.NotContains(t, body, "token_ciphertext")
		require.NotContains(t, body, `"token":`)
		require.Contains(t, body, `"has_token":true`)
REDACTED)
REDACTED

func TestPromptAdminGetConfigReturnsSecretFreeUnavailableError(t *testing.T) {
	const canary = "persisted-config-secret-canary"
	repository := &switchableSettingRepository{loadErr: errors.New("failed to load token " + canary)REDACTED
	manager := NewConfigManager(nil, repository, nil, prefixEncryptor{REDACTED)
	require.Error(t, manager.Reload(context.Background()))
	service := &PromptService{config: managerREDACTED

	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodGet, "/admin/prompt-audit/config", nil)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), ErrorCodeConfigUnavailable)
	require.NotContains(t, response.Body.String(), canary)
	require.NotContains(t, response.Body.String(), `"config_version"`)
	require.NotContains(t, response.Body.String(), `"token"`)
REDACTED

func TestPromptAdminProbeSupportsTemporaryOrSavedTokenWithoutEcho(t *testing.T) {
	const canary = "probe-token-canary"
	for _, tc := range []struct {
		name         string
		token        string
		tokenApplied bool
REDACTED{
		{name: "temporary token", token: canary, tokenApplied: trueREDACTED,
		{name: "saved token", token: "", tokenApplied: trueREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakePromptAdminService{probe: func(_ context.Context, req ProbeRequest) ProbeResult {
				require.Equal(t, tc.token, req.Endpoint.Token)
				return ProbeResult{OK: true, Status: "healthy", Message: "ok", TokenApplied: tc.tokenAppliedREDACTED
		REDACTEDREDACTED
			endpoint := validHandlerUpdateRequest(tc.token).Endpoints[0]
			response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPost, "/admin/prompt-audit/endpoints/probe", ProbeRequest{Endpoint: endpointREDACTED)
			require.Equal(t, http.StatusOK, response.Code)
			require.NotContains(t, response.Body.String(), canary)
			require.NotContains(t, response.Body.String(), `"token":`)
			require.Contains(t, response.Body.String(), `"token_applied":true`)
	REDACTED)
REDACTED
REDACTED

func TestPromptAdminRejectsInvalidEventIDsTimesAndPagination(t *testing.T) {
	router := promptAdminRouter(&fakePromptAdminService{REDACTED)
	for _, tc := range []struct {
		method string
		path   string
		body   any
		reason string
REDACTED{
		{http.MethodGet, "/admin/prompt-audit/events/not-a-number", nil, "prompt_audit_invalid_event_id"REDACTED,
		{http.MethodDelete, "/admin/prompt-audit/events/-1", nil, "prompt_audit_invalid_event_id"REDACTED,
		{http.MethodGet, "/admin/prompt-audit/events?group_id=bad", nil, "prompt_audit_invalid_filter_id"REDACTED,
		{http.MethodGet, "/admin/prompt-audit/events?start_at=not-time", nil, "prompt_audit_invalid_time"REDACTED,
		{http.MethodGet, "/admin/prompt-audit/events?page=0", nil, "prompt_audit_invalid_pagination"REDACTED,
		{http.MethodPost, "/admin/prompt-audit/events/batch-delete", map[string]any{"ids": []int64{1, -2REDACTEDREDACTED, "prompt_audit_invalid_event_id"REDACTED,
REDACTED {
		response := promptAdminRequest(t, router, tc.method, tc.path, tc.body)
		require.Equalf(t, http.StatusBadRequest, response.Code, "%s %s", tc.method, tc.path)
		require.Contains(t, response.Body.String(), tc.reason)
REDACTED
REDACTED

func validHandlerUpdateRequest(token string) UpdateConfigRequest {
	return UpdateConfigRequest{
		ExpectedConfigVersion: 7,
		Strategy:              "priority",
		WorkerCount:           1,
		QueueCapacity:         10,
		Scanners:              []string{"pii"REDACTED,
		AllGroups:             true,
		Endpoints: []UpdateEndpoint{{
			ID: "guard-1", Name: "Guard One", Protocol: "openai_compatible",
			BaseURL: "http://127.0.0.1:18080", Model: DefaultGuardModel, Token: token,
			TimeoutMS: 1000, InputLimit: 1024, Enabled: true,
REDACTED
REDACTED
REDACTED

func TestPromptAdminDeleteConfirmationErrorsStayGeneric(t *testing.T) {
	service := &fakePromptAdminService{deleteFilter: func(context.Context, DeleteByFilterRequest, int64) (*DeleteResult, error) {
		return nil, errors.New("sensitive-token-or-filter-detail")
REDACTEDREDACTED
	response := promptAdminRequest(t, promptAdminRouter(service), http.MethodPost, "/admin/prompt-audit/events/delete-by-filter", DeleteByFilterRequest{
		SnapshotMaxID: 3, FilterHash: strings.Repeat("a", 64), ConfirmationToken: "secret-confirmation", Confirm: true,
REDACTED)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "prompt_audit_delete_confirmation_invalid")
	require.NotContains(t, response.Body.String(), "sensitive-token")
	require.NotContains(t, response.Body.String(), "secret-confirmation")
REDACTED
