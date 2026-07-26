//go:build unit

package admin

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminCustomDomainRepo struct {
	domain *service.CustomDomain
}

func (r *adminCustomDomainRepo) Create(_ context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	domain.ID = 1
	copy := *domain
	r.domain = &copy
	return &copy, nil
}
func (r *adminCustomDomainRepo) GetByID(context.Context, int64) (*service.CustomDomain, error) {
	if r.domain == nil {
		return nil, service.ErrCustomDomainNotFound
	}
	copy := *r.domain
	return &copy, nil
}
func (r *adminCustomDomainRepo) GetByDomain(context.Context, string) (*service.CustomDomain, error) {
	return r.GetByID(context.Background(), 1)
}
func (r *adminCustomDomainRepo) ListByUserID(context.Context, int64) ([]service.CustomDomain, error) {
	if r.domain == nil {
		return nil, nil
	}
	return []service.CustomDomain{*r.domain}, nil
}
func (r *adminCustomDomainRepo) ListAll(context.Context, service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	if r.domain == nil {
		return nil, nil
	}
	return []service.CustomDomain{*r.domain}, nil
}
func (r *adminCustomDomainRepo) Update(_ context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	copy := *domain
	r.domain = &copy
	return &copy, nil
}
func (r *adminCustomDomainRepo) Delete(context.Context, int64) error {
	r.domain = nil
	return nil
}
func (r *adminCustomDomainRepo) SetAccess(_ context.Context, _ int64, allUsers bool, userIDs []int64) (*service.CustomDomain, error) {
	r.domain.AllUsers = allUsers
	r.domain.AuthorizedUserIDs = append([]int64(nil), userIDs...)
	return r.domain, nil
}

type adminCustomDomainSettings map[string]string

type adminCustomDomainDNS struct {
	repo *adminCustomDomainRepo
}

func (d adminCustomDomainDNS) LookupTXT(context.Context, string) ([]string, error) {
	if d.repo != nil && d.repo.domain != nil {
		return []string{d.repo.domain.VerificationTXTValue}, nil
	}
	return nil, nil
}

func (s adminCustomDomainSettings) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}
func (s adminCustomDomainSettings) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}
func (s adminCustomDomainSettings) Set(_ context.Context, key, value string) error {
	s[key] = value
	return nil
}
func (s adminCustomDomainSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (s adminCustomDomainSettings) SetMultiple(context.Context, map[string]string) error { return nil }
func (s adminCustomDomainSettings) GetAll(context.Context) (map[string]string, error)    { return s, nil }
func (s adminCustomDomainSettings) Delete(_ context.Context, key string) error {
	delete(s, key)
	return nil
}

func adminCustomDomainContext(t *testing.T, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 99})
	c.Set(string(middleware2.ContextKeyUserRole), "admin")
	return c, recorder
}

func TestAdminCustomDomainAuditPayloadContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCustomDomainHandler(nil)
	c, _ := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains/73/verify", "")

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	h.audit(c, "custom domain verified", 73, "api.example.com", "status", service.CustomDomainStatusActive)

	require.Contains(t, logs.String(), "custom domain verified")
	require.Contains(t, logs.String(), "audit=true")
	require.Contains(t, logs.String(), "user_id=99")
	require.Contains(t, logs.String(), "role=admin")
	require.Contains(t, logs.String(), "domain_id=73")
	require.Contains(t, logs.String(), "domain=api.example.com")
	require.Contains(t, logs.String(), "status=active")
}

func TestAdminCustomDomainAuditAlwaysCarriesIdentityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCustomDomainHandler(nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/custom-domains/73/verify", nil)

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	h.audit(c, "custom domain verified", 73, "api.example.com")

	require.Contains(t, logs.String(), "user_id=0")
	require.Contains(t, logs.String(), `role=""`)
}

func TestAdminCustomDomainHandlerLifecycleContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminCustomDomainRepo{}
	settings := adminCustomDomainSettings{
		service.SettingKeyCustomDomainsEnabled: "true",
		service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
	}
	h := NewCustomDomainHandler(service.NewCustomDomainService(repo, settings, adminCustomDomainDNS{repo: repo}))

	configContext, configRecorder := adminCustomDomainContext(t, http.MethodGet, "/admin/custom-domains/config", "")
	h.GetConfig(configContext)
	require.Equal(t, http.StatusOK, configRecorder.Code)
	require.Contains(t, configRecorder.Body.String(), `"enabled":true`)

	updateConfigContext, updateConfigRecorder := adminCustomDomainContext(t, http.MethodPut, "/admin/custom-domains/config", `{"enabled":false}`)
	h.UpdateConfig(updateConfigContext)
	require.Equal(t, http.StatusOK, updateConfigRecorder.Code)
	require.Equal(t, "false", settings[service.SettingKeyCustomDomainsEnabled])
	settings[service.SettingKeyCustomDomainsEnabled] = "true"

	createContext, createRecorder := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains", `{"user_id":41,"domain":"api.example.com","all_users":false,"user_ids":[42]}`)
	h.Create(createContext)
	require.Equal(t, http.StatusOK, createRecorder.Code)

	listContext, listRecorder := adminCustomDomainContext(t, http.MethodGet, "/admin/custom-domains", "")
	h.List(listContext)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	require.Contains(t, listRecorder.Body.String(), `"domain":"api.example.com"`)

	accessContext, accessRecorder := adminCustomDomainContext(t, http.MethodPut, "/admin/custom-domains/1/access", `{"all_users":true,"user_ids":[]}`)
	accessContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.UpdateAccess(accessContext)
	require.Equal(t, http.StatusOK, accessRecorder.Code)
	require.True(t, repo.domain.AllUsers)

	verifyContext, verifyRecorder := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains/1/verify", "")
	verifyContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Verify(verifyContext)
	require.Equal(t, http.StatusOK, verifyRecorder.Code)

	disableContext, disableRecorder := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains/1/disable", `{"reason":"maintenance"}`)
	disableContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Disable(disableContext)
	require.Equal(t, http.StatusOK, disableRecorder.Code)
	require.Equal(t, service.CustomDomainStatusDisabled, repo.domain.Status)

	enableContext, enableRecorder := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains/1/enable", "")
	enableContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Enable(enableContext)
	require.Equal(t, http.StatusOK, enableRecorder.Code)
	require.Equal(t, service.CustomDomainStatusPendingDNS, repo.domain.Status)

	deleteContext, deleteRecorder := adminCustomDomainContext(t, http.MethodDelete, "/admin/custom-domains/1", "")
	deleteContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Delete(deleteContext)
	require.Equal(t, http.StatusOK, deleteRecorder.Code)
	require.Nil(t, repo.domain)
}

func TestAdminCustomDomainDisableUsesRetainedRequestContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminCustomDomainRepo{domain: &service.CustomDomain{
		ID:     1,
		Domain: "api.example.com",
		Status: service.CustomDomainStatusActive,
	}}
	h := NewCustomDomainHandler(service.NewCustomDomainService(repo, adminCustomDomainSettings{}, adminCustomDomainDNS{}))

	malformedContext, malformedRecorder := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains/1/disable", `{`)
	malformedContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Disable(malformedContext)
	require.Equal(t, http.StatusOK, malformedRecorder.Code)
	require.Equal(t, service.CustomDomainStatusDisabled, repo.domain.Status)
	require.Nil(t, repo.domain.DisabledReason)

	invalidContext, invalidRecorder := adminCustomDomainContext(t, http.MethodPost, "/admin/custom-domains/nope/disable", `{}`)
	invalidContext.Params = gin.Params{{Key: "id", Value: "nope"}}
	h.Disable(invalidContext)
	require.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
	require.Contains(t, invalidRecorder.Body.String(), `"message":"Invalid custom domain id"`)
}

func TestAdminCustomDomainUpdateConfigRejectsInvalidBodyWithRetainedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCustomDomainHandler(nil)
	c, recorder := adminCustomDomainContext(t, http.MethodPut, "/admin/custom-domains/config", `{`)

	h.UpdateConfig(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"message":"Invalid request body"`)
}
