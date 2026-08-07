package repository

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3depositwallet"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3DepositWalletRepository struct {
	client *dbent.Client
}

var _ web3deposit.WalletMetadataStore = (*Web3DepositWalletRepository)(nil)

func NewWeb3DepositWalletRepository(client *dbent.Client) *Web3DepositWalletRepository {
	return &Web3DepositWalletRepository{client: client}
}

func (r *Web3DepositWalletRepository) Create(ctx context.Context, wallet web3deposit.WalletMetadata) (web3deposit.WalletMetadata, error) {
	create := r.client.Web3DepositWallet.Create().
		SetWalletID(wallet.WalletID).
		SetAccountPath(wallet.AccountPath).
		SetXpubFingerprint(wallet.XPubFingerprint).
		SetNextDerivationIndex(wallet.NextDerivationIndex)
	if wallet.Status != "" {
		create.SetStatus(string(wallet.Status))
	}

	entity, err := create.Save(ctx)
	if dbent.IsConstraintError(err) {
		return web3deposit.WalletMetadata{}, web3deposit.ErrWalletAlreadyExists
	}
	if err != nil {
		return web3deposit.WalletMetadata{}, fmt.Errorf("create web3 deposit wallet: %w", err)
	}
	return web3DepositWalletFromEnt(entity), nil
}

func (r *Web3DepositWalletRepository) GetByWalletID(ctx context.Context, walletID string) (web3deposit.WalletMetadata, error) {
	entity, err := r.client.Web3DepositWallet.Query().
		Where(web3depositwallet.WalletIDEQ(walletID)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return web3deposit.WalletMetadata{}, web3deposit.ErrWalletNotFound
	}
	if err != nil {
		return web3deposit.WalletMetadata{}, fmt.Errorf("get web3 deposit wallet: %w", err)
	}
	return web3DepositWalletFromEnt(entity), nil
}

func (r *Web3DepositWalletRepository) ReserveDerivationIndex(
	ctx context.Context,
	verifiedWallet web3deposit.WalletMetadata,
) (int64, error) {
	for {
		stored, err := r.GetByWalletID(ctx, verifiedWallet.WalletID)
		if err != nil {
			return 0, err
		}
		if stored.AccountPath != verifiedWallet.AccountPath {
			return 0, fmt.Errorf("reserve derivation index for wallet %q: %w", verifiedWallet.WalletID, web3deposit.ErrWalletAccountPathMismatch)
		}
		if stored.XPubFingerprint != verifiedWallet.XPubFingerprint {
			return 0, fmt.Errorf("reserve derivation index for wallet %q: %w", verifiedWallet.WalletID, web3deposit.ErrWalletFingerprintMismatch)
		}
		if stored.Status != web3deposit.WalletStatusActive {
			return 0, fmt.Errorf("reserve derivation index for wallet %q: %w", verifiedWallet.WalletID, web3deposit.ErrWalletDisabled)
		}
		if stored.NextDerivationIndex >= web3deposit.MaxDerivationIndexExclusive {
			return 0, fmt.Errorf("reserve derivation index for wallet %q: %w", verifiedWallet.WalletID, web3deposit.ErrDerivationIndexExhausted)
		}

		updated, err := r.client.Web3DepositWallet.Update().
			Where(
				web3depositwallet.IDEQ(stored.ID),
				web3depositwallet.WalletIDEQ(stored.WalletID),
				web3depositwallet.AccountPathEQ(stored.AccountPath),
				web3depositwallet.XpubFingerprintEQ(stored.XPubFingerprint),
				web3depositwallet.StatusEQ(string(web3deposit.WalletStatusActive)),
				web3depositwallet.NextDerivationIndexEQ(stored.NextDerivationIndex),
			).
			AddNextDerivationIndex(1).
			Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("reserve derivation index for wallet %q: %w", verifiedWallet.WalletID, err)
		}
		if updated == 1 {
			return stored.NextDerivationIndex, nil
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
}

func web3DepositWalletFromEnt(entity *dbent.Web3DepositWallet) web3deposit.WalletMetadata {
	return web3deposit.WalletMetadata{
		ID:                  entity.ID,
		WalletID:            entity.WalletID,
		AccountPath:         entity.AccountPath,
		XPubFingerprint:     entity.XpubFingerprint,
		NextDerivationIndex: entity.NextDerivationIndex,
		Status:              web3deposit.WalletStatus(entity.Status),
		CreatedAt:           entity.CreatedAt,
		UpdatedAt:           entity.UpdatedAt,
	}
}
