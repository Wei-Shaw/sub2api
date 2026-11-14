//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	chatMetaSessionID   = "3f0c9a52-7b1e-4d6a-9c38-5e2f0a1b7c44"
	chatMetaJSONUserID  = `{"device_id":"device-1","session_id":"` + chatMetaSessionID + `"}`
	chatMetaLegacyUser  = "user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account__session_" + chatMetaSessionID
	chatMetaConverted   = `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":true}`
	chatMetaChatPrefix  = `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],`
	chatMetaNoMetaChat  = `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`
	chatMetaOnlyUserKey = `{"user_id":`
)

func chatMetaQuote(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return string(b)
}

func TestWithChatCompletionsMetadataUserID_Carries(t *testing.T) {
	for name, userID := range map[string]string{"json": chatMetaJSONUserID, "legacy": chatMetaLegacyUser} {
		t.Run(name, func(t *testing.T) {
			quoted := chatMetaQuote(t, userID)
			chat := []byte(chatMetaChatPrefix + `"metadata":{"user_id":` + quoted + `}}`)
			out := withChatCompletionsMetadataUserID(chat, []byte(chatMetaConverted))

			require.Equal(t, chatMetaOnlyUserKey+quoted+`}`, gjson.GetBytes(out, "metadata").Raw)
			require.Equal(t, userID, gjson.GetBytes(out, "metadata.user_id").String())
			rest, err := sjson.DeleteBytes(out, "metadata")
			require.NoError(t, err)
			require.Equal(t, chatMetaConverted, string(rest), "everything but metadata is unchanged")
		})
	}
}

func TestWithChatCompletionsMetadataUserID_KeepsEscapesAndOtherKeysOut(t *testing.T) {
	// The id is carried as the client's own string token, escapes included.
	userID := `{"device_id":"dév","session_id":"` + chatMetaSessionID + `"}`
	chat := []byte(chatMetaChatPrefix + `"metadata":{"trace":"x","user_id":"{\"device_id\":\"dév\",\"session_id\":\"` + chatMetaSessionID + `\"}","other":{"a":1}}}`)
	out := withChatCompletionsMetadataUserID(chat, []byte(chatMetaConverted))
	require.Equal(t, `{"user_id":"{\"device_id\":\"dév\",\"session_id\":\"`+chatMetaSessionID+`\"}"}`, gjson.GetBytes(out, "metadata").Raw)
	require.Equal(t, userID, gjson.GetBytes(out, "metadata.user_id").String())
	require.False(t, gjson.GetBytes(out, "metadata.trace").Exists())
	require.False(t, gjson.GetBytes(out, "metadata.other").Exists())
}

func TestWithChatCompletionsMetadataUserID_LeavesBodyAlone(t *testing.T) {
	cases := map[string]string{
		"no metadata":             chatMetaNoMetaChat,
		"metadata string":         chatMetaChatPrefix + `"metadata":"x"}`,
		"metadata array":          chatMetaChatPrefix + `"metadata":[{"user_id":` + chatMetaQuote(t, chatMetaJSONUserID) + `}]}`,
		"metadata null":           chatMetaChatPrefix + `"metadata":null}`,
		"metadata without id":     chatMetaChatPrefix + `"metadata":{"trace":"x"}}`,
		"user_id number":          chatMetaChatPrefix + `"metadata":{"user_id":12345}}`,
		"user_id object":          chatMetaChatPrefix + `"metadata":{"user_id":` + chatMetaJSONUserID + `}}`,
		"user_id array":           chatMetaChatPrefix + `"metadata":{"user_id":[` + chatMetaQuote(t, chatMetaJSONUserID) + `]}}`,
		"user_id null":            chatMetaChatPrefix + `"metadata":{"user_id":null}}`,
		"user_id empty":           chatMetaChatPrefix + `"metadata":{"user_id":""}}`,
		"user_id bare uuid":       chatMetaChatPrefix + `"metadata":{"user_id":"` + chatMetaSessionID + `"}}`,
		"user_id free text":       chatMetaChatPrefix + `"metadata":{"user_id":"alice"}}`,
		"json without device_id":  chatMetaChatPrefix + `"metadata":{"user_id":` + chatMetaQuote(t, `{"session_id":"`+chatMetaSessionID+`"}`) + `}}`,
		"json empty device_id":    chatMetaChatPrefix + `"metadata":{"user_id":` + chatMetaQuote(t, `{"device_id":"","session_id":"`+chatMetaSessionID+`"}`) + `}}`,
		"json without session_id": chatMetaChatPrefix + `"metadata":{"user_id":` + chatMetaQuote(t, `{"device_id":"device-1"}`) + `}}`,
		"json empty session_id":   chatMetaChatPrefix + `"metadata":{"user_id":` + chatMetaQuote(t, `{"device_id":"device-1","session_id":""}`) + `}}`,
		"json malformed":          chatMetaChatPrefix + `"metadata":{"user_id":"{\"device_id\":"}}`,
		"legacy short session":    chatMetaChatPrefix + `"metadata":{"user_id":"user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account__session_123"}}`,
		"top-level user only":     chatMetaChatPrefix + `"user":` + chatMetaQuote(t, chatMetaJSONUserID) + `}`,
	}
	for name, chat := range cases {
		t.Run(name, func(t *testing.T) {
			out := withChatCompletionsMetadataUserID([]byte(chat), []byte(chatMetaConverted))
			require.Equal(t, chatMetaConverted, string(out))
			require.False(t, gjson.GetBytes(out, "metadata").Exists())
		})
	}
}

func chatMetaStreamUpstream() *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
}

func chatMetaRequestBody(userID string) []byte {
	return []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":true,` +
		`"metadata":{"trace":"x","user_id":` + userID + `}}`)
}

func chatMetaContext(body []byte) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// The Anthropic-platform path (GatewayService.ForwardAsChatCompletions).
func TestForwardAsChatCompletions_CarriesMetadataUserID(t *testing.T) {
	for name, tc := range map[string]struct {
		userID string
		want   bool
	}{
		"json":    {chatMetaQuote(t, chatMetaJSONUserID), true},
		"legacy":  {chatMetaQuote(t, chatMetaLegacyUser), true},
		"bare id": {chatMetaQuote(t, chatMetaSessionID), false},
		"number":  {`12345`, false},
	} {
		t.Run(name, func(t *testing.T) {
			body := chatMetaRequestBody(tc.userID)
			upstream := chatMetaStreamUpstream()
			svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}
			_, err := svc.ForwardAsChatCompletions(context.Background(), chatMetaContext(body), account, body, nil)
			require.NoError(t, err)
			require.NotEmpty(t, upstream.lastBody)
			if !tc.want {
				require.False(t, gjson.GetBytes(upstream.lastBody, "metadata").Exists(), string(upstream.lastBody))
				return
			}
			require.Equal(t, `{"user_id":`+tc.userID+`}`, gjson.GetBytes(upstream.lastBody, "metadata").Raw, string(upstream.lastBody))
		})
	}
}

// OAuth accounts keep the id the mimicry synthesizes: the client's id is not carried.
func TestForwardAsChatCompletions_OAuthAccountKeepsSynthesizedMetadataUserID(t *testing.T) {
	forward := func(t *testing.T, body []byte) []byte {
		t.Helper()
		upstream := chatMetaStreamUpstream()
		svc := &GatewayService{
			cfg:          rawChatCompletionsTestConfig(),
			httpUpstream: upstream,
			identityService: NewIdentityService(&stubIdentityCache{fingerprint: &Fingerprint{
				UserAgent: "claude-cli/2.9.0 (external, cli)", ClientID: "test-client", UpdatedAt: time.Now().Unix(),
			}}),
		}
		account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"}}
		_, err := svc.ForwardAsChatCompletions(context.Background(), chatMetaContext(body), account, body, nil)
		require.NoError(t, err)
		require.NotEmpty(t, upstream.lastBody)
		return upstream.lastBody
	}

	withID := forward(t, chatMetaRequestBody(chatMetaQuote(t, chatMetaJSONUserID)))
	withoutID := forward(t, []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":true,"metadata":{"trace":"x"}}`))

	require.NotEmpty(t, gjson.GetBytes(withoutID, "metadata.user_id").String(), string(withoutID))
	require.Equal(t, gjson.GetBytes(withoutID, "metadata").Raw, gjson.GetBytes(withID, "metadata").Raw, string(withID))
	require.NotContains(t, string(withID), "device-1")
}

// The native Anthropic endpoint path (OpenAIGatewayService.forwardChatCompletionsViaNativeAnthropic).
func TestForwardChatCompletionsViaNativeAnthropic_CarriesMetadataUserID(t *testing.T) {
	for name, tc := range map[string]struct {
		userID string
		want   bool
	}{
		"json":    {chatMetaQuote(t, chatMetaJSONUserID), true},
		"legacy":  {chatMetaQuote(t, chatMetaLegacyUser), true},
		"bare id": {chatMetaQuote(t, chatMetaSessionID), false},
		"number":  {`12345`, false},
	} {
		t.Run(name, func(t *testing.T) {
			body := chatMetaRequestBody(tc.userID)
			upstream := chatMetaStreamUpstream()
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, err := svc.forwardChatCompletionsViaNativeAnthropic(context.Background(), chatMetaContext(body), nativeAnthropicTestAccount(), body, "")
			require.NoError(t, err)
			require.NotEmpty(t, upstream.lastBody)
			if !tc.want {
				require.False(t, gjson.GetBytes(upstream.lastBody, "metadata").Exists(), string(upstream.lastBody))
				return
			}
			require.Equal(t, `{"user_id":`+tc.userID+`}`, gjson.GetBytes(upstream.lastBody, "metadata").Raw, string(upstream.lastBody))
		})
	}
}
