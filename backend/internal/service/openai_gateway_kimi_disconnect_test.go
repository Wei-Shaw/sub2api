//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kimiDisconnectTrackingBody struct {
	closed bool
}

func (b *kimiDisconnectTrackingBody) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (b *kimiDisconnectTrackingBody) Close() error {
	b.closed = true
	return nil
}

func newKimiDisconnectTestService(t *testing.T, enabled bool) *OpenAIGatewayService {
	t.Helper()
	resetGatewayForwardingSettingsCacheForTest(t)
	return &OpenAIGatewayService{
		settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
			SettingKeyCancelKimiUpstreamOnClientDisconnect: strconv.FormatBool(enabled),
		}}, &config.Config{}),
	}
}

func kimiDisconnectTestAccount(platform string, protocol string) *Account {
	return &Account{
		ID:       21,
		Platform: platform,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_protocol": protocol,
		},
	}
}

func TestKimiUpstreamContext_ClientDisconnectScope(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		account      *Account
		stream       bool
		chatPipeline bool
		wantCanceled bool
	}{
		{
			name:         "disabled Kimi Chat Completions stream remains detached",
			enabled:      false,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolChatCompletions),
			stream:       true,
			chatPipeline: true,
			wantCanceled: false,
		},
		{
			name:         "enabled Kimi Chat Completions stream inherits cancellation",
			enabled:      true,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolChatCompletions),
			stream:       true,
			chatPipeline: true,
			wantCanceled: true,
		},
		{
			name:         "enabled Kimi Anthropic stream inherits cancellation",
			enabled:      true,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolAnthropic),
			stream:       true,
			wantCanceled: true,
		},
		{
			name:         "disabled Kimi Anthropic stream remains detached",
			enabled:      false,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolAnthropic),
			stream:       true,
			wantCanceled: false,
		},
		{
			name:         "Kimi adaptive stream is excluded",
			enabled:      true,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolAdaptive),
			stream:       true,
			wantCanceled: false,
		},
		{
			name:         "other providers remain detached",
			enabled:      true,
			account:      kimiDisconnectTestAccount(PlatformOpenAI, APIProtocolChatCompletions),
			stream:       true,
			chatPipeline: true,
			wantCanceled: false,
		},
		{
			name:         "Chat Completions non-stream keeps historical detached context",
			enabled:      true,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolChatCompletions),
			stream:       false,
			chatPipeline: true,
			wantCanceled: false,
		},
		{
			name:         "native Anthropic non-stream keeps historical client context",
			enabled:      true,
			account:      kimiDisconnectTestAccount(PlatformKimi, APIProtocolAnthropic),
			stream:       false,
			wantCanceled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newKimiDisconnectTestService(t, tt.enabled)
			clientCtx, cancelClient := context.WithCancel(context.Background())

			var upstreamCtx context.Context
			var release context.CancelFunc
			if tt.chatPipeline {
				upstreamCtx, release = svc.kimiCCUpstreamContext(clientCtx, tt.account, tt.stream)
			} else {
				upstreamCtx, release = svc.kimiStreamUpstreamContext(clientCtx, tt.account, tt.stream)
			}
			defer release()

			cancelClient()
			canceled := false
			select {
			case <-upstreamCtx.Done():
				canceled = true
			default:
			}
			require.Equal(t, tt.wantCanceled, canceled)
		})
	}
}

func TestCloseKimiUpstreamAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		enabled    bool
		account    *Account
		wantClosed bool
	}{
		{
			name:       "Kimi Chat Completions closes upstream",
			enabled:    true,
			account:    kimiDisconnectTestAccount(PlatformKimi, APIProtocolChatCompletions),
			wantClosed: true,
		},
		{
			name:       "Kimi Anthropic closes upstream",
			enabled:    true,
			account:    kimiDisconnectTestAccount(PlatformKimi, APIProtocolAnthropic),
			wantClosed: true,
		},
		{
			name:       "disabled switch keeps upstream open",
			enabled:    false,
			account:    kimiDisconnectTestAccount(PlatformKimi, APIProtocolChatCompletions),
			wantClosed: false,
		},
		{
			name:       "Kimi adaptive keeps upstream open",
			enabled:    true,
			account:    kimiDisconnectTestAccount(PlatformKimi, APIProtocolAdaptive),
			wantClosed: false,
		},
		{
			name:       "other provider keeps upstream open",
			enabled:    true,
			account:    kimiDisconnectTestAccount(PlatformOpenAI, APIProtocolChatCompletions),
			wantClosed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newKimiDisconnectTestService(t, tt.enabled)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			body := &kimiDisconnectTrackingBody{}
			resp := &http.Response{Body: body}

			canceled := svc.closeKimiUpstreamAfterClientDisconnect(c, resp, tt.account)

			require.Equal(t, tt.wantClosed, canceled)
			require.Equal(t, tt.wantClosed, body.closed)
		})
	}
}

func TestKimiDisconnectSetting_DefaultAndPersistence(t *testing.T) {
	originalModelMappingOptions := xai.RuntimeModelMappingOptions()
	t.Cleanup(func() {
		xai.SetRuntimeModelMappingOptions(originalModelMappingOptions)
	})

	t.Run("missing setting defaults to false", func(t *testing.T) {
		svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{}}, &config.Config{})

		settings, err := svc.GetAllSettings(context.Background())

		require.NoError(t, err)
		require.False(t, settings.CancelKimiUpstreamOnClientDisconnect)
	})

	t.Run("UpdateSettings persists enabled value", func(t *testing.T) {
		resetGatewayForwardingSettingsCacheForTest(t)
		repo := &settingUpdateRepoStub{}
		svc := NewSettingService(repo, &config.Config{})

		err := svc.UpdateSettings(context.Background(), &SystemSettings{
			CancelKimiUpstreamOnClientDisconnect: true,
		})

		require.NoError(t, err)
		require.Equal(t, "true", repo.updates[SettingKeyCancelKimiUpstreamOnClientDisconnect])
	})
}
