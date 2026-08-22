package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userIsolationLookupAccountRepoStub struct {
	AccountRepository
	account *Account
	err     error
	calls   int
}

func (r *userIsolationLookupAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	r.calls++
	return r.account, r.err
}

type userIsolationLookupUserRepoStub struct {
	UserRepository
	users                []User
	calls                int
	includeSubscriptions []*bool
}

func (r *userIsolationLookupUserRepoStub) ListWithFilters(
	_ context.Context,
	params pagination.PaginationParams,
	filters UserListFilters,
) ([]User, *pagination.PaginationResult, error) {
	r.calls++
	r.includeSubscriptions = append(r.includeSubscriptions, filters.IncludeSubscriptions)
	start := params.Offset()
	if start >= len(r.users) {
		return []User{}, &pagination.PaginationResult{Page: params.Page, PageSize: params.Limit()}, nil
	}
	end := min(start+params.Limit(), len(r.users))
	pages := (len(r.users) + params.Limit() - 1) / params.Limit()
	return r.users[start:end], &pagination.PaginationResult{
		Total:    int64(len(r.users)),
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
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
	require.Equal(t, 2, userRepo.calls)
	for _, include := range userRepo.includeSubscriptions {
		require.NotNil(t, include)
		require.False(t, *include)
	}
}

func TestUserIsolationLookupRejectsInvalidIDBeforeRepositories(t *testing.T) {
	accountRepo := &userIsolationLookupAccountRepoStub{account: userIsolationLookupTestAccount()}
	userRepo := &userIsolationLookupUserRepoStub{}
	svc := NewUserIsolationLookupService(userRepo, accountRepo, userIsolationLookupTestConfig())

	result, err := svc.Lookup(context.Background(), 7, "u1_not-valid")
	require.Nil(t, result)
	require.Equal(t, "INVALID_USER_ISOLATION_ID", infraerrors.Reason(err))
	require.Zero(t, accountRepo.calls)
	require.Zero(t, userRepo.calls)
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
	require.Zero(t, userRepo.calls)
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
	require.Zero(t, userRepo.calls)
}
