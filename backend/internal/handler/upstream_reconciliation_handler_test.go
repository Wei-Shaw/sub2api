//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type reconciliationHandlerFake struct {
	userID     int64
	admin      bool
	month      string
	saved      bool
	adminInput service.AdminBillingListInput
	billID     int64
}

func (*reconciliationHandlerFake) Connections(context.Context) ([]service.BillingConnection, error) {
	return []service.BillingConnection{}, nil
}
func (*reconciliationHandlerFake) Accounts(context.Context) ([]service.BillingAccountOption, error) {
	return []service.BillingAccountOption{}, nil
}
func (f *reconciliationHandlerFake) SaveConnection(_ context.Context, _ int64, ownerID int64, _ service.SaveBillingConnectionInput) (*service.BillingConnection, error) {
	f.saved, f.userID = true, ownerID
	return &service.BillingConnection{}, nil
}
func (*reconciliationHandlerFake) Sync(context.Context, int64, string) error { return nil }
func (f *reconciliationHandlerFake) Summary(ctx context.Context, month string, userID int64, admin bool) (*service.BillingReport, error) {
	return f.Report(ctx, month, userID, admin)
}
func (*reconciliationHandlerFake) RawBillSource(context.Context, int64) (*service.BillingBillSource, error) {
	return &service.BillingBillSource{}, nil
}
func (f *reconciliationHandlerFake) AdminBills(_ context.Context, input service.AdminBillingListInput) (*service.AdminBillingList, error) {
	f.adminInput = input
	return &service.AdminBillingList{Items: []service.AdminBillingRecord{}}, nil
}
func (f *reconciliationHandlerFake) AdminBill(_ context.Context, id int64, filter service.AdminBillingFilter) (*service.AdminBillingRecord, error) {
	f.billID = id
	f.adminInput.AdminBillingFilter = filter
	return &service.AdminBillingRecord{ID: id}, nil
}
func (f *reconciliationHandlerFake) Report(_ context.Context, month string, userID int64, admin bool) (*service.BillingReport, error) {
	f.userID, f.admin, f.month = userID, admin, month
	return &service.BillingReport{}, nil
}

func billingHandlerContext(method, path, body, role string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if role != "" {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), role)
	}
	return c, w
}

func TestUpstreamBillingAdminActionsRejectOrdinaryUser(t *testing.T) {
	h := &UpstreamReconciliationHandler{} // Any accidental service access panics.
	actions := []struct {
		name   string
		method string
		call   func(*gin.Context)
	}{
		{"connections", "GET", h.Connections},
		{"accounts", "GET", h.Accounts},
		{"raw_source", "GET", h.BillSource},
		{"admin_raw_sources", "GET", h.AdminBills},
		{"admin_raw_source", "GET", h.AdminBill},
		{"admin_capabilities", "GET", h.AdminCapabilities},
		{"create", "POST", h.SaveConnection},
		{"update", "PUT", h.SaveConnection},
		{"sync", "POST", h.Sync},
	}
	for _, action := range actions {
		for _, role := range []string{"", "user"} {
			t.Run(action.name+"/"+role, func(t *testing.T) {
				c, w := billingHandlerContext(action.method, "/", `{}`, role)
				action.call(c)
				want := http.StatusForbidden
				if role == "" {
					want = http.StatusUnauthorized
				}
				require.Equal(t, want, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			})
		}
	}
}

func TestUpstreamBillingAdminAPIParsesFilters(t *testing.T) {
	fake := &reconciliationHandlerFake{}
	h := &UpstreamReconciliationHandler{service: fake}
	c, w := billingHandlerContext("GET", "/?month=2026-09&connection_id=3&provider=anthropic&resource_id=workspace&limit=2&cursor=opaque", "", "admin")
	h.AdminBills(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "2026-09", fake.adminInput.Month)
	require.EqualValues(t, 3, fake.adminInput.ConnectionID)
	require.Equal(t, "anthropic", fake.adminInput.Provider)
	require.Equal(t, "workspace", fake.adminInput.ResourceID)
	require.Equal(t, 2, fake.adminInput.Limit)
	require.Equal(t, "opaque", fake.adminInput.Cursor)
	c, w = billingHandlerContext("GET", "/?month=2026-09&provider=azure", "", "admin")
	c.Params = gin.Params{{Key: "id", Value: "91"}}
	h.AdminBill(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 91, fake.billID)
	require.Equal(t, "azure", fake.adminInput.Provider)
}

func TestUpstreamBillingAdminAPIRejectsInvalidQuery(t *testing.T) {
	for _, query := range []string{"", "?month=2026-09&month=2026-08", "?month=2026-09&connection_id=-1", "?month=2026-09&connection_id=not-an-id", "?month=2026-09&limit=0", "?month=2026-09&limit=21", "?month=2026-09&limit=2&limit=3"} {
		c, w := billingHandlerContext("GET", "/"+query, "", "admin")
		(&UpstreamReconciliationHandler{}).AdminBills(c)
		require.Equal(t, http.StatusBadRequest, w.Code, query)
	}
}

func TestUpstreamBillingReportUsesSessionIdentityOnly(t *testing.T) {
	fake := &reconciliationHandlerFake{}
	h := &UpstreamReconciliationHandler{service: fake}
	c, w := billingHandlerContext("GET", "/?month=2026-08&user_id=99&is_admin=true", "", "user")
	h.Report(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 7, fake.userID)
	require.False(t, fake.admin)
	require.Equal(t, "2026-08", fake.month)

	c, w = billingHandlerContext("GET", "/?month=2026-08", "", "admin")
	h.Report(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.admin)
}

func TestUpstreamBillingReportRequiresIdentity(t *testing.T) {
	c, w := billingHandlerContext("GET", "/?month=2026-08", "", "")
	(&UpstreamReconciliationHandler{}).Report(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpstreamBillingSummaryUsesSessionIdentity(t *testing.T) {
	fake := &reconciliationHandlerFake{}
	c, w := billingHandlerContext("GET", "/?month=2026-08&user_id=99&is_admin=true", "", "user")
	(&UpstreamReconciliationHandler{service: fake}).Summary(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 7, fake.userID)
	require.False(t, fake.admin)
	require.Equal(t, "2026-08", fake.month)
}

func TestUpstreamBillingSourceRequiresValidID(t *testing.T) {
	c, w := billingHandlerContext("GET", "/", "", "admin")
	c.Params = gin.Params{{Key: "id", Value: "-1"}}
	(&UpstreamReconciliationHandler{}).BillSource(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpstreamBillingInputDoesNotEchoSecrets(t *testing.T) {
	fake := &reconciliationHandlerFake{}
	h := &UpstreamReconciliationHandler{service: fake}
	for _, body := range []string{
		`{"enabled":"private-client-secret-canary"}`,
		`{"name":"` + strings.Repeat("x", 128<<10) + `","secrets":{"client_secret":"private-client-secret-canary"}}`,
	} {
		c, w := billingHandlerContext("POST", "/", body, "admin")
		h.SaveConnection(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.NotContains(t, w.Body.String(), "private-client-secret-canary")
		require.False(t, fake.saved)
	}
}

func TestUpstreamBillingUpdateRejectsInvalidID(t *testing.T) {
	for _, id := range []string{"", "0", "-1", "not-a-number"} {
		c, w := billingHandlerContext("PUT", "/", `{}`, "admin")
		c.Params = gin.Params{{Key: "id", Value: id}}
		(&UpstreamReconciliationHandler{}).SaveConnection(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
}

func TestUpstreamBillingSaveUsesAuthenticatedOwner(t *testing.T) {
	fake := &reconciliationHandlerFake{}
	c, w := billingHandlerContext("POST", "/", `{"owner_id":99}`, "admin")
	(&UpstreamReconciliationHandler{service: fake}).SaveConnection(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.saved)
	require.EqualValues(t, 7, fake.userID)
}
