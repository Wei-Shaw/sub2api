package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type DerivationIndexReserver interface {
	ReserveDerivationIndex(ctx context.Context, verifiedWallet WalletMetadata) (int64, error)
}

type DepositAddressStore interface {
	Create(ctx context.Context, address DepositAddress) (DepositAddress, error)
	GetByUserAndWallet(ctx context.Context, userID int64, walletID string) (DepositAddress, error)
}

type AddressAllocator struct {
	walletVerifier *WalletIdentityVerifier
	indexReserver  DerivationIndexReserver
	addressStore   DepositAddressStore
}

func NewAddressAllocator(
	walletStore WalletMetadataStore,
	indexReserver DerivationIndexReserver,
	addressStore DepositAddressStore,
) *AddressAllocator {
	return &AddressAllocator{
		walletVerifier: NewWalletIdentityVerifier(walletStore),
		indexReserver:  indexReserver,
		addressStore:   addressStore,
	}
}

func (a *AddressAllocator) GetOrCreate(
	ctx context.Context,
	userID int64,
	configuredWallet ConfiguredWallet,
) (DepositAddress, error) {
	walletID := strings.TrimSpace(configuredWallet.WalletID)
	existing, err := a.addressStore.GetByUserAndWallet(ctx, userID, walletID)
	if err == nil {
		return usableDepositAddress(existing)
	}
	if !errors.Is(err, ErrAddressNotFound) {
		return DepositAddress{}, fmt.Errorf("get existing web3 deposit address: %w", err)
	}

	verifiedWallet, err := a.walletVerifier.Verify(ctx, configuredWallet)
	if err != nil {
		return DepositAddress{}, err
	}
	derivationIndex, err := a.indexReserver.ReserveDerivationIndex(ctx, verifiedWallet)
	if err != nil {
		return DepositAddress{}, err
	}
	derived, err := DeriveEVMAddress(configuredWallet.AccountXPub, derivationIndex)
	if err != nil {
		return DepositAddress{}, err
	}

	created, err := a.addressStore.Create(ctx, DepositAddress{
		UserID:            userID,
		WalletID:          verifiedWallet.WalletID,
		DerivationIndex:   derived.DerivationIndex,
		Address:           derived.Address,
		NormalizedAddress: derived.NormalizedAddress,
		Status:            AddressStatusActive,
	})
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, ErrAddressAlreadyExists) {
		return DepositAddress{}, fmt.Errorf("create web3 deposit address: %w", err)
	}

	existing, getErr := a.addressStore.GetByUserAndWallet(ctx, userID, verifiedWallet.WalletID)
	if getErr == nil {
		return usableDepositAddress(existing)
	}
	if !errors.Is(getErr, ErrAddressNotFound) {
		return DepositAddress{}, fmt.Errorf("resolve web3 deposit address conflict: %w", getErr)
	}
	return DepositAddress{}, fmt.Errorf(
		"allocate web3 deposit address for user %d wallet %q: %w",
		userID,
		verifiedWallet.WalletID,
		ErrAddressAllocationConflict,
	)
}

func usableDepositAddress(address DepositAddress) (DepositAddress, error) {
	if address.Status != AddressStatusActive {
		return DepositAddress{}, ErrAddressDisabled
	}
	return address, nil
}
