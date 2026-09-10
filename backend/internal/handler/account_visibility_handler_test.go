package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountVisibilityServiceStub struct {
	accounts []*service.Account
	result   *pagination.PaginationResult
	visible  bool
}

func (s *accountVisibilityServiceStub) ListVisibleAccounts(context.Context, int64, pagination.PaginationParams, service.AccountVisibilityFilters) ([]*service.Account, *pagination.PaginationResult, error) {
	return s.accounts, s.result, nil
}

func (s *accountVisibilityServiceStub) CanViewAccount(context.Context, int64, int64) (bool, error) {
	return s.visible, nil
}

func (s *accountVisibilityServiceStub) ListVisibleGroups(context.Context, int64) ([]service.AccountVisibilityGroup, error) {
	return []service.AccountVisibilityGroup{}, nil
}

func (s *accountVisibilityServiceStub) ListAccountVisibleUsers(context.Context, int64) ([]service.AccountVisibleUser, error) {
	return []service.AccountVisibleUser{}, nil
}

func (s *accountVisibilityServiceStub) ReplaceAccountVisibleUsers(context.Context, int64, []int64) error {
	return nil
}

func TestAccountVisibilityListReturnsSafeProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	h := &AccountVisibilityHandler{service: &accountVisibilityServiceStub{
		accounts: []*service.Account{{
			ID: 7, Name: "shared-account", Platform: service.PlatformOpenAI, Type: "oauth",
			Credentials:  map[string]any{"access_token": "top-secret"},
			Extra:        map[string]any{"private_setting": "hidden"},
			ErrorMessage: "sensitive upstream error", Status: service.StatusActive,
			Schedulable: true, Concurrency: 3, Priority: 50, CreatedAt: now,
		}},
		result: &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 20, Pages: 1},
	}}

	router := gin.New()
	router.GET("/accounts", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 19})
		c.Next()
	}, h.List)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "top-secret")
	require.NotContains(t, recorder.Body.String(), "private_setting")
	require.NotContains(t, recorder.Body.String(), "sensitive upstream error")

	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "shared-account", payload.Data.Items[0]["name"])
	_, hasCredentials := payload.Data.Items[0]["credentials"]
	require.False(t, hasCredentials)
}

func TestAccountVisibilityUpdateRejectsNonArrayUserIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountVisibilityHandler{service: &accountVisibilityServiceStub{}}
	router := gin.New()
	router.PUT("/accounts/:id/visible-users", h.UpdateAccountVisibleUsers)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/7/visible-users", strings.NewReader(`{"user_ids":"19"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestRequireVisibleAccountHidesUnmappedAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountVisibilityHandler{service: &accountVisibilityServiceStub{visible: false}}
	nextCalled := false
	router := gin.New()
	router.GET("/accounts/:id", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 19})
		c.Next()
	}, h.RequireVisibleAccount(), func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/accounts/7", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.False(t, nextCalled)
}
