package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountListItemZhipuResetCardsBindingMarker(t *testing.T) {
	account := &service.Account{
		Platform:    service.PlatformZhipu,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": service.AccountModeCoding},
		Extra: map[string]any{service.ZhipuResetCardsExtraKey: map[string]any{
			"id": "private-binding-id", "ciphertext": "private-sealed-session",
		}},
	}
	raw, err := json.Marshal(AccountListItemFromAccount(AccountFromServiceShallow(account)))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"zhipu_reset_cards_bound":true`)
	require.NotContains(t, string(raw), "private-binding-id")
	require.NotContains(t, string(raw), "private-sealed-session")
	account.Credentials["zhipu_organization"] = "team"
	require.False(t, AccountFromServiceShallow(account).ZhipuResetCardsBound)
}
