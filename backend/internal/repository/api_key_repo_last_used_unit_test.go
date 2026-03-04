package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
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

	return &apiKeyRepository{client: clientREDACTED, client
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
