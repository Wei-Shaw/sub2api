package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinAccountAPIKeyRoundRobin(t *testing.T) {
	account := &Account{
		ID:   91001,
		Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":          "sk-a",
			"api_key_strategy": apiKeyStrategyRoundRobin,
			"api_key_slots": []any{
				map[string]any{"id": "primary", "weight": 1, "enabled": true},
				map[string]any{"id": "b", "weight": 1, "enabled": true},
				map[string]any{"id": "c", "weight": 1, "enabled": false},
			},
			"api_keys": []any{
				map[string]any{"id": "primary", "key": "sk-a"},
				map[string]any{"id": "b", "key": "sk-b"},
				map[string]any{"id": "c", "key": "sk-c"},
			},
		},
	}

	first := pinAccountAPIKey(account)
	second := pinAccountAPIKey(account)
	third := pinAccountAPIKey(first)

	require.Equal(t, "sk-a", account.GetCredential("api_key"))
	require.Equal(t, "sk-a", first.GetCredential("api_key"))
	require.Equal(t, "sk-b", second.GetCredential("api_key"))
	require.Equal(t, "sk-a", third.GetCredential("api_key"))
	require.NotSame(t, account, first)
}

func TestPinAccountAPIKeyWeighted(t *testing.T) {
	account := &Account{
		ID:   91002,
		Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":          "sk-light",
			"api_key_strategy": apiKeyStrategyWeighted,
			"api_key_slots": []any{
				map[string]any{"id": "primary", "weight": 1, "enabled": true},
				map[string]any{"id": "heavy", "weight": 3, "enabled": true},
			},
			"api_keys": []any{
				map[string]any{"id": "primary", "key": "sk-light"},
				map[string]any{"id": "heavy", "key": "sk-heavy"},
			},
		},
	}

	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		picked := pinAccountAPIKey(account).GetCredential("api_key")
		counts[picked]++
	}
	require.Greater(t, counts["sk-heavy"], counts["sk-light"]*2)
}

func TestPinAccountAPIKeySingleKeyUnchanged(t *testing.T) {
	account := &Account{
		ID:          91003,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-only"},
	}
	require.Same(t, account, pinAccountAPIKey(account))
}

func TestReconcileAPIKeyPoolPreservesUntouchedSecrets(t *testing.T) {
	existing := map[string]any{
		"api_key": "sk-old-primary",
		"api_key_slots": []any{
			map[string]any{"id": "primary", "weight": 1, "enabled": true},
			map[string]any{"id": "b", "label": "备用", "weight": 2, "enabled": true},
		},
		"api_keys": []any{
			map[string]any{"id": "primary", "key": "sk-old-primary"},
			map[string]any{"id": "b", "key": "sk-old-b"},
		},
	}
	incoming := map[string]any{
		"base_url":          "https://example.com",
		"api_key_strategy":  apiKeyStrategyWeighted,
		"api_key_slots": []any{
			map[string]any{"id": "primary", "weight": 1, "enabled": true},
			map[string]any{"id": "b", "label": "备用", "weight": 4, "enabled": true},
		},
		"api_keys": []any{
			map[string]any{"id": "b", "key": "sk-new-b"},
		},
	}

	out := MergePreservingSensitiveCreds(existing, incoming)
	secrets := parseAPIKeySecrets(out["api_keys"])
	require.Equal(t, "sk-old-primary", secrets["primary"])
	require.Equal(t, "sk-new-b", secrets["b"])
	require.Equal(t, apiKeyStrategyWeighted, out["api_key_strategy"])
	require.Equal(t, "sk-old-primary", out["api_key"])
}

func TestReconcileAPIKeyPoolDropsSingleSlot(t *testing.T) {
	existing := map[string]any{
		"api_key": "sk-a",
		"api_key_slots": []any{
			map[string]any{"id": "primary", "weight": 1, "enabled": true},
			map[string]any{"id": "b", "weight": 1, "enabled": true},
		},
		"api_keys": []any{
			map[string]any{"id": "primary", "key": "sk-a"},
			map[string]any{"id": "b", "key": "sk-b"},
		},
	}
	incoming := map[string]any{
		"api_key_slots": []any{
			map[string]any{"id": "primary", "weight": 1, "enabled": true},
		},
	}
	out := MergePreservingSensitiveCreds(existing, incoming)
	require.NotContains(t, out, "api_key_slots")
	require.NotContains(t, out, "api_keys")
	require.Equal(t, "sk-a", out["api_key"])
}
