package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAPIKeyRepoSQLite(t *testing.T) (*apiKeyRepository, *dbent.Client) {
REDACTED

	db, err := sql.Open("sqlite", "file:api_key_repo_last_used?mode=memory&cache=shared")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	return &apiKeyRepository{client: client, sql: dbREDACTED, client
REDACTED

func mustCreateAPIKeyRepoUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *service.User {
REDACTED
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED
	return userEntityToService(u)
REDACTED

func mustCreateAPIKeyRepoAccount(t *testing.T, ctx context.Context, client *dbent.Client, name string) int64 {
REDACTED
	a, err := client.Account.Create().
		SetName(name).
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeAPIKey).
		SetStatus(service.StatusActive).
		SetCredentials(map[string]any{"api_key": "sk-test"REDACTED).
		Save(ctx)
REDACTED
	return a.ID
REDACTED

func mustCreateAPIKeyRepoUsageLog(t *testing.T, ctx context.Context, client *dbent.Client, userID, apiKeyID, accountID int64, requestID string, createdAt time.Time, ipAddress *string) {
REDACTED
	builder := client.UsageLog.Create().
		SetUserID(userID).
		SetAPIKeyID(apiKeyID).
		SetAccountID(accountID).
		SetRequestID(requestID).
		SetModel("gpt-5").
		SetCreatedAt(createdAt)
	if ipAddress != nil {
		builder.SetIPAddress(*ipAddress)
REDACTED
	_, err := builder.Save(ctx)
REDACTED
REDACTED

func TestAPIKeyRepositoryListByUserIDAttachesLastUsedIP(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "list-last-used-ip@test.com")
	accountID := mustCreateAPIKeyRepoAccount(t, ctx, client, "acc-list-last-used-ip")

	withLogs := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-list-last-used-ip-logs",
		Name:   "With Logs",
		Status: service.StatusActive,
REDACTED
	emptyOnly := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-list-last-used-ip-empty",
		Name:   "Empty Only",
		Status: service.StatusActive,
REDACTED
	noLogs := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-list-last-used-ip-none",
		Name:   "No Logs",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, withLogs))
	require.NoError(t, repo.Create(ctx, emptyOnly))
	require.NoError(t, repo.Create(ctx, noLogs))

	olderIP := "198.51.100.10"
	newerEmptyIP := ""
	newestIP := "203.0.113.20"
	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	mustCreateAPIKeyRepoUsageLog(t, ctx, client, user.ID, withLogs.ID, accountID, "req-last-ip-older", base, &olderIP)
	mustCreateAPIKeyRepoUsageLog(t, ctx, client, user.ID, withLogs.ID, accountID, "req-last-ip-empty", base.Add(time.Hour), &newerEmptyIP)
	mustCreateAPIKeyRepoUsageLog(t, ctx, client, user.ID, withLogs.ID, accountID, "req-last-ip-newest", base.Add(2*time.Hour), &newestIP)
	mustCreateAPIKeyRepoUsageLog(t, ctx, client, user.ID, emptyOnly.ID, accountID, "req-empty-ip", base.Add(3*time.Hour), &newerEmptyIP)

	keys, _, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, service.APIKeyListFilters{REDACTED)
REDACTED

	byID := make(map[int64]service.APIKey, len(keys))
	for _, key := range keys {
		byID[key.ID] = key
REDACTED
	require.NotNil(t, byID[withLogs.ID].LastUsedIP)
	require.Equal(t, newestIP, *byID[withLogs.ID].LastUsedIP)
	require.Nil(t, byID[emptyOnly.ID].LastUsedIP)
	require.Nil(t, byID[noLogs.ID].LastUsedIP)
REDACTED

func TestAPIKeyRepository_CreateWithLastUsedAt(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "create-last-used@test.com")

	lastUsed := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	key := &service.APIKey{
		UserID:     user.ID,
		Key:        "sk-create-last-used",
		Name:       "CreateWithLastUsed",
		Status:     service.StatusActive,
		LastUsedAt: &lastUsed,
REDACTED

	require.NoError(t, repo.Create(ctx, key))
	require.NotNil(t, key.LastUsedAt)
	require.WithinDuration(t, lastUsed, *key.LastUsedAt, time.Second)

	got, err := repo.GetByID(ctx, key.ID)
REDACTED
	require.NotNil(t, got.LastUsedAt)
	require.WithinDuration(t, lastUsed, *got.LastUsedAt, time.Second)
REDACTED

func TestAPIKeyRepository_UpdateLastUsed(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "update-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used",
		Name:   "UpdateLastUsed",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, key))

	before, err := repo.GetByID(ctx, key.ID)
REDACTED
	require.Nil(t, before.LastUsedAt)

	target := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	require.NoError(t, repo.UpdateLastUsed(ctx, key.ID, target))

	after, err := repo.GetByID(ctx, key.ID)
REDACTED
	require.NotNil(t, after.LastUsedAt)
	require.WithinDuration(t, target, *after.LastUsedAt, time.Second)
	require.WithinDuration(t, target, after.UpdatedAt, time.Second)
REDACTED

func TestAPIKeyRepository_UpdateLastUsedDeletedKey(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "deleted-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used-deleted",
		Name:   "UpdateLastUsedDeleted",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, key))
	require.NoError(t, repo.Delete(ctx, key.ID))

	err := repo.UpdateLastUsed(ctx, key.ID, time.Now().UTC())
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
REDACTED

func TestAPIKeyRepository_UpdateLastUsedDBError(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "db-error-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used-db-error",
		Name:   "UpdateLastUsedDBError",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, key))

	require.NoError(t, client.Close())
	err := repo.UpdateLastUsed(ctx, key.ID, time.Now().UTC())
REDACTED
REDACTED

func TestAPIKeyRepository_CreateDuplicateKey(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "duplicate-key@test.com")

	first := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-duplicate",
		Name:   "first",
		Status: service.StatusActive,
REDACTED
	second := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-duplicate",
		Name:   "second",
		Status: service.StatusActive,
REDACTED

	require.NoError(t, repo.Create(ctx, first))
	err := repo.Create(ctx, second)
	require.ErrorIs(t, err, service.ErrAPIKeyExists)
REDACTED
