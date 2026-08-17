//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func requireCanonicalUUIDString(t *testing.T, value string) {
REDACTED
	parsed, err := uuid.Parse(value)
REDACTED
	require.NotEqual(t, uuid.Nil, parsed)
	require.Equal(t, parsed.String(), value)
REDACTED

func TestMigration225BackfillsOnlyEnabledOpenAIOAuthMissingOrMalformedSeeds(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("225_backfill_codex_fingerprint_seed.sql")
REDACTED

	var missingID, blankID, malformedID, validID, offID, apiKeyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-225-missing', 'openai', 'oauth', '{"codex_fingerprint_mode":"session"REDACTED'::jsonb)
RETURNING id
`).Scan(&missingID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-225-blank', 'openai', 'oauth', '{"codex_fingerprint_mode":"device","codex_fingerprint_seed":""REDACTED'::jsonb)
RETURNING id
`).Scan(&blankID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-225-malformed', 'openai', 'oauth', '{"codex_fingerprint_mode":"full","codex_fingerprint_seed":"BAD"REDACTED'::jsonb)
RETURNING id
`).Scan(&malformedID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-225-valid', 'openai', 'oauth', '{"codex_fingerprint_mode":"session","codex_fingerprint_seed":"11111111-1111-4111-8111-111111111111"REDACTED'::jsonb)
RETURNING id
`).Scan(&validID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-225-off', 'openai', 'oauth', '{"codex_fingerprint_mode":"off"REDACTED'::jsonb)
RETURNING id
`).Scan(&offID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-225-apikey', 'openai', 'apikey', '{"codex_fingerprint_mode":"session"REDACTED'::jsonb)
RETURNING id
`).Scan(&apiKeyID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
REDACTED

	seedsAfterFirst := map[int64]string{REDACTED
	for _, id := range []int64{missingID, blankID, malformedID, validIDREDACTED {
		var seed string
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra->>'codex_fingerprint_seed' FROM accounts WHERE id = $1`, id).Scan(&seed))
		requireCanonicalUUIDString(t, seed)
		seedsAfterFirst[id] = seed
REDACTED
	require.Equal(t, "11111111-1111-4111-8111-111111111111", seedsAfterFirst[validID])

	for _, id := range []int64{offID, apiKeyIDREDACTED {
		var hasSeed bool
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra ? 'codex_fingerprint_seed' FROM accounts WHERE id = $1`, id).Scan(&hasSeed))
		require.False(t, hasSeed)
REDACTED

	_, err = tx.ExecContext(ctx, string(migrationSQL))
REDACTED

	for id, want := range seedsAfterFirst {
		var got string
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra->>'codex_fingerprint_seed' FROM accounts WHERE id = $1`, id).Scan(&got))
		require.Equal(t, want, got)
REDACTED
REDACTED

func TestBulkUpdateGeneratesDistinctStableCodexFingerprintSeedsPerEligibleRow(t *testing.T) {
	ctx := context.Background()
	testName := "bulk-codex-seed-" + uuid.NewString()
	type fixture struct {
		name        string
		accountType string
		extra       string
REDACTED
	fixtures := []fixture{
		{name: testName + "-missing-a", accountType: service.AccountTypeOAuth, extra: `{REDACTED`REDACTED,
		{name: testName + "-missing-b", accountType: service.AccountTypeOAuth, extra: `{"codex_fingerprint_seed":"BAD"REDACTED`REDACTED,
		{name: testName + "-valid", accountType: service.AccountTypeOAuth, extra: `{"codex_fingerprint_seed":"11111111-1111-4111-8111-111111111111"REDACTED`REDACTED,
		{name: testName + "-apikey", accountType: service.AccountTypeAPIKey, extra: `{REDACTED`REDACTED,
REDACTED

	ids := make([]int64, 0, len(fixtures))
	for _, f := range fixtures {
		var id int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ($1, 'openai', $2, $3::jsonb)
RETURNING id
`, f.name, f.accountType, f.extra).Scan(&id))
		ids = append(ids, id)
REDACTED
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM scheduler_outbox WHERE account_id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
REDACTED)

	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	updates := service.AccountBulkUpdate{
		Extra: map[string]any{
			"codex_fingerprint_mode": "session",
	REDACTED,
		EnsureCodexFingerprintSeed: true,
REDACTED
	rows, err := repo.BulkUpdate(ctx, ids, updates)
REDACTED
	require.Equal(t, int64(len(ids)), rows)

	readSeed := func(id int64) string {
	REDACTED
		var seed string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE(extra->>'codex_fingerprint_seed', '') FROM accounts WHERE id = $1`, id).Scan(&seed))
		return seed
REDACTED
	firstSeeds := []string{readSeed(ids[0]), readSeed(ids[1]), readSeed(ids[2]), readSeed(ids[3])REDACTED
	requireCanonicalUUIDString(t, firstSeeds[0])
	requireCanonicalUUIDString(t, firstSeeds[1])
	require.NotEqual(t, firstSeeds[0], firstSeeds[1], "gen_random_uuid must be evaluated per eligible row")
	require.Equal(t, "11111111-1111-4111-8111-111111111111", firstSeeds[2])
	require.Empty(t, firstSeeds[3], "API-key accounts must not receive a Codex fingerprint seed")

	rows, err = repo.BulkUpdate(ctx, ids, updates)
REDACTED
	require.Equal(t, int64(len(ids)), rows)
	for i, want := range firstSeeds {
		require.Equal(t, want, readSeed(ids[i]), "retry must not rotate an existing valid seed")
REDACTED
REDACTED
