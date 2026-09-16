//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type fakeQuotaRepoForAdmin struct {
	service.UserPlatformQuotaRepository
	records []service.UserPlatformQuotaRecord
	err     error
}

func (f *fakeQuotaRepoForAdmin) ListByUser(_ context.Context, _ int64) ([]service.UserPlatformQuotaRecord, error) {
	return f.records, f.err
}

func newAdminQuotaTestContext(w *httptest.ResponseRecorder) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	c.Request = req
	return c
}

func TestAdminGetUserPlatformQuotas_IncludesWindowStart(t *testing.T) {
	start := time.Now().Add(-1 * time.Hour)
	repo := &fakeQuotaRepoForAdmin{records: []service.UserPlatformQuotaRecord{{
		UserID: 99, Platform: "anthropic",
		DailyUsageUSD: 1.0, DailyWindowStart: &start,
	}}}
	h := &UserHandler{userPlatformQuotaRepo: repo, adminService: newStubAdminService()}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c := newAdminQuotaTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "99"}}
	h.GetUserPlatformQuotas(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"daily_window_start"`) {
		t.Errorf("admin response missing daily_window_start, got: %s", w.Body.String())
	}
}

func TestAdminGetUserPlatformQuotas_InvalidIDReturns400(t *testing.T) {
	h := &UserHandler{userPlatformQuotaRepo: &fakeQuotaRepoForAdmin{}}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c := newAdminQuotaTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	h.GetUserPlatformQuotas(c)
	if w.Code < 400 || w.Code >= 500 {
		t.Errorf("invalid id should yield 4xx, got %d", w.Code)
	}
}

func TestAdminGetUserPlatformQuotas_EmptyReturnsEmptyArray(t *testing.T) {
	repo := &fakeQuotaRepoForAdmin{records: nil}
	h := &UserHandler{userPlatformQuotaRepo: repo, adminService: newStubAdminService()}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c := newAdminQuotaTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "99"}}
	h.GetUserPlatformQuotas(c)
	if w.Code != 200 {
		t.Errorf("empty list should be 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response missing data object: %v", body)
	}
	quotas, ok := data["platform_quotas"].([]any)
	if !ok {
		t.Fatalf("data.platform_quotas missing or wrong type: %v", data)
	}
	if len(quotas) != 0 {
		t.Errorf("expected empty platform_quotas, got %d entries: %v", len(quotas), quotas)
	}
}

func TestAdminGetUserPlatformQuotas_NilRepoReturnsEmpty(t *testing.T) {
	h := &UserHandler{userPlatformQuotaRepo: nil}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c := newAdminQuotaTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	h.GetUserPlatformQuotas(c)
	if w.Code != 200 {
		t.Errorf("nil repo should return 200 empty, got %d", w.Code)
	}
}

// TestAdminGetUserPlatformQuotas_UserNotFoundReturns404 验证 GET 在用户不存在时返回 404
// （与 PUT / POST reset 端点行为一致；review fix：原实现返回空数组会让 admin 界面误判用户存在）
func TestAdminGetUserPlatformQuotas_UserNotFoundReturns404(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getUserErr = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	repo := &fakeQuotaRepoForAdmin{records: nil}
	h := &UserHandler{userPlatformQuotaRepo: repo, adminService: adminSvc}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c := newAdminQuotaTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	h.GetUserPlatformQuotas(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent user, got %d: %s", w.Code, w.Body.String())
	}
}
