//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateKiroCreditUnitPriceFromExtra(t *testing.T) {
	t.Run("accepts_missing_zero_and_positive", func(t *testing.T) {
		require.NoError(t, ValidateKiroCreditUnitPriceFromExtra(nil))
		require.NoError(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{}))
		require.NoError(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{"kiro_credit_unit_price_usd": 0}))
		require.NoError(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{"kiro_credit_unit_price_usd": 0.071}))
		require.NoError(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{"kiro_credit_unit_price_usd": json.Number("0.071")}))
	})

	t.Run("normalizes_numeric_string", func(t *testing.T) {
		extra := map[string]any{"kiro_credit_unit_price_usd": "0.071"}
		require.NoError(t, ValidateKiroCreditUnitPriceFromExtra(extra))
		require.Equal(t, 0.071, extra["kiro_credit_unit_price_usd"])
	})

	t.Run("rejects_negative_and_non_numeric", func(t *testing.T) {
		require.Error(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{"kiro_credit_unit_price_usd": -0.1}))
		require.Error(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{"kiro_credit_unit_price_usd": "nope"}))
		require.Error(t, ValidateKiroCreditUnitPriceFromExtra(map[string]any{"kiro_credit_unit_price_usd": "NaN"}))
	})
}

func TestUpdateAccount_ValidatesKiroCreditUnitPriceExtra(t *testing.T) {
	accountID := int64(201)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformKiro,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra:    map[string]any{},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"kiro_credit_unit_price_usd": -0.1,
		},
	})
	require.Error(t, err)
	require.Zero(t, repo.updateCalls)

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"kiro_credit_unit_price_usd": 0,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, float64(0), updated.Extra["kiro_credit_unit_price_usd"])

	updated, err = svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"kiro_credit_unit_price_usd": "0.071",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, repo.updateCalls)
	require.Equal(t, 0.071, updated.Extra["kiro_credit_unit_price_usd"])
}

type createAccountKiroCreditRepoStub struct {
	mockAccountRepoForGemini
	createdAccount *Account
	createCalls    int
}

func (r *createAccountKiroCreditRepoStub) Create(ctx context.Context, account *Account) error {
	r.createCalls++
	r.createdAccount = account
	return nil
}

func TestCreateAccount_ValidatesKiroCreditUnitPriceExtra(t *testing.T) {
	repo := &createAccountKiroCreditRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "kiro-oauth",
		Platform:             PlatformKiro,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			"kiro_credit_unit_price_usd": -0.1,
		},
	})
	require.Error(t, err)
	require.Zero(t, repo.createCalls)

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "kiro-oauth",
		Platform:             PlatformKiro,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			"kiro_credit_unit_price_usd": 0,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, created, repo.createdAccount)
	require.Equal(t, float64(0), created.Extra["kiro_credit_unit_price_usd"])
}
