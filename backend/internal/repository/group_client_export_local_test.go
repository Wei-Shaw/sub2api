//go:build localintegration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// TestGroupClientExportPersistence uses an explicitly configured isolated
// PostgreSQL database, real repositories and auth projections (no ORM mocks).
func TestGroupClientExportPersistence(t *testing.T) {
	dsn := os.Getenv("SUB2API_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "localintegration requires an isolated test database")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB("postgres", db)))
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	repo := NewGroupRepository(client, db)
	cfg := service.GroupClientExportConfig{CodexModel: "gpt-5.6-sol", CodexAuthMode: "api-key", CodexCatalogMode: "file", CodexReasoningEffort: "high", CCUsageMode: "disabled", CCUsageInterval: "15"}
	group := &service.Group{Name: fmt.Sprintf("export-local-%d", suffix), Platform: service.PlatformOpenAI, Status: service.StatusActive, RateMultiplier: 1, SubscriptionType: "standard", ClientExportConfig: cfg}
	require.NoError(t, repo.Create(ctx, group))
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", group.ID) })
	stored, err := repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, cfg, stored.ClientExportConfig)

	// The migration is idempotent and does not erase configured group values.
	migration, err := dbmigrations.FS.ReadFile("246_group_client_export_config.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	stored, err = repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, cfg, stored.ClientExportConfig)

	user, err := client.User.Create().SetEmail(fmt.Sprintf("export-local-%d@example.test", suffix)).SetPasswordHash("local-test-placeholder").SetRole("user").SetStatus("active").Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID) })
	keyRepo := NewAPIKeyRepository(client, db)
	key := &service.APIKey{UserID: user.ID, GroupID: &group.ID, Key: fmt.Sprintf("sk-local-export-%d", suffix), Name: "local export test", Status: "active"}
	require.NoError(t, keyRepo.Create(ctx, key))
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", key.ID) })
	auth, err := keyRepo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, auth.Group)
	require.Equal(t, cfg, auth.Group.ClientExportConfig)

	stored.ClientExportConfig = service.GroupClientExportConfig{}
	require.NoError(t, repo.Update(ctx, stored))
	updated, err := repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.Empty(t, updated.ClientExportConfig.CodexModel)
}
