//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAccountIsOpenAILongContextBillingEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil account is disabled", account: nil, want: false},
		{name: "non OpenAI account is disabled", account: &Account{Platform: PlatformGrok}, want: false},
		{name: "missing extra defaults disabled", account: &Account{Platform: PlatformOpenAI}, want: false},
		{name: "missing key defaults disabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{}}, want: false},
		{name: "explicit true is enabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_long_context_billing_enabled": true}}, want: true},
		{name: "explicit false is disabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_long_context_billing_enabled": false}}, want: false},
		{name: "malformed value is disabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_long_context_billing_enabled": "false"}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAILongContextBillingEnabled())
		})
	}
}

func TestNormalizeOpenAILongContextBillingExtra(t *testing.T) {
	t.Run("OpenAI missing key persists disabled default", func(t *testing.T) {
		extra, err := normalizeOpenAILongContextBillingExtra(PlatformOpenAI, nil)

		require.NoError(t, err)
		require.Equal(t, false, extra["openai_long_context_billing_enabled"])
	})

	t.Run("OpenAI explicit false is preserved", func(t *testing.T) {
		extra, err := normalizeOpenAILongContextBillingExtra(PlatformOpenAI, map[string]any{"openai_long_context_billing_enabled": false})

		require.NoError(t, err)
		require.Equal(t, false, extra["openai_long_context_billing_enabled"])
	})

	t.Run("OpenAI malformed value is rejected", func(t *testing.T) {
		_, err := normalizeOpenAILongContextBillingExtra(PlatformOpenAI, map[string]any{"openai_long_context_billing_enabled": "false"})

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("non OpenAI extra is unchanged", func(t *testing.T) {
		extra, err := normalizeOpenAILongContextBillingExtra(PlatformGrok, nil)

		require.NoError(t, err)
		require.Nil(t, extra)
	})

	t.Run("non OpenAI malformed value is ignored", func(t *testing.T) {
		extra := map[string]any{openAILongContextBillingEnabledKey: "provider-owned"}
		normalized, err := normalizeOpenAILongContextBillingExtra(PlatformAnthropic, extra)

		require.NoError(t, err)
		require.Equal(t, extra, normalized)
	})
}

type longContextBillingRepoStub struct {
	accountRepoStub
	account          *Account
	accounts         []*Account
	createdAccount   *Account
	updateExtraCalls int
	bulkUpdateCalls  int
}

func (r *longContextBillingRepoStub) Create(_ context.Context, account *Account) error {
	account.ID = 1
	r.account = account
	r.createdAccount = account
	return nil
}

func (r *longContextBillingRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *longContextBillingRepoStub) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	if r.accounts != nil {
		return r.accounts, nil
	}
	if r.account == nil {
		return nil, nil
	}
	return []*Account{r.account}, nil
}

func (r *longContextBillingRepoStub) Update(_ context.Context, account *Account) error {
	r.account = account
	return nil
}

func (r *longContextBillingRepoStub) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	r.updateExtraCalls++
	return nil
}

func (r *longContextBillingRepoStub) BulkUpdate(_ context.Context, _ []int64, _ AccountBulkUpdate) (int64, error) {
	r.bulkUpdateCalls++
	return 1, nil
}

func TestAdminServiceCreateAccountDefaultsOpenAILongContextBillingDisabled(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "openai-account",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "test"},
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Same(t, account, repo.createdAccount)
	require.Equal(t, false, account.Extra[openAILongContextBillingEnabledKey])
}

func TestAdminServiceCreateAccountAppliesOpenAIOAuthResponsesWebsocketDefault(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{
		cfg:         &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{OAuthResponsesWebsocketsV2ModeDefault: OpenAIWSIngressModePassthrough}}},
		accountRepo: repo,
	}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "openai-oauth-account",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		Credentials:          map[string]any{"access_token": "test"},
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Equal(t, OpenAIWSIngressModePassthrough, account.Extra[openAIOAuthResponsesWebsocketsV2ModeKey])
	require.Equal(t, true, account.Extra[openAIOAuthResponsesWebsocketsV2EnabledKey])
}

func TestAdminServiceCreateAccountPreservesExplicitOpenAIOAuthResponsesWebsocketOff(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{
		cfg:         &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{OAuthResponsesWebsocketsV2ModeDefault: OpenAIWSIngressModePassthrough}}},
		accountRepo: repo,
	}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAIOAuthResponsesWebsocketsV2ModeKey:    OpenAIWSIngressModeOff,
			openAIOAuthResponsesWebsocketsV2EnabledKey: false,
		},
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Equal(t, OpenAIWSIngressModeOff, account.Extra[openAIOAuthResponsesWebsocketsV2ModeKey])
	require.Equal(t, false, account.Extra[openAIOAuthResponsesWebsocketsV2EnabledKey])
}

func TestAdminServiceCreateAccountAppliesOpenAIOAuthResponsesWebsocketDefaultToSetupToken(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{
		cfg:         &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{OAuthResponsesWebsocketsV2ModeDefault: OpenAIWSIngressModeHTTPBridge}}},
		accountRepo: repo,
	}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeSetupToken,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Equal(t, OpenAIWSIngressModeHTTPBridge, account.Extra[openAIOAuthResponsesWebsocketsV2ModeKey])
	require.Equal(t, true, account.Extra[openAIOAuthResponsesWebsocketsV2EnabledKey])
}

func TestAdminServiceCreateAccountDoesNotApplyOAuthResponsesWebsocketDefaultToAPIKey(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{
		cfg:         &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{OAuthResponsesWebsocketsV2ModeDefault: OpenAIWSIngressModePassthrough}}},
		accountRepo: repo,
	}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotContains(t, account.Extra, openAIOAuthResponsesWebsocketsV2ModeKey)
}

func TestAdminServiceCreateAccountUsesOffWhenConfigIsNil(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Equal(t, OpenAIWSIngressModeOff, account.Extra[openAIOAuthResponsesWebsocketsV2ModeKey])
	require.Equal(t, false, account.Extra[openAIOAuthResponsesWebsocketsV2EnabledKey])
}

func TestAdminServiceCreateAccountPreservesLegacyOpenAIWebsocketFlagsOnOAuth(t *testing.T) {
	for _, key := range []string{openAIOAuthResponsesWebsocketsV2EnabledKey, "responses_websockets_v2_enabled", "openai_ws_enabled"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s=%t", key, enabled), func(t *testing.T) {
				repo := &longContextBillingRepoStub{}
				svc := &adminServiceImpl{
					cfg:         &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{OAuthResponsesWebsocketsV2ModeDefault: OpenAIWSIngressModePassthrough}}},
					accountRepo: repo,
				}
				account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
					Platform:             PlatformOpenAI,
					Type:                 AccountTypeOAuth,
					Extra:                map[string]any{key: enabled},
					SkipDefaultGroupBind: true,
				})
				require.NoError(t, err)
				require.Equal(t, enabled, account.Extra[key])
				require.NotContains(t, account.Extra, openAIOAuthResponsesWebsocketsV2ModeKey)
				want := OpenAIWSIngressModeOff
				if enabled {
					want = OpenAIWSIngressModeCtxPool
				}
				require.Equal(t, want, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModePassthrough))
			})
		}
	}
}

func TestAdminServiceUpdateAccountDoesNotApplyOAuthWebsocketCreationDefault(t *testing.T) {
	for _, existingMode := range []string{"", OpenAIWSIngressModeOff, OpenAIWSIngressModeHTTPBridge} {
		t.Run("existing="+existingMode, func(t *testing.T) {
			extra := map[string]any{}
			if existingMode != "" {
				extra[openAIOAuthResponsesWebsocketsV2ModeKey] = existingMode
			}
			repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}}
			svc := &adminServiceImpl{
				cfg:         &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{OAuthResponsesWebsocketsV2ModeDefault: OpenAIWSIngressModePassthrough}}},
				accountRepo: repo,
			}
			account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Name: "renamed"})
			require.NoError(t, err)
			if existingMode == "" {
				require.NotContains(t, account.Extra, openAIOAuthResponsesWebsocketsV2ModeKey)
			} else {
				require.Equal(t, existingMode, account.Extra[openAIOAuthResponsesWebsocketsV2ModeKey])
			}
			require.NotContains(t, account.Extra, openAIOAuthResponsesWebsocketsV2EnabledKey)
		})
	}
}

func TestAdminServiceCreateAccountRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform: PlatformOpenAI,
		Extra:    map[string]any{openAILongContextBillingEnabledKey: "false"},
	})

	require.Nil(t, account)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Nil(t, repo.createdAccount)
}

func TestAdminServiceUpdateAccountPreservesOpenAILongContextBillingOptOutWhenOmitted(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{openAILongContextBillingEnabledKey: false},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{}})

	require.NoError(t, err)
	require.Equal(t, false, account.Extra[openAILongContextBillingEnabledKey])
}

func TestAdminServiceUpdateAccountAllowsExplicitCodexImportOptIn(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{
		ID:          1,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "old-token"},
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: false,
			"import_source":                    "codex_session",
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{
		Credentials: map[string]any{"access_token": "new-token"},
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: true,
			"import_source":                    "codex_session",
		},
	})

	require.NoError(t, err)
	require.Equal(t, true, account.Extra[openAILongContextBillingEnabledKey])
}

func TestAdminServiceUpdateAccountAllowsExplicitOptInOutsideCodexImport(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: false,
			"import_source":                    "codex_session",
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		openAILongContextBillingEnabledKey: true,
		"import_source":                    "codex_session",
	}})

	require.NoError(t, err)
	require.Equal(t, true, account.Extra[openAILongContextBillingEnabledKey])
}

func TestAdminServiceUpdateAccountRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAI}}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		openAILongContextBillingEnabledKey: 1,
	}})

	require.Nil(t, account)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
}

func TestAdminServiceUpdateAccountExtraRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAI}}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 1, map[string]any{
		openAILongContextBillingEnabledKey: "true",
	})

	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.updateExtraCalls)
}

func TestAdminServiceUpdateAccountExtraAllowsProviderOwnedValueForNonOpenAIAccount(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformAnthropic}}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 1, map[string]any{
		openAILongContextBillingEnabledKey: "provider-owned",
	})

	require.NoError(t, err)
	require.Equal(t, 1, repo.updateExtraCalls)
}

func TestAdminServiceBulkUpdateAccountsRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAI}}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra:      map[string]any{openAILongContextBillingEnabledKey: []bool{true}},
	})

	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.bulkUpdateCalls)
}

func TestAdminServiceBulkUpdateAccountsRejectsOpenAILongContextKeyForNonOpenAIAccounts(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformGrok}}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra:      map[string]any{openAILongContextBillingEnabledKey: true},
	})

	require.Nil(t, result)
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "OPENAI_BULK_TARGET_INVALID", appErr.Reason)
	require.Zero(t, repo.bulkUpdateCalls)
}

func TestAdminServiceBulkUpdateAccountsRejectsMalformedValueForMixedTargetsIncludingOpenAI(t *testing.T) {
	repo := &longContextBillingRepoStub{accounts: []*Account{
		{ID: 1, Platform: PlatformGrok},
		{ID: 2, Platform: PlatformOpenAI},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2},
		Extra:      map[string]any{openAILongContextBillingEnabledKey: "malformed"},
	})

	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.bulkUpdateCalls)
}
