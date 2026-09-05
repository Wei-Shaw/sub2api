package web3deposit

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testEVMAccountXPubFingerprint = "c16965326543d7eac4bb947582a784b7aba36d32b5fef5ad809957feb1a4e2fc"

func TestComputeAccountXPubFingerprint(t *testing.T) {
	t.Parallel()

	fingerprint, err := ComputeAccountXPubFingerprint(testEVMAccountXPub)
	require.NoError(t, err)
	require.Equal(t, testEVMAccountXPubFingerprint, fingerprint)

	trimmedFingerprint, err := ComputeAccountXPubFingerprint(" \n" + testEVMAccountXPub + "\t")
	require.NoError(t, err)
	require.Equal(t, fingerprint, trimmedFingerprint)
}

func TestComputeAccountXPubFingerprintRejectsInvalidKeyWithoutLeakingIt(t *testing.T) {
	t.Parallel()

	invalidKey := "not-an-account-xpub"
	_, err := ComputeAccountXPubFingerprint(invalidKey)
	require.ErrorIs(t, err, ErrAccountXPubInvalid)
	require.NotContains(t, err.Error(), invalidKey)
}

func TestWalletIdentityVerifierInitializesMissingWallet(t *testing.T) {
	t.Parallel()

	store := &walletMetadataStoreStub{}
	verifier := NewWalletIdentityVerifier(store)

	verified, err := verifier.Verify(context.Background(), testConfiguredWallet())
	require.NoError(t, err)
	require.Equal(t, "evm_deposit_v1", verified.WalletID)
	require.Equal(t, "m/44'/60'/0'", verified.AccountPath)
	require.Equal(t, testEVMAccountXPubFingerprint, verified.XPubFingerprint)
	require.Equal(t, WalletStatusActive, verified.Status)
	require.Equal(t, 1, store.createCalls)
}

func TestWalletIdentityVerifierAcceptsMatchingWallet(t *testing.T) {
	t.Parallel()

	stored := testWalletMetadata()
	store := &walletMetadataStoreStub{wallet: &stored}

	verified, err := NewWalletIdentityVerifier(store).Verify(context.Background(), testConfiguredWallet())
	require.NoError(t, err)
	require.Equal(t, stored, verified)
	require.Zero(t, store.createCalls)
}

func TestWalletIdentityVerifierRejectsIdentityChangesAndDisabledWallet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*WalletMetadata)
		wantError error
	}{
		{
			name: "account path mismatch",
			mutate: func(wallet *WalletMetadata) {
				wallet.AccountPath = "m/44'/60'/1'"
			},
			wantError: ErrWalletAccountPathMismatch,
		},
		{
			name: "fingerprint mismatch",
			mutate: func(wallet *WalletMetadata) {
				wallet.XPubFingerprint = strings.Repeat("0", 64)
			},
			wantError: ErrWalletFingerprintMismatch,
		},
		{
			name: "disabled",
			mutate: func(wallet *WalletMetadata) {
				wallet.Status = WalletStatusDisabled
			},
			wantError: ErrWalletDisabled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stored := testWalletMetadata()
			test.mutate(&stored)
			store := &walletMetadataStoreStub{wallet: &stored}

			_, err := NewWalletIdentityVerifier(store).Verify(context.Background(), testConfiguredWallet())
			require.ErrorIs(t, err, test.wantError)
			require.NotContains(t, err.Error(), testEVMAccountXPub)
		})
	}
}

func TestWalletIdentityVerifierHandlesConcurrentInitialization(t *testing.T) {
	t.Parallel()

	stored := testWalletMetadata()
	store := &walletMetadataStoreStub{
		wallet:        &stored,
		firstGetMiss:  true,
		createFailure: ErrWalletAlreadyExists,
	}

	verified, err := NewWalletIdentityVerifier(store).Verify(context.Background(), testConfiguredWallet())
	require.NoError(t, err)
	require.Equal(t, stored, verified)
	require.Equal(t, 2, store.getCalls)
	require.Equal(t, 1, store.createCalls)
}

type walletMetadataStoreStub struct {
	wallet        *WalletMetadata
	firstGetMiss  bool
	createFailure error
	getCalls      int
	createCalls   int
}

func (s *walletMetadataStoreStub) Create(_ context.Context, wallet WalletMetadata) (WalletMetadata, error) {
	s.createCalls++
	if s.createFailure != nil {
		return WalletMetadata{}, s.createFailure
	}
	s.wallet = &wallet
	return wallet, nil
}

func (s *walletMetadataStoreStub) GetByWalletID(_ context.Context, _ string) (WalletMetadata, error) {
	s.getCalls++
	if s.firstGetMiss && s.getCalls == 1 {
		return WalletMetadata{}, ErrWalletNotFound
	}
	if s.wallet == nil {
		return WalletMetadata{}, ErrWalletNotFound
	}
	return *s.wallet, nil
}

func testConfiguredWallet() ConfiguredWallet {
	return ConfiguredWallet{
		WalletID:    "evm_deposit_v1",
		AccountPath: "m/44'/60'/0'",
		AccountXPub: testEVMAccountXPub,
	}
}

func testWalletMetadata() WalletMetadata {
	return WalletMetadata{
		WalletID:        "evm_deposit_v1",
		AccountPath:     "m/44'/60'/0'",
		XPubFingerprint: testEVMAccountXPubFingerprint,
		Status:          WalletStatusActive,
	}
}

var _ WalletMetadataStore = (*walletMetadataStoreStub)(nil)
