package web3deposit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type ConfiguredWallet struct {
	WalletID    string
	AccountPath string
	AccountXPub string
}

type WalletMetadataStore interface {
	Create(ctx context.Context, wallet WalletMetadata) (WalletMetadata, error)
	GetByWalletID(ctx context.Context, walletID string) (WalletMetadata, error)
}

type WalletIdentityVerifier struct {
	store WalletMetadataStore
}

func NewWalletIdentityVerifier(store WalletMetadataStore) *WalletIdentityVerifier {
	return &WalletIdentityVerifier{store: store}
}

func ComputeAccountXPubFingerprint(accountXPub string) (string, error) {
	accountKey, err := parseAccountXPub(accountXPub)
	if err != nil {
		return "", err
	}
	defer accountKey.Zero()

	fingerprint := sha256.Sum256([]byte(accountKey.String()))
	return hex.EncodeToString(fingerprint[:]), nil
}

func (v *WalletIdentityVerifier) Verify(ctx context.Context, configured ConfiguredWallet) (WalletMetadata, error) {
	walletID := strings.TrimSpace(configured.WalletID)
	accountPath := strings.TrimSpace(configured.AccountPath)
	fingerprint, err := ComputeAccountXPubFingerprint(configured.AccountXPub)
	if err != nil {
		return WalletMetadata{}, err
	}

	stored, err := v.store.GetByWalletID(ctx, walletID)
	if errors.Is(err, ErrWalletNotFound) {
		stored, err = v.store.Create(ctx, WalletMetadata{
			WalletID:        walletID,
			AccountPath:     accountPath,
			XPubFingerprint: fingerprint,
			Status:          WalletStatusActive,
		})
		if errors.Is(err, ErrWalletAlreadyExists) {
			stored, err = v.store.GetByWalletID(ctx, walletID)
		}
	}
	if err != nil {
		return WalletMetadata{}, fmt.Errorf("verify web3 deposit wallet %q: %w", walletID, err)
	}
	if stored.AccountPath != accountPath {
		return WalletMetadata{}, fmt.Errorf("verify web3 deposit wallet %q: %w", walletID, ErrWalletAccountPathMismatch)
	}
	if stored.XPubFingerprint != fingerprint {
		return WalletMetadata{}, fmt.Errorf("verify web3 deposit wallet %q: %w", walletID, ErrWalletFingerprintMismatch)
	}
	if stored.Status != WalletStatusActive {
		return WalletMetadata{}, fmt.Errorf("verify web3 deposit wallet %q: %w", walletID, ErrWalletDisabled)
	}
	return stored, nil
}
