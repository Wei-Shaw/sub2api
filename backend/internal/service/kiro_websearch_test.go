//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildKiroWebSearchMCPRequest_UsesUnderscoredMetaKeys(t *testing.T) {
	req := buildKiroWebSearchMCPRequest("golang concurrency")

	body, err := json.Marshal(req)
	require.NoError(t, err)

	require.Equal(t, "tools/call", gjson.GetBytes(body, "method").String())
	require.Equal(t, "web_search", gjson.GetBytes(body, "params.name").String())
	require.Equal(t, "golang concurrency", gjson.GetBytes(body, "params.arguments.query").String())
	require.True(t, gjson.GetBytes(body, "params.arguments._meta._isValid").Bool())
	require.Equal(t, "query", gjson.GetBytes(body, "params.arguments._meta._activePath.0").String())
	require.Equal(t, "query", gjson.GetBytes(body, "params.arguments._meta._completedPaths.0.0").String())
	require.False(t, gjson.GetBytes(body, "params.arguments._meta.isValid").Exists())
	require.False(t, gjson.GetBytes(body, "params.arguments._meta.activePath").Exists())
	require.False(t, gjson.GetBytes(body, "params.arguments._meta.completedPaths").Exists())
}

func TestWriteAnthropicMessageStart_UsesCacheEmulationUsage(t *testing.T) {
	var out bytes.Buffer
	err := writeAnthropicMessageStart(&out, "msg_test", "claude-sonnet-4-6", 100, &kiroCacheEmulationUsage{
		InputTokens:              25,
		CacheCreationInputTokens: 75,
		CacheReadInputTokens:     0,
	})
	require.NoError(t, err)
	body := out.String()
	require.Contains(t, body, `"input_tokens":25`)
	require.Contains(t, body, `"cache_creation_input_tokens":75`)
	require.Contains(t, body, `"cache_read_input_tokens":0`)
}

func TestSumKiroUsage_AddsAllFields(t *testing.T) {
	a := kiropkg.Usage{
		InputTokens:                10,
		OutputTokens:               5,
		TotalTokens:                15,
		CacheReadInputTokens:       100,
		CacheCreationInputTokens:   50,
		CacheCreation5mInputTokens: 20,
		CacheCreation1hInputTokens: 30,
		KiroCredits:                0.5,
	}
	b := kiropkg.Usage{
		InputTokens:                7,
		OutputTokens:               3,
		TotalTokens:                10,
		CacheReadInputTokens:       40,
		CacheCreationInputTokens:   10,
		CacheCreation5mInputTokens: 5,
		CacheCreation1hInputTokens: 1,
		KiroCredits:                0.25,
	}
	total := sumKiroUsage(a, b)
	require.Equal(t, 17, total.InputTokens)
	require.Equal(t, 8, total.OutputTokens)
	require.Equal(t, 25, total.TotalTokens)
	require.Equal(t, 140, total.CacheReadInputTokens)
	require.Equal(t, 60, total.CacheCreationInputTokens)
	require.Equal(t, 25, total.CacheCreation5mInputTokens)
	require.Equal(t, 31, total.CacheCreation1hInputTokens)
	require.InDelta(t, 0.75, total.KiroCredits, 1e-9)
}

func TestKiroWebSearchFinalUsageMap_ConditionalFields(t *testing.T) {
	withCredits := kiroWebSearchFinalUsageMap(kiropkg.Usage{InputTokens: 1, KiroCredits: 1.5, CacheCreation5mInputTokens: 10})
	require.Equal(t, 1.5, withCredits["_sub2api_kiro_credits"])
	require.Contains(t, withCredits, "cache_creation")

	without := kiroWebSearchFinalUsageMap(kiropkg.Usage{InputTokens: 1})
	require.NotContains(t, without, "_sub2api_kiro_credits")
	require.NotContains(t, without, "cache_creation")
}

func TestWriteKiroWebSearchTerminalEvents_EmitsDeltaAndStop(t *testing.T) {
	var out bytes.Buffer
	err := writeKiroWebSearchTerminalEvents(&out, kiropkg.Usage{
		InputTokens:  20,
		OutputTokens: 42,
		KiroCredits:  0.75,
	}, "tool_use")
	require.NoError(t, err)

	events := strings.Split(strings.TrimSpace(out.String()), "\n\n")
	require.GreaterOrEqual(t, len(events), 2)
	require.Contains(t, events[0], "event: message_delta")
	require.Contains(t, events[1], "event: message_stop")

	deltaData := strings.TrimPrefix(strings.Split(events[0], "\n")[1], "data: ")
	require.Equal(t, "tool_use", gjson.Get(deltaData, "delta.stop_reason").String())
	require.Equal(t, 20, int(gjson.Get(deltaData, "usage.input_tokens").Int()))
	require.Equal(t, 42, int(gjson.Get(deltaData, "usage.output_tokens").Int()))
	require.InDelta(t, 0.75, gjson.Get(deltaData, "usage._sub2api_kiro_credits").Float(), 1e-9)

	// 终止事件必须能被通用流处理器的终止判定识别（修复前该事件被完全抑制）。
	stopData := strings.TrimPrefix(strings.Split(events[1], "\n")[1], "data: ")
	require.True(t, anthropicStreamEventIsTerminal("message_stop", stopData))
}

func TestGetOAuthToken_KiroOAuthUsesKiroTokenProvider(t *testing.T) {
	s := &GatewayService{kiroTokenProvider: &KiroTokenProvider{}}
	account := &Account{
		ID:       7,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	token, tokenType, err := s.getOAuthToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "kiro-access-token", token)
	require.Equal(t, "oauth", tokenType)
}
