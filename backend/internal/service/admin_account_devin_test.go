package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type devinAdminAccountRepo struct {
	AccountRepository
	account *Account
	writes  int
}

func (r *devinAdminAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	account := *r.account
	account.Credentials = make(map[string]any, len(r.account.Credentials))
	for key, value := range r.account.Credentials {
		account.Credentials[key] = value
	}
	return &account, nil
}
func (r *devinAdminAccountRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return []*Account{r.account}, nil
}
func (r *devinAdminAccountRepo) Create(_ context.Context, account *Account) error {
	r.account = account
	r.writes++
	return nil
}
func (r *devinAdminAccountRepo) Update(ctx context.Context, account *Account) error {
	return r.Create(ctx, account)
}
func (r *devinAdminAccountRepo) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	r.writes++
	return 1, nil
}

func devinWiringSessionToken(exp int64) string {
	return "eyJhbGciOiJIUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp))) + ".signature"
}

func TestDevinAccountCreationWiring(t *testing.T) {
	for _, admin := range []bool{true, false} {
		t.Run(fmt.Sprintf("admin=%t", admin), func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				accountType string
				credentials map[string]any
				valid       bool
			}{
				{"oauth", AccountTypeOAuth, map[string]any{"access_token": "  cli-session  "}, true},
				{"apikey", AccountTypeAPIKey, map[string]any{"access_token": "cli-session"}, false},
				{"setup-token", AccountTypeSetupToken, map[string]any{"access_token": "cli-session"}, false},
				{"missing-token", AccountTypeOAuth, map[string]any{}, false},
				{"refresh-token", AccountTypeOAuth, map[string]any{"access_token": "cli-session", "refresh_token": "unsupported"}, false},
				{"custom-endpoint", AccountTypeOAuth, map[string]any{"access_token": "cli-session", "base_url": "https://example.com"}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					repo := &devinAdminAccountRepo{}
					var account *Account
					var err error
					if admin {
						account, err = (&adminServiceImpl{accountRepo: repo}).CreateAccount(context.Background(), &CreateAccountInput{
							Platform: PlatformDevin, Type: tc.accountType, Credentials: tc.credentials, SkipDefaultGroupBind: true,
						})
					} else {
						account, err = (&AccountService{accountRepo: repo}).Create(context.Background(), CreateAccountRequest{
							Platform: PlatformDevin, Type: tc.accountType, Credentials: tc.credentials,
						})
					}
					if !tc.valid {
						require.Error(t, err)
						require.Zero(t, repo.writes)
						return
					}
					require.NoError(t, err)
					require.Equal(t, "cli-session", account.GetCredential("access_token"))
					require.Equal(t, 1, repo.writes)
				})
			}
		})
	}
}

func TestDevinAccountEditNormalizesMergedCredentials(t *testing.T) {
	for _, admin := range []bool{true, false} {
		for _, tc := range []struct {
			name       string
			patch      map[string]any
			wantToken  string
			wantExpiry string
			invalid    bool
		}{
			{"name-only", nil, devinWiringSessionToken(2100000000), "2100000000", false},
			{"redacted-edit", map[string]any{"model_mapping": map[string]any{"alias": "swe-1-6"}}, devinWiringSessionToken(2100000000), "2100000000", false},
			{"rotated-jwt", map[string]any{"access_token": " " + devinWiringSessionToken(2200000000) + " ", "expires_at": "2100000000"}, devinWiringSessionToken(2200000000), "2200000000", false},
			{"rotated-opaque", map[string]any{"access_token": "replacement", "expires_at": "2100000000"}, "replacement", "", false},
			{"empty-token", map[string]any{"access_token": " "}, "", "", true},
			{"refresh-token", map[string]any{"refresh_token": "unsupported"}, "", "", true},
			{"endpoint", map[string]any{"base_url": "https://example.com"}, "", "", true},
		} {
			t.Run(fmt.Sprintf("admin=%t/%s", admin, tc.name), func(t *testing.T) {
				repo := &devinAdminAccountRepo{account: &Account{
					ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth,
					Credentials: map[string]any{"access_token": devinWiringSessionToken(2100000000), "expires_at": "2100000000"},
				}}
				var account *Account
				var err error
				if admin {
					account, err = (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), 1, &UpdateAccountInput{Name: "edited", Credentials: tc.patch})
				} else {
					req := UpdateAccountRequest{}
					if tc.patch != nil {
						req.Credentials = &tc.patch
					}
					account, err = (&AccountService{accountRepo: repo}).Update(context.Background(), 1, req)
				}
				if tc.invalid {
					require.Error(t, err)
					require.Zero(t, repo.writes)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.wantToken, account.GetCredential("access_token"))
				if tc.wantExpiry == "" {
					require.Nil(t, account.GetCredentialAsTime("expires_at"))
				} else {
					expiry := account.GetCredentialAsTime("expires_at")
					require.NotNil(t, expiry)
					require.Equal(t, tc.wantExpiry, strconv.FormatInt(expiry.Unix(), 10))
				}
			})
		}
	}
}

func TestDevinAdminRejectsTypeChangeWithoutCredentials(t *testing.T) {
	repo := &devinAdminAccountRepo{account: &Account{ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "session"}}}
	_, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), 1, &UpdateAccountInput{Type: AccountTypeAPIKey})
	require.Error(t, err)
	require.Zero(t, repo.writes)
}

func TestDevinAdminBulkCredentialsRequireIndividualEdit(t *testing.T) {
	for _, patch := range []map[string]any{{"access_token": "replacement"}, {"refresh_token": "unsupported"}, {"base_url": "https://example.com"}} {
		repo := &devinAdminAccountRepo{account: &Account{ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth}}
		_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Credentials: patch})
		require.ErrorContains(t, err, "individually")
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
		require.Zero(t, repo.writes)
	}
}

func TestDevinBulkGuardPreservesOrdinaryUpdatesAndOtherProviders(t *testing.T) {
	for _, tc := range []struct {
		name        string
		platform    string
		credentials map[string]any
	}{
		{"devin-status", PlatformDevin, nil},
		{"openai-credentials", PlatformOpenAI, map[string]any{"access_token": "replacement"}},
		{"anthropic-credentials", PlatformAnthropic, map[string]any{"refresh_token": "refresh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &devinAdminAccountRepo{account: &Account{ID: 1, Platform: tc.platform, Type: AccountTypeOAuth}}
			result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
				AccountIDs: []int64{1}, Status: StatusDisabled, Credentials: tc.credentials,
			})
			require.NoError(t, err)
			require.Equal(t, 1, result.Success)
			require.Equal(t, 1, repo.writes)
		})
	}
}

func TestDevinAdminRefreshUnsupported(t *testing.T) {
	repo := &devinAdminAccountRepo{account: &Account{ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth}}
	_, err := (&adminServiceImpl{accountRepo: repo}).RefreshAccountCredentials(context.Background(), 1)
	require.ErrorContains(t, err, "sign in again")
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.writes)
}

func TestDevinAccountServiceTestCredentialsDoesNotFallThrough(t *testing.T) {
	repo := &devinAdminAccountRepo{account: &Account{ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "session"}}}
	svc := &AccountService{accountRepo: repo}
	require.NoError(t, svc.TestCredentials(context.Background(), 1))
	repo.account.Credentials["refresh_token"] = "unsupported"
	require.Error(t, svc.TestCredentials(context.Background(), 1))
}

func TestDevinGroupModelCandidates(t *testing.T) {
	want := []string{"swe-1-6", "swe-1-6-fast"}
	require.Equal(t, want, defaultModelsListCandidateIDs(PlatformDevin))
	for _, id := range want {
		require.Contains(t, defaultModelsListCandidateIDs(PlatformComposite), id)
	}
}
