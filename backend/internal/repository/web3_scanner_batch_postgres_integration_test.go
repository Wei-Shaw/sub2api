//go:build integration

package repository

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestWeb3ScannerBatchRepositoryRetriesIdempotentlyOnPostgres(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	walletID := fmt.Sprintf("evm_scanner_%d", suffix)
	_, err := integrationEntClient.Web3DepositWallet.Create().
		SetWalletID(walletID).
		SetAccountPath("m/44'/60'/0'").
		SetXpubFingerprint(fmt.Sprintf("%064x", suffix)).
		Save(ctx)
	require.NoError(t, err)

	user, err := integrationEntClient.User.Create().
		SetEmail(fmt.Sprintf("web3-scanner-%d@example.com", suffix)).
		SetPasswordHash("password-hash").
		Save(ctx)
	require.NoError(t, err)
	addressValue := fmt.Sprintf("0x%040x", suffix)
	address, err := integrationEntClient.Web3DepositAddress.Create().
		SetUserID(user.ID).
		SetWalletID(walletID).
		SetDerivationIndex(suffix % 2_000_000_000).
		SetAddress(addressValue).
		SetNormalizedAddress(addressValue).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC()
	scannerKey := fmt.Sprintf("postgres-idempotency-%d", suffix)
	leaseToken := fmt.Sprintf("lease-%d", suffix)
	tokenContract := fmt.Sprintf("0x%040x", suffix+1)
	cursorRepo := NewWeb3ScannerCursorRepository(integrationEntClient)
	_, err = cursorRepo.Initialize(ctx, scannerKey, 1030, tokenContract, 100)
	require.NoError(t, err)
	acquired, err := cursorRepo.AcquireLease(ctx, scannerKey, "integration-test", leaseToken, now, time.Hour)
	require.NoError(t, err)
	require.True(t, acquired)

	event, err := web3deposit.NewTransferEvent(
		web3deposit.DepositEventID{
			ChainID:  1030,
			TxHash:   common.BigToHash(big.NewInt(suffix)),
			LogIndex: 7,
		},
		105,
		common.BigToHash(big.NewInt(suffix+1)),
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress(addressValue),
		big.NewInt(1_000_000),
	)
	require.NoError(t, err)
	batch := web3deposit.ScannerBatch{
		ScannerKey:     scannerKey,
		LeaseToken:     leaseToken,
		ScannedThrough: 110,
		Now:            now,
		Config: web3deposit.ChainConfig{
			ChainID:       1030,
			TokenAddress:  common.HexToAddress(tokenContract),
			TokenDecimals: web3deposit.USDT0Decimals,
		},
		Matches: []web3deposit.MatchedTransferEvent{{
			Event: event, DepositAddressID: address.ID, UserID: user.ID,
		}},
	}

	repo := NewWeb3ScannerBatchRepository(integrationEntClient)
	first, err := repo.CommitDetectedBatch(ctx, batch)
	require.NoError(t, err)
	second, err := repo.CommitDetectedBatch(ctx, batch)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	require.Equal(t, first[0].ID, second[0].ID)
}
