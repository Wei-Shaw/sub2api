//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateAccountStickyModeOmittedPreservesExistingValue(t *testing.T) {
	accountID := int64(93001)
	mode := OpenAISessionStickyModeFallbackOnly
	repo := &updateAccountCredsRepoStub{account: &Account{
		ID:                      accountID,
		Name:                    "fallback",
		Platform:                PlatformOpenAI,
		Type:                    AccountTypeOAuth,
		Status:                  StatusActive,
		OpenAISessionStickyMode: mode,
	}}

	updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{Name: "renamed"})

	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
	require.Equal(t, mode, repo.account.OpenAISessionStickyMode)
}

func TestUpdateAccountStickyModeRejectsUnknownValue(t *testing.T) {
	accountID := int64(93002)
	repo := &updateAccountCredsRepoStub{account: &Account{
		ID:       accountID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
	}}
	unknown := "sometimes"

	_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{OpenAISessionStickyMode: &unknown})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_session_sticky_mode")
	require.Zero(t, repo.updateCalls)
}
