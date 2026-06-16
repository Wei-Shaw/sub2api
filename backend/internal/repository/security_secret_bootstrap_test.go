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
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)

	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestEnsureBootstrapSecretsNilInputs(t *testing.T) {
	err := ensureBootstrapSecrets(context.Background(), nil, &config.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil ent client")

	client := newSecuritySecretTestClient(t)
	err = ensureBootstrapSecrets(context.Background(), client, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil config")
}

func TestEnsureBootstrapSecretsGenerateAndPersistJWTSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{}

	err := ensureBootstrapSecrets(context.Background(), client, cfg)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.JWT.Secret)
	require.GreaterOrEqual(t, len([]byte(cfg.JWT.Secret)), 32)

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyJWT)).Only(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg.JWT.Secret, stored.Value)
}

func TestEnsureBootstrapSecretsLoadExistingJWTSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().SetKey(securitySecretKeyJWT).SetValue("existing-jwt-secret-32bytes-long!!!!").Save(context.Background())
	require.NoError(t, err)

	cfg := &config.Config{}
	err = ensureBootstrapSecrets(context.Background(), client, cfg)
	require.NoError(t, err)
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", cfg.JWT.Secret)
}

func TestEnsureBootstrapSecretsRejectInvalidStoredSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().SetKey(securitySecretKeyJWT).SetValue("too-short").Save(context.Background())
	require.NoError(t, err)

	cfg := &config.Config{}
	err = ensureBootstrapSecrets(context.Background(), client, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 32 bytes")
}

func TestEnsureBootstrapSecretsPersistConfiguredJWTSecret(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "configured-jwt-secret-32bytes-long!!"},
	}

	err := ensureBootstrapSecrets(context.Background(), client, cfg)
	require.NoError(t, err)

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyJWT)).Only(context.Background())
	require.NoError(t, err)
	require.Equal(t, "configured-jwt-secret-32bytes-long!!", stored.Value)
}

func TestEnsureBootstrapSecretsConfiguredSecretTooShort(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "short"}}

	err := ensureBootstrapSecrets(context.Background(), client, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 32 bytes")
}

func TestEnsureBootstrapSecretsConfiguredSecretDuplicateIgnored(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().
		SetKey(securitySecretKeyJWT).
		SetValue("existing-jwt-secret-32bytes-long!!!!").
		Save(context.Background())
	require.NoError(t, err)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "another-configured-jwt-secret-32!!!!"}}
	err = ensureBootstrapSecrets(context.Background(), client, cfg)
	require.NoError(t, err)

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(securitySecretKeyJWT)).Only(context.Background())
	require.NoError(t, err)
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", stored.Value)
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", cfg.JWT.Secret)
}

func TestGetOrCreateGeneratedSecuritySecretTrimmedExistingValue(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := client.SecuritySecret.Create().
		SetKey("trimmed_key").
		SetValue("  existing-trimmed-secret-32bytes-long!!  ").
		Save(context.Background())
	require.NoError(t, err)

	value, created, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, "trimmed_key", 32)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, "existing-trimmed-secret-32bytes-long!!", value)
}

func TestGetOrCreateGeneratedSecuritySecretQueryError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.Close())

	_, _, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, "closed_client_key", 32)
	require.Error(t, err)
}

func TestGetOrCreateGeneratedSecuritySecretCreateValidationError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	tooLongKey := strings.Repeat("k", 101)

	_, _, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, tooLongKey, 32)
	require.Error(t, err)
}

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
		}(i)
	}
	wg.Wait()

	for i := range errs {
		require.NoError(t, errs[i])
		require.NotEmpty(t, values[i])
	}
	for i := 1; i < len(values); i++ {
		require.Equal(t, values[0], values[i])
	}

	createdCount := 0
	for _, created := range createdFlags {
		if created {
			createdCount++
		}
	}
	require.GreaterOrEqual(t, createdCount, 1)
	require.LessOrEqual(t, createdCount, 1)

	count, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestGetOrCreateGeneratedSecuritySecretGenerateError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	originalRead := readRandomBytes
	readRandomBytes = func([]byte) (int, error) {
		return 0, errors.New("boom")
	}
	t.Cleanup(func() {
		readRandomBytes = originalRead
	})

	_, _, err := getOrCreateGeneratedSecuritySecret(context.Background(), client, "gen_error_key", 32)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestCreateSecuritySecretIfAbsent(t *testing.T) {
	client := newSecuritySecretTestClient(t)

	_, err := createSecuritySecretIfAbsent(context.Background(), client, "abc", "short")
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 32 bytes")

	stored, err := createSecuritySecretIfAbsent(context.Background(), client, "abc", "valid-jwt-secret-value-32bytes-long")
	require.NoError(t, err)
	require.Equal(t, "valid-jwt-secret-value-32bytes-long", stored)

	stored, err = createSecuritySecretIfAbsent(context.Background(), client, "abc", "another-valid-secret-value-32bytes")
	require.NoError(t, err)
	require.Equal(t, "valid-jwt-secret-value-32bytes-long", stored)

	count, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ("abc")).Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCreateSecuritySecretIfAbsentValidationError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	_, err := createSecuritySecretIfAbsent(
		context.Background(),
		client,
		strings.Repeat("k", 101),
		"valid-jwt-secret-value-32bytes-long",
	)
	require.Error(t, err)
}

func TestCreateSecuritySecretIfAbsentExecError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.Close())

	_, err := createSecuritySecretIfAbsent(context.Background(), client, "closed-client-key", "valid-jwt-secret-value-32bytes-long")
	require.Error(t, err)
}

func TestQuerySecuritySecretWithRetrySuccess(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	created, err := client.SecuritySecret.Create().
		SetKey("retry_success_key").
		SetValue("retry-success-jwt-secret-value-32!!").
		Save(context.Background())
	require.NoError(t, err)

	got, err := querySecuritySecretWithRetry(context.Background(), client, "retry_success_key")
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "retry-success-jwt-secret-value-32!!", got.Value)
}

func TestQuerySecuritySecretWithRetryExhausted(t *testing.T) {
	client := newSecuritySecretTestClient(t)

	_, err := querySecuritySecretWithRetry(context.Background(), client, "retry_missing_key")
	require.Error(t, err)
	require.True(t, isSecretNotFoundError(err))
}

func TestQuerySecuritySecretWithRetryContextCanceled(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), securitySecretReadRetryWait/2)
	defer cancel()

	_, err := querySecuritySecretWithRetry(ctx, client, "retry_ctx_cancel_key")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestQuerySecuritySecretWithRetryNonNotFoundError(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.Close())

	_, err := querySecuritySecretWithRetry(context.Background(), client, "retry_closed_client_key")
	require.Error(t, err)
	require.False(t, isSecretNotFoundError(err))
}

func TestSecretNotFoundHelpers(t *testing.T) {
	require.False(t, isSecretNotFoundError(nil))
	require.False(t, isSQLNoRowsError(nil))

	require.True(t, isSQLNoRowsError(sql.ErrNoRows))
	require.True(t, isSQLNoRowsError(fmt.Errorf("wrapped: %w", sql.ErrNoRows)))
	require.True(t, isSQLNoRowsError(errors.New("sql: no rows in result set")))

	require.True(t, isSecretNotFoundError(sql.ErrNoRows))
	require.True(t, isSecretNotFoundError(errors.New("sql: no rows in result set")))
	require.False(t, isSecretNotFoundError(errors.New("some other error")))
}

func TestGenerateHexSecretReadError(t *testing.T) {
	originalRead := readRandomBytes
	readRandomBytes = func([]byte) (int, error) {
		return 0, errors.New("read random failed")
	}
	t.Cleanup(func() {
		readRandomBytes = originalRead
	})

	_, err := generateHexSecret(32)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read random failed")
}

func TestGenerateHexSecretLengths(t *testing.T) {
	v1, err := generateHexSecret(0)
	require.NoError(t, err)
	require.Len(t, v1, 64)
	_, err = hex.DecodeString(v1)
	require.NoError(t, err)

	v2, err := generateHexSecret(16)
	require.NoError(t, err)
	require.Len(t, v2, 32)
	_, err = hex.DecodeString(v2)
	require.NoError(t, err)

	require.NotEqual(t, v1, v2)
}
