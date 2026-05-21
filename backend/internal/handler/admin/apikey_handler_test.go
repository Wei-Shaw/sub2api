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
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAPIKeyHandler(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewAdminAPIKeyHandler(adminSvc)
	router.PUT("/api/v1/admin/api-keys/:id", h.UpdateGroup)
	return router
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
