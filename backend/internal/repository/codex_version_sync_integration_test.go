//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 验证真实发布客户端、同步任务与 PostgreSQL 的整条链路，不使用生产凭据或外部模型。
func TestCodexVersionSyncPostgresRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limited bool
		latest  string
	}{
		{name: "版本向前更新并保留手动优先级", latest: "0.160.0"},
		{name: "版本不变仍记录成功但不改版本时间", latest: "0.158.0"},
		{name: "主限流持久化冷却且保留旧版本", limited: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// 后台任务和状态读取并发使用连接池，不能共用单个测试事务。
			repo := NewSettingRepository(testEntClient(t))
			keys := []string{
				service.SettingKeyOpenAICodexClientVersion,
				service.SettingKeyOpenAICodexClientVersionSynced,
				service.SettingKeyOpenAICodexVersionAutoSyncEnabled,
				service.SettingKeyOpenAICodexVersionSyncState,
			}
			original, err := repo.GetMultiple(ctx, keys)
			require.NoError(t, err)
			t.Cleanup(func() {
				for _, key := range keys {
					if value, exists := original[key]; exists {
						require.NoError(t, repo.Set(ctx, key, value))
					} else {
						require.NoError(t, repo.Delete(ctx, key))
					}
				}
			})
			require.NoError(t, repo.SetMultiple(ctx, map[string]string{
				service.SettingKeyOpenAICodexClientVersion:          "0.159.0",
				service.SettingKeyOpenAICodexClientVersionSynced:    "0.158.0",
				service.SettingKeyOpenAICodexVersionAutoSyncEnabled: "true",
				service.SettingKeyOpenAICodexVersionSyncState:       `{"status":"never_checked"}`,
			}))
			before, err := repo.Get(ctx, service.SettingKeyOpenAICodexClientVersionSynced)
			require.NoError(t, err)
			reset := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/repos/openai/codex/releases/latest" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.limited {
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"测试凭据不应进入状态记录"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "rust-v" + tc.latest, "draft": false, "prerelease": false})
			}))
			t.Cleanup(upstream.Close)
			client := newTestGitHubReleaseClient()
			client.httpClient.Transport = &testTransport{testServerURL: upstream.URL}
			settings := service.NewSettingService(repo, nil)
			task := service.NewOpenAICodexVersionSyncService(repo, settings, client, time.Hour)
			t.Cleanup(task.Stop)
			task.Start()
			wantStatus := "success"
			if tc.limited {
				wantStatus = "failed"
			}
			require.Eventually(t, func() bool {
				status, err := settings.GetOpenAICodexVersionSyncStatus(ctx)
				return err == nil && status.Status == wantStatus && status.LastCheckedAt != nil
			}, 5*time.Second, 10*time.Millisecond)
			task.Stop()

			// 新建服务从数据库读取，而非复用任务的内存快照。
			restartedSettings := service.NewSettingService(repo, nil)
			status, err := restartedSettings.GetOpenAICodexVersionSyncStatus(ctx)
			require.NoError(t, err)
			require.Equal(t, "manual", status.VersionSource)
			require.Equal(t, "0.159.0", status.EffectiveVersion)
			require.Equal(t, status.EffectiveVersion, restartedSettings.GetOpenAICodexClientVersion(ctx))
			require.NotNil(t, status.LastCheckedAt)
			require.NotNil(t, status.NextCheckAt)
			require.Equal(t, int32(1), requests.Load(), "限流不应追加共享额度的列表请求")
			if tc.limited {
				require.Equal(t, "0.158.0", status.SyncedVersion)
				require.Equal(t, "github_primary_rate_limit", status.ErrorCode)
				require.Equal(t, http.StatusForbidden, status.HTTPStatus)
				require.Equal(t, reset, *status.RateLimitResetAt)
				require.True(t, status.NextCheckAt.After(reset))
				require.Equal(t, 1, status.RetryCount)
				require.Nil(t, status.LastSucceededAt)
			} else {
				require.Equal(t, tc.latest, status.SyncedVersion)
				require.NotNil(t, status.LastSucceededAt)
				require.Empty(t, status.ErrorCode)
			}
			if tc.limited || tc.latest == "0.158.0" {
				require.True(t, status.LastVersionUpdatedAt.Equal(before.UpdatedAt), "没有版本推进时不能改写版本行时间")
			} else {
				require.True(t, status.LastVersionUpdatedAt.After(before.UpdatedAt))
			}
			value, err := repo.GetValue(ctx, service.SettingKeyOpenAICodexVersionSyncState)
			require.NoError(t, err)
			require.NotContains(t, value, "测试凭据")
			require.NotContains(t, value, upstream.URL)

			restarted := service.NewOpenAICodexVersionSyncService(repo, restartedSettings, client, time.Hour)
			t.Cleanup(restarted.Stop)
			restarted.Start()
			require.Never(t, func() bool { return requests.Load() != 1 }, 200*time.Millisecond, 10*time.Millisecond,
				"重启必须恢复已有检查计划，不能立即重复请求")
			restarted.Stop()
			require.NoError(t, repo.Set(ctx, service.SettingKeyOpenAICodexClientVersion, ""))
			restartedSettings.InvalidateOpenAICodexClientVersionCache()
			status, err = restartedSettings.GetOpenAICodexVersionSyncStatus(ctx)
			require.NoError(t, err)
			require.Equal(t, "synced", status.VersionSource)
			require.Equal(t, status.SyncedVersion, restartedSettings.GetOpenAICodexClientVersion(ctx))
		})
	}
}
