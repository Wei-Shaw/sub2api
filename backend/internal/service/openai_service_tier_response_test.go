package service

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIResponseServiceTier(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, tc := range []struct{ name, requested, observed, want string }{
			{"priority", "priority", "default", "priority"},
			{"alias", "fast", "default", "priority"},
			{"ultrafast", "ultrafast", "default", "ultrafast"},
			{"requested_flex", "flex", "default", "flex"},
			{"real_flex_downgrade", "priority", "flex", "flex"},
			{"honoured", "priority", "priority", "priority"},
			{"omitted_request", "", "default", "default"},
			{"auto_not_a_processing_tier", "auto", "default", "default"},
			{"standard_request", "default", "default", "default"},
			{"unknown_response", "priority", "future-tier", "future-tier"},
		} {
			t.Run(accountType+"/"+tc.name, func(t *testing.T) {
				account := &Account{Platform: PlatformOpenAI, Type: accountType}
				raw := []byte(fmt.Sprintf(`{"type":"response.completed","response":{"model":"gpt-5.5","service_tier":%q,"sequence":900719925474099312345,"output":[{"text":"中文<&>"}]}}`, tc.observed))
				original := bytes.Clone(raw)
				got := normalizeOpenAIResponseServiceTier(account, tc.requested, raw)
				want := tc.want
				if accountType == AccountTypeAPIKey {
					want = tc.observed
				}
				require.Equal(t, want, gjson.GetBytes(got, "response.service_tier").String())
				require.Equal(t, original, raw, "upstream diagnostics retain untouched bytes")
				require.True(t, gjson.ValidBytes(got))
				require.Equal(t, "900719925474099312345", gjson.GetBytes(got, "response.sequence").Raw)
				require.Equal(t, "中文<&>", gjson.GetBytes(got, "response.output.0.text").String())
				if want == tc.observed {
					require.Equal(t, original, got)
				}
			})
		}
	}
}

func TestNormalizeOpenAIResponseServiceTierOnlyResponseDeclarations(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, raw := range []string{
		`{"type":"response.created","response":{"model":"m","service_tier":"default"}}`,
		`{"type":"response.output_text.delta","delta":"service_tier: default"}`,
		`{"type":"response.completed","response":{"model":"m","service_tier":null}}`,
		`{"type":"response.completed","response":{"model":"m","output":[{"service_tier":"default"}]}}`,
		`{"type":"response.completed","response":{"model":"m"}}`,
		`[DONE]`,
	} {
		require.Equal(t, raw, string(normalizeOpenAIResponseServiceTier(account, "priority", []byte(raw))))
	}
	raw := []byte(`{"model":"m","service_tier":"default","usage":{"output_tokens":2}}`)
	got := normalizeOpenAIResponseServiceTier(account, "priority", raw)
	require.Equal(t, "priority", gjson.GetBytes(got, "service_tier").String())
	require.Equal(t, int64(2), gjson.GetBytes(got, "usage.output_tokens").Int())
	require.Equal(t, raw, normalizeOpenAIResponseServiceTier(nil, "priority", raw))
	require.Equal(t, raw, normalizeOpenAIResponseServiceTier(&Account{Platform: PlatformGrok, Type: AccountTypeOAuth}, "priority", raw))
}

func TestOpenAIResponseServiceTierAttemptReset(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	priority := "priority"
	setOpenAIResponseServiceTier(c, &priority)
	require.Equal(t, priority, openAIResponseServiceTier(c))
	setOpenAIResponseServiceTier(c, nil)
	require.Empty(t, openAIResponseServiceTier(c), "a filtered or omitted tier must clear the previous attempt")
}
