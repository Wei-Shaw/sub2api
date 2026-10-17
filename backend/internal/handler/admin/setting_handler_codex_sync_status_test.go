package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexSyncStatusRepoStub struct {
	*settingHandlerRepoStub
	updatedAt time.Time
	readErr   error
}

func (r *codexSyncStatusRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, err := r.GetValue(ctx, key)
	return &service.Setting{Key: key, Value: value, UpdatedAt: r.updatedAt}, err
}

func (r *codexSyncStatusRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.settingHandlerRepoStub.GetMultiple(ctx, keys)
}

func codexSyncStatusRequest(t *testing.T, repo service.SettingRepository) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(repo, &config.Config{})
	h := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/openai-codex-version-sync", nil)
	h.GetOpenAICodexVersionSyncStatus(c)
	return rec
}

func TestGetCodexSyncStatusIsReadOnlyAndSeparatesTimestamps(t *testing.T) {
	checked := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	updated := checked.Add(-7 * 24 * time.Hour)
	state, err := json.Marshal(service.OpenAICodexVersionSyncState{Status: "success", LastCheckedAt: &checked, LastSucceededAt: &checked})
	require.NoError(t, err)
	repo := &codexSyncStatusRepoStub{
		settingHandlerRepoStub: &settingHandlerRepoStub{values: map[string]string{
			service.SettingKeyOpenAICodexClientVersion:       "0.159.0",
			service.SettingKeyOpenAICodexClientVersionSynced: "0.160.0",
			service.SettingKeyOpenAICodexVersionSyncState:    string(state),
		}}, updatedAt: updated,
	}
	rec := codexSyncStatusRequest(t, repo)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data service.OpenAICodexVersionSyncStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "manual", body.Data.VersionSource)
	require.Equal(t, "0.159.0", body.Data.EffectiveVersion)
	require.Equal(t, checked, *body.Data.LastSucceededAt)
	require.Equal(t, updated, *body.Data.LastVersionUpdatedAt)
	require.Nil(t, repo.lastUpdates, "读取状态不能持久化设置或启动上游同步")
}

func TestGetCodexSyncStatusDoesNotLeakRepositoryError(t *testing.T) {
	repo := &codexSyncStatusRepoStub{settingHandlerRepoStub: &settingHandlerRepoStub{}, readErr: errors.New("database password=private-secret")}
	rec := codexSyncStatusRequest(t, repo)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "private-secret")
}

func TestUpdateSettingsCannotOverwriteCodexSyncState(t *testing.T) {
	state := `{"status":"failed","error_code":"github_primary_rate_limit","retry_count":2}`
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyOpenAICodexVersionSyncState:    state,
		service.SettingKeyOpenAICodexClientVersionSynced: "0.160.0",
	})
	rec := doUpdateSettings(t, h, map[string]any{
		"openai_codex_client_version":        "0.159.0",
		"openai_codex_client_version_synced": "0.100.0",
		"openai_codex_version_sync_state":    "cleared",
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, state, repo.values[service.SettingKeyOpenAICodexVersionSyncState])
	require.Equal(t, "0.160.0", repo.values[service.SettingKeyOpenAICodexClientVersionSynced])
}
