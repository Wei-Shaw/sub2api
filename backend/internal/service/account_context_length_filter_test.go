package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestAccount_ContextLengthFilterSettings_FromExtra(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"context_length_filter": map[string]any{
				"enabled":          true,
				"mode":             "gt",
				"threshold":        4096,
				"min_input_tokens": 1200,
				"max_input_tokens": 8000,
				"allow_percent":    25,
			},
		},
	}

	cfg := account.ContextLengthFilterSettings()
	require.NotNil(t, cfg)
	require.True(t, cfg.Enabled)
	require.Equal(t, "gt", cfg.Mode)
	require.Equal(t, 4096, cfg.Threshold)
	require.Equal(t, 1200, cfg.MinInputTokens)
	require.Equal(t, 8000, cfg.MaxInputTokens)
	require.Equal(t, 25, cfg.AllowPercent)
}

func TestAccount_ContextLengthFilterAllows_DefaultsToAllow(t *testing.T) {
	account := &Account{}
	require.True(t, account.ContextLengthFilterAllows(context.Background(), 1500, "session-a"), "missing config should not block")
}

func TestAccount_ContextLengthFilterAllows_BlocksAndAllowsByPercent(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"context_length_filter": map[string]any{
				"enabled":          true,
				"min_input_tokens": 1000,
				"allow_percent":    0,
			},
		},
	}

	require.False(t, account.ContextLengthFilterAllows(context.Background(), 1501, "session-a"))

	account.Extra["context_length_filter"].(map[string]any)["allow_percent"] = 100
	require.True(t, account.ContextLengthFilterAllows(context.Background(), 1501, "session-a"))
}

func TestAccount_ContextLengthFilterAllows_ModeGreaterThanAndLessThan(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"context_length_filter": map[string]any{
				"enabled":       true,
				"mode":          "gt",
				"threshold":     1000,
				"allow_percent": 0,
			},
		},
	}
	require.True(t, account.ContextLengthFilterAllows(context.Background(), 1000, "session-a"))
	require.False(t, account.ContextLengthFilterAllows(context.Background(), 1001, "session-a"))

	account.Extra["context_length_filter"].(map[string]any)["mode"] = "lt"
	require.False(t, account.ContextLengthFilterAllows(context.Background(), 999, "session-a"))
	require.True(t, account.ContextLengthFilterAllows(context.Background(), 1000, "session-a"))
}

func TestAccount_ContextLengthFilterAllows_DeterministicPerRequest(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"context_length_filter": map[string]any{
				"enabled":          true,
				"min_input_tokens": 1000,
				"allow_percent":    50,
			},
		},
	}

	first := account.ContextLengthFilterAllows(context.Background(), 2000, "session-a")
	second := account.ContextLengthFilterAllows(context.Background(), 2000, "session-a")
	require.Equal(t, first, second)
}

func TestAccount_ContextLengthFilterAllows_UsesClientRequestIDForPercentSampling(t *testing.T) {
	account := &Account{
		ID: 6,
		Extra: map[string]any{
			"context_length_filter": map[string]any{
				"enabled":       true,
				"mode":          "gt",
				"threshold":     1000,
				"allow_percent": 50,
			},
		},
	}

	seenAllowed := false
	seenBlocked := false
	for i := 0; i < 200; i++ {
		ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, fmt.Sprintf("client-req-%d", i))
		if account.ContextLengthFilterAllows(ctx, 2000, "same-sticky-session") {
			seenAllowed = true
		} else {
			seenBlocked = true
		}
	}

	require.True(t, seenAllowed, "percent sampling should allow some client request ids")
	require.True(t, seenBlocked, "percent sampling should block some client request ids")
}
