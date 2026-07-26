//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type customDomainHandlerRepo struct {
	domains map[int64]*service.CustomDomain
	nextID  int64
}

func (r *customDomainHandlerRepo) Create(_ context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	domain.ID = r.nextID
	r.nextID++
	copy := *domain
	r.domains[domain.ID] = &copy
	return &copy, nil
}

func (r *customDomainHandlerRepo) GetByID(_ context.Context, id int64) (*service.CustomDomain, error) {
	domain, ok := r.domains[id]
	if !ok {
		return nil, service.ErrCustomDomainNotFound
	}
	copy := *domain
	return &copy, nil
}

func (r *customDomainHandlerRepo) GetByDomain(_ context.Context, name string) (*service.CustomDomain, error) {
	for _, domain := range r.domains {
		if domain.Domain == name {
			copy := *domain
			return &copy, nil
		}
	}
	return nil, service.ErrCustomDomainNotFound
}

func (r *customDomainHandlerRepo) ListByUserID(_ context.Context, userID int64) ([]service.CustomDomain, error) {
	out := make([]service.CustomDomain, 0)
	for _, domain := range r.domains {
		if domain.UserID == userID || domain.AllUsers {
			out = append(out, *domain)
		}
	}
	return out, nil
}

func (r *customDomainHandlerRepo) ListAll(_ context.Context, _ service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	out := make([]service.CustomDomain, 0, len(r.domains))
	for _, domain := range r.domains {
		out = append(out, *domain)
	}
	return out, nil
}

func (r *customDomainHandlerRepo) Update(_ context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	copy := *domain
	r.domains[domain.ID] = &copy
	return &copy, nil
}

func (r *customDomainHandlerRepo) Delete(_ context.Context, id int64) error {
	delete(r.domains, id)
	return nil
}

func (r *customDomainHandlerRepo) SetAccess(_ context.Context, id int64, allUsers bool, userIDs []int64) (*service.CustomDomain, error) {
	domain := r.domains[id]
	domain.AllUsers = allUsers
	domain.AuthorizedUserIDs = append([]int64(nil), userIDs...)
	return domain, nil
}

type customDomainHandlerSettings map[string]string

type customDomainHandlerDNS struct {
	records map[string][]string
}

func (d customDomainHandlerDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	return append([]string(nil), d.records[name]...), nil
}

func TestCustomDomainHandlerVerifyAuditContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &customDomainHandlerRepo{domains: make(map[int64]*service.CustomDomain), nextID: 1}
	settings := customDomainHandlerSettings{
		service.SettingKeyCustomDomainsEnabled: "true",
		service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
	}
	dns := customDomainHandlerDNS{records: make(map[string][]string)}
	h := NewCustomDomainHandler(service.NewCustomDomainService(repo, settings, dns))
	domain, err := h.customDomainService.CreateForUserWithAccess(context.Background(), 41, "api.example.com", false, nil)
	require.NoError(t, err)

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	pendingContext, pendingRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains/1/verify", "", 41)
	pendingContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Verify(pendingContext)
	require.Equal(t, http.StatusOK, pendingRecorder.Code)
	require.Contains(t, pendingRecorder.Body.String(), `"status":"pending_dns"`)
	require.Contains(t, logs.String(), "custom domain verification pending")
	require.Contains(t, logs.String(), "audit=true")
	require.Contains(t, logs.String(), "user_id=41")
	require.Contains(t, logs.String(), "domain_id=1")
	require.Contains(t, logs.String(), "domain=api.example.com")
	require.Contains(t, logs.String(), "status=pending_dns")

	logs.Reset()
	dns.records[domain.VerificationTXTName] = []string{domain.VerificationTXTValue}
	verifiedContext, verifiedRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains/1/verify", "", 41)
	verifiedContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Verify(verifiedContext)
	require.Equal(t, http.StatusOK, verifiedRecorder.Code)
	require.Contains(t, verifiedRecorder.Body.String(), `"status":"active"`)
	require.Contains(t, logs.String(), "custom domain verified")
	require.Contains(t, logs.String(), "audit=true")
	require.Contains(t, logs.String(), "user_id=41")
	require.Contains(t, logs.String(), "domain_id=1")
	require.Contains(t, logs.String(), "domain=api.example.com")
	require.Contains(t, logs.String(), "status=active")
}

func TestCustomDomainHandlerVerifyValidationContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &customDomainHandlerRepo{domains: make(map[int64]*service.CustomDomain), nextID: 1}
	h := NewCustomDomainHandler(service.NewCustomDomainService(repo, customDomainHandlerSettings{
		service.SettingKeyCustomDomainsEnabled: "true",
	}, customDomainHandlerDNS{}))

	unauthenticatedRecorder := httptest.NewRecorder()
	unauthenticatedContext, _ := gin.CreateTestContext(unauthenticatedRecorder)
	unauthenticatedContext.Request = httptest.NewRequest(http.MethodPost, "/custom-domains/1/verify", nil)
	h.Verify(unauthenticatedContext)
	require.Equal(t, http.StatusUnauthorized, unauthenticatedRecorder.Code)
	require.Contains(t, unauthenticatedRecorder.Body.String(), "User not authenticated")

	invalidContext, invalidRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains/not-a-number/verify", "", 41)
	invalidContext.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	h.Verify(invalidContext)
	require.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
	require.Contains(t, invalidRecorder.Body.String(), "Invalid custom domain id")

	negativeContext, negativeRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains/-1/verify", "", 41)
	negativeContext.Params = gin.Params{{Key: "id", Value: "-1"}}
	h.Verify(negativeContext)
	require.Equal(t, http.StatusNotFound, negativeRecorder.Code)
	require.NotContains(t, negativeRecorder.Body.String(), "Invalid custom domain id")
}

func (s customDomainHandlerSettings) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}
func (s customDomainHandlerSettings) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}
func (s customDomainHandlerSettings) Set(_ context.Context, key, value string) error {
	s[key] = value
	return nil
}
func (s customDomainHandlerSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (s customDomainHandlerSettings) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (s customDomainHandlerSettings) GetAll(context.Context) (map[string]string, error) {
	return s, nil
}
func (s customDomainHandlerSettings) Delete(_ context.Context, key string) error {
	delete(s, key)
	return nil
}

func newCustomDomainHandlerTestContext(t *testing.T, method, target, body string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
	return c, recorder
}

func TestCustomDomainHandlerUserLifecycleContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &customDomainHandlerRepo{domains: make(map[int64]*service.CustomDomain), nextID: 1}
	settings := customDomainHandlerSettings{
		service.SettingKeyCustomDomainsEnabled: "true",
		service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
	}
	h := NewCustomDomainHandler(service.NewCustomDomainService(repo, settings, customDomainHandlerDNS{}))
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	createContext, createRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains", `{"domain":"api.example.com"}`, 41)
	h.Create(createContext)
	require.Equal(t, http.StatusOK, createRecorder.Code)
	require.Contains(t, createRecorder.Body.String(), `"domain":"api.example.com"`)
	require.Contains(t, createRecorder.Body.String(), `"can_manage":true`)
	require.Equal(t, []int64{41}, repo.domains[1].AuthorizedUserIDs)
	require.Contains(t, logs.String(), "custom domain created")
	require.Contains(t, logs.String(), "audit=true")
	require.Contains(t, logs.String(), "user_id=41")
	require.Contains(t, logs.String(), "domain_id=1")
	require.Contains(t, logs.String(), "domain=api.example.com")

	listContext, listRecorder := newCustomDomainHandlerTestContext(t, http.MethodGet, "/custom-domains", "", 41)
	h.List(listContext)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	var envelope struct {
		Data struct {
			Enabled     bool             `json:"enabled"`
			CNAMETarget string           `json:"cname_target"`
			Domains     []map[string]any `json:"domains"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Data.Enabled)
	require.Equal(t, "gateway.example.com", envelope.Data.CNAMETarget)
	require.Len(t, envelope.Data.Domains, 1)

	verifyContext, verifyRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains/1/verify", "", 41)
	verifyContext.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Verify(verifyContext)
	require.Equal(t, http.StatusOK, verifyRecorder.Code)

	deleteContext, deleteRecorder := newCustomDomainHandlerTestContext(t, http.MethodDelete, "/custom-domains/1", "", 41)
	deleteContext.Params = gin.Params{{Key: "id", Value: "1"}}
	logs.Reset()
	h.Delete(deleteContext)
	require.Equal(t, http.StatusOK, deleteRecorder.Code)
	require.Contains(t, deleteRecorder.Body.String(), `"message":"deleted"`)
	require.Contains(t, logs.String(), "custom domain deleted")
	require.Contains(t, logs.String(), "audit=true")
	require.Contains(t, logs.String(), "user_id=41")
	require.Contains(t, logs.String(), "domain_id=1")
	require.Empty(t, repo.domains)
}

func TestCustomDomainHandlerRetainedValidationMessages(t *testing.T) {
	h := NewCustomDomainHandler(nil)

	unauthenticated, unauthenticatedRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains", `{"domain":"api.example.com"}`, 0)
	unauthenticated.Keys = nil
	h.Create(unauthenticated)
	require.Contains(t, unauthenticatedRecorder.Body.String(), "User not authenticated")

	invalidBody, invalidBodyRecorder := newCustomDomainHandlerTestContext(t, http.MethodPost, "/custom-domains", `{`, 41)
	h.Create(invalidBody)
	require.Contains(t, invalidBodyRecorder.Body.String(), "Invalid request body")

	invalidID, invalidIDRecorder := newCustomDomainHandlerTestContext(t, http.MethodDelete, "/custom-domains/nope", "", 41)
	invalidID.Params = gin.Params{{Key: "id", Value: "nope"}}
	h.Delete(invalidID)
	require.Contains(t, invalidIDRecorder.Body.String(), "Invalid custom domain id")
}
