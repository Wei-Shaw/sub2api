package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestUpdateAccountPreservesCodexTicketOnEdit(t *testing.T) {
	account := ticketTestAccount(41)
	ticket := &openAICodexTicket{Model: "gpt-6-astra", State: fakeCodexTicketState(292), Length: 292, ExpiresAt: time.Now().Add(time.Hour)}
	key := openAICodexTicketExtraKey(ticket.Model)
	account.Extra = map[string]any{key: ticket}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{41: account}}
	svc := &adminServiceImpl{accountRepo: repo}
	for _, extra := range []map[string]any{{key: map[string]any{"model": ticket.Model, "length": 292}, "custom": true}, {"custom": true}} {
		updated, err := svc.UpdateAccount(context.Background(), 41, &UpdateAccountInput{Extra: extra})
		require.NoError(t, err)
		require.Equal(t, ticket, updated.Extra[key])
		require.Equal(t, true, updated.Extra["custom"])
	}
	require.NoError(t, svc.UpdateAccountExtra(context.Background(), 41, map[string]any{key: map[string]any{"state": "spoofed"}}))
	require.Equal(t, ticket, repo.accounts[41].Extra[key])
}
func TestCreateAccountDropsUserSuppliedCodexTickets(t *testing.T) {
	account, err := buildAccountForCreate(&CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, map[string]any{"codex_turn_ticket:custom": map[string]any{"state": "spoofed"}, "custom": true})
	require.NoError(t, err)
	require.NotContains(t, account.Extra, "codex_turn_ticket:custom")
	require.Equal(t, true, account.Extra["custom"])
}
