package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newSecuritySecretTestClient(t *testing.T) *dbent.Client {
REDACTED
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)

	db, err := sql.Open("sqlite", dsn)
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	return client
REDACTED

func TestEnsureBootstrapSecretsNilInputs(t *testing.T) {
	err := ensureBootstrapSecrets(context.Background(), nil, &config.Config{REDACTED)
REDACTED
	require.Contains(t, err.Error(), "nil ent client")

	client := newSecuritySecretTestClient(t)
	err = ensureBootstrapSecrets(context.Background(), client, nil)
REDACTED
	require.Contains(t, err.Error(), "nil config")
REDACTED

func TestEnsureBootstrapSecretsGenerateAndPersistJWTSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{REDACTED

	err := ensureBootstrapSecrets(context.Background(), client, cfg)
REDACTED
	require.NotEmpty(t, cfg.JWT.Secret)
	require.GreaterOrEqual(t, len([]byte(cfg.JWT.Secret)), 32)

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyJWT)).Only(context.Background())
REDACTED
	require.Equal(t, cfg.JWT.Secret, stored.Value)
REDACTED

func TestEnsureBootstrapSecretsLoadExistingJWTSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().SetKey(securitySecretKeyJWT).SetValue("existing-jwt-secret-32bytes-long!!!!").Save(context.Background())
REDACTED

	cfg := &config.Config{REDACTED
	err = ensureBootstrapSecrets(context.Background(), client, cfg)
REDACTED
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", cfg.JWT.Secret)
REDACTED

func TestEnsureBootstrapSecretsRejectInvalidStoredSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().SetKey(securitySecretKeyJWT).SetValue("too-short").Save(context.Background())
REDACTED

	cfg := &config.Config{REDACTED
	err = ensureBootstrapSecrets(context.Background(), client, cfg)
REDACTED
	require.Contains(t, err.Error(), "at least 32 bytes")
REDACTED

func TestEnsureBootstrapSecretsPersistConfiguredJWTSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "configured-jwt-secret-32bytes-long!!"REDACTED,
REDACTED

	err := ensureBootstrapSecrets(context.Background(), client, cfg)
REDACTED

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyJWT)).Only(context.Background())
REDACTED
	require.Equal(t, "configured-jwt-secret-32bytes-long!!", stored.Value)
REDACTED

func TestEnsureBootstrapSecretsConfiguredSecretTooShort(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "short"REDACTEDREDACTED

	err := ensureBootstrapSecrets(context.Background(), client, cfg)
REDACTED
	require.Contains(t, err.Error(), "at least 32 bytes")
REDACTED

func TestEnsureBootstrapSecretsConfiguredSecretDuplicateIgnored(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().
		SetKey(securitySecretKeyJWT).
		SetValue("existing-jwt-secret-32bytes-long!!!!").
		Save(context.Background())
REDACTED

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "another-configured-jwt-secret-32!!!!"REDACTEDREDACTED
	err = ensureBootstrapSecrets(context.Background(), client, cfg)
REDACTED

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyJWT)).Only(context.Background())
REDACTED
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", stored.Value)
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", cfg.JWT.Secret)
REDACTED

func TestGetOrCreateGeneratedSecuritySecretTrimmedExistingValue(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().
		SetKey("trimmed_key").
		SetValue("  existing-trimmed-secret-32bytes-long!!  ").
		Save(context.Background())
REDACTED

	value, created, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, "trimmed_key", 32)
REDACTED
	require.False(t, created)
	require.Equal(t, "existing-trimmed-secret-32bytes-long!!", value)
REDACTED

func TestGetOrCreateGeneratedSecuritySecretQueryError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.Close())

	_, _, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, "closed_client_key", 32)
REDACTED
REDACTED

func TestGetOrCreateGeneratedSecuritySecretCreateValidationError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	tooLongKey := strings.Repeat("k", 101)

	_, _, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, tooLongKey, 32)
REDACTED
REDACTED

func TestGetOrCreateGeneratedSecuritySecretConcurrentCreation(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	const goroutines = 8
	key := "concurrent_bootstrap_key"

	values := make([]string, goroutines)
	createdFlags := make([]bool, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			values[idx], createdFlags[idx], errs[idx] = getOrCreateGeneratedSecuritySecret(context.Background(), client, key, 32)
	REDACTED(i)
REDACTED
	wg.Wait()

	for i := range errs {
		require.NoError(t, errs[i])
		require.NotEmpty(t, values[i])
REDACTED
	for i := 1; i < len(values); i++ {
		require.Equal(t, values[0], values[i])
REDACTED

	createdCount := 0
	for _, created := range createdFlags {
		if created {
			createdCount++
	REDACTED
REDACTED
	require.GreaterOrEqual(t, createdCount, 1)
	require.LessOrEqual(t, createdCount, 1)

	count, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Count(context.Background())
REDACTED
	require.Equal(t, 1, count)
REDACTED

func TestGetOrCreateGeneratedSecuritySecretGenerateError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	originalRead := readRandomBytes
	readRandomBytes = func([]byte) (int, error) {
		return 0, errors.New("boom")
REDACTED
	t.Cleanup(func() {
		readRandomBytes = originalRead
REDACTED)

	_, _, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, "gen_error_key", 32)
REDACTED
	require.Contains(t, err.Error(), "boom")
REDACTED

func TestCreateSecuritySecretIfAbsent(t *testing.T) {
	client := newSecuritySecretTestClient(t)

	_, err := createSecuritySecretIfAbsent(context.Background(), client, "abc", "short")
REDACTED
	require.Contains(t, err.Error(), "at least 32 bytes")

	stored, err := createSecuritySecretIfAbsent(context.Background(), client, "abc", "valid-jwt-secret-value-32bytes-long")
REDACTED
	require.Equal(t, "valid-jwt-secret-value-32bytes-long", stored)

	stored, err = createSecuritySecretIfAbsent(context.Background(), client, "abc", "another-valid-secret-value-32bytes")
REDACTED
	require.Equal(t, "valid-jwt-secret-value-32bytes-long", stored)

	count, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ("abc")).Count(context.Background())
REDACTED
	require.Equal(t, 1, count)
REDACTED

func TestCreateSecuritySecretIfAbsentValidationError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := createSecuritySecretIfAbsent(
		context.Background(),
		client,
		strings.Repeat("k", 101),
		"valid-jwt-secret-value-32bytes-long",
	)
REDACTED
REDACTED

func TestCreateSecuritySecretIfAbsentExecError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.Close())

	_, err := createSecuritySecretIfAbsent(context.Background(), client, "closed-client-key", "valid-jwt-secret-value-32bytes-long")
REDACTED
REDACTED

func TestQuerySecuritySecretWithRetrySuccess(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	created, err := client.SecuritySecret.Create().
		SetKey("retry_success_key").
		SetValue("retry-success-jwt-secret-value-32!!").
		Save(context.Background())
REDACTED

	got, err := querySecuritySecretWithRetry(context.Background(), client, "retry_success_key")
REDACTED
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "retry-success-jwt-secret-value-32!!", got.Value)
REDACTED

func TestQuerySecuritySecretWithRetryExhausted(t *testing.T) {
	client := newSecuritySecretTestClient(t)

	_, err := querySecuritySecretWithRetry(context.Background(), client, "retry_missing_key")
REDACTED
	require.True(t, isSecretNotFoundError(err))
REDACTED

func TestQuerySecuritySecretWithRetryContextCanceled(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), securitySecretReadRetryWait/2)
	defer cancel()

	_, err := querySecuritySecretWithRetry(ctx, client, "retry_ctx_cancel_key")
REDACTED
	require.ErrorIs(t, err, context.DeadlineExceeded)
REDACTED

func TestQuerySecuritySecretWithRetryNonNotFoundError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.Close())

	_, err := querySecuritySecretWithRetry(context.Background(), client, "retry_closed_client_key")
REDACTED
	require.False(t, isSecretNotFoundError(err))
REDACTED

func TestSecretNotFoundHelpers(t *testing.T) {
	require.False(t, isSecretNotFoundError(nil))
	require.False(t, isSQLNoRowsError(nil))

	require.True(t, isSQLNoRowsError(sql.ErrNoRows))
	require.True(t, isSQLNoRowsError(fmt.Errorf("wrapped: %w", sql.ErrNoRows)))
	require.True(t, isSQLNoRowsError(errors.New("sql: no rows in result set")))

	require.True(t, isSecretNotFoundError(sql.ErrNoRows))
	require.True(t, isSecretNotFoundError(errors.New("sql: no rows in result set")))
	require.False(t, isSecretNotFoundError(errors.New("some other error")))
REDACTED

func TestGenerateHexSecretReadError(t *testing.T) {
	originalRead := readRandomBytes
	readRandomBytes = func([]byte) (int, error) {
		return 0, errors.New("read random failed")
REDACTED
	t.Cleanup(func() {
		readRandomBytes = originalRead
REDACTED)

	_, err := generateHexSecret(32)
REDACTED
	require.Contains(t, err.Error(), "read random failed")
REDACTED

func TestGenerateHexSecretLengths(t *testing.T) {
	v1, err := generateHexSecret(0)
REDACTED
	require.Len(t, v1, 64)
	_, err = hex.DecodeString(v1)
REDACTED

	v2, err := generateHexSecret(16)
REDACTED
	require.Len(t, v2, 32)
	_, err = hex.DecodeString(v2)
REDACTED

	require.NotEqual(t, v1, v2)
REDACTED
