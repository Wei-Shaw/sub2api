//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type customDomainGuardRepo struct {
	domain *service.CustomDomain
}

func (r customDomainGuardRepo) Create(context.Context, *service.CustomDomain) (*service.CustomDomain, error) {
	return nil, nil
}
func (r customDomainGuardRepo) GetByID(context.Context, int64) (*service.CustomDomain, error) {
	return r.domain, nil
}
func (r customDomainGuardRepo) GetByDomain(_ context.Context, domain string) (*service.CustomDomain, error) {
	if r.domain == nil || r.domain.Domain != domain {
		return nil, service.ErrCustomDomainNotFound
	}
	copy := *r.domain
	return &copy, nil
}
func (r customDomainGuardRepo) ListByUserID(context.Context, int64) ([]service.CustomDomain, error) {
	return nil, nil
}
func (r customDomainGuardRepo) ListAll(context.Context, service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	return nil, nil
}
func (r customDomainGuardRepo) Update(_ context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	return domain, nil
}
func (r customDomainGuardRepo) Delete(context.Context, int64) error { return nil }
func (r customDomainGuardRepo) SetAccess(context.Context, int64, bool, []int64) (*service.CustomDomain, error) {
	return r.domain, nil
}

type customDomainGuardSettings map[string]string

func (s customDomainGuardSettings) Get(context.Context, string) (*service.Setting, error) {
	return nil, nil
}
func (s customDomainGuardSettings) GetValue(_ context.Context, key string) (string, error) {
	return s[key], nil
}
func (s customDomainGuardSettings) Set(_ context.Context, key, value string) error {
	s[key] = value
	return nil
}
func (s customDomainGuardSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (s customDomainGuardSettings) SetMultiple(context.Context, map[string]string) error { return nil }
func (s customDomainGuardSettings) GetAll(context.Context) (map[string]string, error)    { return s, nil }
func (s customDomainGuardSettings) Delete(_ context.Context, key string) error {
	delete(s, key)
	return nil
}

func TestCustomDomainGuardAttributesAuthorizedDomain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	domain := &service.CustomDomain{
		ID:                73,
		UserID:            41,
		Domain:            "api.example.com",
		Status:            service.CustomDomainStatusActive,
		AuthorizedUserIDs: []int64{42},
	}
	settings := customDomainGuardSettings{
		service.SettingKeyCustomDomainsEnabled: "true",
		service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
	}
	svc := service.NewCustomDomainService(customDomainGuardRepo{domain: domain}, settings, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 42})
		c.Next()
	})
	router.Use(CustomDomainGuard(svc))
	router.POST("/v1/messages", func(c *gin.Context) {
		require.Equal(t, int64(73), c.Request.Context().Value(ctxkey.CustomDomainID))
		require.Equal(t, "api.example.com", c.Request.Context().Value(ctxkey.CustomDomain))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "https://api.example.com/v1/messages", nil)
	req.Host = "api.example.com"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestCustomDomainGuardRejectsInactiveOrUnauthorizedDomain(t *testing.T) {
	tests := []struct {
		name       string
		domain     *service.CustomDomain
		host       string
		subject    *AuthSubject
		wantStatus int
		wantCode   string
	}{
		{
			name: "inactive recognized domain",
			domain: &service.CustomDomain{
				ID: 73, UserID: 42, Domain: "api.example.com", Status: service.CustomDomainStatusDisabled,
			},
			host:       "api.example.com",
			subject:    &AuthSubject{UserID: 42},
			wantStatus: http.StatusForbidden,
			wantCode:   "CUSTOM_DOMAIN_INACTIVE",
		},
		{
			name: "active domain not authorized for API key user",
			domain: &service.CustomDomain{
				ID: 73, UserID: 41, Domain: "api.example.com", Status: service.CustomDomainStatusActive,
			},
			host:       "api.example.com",
			subject:    &AuthSubject{UserID: 42},
			wantStatus: http.StatusForbidden,
			wantCode:   "CUSTOM_DOMAIN_FORBIDDEN",
		},
		{
			name: "unknown request host",
			domain: &service.CustomDomain{
				ID: 73, UserID: 42, Domain: "api.example.com", Status: service.CustomDomainStatusActive,
			},
			host:       "unknown.example.com",
			subject:    &AuthSubject{UserID: 42},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "configured gateway host",
			domain: &service.CustomDomain{
				ID: 73, UserID: 42, Domain: "api.example.com", Status: service.CustomDomainStatusActive,
			},
			host:       "gateway.example.com",
			subject:    &AuthSubject{UserID: 42},
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := customDomainGuardSettings{
				service.SettingKeyCustomDomainsEnabled: "true",
				service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
			}
			svc := service.NewCustomDomainService(customDomainGuardRepo{domain: tt.domain}, settings, nil)
			router := gin.New()
			if tt.subject != nil {
				router.Use(func(c *gin.Context) {
					c.Set(string(ContextKeyUser), *tt.subject)
					c.Next()
				})
			}
			router.Use(CustomDomainGuard(svc))
			router.POST("/v1/messages", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			req := httptest.NewRequest(http.MethodPost, "https://"+tt.host+"/v1/messages", nil)
			req.Host = tt.host
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())
			if tt.wantCode != "" {
				require.Contains(t, w.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestCustomDomainGuardRejectsMissingMalformedAndUnauthorizedAPIKeyContext(t *testing.T) {
	domain := &service.CustomDomain{
		ID:     73,
		UserID: 41,
		Domain: "api.example.com",
		Status: service.CustomDomainStatusActive,
	}
	settings := customDomainGuardSettings{
		service.SettingKeyCustomDomainsEnabled: "true",
		service.SettingKeyAPIBaseURL:           "https://gateway.example.com",
	}
	svc := service.NewCustomDomainService(customDomainGuardRepo{domain: domain}, settings, nil)

	for _, tc := range []struct {
		name        string
		context     any
		wantStatus  int
		wantReason  string
		wantMessage string
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized, wantReason: "API_KEY_REQUIRED", wantMessage: "API key is required"},
		{name: "malformed", context: "not-an-auth-subject", wantStatus: http.StatusUnauthorized, wantReason: "INVALID_API_KEY", wantMessage: "Invalid API key"},
		{name: "unauthorized", context: AuthSubject{UserID: 99}, wantStatus: http.StatusForbidden, wantReason: "CUSTOM_DOMAIN_FORBIDDEN", wantMessage: "custom domain does not belong to this API key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			if tc.context != nil {
				router.Use(func(c *gin.Context) {
					c.Set(string(ContextKeyUser), tc.context)
					c.Next()
				})
			}
			router.Use(CustomDomainGuard(svc))
			router.POST("/v1/messages", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			req := httptest.NewRequest(http.MethodPost, "https://api.example.com/v1/messages", nil)
			req.Host = "api.example.com"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
			require.Contains(t, w.Body.String(), tc.wantReason)
			require.Contains(t, w.Body.String(), tc.wantMessage)
		})
	}
}
