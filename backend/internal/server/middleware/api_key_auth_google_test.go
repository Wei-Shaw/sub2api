package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeApiKeyRepo struct {
	getByKey func(ctx context.Context, key string) (*service.ApiKey, error)
REDACTED

func (f fakeApiKeyRepo) Create(ctx context.Context, key *service.ApiKey) error {
	return errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.ApiKey, error) {
	return nil, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) GetOwnerID(ctx context.Context, id int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.ApiKey, error) {
	if f.getByKey == nil {
		return nil, errors.New("unexpected call")
REDACTED
	return f.getByKey(ctx, key)
REDACTED
func (f fakeApiKeyRepo) Update(ctx context.Context, key *service.ApiKey) error {
	return errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.ApiKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	return false, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.ApiKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.ApiKey, error) {
	return nil, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED
func (f fakeApiKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

type googleErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
REDACTED `json:"error"`
REDACTED

func newTestApiKeyService(repo service.ApiKeyRepository) *service.ApiKeyService {
	return service.NewApiKeyService(
		repo,
		nil, // userRepo (unused in GetByKey)
		nil, // groupRepo
		nil, // userSubRepo
		nil, // cache
		&config.Config{REDACTED,
	)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_MissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return nil, errors.New("should not be called")
	REDACTED,
REDACTED)
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{REDACTED))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Error.Code)
	require.Equal(t, "API key is required", resp.Error.Message)
	require.Equal(t, "UNAUTHENTICATED", resp.Error.Status)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_QueryApiKeyRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return nil, errors.New("should not be called")
	REDACTED,
REDACTED)
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{REDACTED))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test?api_key=legacy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusBadRequest, resp.Error.Code)
	require.Equal(t, "Query parameter api_key is deprecated. Use Authorization header or key instead.", resp.Error.Message)
	require.Equal(t, "INVALID_ARGUMENT", resp.Error.Status)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_QueryKeyAllowedOnV1Beta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return &service.ApiKey{
				ID:     1,
				Key:    key,
				Status: service.StatusActive,
				User: &service.User{
					ID:     123,
					Status: service.StatusActive,
			REDACTED,
		REDACTED, nil
	REDACTED,
REDACTED)
	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test?key=valid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_InvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return nil, service.ErrApiKeyNotFound
	REDACTED,
REDACTED)
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{REDACTED))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Error.Code)
	require.Equal(t, "Invalid API key", resp.Error.Message)
	require.Equal(t, "UNAUTHENTICATED", resp.Error.Status)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_RepoError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return nil, errors.New("db down")
	REDACTED,
REDACTED)
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{REDACTED))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Error.Code)
	require.Equal(t, "Failed to validate API key", resp.Error.Message)
	require.Equal(t, "INTERNAL", resp.Error.Status)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_DisabledKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return &service.ApiKey{
				ID:     1,
				Key:    key,
				Status: service.StatusDisabled,
				User: &service.User{
					ID:     123,
					Status: service.StatusActive,
			REDACTED,
		REDACTED, nil
	REDACTED,
REDACTED)
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{REDACTED))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer disabled")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Error.Code)
	require.Equal(t, "API key is disabled", resp.Error.Message)
	require.Equal(t, "UNAUTHENTICATED", resp.Error.Status)
REDACTED

func TestApiKeyAuthWithSubscriptionGoogle_InsufficientBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestApiKeyService(fakeApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.ApiKey, error) {
			return &service.ApiKey{
				ID:     1,
				Key:    key,
				Status: service.StatusActive,
				User: &service.User{
					ID:      123,
					Status:  service.StatusActive,
					Balance: 0,
			REDACTED,
		REDACTED, nil
	REDACTED,
REDACTED)
	r.Use(ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{REDACTED))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": trueREDACTED) REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusForbidden, resp.Error.Code)
	require.Equal(t, "Insufficient account balance", resp.Error.Message)
	require.Equal(t, "PERMISSION_DENIED", resp.Error.Status)
REDACTED
