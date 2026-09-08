package repository

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

const testWeb3AddressAllocatorAccountXPub = "xpub6Ce9NcJvTk36xtLSrJLZqE7wtgA5deCeYs7rSQtreh4cj6ByPtrg9sD7V2FNFLPnf8heNP3FGkeV9qwfzvZNSd54JoNXVsXFYSYwHsnJxqP"

func newWeb3AddressAllocator(
	t *testing.T,
) (*web3deposit.AddressAllocator, *Web3DepositWalletRepository, *Web3DepositAddressRepository, *dbent.Client) {
	t.Helper()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	driver := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(driver)))
	t.Cleanup(func() { _ = client.Close() })

	walletRepo := NewWeb3DepositWalletRepository(client)
	addressRepo := NewWeb3DepositAddressRepository(client)
	allocator := web3deposit.NewAddressAllocator(walletRepo, walletRepo, addressRepo)
	return allocator, walletRepo, addressRepo, client
}

func TestWeb3AddressAllocatorLazilyCreatesAndReusesAddress(t *testing.T) {
	allocator, walletRepo, _, client := newWeb3AddressAllocator(t)
	ctx := context.Background()
	configured := testWeb3AddressAllocatorWallet()

	created, err := allocator.GetOrCreate(ctx, 42, configured)
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.EqualValues(t, 0, created.DerivationIndex)
	require.Equal(t, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", created.Address)
	require.Equal(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266", created.NormalizedAddress)
	require.Equal(t, web3deposit.AddressStatusActive, created.Status)

	loaded, err := allocator.GetOrCreate(ctx, 42, configured)
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)

	count, err := client.Web3DepositAddress.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	wallet, err := walletRepo.GetByWalletID(ctx, configured.WalletID)
	require.NoError(t, err)
	require.EqualValues(t, 1, wallet.NextDerivationIndex)
}

func TestWeb3AddressAllocatorConcurrentRequestsReturnOneAddress(t *testing.T) {
	allocator, _, _, client := newWeb3AddressAllocator(t)
	ctx := context.Background()
	configured := testWeb3AddressAllocatorWallet()

	const requests = 100
	addresses := make(chan web3deposit.DepositAddress, requests)
	errorsCh := make(chan error, requests)
	var waitGroup sync.WaitGroup
	for range requests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			address, err := allocator.GetOrCreate(ctx, 42, configured)
			if err != nil {
				errorsCh <- err
				return
			}
			addresses <- address
		}()
	}
	waitGroup.Wait()
	close(addresses)
	close(errorsCh)

	for err := range errorsCh {
		require.NoError(t, err)
	}
	var first web3deposit.DepositAddress
	countResults := 0
	for address := range addresses {
		if countResults == 0 {
			first = address
		} else {
			require.Equal(t, first.ID, address.ID)
			require.Equal(t, first.Address, address.Address)
		}
		countResults++
	}
	require.Equal(t, requests, countResults)

	count, err := client.Web3DepositAddress.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestWeb3AddressAllocatorRejectsDisabledExistingAddress(t *testing.T) {
	allocator, _, addressRepo, _ := newWeb3AddressAllocator(t)
	_, err := addressRepo.Create(context.Background(), web3deposit.DepositAddress{
		UserID:            42,
		WalletID:          "evm_deposit_v1",
		DerivationIndex:   0,
		Address:           "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		NormalizedAddress: "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
		Status:            web3deposit.AddressStatusDisabled,
	})
	require.NoError(t, err)

	_, err = allocator.GetOrCreate(context.Background(), 42, testWeb3AddressAllocatorWallet())
	require.ErrorIs(t, err, web3deposit.ErrAddressDisabled)
}

func TestWeb3AddressAllocatorReportsNonUserUniqueConflict(t *testing.T) {
	allocator, walletRepo, addressRepo, _ := newWeb3AddressAllocator(t)
	ctx := context.Background()
	configured := testWeb3AddressAllocatorWallet()
	verified, err := web3deposit.NewWalletIdentityVerifier(walletRepo).Verify(ctx, configured)
	require.NoError(t, err)
	derived, err := web3deposit.DeriveEVMAddress(configured.AccountXPub, 0)
	require.NoError(t, err)
	_, err = addressRepo.Create(ctx, web3deposit.DepositAddress{
		UserID:            99,
		WalletID:          verified.WalletID,
		DerivationIndex:   derived.DerivationIndex,
		Address:           derived.Address,
		NormalizedAddress: derived.NormalizedAddress,
	})
	require.NoError(t, err)

	_, err = allocator.GetOrCreate(ctx, 42, configured)
	require.ErrorIs(t, err, web3deposit.ErrAddressAllocationConflict)

	wallet, err := walletRepo.GetByWalletID(ctx, configured.WalletID)
	require.NoError(t, err)
	require.EqualValues(t, 1, wallet.NextDerivationIndex)
}

func testWeb3AddressAllocatorWallet() web3deposit.ConfiguredWallet {
	return web3deposit.ConfiguredWallet{
		WalletID:    "evm_deposit_v1",
		AccountPath: "m/44'/60'/0'",
		AccountXPub: testWeb3AddressAllocatorAccountXPub,
	}
}
