package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelRoutingRule_UnmarshalLegacyArray(t *testing.T) {
	var rule ModelRoutingRule
	require.NoError(t, json.Unmarshal([]byte(`[1,2,0,-3]`), &rule))
	require.Equal(t, 0, rule.MaxConcurrency)
	require.Equal(t, []ModelRoutingAccountRef{
		{ID: 1, Priority: DefaultModelRoutingAccountPriority, Concurrency: 0},
		{ID: 2, Priority: DefaultModelRoutingAccountPriority, Concurrency: 0},
	}, rule.Accounts)
}

func TestModelRoutingRule_UnmarshalStructured(t *testing.T) {
	raw := `{"max_concurrency":10,"accounts":[{"id":7,"priority":0,"concurrency":3},{"id":8,"priority":20}]}`
	var rule ModelRoutingRule
	require.NoError(t, json.Unmarshal([]byte(raw), &rule))
	require.Equal(t, 10, rule.MaxConcurrency)
	require.Equal(t, []ModelRoutingAccountRef{
		{ID: 7, Priority: 0, Concurrency: 3},
		{ID: 8, Priority: 20, Concurrency: 0},
	}, rule.Accounts)
}

func TestModelRoutingMap_RoundTripNormalizesWriteShape(t *testing.T) {
	var m ModelRoutingMap
	require.NoError(t, json.Unmarshal([]byte(`{"claude-*":[9,10]}`), &m))
	out, err := json.Marshal(m)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"claude-*": {
			"max_concurrency": 0,
			"accounts": [
				{"id": 9, "priority": 50, "concurrency": 0},
				{"id": 10, "priority": 50, "concurrency": 0}
			]
		}
	}`, string(out))
}

func TestMatchModelRouting(t *testing.T) {
	routing := ModelRoutingMap{
		"claude-opus-*": {Accounts: []ModelRoutingAccountRef{{ID: 1, Priority: 10}}},
		"exact":         {Accounts: []ModelRoutingAccountRef{{ID: 2, Priority: 5}}},
	}
	matched, ok := MatchModelRouting(routing, "exact")
	require.True(t, ok)
	require.Equal(t, "exact", matched.Pattern)
	require.Equal(t, int64(2), matched.Rule.Accounts[0].ID)

	matched, ok = MatchModelRouting(routing, "claude-opus-4")
	require.True(t, ok)
	require.Equal(t, "claude-opus-*", matched.Pattern)

	_, ok = MatchModelRouting(routing, "gpt-4")
	require.False(t, ok)
}

func TestNormalizeModelRoutingMap(t *testing.T) {
	in := ModelRoutingMap{
		"":              {Accounts: []ModelRoutingAccountRef{{ID: 1}}},
		"keep":          {MaxConcurrency: -1, Accounts: []ModelRoutingAccountRef{{ID: 1}, {ID: 1}, {ID: 0, Priority: 1}}},
		"empty":         {MaxConcurrency: 5},
		"with-accounts": {Accounts: []ModelRoutingAccountRef{{ID: 3, Concurrency: -2}}},
	}
	out := NormalizeModelRoutingMap(in)
	require.Len(t, out, 2)
	require.Equal(t, 0, out["keep"].MaxConcurrency)
	require.Len(t, out["keep"].Accounts, 1)
	require.Equal(t, 0, out["with-accounts"].Accounts[0].Concurrency)
}
