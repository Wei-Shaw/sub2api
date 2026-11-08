package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

const userSuppliedCodexFingerprintSeed = "22222222-2222-4222-8222-222222222222"

func requireValidCodexFingerprintSeed(t *testing.T, extra map[string]any) string {
	t.Helper()
	seed, ok := codexFingerprintSeed(extra)
	require.True(t, ok, "expected valid canonical Codex fingerprint seed")
	return seed
}

func TestAdminCreateAccountStripsUserSeedAndCreatesFreshSeedWhenEnabled(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "codex-oauth",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
			codexFingerprintSeedExtraKey: userSuppliedCodexFingerprintSeed,
		},
	})

	require.NoError(t, err)
	seed := requireValidCodexFingerprintSeed(t, created.Extra)
	require.NotEqual(t, userSuppliedCodexFingerprintSeed, seed)
	require.Equal(t, "session", created.Extra[codexFingerprintModeExtraKey])
}

func TestAdminUpdateAccountPreservesExistingSeedAndStripsUserSeed(t *testing.T) {
	accountID := int64(201)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Name:     "before",
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				codexFingerprintModeExtraKey: "session",
				codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
			},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "full",
			codexFingerprintSeedExtraKey: userSuppliedCodexFingerprintSeed,
			"custom":                     "value",
		},
	})

	require.NoError(t, err)
	require.Equal(t, testCodexFingerprintSeed, requireValidCodexFingerprintSeed(t, updated.Extra))
	require.Equal(t, "full", updated.Extra[codexFingerprintModeExtraKey])
	require.Equal(t, "value", updated.Extra["custom"])
}

func TestAdminUpdateAccountInitializesSeedWhenFullEditEnables(t *testing.T) {
	accountID := int64(202)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Name:     "before",
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				codexFingerprintModeExtraKey: "off",
				codexFingerprintSeedExtraKey: "not-a-seed",
			},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{codexFingerprintModeExtraKey: "device"},
	})

	require.NoError(t, err)
	require.NotEqual(t, "not-a-seed", requireValidCodexFingerprintSeed(t, updated.Extra))
	require.Equal(t, "device", updated.Extra[codexFingerprintModeExtraKey])
}

func TestAdminUpdateAccountDisableReenablePreservesValidSeed(t *testing.T) {
	accountID := int64(203)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				codexFingerprintModeExtraKey: "session",
				codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
			},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	disabled, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{codexFingerprintModeExtraKey: "off"},
	})
	require.NoError(t, err)
	require.Equal(t, testCodexFingerprintSeed, requireValidCodexFingerprintSeed(t, disabled.Extra))

	reenabled, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{codexFingerprintModeExtraKey: "session"},
	})
	require.NoError(t, err)
	require.Equal(t, testCodexFingerprintSeed, requireValidCodexFingerprintSeed(t, reenabled.Extra))
}

func TestAdminUpdateAccountExtraStripsSeedAndLeavesAtomicEnsureToRepository(t *testing.T) {
	accountID := int64(204)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		},
	}}

	err := (&adminServiceImpl{accountRepo: repo}).UpdateAccountExtra(context.Background(), accountID, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: userSuppliedCodexFingerprintSeed,
	})

	require.NoError(t, err)
	require.Len(t, repo.updates[accountID], 1)
	require.Equal(t, "device", repo.updates[accountID][0][codexFingerprintModeExtraKey])
	require.NotContains(t, repo.updates[accountID][0], codexFingerprintSeedExtraKey)
}

func TestBulkUpdateAccountsDoesNotPrewriteCodexSeed(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}

	result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{301, 302},
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
			codexFingerprintSeedExtraKey: userSuppliedCodexFingerprintSeed,
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Empty(t, repo.updates, "bulk enable must not loop through UpdateExtra before BulkUpdate")
	require.Len(t, repo.bulkUpdates, 1)
	require.True(t, repo.bulkUpdates[0].EnsureCodexFingerprintSeed)
	require.Equal(t, "session", repo.bulkUpdates[0].Extra[codexFingerprintModeExtraKey])
	require.NotContains(t, repo.bulkUpdates[0].Extra, codexFingerprintSeedExtraKey)
}

type codexSeedDuplicateRepo struct {
	*upstreamBillingProbeAccountRepo
}

func (r *codexSeedDuplicateRepo) CreateWithAccountGroups(ctx context.Context, account *Account, _ []AccountGroup) error {
	return r.Create(ctx, account)
}

func TestDuplicateAccountDoesNotCopyCodexFingerprintSeed(t *testing.T) {
	ctx := context.Background()
	repo := &codexSeedDuplicateRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}}
	svc := &adminServiceImpl{accountRepo: repo, accountDuplicateRepo: repo}
	source := &Account{
		Name:     "source",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
			codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
		},
	}
	require.NoError(t, repo.Create(ctx, source))

	duplicate, err := svc.DuplicateAccount(ctx, source.ID, "admin:1", "")

	require.NoError(t, err)
	require.NotEqual(t, source.ID, duplicate.ID)
	require.NotContains(t, duplicate.Extra, codexFingerprintSeedExtraKey)
	require.Equal(t, "session", duplicate.Extra[codexFingerprintModeExtraKey])
}

func TestDuplicateCreatePathMintsFreshSeedWhenEligible(t *testing.T) {
	extra, err := duplicateAccountExtra(map[string]any{
		codexFingerprintModeExtraKey: "session",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
	})
	require.NoError(t, err)

	account, err := buildAccountForCreate(&CreateAccountInput{
		Name:     "eligible-copy",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    extra,
	}, extra)

	require.NoError(t, err)
	require.NotEqual(t, testCodexFingerprintSeed, requireValidCodexFingerprintSeed(t, account.Extra))
	require.Equal(t, "session", account.Extra[codexFingerprintModeExtraKey])
}

func TestAccountServiceCreateAndUpdateCodexSeedLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
	svc := NewAccountService(repo, nil)

	created, err := svc.Create(ctx, CreateAccountRequest{
		Name:     "legacy-create",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
			codexFingerprintSeedExtraKey: userSuppliedCodexFingerprintSeed,
		},
	})
	require.NoError(t, err)
	createdSeed := requireValidCodexFingerprintSeed(t, created.Extra)
	require.NotEqual(t, userSuppliedCodexFingerprintSeed, createdSeed)

	updateSeed := userSuppliedCodexFingerprintSeed
	updated, err := svc.Update(ctx, created.ID, UpdateAccountRequest{
		Extra: &map[string]any{
			codexFingerprintModeExtraKey: "full",
			codexFingerprintSeedExtraKey: updateSeed,
		},
	})
	require.NoError(t, err)
	require.Equal(t, createdSeed, requireValidCodexFingerprintSeed(t, updated.Extra))
	require.Equal(t, "full", updated.Extra[codexFingerprintModeExtraKey])
}
