package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// OpenAICodexVersionSyncState 与版本值分开保存：检查成功但版本不变也更新检查时间。
// 错误只保存受控分类；不可保存原始响应、请求头、URL 或任意 error.Error()。
type OpenAICodexVersionSyncState struct {
	Status            string     `json:"status"`
	LastCheckedAt     *time.Time `json:"last_checked_at"`
	LastSucceededAt   *time.Time `json:"last_succeeded_at"`
	NextCheckAt       *time.Time `json:"next_check_at"`
	ErrorCode         string     `json:"error_code,omitempty"`
	HTTPStatus        int        `json:"http_status,omitempty"`
	RateLimitResetAt  *time.Time `json:"rate_limit_reset_at,omitempty"`
	RetryCount        int        `json:"retry_count"`
	RetryExhausted    bool       `json:"retry_exhausted"`
	PersistenceFailed bool       `json:"persistence_failed,omitempty"`
}

// OpenAICodexVersionSyncStatus 只通过管理员专用接口返回，不参与通用设置保存。
type OpenAICodexVersionSyncStatus struct {
	OpenAICodexVersionSyncState
	EffectiveVersion     string     `json:"effective_version"`
	VersionSource        string     `json:"version_source"`
	SyncedVersion        string     `json:"synced_version"`
	AutoSyncEnabled      bool       `json:"auto_sync_enabled"`
	LastVersionUpdatedAt *time.Time `json:"last_version_updated_at"`
}

func decodeCodexVersionSyncState(value string) (OpenAICodexVersionSyncState, error) {
	state := OpenAICodexVersionSyncState{Status: "never_checked"}
	if value == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(value), &state); err != nil {
		return OpenAICodexVersionSyncState{Status: "never_checked"}, errors.New("invalid Codex version sync state")
	}
	if (state.Status != "never_checked" && state.Status != "success" && state.Status != "failed") || state.RetryCount < 0 || state.RetryCount > openAICodexVersionMaxRetries {
		return OpenAICodexVersionSyncState{Status: "never_checked"}, errors.New("invalid Codex version sync state")
	}
	return state, nil
}

// selectOpenAICodexClientVersion 与出站规范身份复用同一优先级，避免 UI 自己猜版本。
func selectOpenAICodexClientVersion(values map[string]string) (string, string) {
	if manual := NormalizeCodexClientVersion(values[SettingKeyOpenAICodexClientVersion]); manual != "" {
		return manual, "manual"
	}
	if synced := NormalizeCodexClientVersion(values[SettingKeyOpenAICodexClientVersionSynced]); synced != "" {
		return synced, "synced"
	}
	return codexCLIVersion, "builtin"
}

func (s *SettingService) GetOpenAICodexVersionSyncStatus(ctx context.Context) (*OpenAICodexVersionSyncStatus, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyOpenAICodexClientVersion,
		SettingKeyOpenAICodexClientVersionSynced,
		SettingKeyOpenAICodexVersionAutoSyncEnabled,
		SettingKeyOpenAICodexVersionSyncState,
	})
	if err != nil {
		return nil, errors.New("unable to read Codex version sync settings")
	}
	state, err := decodeCodexVersionSyncState(values[SettingKeyOpenAICodexVersionSyncState])
	if err != nil {
		return nil, err
	}
	// DB 写入失败但仍可读时，不能用旧的成功快照掩盖本实例刚发生的失败。
	if live, ok := s.openAICodexSyncState.Load().(*OpenAICodexVersionSyncState); ok && live != nil && live.LastCheckedAt != nil {
		if state.LastCheckedAt == nil || live.LastCheckedAt.After(*state.LastCheckedAt) || (live.LastCheckedAt.Equal(*state.LastCheckedAt) && live.PersistenceFailed) {
			state = *live
		}
	}
	version, source := selectOpenAICodexClientVersion(values)
	status := &OpenAICodexVersionSyncStatus{
		OpenAICodexVersionSyncState: state,
		EffectiveVersion:            version,
		VersionSource:               source,
		SyncedVersion:               NormalizeCodexClientVersion(values[SettingKeyOpenAICodexClientVersionSynced]),
		AutoSyncEnabled:             strings.TrimSpace(values[SettingKeyOpenAICodexVersionAutoSyncEnabled]) == "" || strings.TrimSpace(values[SettingKeyOpenAICodexVersionAutoSyncEnabled]) == "true",
	}
	if !status.AutoSyncEnabled {
		status.NextCheckAt = nil
	}
	if status.SyncedVersion != "" {
		setting, err := s.settingRepo.Get(ctx, SettingKeyOpenAICodexClientVersionSynced)
		if err != nil {
			return nil, errors.New("unable to read Codex version update time")
		}
		if setting != nil && !setting.UpdatedAt.IsZero() {
			at := setting.UpdatedAt.UTC()
			status.LastVersionUpdatedAt = &at
		}
	}
	return status, nil
}
