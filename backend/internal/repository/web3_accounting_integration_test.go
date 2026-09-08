//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

func TestWeb3AccountingCreditsAndTransfersExactlyOnceUnderHundredWayConcurrency(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	derivationIndex := suffix % 2_000_000_000
	walletID := fmt.Sprintf("evm_test_%d", suffix)
	_, err := integrationEntClient.Web3DepositWallet.Create().SetWalletID(walletID).
		SetAccountPath("m/44'/60'/0'").SetXpubFingerprint(fmt.Sprintf("%064x", suffix)).Save(ctx)
	require.NoError(t, err)
	user, err := integrationEntClient.User.Create().
		SetEmail(fmt.Sprintf("web3-accounting-%d@example.com", suffix)).
		SetPasswordHash("password-hash").
		Save(ctx)
	require.NoError(t, err)
	address, err := integrationEntClient.Web3DepositAddress.Create().
		SetUserID(user.ID).SetWalletID(walletID).SetDerivationIndex(derivationIndex).
		SetAddress(fmt.Sprintf("0x%040x", suffix)).SetNormalizedAddress(fmt.Sprintf("0x%040x", suffix)).Save(ctx)
	require.NoError(t, err)
	deposit, err := NewWeb3DepositRepository(integrationEntClient).Create(ctx, web3deposit.Deposit{
		UserID: user.ID, DepositAddressID: address.ID, ChainID: 1030,
		TokenContract: "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		TxHash:        fmt.Sprintf("0x%064x", suffix), LogIndex: 0, BlockNumber: 1,
		BlockHash:   fmt.Sprintf("0x%064x", suffix+1),
		FromAddress: "0x1111111111111111111111111111111111111111", ToAddress: address.NormalizedAddress,
		RawAmount: "10000000", TokenDecimals: 6, TokenAmount: "10.000000", Status: web3deposit.DepositStatusReadyToCredit,
	})
	require.NoError(t, err)

	beforePayment := tableCount(t, "payment_orders")
	beforeRedeem := tableCount(t, "redeem_codes")
	beforeAffiliate := tableCount(t, "user_affiliate_ledger")
	repo := NewWeb3AccountingRepository(integrationDB)
	runConcurrently(t, 100, func() error {
		_, err := repo.CreditDeposit(ctx, web3deposit.CreditDepositRequest{DepositID: deposit.ID})
		return err
	})

	var available, deposited string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT available_amount::text,total_deposited::text FROM web3_user_balances WHERE user_id=$1 AND asset_key='usdt'`, user.ID).Scan(&available, &deposited))
	require.Equal(t, "10.00000000", available)
	require.Equal(t, "10.00000000", deposited)

	key := fmt.Sprintf("web3-transfer-%d", suffix)
	runConcurrently(t, 100, func() error {
		_, err := repo.TransferToMainBalance(ctx, web3deposit.TransferToMainBalanceRequest{UserID: user.ID, Amount: "4", IdempotencyKey: key})
		return err
	})
	var transferCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM web3_balance_transfers WHERE idempotency_key=$1`, key).Scan(&transferCount))
	require.Equal(t, 1, transferCount)
	var web3Available, userBalance, totalRecharged string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT available_amount::text FROM web3_user_balances WHERE user_id=$1 AND asset_key='usdt'`, user.ID).Scan(&web3Available))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance::text,total_recharged::text FROM users WHERE id=$1`, user.ID).Scan(&userBalance, &totalRecharged))
	require.Equal(t, "6.00000000", web3Available)
	require.Equal(t, "4.00000000", userBalance)
	require.Equal(t, "4.00000000", totalRecharged)
	require.Equal(t, beforePayment, tableCount(t, "payment_orders"))
	require.Equal(t, beforeRedeem, tableCount(t, "redeem_codes"))
	require.Equal(t, beforeAffiliate, tableCount(t, "user_affiliate_ledger"))
}

func runConcurrently(t *testing.T, count int, operation func() error) {
	t.Helper()
	var wg sync.WaitGroup
	errors := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- operation() }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func tableCount(t *testing.T, table string) int {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count))
	return count
}
