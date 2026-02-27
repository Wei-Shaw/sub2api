package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
REDACTED

func TestAdminAPIKeyHandler_UpdateGroup_InvalidID(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2REDACTED`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/abc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid API key ID")
REDACTED

func TestAdminAPIKeyHandler_UpdateGroup_InvalidJSON(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid request")
REDACTED

func TestAdminAPIKeyHandler_UpdateGroup_KeyNotFound(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2REDACTED`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	// ErrAPIKeyNotFound maps to 404
	require.Equal(t, http.StatusNotFound, rec.Code)
REDACTED

func TestAdminAPIKeyHandler_UpdateGroup_BindGroup(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2REDACTED`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	var apiKey struct {
		ID      int64  `json:"id"`
		GroupID *int64 `json:"group_id"`
REDACTED
	require.NoError(t, json.Unmarshal(resp.Data, &apiKey))
	require.Equal(t, int64(10), apiKey.ID)
	require.NotNil(t, apiKey.GroupID)
	require.Equal(t, int64(2), *apiKey.GroupID)
REDACTED

func TestAdminAPIKeyHandler_UpdateGroup_Unbind(t *testing.T) {
	svc := newStubAdminService()
	gid := int64(2)
	svc.apiKeys[0].GroupID = &gid
	router := setupAPIKeyHandler(svc)
	body := `{"group_id": 0REDACTED`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			GroupID *int64 `json:"group_id"`
	REDACTED `json:"data"`
REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Nil(t, resp.Data.GroupID)
REDACTED

func TestAdminAPIKeyHandler_UpdateGroup_ServiceError(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              errors.New("internal failure"),
REDACTED
	router := setupAPIKeyHandler(svc)
	body := `{"group_id": 2REDACTED`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
REDACTED

// H2: empty body → group_id is nil → no-op, returns original key
func TestAdminAPIKeyHandler_UpdateGroup_EmptyBody_NoChange(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ID int64 `json:"id"`
	REDACTED `json:"data"`
REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, int64(10), resp.Data.ID)
REDACTED

// M2: service returns GROUP_NOT_ACTIVE → handler maps to 400
func TestAdminAPIKeyHandler_UpdateGroup_GroupNotActive(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              infraerrors.BadRequest("GROUP_NOT_ACTIVE", "target group is not active"),
REDACTED
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"group_id": 5REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "GROUP_NOT_ACTIVE")
REDACTED

// M2: service returns INVALID_GROUP_ID → handler maps to 400
func TestAdminAPIKeyHandler_UpdateGroup_NegativeGroupID(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be non-negative"),
REDACTED
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"group_id": -5REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "INVALID_GROUP_ID")
REDACTED

// failingUpdateGroupService overrides AdminUpdateAPIKeyGroupID to return an error.
type failingUpdateGroupService struct {
	*stubAdminService
	err error
REDACTED

func (f *failingUpdateGroupService) AdminUpdateAPIKeyGroupID(_ context.Context, _ int64, _ *int64) (*service.APIKey, error) {
	return nil, f.err
REDACTED
