package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type userIsolationLookupAccountRepoStub struct {
	AccountRepository
	account   *Account
	accounts  []Account
	err       error
	calls     int
	listCalls int
}

func (r *userIsolationLookupAccountRepoStub) ListUserIsolationAccounts(context.Context) ([]Account, error) {
	r.listCalls++
	return r.accounts, r.err
}

func (r *userIsolationLookupAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	r.calls++
	return r.account, r.err
}

type userIsolationLookupUserRepoStub struct {
	UserRepository
	users          []User
	candidateCalls int
	getCalls       int
}

func (r *userIsolationLookupUserRepoStub) ListUserIsolationCandidateIDs(_ context.Context, afterID int64, limit int) ([]int64, error) {
	r.candidateCalls++
	ids := make([]int64, 0, limit)
	for i := range r.users {
		if r.users[i].ID <= afterID {
			continue
		}
		ids = append(ids, r.users[i].ID)
		if len(ids) == limit {
			break
		}
	}
	return ids, nil
}

func (r *userIsolationLookupUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	r.getCalls++
	for i := range r.users {
		if r.users[i].ID == id {
			return &r.users[i], nil
		}
	}
	return nil, ErrUserNotFound
}

func userIsolationLookupTestAccount() *Account {
	return &Account{
		ID:       7,
		Name:     "risk-account",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{UserIsolationEnabledExtraKey: true},
	}
}

func userIsolationLookupTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{
		UserIsolationSecret: "lookup-secret-012345678901234567890",
	}}
}

func TestUserIsolationLookupFindsUserAcrossPages(t *testing.T) {
	account := userIsolationLookupTestAccount()
	users := make([]User, userIsolationLookupPageSize+1)
	for i := range users {
		users[i] = User{ID: int64(i + 1), Email: "other@example.com", Status: StatusActive}
	}
	target := &users[len(users)-1]
	target.Email = "risk@example.com"
	target.Username = "risk-user"
	target.Status = StatusDisabled

	accountRepo := &userIsolationLookupAccountRepoStub{account: account}
	userRepo := &userIsolationLookupUserRepoStub{users: users}
	cfg := userIsolationLookupTestConfig()
	svc := NewUserIsolationLookupService(userRepo, accountRepo, cfg)
	isolationID := deriveManagedUserIsolationID(cfg.Security.UserIsolationSecret, account, target.ID)

	result, err := svc.Lookup(context.Background(), account.ID, isolationID)
	require.NoError(t, err)
	require.Equal(t, account.ID, result.Account.ID)
	require.Equal(t, account.Platform, result.Account.Platform)
	require.Equal(t, target.ID, result.User.ID)
	require.Equal(t, "risk@example.com", result.User.Email)
	require.Equal(t, StatusDisabled, result.User.Status)
	require.Equal(t, 2, userRepo.candidateCalls)
	require.Equal(t, 1, userRepo.getCalls)
	require.Equal(t, 1, accountRepo.calls)
	require.Zero(t, accountRepo.listCalls)
}

func TestUserIsolationLookupFindsUserAcrossAllEnabledAccounts(t *testing.T) {
	first := userIsolationLookupTestAccount()
	second := Account{
		ID:       8,
		Name:     "second-account",
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{UserIsolationEnabledExtraKey: true},
	}
	unsupported := Account{
		ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Extra: map[string]any{UserIsolationEnabledExtraKey: true},
	}
	notEnabled := Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	users := make([]User, userIsolationLookupPageSize+1)
	for i := range users {
		users[i] = User{ID: int64(i + 1), Email: "other@example.com", Status: StatusActive}
	}
	target := &users[len(users)-1]
	target.Email = "risk@example.com"
	accountRepo := &userIsolationLookupAccountRepoStub{accounts: []Account{*first, unsupported, notEnabled, second}}
	userRepo := &userIsolationLookupUserRepoStub{users: users}
	cfg := userIsolationLookupTestConfig()
	svc := NewUserIsolationLookupService(userRepo, accountRepo, cfg)
	isolationID := deriveManagedUserIsolationID(cfg.Security.UserIsolationSecret, &second, target.ID)

	result, err := svc.Lookup(context.Background(), 0, isolationID)
	require.NoError(t, err)
	require.Equal(t, second.ID, result.Account.ID)
	require.Equal(t, target.ID, result.User.ID)
	require.Equal(t, 1, accountRepo.listCalls)
	require.Zero(t, accountRepo.calls)
	require.Equal(t, 2, userRepo.candidateCalls)
	require.Equal(t, 1, userRepo.getCalls)
}

func TestUserIsolationLookupRejectsInvalidIDBeforeRepositories(t *testing.T) {
	accountRepo := &userIsolationLookupAccountRepoStub{account: userIsolationLookupTestAccount()}
	userRepo := &userIsolationLookupUserRepoStub{}
	svc := NewUserIsolationLookupService(userRepo, accountRepo, userIsolationLookupTestConfig())

	result, err := svc.Lookup(context.Background(), 7, "u1_not-valid")
	require.Nil(t, result)
	require.Equal(t, "INVALID_USER_ISOLATION_ID", infraerrors.Reason(err))
	require.Zero(t, accountRepo.calls)
	require.Zero(t, userRepo.candidateCalls)
}

func TestUserIsolationLookupRequiresEnabledSupportedAccount(t *testing.T) {
	account := userIsolationLookupTestAccount()
	delete(account.Extra, UserIsolationEnabledExtraKey)
	accountRepo := &userIsolationLookupAccountRepoStub{account: account}
	userRepo := &userIsolationLookupUserRepoStub{}
	svc := NewUserIsolationLookupService(userRepo, accountRepo, userIsolationLookupTestConfig())
	validID := deriveManagedUserIsolationID(userIsolationLookupTestConfig().Security.UserIsolationSecret, account, 1)

	result, err := svc.Lookup(context.Background(), account.ID, validID)
	require.Nil(t, result)
	require.Equal(t, "USER_ISOLATION_NOT_ENABLED", infraerrors.Reason(err))
	require.Zero(t, userRepo.candidateCalls)
}

func TestUserIsolationLookupReturnsNotFound(t *testing.T) {
	account := userIsolationLookupTestAccount()
	userRepo := &userIsolationLookupUserRepoStub{users: []User{{ID: 1, Email: "other@example.com"}}}
	svc := NewUserIsolationLookupService(
		userRepo,
		&userIsolationLookupAccountRepoStub{account: account},
		userIsolationLookupTestConfig(),
	)
	missingID := deriveManagedUserIsolationID(userIsolationLookupTestConfig().Security.UserIsolationSecret, account, 999)

	result, err := svc.Lookup(context.Background(), account.ID, missingID)
	require.Nil(t, result)
	require.Equal(t, "USER_ISOLATION_USER_NOT_FOUND", infraerrors.Reason(err))
}

func TestUserIsolationLookupRejectsMissingSecret(t *testing.T) {
	account := userIsolationLookupTestAccount()
	userRepo := &userIsolationLookupUserRepoStub{}
	svc := NewUserIsolationLookupService(
		userRepo,
		&userIsolationLookupAccountRepoStub{account: account},
		&config.Config{},
	)
	validID := deriveManagedUserIsolationID("temporary-secret", account, 1)

	result, err := svc.Lookup(context.Background(), account.ID, validID)
	require.Nil(t, result)
	require.Equal(t, "USER_ISOLATION_SECRET_UNAVAILABLE", infraerrors.Reason(err))
	require.Zero(t, userRepo.candidateCalls)
}

func TestUserIsolationLookupGlobalScanRequiresEligibleAccounts(t *testing.T) {
	accountRepo := &userIsolationLookupAccountRepoStub{accounts: []Account{{
		ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey,
	}}}
	userRepo := &userIsolationLookupUserRepoStub{}
	svc := NewUserIsolationLookupService(userRepo, accountRepo, userIsolationLookupTestConfig())
	validID := deriveManagedUserIsolationID(userIsolationLookupTestConfig().Security.UserIsolationSecret, userIsolationLookupTestAccount(), 1)

	result, err := svc.Lookup(context.Background(), 0, validID)
	require.Nil(t, result)
	require.Equal(t, "USER_ISOLATION_ACCOUNTS_NOT_FOUND", infraerrors.Reason(err))
	require.Zero(t, userRepo.candidateCalls)
}

func TestUserIsolationLookupRejectsConcurrentGlobalScan(t *testing.T) {
	account := userIsolationLookupTestAccount()
	accountRepo := &userIsolationLookupAccountRepoStub{accounts: []Account{*account}}
	userRepo := &userIsolationLookupUserRepoStub{}
	svc := NewUserIsolationLookupService(userRepo, accountRepo, userIsolationLookupTestConfig())
	svc.globalScan <- struct{}{}
	validID := deriveManagedUserIsolationID(userIsolationLookupTestConfig().Security.UserIsolationSecret, account, 1)

	result, err := svc.Lookup(context.Background(), 0, validID)
	require.Nil(t, result)
	require.Equal(t, "USER_ISOLATION_LOOKUP_BUSY", infraerrors.Reason(err))
	require.Zero(t, userRepo.candidateCalls)
}
