package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type customDomainMiddlewareRepoStub struct {
	byID   map[int64]*service.CustomDomain
	byHost map[string]int64
}

func newCustomDomainMiddlewareRepoStub(domains ...*service.CustomDomain) *customDomainMiddlewareRepoStub {
	repo := &customDomainMiddlewareRepoStub{
		byID:   map[int64]*service.CustomDomain{},
		byHost: map[string]int64{},
	}
	for _, domain := range domains {
		repo.byID[domain.ID] = cloneMiddlewareCustomDomain(domain)
		repo.byHost[strings.ToLower(domain.Domain)] = domain.ID
	}
	return repo
}

func (r *customDomainMiddlewareRepoStub) Create(ctx context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	return nil, service.ErrCustomDomainConflict
}

func (r *customDomainMiddlewareRepoStub) GetByID(ctx context.Context, id int64) (*service.CustomDomain, error) {
	domain, ok := r.byID[id]
	if !ok {
		return nil, service.ErrCustomDomainNotFound
	}
	return cloneMiddlewareCustomDomain(domain), nil
}

func (r *customDomainMiddlewareRepoStub) GetByDomain(ctx context.Context, domain string) (*service.CustomDomain, error) {
	id, ok := r.byHost[strings.ToLower(domain)]
	if !ok {
		return nil, service.ErrCustomDomainNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *customDomainMiddlewareRepoStub) ListByUserID(ctx context.Context, userID int64) ([]service.CustomDomain, error) {
	return nil, nil
}

func (r *customDomainMiddlewareRepoStub) ListAll(ctx context.Context, filters service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	return nil, nil
}

func (r *customDomainMiddlewareRepoStub) SetAccess(ctx context.Context, id int64, allUsers bool, userIDs []int64) (*service.CustomDomain, error) {
	return nil, service.ErrCustomDomainNotFound
}

func (r *customDomainMiddlewareRepoStub) Update(ctx context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	return cloneMiddlewareCustomDomain(domain), nil
}

func (r *customDomainMiddlewareRepoStub) Delete(ctx context.Context, id int64) error {
	return nil
}

type customDomainMiddlewareSettingStub struct {
	values map[string]string
}

func (s *customDomainMiddlewareSettingStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (s *customDomainMiddlewareSettingStub) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (s *customDomainMiddlewareSettingStub) Set(ctx context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *customDomainMiddlewareSettingStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *customDomainMiddlewareSettingStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *customDomainMiddlewareSettingStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *customDomainMiddlewareSettingStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestCustomDomainGuardEnforcesAuthorizedUsersAndStoresContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifiedAt := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	svc := newCustomDomainMiddlewareService(&service.CustomDomain{
		ID:         7,
		UserID:     42,
		UserIDs:    []int64{42, 99},
		Domain:     "api.customer.example",
		Status:     service.CustomDomainStatusActive,
		VerifiedAt: &verifiedAt,
	})

	status, called, domainID, domain := runCustomDomainGuardRequest(svc, "api.customer.example", 42)
	if status != http.StatusOK || !called {
		t.Fatalf("matching owner should pass, status=%d called=%v", status, called)
	}
	if domainID != int64(7) || domain != "api.customer.example" {
		t.Fatalf("custom domain context was not stored, id=%d domain=%q", domainID, domain)
	}

	status, called, _, _ = runCustomDomainGuardRequest(svc, "api.customer.example", 99)
	if status != http.StatusOK || !called {
		t.Fatalf("explicitly authorized API key owner should pass, status=%d called=%v", status, called)
	}

	status, called, _, _ = runCustomDomainGuardRequest(svc, "api.customer.example", 100)
	if status != http.StatusForbidden || called {
		t.Fatalf("different API key owner should be forbidden, status=%d called=%v", status, called)
	}
}

func TestCustomDomainGuardAllowsAllUsersDomain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifiedAt := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	svc := newCustomDomainMiddlewareService(&service.CustomDomain{
		ID:         8,
		UserID:     42,
		AllUsers:   true,
		Domain:     "api.shared.example",
		Status:     service.CustomDomainStatusActive,
		VerifiedAt: &verifiedAt,
	})

	status, called, _, _ := runCustomDomainGuardRequest(svc, "api.shared.example", 123)
	if status != http.StatusOK || !called {
		t.Fatalf("all-users custom domain should pass for any API key owner, status=%d called=%v", status, called)
	}
}

func TestCustomDomainGuardRejectsInactiveKnownDomainAndIgnoresUnknownHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newCustomDomainMiddlewareService(&service.CustomDomain{
		ID:     9,
		UserID: 42,
		Domain: "pending.customer.example",
		Status: service.CustomDomainStatusPendingDNS,
	})

	status, called, _, _ := runCustomDomainGuardRequest(svc, "pending.customer.example", 42)
	if status != http.StatusForbidden || called {
		t.Fatalf("known inactive custom host should be forbidden, status=%d called=%v", status, called)
	}

	status, called, _, _ = runCustomDomainGuardRequest(svc, "unknown.example.com", 42)
	if status != http.StatusOK || !called {
		t.Fatalf("unknown host should fall through, status=%d called=%v", status, called)
	}
}

func runCustomDomainGuardRequest(svc *service.CustomDomainService, host string, apiKeyUserID int64) (int, bool, int64, string) {
	router := gin.New()
	called := false
	var capturedID int64
	var capturedDomain string
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{UserID: apiKeyUserID})
		c.Next()
	})
	router.Use(CustomDomainGuard(svc))
	router.GET("/v1/models", func(c *gin.Context) {
		called = true
		if id, ok := c.Request.Context().Value(ctxkey.CustomDomainID).(int64); ok {
			capturedID = id
		}
		if domain, ok := c.Request.Context().Value(ctxkey.CustomDomain).(string); ok {
			capturedDomain = domain
		}
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code, called, capturedID, capturedDomain
}

func newCustomDomainMiddlewareService(domains ...*service.CustomDomain) *service.CustomDomainService {
	return service.NewCustomDomainService(
		newCustomDomainMiddlewareRepoStub(domains...),
		&customDomainMiddlewareSettingStub{values: map[string]string{
			service.SettingKeyCustomDomainsEnabled: "true",
			service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
		}},
	)
}

func cloneMiddlewareCustomDomain(domain *service.CustomDomain) *service.CustomDomain {
	if domain == nil {
		return nil
	}
	cp := *domain
	cp.UserIDs = append([]int64(nil), domain.UserIDs...)
	return &cp
}
