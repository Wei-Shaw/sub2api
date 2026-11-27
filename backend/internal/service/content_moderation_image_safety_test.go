package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsOpenAIImagesSafetyRefusal(t *testing.T) {
	const safetyMsg = "Your request was rejected by the safety system. If you believe this is an error, contact us at help.openai.com and include the request ID b553d2f5."
	tests := []struct {
		name string
		err  *OpenAIImagesUpstreamError
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "issue 8992: 400 safety system message without type or code",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"`+safetyMsg+`"}}`)),
			want: true,
		},
		{
			name: "400 safety system message with numeric code 400",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"`+safetyMsg+`","code":400}}`)),
			want: true,
		},
		{
			name: "moderation_blocked code",
			err:  &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "image_generation_user_error", Code: "moderation_blocked", Message: "blocked"},
			want: true,
		},
		{
			name: "responses text fallback refusal",
			err:  openAIImagesTextFallbackErrorForText("I can't help with that because it violates our content policy."),
			want: true,
		},
		{
			name: "response.incomplete content_filter",
			err:  &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "image_generation_user_error", Code: "response_incomplete", Message: "Upstream image generation incomplete: content_filter"},
			want: true,
		},
		{
			name: "invalid moderation parameter is a validation error",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"Invalid value: 'strict'. Supported values for moderation are: 'low' and 'auto'.","type":"invalid_request_error","param":"moderation","code":"invalid_value"}}`)),
			want: false,
		},
		{
			name: "plain invalid size 400",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"Invalid size","code":400}}`)),
			want: false,
		},
		{
			name: "safety wording on non-400 status",
			err:  &OpenAIImagesUpstreamError{StatusCode: http.StatusBadGateway, Message: safetyMsg},
			want: false,
		},
		{
			name: "azure: unrecognized moderation argument with null param",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"Unrecognized request argument supplied: moderation","type":"invalid_request_error","param":null,"code":null}}`)),
			want: false,
		},
		{
			name: "unknown moderation parameter without param field",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"Unknown parameter: 'moderation'."}}`)),
			want: false,
		},
		{
			name: "error passthrough rewrite drops param and code",
			err:  &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "upstream_error", Message: "Invalid moderation setting for this model"},
			want: false,
		},
		{
			name: "azure content filter with param prompt",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"The response was filtered due to the prompt triggering Azure OpenAI's content management policy.","type":null,"param":"prompt","code":"content_filter"}}`)),
			want: true,
		},
		{
			name: "azure contentFilter code",
			err:  openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"code":"contentFilter","message":"Your task failed as a result of our safety system."}}`)),
			want: true,
		},
		{
			name: "response.incomplete moderation reason",
			err:  openAIImagesIncompleteUpstreamError(gjson.Parse(`{"id":"resp_1","incomplete_details":{"reason":"moderation"}}`)),
			want: true,
		},
		{
			name: "response.incomplete max_output_tokens is retryable, not safety",
			err:  openAIImagesIncompleteUpstreamError(gjson.Parse(`{"id":"resp_1","incomplete_details":{"reason":"max_output_tokens"}}`)),
			want: false,
		},
		{
			name: "safety wording with param set is excluded on the message path",
			err:  &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, Param: "size", Message: "content policy for size values"},
			want: false,
		},
		{
			name: "text fallback capability failure",
			err:  openAIImagesTextFallbackErrorForText("Here is a description of the picture instead."),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsOpenAIImagesSafetyRefusal(tt.err))
		})
	}
}

func newImageSafetyTestService(t *testing.T, settings map[string]string) (*ContentModerationService, *contentModerationTestRepo) {
	t.Helper()
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: settings},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return svc, repo
}

func imageSafetyTestInput(groupID *int64) ImageSafetyRecordInput {
	return ImageSafetyRecordInput{
		RequestID:         "req-1",
		UserID:            42,
		UserEmail:         "u@example.com",
		APIKeyID:          9,
		APIKeyName:        "key",
		GroupID:           groupID,
		GroupName:         "g",
		Endpoint:          "/v1/images/generations",
		Provider:          PlatformOpenAI,
		Model:             "gpt-image-2",
		Prompt:            "draw something sk-abcdefghijklmnopqrstuvwxyz0123456789",
		UpstreamStatus:    http.StatusBadRequest,
		UpstreamType:      "invalid_request_error",
		UpstreamMessage:   "Your request was rejected by the safety system.",
		UpstreamRequestID: "b553d2f5",
	}
}

func TestRecordImageSafetyEvent_DisabledWhenRiskControlOff(t *testing.T) {
	svc, repo := newImageSafetyTestService(t, map[string]string{SettingKeyRiskControlEnabled: "false"})
	svc.RecordImageSafetyEvent(context.Background(), imageSafetyTestInput(nil))
	require.Empty(t, repo.snapshotLogs())
}

func TestRecordImageSafetyEvent_WritesLogOnlyEvidence(t *testing.T) {
	svc, repo := newImageSafetyTestService(t, map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: `{"enabled":false,"mode":"off","all_groups":true,"auto_ban_enabled":true,"ban_threshold":1}`,
	})
	svc.RecordImageSafetyEvent(context.Background(), imageSafetyTestInput(nil))

	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	log := logs[0]
	require.Equal(t, ContentModerationActionImageSafety, log.Action)
	require.Equal(t, ContentModerationModeRiskControlLogOnly, log.Mode, "image safety evidence must never feed the auto-ban count")
	require.True(t, log.Flagged)
	require.Equal(t, ContentModerationActionImageSafety, log.HighestCategory)
	require.Equal(t, 1.0, log.HighestScore)
	require.NotNil(t, log.UserID)
	require.Equal(t, int64(42), *log.UserID)
	require.Equal(t, "u@example.com", log.UserEmail)
	require.Equal(t, "/v1/images/generations", log.Endpoint)
	require.Equal(t, PlatformOpenAI, log.Provider)
	require.Equal(t, "gpt-image-2", log.Model)
	require.Contains(t, log.InputExcerpt, "draw something")
	require.NotContains(t, log.InputExcerpt, "sk-abcdefghijklmnopqrstuvwxyz0123456789", "prompt excerpt must be redacted")
	require.Contains(t, log.Error, "upstream_status=400")
	require.Contains(t, log.Error, "upstream_request_id=b553d2f5")
	require.Contains(t, log.Error, "safety system")
	require.False(t, log.AutoBanned)
	require.False(t, log.EmailSent)
	require.Zero(t, log.ViolationCount)
}

func TestRecordImageSafetyEvent_RespectsScope(t *testing.T) {
	groupID := int64(7)
	tests := []struct {
		name     string
		config   string
		groupID  *int64
		wantLogs int
	}{
		{name: "excluded group", config: `{"all_groups":false,"group_ids":[8]}`, groupID: &groupID, wantLogs: 0},
		{name: "excluded model", config: `{"all_groups":true,"model_filter":{"type":"include","models":["gpt-5"]}}`, groupID: &groupID, wantLogs: 0},
		{name: "included", config: `{"all_groups":false,"group_ids":[7],"model_filter":{"type":"include","models":["gpt-image-2"]}}`, groupID: &groupID, wantLogs: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newImageSafetyTestService(t, map[string]string{
				SettingKeyRiskControlEnabled:      "true",
				SettingKeyContentModerationConfig: tt.config,
			})
			svc.RecordImageSafetyEvent(context.Background(), imageSafetyTestInput(tt.groupID))
			require.Len(t, repo.snapshotLogs(), tt.wantLogs)
		})
	}
}
