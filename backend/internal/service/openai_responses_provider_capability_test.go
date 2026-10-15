package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeResponsesProviderCapabilities(t *testing.T) {
	zhipuCoding := &Account{
		Platform: PlatformZhipu,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": AccountModeCoding,
			"api_protocol": APIProtocolAdaptive,
		},
	}
	zhipuPayG := &Account{
		Platform: PlatformZhipu,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolAdaptive,
		},
	}

	require.True(t, zhipuCoding.SupportsNativeResponsesEndpoint())
	require.True(t, zhipuCoding.UsesNativeResponsesProtocol())
	require.True(t, zhipuCoding.SupportsCodexResponsesProtocol())
	require.Equal(t, DefaultZhipuCodingResponsesBaseURL, zhipuCoding.GetCNProtocolBaseURL(APIProtocolResponses))
	require.False(t, zhipuCoding.SupportsResponsesWebSocketHTTPBridge())
	require.False(t, zhipuPayG.SupportsNativeResponsesEndpoint())
	require.True(t, (&Account{Platform: PlatformOpenAI}).SupportsResponsesWebSocketHTTPBridge())
	require.True(t, (&Account{Platform: PlatformGrok}).SupportsResponsesWebSocketHTTPBridge())
}

func TestNativeResponsesToolAdaptationUsesCapabilities(t *testing.T) {
	accounts := []*Account{
		{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolResponses}},
		{Platform: PlatformZhipu, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolResponses, "account_mode": AccountModeCoding}},
	}
	applyPatch := []byte(`{"tools":[{"type":"custom","name":"apply_patch"}]}`)
	exec := []byte(`{"tools":[{"type":"custom","name":"exec"}]}`)
	namespace := []byte(`{"tools":[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"send_message","parameters":{"type":"object"}}]}]}`)

	for _, account := range accounts {
		require.False(t, planOpenAIResponsesAdaptation(account, applyPatch).lowerClientTools, account.Platform)
		require.True(t, planOpenAIResponsesAdaptation(account, exec).lowerClientTools, account.Platform)
		require.True(t, planOpenAIResponsesAdaptation(account, namespace).lowerClientTools, account.Platform)
		adapted, mapping, err := adaptOpenAIResponsesClientToolsForFunctionUpstream(namespace)
		require.NoError(t, err)
		require.Equal(t, "function", gjson.GetBytes(adapted, "tools.0.type").String())
		require.NotEmpty(t, mapping.NamespaceTools)
	}
}

func TestNormalizeResponsesAgentMessagesForCompatibleProvider(t *testing.T) {
	account := &Account{
		Platform: PlatformZhipu,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": AccountModeCoding,
			"api_protocol": APIProtocolResponses,
		},
	}
	body := []byte(`{"input":[{"type":"agent_message","author":"worker","recipient":"root","content":"done"}]}`)
	require.True(t, planOpenAIResponsesAdaptation(account, body).normalizeAgentMessages)

	normalized, changed, err := normalizeOpenAIResponsesAgentMessages(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(normalized, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(normalized, "input.0.role").String())
	require.False(t, gjson.GetBytes(normalized, "input.0.author").Exists())
	require.False(t, gjson.GetBytes(normalized, "input.0.recipient").Exists())
}
