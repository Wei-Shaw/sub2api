//go:build unit

package admin

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 在轮换读取旧值后暂停，让删除在恢复轮换之前完整提交。
type legacyKeyReadBarrier struct {
	service.SettingRepository
	read   chan struct{}
	resume chan struct{}
	once   sync.Once
}

func (r *legacyKeyReadBarrier) GetValue(ctx context.Context, key string) (string, error) {
	value, err := r.SettingRepository.GetValue(ctx, key)
	if key == service.SettingKeyAdminAPIKey && err == nil {
		r.once.Do(func() { close(r.read); <-r.resume })
	}
	return value, err
}

func (r *legacyKeyReadBarrier) ReplaceExistingAdminAPIKey(ctx context.Context, value string) (bool, error) {
	return r.SettingRepository.(interface {
		ReplaceExistingAdminAPIKey(context.Context, string) (bool, error)
	}).ReplaceExistingAdminAPIKey(ctx, value)
}

func legacyKeyRequest(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/admin/settings/admin-api-key/regenerate", nil)
	c.Set("auth_method", service.AuditAuthMethodJWT)
	c.Set("user_role", "admin")
	handler(c)
	return w
}

func TestLegacyAdminKeyRotationPostgres(t *testing.T) {
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("legacy_key_test_%d", time.Now().UnixNano())
	_, err = base.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close(); _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); _ = base.Close() })
	_, err = db.Exec(`CREATE TABLE settings(id BIGSERIAL PRIMARY KEY, key VARCHAR(100) UNIQUE NOT NULL, value TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`)
	require.NoError(t, err)
	repo := repository.NewSettingRepository(client)
	settings := service.NewSettingService(repo, nil)
	h := &SettingHandler{settingService: settings}
	ctx := context.Background()

	t.Run("删除先完成不得被轮换重建", func(t *testing.T) {
		require.NoError(t, repo.Set(ctx, service.SettingKeyAdminAPIKey, "admin-test-old-key"))
		barrier := &legacyKeyReadBarrier{SettingRepository: repo, read: make(chan struct{}), resume: make(chan struct{})}
		var release sync.Once
		unblock := func() { release.Do(func() { close(barrier.resume) }) }
		defer unblock()
		rotating := &SettingHandler{settingService: service.NewSettingService(barrier, nil)}
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- legacyKeyRequest(rotating.RegenerateAdminAPIKey) }()
		select {
		case <-barrier.read:
		case <-time.After(5 * time.Second):
			t.Fatal("轮换未进入读取屏障")
		}
		require.Equal(t, 200, legacyKeyRequest(h.DeleteAdminAPIKey).Code)
		var afterDelete int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key=$1`, service.SettingKeyAdminAPIKey).Scan(&afterDelete))
		require.Zero(t, afterDelete, "恢复轮换前删除必须已经落库")
		unblock()
		var response *httptest.ResponseRecorder
		select {
		case response = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("轮换未结束")
		}
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key=$1`, service.SettingKeyAdminAPIKey).Scan(&count))
		require.Zero(t, count, "已删除旧版密钥被轮换重新创建")
		require.Equal(t, 403, response.Code)
		require.NotContains(t, response.Body.String(), `"key":`)
	})

	t.Run("已有密钥可轮换且随后可删除", func(t *testing.T) {
		require.NoError(t, repo.Set(ctx, service.SettingKeyAdminAPIKey, "admin-test-old-key"))
		response := legacyKeyRequest(h.RegenerateAdminAPIKey)
		require.Equal(t, 200, response.Code)
		key, err := settings.GetAdminAPIKey(ctx)
		require.NoError(t, err)
		require.NotEqual(t, "admin-test-old-key", key)
		require.NotEmpty(t, key)
		require.Contains(t, response.Body.String(), key)
		require.Equal(t, 200, legacyKeyRequest(h.DeleteAdminAPIKey).Code)
		key, err = settings.GetAdminAPIKey(ctx)
		require.NoError(t, err)
		require.Empty(t, key)
	})

	t.Run("缺失与空值不能新建", func(t *testing.T) {
		for _, emptyRow := range []bool{false, true} {
			require.NoError(t, settings.DeleteAdminAPIKey(ctx))
			if emptyRow {
				require.NoError(t, repo.Set(ctx, service.SettingKeyAdminAPIKey, ""))
			}
			require.Equal(t, 403, legacyKeyRequest(h.RegenerateAdminAPIKey).Code)
			key, err := settings.GenerateAdminAPIKey(ctx)
			require.Error(t, err, "服务层也必须拒绝从无到有创建")
			require.Empty(t, key)
			stored, err := settings.GetAdminAPIKey(ctx)
			require.NoError(t, err)
			require.Empty(t, stored)
		}
	})
}
