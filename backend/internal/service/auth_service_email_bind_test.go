//go:build unit

package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type emailBindDefaultSubAssignerStub struct {
	calls []*service.AssignSubscriptionInput
REDACTED

func (s *emailBindDefaultSubAssignerStub) AssignOrExtendSubscription(
	_ context.Context,
	input *service.AssignSubscriptionInput,
) (*service.UserSubscription, bool, error) {
	cloned := *input
	s.calls = append(s.calls, &cloned)
	return &service.UserSubscription{UserID: input.UserID, GroupID: input.GroupIDREDACTED, false, nil
REDACTED

type flakyEmailBindDefaultSubAssignerStub struct {
	err   error
	calls []*service.AssignSubscriptionInput
REDACTED

func (s *flakyEmailBindDefaultSubAssignerStub) AssignOrExtendSubscription(
	_ context.Context,
	input *service.AssignSubscriptionInput,
) (*service.UserSubscription, bool, error) {
	cloned := *input
	s.calls = append(s.calls, &cloned)
	return nil, false, s.err
REDACTED

func newAuthServiceForEmailBind(
	t *testing.T,
	settings map[string]string,
	emailCache service.EmailCache,
	defaultSubAssigner service.DefaultSubscriptionAssigner,
) (*service.AuthService, service.UserRepository, *dbent.Client) {
REDACTED

	db, err := sql.Open("sqlite", "file:auth_service_email_bind?mode=memory&cache=shared")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_provider_default_grants (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	provider_type TEXT NOT NULL,
	grant_reason TEXT NOT NULL DEFAULT 'first_bind',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, provider_type, grant_reason)
)`)
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	repo := repository.NewUserRepository(client, db)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-bind-email-secret",
			ExpireHour: 1,
	REDACTED,
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
	REDACTED,
REDACTED

	settingRepo := &emailBindSettingRepoStub{values: settingsREDACTED
	settingSvc := service.NewSettingService(settingRepo, cfg)

	var emailSvc *service.EmailService
	if emailCache != nil {
		emailSvc = service.NewEmailService(settingRepo, emailCache)
REDACTED

	svc := service.NewAuthService(client, repo, nil, nil, cfg, settingSvc, emailSvc, nil, nil, nil, defaultSubAssigner)
	return svc, repo, client
REDACTED

func TestAuthServiceBindEmailIdentity_UpdatesEmailAndAppliesFirstBindDefaults(t *testing.T) {
	assigner := &emailBindDefaultSubAssignerStub{REDACTED
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	REDACTED,
REDACTED
	svc, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyAuthSourceDefaultEmailBalance:          "8.5",
		service.SettingKeyAuthSourceDefaultEmailConcurrency:      "4",
		service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":11,"validity_days":30REDACTED]`,
		service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "true",
REDACTED, cache, assigner)

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
REDACTED

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "  NewEmail@Example.com  ", "123456", "new-password")
REDACTED
	require.NotNil(t, updatedUser)
	require.Equal(t, "newemail@example.com", updatedUser.Email)

	storedUser, err := client.User.Get(ctx, user.ID)
REDACTED
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
REDACTED
	require.Equal(t, 1, identityCount)

	require.Len(t, assigner.calls, 1)
	require.Equal(t, user.ID, assigner.calls[0].UserID)
	require.Equal(t, int64(11), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
	require.Equal(t, 1, countProviderGrantRecords(t, client, user.ID, "email", "first_bind"))
REDACTED

func TestAuthServiceBindEmailIdentity_RejectsExistingEmailOnAnotherUser(t *testing.T) {
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	REDACTED,
REDACTED
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
REDACTED
	_, err = client.User.Create().
		SetEmail("taken@example.com").
		SetUsername("taken-user").
		SetPasswordHash("hash").
		SetBalance(1).
		SetConcurrency(1).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	updatedUser, err := svc.BindEmailIdentity(ctx, sourceUser.ID, "taken@example.com", "123456", "new-password")
	require.ErrorIs(t, err, service.ErrEmailExists)
	require.Nil(t, updatedUser)

	storedUser, err := client.User.Get(ctx, sourceUser.ID)
REDACTED
	require.Equal(t, "source-user"+service.OIDCConnectSyntheticEmailDomain, storedUser.Email)
	require.Equal(t, 0, countProviderGrantRecords(t, client, sourceUser.ID, "email", "first_bind"))
REDACTED

func TestAuthServiceBindEmailIdentity_RollsBackWhenFirstBindDefaultsFail(t *testing.T) {
	assigner := &flakyEmailBindDefaultSubAssignerStub{err: errors.New("temporary assign failure")REDACTED
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	REDACTED,
REDACTED
	svc, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyAuthSourceDefaultEmailBalance:          "8.5",
		service.SettingKeyAuthSourceDefaultEmailConcurrency:      "4",
		service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":11,"validity_days":30REDACTED]`,
		service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "true",
REDACTED, cache, assigner)

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
REDACTED

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "rollback@example.com", "123456", "new-password")
	require.ErrorContains(t, err, "apply email first bind defaults")
	require.ErrorContains(t, err, "temporary assign failure")
	require.Nil(t, updatedUser)

	storedUser, err := client.User.Get(ctx, user.ID)
REDACTED
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
REDACTED
	require.Equal(t, 0, identityCount)

	require.Len(t, assigner.calls, 1)
	require.Equal(t, 0, countProviderGrantRecords(t, client, user.ID, "email", "first_bind"))
REDACTED

func TestAuthServiceBindEmailIdentity_RejectsReservedEmail(t *testing.T) {
	cache := &emailBindCacheStub{
		data: &service.VerificationCodeData{
			Code:      "123456",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	REDACTED,
REDACTED
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
REDACTED

	updatedUser, err := svc.BindEmailIdentity(ctx, user.ID, "reserved"+service.LinuxDoConnectSyntheticEmailDomain, "123456", "new-password")
	require.ErrorIs(t, err, service.ErrEmailReserved)
	require.Nil(t, updatedUser)
REDACTED

type emailBindSettingRepoStub struct {
	values map[string]string
REDACTED

func (s *emailBindSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *emailBindSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
REDACTED
	return "", service.ErrSettingNotFound
REDACTED

func (s *emailBindSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
REDACTED

func (s *emailBindSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *emailBindSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *emailBindSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *emailBindSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
REDACTED

type emailBindCacheStub struct {
	data *service.VerificationCodeData
	err  error
REDACTED

func (s *emailBindCacheStub) GetVerificationCode(context.Context, string) (*service.VerificationCodeData, error) {
	if s.err != nil {
		return nil, s.err
REDACTED
	return s.data, nil
REDACTED

func (s *emailBindCacheStub) SetVerificationCode(context.Context, string, *service.VerificationCodeData, time.Duration) error {
	return nil
REDACTED

func (s *emailBindCacheStub) DeleteVerificationCode(context.Context, string) error {
	return nil
REDACTED

func (s *emailBindCacheStub) GetNotifyVerifyCode(context.Context, string) (*service.VerificationCodeData, error) {
	return nil, nil
REDACTED

func (s *emailBindCacheStub) SetNotifyVerifyCode(context.Context, string, *service.VerificationCodeData, time.Duration) error {
	return nil
REDACTED

func (s *emailBindCacheStub) DeleteNotifyVerifyCode(context.Context, string) error {
	return nil
REDACTED

func (s *emailBindCacheStub) GetPasswordResetToken(context.Context, string) (*service.PasswordResetTokenData, error) {
	return nil, nil
REDACTED

func (s *emailBindCacheStub) SetPasswordResetToken(context.Context, string, *service.PasswordResetTokenData, time.Duration) error {
	return nil
REDACTED

func (s *emailBindCacheStub) DeletePasswordResetToken(context.Context, string) error {
	return nil
REDACTED

func (s *emailBindCacheStub) IsPasswordResetEmailInCooldown(context.Context, string) bool {
	return false
REDACTED

func (s *emailBindCacheStub) SetPasswordResetEmailCooldown(context.Context, string, time.Duration) error {
	return nil
REDACTED

func (s *emailBindCacheStub) GetNotifyCodeUserRate(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED

func (s *emailBindCacheStub) IncrNotifyCodeUserRate(context.Context, int64, time.Duration) (int64, error) {
	return 0, nil
REDACTED
