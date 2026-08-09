package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func testCredentialCipher(t *testing.T, keyByte string, keyID string) *CredentialCipher {
	t.Helper()
	cipher, err := NewCredentialCipher(&config.Config{
		Server: config.ServerConfig{Mode: "release"},
		CredentialEncryption: config.CredentialEncryptionConfig{
			Key:   strings.Repeat(keyByte, 64),
			KeyID: keyID,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, cipher)
	return cipher
}

func TestCredentialCipherRoundTripUsesRandomNonceAndNoPlaintext(t *testing.T) {
	cipher := testCredentialCipher(t, "a", "primary-2026")
	credentials := map[string]any{
		"access_token":  "secret-access-token",
		"refresh_token": "secret-refresh-token",
		"api_key":       "secret-api-key",
		"base_url":      "https://api.example.test/v1",
		"model_mapping": map[string]any{"alias": "model"},
	}
	first, err := cipher.Encrypt(42, "openai", credentials)
	require.NoError(t, err)
	second, err := cipher.Encrypt(42, "openai", credentials)
	require.NoError(t, err)
	require.NotEqual(t, first[credentialEnvelopeKey], second[credentialEnvelopeKey])

	storedJSON, err := json.Marshal(first)
	require.NoError(t, err)
	for _, secret := range []string{"secret-access-token", "secret-refresh-token", "secret-api-key", "model"} {
		require.NotContains(t, string(storedJSON), secret)
	}
	require.NotContains(t, string(storedJSON), "api.example.test")
	require.Contains(t, string(storedJSON), "primary-2026")
	require.Contains(t, string(storedJSON), credentialEnvelopeAlgorithm)

	decoded, legacy, err := cipher.Decrypt(42, "openai", first)
	require.NoError(t, err)
	require.False(t, legacy)
	require.Equal(t, credentials, decoded)
}

func TestOllamaBaseURLProjectionNeverStoresURLSecrets(t *testing.T) {
	require.True(t, isOllamaCloudBaseURL("HTTPS://WWW.OLLAMA.COM:443/v1"))
	require.False(t, isOllamaCloudBaseURL("https://token@ollama.com/v1"))
	require.False(t, isOllamaCloudBaseURL("https://ollama.com/v1?"))
	require.False(t, isOllamaCloudBaseURL("https://ollama.com/v1?api_key=secret"))
	require.False(t, isOllamaCloudBaseURL("https://ollama.com/v1#secret"))
}

func TestSchedulerCacheCredentialPayloadIsEncryptedAndAADBound(t *testing.T) {
	cipher := testCredentialCipher(t, "f", "primary")
	account := service.Account{
		ID:       99,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "cache-access-secret",
			"api_key":      "cache-api-secret",
		},
		Proxy: &service.Proxy{
			ID:       77,
			Protocol: "https",
			Host:     "proxy.example.test",
			Port:     443,
			Username: "proxy-user",
			Password: "cache-proxy-password",
		},
	}
	full, meta, err := marshalSchedulerCacheAccountWithCipher(account, cipher)
	require.NoError(t, err)
	require.NotContains(t, string(full), "cache-access-secret")
	require.NotContains(t, string(full), "cache-api-secret")
	require.NotContains(t, string(full), "cache-proxy-password")
	require.NotContains(t, string(meta), "cache-api-secret")

	decoded, err := decodeCachedAccountWithCipher(full, cipher)
	require.NoError(t, err)
	require.Equal(t, account.Credentials, decoded.Credentials)
	require.Equal(t, account.Proxy, decoded.Proxy)

	var tampered map[string]any
	require.NoError(t, json.Unmarshal(full, &tampered))
	tampered["ID"] = float64(100)
	tamperedJSON, err := json.Marshal(tampered)
	require.NoError(t, err)
	_, err = decodeCachedAccountWithCipher(tamperedJSON, cipher)
	require.Error(t, err)

	var proxyTampered map[string]any
	require.NoError(t, json.Unmarshal(full, &proxyTampered))
	proxyPayload := proxyTampered["Proxy"].(map[string]any)
	proxyPayload["Password"] = proxyPayload["Password"].(string) + "A"
	proxyTamperedJSON, err := json.Marshal(proxyTampered)
	require.NoError(t, err)
	_, err = decodeCachedAccountWithCipher(proxyTamperedJSON, cipher)
	require.Error(t, err)

	legacyPayload, err := json.Marshal(account)
	require.NoError(t, err)
	_, err = decodeCachedAccountWithCipher(legacyPayload, cipher)
	require.ErrorIs(t, err, errLegacySchedulerCredentialCache)
}

func TestCredentialCipherRejectsTamperWrongKeyAndWrongAAD(t *testing.T) {
	cipher := testCredentialCipher(t, "a", "primary")
	storage, err := cipher.Encrypt(7, "grok", map[string]any{"refresh_token": "secret"})
	require.NoError(t, err)

	tampered := make(map[string]any, len(storage))
	for key, value := range storage {
		tampered[key] = value
	}
	envelopeJSON, err := json.Marshal(tampered[credentialEnvelopeKey])
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(envelopeJSON, &envelope))
	envelope["ciphertext"] = envelope["ciphertext"].(string) + "A"
	tampered[credentialEnvelopeKey] = envelope
	_, _, err = cipher.Decrypt(7, "grok", tampered)
	require.Error(t, err)

	wrongKey := testCredentialCipher(t, "b", "primary")
	_, _, err = wrongKey.Decrypt(7, "grok", storage)
	require.Error(t, err)
	_, _, err = cipher.Decrypt(8, "grok", storage)
	require.Error(t, err)
	_, _, err = cipher.Decrypt(7, "openai", storage)
	require.Error(t, err)
}

func TestCredentialCipherLegacyDualReadAndPartialEnvelopeFailClosed(t *testing.T) {
	cipher := testCredentialCipher(t, "c", "primary")
	legacy := map[string]any{"api_key": "legacy-secret", "base_url": "https://example.test"}
	decoded, isLegacy, err := cipher.Decrypt(9, "openai", legacy)
	require.NoError(t, err)
	require.True(t, isLegacy)
	require.Equal(t, legacy, decoded)

	_, _, err = cipher.Decrypt(9, "openai", map[string]any{credentialFingerprintKey: "forged"})
	require.Error(t, err)
}

func TestCredentialCipherRejectsPlaintextSmuggledBesideEnvelope(t *testing.T) {
	cipher := testCredentialCipher(t, "c", "primary")
	storage, err := cipher.Encrypt(10, "openai", map[string]any{"access_token": "encrypted-secret"})
	require.NoError(t, err)

	storage["access_token"] = "plaintext-leak"
	_, _, err = cipher.Decrypt(10, "openai", storage)
	require.ErrorContains(t, err, "unexpected top-level field")

	delete(storage, "access_token")
	envelopeJSON, err := json.Marshal(storage[credentialEnvelopeKey])
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(envelopeJSON, &envelope))
	envelope["plaintext_leak"] = "secret"
	storage[credentialEnvelopeKey] = envelope
	_, _, err = cipher.Decrypt(10, "openai", storage)
	require.ErrorContains(t, err, "decode envelope")
}

func TestMigrateLegacyAccountCredentialsIsAtomicAndWritesOnlyEnvelope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cipher := testCredentialCipher(t, "d", "primary")
	legacy := `{"api_key":"legacy-secret","base_url":"https://api.example.test"}`
	softDeletedLegacy := `{"refresh_token":"soft-deleted-secret"}`

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(credentialMigrationAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT\s+id,\s*platform,\s*credentials\s+FROM\s+accounts\s+ORDER\s+BY\s+id\s+FOR\s+UPDATE`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "platform", "credentials"}).
			AddRow(int64(11), "openai", []byte(legacy)).
			AddRow(int64(12), "grok", []byte(softDeletedLegacy)),
	)
	mock.ExpectExec("UPDATE accounts").
		WithArgs(encryptedCredentialArgument{forbidden: "legacy-secret"}, int64(11), legacy).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE accounts").
		WithArgs(encryptedCredentialArgument{forbidden: "soft-deleted-secret"}, int64(12), softDeletedLegacy).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, migrateLegacyAccountCredentials(context.Background(), db, cipher))
	require.NoError(t, mock.ExpectationsWereMet())
}

type encryptedCredentialArgument struct {
	forbidden string
}

func (a encryptedCredentialArgument) Match(value driver.Value) bool {
	var encoded string
	switch typed := value.(type) {
	case string:
		encoded = typed
	case []byte:
		encoded = string(typed)
	default:
		return false
	}
	return strings.Contains(encoded, credentialEnvelopeKey) && !strings.Contains(encoded, a.forbidden)
}

func TestMigrateLegacyAccountCredentialsRollsBackWholeBatchOnInvalidRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cipher := testCredentialCipher(t, "e", "primary")
	legacy := `{"api_key":"legacy-secret"}`
	invalid := `{"` + credentialFingerprintKey + `":"forged"}`

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(credentialMigrationAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT\s+id,\s*platform,\s*credentials\s+FROM\s+accounts\s+ORDER\s+BY\s+id\s+FOR\s+UPDATE`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "platform", "credentials"}).
			AddRow(int64(11), "openai", []byte(legacy)).
			AddRow(int64(12), "grok", []byte(invalid)),
	)
	mock.ExpectRollback()

	err = migrateLegacyAccountCredentials(context.Background(), db, cipher)
	require.ErrorContains(t, err, "account 12")
	require.NoError(t, mock.ExpectationsWereMet())
}
