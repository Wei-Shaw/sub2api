//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestChatGPTWebImagePublicErrorPreservesSafeFailureStage(t *testing.T) {
	for _, tc := range []struct {
		name, stage, kind string
		status            int
		accepted          bool
		publicStatus      int
		code, hint        string
	}{
		{"bootstrap rejected", "bootstrap", "authentication_or_challenge", 403, false, 502, "chatgpt_web_image_failed", "check the account's ChatGPT web session"},
		{"prepare network", "prepare", "transport", 0, false, 502, "chatgpt_web_image_failed", "check outbound connectivity and retry"},
		{"web quota", "prepare", "rate_limited", 429, false, 429, "chatgpt_web_image_rate_limited", "rate limited"},
		{"web model unavailable", "conversation", "model_not_found", 404, false, 502, "chatgpt_web_image_model_unavailable", "no image generation started and no alternate model was submitted"},
		{"uncertain model failure", "conversation", "model_not_found", 404, true, 400, "image_generation_outcome_unknown", "a new generation was not submitted"},
		{"conversation interrupted", "conversation", "transport", 0, true, 400, "image_generation_outcome_unknown", "a new generation was not submitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := chatGPTWebImagePublicError(&chatgptwebimage.Error{Stage: tc.stage, Kind: tc.kind, StatusCode: tc.status, Accepted: tc.accepted, ConversationID: "private-conversation"})
			require.Equal(t, tc.publicStatus, err.StatusCode)
			require.Equal(t, tc.code, err.Code)
			require.Contains(t, err.Message, "stage="+tc.stage)
			require.Contains(t, err.Message, "reason="+tc.kind)
			require.Contains(t, err.Message, tc.hint)
			require.NotContains(t, err.Message, "private-conversation")
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			if tc.accepted {
				require.NotContains(t, err.Message, "no image generation started")
			}
		})
	}
}

func TestGenerateWebImagePreservesTypedErrorAndLogsAttribution(t *testing.T) {
	account := webImageTestAccount()
	account.ID = 239
	account.Credentials["access_token"] = "private-access-token"
	proxyID := int64(2)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{Protocol: "invalid-protocol", Host: "private-proxy-host", Port: 1234, Username: "private-user", Password: "private-password"}
	core, logs := observer.New(zap.ErrorLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core).With(zap.String("request_id", "req-web-image")))
	// Image jobs detach cancellation while retaining attribution.
	_, err := (&OpenAIGatewayService{}).generateWebImage(context.WithoutCancel(ctx), account, "private prompt")
	var providerErr *chatgptwebimage.Error
	require.ErrorAs(t, err, &providerErr)
	require.Equal(t, "initialize", providerErr.Stage)
	require.False(t, providerErr.Accepted)
	var publicErr *OpenAIImagesUpstreamError
	require.False(t, errors.As(err, &publicErr), "the job store must receive original acceptance metadata")
	entries := logs.FilterMessage("chatgpt_web_image_failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, int64(239), fields["account_id"])
	require.Equal(t, "req-web-image", fields["request_id"])
	require.Equal(t, "initialize", fields["provider_stage"])
	require.Equal(t, "invalid_configuration", fields["provider_error_kind"])
	require.Equal(t, false, fields["generation_may_have_started"])
	serialized, marshalErr := json.Marshal(fields)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(serialized), "private")
	require.Equal(t, http.StatusBadGateway, chatGPTWebImagePublicError(err).StatusCode)
}

func TestGenerateWebImageCredentialFailureIsKnownNotStarted(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))
	_, err := (&OpenAIGatewayService{}).generateWebImage(ctx, webImageTestAccount(), "private prompt")
	var providerErr *chatgptwebimage.Error
	require.ErrorAs(t, err, &providerErr)
	require.Equal(t, "credentials", providerErr.Stage)
	require.False(t, providerErr.Accepted)
	require.Equal(t, "chatgpt_web_image_credentials_unavailable", chatGPTWebImagePublicError(err).Code)
	require.Len(t, logs.FilterMessage("chatgpt_web_image_failed").All(), 1)
}
