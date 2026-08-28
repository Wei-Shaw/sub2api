package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ipBanCheckRepository struct {
	banned bool
	seenIP string
}

func (r *ipBanCheckRepository) Create(context.Context, string) (*service.IPBan, error) {
	return nil, nil
}

func (r *ipBanCheckRepository) List(context.Context, pagination.PaginationParams) ([]service.IPBan, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *ipBanCheckRepository) Delete(context.Context, int64) error { return nil }

func (r *ipBanCheckRepository) IsBanned(_ context.Context, ipAddress string) (bool, error) {
	r.seenIP = ipAddress
	return r.banned, nil
}

func TestIPBanGuardRejectsBeforeAPIKeyAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &ipBanCheckRepository{banned: true}
	banService := service.NewIPBanService(repo)
	called := false
	next := APIKeyAuthMiddleware(func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	guard := IPBanGuard(next, banService, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "198.51.100.7:1234"
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = req

	guard(c)

	if writer.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", writer.Code, http.StatusForbidden)
	}
	if called {
		t.Fatal("API key authentication was called for a banned IP")
	}
	if repo.seenIP != "198.51.100.7" {
		t.Fatalf("checked IP = %q, want 198.51.100.7", repo.seenIP)
	}
}

func TestIPBanGuardPassesAllowedIPToAPIKeyAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &ipBanCheckRepository{}
	banService := service.NewIPBanService(repo)
	called := false
	next := APIKeyAuthMiddleware(func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	guard := IPBanGuard(next, banService, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.8:1234"
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = req

	guard(c)

	if !called || writer.Code != http.StatusOK {
		t.Fatalf("allowed request did not reach API key authentication: called=%v status=%d", called, writer.Code)
	}
}
