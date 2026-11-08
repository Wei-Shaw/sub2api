//go:build unit

package admin

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminKeyHandlersRejectNonJWT(t *testing.T) {
	h := &SettingHandler{}
	for _, fn := range []gin.HandlerFunc{h.ListScopedAdminKeys, h.CreateScopedAdminKey, h.UpdateScopedAdminKey, h.RotateScopedAdminKey, h.RevokeScopedAdminKey, h.DeleteScopedAdminKeyRecord, h.GetAdminAPIKey, h.RegenerateAdminAPIKey, h.DeleteAdminAPIKey} {
		for _, method := range []string{"", "admin_api_key"} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("auth_method", method)
			fn(c)
			if w.Code != 403 {
				t.Fatalf("%s 应拒绝，返回 %d", method, w.Code)
			}
		}
	}
}

type deleteScopedKeyRecordRepo struct {
	service.ScopedAdminKeyRepository
	err error
	id  string
}

func (r *deleteScopedKeyRecordRepo) DeleteScopedAdminKeyRecord(_ context.Context, id string, at time.Time) error {
	r.id = id
	if at.IsZero() {
		panic("删除时间不能为空")
	}
	return r.err
}

func TestScopedAdminKeyDeleteRecordHandler(t *testing.T) {
	for _, tc := range []struct {
		name, role string
		err        error
		status     int
	}{
		{"仅列表移除", "admin", nil, 200},
		{"未撤销或已移除", "admin", service.ErrScopedAdminKeyNotFound, 404},
		{"普通用户拒绝", "user", nil, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &deleteScopedKeyRecordRepo{err: tc.err}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(repo)
			h := &SettingHandler{settingService: settings}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("DELETE", "/api/v1/admin/settings/admin-api-keys/key-1/record", nil)
			c.Params = gin.Params{{Key: "id", Value: "key-1"}}
			c.Set("auth_method", service.AuditAuthMethodJWT)
			c.Set("user_role", tc.role)
			h.DeleteScopedAdminKeyRecord(c)
			require.Equal(t, tc.status, w.Code)
			if tc.role != "admin" {
				require.Empty(t, repo.id)
			} else {
				require.Equal(t, "key-1", repo.id)
			}
			if tc.status == 200 {
				require.Contains(t, w.Body.String(), "历史操作记录仍会继续保留")
			}
		})
	}
}
