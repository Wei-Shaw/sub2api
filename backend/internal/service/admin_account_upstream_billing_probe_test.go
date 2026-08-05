package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type upstreamBillingProbeAdminRepo struct {
	*upstreamBillingProbeAccountRepo
}

func (r *upstreamBillingProbeAdminRepo) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}

type accountBillingSettingsAdminRepo struct {
	*upstreamBillingProbeAccountRepo
	concurrentRate   *float64
	lastExplicitRate *float64
	updateCalls      int
}

func (r *accountBillingSettingsAdminRepo) UpdateWithAccountBillingSettings(
	_ context.Context,
	account *Account,
	probeEnabled *bool,
	rateSyncEnabled *bool,
	rateMultiplier *float64,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.accounts[account.ID]
	if current == nil {
		return ErrAccountNotFound
	}
	updated := *account
	updated.Credentials = mergeMap(nil, account.Credentials)
	updated.Extra = mergeMap(nil, account.Extra)
	if updated.Extra == nil {
		updated.Extra = make(map[string]any)
	}
	if probeEnabled != nil {
		updated.Extra[UpstreamBillingProbeEnabledExtraKey] = *probeEnabled
	}
	if rateSyncEnabled != nil {
		updated.Extra[UpstreamBillingRateSyncEnabledExtraKey] = *rateSyncEnabled
	}
	switch {
	case rateMultiplier != nil:
		value := *rateMultiplier
		updated.RateMultiplier = &value
		r.lastExplicitRate = &value
	case r.concurrentRate != nil:
		value := *r.concurrentRate
		updated.RateMultiplier = &value
		r.lastExplicitRate = nil
	default:
		updated.RateMultiplier = cloneAccountValuePointer(current.RateMultiplier)
		r.lastExplicitRate = nil
	}
	r.accounts[account.ID] = &updated
	r.updateCalls++
	return nil
}

func TestUpdateAccountRoutesRateIntentThroughAtomicBillingUpdater(t *testing.T) {
	accountID := int64(109)
	initialRate := 0.1
	concurrentRate := 0.2
	repo := &accountBillingSettingsAdminRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			accountID: {
				ID:             accountID,
				Name:           "before",
				Platform:       PlatformOpenAI,
				Type:           AccountTypeAPIKey,
				Status:         StatusActive,
				RateMultiplier: &initialRate,
				Extra: map[string]any{
					UpstreamBillingProbeEnabledExtraKey:    true,
					UpstreamBillingRateSyncEnabledExtraKey: true,
				},
			},
		}},
		concurrentRate: &concurrentRate,
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{Name: "after"})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Nil(t, repo.lastExplicitRate)
	require.Equal(t, concurrentRate, *updated.RateMultiplier)

	// 手工倍率只有在同步不再开启时才被接受，所以同一请求先关闭同步再设值
	// （同步仍开启时的手工倍率由 TestUpdateAccountRejectsManualRateWhileRateSyncEnabled 覆盖）。
	zero := 0.0
	syncDisabled := false
	updated, err = svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		RateSyncEnabled: &syncDisabled,
		RateMultiplier:  &zero,
	})
	require.NoError(t, err)
	require.Equal(t, 2, repo.updateCalls)
	require.NotNil(t, repo.lastExplicitRate)
	require.Zero(t, *repo.lastExplicitRate)
	require.Zero(t, *updated.RateMultiplier)
}

func TestCreateAccountDropsManagedUpstreamBillingProbeState(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "upstream",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-test"},
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey:    true,
			UpstreamBillingRateSyncEnabledExtraKey: true,
			UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
		},
	})

	require.NoError(t, err)
	require.NotContains(t, created.Extra, UpstreamBillingProbeEnabledExtraKey)
	require.NotContains(t, created.Extra, UpstreamBillingRateSyncEnabledExtraKey)
	require.NotContains(t, created.Extra, UpstreamBillingProbeExtraKey)
}

func TestCreateAccountAcceptsDedicatedUpstreamBillingProbeSetting(t *testing.T) {
	enabled := true
	repo := &upstreamBillingProbeAccountRepo{}
	created, err := (&adminServiceImpl{accountRepo: repo}).CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "upstream",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-test"},
		ProbeEnabled:         &enabled,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Equal(t, true, created.Extra[UpstreamBillingProbeEnabledExtraKey])

	_, err = (&adminServiceImpl{accountRepo: repo}).CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "oauth",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		Credentials:          map[string]any{"access_token": "token"},
		ProbeEnabled:         &enabled,
		SkipDefaultGroupBind: true,
	})
	require.ErrorIs(t, err, ErrUpstreamBillingProbeAccountInvalid)
}

func TestUpdateAccountPreservesManagedUpstreamBillingProbeStateForUnrelatedEdit(t *testing.T) {
	accountID := int64(110)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra: map[string]any{
				UpstreamBillingProbeEnabledExtraKey:    true,
				UpstreamBillingRateSyncEnabledExtraKey: true,
				UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
			},
		},
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{"custom": "value"},
	})

	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[UpstreamBillingProbeEnabledExtraKey])
	require.Equal(t, true, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])
	require.Contains(t, updated.Extra, UpstreamBillingProbeExtraKey)
	require.Equal(t, "value", updated.Extra["custom"])
}

func TestUpdateAccountPreservesGrokBillingSnapshotForUnrelatedEdit(t *testing.T) {
	accountID := int64(112)
	billing := &xai.BillingSummary{
		StatusCode:       http.StatusForbidden,
		WeeklyStatusCode: http.StatusForbidden,
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra:    map[string]any{grokBillingExtraKey: billing},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{"custom": "value"},
	})

	require.NoError(t, err)
	require.Equal(t, billing, updated.Extra[grokBillingExtraKey])
	require.Equal(t, "value", updated.Extra["custom"])
	eligible, reason := updated.GrokMediaGenerationEligibility()
	require.False(t, eligible)
	require.Equal(t, "billing_forbidden", reason)
}

func TestUpdateAccountPreservesProbeSnapshotWhenIdentityValuesAreUnchanged(t *testing.T) {
	accountID := int64(119)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Credentials: map[string]any{
				"api_key":                    "sk-existing",
				"base_url":                   "https://upstream.example",
				credKeyHeaderOverrideEnabled: true,
				credKeyHeaderOverrides:       map[string]any{"x-route": "stable"},
			},
			Extra: map[string]any{
				UpstreamBillingProbeEnabledExtraKey: true,
				UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
			},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{
			"base_url":                   "https://upstream.example",
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides:       map[string]any{"x-route": "stable"},
		},
	})

	require.NoError(t, err)
	require.Contains(t, updated.Extra, UpstreamBillingProbeExtraKey)
}

func TestUpdateAccountInvalidatesProbeSnapshotWhenUpstreamIdentityChanges(t *testing.T) {
	tests := []struct {
		name        string
		input       *UpdateAccountInput
		wantEnabled bool
	}{
		{
			name:        "api key",
			input:       &UpdateAccountInput{Credentials: map[string]any{"api_key": "sk-new"}},
			wantEnabled: true,
		},
		{
			name:        "base url",
			input:       &UpdateAccountInput{Credentials: map[string]any{"base_url": "https://new.example"}},
			wantEnabled: true,
		},
		{
			name: "header override",
			input: &UpdateAccountInput{Credentials: map[string]any{
				credKeyHeaderOverrideEnabled: true,
				credKeyHeaderOverrides:       map[string]any{"x-route": "new"},
			}},
			wantEnabled: true,
		},
		{
			name:        "account type",
			input:       &UpdateAccountInput{Type: AccountTypeOAuth},
			wantEnabled: false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountID := int64(120 + i)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
				accountID: {
					ID:       accountID,
					Platform: PlatformOpenAI,
					Type:     AccountTypeAPIKey,
					Status:   StatusActive,
					Credentials: map[string]any{
						"api_key":  "sk-old",
						"base_url": "https://old.example",
					},
					Extra: map[string]any{
						UpstreamBillingProbeEnabledExtraKey:    true,
						UpstreamBillingRateSyncEnabledExtraKey: true,
						UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
					},
				},
			}}

			updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, tt.input)

			require.NoError(t, err)
			require.NotContains(t, updated.Extra, UpstreamBillingProbeExtraKey)
			if tt.wantEnabled {
				require.Equal(t, true, updated.Extra[UpstreamBillingProbeEnabledExtraKey])
			} else {
				require.NotContains(t, updated.Extra, UpstreamBillingProbeEnabledExtraKey)
				require.NotContains(t, updated.Extra, UpstreamBillingRateSyncEnabledExtraKey)
			}
		})
	}
}

func TestUpdateAccountInvalidatesProbeSnapshotWhenProxyChanges(t *testing.T) {
	accountID := int64(140)
	oldProxyID := int64(7)
	newProxyID := int64(8)
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:          accountID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Credentials: map[string]any{"api_key": "sk-test"},
			ProxyID:     &oldProxyID,
			Extra: map[string]any{
				UpstreamBillingProbeEnabledExtraKey: true,
				UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
			},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: &upstreamBillingProbeAdminRepo{baseRepo}}).UpdateAccount(
		context.Background(),
		accountID,
		&UpdateAccountInput{ProxyID: &newProxyID},
	)

	require.NoError(t, err)
	require.Equal(t, newProxyID, *updated.ProxyID)
	require.NotContains(t, updated.Extra, UpstreamBillingProbeExtraKey)
}

func TestUpdateAccountPreservesProbeSnapshotWhenProxyIsUnchanged(t *testing.T) {
	accountID := int64(141)
	existingProxyID := int64(7)
	unchangedProxyID := int64(7)
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:          accountID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Credentials: map[string]any{"api_key": "sk-test"},
			ProxyID:     &existingProxyID,
			Extra: map[string]any{
				UpstreamBillingProbeEnabledExtraKey: true,
				UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
			},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: &upstreamBillingProbeAdminRepo{baseRepo}}).UpdateAccount(
		context.Background(),
		accountID,
		&UpdateAccountInput{ProxyID: &unchangedProxyID},
	)

	require.NoError(t, err)
	require.Contains(t, updated.Extra, UpstreamBillingProbeExtraKey)
}

func TestUpdateAccountAcceptsProbeEnabledAndRejectsInjectedSnapshot(t *testing.T) {
	accountID := int64(111)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra:    map[string]any{},
		},
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey:    true,
			UpstreamBillingRateSyncEnabledExtraKey: true,
			UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[UpstreamBillingProbeEnabledExtraKey])
	require.NotContains(t, updated.Extra, UpstreamBillingRateSyncEnabledExtraKey)
	require.NotContains(t, updated.Extra, UpstreamBillingProbeExtraKey)
}

func TestUpdateAccountRateSyncControlsProbeAndManualMode(t *testing.T) {
	accountID := int64(151)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformGemini,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra:    map[string]any{},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	syncEnabled := true
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		RateSyncEnabled: &syncEnabled,
	})
	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[UpstreamBillingProbeEnabledExtraKey])
	require.Equal(t, true, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])

	syncEnabled = false
	updated, err = svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		RateSyncEnabled: &syncEnabled,
	})
	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[UpstreamBillingProbeEnabledExtraKey])
	require.Equal(t, false, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])
}

// 单账号编辑必须和批量路径语义一致：同步开启时倍率归上游所有，手工值会在下一次
// 成功探测时被覆盖，因此直接拒绝而不是静默接受。
func TestUpdateAccountRejectsManualRateWhileRateSyncEnabled(t *testing.T) {
	newRepo := func(accountID int64, extra map[string]any) *upstreamBillingProbeAccountRepo {
		initialRate := 0.25
		return &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			accountID: {
				ID:             accountID,
				Platform:       PlatformOpenAI,
				Type:           AccountTypeAPIKey,
				Status:         StatusActive,
				RateMultiplier: &initialRate,
				Extra:          extra,
			},
		}}
	}
	manualRate := 3.5
	syncEnabled := map[string]any{
		UpstreamBillingProbeEnabledExtraKey:    true,
		UpstreamBillingRateSyncEnabledExtraKey: true,
	}

	t.Run("sync enabled rejects manual rate", func(t *testing.T) {
		accountID := int64(153)
		repo := newRepo(accountID, mergeMap(nil, syncEnabled))

		_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
			RateMultiplier: &manualRate,
		})

		require.ErrorIs(t, err, ErrUpstreamBillingRateSyncConflict)
		require.Equal(t, 0.25, *repo.accounts[accountID].RateMultiplier)
	})

	t.Run("enabling sync in the same request rejects manual rate", func(t *testing.T) {
		accountID := int64(154)
		repo := newRepo(accountID, map[string]any{})
		enable := true

		_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
			RateSyncEnabled: &enable,
			RateMultiplier:  &manualRate,
		})

		require.ErrorIs(t, err, ErrUpstreamBillingRateSyncConflict)
		require.Equal(t, 0.25, *repo.accounts[accountID].RateMultiplier)
	})

	// 用户显式收回所有权：同一请求关闭同步并改倍率必须放行。
	t.Run("disabling sync in the same request allows manual rate", func(t *testing.T) {
		accountID := int64(155)
		repo := newRepo(accountID, mergeMap(nil, syncEnabled))
		disable := false

		updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
			RateSyncEnabled: &disable,
			RateMultiplier:  &manualRate,
		})

		require.NoError(t, err)
		require.Equal(t, false, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])
		require.NotNil(t, updated.RateMultiplier)
		require.Equal(t, manualRate, *updated.RateMultiplier)
	})

	t.Run("sync disabled allows manual rate", func(t *testing.T) {
		accountID := int64(156)
		repo := newRepo(accountID, map[string]any{UpstreamBillingProbeEnabledExtraKey: true})

		updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
			RateMultiplier: &manualRate,
		})

		require.NoError(t, err)
		require.NotNil(t, updated.RateMultiplier)
		require.Equal(t, manualRate, *updated.RateMultiplier)
	})
}

func TestUpdateAccountRejectsSyncWithExplicitlyDisabledProbe(t *testing.T) {
	accountID := int64(152)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
		},
	}}
	probeEnabled := false
	syncEnabled := true

	_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		ProbeEnabled:    &probeEnabled,
		RateSyncEnabled: &syncEnabled,
	})

	require.Error(t, err)
	require.Empty(t, repo.updates[accountID])
}

func TestUpdateAccountExplicitProbeDisableUsesDedicatedExtraUpdate(t *testing.T) {
	accountID := int64(113)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra: map[string]any{
				UpstreamBillingProbeEnabledExtraKey: true,
				UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
			},
		},
	}}

	_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{UpstreamBillingProbeEnabledExtraKey: false},
	})

	require.NoError(t, err)
	require.Len(t, repo.updates[accountID], 1)
	require.Equal(t, false, repo.updates[accountID][0][UpstreamBillingProbeEnabledExtraKey])
	require.Equal(t, false, repo.updates[accountID][0][UpstreamBillingRateSyncEnabledExtraKey])
}

func TestUpdateAccountExplicitUnchangedProbeEnabledStillUsesDedicatedExtraUpdate(t *testing.T) {
	accountID := int64(114)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra:    map[string]any{UpstreamBillingProbeEnabledExtraKey: true},
		},
	}}

	_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{UpstreamBillingProbeEnabledExtraKey: true},
	})

	require.NoError(t, err)
	require.Len(t, repo.updates[accountID], 1)
	require.Equal(t, true, repo.updates[accountID][0][UpstreamBillingProbeEnabledExtraKey])
}

func TestUpdateAccountRejectsInvalidProbeEnabled(t *testing.T) {
	accountID := int64(112)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra:    map[string]any{},
		},
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{UpstreamBillingProbeEnabledExtraKey: "true"},
	})

	require.Error(t, err)
}

func TestUpdateAccountExtraDropsManagedBillingProbeFields(t *testing.T) {
	accountID := int64(153)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}}

	err := (&adminServiceImpl{accountRepo: repo}).UpdateAccountExtra(context.Background(), accountID, map[string]any{
		"custom":                               "value",
		UpstreamBillingProbeEnabledExtraKey:    true,
		UpstreamBillingRateSyncEnabledExtraKey: true,
		UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
	})

	require.NoError(t, err)
	require.Equal(t, "value", repo.accounts[accountID].Extra["custom"])
	require.NotContains(t, repo.accounts[accountID].Extra, UpstreamBillingProbeEnabledExtraKey)
	require.NotContains(t, repo.accounts[accountID].Extra, UpstreamBillingRateSyncEnabledExtraKey)
	require.NotContains(t, repo.accounts[accountID].Extra, UpstreamBillingProbeExtraKey)
}

func TestBulkUpdateAccountsDropsManagedUpstreamBillingProbeState(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra: map[string]any{
			"custom":                               "value",
			UpstreamBillingProbeEnabledExtraKey:    true,
			UpstreamBillingRateSyncEnabledExtraKey: true,
			UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
		},
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)

	require.NoError(t, err)
	require.Equal(t, 1, result.Success)
	require.Len(t, repo.bulkUpdates, 1)
	require.Equal(t, "value", repo.bulkUpdates[0].Extra["custom"])
	require.NotContains(t, repo.bulkUpdates[0].Extra, UpstreamBillingProbeEnabledExtraKey)
	require.NotContains(t, repo.bulkUpdates[0].Extra, UpstreamBillingRateSyncEnabledExtraKey)
	require.NotContains(t, repo.bulkUpdates[0].Extra, UpstreamBillingProbeExtraKey)
}

func TestBulkUpdateAccountsAcceptsDedicatedUpstreamBillingProbeSetting(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[enabled], func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
				1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
				2: {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			}}

			result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
				AccountIDs:   []int64{1, 2},
				ProbeEnabled: &enabled,
			})

			require.NoError(t, err)
			require.Equal(t, 2, result.Success)
			require.Len(t, repo.bulkUpdates, 1)
			require.Equal(t, enabled, repo.bulkUpdates[0].Extra[UpstreamBillingProbeEnabledExtraKey])
			if !enabled {
				require.Equal(t, false, repo.bulkUpdates[0].Extra[UpstreamBillingRateSyncEnabledExtraKey])
			}
			require.NotNil(t, repo.bulkUpdates[0].ProbeEnabled)
			require.Equal(t, enabled, *repo.bulkUpdates[0].ProbeEnabled)
		})
	}
}

func TestBulkUpdateAccountsRejectsProbeSettingForIneligibleTargetBeforeWrite(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[enabled], func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
				1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
				2: {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			}}

			_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
				AccountIDs:   []int64{1, 2},
				ProbeEnabled: &enabled,
			})

			require.ErrorIs(t, err, ErrUpstreamBillingProbeAccountInvalid)
			require.Empty(t, repo.bulkUpdates)
		})
	}
}

func TestBulkUpdateAccountsRejectsProbeSettingWhenTargetIsMissing(t *testing.T) {
	enabled := true
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}}

	_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:   []int64{1, 2},
		ProbeEnabled: &enabled,
	})

	require.ErrorIs(t, err, ErrAccountNotFound)
	require.Empty(t, repo.bulkUpdates)
}

func TestBulkUpdateAccountsInvalidatesProbeSnapshotForIdentityCredentials(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1},
		Credentials: map[string]any{"api_key": "sk-new"},
	}

	result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), input)

	require.NoError(t, err)
	require.Equal(t, 1, result.Success)
	require.Len(t, repo.bulkUpdates, 1)
	require.Contains(t, repo.bulkUpdates[0].Extra, UpstreamBillingProbeExtraKey)
	require.Nil(t, repo.bulkUpdates[0].Extra[UpstreamBillingProbeExtraKey])
}

func TestBulkUpdateAccountsInvalidatesProbeSnapshotForProxyUpdate(t *testing.T) {
	proxyID := int64(9)
	baseRepo := &upstreamBillingProbeAccountRepo{}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		ProxyID:    &proxyID,
	}

	result, err := (&adminServiceImpl{accountRepo: &upstreamBillingProbeAdminRepo{baseRepo}}).BulkUpdateAccounts(context.Background(), input)

	require.NoError(t, err)
	require.Equal(t, 1, result.Success)
	require.Len(t, baseRepo.bulkUpdates, 1)
	require.Contains(t, baseRepo.bulkUpdates[0].Extra, UpstreamBillingProbeExtraKey)
	require.Nil(t, baseRepo.bulkUpdates[0].Extra[UpstreamBillingProbeExtraKey])
}

func TestBulkUpdateAccountsKeepsProbeSnapshotForUnrelatedCredentials(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1},
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-old": "gpt-new"}},
	}

	_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), input)

	require.NoError(t, err)
	require.Len(t, repo.bulkUpdates, 1)
	require.NotContains(t, repo.bulkUpdates[0].Extra, UpstreamBillingProbeExtraKey)
}
