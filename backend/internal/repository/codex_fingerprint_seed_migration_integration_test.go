//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func requireCanonicalUUIDString(t *testing.T, value string) {
	t.Helper()
	parsed, err := uuid.Parse(value)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, parsed)
	require.Equal(t, parsed.String(), value)
}

func TestMigration225BackfillsOnlyEnabledOpenAIOAuthMissingOrMalformedSeeds(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	insertAccount := func(name, accountType, extra string) int64 {
		t.Helper()
		id, err := insertReturningID(ctx, tx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (`+ph(1)+`, 'openai', `+ph(2)+`, `+ph(3)+`)`, name, accountType, extra)
		require.NoError(t, err)
		return id
	}

	missingID := insertAccount("migration-225-missing", "oauth", `{"codex_fingerprint_mode":"session"}`)
	blankID := insertAccount("migration-225-blank", "oauth", `{"codex_fingerprint_mode":"device","codex_fingerprint_seed":""}`)
	malformedID := insertAccount("migration-225-malformed", "oauth", `{"codex_fingerprint_mode":"full","codex_fingerprint_seed":"BAD"}`)
	validID := insertAccount("migration-225-valid", "oauth", `{"codex_fingerprint_mode":"session","codex_fingerprint_seed":"11111111-1111-4111-8111-111111111111"}`)
	offID := insertAccount("migration-225-off", "oauth", `{"codex_fingerprint_mode":"off"}`)
	apiKeyID := insertAccount("migration-225-apikey", "apikey", `{"codex_fingerprint_mode":"session"}`)

	_, err := tx.ExecContext(ctx, readMigrationSQLForDialect(t, "225_backfill_codex_fingerprint_seed.sql"))
	require.NoError(t, err)

	selectSeed := `SELECT ` + jsonExtractTextExpr("extra", "codex_fingerprint_seed") + ` FROM accounts WHERE id = ` + ph(1)
	seedsAfterFirst := map[int64]string{}
	for _, id := range []int64{missingID, blankID, malformedID, validID} {
		var seed string
		require.NoError(t, tx.QueryRowContext(ctx, selectSeed, id).Scan(&seed))
		requireCanonicalUUIDString(t, seed)
		seedsAfterFirst[id] = seed
	}
	require.Equal(t, "11111111-1111-4111-8111-111111111111", seedsAfterFirst[validID])

	hasSeedStmt := `SELECT ` + jsonHasPathExpr("extra", "codex_fingerprint_seed") + ` FROM accounts WHERE id = ` + ph(1)
	for _, id := range []int64{offID, apiKeyID} {
		var hasSeed bool
		require.NoError(t, tx.QueryRowContext(ctx, hasSeedStmt, id).Scan(&hasSeed))
		require.False(t, hasSeed)
	}

	// 该迁移是单条幂等 UPDATE（MySQL 翻译版同为纯 UPDATE），重放不轮换有效 seed。
	_, err = tx.ExecContext(ctx, readMigrationSQLForDialect(t, "225_backfill_codex_fingerprint_seed.sql"))
	require.NoError(t, err)

	for id, want := range seedsAfterFirst {
		var got string
		require.NoError(t, tx.QueryRowContext(ctx, selectSeed, id).Scan(&got))
		require.Equal(t, want, got)
	}
}

func TestBulkUpdateGeneratesDistinctStableCodexFingerprintSeedsPerEligibleRow(t *testing.T) {
	ctx := context.Background()
	testName := "bulk-codex-seed-" + uuid.NewString()
	type fixture struct {
		name        string
		accountType string
		extra       string
	}
	fixtures := []fixture{
		{name: testName + "-missing-a", accountType: service.AccountTypeOAuth, extra: `{}`},
		{name: testName + "-missing-b", accountType: service.AccountTypeOAuth, extra: `{"codex_fingerprint_seed":"BAD"}`},
		{name: testName + "-valid", accountType: service.AccountTypeOAuth, extra: `{"codex_fingerprint_seed":"11111111-1111-4111-8111-111111111111"}`},
		{name: testName + "-apikey", accountType: service.AccountTypeAPIKey, extra: `{}`},
	}

	ids := make([]int64, 0, len(fixtures))
	for _, f := range fixtures {
		id, err := insertReturningID(ctx, integrationDB, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (`+ph(1)+`, 'openai', `+ph(2)+`, `+ph(3)+`)`, f.name, f.accountType, f.extra)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	t.Cleanup(func() {
		if IsMySQL() {
			idArgs := make([]any, 0, len(ids))
			for _, id := range ids {
				idArgs = append(idArgs, id)
			}
			_, _ = integrationDB.ExecContext(context.Background(),
				"DELETE FROM scheduler_outbox WHERE account_id IN ("+Placeholders(len(ids))+")", idArgs...)
			_, _ = integrationDB.ExecContext(context.Background(),
				"DELETE FROM accounts WHERE id IN ("+Placeholders(len(ids))+")", idArgs...)
			return
		}
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM scheduler_outbox WHERE account_id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
	})

	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	updates := service.AccountBulkUpdate{
		Extra: map[string]any{
			"codex_fingerprint_mode": "session",
		},
		EnsureCodexFingerprintSeed: true,
	}
	rows, err := repo.BulkUpdate(ctx, ids, updates)
	require.NoError(t, err)
	require.Equal(t, int64(len(ids)), rows)

	readSeed := func(id int64) string {
		t.Helper()
		var seed string
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			`SELECT COALESCE(`+jsonExtractTextExpr("extra", "codex_fingerprint_seed")+`, '') FROM accounts WHERE id = `+ph(1), id).Scan(&seed))
		return seed
	}
	firstSeeds := []string{readSeed(ids[0]), readSeed(ids[1]), readSeed(ids[2]), readSeed(ids[3])}
	requireCanonicalUUIDString(t, firstSeeds[0])
	requireCanonicalUUIDString(t, firstSeeds[1])
	require.NotEqual(t, firstSeeds[0], firstSeeds[1], "gen_random_uuid must be evaluated per eligible row")
	require.Equal(t, "11111111-1111-4111-8111-111111111111", firstSeeds[2])
	require.Empty(t, firstSeeds[3], "API-key accounts must not receive a Codex fingerprint seed")

	rows, err = repo.BulkUpdate(ctx, ids, updates)
	require.NoError(t, err)
	require.Equal(t, int64(len(ids)), rows)
	for i, want := range firstSeeds {
		require.Equal(t, want, readSeed(ids[i]), "retry must not rotate an existing valid seed")
	}
}
