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

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAPIKeyHandler(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewAdminAPIKeyHandler(adminSvc, nil)
	router.PUT("/api/v1/admin/api-keys/:id", h.UpdateGroup)
	return router
}

type stubUserAPIKeyService struct {
	keys               map[int64]*service.APIKey
	createdReq         service.CreateAPIKeyRequest
	createCalls        int
	updatedReq         service.UpdateAPIKeyRequest
	updateCalls        int
	deleteCalls        int
	rejectGroupBinding bool
}

func newStubUserAPIKeyService() *stubUserAPIKeyService {
	now := time.Now().UTC()
	return &stubUserAPIKeyService{keys: map[int64]*service.APIKey{
		10: {ID: 10, UserID: 1, Key: "sk-secret-1234567890", Name: "test", Status: service.StatusActive, CreatedAt: now, UpdatedAt: now},
	}}
}

func (s *stubUserAPIKeyService) Create(_ context.Context, userID int64, req service.CreateAPIKeyRequest) (*service.APIKey, error) {
	s.createCalls++
	s.createdReq = req
	key := &service.APIKey{ID: 11, UserID: userID, Key: "sk-created-1234567890", Name: req.Name, Status: service.StatusActive, ExpiresAt: req.ExpiresAt}
	s.keys[key.ID] = key
	return key, nil
}

func (s *stubUserAPIKeyService) List(_ context.Context, userID int64, params pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	keys := make([]service.APIKey, 0)
	for _, key := range s.keys {
		if key.UserID == userID {
			keys = append(keys, *key)
		}
	}
	return keys, &pagination.PaginationResult{Total: int64(len(keys)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *stubUserAPIKeyService) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	key, ok := s.keys[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	copy := *key
	return &copy, nil
}

func (s *stubUserAPIKeyService) Update(_ context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
	s.updateCalls++
	s.updatedReq = req
	if s.rejectGroupBinding && req.GroupID != nil {
		return nil, service.ErrGroupNotAllowed
	}
	key, ok := s.keys[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	if key.UserID != userID {
		return nil, service.ErrInsufficientPerms
	}
	if req.Name != nil {
		key.Name = *req.Name
	}
	if req.CustomKey != nil {
		key.Key = *req.CustomKey
	}
	if req.ClearGroup {
		key.GroupID = nil
	} else if req.GroupID != nil {
		groupID := *req.GroupID
		key.GroupID = &groupID
	}
	copy := *key
	return &copy, nil
}

func (s *stubUserAPIKeyService) Delete(_ context.Context, id, userID int64) error {
	s.deleteCalls++
	key, ok := s.keys[id]
	if !ok {
		return service.ErrAPIKeyNotFound
	}
	if key.UserID != userID {
		return service.ErrInsufficientPerms
	}
	delete(s.keys, id)
	return nil
}

func (s *stubUserAPIKeyService) ValidateCustomKeyPrefix(key string) error {
	if len(key) < 3 || key[:3] != "sk-" {
		return service.ErrAPIKeyInvalidPrefix
	}
	return nil
}

func setupFullAPIKeyHandler(adminSvc service.AdminService, apiKeySvc adminAPIKeyService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewAdminAPIKeyHandler(adminSvc, nil)
	h.apiKeyService = apiKeySvc
	router.POST("/api/v1/admin/users/:user_id/api-keys", h.Create)
	router.GET("/api/v1/admin/users/:user_id/api-keys", h.List)
	router.GET("/api/v1/admin/users/:user_id/api-keys/:key_id", h.Get)
	router.PUT("/api/v1/admin/users/:user_id/api-keys/:key_id", h.Update)
	router.DELETE("/api/v1/admin/users/:user_id/api-keys/:key_id", h.Delete)
	return router
}

type recordingAPIKeyAdminService struct {
	*stubAdminService
	groupUpdateCalls int
	groupUpdateErr   error
}

func (s *recordingAPIKeyAdminService) AdminUpdateAPIKeyGroupID(_ context.Context, keyID int64, groupID *int64) (*service.AdminUpdateAPIKeyGroupIDResult, error) {
	s.groupUpdateCalls++
	if s.groupUpdateErr != nil {
		return nil, s.groupUpdateErr
	}
	key := service.APIKey{ID: keyID, UserID: 1, Key: "sk-created-1234567890", Name: "test", Status: service.StatusActive}
	if groupID != nil && *groupID > 0 {
		gid := *groupID
		key.GroupID = &gid
	}
	return &service.AdminUpdateAPIKeyGroupIDResult{APIKey: &key, AutoGrantedGroupAccess: true}, nil
}

func TestAdminAPIKeyHandler_CreateForUser_ReturnsCredentialAndExplicitExpiry(t *testing.T) {
	svc := newStubUserAPIKeyService()
	router := setupFullAPIKeyHandler(newStubAdminService(), svc)
	expiresAt := "2026-08-26T23:59:59+08:00"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/api-keys", bytes.NewBufferString(`{"name":"PRO","custom_key":"sk-custom-1234567890","quota":0,"rate_limit_5h":0,"expires_at":"`+expiresAt+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"key":"sk-created-1234567890"`)
	require.NotNil(t, svc.createdReq.ExpiresAt)
	require.Equal(t, expiresAt, svc.createdReq.ExpiresAt.Format(time.RFC3339))
	require.Zero(t, svc.createdReq.Quota)
	require.Zero(t, svc.createdReq.RateLimit5h)
}

func TestAdminAPIKeyHandler_CreateForUser_RejectsExpiryConflict(t *testing.T) {
	router := setupFullAPIKeyHandler(newStubAdminService(), newStubUserAPIKeyService())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/api-keys", bytes.NewBufferString(`{"name":"PRO","expires_in_days":30,"expires_at":"2026-08-26T23:59:59+08:00"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "API_KEY_EXPIRY_CONFLICT")
}

func TestAdminAPIKeyHandler_CreateForUser_BindsGroupThroughAdminService(t *testing.T) {
	apiKeySvc := newStubUserAPIKeyService()
	adminSvc := &recordingAPIKeyAdminService{stubAdminService: newStubAdminService()}
	router := setupFullAPIKeyHandler(adminSvc, apiKeySvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/api-keys", bytes.NewBufferString(`{"name":"PRO","group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, adminSvc.groupUpdateCalls)
	require.Nil(t, apiKeySvc.createdReq.GroupID, "group binding must use the admin grant path")
	require.Contains(t, rec.Body.String(), `"group_id":2`)
}

func TestAdminAPIKeyHandler_CreateForUser_RollsBackWhenGroupBindingFails(t *testing.T) {
	apiKeySvc := newStubUserAPIKeyService()
	adminSvc := &recordingAPIKeyAdminService{
		stubAdminService: newStubAdminService(),
		groupUpdateErr:   service.ErrGroupNotAllowed,
	}
	router := setupFullAPIKeyHandler(adminSvc, apiKeySvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/api-keys", bytes.NewBufferString(`{"name":"PRO","group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, 1, apiKeySvc.deleteCalls)
	_, exists := apiKeySvc.keys[11]
	require.False(t, exists)
}

func TestAdminAPIKeyHandler_CreateForUser_IdempotencyReplayDoesNotCreateDuplicate(t *testing.T) {
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(
		newMemoryIdempotencyRepoStub(),
		service.DefaultIdempotencyConfig(),
	))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	apiKeySvc := newStubUserAPIKeyService()
	router := setupFullAPIKeyHandler(newStubAdminService(), apiKeySvc)
	doRequest := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/api-keys", bytes.NewBufferString(`{"name":"PRO"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "token-shop-key-1")
		router.ServeHTTP(rec, req)
		return rec
	}

	first := doRequest()
	second := doRequest()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, apiKeySvc.createCalls)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
	require.Contains(t, second.Body.String(), `"key":"sk-created-1234567890"`)
}

func TestAdminAPIKeyHandler_ReadResponsesHideCredential(t *testing.T) {
	router := setupFullAPIKeyHandler(newStubAdminService(), newStubUserAPIKeyService())

	for _, path := range []string{
		"/api/v1/admin/users/1/api-keys",
		"/api/v1/admin/users/1/api-keys/10",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.NotContains(t, rec.Body.String(), "sk-secret-1234567890")
		require.Contains(t, rec.Body.String(), `"key":"sk-sec****7890"`)
	}
}

func TestAdminAPIKeyHandler_UpdateRejectsCrossUserKey(t *testing.T) {
	svc := newStubUserAPIKeyService()
	router := setupFullAPIKeyHandler(newStubAdminService(), svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/2/api-keys/10", bytes.NewBufferString(`{"name":"other"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Zero(t, svc.updateCalls)
}

func TestAdminAPIKeyHandler_UpdateForUser_MapsAllManagedFields(t *testing.T) {
	svc := newStubUserAPIKeyService()
	router := setupFullAPIKeyHandler(newStubAdminService(), svc)
	rec := httptest.NewRecorder()
	body := `{"name":"PRO","group_id":2,"custom_key":"sk-replaced-1234567890","status":"inactive","ip_whitelist":[],"ip_blacklist":["10.0.0.0/8"],"quota":200,"reset_quota":true,"rate_limit_5h":20,"rate_limit_1d":50,"rate_limit_7d":200,"reset_rate_limit_usage":true,"expires_in_days":30}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, svc.updateCalls)
	require.Equal(t, "PRO", *svc.updatedReq.Name)
	require.Equal(t, int64(2), *svc.updatedReq.GroupID)
	require.Equal(t, "sk-replaced-1234567890", *svc.updatedReq.CustomKey)
	require.Equal(t, "inactive", *svc.updatedReq.Status)
	require.Empty(t, *svc.updatedReq.IPWhitelist)
	require.Equal(t, []string{"10.0.0.0/8"}, *svc.updatedReq.IPBlacklist)
	require.Equal(t, 200.0, *svc.updatedReq.Quota)
	require.True(t, *svc.updatedReq.ResetQuota)
	require.Equal(t, 20.0, *svc.updatedReq.RateLimit5h)
	require.Equal(t, 50.0, *svc.updatedReq.RateLimit1d)
	require.Equal(t, 200.0, *svc.updatedReq.RateLimit7d)
	require.True(t, *svc.updatedReq.ResetRateLimitUsage)
	require.WithinDuration(t, time.Now().AddDate(0, 0, 30), *svc.updatedReq.ExpiresAt, 2*time.Second)
	require.Contains(t, rec.Body.String(), `"key":"sk-rep****7890"`)
}

func TestAdminAPIKeyHandler_UpdateForUser_AutoGrantsExclusiveGroup(t *testing.T) {
	apiKeySvc := newStubUserAPIKeyService()
	apiKeySvc.rejectGroupBinding = true
	adminSvc := &recordingAPIKeyAdminService{stubAdminService: newStubAdminService()}
	router := setupFullAPIKeyHandler(adminSvc, apiKeySvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1/api-keys/10", bytes.NewBufferString(`{"name":"PRO","group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, adminSvc.groupUpdateCalls)
	require.Equal(t, 2, apiKeySvc.updateCalls)
	require.Nil(t, apiKeySvc.updatedReq.GroupID)
}

func TestAdminAPIKeyHandler_UpdateForUser_CanClearGroup(t *testing.T) {
	apiKeySvc := newStubUserAPIKeyService()
	groupID := int64(2)
	apiKeySvc.keys[10].GroupID = &groupID
	router := setupFullAPIKeyHandler(newStubAdminService(), apiKeySvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1/api-keys/10", bytes.NewBufferString(`{"group_id":0}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, apiKeySvc.updatedReq.ClearGroup)
	require.Nil(t, apiKeySvc.keys[10].GroupID)
}

func TestAdminAPIKeyHandler_DeleteRejectsCrossUserKey(t *testing.T) {
	svc := newStubUserAPIKeyService()
	router := setupFullAPIKeyHandler(newStubAdminService(), svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/2/api-keys/10", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Zero(t, svc.deleteCalls)
}

func TestAdminAPIKeyHandler_UpdateGroup_InvalidID(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/abc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid API key ID")
}

func TestAdminAPIKeyHandler_UpdateGroup_InvalidJSON(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid request")
}

func TestAdminAPIKeyHandler_UpdateGroup_KeyNotFound(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	// ErrAPIKeyNotFound maps to 404
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAdminAPIKeyHandler_UpdateGroup_BindGroup(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	var data struct {
		APIKey struct {
			ID      int64  `json:"id"`
			GroupID *int64 `json:"group_id"`
		} `json:"api_key"`
		AutoGrantedGroupAccess bool `json:"auto_granted_group_access"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Equal(t, int64(10), data.APIKey.ID)
	require.NotNil(t, data.APIKey.GroupID)
	require.Equal(t, int64(2), *data.APIKey.GroupID)
}

func TestAdminAPIKeyHandler_UpdateGroup_Unbind(t *testing.T) {
	svc := newStubAdminService()
	gid := int64(2)
	svc.apiKeys[0].GroupID = &gid
	router := setupAPIKeyHandler(svc)
	body := `{"group_id": 0}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			APIKey struct {
				GroupID *int64 `json:"group_id"`
			} `json:"api_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Nil(t, resp.Data.APIKey.GroupID)
}

func TestAdminAPIKeyHandler_ResetRateLimitUsage(t *testing.T) {
	svc := newStubAdminService()
	now := time.Now()
	svc.apiKeys[0].Usage5h = 1.2
	svc.apiKeys[0].Usage1d = 3.4
	svc.apiKeys[0].Usage7d = 5.6
	svc.apiKeys[0].Window5hStart = &now
	svc.apiKeys[0].Window1dStart = &now
	svc.apiKeys[0].Window7dStart = &now
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"reset_rate_limit_usage":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			APIKey struct {
				Usage5h       float64    `json:"usage_5h"`
				Usage1d       float64    `json:"usage_1d"`
				Usage7d       float64    `json:"usage_7d"`
				Window5hStart *time.Time `json:"window_5h_start"`
				Window1dStart *time.Time `json:"window_1d_start"`
				Window7dStart *time.Time `json:"window_7d_start"`
			} `json:"api_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Zero(t, resp.Data.APIKey.Usage5h)
	require.Zero(t, resp.Data.APIKey.Usage1d)
	require.Zero(t, resp.Data.APIKey.Usage7d)
	require.Nil(t, resp.Data.APIKey.Window5hStart)
	require.Nil(t, resp.Data.APIKey.Window1dStart)
	require.Nil(t, resp.Data.APIKey.Window7dStart)
}

func TestAdminAPIKeyHandler_UpdateGroup_ServiceError(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              errors.New("internal failure"),
	}
	router := setupAPIKeyHandler(svc)
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// H2: empty body → group_id is nil → no-op, returns original key
func TestAdminAPIKeyHandler_UpdateGroup_EmptyBody_NoChange(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			APIKey struct {
				ID int64 `json:"id"`
			} `json:"api_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, int64(10), resp.Data.APIKey.ID)
}

// M2: service returns GROUP_NOT_ACTIVE → handler maps to 400
func TestAdminAPIKeyHandler_UpdateGroup_GroupNotActive(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              infraerrors.BadRequest("GROUP_NOT_ACTIVE", "target group is not active"),
	}
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"group_id": 5}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "GROUP_NOT_ACTIVE")
}

// M2: service returns INVALID_GROUP_ID → handler maps to 400
func TestAdminAPIKeyHandler_UpdateGroup_NegativeGroupID(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be non-negative"),
	}
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"group_id": -5}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "INVALID_GROUP_ID")
}

// failingUpdateGroupService overrides AdminUpdateAPIKeyGroupID to return an error.
type failingUpdateGroupService struct {
	*stubAdminService
	err error
}

func (f *failingUpdateGroupService) AdminUpdateAPIKeyGroupID(_ context.Context, _ int64, _ *int64) (*service.AdminUpdateAPIKeyGroupIDResult, error) {
	return nil, f.err
}
