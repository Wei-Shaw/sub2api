package securityaudit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type promptRecordingSettingRepository struct {
	staticSettingRepository
}

func (r *promptRecordingSettingRepository) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *promptRecordingSettingRepository) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = r.values[key]
	}
	return result, nil
}

func TestPromptRecordingConfigDefaultsEnabledAndPersistsChanges(t *testing.T) {
	repository := &promptRecordingSettingRepository{staticSettingRepository: staticSettingRepository{values: map[string]string{
		SettingKeyPromptAuditConfig: "",
		SettingKeyRiskControl:       "false",
	}}}
	manager := NewConfigManager(nil, repository, nil, prefixEncryptor{}, testTotpKeyConfig())
	require.NoError(t, manager.Reload(context.Background()))
	require.True(t, manager.PromptRecordingEnabled())

	require.NoError(t, manager.SavePromptRecordingEnabled(context.Background(), false))
	require.False(t, manager.PromptRecordingEnabled())
	require.Equal(t, "false", repository.values[SettingKeyPromptRecording])

	require.NoError(t, manager.Reload(context.Background()))
	require.False(t, manager.PromptRecordingEnabled())
}

type disabledPromptRecordingStore struct {
	*fakeConfigStore
}

func (*disabledPromptRecordingStore) PromptRecordingEnabled() bool { return false }
func (*disabledPromptRecordingStore) SavePromptRecordingEnabled(context.Context, bool) error {
	return nil
}

func TestPromptServiceDoesNotQueuePromptOrResponseWhenRecordingDisabled(t *testing.T) {
	repository := &blockingPromptRecordRepository{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	close(repository.release)
	records := newPromptRecordService(repository, 1, 1, 1)
	service := &PromptService{
		config:  &disabledPromptRecordingStore{fakeConfigStore: &fakeConfigStore{}},
		records: records,
	}
	request := Request{RequestID: "disabled", Body: []byte(`{"messages":[{"role":"user","content":"private"}]}`)}

	service.RecordPrompt(context.Background(), request)
	service.RecordResponse(context.Background(), request, PromptResponse{Text: "private response", CapturedAt: time.Now()})
	time.Sleep(20 * time.Millisecond)

	require.Zero(t, repository.inserted.Load())
	require.Zero(t, records.QueueStats().QueueLength)
	require.Zero(t, records.QueueStats().OverflowLength)
}
