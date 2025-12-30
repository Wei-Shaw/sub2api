//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func uniqueSoftDeleteValue(t *testing.T, prefix string) string {
REDACTED
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("%s-%s", prefix, safeName)
REDACTED

func createEntUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *dbent.User {
REDACTED

	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		Save(ctx)
	require.NoError(t, err, "create ent user")
	return u
REDACTED

func TestEntSoftDelete_ApiKey_DefaultFilterAndSkip(t *testing.T) {
	ctx := context.Background()
	// 使用全局 ent client，确保软删除验证在实际持久化数据上进行。
	client := testEntClient(t)

	u := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "sd-user")+"@example.com")

	repo := NewApiKeyRepository(client)
	key := &service.ApiKey{
		UserID: u.ID,
		Key:    uniqueSoftDeleteValue(t, "sk-soft-delete"),
		Name:   "soft-delete",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, key), "create api key")

	require.NoError(t, repo.Delete(ctx, key.ID), "soft delete api key")

	_, err := repo.GetByID(ctx, key.ID)
	require.ErrorIs(t, err, service.ErrApiKeyNotFound, "deleted rows should be hidden by default")

	_, err = client.ApiKey.Query().Where(apikey.IDEQ(key.ID)).Only(ctx)
	require.Error(t, err, "default ent query should not see soft-deleted rows")
	require.True(t, dbent.IsNotFound(err), "expected ent not-found after default soft delete filter")

	got, err := client.ApiKey.Query().
		Where(apikey.IDEQ(key.ID)).
		Only(mixins.SkipSoftDelete(ctx))
	require.NoError(t, err, "SkipSoftDelete should include soft-deleted rows")
	require.NotNil(t, got.DeletedAt, "deleted_at should be set after soft delete")
REDACTED

func TestEntSoftDelete_ApiKey_DeleteIdempotent(t *testing.T) {
	ctx := context.Background()
	// 使用全局 ent client，避免事务回滚影响幂等性验证。
	client := testEntClient(t)

	u := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "sd-user2")+"@example.com")

	repo := NewApiKeyRepository(client)
	key := &service.ApiKey{
		UserID: u.ID,
		Key:    uniqueSoftDeleteValue(t, "sk-soft-delete2"),
		Name:   "soft-delete2",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, key), "create api key")

	require.NoError(t, repo.Delete(ctx, key.ID), "first delete")
	require.NoError(t, repo.Delete(ctx, key.ID), "second delete should be idempotent")
REDACTED

func TestEntSoftDelete_ApiKey_HardDeleteViaSkipSoftDelete(t *testing.T) {
	ctx := context.Background()
	// 使用全局 ent client，确保 SkipSoftDelete 的硬删除语义可验证。
	client := testEntClient(t)

	u := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "sd-user3")+"@example.com")

	repo := NewApiKeyRepository(client)
	key := &service.ApiKey{
		UserID: u.ID,
		Key:    uniqueSoftDeleteValue(t, "sk-soft-delete3"),
		Name:   "soft-delete3",
		Status: service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, key), "create api key")

	require.NoError(t, repo.Delete(ctx, key.ID), "soft delete api key")

	// Hard delete using SkipSoftDelete so the hook doesn't convert it to update-deleted_at.
	_, err := client.ApiKey.Delete().Where(apikey.IDEQ(key.ID)).Exec(mixins.SkipSoftDelete(ctx))
	require.NoError(t, err, "hard delete")

	_, err = client.ApiKey.Query().
		Where(apikey.IDEQ(key.ID)).
		Only(mixins.SkipSoftDelete(ctx))
	require.True(t, dbent.IsNotFound(err), "expected row to be hard deleted")
REDACTED
