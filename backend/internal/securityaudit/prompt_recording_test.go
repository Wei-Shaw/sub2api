package securityaudit

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type promptRecordingSettingRepository struct {
	staticSettingRepository
	writeError error
}

func (r *promptRecordingSettingRepository) SetMultiple(ctx context.Context, values map[string]string) error {
	if r.writeError != nil {
		return r.writeError
	}
	for key, value := range values {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
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

func TestPromptRecordingContentPersistsIndependentSwitches(t *testing.T) {
	repository := &promptRecordingSettingRepository{}
	manager := NewConfigManager(nil, repository, nil, prefixEncryptor{}, testTotpKeyConfig())
	ctx := context.Background()
	require.NoError(t, manager.Reload(ctx))
	headers, prompt := manager.PromptRecordingContent()
	require.True(t, headers)
	require.True(t, prompt)
	disabled := false
	require.NoError(t, manager.SavePromptRecordingSettings(ctx, nil, &disabled, nil, nil))
	require.NoError(t, manager.Reload(ctx))
	headers, prompt = manager.PromptRecordingContent()
	require.False(t, headers)
	require.True(t, prompt)
	require.NoError(t, manager.SavePromptRecordingSettings(ctx, nil, nil, &disabled, nil))
	require.NoError(t, manager.Reload(ctx))
	headers, prompt = manager.PromptRecordingContent()
	require.False(t, headers)
	require.False(t, prompt)
	repository.writeError = errors.New("database unavailable")
	enabled := true
	require.Error(t, manager.SavePromptRecordingSettings(ctx, nil, &enabled, &enabled, nil))
	headers, prompt = manager.PromptRecordingContent()
	require.False(t, headers)
	require.False(t, prompt)
}

type capturedRequestRepository struct {
	blockingPromptRecordRepository
	records      chan *PromptRecord
	responseHash string
}

func (r *capturedRequestRepository) InsertPromptRecord(_ context.Context, record *PromptRecord) error {
	r.records <- record
	return nil
}

func (r *capturedRequestRepository) UpdatePromptRecordResponse(_ context.Context, _ Request, hash string, _ PromptResponse) (bool, error) {
	r.responseHash = hash
	return true, nil
}

func TestPromptRecordingContentCombinationsRetainFullRequest(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"` + strings.Repeat("完整内容", 20000) + `"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"temperature":0.7}`
	for _, headersEnabled := range []bool{false, true} {
		for _, promptEnabled := range []bool{false, true} {
			repository := &promptRecordingSettingRepository{}
			manager := NewConfigManager(nil, repository, nil, prefixEncryptor{}, testTotpKeyConfig())
			require.NoError(t, manager.SavePromptRecordingSettings(context.Background(), nil, &headersEnabled, &promptEnabled, nil))
			repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
			service := &PromptService{config: manager, records: newPromptRecordService(repo, 1, 1, 1)}
			req := Request{RequestID: "full-request", Body: []byte(body), Headers: http.Header{"X-Test": {"first", "second"}}}
			service.RecordPrompt(context.Background(), req)
			// The queued job owns its input and the recording policy at capture time.
			req.Body[0] = '!'
			req.Headers.Set("X-Test", "changed")
			oppositeHeaders, oppositePrompt := !headersEnabled, !promptEnabled
			require.NoError(t, manager.SavePromptRecordingSettings(context.Background(), nil, &oppositeHeaders, &oppositePrompt, nil))
			select {
			case record := <-repo.records:
				if headersEnabled {
					require.JSONEq(t, `{"X-Test":["first","second"]}`, record.RequestHeaders)
				} else {
					require.Empty(t, record.RequestHeaders)
				}
				if promptEnabled {
					require.Equal(t, body, record.RequestBody)
					require.NotEmpty(t, record.PromptText)
				} else {
					require.Empty(t, record.RequestBody)
					require.Empty(t, record.PromptText)
					require.Zero(t, record.PromptLength)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("record was not saved")
			}
		}
	}
}

func TestPromptRecordingRetainsRequestsWithoutTextAndMatchesResponse(t *testing.T) {
	repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
	service := newPromptRecordService(repo, 1, 1, 1)
	req := Request{RequestID: "image-only", Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abcd"}}]}]}`)}
	service.persist(req)
	record := <-repo.records
	require.Equal(t, string(req.Body), record.RequestBody)
	require.Equal(t, "image-only", record.RequestID)
	service.persistResponse(req, PromptResponse{Text: "image response", CapturedAt: time.Now()})
	require.NotEmpty(t, record.PromptHash)
	require.Equal(t, record.PromptHash, repo.responseHash)
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
