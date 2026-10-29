//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminAccountDTORedactsSecretsAndForeignGroups(t *testing.T) {
	p := adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}
	a := &service.Account{ID: 9, Type: "oauth", GroupIDs: []int64{1, 2}, Credentials: map[string]any{"access_token": "TOP_SECRET", "refresh_token": "REFRESH_SECRET", "base_url": "URL_SECRET", "model_mapping": map[string]any{"alias": "target", "invalid": map[string]any{"token": "NESTED_SECRET"}}}, Extra: map[string]any{"private": "EXTRA_SECRET"}}
	body, _ := json.Marshal(scopedAdminAccountDTO(a, p))
	s := string(body)
	if strings.Contains(s, "SECRET") || strings.Contains(s, "extra") {
		t.Fatalf("泄漏敏感字段: %s", s)
	}
	require.Contains(t, s, `"credentials":{"model_mapping":{"alias":"target"}}`)
	if !strings.Contains(s, `"group_ids":[1]`) {
		t.Fatalf("泄漏越权组: %s", s)
	}
}

func TestScopedAdminAccountErrorsAreClassifiedAndRedacted(t *testing.T) {
	for _, code := range []int{400, 403, 404, 409, 502, 503, 500} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		scopedAdminAccountError(c, infraerrors.New(code, "secret", "TOP_SECRET"))
		want := code
		if code == 500 {
			want = 503
		}
		if w.Code != want || strings.Contains(w.Body.String(), "TOP_SECRET") {
			t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	scopedAdminAccountError(c, errors.New("TOP_SECRET"))
	if w.Code != 503 || strings.Contains(w.Body.String(), "TOP_SECRET") {
		t.Fatal(w.Body.String())
	}
}
func TestScopedAdminAccountMiddlewareKeepsJWTAndDeniesUnavailableService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, restricted := range []bool{false, true} {
		r := gin.New()
		h := &AccountHandler{}
		r.Use(func(c *gin.Context) {
			if restricted {
				c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
			}
		})
		r.Use(h.ScopedAdminKeyAccounts)
		r.GET("/api/v1/admin/accounts", func(c *gin.Context) { c.Status(204) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil))
		want := 204
		if restricted {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("restricted=%v status=%d", restricted, w.Code)
		}
	}
}

// 批量入口必须使用受限服务，不得回落到嵌入的全权 BulkUpdateAccounts。
type scopedBulkAccountService struct {
	service.AdminService
	calls  []int64
	fields map[string]json.RawMessage
}

func (s *scopedBulkAccountService) ScopedAdminList(context.Context, pagination.PaginationParams, string, string, string, string) ([]service.Account, int64, error) {
	panic("不应读取全站账户")
}
func (s *scopedBulkAccountService) ScopedAdminWrite(_ context.Context, id int64, _ string, fields map[string]json.RawMessage, _ func(context.Context, *service.Account) (*service.Account, error)) (*service.Account, error) {
	s.calls = append(s.calls, id)
	s.fields = fields
	switch id {
	case 2:
		return nil, infraerrors.NotFound("SECRET", "TOP_SECRET")
	case 3:
		return nil, infraerrors.Forbidden("SECRET", "TOP_SECRET")
	case 4:
		return nil, errors.New("TOP_SECRET")
	}
	return &service.Account{ID: id}, nil
}

func TestScopedAdminBulkUpdateValidatesTargetsAndRedactsResults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		body   string
		status int
		ids    []int64
	}{
		{`{"account_ids":[1,2,1,3,4],"credentials":{"model_mapping":{"alias":"target"}}}`, 200, []int64{1, 2, 3, 4}},
		{`null`, 400, nil},
		{`{"credentials":{"model_mapping":{}}}`, 400, nil},
		{`{"account_ids":[],"credentials":{"model_mapping":{}}}`, 400, nil},
		{`{"account_ids":[1,0],"credentials":{"model_mapping":{}}}`, 400, nil},
		{`{"account_ids":[1,-1],"credentials":{"model_mapping":{}}}`, 400, nil},
		{`{"account_ids":[1]}`, 400, nil},
		{`{"account_ids":[1],"filters":{},"credentials":{"model_mapping":{}}}`, 400, nil},
		{`{"account_ids":[1.5],"credentials":{"model_mapping":{}}}`, 400, nil},
		{`{"account_ids":[` + strings.Repeat("1,", 100) + `1],"credentials":{"model_mapping":{}}}`, 400, nil},
	} {
		t.Run(tc.body, func(t *testing.T) {
			svc := &scopedBulkAccountService{}
			h := &AccountHandler{adminService: svc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/v1/admin/accounts/bulk-update", strings.NewReader(tc.body))
			h.scopedAdminBulkUpdate(c, svc)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, tc.ids, svc.calls)
			require.NotContains(t, w.Body.String(), "SECRET")
			if tc.status == 200 {
				body, err := json.Marshal(svc.fields)
				require.NoError(t, err)
				require.JSONEq(t, `{"credentials":{"model_mapping":{"alias":"target"}}}`, string(body))
				var result struct {
					Data service.BulkUpdateAccountsResult `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Equal(t, 1, result.Data.Success)
				require.Equal(t, 3, result.Data.Failed)
				require.Equal(t, []int64{1}, result.Data.SuccessIDs)
				require.Equal(t, []int64{2, 3, 4}, result.Data.FailedIDs)
				require.Equal(t, "账户不存在", result.Data.Results[1].Error)
				require.Equal(t, "操作未授权", result.Data.Results[2].Error)
				require.Equal(t, "账户服务暂不可用", result.Data.Results[3].Error)
			}
		})
	}
}
