package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountMixedChannelRouter(adminSvc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	accountHandler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/check-mixed-channel", accountHandler.CheckMixedChannel)
	router.POST("/api/v1/admin/accounts", accountHandler.Create)
	router.PUT("/api/v1/admin/accounts/:id", accountHandler.Update)
	router.POST("/api/v1/admin/accounts/bulk-update", accountHandler.BulkUpdate)
	return router
REDACTED

func TestAccountHandlerCheckMixedChannelNoRisk(t *testing.T) {
	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"platform":  "antigravity",
		"group_ids": []int64{27REDACTED,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/check-mixed-channel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["has_risk"])
	require.Equal(t, int64(0), adminSvc.lastMixedCheck.accountID)
	require.Equal(t, "antigravity", adminSvc.lastMixedCheck.platform)
	require.Equal(t, []int64{27REDACTED, adminSvc.lastMixedCheck.groupIDs)
REDACTED

func TestAccountHandlerCheckMixedChannelWithRisk(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.checkMixedErr = &service.MixedChannelError{
		GroupID:         27,
		GroupName:       "claude-max",
		CurrentPlatform: "Antigravity",
		OtherPlatform:   "Anthropic",
REDACTED
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"platform":   "antigravity",
		"group_ids":  []int64{27REDACTED,
		"account_id": 99,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/check-mixed-channel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["has_risk"])
	require.Equal(t, "mixed_channel_warning", data["error"])
	details, ok := data["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(27), details["group_id"])
	require.Equal(t, "claude-max", details["group_name"])
	require.Equal(t, "Antigravity", details["current_platform"])
	require.Equal(t, "Anthropic", details["other_platform"])
	require.Equal(t, int64(99), adminSvc.lastMixedCheck.accountID)
REDACTED

func TestAccountHandlerCreateMixedChannelConflictSimplifiedResponse(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.createAccountErr = &service.MixedChannelError{
		GroupID:         27,
		GroupName:       "claude-max",
		CurrentPlatform: "Antigravity",
		OtherPlatform:   "Anthropic",
REDACTED
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"name":        "ag-oauth-1",
		"platform":    "antigravity",
		"type":        "oauth",
		"credentials": map[string]any{"refresh_token": "rt"REDACTED,
		"group_ids":   []int64{27REDACTED,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "mixed_channel_warning", resp["error"])
	require.Contains(t, resp["message"], "mixed_channel_warning")
	_, hasDetails := resp["details"]
	_, hasRequireConfirmation := resp["require_confirmation"]
	require.False(t, hasDetails)
	require.False(t, hasRequireConfirmation)
REDACTED

func TestAccountHandlerUpdateMixedChannelConflictSimplifiedResponse(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.updateAccountErr = &service.MixedChannelError{
		GroupID:         27,
		GroupName:       "claude-max",
		CurrentPlatform: "Antigravity",
		OtherPlatform:   "Anthropic",
REDACTED
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"group_ids": []int64{27REDACTED,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/3", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "mixed_channel_warning", resp["error"])
	require.Contains(t, resp["message"], "mixed_channel_warning")
	_, hasDetails := resp["details"]
	_, hasRequireConfirmation := resp["require_confirmation"]
	require.False(t, hasDetails)
	require.False(t, hasRequireConfirmation)
REDACTED

func TestAccountHandlerBulkUpdateMixedChannelConflict(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.bulkUpdateAccountErr = &service.MixedChannelError{
		GroupID:         27,
		GroupName:       "claude-max",
		CurrentPlatform: "Antigravity",
		OtherPlatform:   "Anthropic",
REDACTED
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1, 2, 3REDACTED,
		"group_ids":   []int64{27REDACTED,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "mixed_channel_warning", resp["error"])
	require.Contains(t, resp["message"], "claude-max")
REDACTED

func TestAccountHandlerBulkUpdateMixedChannelConfirmSkips(t *testing.T) {
	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"account_ids":                []int64{1, 2REDACTED,
		"group_ids":                  []int64{27REDACTED,
		"confirm_mixed_channel_risk": true,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2), data["success"])
	require.Equal(t, float64(0), data["failed"])
REDACTED

func TestBulkUpdateAcceptsFilterTargetRequest(t *testing.T) {
	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"filters": map[string]any{
			"platform":     "openai",
			"type":         "oauth",
			"status":       "active",
			"group":        "12",
			"privacy_mode": "blocked",
			"search":       "bulk-target",
	REDACTED,
		"schedulable": true,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])
REDACTED

func TestBulkUpdateAcceptsDedicatedUpstreamBillingProbeSetting(t *testing.T) {
	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)

	body, _ := json.Marshal(map[string]any{
		"account_ids":                    []int64{1, 2REDACTED,
		"upstream_billing_probe_enabled": false,
REDACTED)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, adminSvc.lastBulkUpdateAccountInput)
	require.NotNil(t, adminSvc.lastBulkUpdateAccountInput.ProbeEnabled)
	require.False(t, *adminSvc.lastBulkUpdateAccountInput.ProbeEnabled)
REDACTED
