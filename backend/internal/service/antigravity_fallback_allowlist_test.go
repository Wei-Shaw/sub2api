package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAntigravityFallbackRespectsAccountMapping(t *testing.T) {
	for _, tc := range []struct {
		name       string
		primary    string
		fallback   string
		mapping    map[string]any
		group      *Group
		wantModel  string
		wantImages int
		wantStatus int
	}{
		{name: "unlisted family", fallback: "gemini-3.8-flash-high"},
		{name: "explicit alias", fallback: "backup", mapping: map[string]any{"backup": "gemini-4-argon-low"}, wantModel: "gemini-4-argon-low"},
		{name: "allowed wildcard", fallback: "gemini-4-argon-low", mapping: map[string]any{"gemini-4-*": "gemini-4-argon-low"}, wantModel: "gemini-4-argon-low"},
		{name: "alias resolves to failed model", fallback: "backup", mapping: map[string]any{"backup": "gemini-4-argon-high"}},
		{name: "image fallback denied by group", fallback: "backup", mapping: map[string]any{"backup": "gemini-3.1-flash-image"},
			group: &Group{Platform: PlatformAntigravity}, wantStatus: http.StatusBadRequest},
		{name: "allowed image fallback uses image billing", fallback: "backup", mapping: map[string]any{"backup": "gemini-3.1-flash-image"},
			group: &Group{Platform: PlatformAntigravity, AllowImageGeneration: true}, wantModel: "gemini-3.1-flash-image", wantImages: 1},
		{name: "text fallback clears image billing", primary: "gemini-3.1-flash-image", fallback: "backup", mapping: map[string]any{"backup": "gemini-4-argon-low"},
			group: &Group{Platform: PlatformAntigravity, AllowImageGeneration: true}, wantModel: "gemini-4-argon-low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			primary := tc.primary
			if primary == "" {
				primary = "gemini-4-argon-high"
			}
			c.Set("api_key", &APIKey{ID: 1, Group: tc.group})
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/"+primary+":generateContent", bytes.NewReader(body))
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{
				{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"error":{"code":404,"message":"model not found"}}`))},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}]}}\n\n"))},
			}}
			settings := &antigravitySettingRepoStub{values: map[string]string{
				SettingKeyEnableModelFallback: "true", SettingKeyFallbackModelAntigravity: tc.fallback,
			}}
			svc := &AntigravityGatewayService{
				settingService: NewSettingService(settings, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
				tokenProvider:  &AntigravityTokenProvider{}, httpUpstream: upstream,
			}
			mapping := map[string]any{primary: primary}
			for key, target := range tc.mapping {
				mapping[key] = target
			}
			account := &Account{ID: 9, Platform: PlatformAntigravity, Type: AccountTypeOAuth,
				Status: StatusActive, Concurrency: 1, Credentials: map[string]any{
					"access_token": "test-token", "project_id": "test-project", "model_mapping": mapping,
				}}
			result, err := svc.ForwardGemini(context.Background(), c, account, primary, "generateContent", true, body, false)
			if tc.wantModel == "" {
				require.Error(t, err)
				require.Nil(t, result)
				require.Len(t, upstream.requestBodies, 1, "disallowed fallback must never reach the upstream")
				if tc.wantStatus != 0 {
					require.Equal(t, tc.wantStatus, writer.Code)
					require.Contains(t, writer.Body.String(), ImageGenerationPermissionMessage())
				}
				return
			}
			require.NoError(t, err)
			require.Len(t, upstream.requestBodies, 2)
			require.Equal(t, tc.wantModel, gjson.GetBytes(upstream.requestBodies[1], "model").String())
			require.Equal(t, tc.wantModel, result.UpstreamModel)
			require.Equal(t, tc.wantImages, result.ImageCount)
			require.Equal(t, tc.wantImages > 0, result.ImageEndpoint)
		})
	}
}
