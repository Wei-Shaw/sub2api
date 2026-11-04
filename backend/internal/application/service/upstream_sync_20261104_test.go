//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestKimiConcurrency403UsesShortCooldown(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			body, err := json.Marshal(map[string]any{"error": map[string]any{"message": kimiConcurrentRequestLimitMessage}})
			require.NoError(t, err)
			before := time.Now()
			disabled := svc.HandleUpstreamError(context.Background(), &Account{ID: 13, Platform: platform, Type: AccountTypeAPIKey}, http.StatusForbidden, http.Header{}, body)
			if platform == PlatformKimi {
				require.False(t, disabled)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.tempCalls)
				require.Equal(t, 1, repo.rateLimitedCalls)
				require.WithinDuration(t, before.Add(time.Duration(defaultRateLimit429CooldownSeconds)*time.Second), repo.lastRateLimitedResetAt, 2*time.Second)
			} else {
				require.True(t, disabled)
				require.Equal(t, 1, repo.setErrorCalls)
				require.Zero(t, repo.rateLimitedCalls)
			}
		})
	}
}

func TestKimiOther403RetainsErrorPolicy(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	require.True(t, svc.HandleUpstreamError(context.Background(), &Account{ID: 14, Platform: PlatformKimi, Type: AccountTypeAPIKey}, http.StatusForbidden, http.Header{}, []byte(`{"error":{"message":"Access denied"}}`)))
	require.Equal(t, 1, repo.setErrorCalls)
	require.Zero(t, repo.rateLimitedCalls)
}

func TestAntigravitySystemIdentityPreservesUserContent(t *testing.T) {
	input := json.RawMessage(`[{"role":"developer","content":[{"type":"input_text","text":"YoU aRe Codex, an agent based on GPT-6.1. Keep this instruction."},{"type":"input_text","text":"You are Claude Code, Anthropic's official CLI for Claude. Keep this too."}]},{"role":"user","content":"You are Codex, an agent based on GPT-6.1. User quotation."}]`)
	req := &apicompat.ResponsesRequest{Model: "claude-sonnet-4-5", Instructions: "x-anthropic-billing-header: private\nYou are a Claude agent, built on Anthropic's Claude Agent SDK. Follow the user.", Input: input}
	original := string(input)
	out, err := convertAntigravityCompatResponsesToAnthropic(req)
	require.NoError(t, err)
	require.Equal(t, original, string(req.Input), "do not mutate the caller's request")
	require.NotContains(t, string(out.System), "Codex")
	require.NotContains(t, string(out.System), "Anthropic")
	require.NotContains(t, string(out.System), "billing-header")
	require.Contains(t, string(out.System), "Keep this instruction.")
	require.Contains(t, string(out.System), "Keep this too.")
	require.Contains(t, string(out.Messages[0].Content), "User quotation.")
	require.Contains(t, string(out.Messages[0].Content), "Codex")
}

func TestGrokDelegatedInputPreservesFragmentOrderAndOtherItems(t *testing.T) {
	body := []byte(`{"model":"grok-4.5","input":[{"type":"message","role":"user","content":"hello","custom":123},{"type":"agent_message","content":[{"type":"input_text","text":"Task:"},{"type":"encrypted_content","encrypted_content":"plaintext body"}]},{"type":"agent_message","content":[]},{"type":"function_call_output","call_id":"call_1","output":"result"}]}`)
	out, err := patchGrokResponsesBody(body, "grok-4.5")
	require.NoError(t, err)
	require.Equal(t, "Task:plaintext body", gjson.GetBytes(out, "input.1.content").String())
	require.Equal(t, "user", gjson.GetBytes(out, "input.1.role").String())
	require.Equal(t, int64(123), gjson.GetBytes(out, "input.0.custom").Int())
	require.Equal(t, "call_1", gjson.GetBytes(out, "input.2.call_id").String())
	require.Equal(t, int64(3), gjson.GetBytes(out, "input.#").Int())
	noAgent := []byte(`{"input":[{"role":"user","content":"hello"}]}`)
	unchanged, err := normalizeGrokAgentMessages(noAgent)
	require.NoError(t, err)
	require.Equal(t, string(noAgent), string(unchanged))
}

func TestAntigravitySystemNullTextIsNotReplaced(t *testing.T) {
	raw := json.RawMessage(`[{"type":"input_text","text":"You are Codex, an agent based on GPT-6.1. Keep this."},{"type":"input_text","text":null}]`)
	out, changed := neutralizeAntigravitySystemContent(raw)
	require.True(t, changed)
	require.Equal(t, "null", gjson.GetBytes(out, "1.text").Raw)
	require.Contains(t, gjson.GetBytes(out, "0.text").String(), "Keep this.")
}

func BenchmarkSyncImageTierFilter(b *testing.B) {
	account := &Account{Extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"1K", "4K"}}}
	for _, mode := range []string{"upstream_slice", "mask"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "mask" {
					if !account.SupportsOpenAIImageSizeTier("4K") {
						b.Fatal("tier")
					}
				} else {
					tiers := upstreamOpenAIImageSizeTiersReference(account)
					if len(tiers) != 2 || tiers[1] != "4K" {
						b.Fatal("tier")
					}
				}
			}
		})
	}
}

func BenchmarkSyncInstructionsDelta(b *testing.B) {
	body := []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("a", 1<<20) + `"}`)
	for _, mode := range []string{"upstream_scan", "event_gate"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "event_gate" {
					_ = restoreOpenAIResponsesInstructionsEchoInEvent(nil, body, "response.output_text.delta")
				} else {
					_ = restoreOpenAIResponsesInstructionsAtPath(body, "response.instructions", "null")
				}
			}
		})
	}
}

func upstreamAntigravityCompatReference(request *apicompat.ResponsesRequest) (*apicompat.AnthropicRequest, error) {
	request.Instructions = antigravity.NeutralizeSystemIdentity(request.Instructions)
	neutralizeText := func(path string, text gjson.Result) error {
		if text.Type != gjson.String {
			return nil
		}
		neutral := antigravity.NeutralizeSystemIdentity(text.String())
		if neutral == text.String() {
			return nil
		}
		var err error
		request.Input, err = sjson.SetBytes(request.Input, path, neutral)
		return err
	}
	for itemIndex, item := range gjson.ParseBytes(request.Input).Array() {
		role := item.Get("role").String()
		if role != "system" && role != "developer" {
			continue
		}
		content := item.Get("content")
		path := fmt.Sprintf("%d.content", itemIndex)
		if content.Type == gjson.String {
			if err := neutralizeText(path, content); err != nil {
				return nil, err
			}
			continue
		}
		if !content.IsArray() {
			continue
		}
		for partIndex, part := range content.Array() {
			switch part.Get("type").String() {
			case "text", "input_text", "output_text":
				if err := neutralizeText(fmt.Sprintf("%s.%d.text", path, partIndex), part.Get("text")); err != nil {
					return nil, err
				}
			}
		}
	}
	return apicompat.ResponsesToAnthropicRequest(request)
}

func BenchmarkSyncAntigravitySystemHistory(b *testing.B) {
	for _, fixture := range []string{"identity", "unchanged"} {
		var parts []map[string]string
		text := "You are Codex, an agent based on GPT-6.1. Keep custom instructions."
		if fixture == "unchanged" {
			text = "Follow the custom project instructions."
		}
		for range 32 {
			parts = append(parts, map[string]string{"type": "input_text", "text": text})
		}
		input, err := json.Marshal([]any{map[string]any{"role": "developer", "content": parts}, map[string]any{"role": "user", "content": strings.Repeat("a", 1<<20)}})
		if err != nil {
			b.Fatal(err)
		}
		for _, mode := range []string{"upstream_rewrite", "batched"} {
			b.Run(fixture+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					req := &apicompat.ResponsesRequest{Model: "claude-sonnet-4-5", Input: input}
					var err error
					if mode == "batched" {
						_, err = convertAntigravityCompatResponsesToAnthropic(req)
					} else {
						_, err = upstreamAntigravityCompatReference(req)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func upstreamOpenAIImageSizeTiersReference(a *Account) []string {
	if a == nil || a.Extra == nil {
		return nil
	}
	var values []string
	switch raw := a.Extra[AccountExtraOpenAIImageSizeTiers].(type) {
	case []any:
		for _, item := range raw {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	case []string:
		values = raw
	}
	tiers := make([]string, 0, len(openAIImageSizeTierOrder))
	for _, tier := range openAIImageSizeTierOrder {
		for _, value := range values {
			if strings.EqualFold(strings.TrimSpace(value), tier) {
				tiers = append(tiers, tier)
				break
			}
		}
	}
	if len(tiers) == 0 || len(tiers) == len(openAIImageSizeTierOrder) {
		return nil
	}
	return tiers
}
