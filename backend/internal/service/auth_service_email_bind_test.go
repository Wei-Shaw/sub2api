//go:build unit

package service_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type emailBindDefaultSubAssignerStub struct {
	calls []*service.AssignSubscriptionInput
}

func (s *emailBindDefaultSubAssignerStub) AssignOrExtendSubscription(
	_ context.Context,
	input *service.AssignSubscriptionInput,
) (*service.UserSubscription, bool, error) {
	cloned := *input
	s.calls = append(s.calls, &cloned)
	return &service.UserSubscription{UserID: input.UserID, GroupID: input.GroupID}, false, nil
}

type flakyEmailBindDefaultSubAssignerStub struct {
	err   error
	calls []*service.AssignSubscriptionInput
}

func (s *flakyEmailBindDefaultSubAssignerStub) AssignOrExtendSubscription(
	_ context.Context,
	input *service.AssignSubscriptionInput,
) (*service.UserSubscription, bool, error) {
	cloned := *input
	s.calls = append(s.calls, &cloned)
	return nil, false, s.err
}

func newAuthServiceForEmailBind(
	t *testing.T,
	settings map[string]string,
	emailCache service.EmailCache,
	defaultSubAssigner service.DefaultSubscriptionAssigner,
) (*service.AuthService, service.UserRepository, *dbent.Client) {
	return newAuthServiceForEmailBindWithRefreshCache(t, settings, emailCache, defaultSubAssigner, nil)
}

func newAuthServiceForEmailBindWithRefreshCache(
	t *testing.T,
	settings map[string]string,
	emailCache service.EmailCache,
	defaultSubAssigner service.DefaultSubscriptionAssigner,
	refreshTokenCache service.RefreshTokenCache,
) (*service.AuthService, service.UserRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth_service_email_bind?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_provider_default_grants (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	provider_type TEXT NOT NULL,
	grant_reason TEXT NOT NULL DEFAULT 'first_bind',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, provider_type, grant_reason)
)`)
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	repo := repository.NewUserRepository(client, db)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-bind-email-secret",
			ExpireHour: 1,
		},
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}

	settingRepo := &emailBindSettingRepoStub{values: settings}
	settingSvc := service.NewSettingService(settingRepo, cfg)

	var emailSvc *service.EmailService
	if emailCache != nil {
		emailSvc = service.NewEmailService(settingRepo, emailCache)
	}

	svc := service.NewAuthService(client, repo, nil, refreshTokenCache, cfg, settingSvc, emailSvc, nil, nil, nil, defaultSubAssigner, nil)
	return svc, repo, client
}

func TestAuthServiceBindEmailIdentity_UpdatesEmailAndAppliesFirstBindDefaults(t *testing.T) {
	assigner := &emailBindDefaultSubAssignerStub{}
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	svc, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyAuthSourceDefaultEmailBalance:          "8.5",
		service.SettingKeyAuthSourceDefaultEmailConcurrency:      "4",
		service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":11,"validity_days":30}]`,
		service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "true",
	}, cache, assigner)

	ctx := context.Background()
	user, err := client.User.Create().
		SetEmail("legacy-user" + service.LinuxDoConnectSyntheticEmailDomain).
		SetUsername("legacy-user").
		SetPasswordHash("old-hash").
		SetBalance(2.5).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "  NewEmail@Example.com  ", "123456", "new-password")
	require.NoError(t, err)
	require.NotNil(t, updatedUser)
	require.Equal(t, "newemail@example.com", updatedUser.Email)

	storedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "newemail@example.com", storedUser.Email)
	require.Equal(t, 11.0, storedUser.Balance)
	require.Equal(t, 5, storedUser.Concurrency)
	require.True(t, svc.CheckPassword("new-password", storedUser.PasswordHash))

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("newemail@example.com"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, identityCount)

	require.Len(t, assigner.calls, 1)
	require.Equal(t, user.ID, assigner.calls[0].UserID)
	require.Equal(t, int64(11), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
	require.Equal(t, 1, countProviderGrantRecords(t, client, user.ID, "email", "first_bind"))
}

func TestAuthServiceBindEmailIdentity_RejectsExistingEmailOnAnotherUser(t *testing.T) {
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	svc, _, client := newAuthServiceForEmailBind(t, nil, cache, nil)

	ctx := context.Background()
	sourceUser, err := client.User.Create().
		SetEmail("source-user" + service.OIDCConnectSyntheticEmailDomain).
		SetUsername("source-user").
		SetPasswordHash("old-hash").
		SetBalance(1).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.User.Create().
		SetEmail("taken@example.com").
		SetUsername("taken-user").
		SetPasswordHash("hash").
		SetBalance(1).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	updatedUser, err := svc.BindEmailIdentity(ctx, sourceUser.ID, "taken@example.com", "123456", "new-password")
	require.ErrorIs(t, err, service.ErrEmailExists)
	require.Nil(t, updatedUser)

	storedUser, err := client.User.Get(ctx, sourceUser.ID)
	require.NoError(t, err)
	require.Equal(t, "source-user"+service.OIDCConnectSyntheticEmailDomain, storedUser.Email)
	require.Equal(t, 0, countProviderGrantRecords(t, client, sourceUser.ID, "email", "first_bind"))
}

func TestAuthServiceBindEmailIdentity_RollsBackWhenFirstBindDefaultsFail(t *testing.T) {
	assigner := &flakyEmailBindDefaultSubAssignerStub{err: errors.New("temporary assign failure")}
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	svc, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyAuthSourceDefaultEmailBalance:          "8.5",
		service.SettingKeyAuthSourceDefaultEmailConcurrency:      "4",
		service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":11,"validity_days":30}]`,
		service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "true",
	}, cache, assigner)

	ctx := context.Background()
	originalEmail := "legacy-rollback" + service.LinuxDoConnectSyntheticEmailDomain
	user, err := client.User.Create().
		SetEmail(originalEmail).
		SetUsername("legacy-rollback").
		SetPasswordHash("old-hash").
		SetBalance(2.5).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "rollback@example.com", "123456", "new-password")
	require.ErrorContains(t, err, "apply email first bind defaults")
	require.ErrorContains(t, err, "temporary assign failure")
	require.Nil(t, updatedUser)

	storedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, originalEmail, storedUser.Email)
	require.Equal(t, "old-hash", storedUser.PasswordHash)
	require.Equal(t, 2.5, storedUser.Balance)
	require.Equal(t, 1, storedUser.Concurrency)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("rollback@example.com"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, identityCount)

	require.Len(t, assigner.calls, 1)
	require.Equal(t, 0, countProviderGrantRecords(t, client, user.ID, "email", "first_bind"))
}

func TestAuthServiceBindEmailIdentity_RejectsReservedEmail(t *testing.T) {
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	svc, _, client := newAuthServiceForEmailBind(t, nil, cache, nil)

	ctx := context.Background()
	user, err := client.User.Create().
		SetEmail("source-user@example.com").
		SetUsername("source-user").
		SetPasswordHash("old-hash").
		SetBalance(1).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "reserved"+service.LinuxDoConnectSyntheticEmailDomain, "123456", "new-password")
	require.ErrorIs(t, err, service.ErrEmailReserved)
	require.Nil(t, updatedUser)
}

func TestAuthServiceBindEmailIdentity_ReplacesBoundEmailAndSkipsFirstBindDefaults(t *testing.T) {
	assigner := &emailBindDefaultSubAssignerStub{}
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	svc, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyAuthSourceDefaultEmailBalance:          "8.5",
		service.SettingKeyAuthSourceDefaultEmailConcurrency:      "4",
		service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":11,"validity_days":30}]`,
		service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "true",
	}, cache, assigner)

	ctx := context.Background()
	hashedPassword, err := svc.HashPassword("current-password")
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("current@example.com").
		SetUsername("bound-user").
		SetPasswordHash(hashedPassword).
		SetBalance(7.5).
		SetConcurrency(3).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("email").
		SetProviderKey("email").
		SetProviderSubject("current@example.com").
		SetVerifiedAt(time.Now().UTC()).
		SetMetadata(map[string]any{"source": "test"}).
		Exec(ctx))

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "new@example.com", "123456", "current-password")
	require.NoError(t, err)
	require.NotNil(t, updatedUser)
	require.Equal(t, "new@example.com", updatedUser.Email)

	storedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "new@example.com", storedUser.Email)
	require.Equal(t, 7.5, storedUser.Balance)
	require.Equal(t, 3, storedUser.Concurrency)
	require.True(t, svc.CheckPassword("current-password", storedUser.PasswordHash))

	newIdentityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("new@example.com"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, newIdentityCount)

	oldIdentityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("current@example.com"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, oldIdentityCount)

	require.Empty(t, assigner.calls)
	require.Equal(t, 0, countProviderGrantRecords(t, client, user.ID, "email", "first_bind"))
}

func TestAuthServiceBindEmailIdentity_RejectsWrongCurrentPasswordForBoundEmail(t *testing.T) {
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	svc, _, client := newAuthServiceForEmailBind(t, nil, cache, nil)

	ctx := context.Background()
	hashedPassword, err := svc.HashPassword("current-password")
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("current@example.com").
		SetUsername("bound-user").
		SetPasswordHash(hashedPassword).
		SetBalance(1).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("email").
		SetProviderKey("email").
		SetProviderSubject("current@example.com").
		SetVerifiedAt(time.Now().UTC()).
		SetMetadata(map[string]any{"source": "test"}).
		Exec(ctx))

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "new@example.com", "123456", "wrong-password")
	require.ErrorIs(t, err, service.ErrPasswordIncorrect)
	require.Nil(t, updatedUser)

	storedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "current@example.com", storedUser.Email)
	require.True(t, svc.CheckPassword("current-password", storedUser.PasswordHash))

	oldIdentityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("current@example.com"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, oldIdentityCount)

	newIdentityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("new@example.com"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, newIdentityCount)
}

func TestAuthServiceBindEmailIdentity_RevokesExistingAccessAndRefreshTokens(t *testing.T) {
	ctx := context.Background()
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		},
	}
	refreshTokenCache := newEmailBindRefreshTokenCacheStub()
	userRepo := newEmailBindUserRepoStub(&service.User{
		ID:           41,
		Email:        "legacy-user" + service.OIDCConnectSyntheticEmailDomain,
		Username:     "legacy-user",
		PasswordHash: "old-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 4,
	})
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-bind-email-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
	}
	emailService := service.NewEmailService(nil, cache)
	svc := service.NewAuthService(nil, userRepo, nil, refreshTokenCache, cfg, nil, emailService, nil, nil, nil, nil, nil)

	oldTokenPair, err := svc.GenerateTokenPair(ctx, &service.User{
		ID:           41,
		Email:        "legacy-user" + service.OIDCConnectSyntheticEmailDomain,
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 4,
	}, "")
	require.NoError(t, err)

	updatedUser, err := svc.BindEmailIdentity(ctx, 41, "new@example.com", "123456", "new-password")
	require.NoError(t, err)
	require.NotNil(t, updatedUser)

	storedUser, err := userRepo.GetByID(ctx, 41)
	require.NoError(t, err)
	require.Equal(t, "new@example.com", storedUser.Email)
	require.True(t, svc.CheckPassword("new-password", storedUser.PasswordHash))

	_, err = svc.RefreshToken(ctx, oldTokenPair.AccessToken)
	require.ErrorIs(t, err, service.ErrTokenRevoked)

	_, err = svc.RefreshTokenPair(ctx, oldTokenPair.RefreshToken)
	require.True(t, errors.Is(err, service.ErrTokenRevoked) || errors.Is(err, service.ErrRefreshTokenInvalid))
}

type emailBindSettingRepoStub struct {
	values map[string]string
}

func (s *emailBindSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *emailBindSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *emailBindSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *emailBindSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (s *emailBindSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *emailBindSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *emailBindSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

type emailBindCacheStub struct {
	data *service.VerificationCodeData
	err  error
}

func (s *emailBindCacheStub) GetVerificationCode(context.Context, string) (*service.VerificationCodeData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func (s *emailBindCacheStub) SetVerificationCode(context.Context, string, *service.VerificationCodeData, time.Duration) error {
	return nil
}

func (s *emailBindCacheStub) DeleteVerificationCode(context.Context, string) error {
	return nil
}

func (s *emailBindCacheStub) GetNotifyVerifyCode(context.Context, string) (*service.VerificationCodeData, error) {
	return nil, nil
}

func (s *emailBindCacheStub) SetNotifyVerifyCode(context.Context, string, *service.VerificationCodeData, time.Duration) error {
	return nil
}

func (s *emailBindCacheStub) DeleteNotifyVerifyCode(context.Context, string) error {
	return nil
}

func (s *emailBindCacheStub) GetPasswordResetToken(context.Context, string) (*service.PasswordResetTokenData, error) {
	return nil, nil
}

func (s *emailBindCacheStub) SetPasswordResetToken(context.Context, string, *service.PasswordResetTokenData, time.Duration) error {
	return nil
}

func (s *emailBindCacheStub) DeletePasswordResetToken(context.Context, string) error {
	return nil
}

func (s *emailBindCacheStub) IsPasswordResetEmailInCooldown(context.Context, string) bool {
	return false
}

func (s *emailBindCacheStub) SetPasswordResetEmailCooldown(context.Context, string, time.Duration) error {
	return nil
}

func (s *emailBindCacheStub) GetNotifyCodeUserRate(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *emailBindCacheStub) IncrNotifyCodeUserRate(context.Context, int64, time.Duration) (int64, error) {
	return 0, nil
}

type emailBindRefreshTokenCacheStub struct {
	mu       sync.Mutex
	tokens   map[string]*service.RefreshTokenData
	userSets map[int64]map[string]struct{}
	families map[string]map[string]struct{}
}

func newEmailBindRefreshTokenCacheStub() *emailBindRefreshTokenCacheStub {
	return &emailBindRefreshTokenCacheStub{
		tokens:   make(map[string]*service.RefreshTokenData),
		userSets: make(map[int64]map[string]struct{}),
		families: make(map[string]map[string]struct{}),
	}
}

func (s *emailBindRefreshTokenCacheStub) StoreRefreshToken(_ context.Context, tokenHash string, data *service.RefreshTokenData, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *data
	s.tokens[tokenHash] = &cloned
	return nil
}

func (s *emailBindRefreshTokenCacheStub) GetRefreshToken(_ context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.tokens[tokenHash]
	if !ok {
		return nil, service.ErrRefreshTokenNotFound
	}
	cloned := *data
	return &cloned, nil
}

func (s *emailBindRefreshTokenCacheStub) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, tokenHash)
	for _, tokenSet := range s.userSets {
		delete(tokenSet, tokenHash)
	}
	for _, tokenSet := range s.families {
		delete(tokenSet, tokenHash)
	}
	return nil
}

func (s *emailBindRefreshTokenCacheStub) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tokenHash := range s.userSets[userID] {
		delete(s.tokens, tokenHash)
		for _, tokenSet := range s.families {
			delete(tokenSet, tokenHash)
		}
	}
	delete(s.userSets, userID)
	return nil
}

func (s *emailBindRefreshTokenCacheStub) DeleteTokenFamily(_ context.Context, familyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tokenHash := range s.families[familyID] {
		delete(s.tokens, tokenHash)
		for _, tokenSet := range s.userSets {
			delete(tokenSet, tokenHash)
		}
	}
	delete(s.families, familyID)
	return nil
}

func (s *emailBindRefreshTokenCacheStub) AddToUserTokenSet(_ context.Context, userID int64, tokenHash string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userSets[userID] == nil {
		s.userSets[userID] = make(map[string]struct{})
	}
	s.userSets[userID][tokenHash] = struct{}{}
	return nil
}

func (s *emailBindRefreshTokenCacheStub) AddToFamilyTokenSet(_ context.Context, familyID string, tokenHash string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.families[familyID] == nil {
		s.families[familyID] = make(map[string]struct{})
	}
	s.families[familyID][tokenHash] = struct{}{}
	return nil
}

func (s *emailBindRefreshTokenCacheStub) GetUserTokenHashes(_ context.Context, userID int64) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenSet := s.userSets[userID]
	out := make([]string, 0, len(tokenSet))
	for tokenHash := range tokenSet {
		out = append(out, tokenHash)
	}
	return out, nil
}

func (s *emailBindRefreshTokenCacheStub) GetFamilyTokenHashes(_ context.Context, familyID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenSet := s.families[familyID]
	out := make([]string, 0, len(tokenSet))
	for tokenHash := range tokenSet {
		out = append(out, tokenHash)
	}
	return out, nil
}

func (s *emailBindRefreshTokenCacheStub) IsTokenInFamily(_ context.Context, familyID string, tokenHash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.families[familyID][tokenHash]
	return ok, nil
}

type emailBindUserRepoStub struct {
	mu           sync.Mutex
	usersByID    map[int64]*service.User
	usersByEmail map[string]*service.User
}

func newEmailBindUserRepoStub(user *service.User) *emailBindUserRepoStub {
	cloned := cloneEmailBindUser(user)
	return &emailBindUserRepoStub{
		usersByID: map[int64]*service.User{
			cloned.ID: cloned,
		},
		usersByEmail: map[string]*service.User{
			cloned.Email: cloned,
		},
	}
}

func (s *emailBindUserRepoStub) Create(context.Context, *service.User) error { return nil }

func (s *emailBindUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[id]
	if !ok {
		return nil, service.ErrUserNotFound
	}
	return cloneEmailBindUser(user), nil
}

func (s *emailBindUserRepoStub) GetByEmail(_ context.Context, email string) (*service.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByEmail[email]
	if !ok {
		return nil, service.ErrUserNotFound
	}
	return cloneEmailBindUser(user), nil
}

func (s *emailBindUserRepoStub) GetFirstAdmin(context.Context) (*service.User, error) {
	panic("unexpected GetFirstAdmin call")
}

func (s *emailBindUserRepoStub) Update(_ context.Context, user *service.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.usersByID[user.ID]
	if !ok {
		return service.ErrUserNotFound
	}
	delete(s.usersByEmail, existing.Email)
	cloned := cloneEmailBindUser(user)
	s.usersByID[user.ID] = cloned
	s.usersByEmail[cloned.Email] = cloned
	return nil
}

func (s *emailBindUserRepoStub) Delete(context.Context, int64) error { return nil }

func (s *emailBindUserRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (s *emailBindUserRepoStub) UpsertUserAvatar(context.Context, int64, service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}

func (s *emailBindUserRepoStub) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected DeleteUserAvatar call")
}

func (s *emailBindUserRepoStub) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *emailBindUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *emailBindUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}

func (s *emailBindUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}

func (s *emailBindUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}

func (s *emailBindUserRepoStub) UpdateBalance(context.Context, int64, float64) error { return nil }
func (s *emailBindUserRepoStub) DeductBalance(context.Context, int64, float64) error  { return nil }
func (s *emailBindUserRepoStub) UpdateConcurrency(context.Context, int64, int) error   { return nil }

func (s *emailBindUserRepoStub) ExistsByEmail(_ context.Context, email string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.usersByEmail[email]
	return ok, nil
}

func (s *emailBindUserRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (s *emailBindUserRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }

func (s *emailBindUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *emailBindUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (s *emailBindUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (s *emailBindUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]service.UserAuthIdentityRecord, error) {
	return nil, nil
}

func (s *emailBindUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	return nil
}

func (s *emailBindUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (s *emailBindUserRepoStub) EnableTotp(context.Context, int64) error                { return nil }
func (s *emailBindUserRepoStub) DisableTotp(context.Context, int64) error               { return nil }

func cloneEmailBindUser(user *service.User) *service.User {
	if user == nil {
		return nil
	}
	cloned := *user
	return &cloned
}
