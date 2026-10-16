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
		`{"type":"response.output_text.delta","delta":"service_tier: default"}`,
		`{"delta":"header-only SSE event without a response declaration"}`,
		`{"type":"response.created"}`,
		`{"type":"response.completed","response":{"model":"m","service_tier":null}}`,
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
	nested := normalizeOpenAIResponseServiceTier(account, "priority", []byte(`{"type":"response.completed","response":{"model":"m","output":[{"service_tier":"default"}]}}`))
	require.Equal(t, "priority", gjson.GetBytes(nested, "response.service_tier").String())
	require.Equal(t, "default", gjson.GetBytes(nested, "response.output.0.service_tier").String())
}

func TestNormalizeOpenAIResponseServiceTierMissingAndLifecycleDeclarations(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, requested := range []string{"priority", "fast", "ultrafast", "flex", "", "default"} {
			for _, eventType := range []string{"", "response.created", "response.in_progress", "response.completed"} {
				for _, missing := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/missing=%t", accountType, requested, eventType, missing), func(t *testing.T) {
						body := `{"model":"m","output":[],"sequence":900719925474099312345`
						if !missing {
							body += `,"service_tier":"default"`
						}
						body += `}`
						path := "service_tier"
						if eventType != "" {
							body = fmt.Sprintf(`{"type":%q,"response":%s}`, eventType, body)
							path = "response.service_tier"
						}
						raw := []byte(body)
						got := normalizeOpenAIResponseServiceTier(&Account{Platform: PlatformOpenAI, Type: accountType}, requested, raw)
						require.Equal(t, body, string(raw), "observer source bytes must not change")
						if accountType == AccountTypeAPIKey || requested == "" || requested == "default" {
							require.Equal(t, body, string(got))
							return
						}
						want := requested
						if want == "fast" {
							want = "priority"
						}
						require.Equal(t, want, gjson.GetBytes(got, path).String())
						require.Contains(t, string(got), `"sequence":900719925474099312345`)
					})
				}
			}
		}
	}
}

func TestOpenAIResponseServiceTierAttemptReset(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	priority := "priority"
	setOpenAIResponseServiceTier(c, &priority)
	require.Equal(t, priority, openAIResponseServiceTier(c))
	setOpenAIResponseServiceTier(c, nil)
	require.Empty(t, openAIResponseServiceTier(c), "a filtered or omitted tier must clear the previous attempt")
}
