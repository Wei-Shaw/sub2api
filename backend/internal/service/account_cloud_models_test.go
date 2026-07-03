package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountCloudModelsSnapshot(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"keep": "value",
			AccountExtraCloudModelsKey: []any{
				"grok-4.3-fast",
				map[string]any{"id": "grok-code-fast-1"},
				"grok-4.3-fast",
				"",
			},
		},
	}

	require.Equal(t, []string{"grok-4.3-fast", "grok-code-fast-1"}, account.GetCloudModelIDs())

	refreshedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	extra := account.ExtraWithCloudModels([]string{"new-model", "grok-4.3-fast", "new-model"}, refreshedAt)

	require.Equal(t, "value", extra["keep"])
	require.Equal(t, []string{"grok-4.3-fast", "new-model"}, extra[AccountExtraCloudModelsKey])
	require.Equal(t, "2026-06-06T12:00:00Z", extra[AccountExtraCloudModelsRefreshedAtKey])
}
