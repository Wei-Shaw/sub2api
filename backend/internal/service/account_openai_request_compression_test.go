package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAccountGetOpenAIRequestCompressionOverride(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		wantSet bool
		want    bool
	}{
		{name: "nil account"},
		{name: "nil extra", account: &Account{Platform: PlatformOpenAI}},
		{name: "missing setting", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{}}},
		{name: "non OpenAI setting is ignored", account: &Account{Platform: PlatformAnthropic, Extra: map[string]any{openAIRequestCompressionExtraKey: false}}},
		{name: "null setting fails closed", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestCompressionExtraKey: nil}}, wantSet: true},
		{name: "explicit opt out", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestCompressionExtraKey: false}}, wantSet: true},
		{name: "explicit true", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestCompressionExtraKey: true}}, wantSet: true, want: true},
		{name: "string fails closed", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestCompressionExtraKey: "false"}}, wantSet: true},
		{name: "number fails closed", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestCompressionExtraKey: 0}}, wantSet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.GetOpenAIRequestCompressionOverride()
			if !tt.wantSet {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tt.want, *got)
		})
	}
}

func TestValidateOpenAIRequestCompressionExtra(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		extra    map[string]any
		wantErr  bool
	}{
		{name: "missing inherits global", platform: PlatformOpenAI},
		{name: "explicit opt out", platform: PlatformOpenAI, extra: map[string]any{openAIRequestCompressionExtraKey: false}},
		{name: "explicit opt in", platform: PlatformOpenAI, extra: map[string]any{openAIRequestCompressionExtraKey: true}},
		{name: "null is rejected", platform: PlatformOpenAI, extra: map[string]any{openAIRequestCompressionExtraKey: nil}, wantErr: true},
		{name: "string is rejected", platform: PlatformOpenAI, extra: map[string]any{openAIRequestCompressionExtraKey: "false"}, wantErr: true},
		{name: "number is rejected", platform: PlatformOpenAI, extra: map[string]any{openAIRequestCompressionExtraKey: 0}, wantErr: true},
		{name: "non OpenAI value is provider owned", platform: PlatformAnthropic, extra: map[string]any{openAIRequestCompressionExtraKey: "provider-owned"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOpenAIRequestCompressionExtra(tt.platform, tt.extra)
			if tt.wantErr {
				require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
				return
			}
			require.NoError(t, err)
		})
	}
}

type openAIRequestCompressionAdminRepo struct {
	AccountRepository
	account          *Account
	accounts         []*Account
	createdAccount   *Account
	updateExtraCalls int
	bulkUpdateCalls  int
}

func (r *openAIRequestCompressionAdminRepo) Create(_ context.Context, account *Account) error {
	account.ID = 1
	r.account = account
	r.createdAccount = account
	return nil
}

func (r *openAIRequestCompressionAdminRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *openAIRequestCompressionAdminRepo) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	return r.accounts, nil
}

func (r *openAIRequestCompressionAdminRepo) Update(_ context.Context, account *Account) error {
	r.account = account
	return nil
}

func (r *openAIRequestCompressionAdminRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	r.updateExtraCalls++
	return nil
}

func (r *openAIRequestCompressionAdminRepo) BulkUpdate(_ context.Context, _ []int64, _ AccountBulkUpdate) (int64, error) {
	r.bulkUpdateCalls++
	return int64(len(r.accounts)), nil
}

func TestAdminServiceCreateAccountRejectsMalformedOpenAIRequestCompressionValue(t *testing.T) {
	repo := &openAIRequestCompressionAdminRepo{}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Extra:                map[string]any{openAIRequestCompressionExtraKey: "false"},
		SkipDefaultGroupBind: true,
	})

	require.Nil(t, account)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Nil(t, repo.createdAccount)
}

func TestAdminServiceUpdateAccountPreservesOpenAIRequestCompressionOptOutWhenOmitted(t *testing.T) {
	repo := &openAIRequestCompressionAdminRepo{account: &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{openAIRequestCompressionExtraKey: false},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{}})

	require.NoError(t, err)
	require.Equal(t, false, account.Extra[openAIRequestCompressionExtraKey])
}

func TestAdminServiceUpdateAccountRejectsMalformedOpenAIRequestCompressionValue(t *testing.T) {
	repo := &openAIRequestCompressionAdminRepo{account: &Account{ID: 1, Platform: PlatformOpenAI}}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		openAIRequestCompressionExtraKey: nil,
	}})

	require.Nil(t, account)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
}

func TestAdminServiceUpdateAccountExtraRejectsMalformedOpenAIRequestCompressionValue(t *testing.T) {
	repo := &openAIRequestCompressionAdminRepo{account: &Account{ID: 1, Platform: PlatformOpenAI}}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 1, map[string]any{
		openAIRequestCompressionExtraKey: "true",
	})

	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.updateExtraCalls)
}

func TestAdminServiceBulkUpdateRejectsMalformedOpenAIRequestCompressionValueBeforeWrite(t *testing.T) {
	repo := &openAIRequestCompressionAdminRepo{accounts: []*Account{{ID: 1, Platform: PlatformOpenAI}}}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra:      map[string]any{openAIRequestCompressionExtraKey: "false"},
	})

	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.bulkUpdateCalls)
}
