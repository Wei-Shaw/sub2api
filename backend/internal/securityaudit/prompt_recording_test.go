package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
	headers, prompt, response := manager.PromptRecordingContent()
	require.True(t, headers)
	require.True(t, prompt)
	require.True(t, response)
	agentPreset, skills := manager.PromptRecordingPresetFilters()
	require.True(t, agentPreset)
	require.True(t, skills)

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
	headers, prompt, response := manager.PromptRecordingContent()
	require.True(t, headers)
	require.True(t, prompt)
	require.True(t, response)
	disabled := false
	require.NoError(t, manager.SavePromptRecordingSettings(ctx, PromptRecordingSettingsUpdate{HeadersEnabled: &disabled}))
	require.NoError(t, manager.Reload(ctx))
	headers, prompt, response = manager.PromptRecordingContent()
	require.False(t, headers)
	require.True(t, prompt)
	require.True(t, response)
	require.NoError(t, manager.SavePromptRecordingSettings(ctx, PromptRecordingSettingsUpdate{PromptEnabled: &disabled, ResponseEnabled: &disabled, FilterAgentPreset: &disabled, FilterSkills: &disabled}))
	require.NoError(t, manager.Reload(ctx))
	headers, prompt, response = manager.PromptRecordingContent()
	require.False(t, headers)
	require.False(t, prompt)
	require.False(t, response)
	agentPreset, skills := manager.PromptRecordingPresetFilters()
	require.False(t, agentPreset)
	require.False(t, skills)
	repository.writeError = errors.New("database unavailable")
	enabled := true
	require.Error(t, manager.SavePromptRecordingSettings(ctx, PromptRecordingSettingsUpdate{HeadersEnabled: &enabled, PromptEnabled: &enabled, ResponseEnabled: &enabled}))
	headers, prompt, response = manager.PromptRecordingContent()
	require.False(t, headers)
	require.False(t, prompt)
	require.False(t, response)
}

type capturedRequestRepository struct {
	blockingPromptRecordRepository
	records    chan *PromptRecord
	responseID int64
}

func (r *capturedRequestRepository) InsertPromptRecord(_ context.Context, record *PromptRecord) (int64, error) {
	r.records <- record
	return 37, nil
}

func (r *capturedRequestRepository) UpdatePromptRecordResponse(_ context.Context, key PromptRecordKey, _ PromptResponse) (bool, error) {
	r.responseID = key.ID
	return true, nil
}

func TestPromptRecordingContentCombinationsRetainFullRequest(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"` + strings.Repeat("完整内容", 20000) + `"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"temperature":0.7}`
	for _, headersEnabled := range []bool{false, true} {
		for _, promptEnabled := range []bool{false, true} {
			repository := &promptRecordingSettingRepository{}
			manager := NewConfigManager(nil, repository, nil, prefixEncryptor{}, testTotpKeyConfig())
			require.NoError(t, manager.SavePromptRecordingSettings(context.Background(), PromptRecordingSettingsUpdate{HeadersEnabled: &headersEnabled, PromptEnabled: &promptEnabled}))
			repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
			service := &PromptService{config: manager, records: newPromptRecordService(repo, 1, 1, 1)}
			req := Request{RequestID: "full-request", APIKeyID: 9, APIKeyName: "primary", Body: []byte(body), Headers: http.Header{"Session-Id": {"session-full-request"}, "X-Test": {"first", "second"}}}
			service.RecordPrompt(context.Background(), req)
			// The queued job owns its input and the recording policy at capture time.
			req.Body[0] = '!'
			req.Headers.Set("X-Test", "changed")
			oppositeHeaders, oppositePrompt := !headersEnabled, !promptEnabled
			require.NoError(t, manager.SavePromptRecordingSettings(context.Background(), PromptRecordingSettingsUpdate{HeadersEnabled: &oppositeHeaders, PromptEnabled: &oppositePrompt}))
			select {
			case record := <-repo.records:
				require.Equal(t, "session-full-request", record.SessionID)
				require.Equal(t, int64(9), record.APIKeyID)
				require.Equal(t, "primary", record.APIKeyName)
				if headersEnabled {
					require.JSONEq(t, `{"Session-Id":["session-full-request"],"X-Test":["first","second"]}`, record.RequestHeaders)
				} else {
					require.Empty(t, record.RequestHeaders)
				}
				if promptEnabled {
					require.JSONEq(t, body, record.RequestBody)
					require.Empty(t, record.PromptText, "new records retain one canonical request document")
					require.Positive(t, record.PromptLength)
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

func TestPreparePromptRecordKeepsOnlyLatestThirtyMessages(t *testing.T) {
	messages := make([]any, 35)
	for index := range messages {
		messages[index] = map[string]any{"role": "user", "content": "message-" + strconv.Itoa(index)}
	}
	body, err := json.Marshal(map[string]any{"messages": messages})
	require.NoError(t, err)

	prepared, err := preparePromptRecord(Request{Protocol: "openai_chat", Body: body})
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal(prepared.StoredBody, &stored))
	items, ok := stored["messages"].([]any)
	require.True(t, ok)
	require.Len(t, items, promptRecordMaxMessages)
	for index, item := range items {
		message, ok := item.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "message-"+strconv.Itoa(index+5), message["content"])
	}
	require.Equal(t, promptRecordMaxMessages, prepared.StoredSnapshot.MessageCount)
	// The full request identity remains based on the original request.
	original := promptRecordMetadataDocument(Request{Protocol: "openai_chat"}, mustDecodePromptDocument(t, body), body, false)
	require.Equal(t, original.PromptHash, prepared.OriginalPromptHash)
}

func mustDecodePromptDocument(t *testing.T, body []byte) any {
	t.Helper()
	document, err := decodePromptDocument(body)
	require.NoError(t, err)
	return document
}

func TestPromptRecordingRetainsRequestsWithoutTextAndMatchesResponse(t *testing.T) {
	repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
	service := newPromptRecordService(repo, 1, 1, 1)
	req := Request{RequestID: "image-only", Headers: http.Header{"Session-Id": {"session-image"}}, Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abcd"}}]}]}`)}
	recordRequest, responseReference := newPromptRecordingRequestPair(req)
	service.persist(recordRequest)
	record := <-repo.records
	require.NotContains(t, record.RequestBody, "image_url")
	require.NotContains(t, record.RequestBody, "abcd")
	require.Equal(t, "session-image", record.SessionID)
	service.persistResponse(responseReference, PromptResponse{Text: "image response", CapturedAt: time.Now()})
	require.NotEmpty(t, record.PromptHash)
	require.EqualValues(t, 37, repo.responseID)
}

func TestPreparePromptRecordReturnsStoredBodyTextAndOriginalHash(t *testing.T) {
	body := []byte(`{"model":"test","seed":9007199254740993,"messages":[{"role":"system","content":"You are Codex"},{"role":"user","content":[{"type":"text","text":"keep"},{"type":"image_url","image_url":{"url":"data:image/png;base64,IMAGE"}}]}]}`)
	req := Request{RequestID: "prepared", Protocol: "openai_chat", Body: body}
	original := promptRecordSnapshot(req)

	prepared, err := preparePromptRecord(req)
	require.NoError(t, err)
	require.Equal(t, original.PromptHash, prepared.OriginalPromptHash)
	require.Contains(t, string(prepared.StoredBody), `"seed":9007199254740993`)
	require.NotContains(t, string(prepared.StoredBody), "IMAGE")
	require.NotContains(t, string(prepared.StoredBody), "image_url")
	require.Contains(t, prepared.StoredSnapshot.FullPrompt, "keep")
	require.Equal(t, body, req.Body)
}

func TestPromptServiceDoesNotQueueResponseWhenResponseRecordingDisabled(t *testing.T) {
	repository := &promptRecordingSettingRepository{}
	manager := NewConfigManager(nil, repository, nil, prefixEncryptor{}, testTotpKeyConfig())
	disabled := false
	require.NoError(t, manager.SavePromptRecordingSettings(context.Background(), PromptRecordingSettingsUpdate{ResponseEnabled: &disabled}))
	recordStore := &blockingPromptRecordRepository{started: make(chan struct{}, 1), release: make(chan struct{})}
	close(recordStore.release)
	service := &PromptService{config: manager, records: newPromptRecordService(recordStore, 1, 1, 1)}

	service.RecordResponse(context.Background(), Request{RequestID: "response-disabled"}, PromptResponse{Text: "private response"})
	time.Sleep(20 * time.Millisecond)

	require.Zero(t, recordStore.inserted.Load())
	require.Zero(t, service.records.QueueStats().QueueLength)
	require.Zero(t, service.records.QueueStats().OverflowLength)
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
