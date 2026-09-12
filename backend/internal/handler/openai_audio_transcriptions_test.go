package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newAudioTranscriptionTestHandler() *OpenAIGatewayHandler {
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
	}
}

func audioTranscriptionTestRequest(t *testing.T, group *service.Group, contentType string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	groupID := int64(111)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      222,
		GroupID: &groupID,
		Group:   group,
		User:    &service.User{ID: 333},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 1})
	return c, rec
}

func audioTranscriptionTestMultipart(t *testing.T, withFile bool, fields map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if withFile {
		part, err := writer.CreateFormFile("file", "audio.wav")
		require.NoError(t, err)
		_, err = part.Write([]byte("RIFF-not-really"))
		require.NoError(t, err)
	}
	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value))
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

func TestOpenAIGatewayHandlerAudioTranscriptions_GroupSwitchRejectsBeforeParsing(t *testing.T) {
	body, contentType := audioTranscriptionTestMultipart(t, true, map[string]string{"model": "whisper-1"})
	tests := []struct {
		name  string
		group *service.Group
	}{
		{name: "no group", group: nil},
		{name: "switch off", group: &service.Group{ID: 111, Platform: service.PlatformOpenAI, AllowAudioTranscription: false}},
		{name: "non openai platform", group: &service.Group{ID: 111, Platform: service.PlatformAnthropic, AllowAudioTranscription: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := audioTranscriptionTestRequest(t, tt.group, contentType, body)
			newAudioTranscriptionTestHandler().AudioTranscriptions(c)
			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Equal(t, "permission_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
		})
	}
}

func TestOpenAIGatewayHandlerAudioTranscriptions_RejectsInvalidRequestsBeforeScheduling(t *testing.T) {
	group := &service.Group{ID: 111, Platform: service.PlatformOpenAI, AllowAudioTranscription: true}
	multipartMissingFile, multipartMissingFileType := audioTranscriptionTestMultipart(t, false, map[string]string{"model": "whisper-1"})
	multipartMissingModel, multipartMissingModelType := audioTranscriptionTestMultipart(t, true, nil)
	multipartStream, multipartStreamType := audioTranscriptionTestMultipart(t, true, map[string]string{"model": "whisper-1", "stream": "true"})
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantMessage string
	}{
		{name: "json body", contentType: "application/json", body: []byte(`{"model":"whisper-1"}`), wantMessage: "multipart/form-data is required"},
		{name: "missing file", contentType: multipartMissingFileType, body: multipartMissingFile, wantMessage: "file is required"},
		{name: "missing model", contentType: multipartMissingModelType, body: multipartMissingModel, wantMessage: "model is required"},
		{name: "stream", contentType: multipartStreamType, body: multipartStream, wantMessage: "stream is not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := audioTranscriptionTestRequest(t, group, tt.contentType, tt.body)
			newAudioTranscriptionTestHandler().AudioTranscriptions(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), tt.wantMessage)
		})
	}
}

func TestAudioTranscriptionEnabledForAPIKey(t *testing.T) {
	require.False(t, audioTranscriptionEnabledForAPIKey(nil))
	require.False(t, audioTranscriptionEnabledForAPIKey(&service.APIKey{}))
	require.False(t, audioTranscriptionEnabledForAPIKey(&service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic, AllowAudioTranscription: true}}))
	require.False(t, audioTranscriptionEnabledForAPIKey(&service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI}}))
	require.True(t, audioTranscriptionEnabledForAPIKey(&service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI, AllowAudioTranscription: true}}))
	require.True(t, audioTranscriptionEnabledForAPIKey(&service.APIKey{Group: &service.Group{Platform: service.PlatformComposite, AllowAudioTranscription: true}}))
}
