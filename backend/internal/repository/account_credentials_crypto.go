package repository

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	credentialEnvelopeKey       = "__sub2api_credential_envelope"
	credentialFingerprintKey    = "__sub2api_credential_fingerprint"
	credentialAPIKeyIndexKey    = "__sub2api_api_key_index"
	credentialOllamaBaseURLKey  = "__sub2api_ollama_cloud_base_url"
	credentialHasRefreshKey     = "__sub2api_has_refresh_token"
	credentialEnvelopeVersion   = 1
	credentialEnvelopeAlgorithm = "AES-256-GCM"
	credentialCacheSecretPrefix = "sub2api-secret:v1:"
	oauthTokenCacheSecretScope  = "sub2api/redis/oauth-token"
	schedulerProxySecretScope   = "sub2api/redis/scheduler-proxy-password"

	credentialMigrationAdvisoryLockID int64 = 694208311321144028
)

var (
	errCredentialEnvelopeInvalid  = errors.New("account credential envelope is invalid")
	errCacheSecretEnvelopeInvalid = errors.New("cache secret envelope is invalid")
)

// CredentialCipher encrypts complete account credential documents. The only
// values left beside the envelope are authenticated, non-secret lookup
// projections needed by existing PostgreSQL scheduling queries.
type CredentialCipher struct {
	keyID              string
	aead               cipher.AEAD
	oauthTokenAEAD     cipher.AEAD
	schedulerProxyAEAD cipher.AEAD
	indexKey           [sha256.Size]byte
}

type credentialEnvelope struct {
	Version    int    `json:"version"`
	KeyID      string `json:"key_id"`
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type credentialAAD struct {
	Scope         string `json:"scope"`
	Version       int    `json:"version"`
	KeyID         string `json:"key_id"`
	Algorithm     string `json:"algorithm"`
	AccountID     int64  `json:"account_id"`
	Platform      string `json:"platform"`
	Fingerprint   string `json:"fingerprint"`
	APIKeyIndex   string `json:"api_key_index,omitempty"`
	OllamaBaseURL bool   `json:"ollama_cloud_base_url"`
	HasRefresh    bool   `json:"has_refresh_token"`
}

type cacheSecretAAD struct {
	Scope     string `json:"scope"`
	Version   int    `json:"version"`
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	Context   string `json:"context"`
}

// NewCredentialCipher constructs the production account-credential cipher.
// A nil cipher is returned only for non-release development configurations
// that intentionally omit credential_encryption.key.
func NewCredentialCipher(cfg *config.Config) (*CredentialCipher, error) {
	if cfg == nil {
		return nil, errors.New("credential encryption config is required")
	}
	keyHex := strings.TrimSpace(cfg.CredentialEncryption.Key)
	if keyHex == "" {
		if strings.EqualFold(strings.TrimSpace(cfg.Server.Mode), "release") {
			return nil, errors.New("credential_encryption.key is required in release mode")
		}
		return nil, nil
	}
	master, err := hex.DecodeString(keyHex)
	if err != nil || len(master) != 32 {
		return nil, errors.New("credential_encryption.key must be 64 hexadecimal characters (32 bytes)")
	}
	keyID := strings.TrimSpace(cfg.CredentialEncryption.KeyID)
	if keyID == "" {
		keyID = "primary"
	}
	aeadKey := deriveCredentialSubkey(master, "sub2api/account-credentials/aead/v1")
	indexKey := deriveCredentialSubkey(master, "sub2api/account-credentials/index/v1")
	oauthTokenKey := deriveCredentialSubkey(master, "sub2api/redis-oauth-token/aead/v1")
	schedulerProxyKey := deriveCredentialSubkey(master, "sub2api/redis-scheduler-proxy-password/aead/v1")
	aead, err := newCredentialAEAD(aeadKey)
	if err != nil {
		return nil, fmt.Errorf("create account credential cipher: %w", err)
	}
	oauthTokenAEAD, err := newCredentialAEAD(oauthTokenKey)
	if err != nil {
		return nil, fmt.Errorf("create OAuth token cache cipher: %w", err)
	}
	schedulerProxyAEAD, err := newCredentialAEAD(schedulerProxyKey)
	if err != nil {
		return nil, fmt.Errorf("create scheduler proxy cipher: %w", err)
	}
	return &CredentialCipher{
		keyID:              keyID,
		aead:               aead,
		oauthTokenAEAD:     oauthTokenAEAD,
		schedulerProxyAEAD: schedulerProxyAEAD,
		indexKey:           indexKey,
	}, nil
}

func newCredentialAEAD(key [sha256.Size]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func deriveCredentialSubkey(master []byte, label string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte(label))
	var out [sha256.Size]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func (c *CredentialCipher) EncryptOAuthTokenCache(cacheKey, token string) (string, error) {
	if c == nil || c.oauthTokenAEAD == nil {
		return token, nil
	}
	if strings.TrimSpace(cacheKey) == "" {
		return "", errors.New("OAuth token cache encryption requires a cache key")
	}
	return c.encryptCacheSecret(c.oauthTokenAEAD, oauthTokenCacheSecretScope, cacheKey, token)
}

func (c *CredentialCipher) DecryptOAuthTokenCache(cacheKey, storage string) (token string, legacy bool, err error) {
	if c == nil || c.oauthTokenAEAD == nil {
		return storage, true, nil
	}
	if strings.TrimSpace(cacheKey) == "" {
		return "", false, errors.New("OAuth token cache decryption requires a cache key")
	}
	return c.decryptCacheSecret(c.oauthTokenAEAD, oauthTokenCacheSecretScope, cacheKey, storage)
}

func (c *CredentialCipher) EncryptSchedulerProxyPassword(accountID int64, platform string, proxyID int64, password string) (string, error) {
	if c == nil || c.schedulerProxyAEAD == nil {
		return password, nil
	}
	contextValue, err := schedulerProxySecretContext(accountID, platform, proxyID)
	if err != nil {
		return "", err
	}
	return c.encryptCacheSecret(c.schedulerProxyAEAD, schedulerProxySecretScope, contextValue, password)
}

func (c *CredentialCipher) DecryptSchedulerProxyPassword(accountID int64, platform string, proxyID int64, storage string) (password string, legacy bool, err error) {
	if c == nil || c.schedulerProxyAEAD == nil {
		return storage, true, nil
	}
	contextValue, err := schedulerProxySecretContext(accountID, platform, proxyID)
	if err != nil {
		return "", false, err
	}
	return c.decryptCacheSecret(c.schedulerProxyAEAD, schedulerProxySecretScope, contextValue, storage)
}

func schedulerProxySecretContext(accountID int64, platform string, proxyID int64) (string, error) {
	if accountID <= 0 || proxyID <= 0 || strings.TrimSpace(platform) == "" {
		return "", errors.New("scheduler proxy encryption requires positive account/proxy IDs and platform")
	}
	encoded, err := json.Marshal(struct {
		AccountID int64  `json:"account_id"`
		Platform  string `json:"platform"`
		ProxyID   int64  `json:"proxy_id"`
	}{AccountID: accountID, Platform: platform, ProxyID: proxyID})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (c *CredentialCipher) encryptCacheSecret(aead cipher.AEAD, scope, contextValue, plaintext string) (string, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate cache secret nonce: %w", err)
	}
	aad, err := c.marshalCacheSecretAAD(scope, contextValue)
	if err != nil {
		return "", err
	}
	envelope := credentialEnvelope{
		Version:    credentialEnvelopeVersion,
		KeyID:      c.keyID,
		Algorithm:  credentialEnvelopeAlgorithm,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(aead.Seal(nil, nonce, []byte(plaintext), aad)),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode cache secret envelope: %w", err)
	}
	return credentialCacheSecretPrefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func (c *CredentialCipher) decryptCacheSecret(aead cipher.AEAD, scope, contextValue, storage string) (plaintext string, legacy bool, err error) {
	if !strings.HasPrefix(storage, credentialCacheSecretPrefix) {
		return storage, true, nil
	}
	encoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(storage, credentialCacheSecretPrefix))
	if err != nil {
		return "", false, fmt.Errorf("%w: invalid encoding", errCacheSecretEnvelopeInvalid)
	}
	var envelope credentialEnvelope
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return "", false, fmt.Errorf("%w: decode envelope", errCacheSecretEnvelopeInvalid)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", false, fmt.Errorf("%w: trailing envelope data", errCacheSecretEnvelopeInvalid)
	}
	canonicalEnvelope, err := json.Marshal(envelope)
	if err != nil || !bytes.Equal(encoded, canonicalEnvelope) {
		return "", false, fmt.Errorf("%w: non-canonical envelope", errCacheSecretEnvelopeInvalid)
	}
	if envelope.Version != credentialEnvelopeVersion || envelope.KeyID != c.keyID || envelope.Algorithm != credentialEnvelopeAlgorithm {
		return "", false, fmt.Errorf("%w: unsupported version, key ID, or algorithm", errCacheSecretEnvelopeInvalid)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != aead.NonceSize() {
		return "", false, fmt.Errorf("%w: invalid nonce", errCacheSecretEnvelopeInvalid)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < aead.Overhead() {
		return "", false, fmt.Errorf("%w: invalid ciphertext", errCacheSecretEnvelopeInvalid)
	}
	aad, err := c.marshalCacheSecretAAD(scope, contextValue)
	if err != nil {
		return "", false, err
	}
	opened, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", false, fmt.Errorf("%w: authentication failed", errCacheSecretEnvelopeInvalid)
	}
	return string(opened), false, nil
}

func (c *CredentialCipher) marshalCacheSecretAAD(scope, contextValue string) ([]byte, error) {
	return json.Marshal(cacheSecretAAD{
		Scope:     scope,
		Version:   credentialEnvelopeVersion,
		KeyID:     c.keyID,
		Algorithm: credentialEnvelopeAlgorithm,
		Context:   contextValue,
	})
}

// Encrypt returns a JSONB-safe envelope. Credentials themselves never appear
// outside the AEAD ciphertext.
func (c *CredentialCipher) Encrypt(accountID int64, platform string, credentials map[string]any) (map[string]any, error) {
	if c == nil || c.aead == nil {
		return cloneCredentialMap(credentials)
	}
	if accountID <= 0 || strings.TrimSpace(platform) == "" {
		return nil, errors.New("account credential encryption requires a positive account ID and platform")
	}
	plaintext, err := canonicalCredentialJSON(credentials)
	if err != nil {
		return nil, err
	}
	fingerprint := c.fingerprintBytes(plaintext)
	apiKeyIndex := ""
	if apiKey, ok := credentials["api_key"].(string); ok && apiKey != "" {
		apiKeyIndex = c.APIKeyIndex(apiKey)
	}
	baseURL, _ := credentials["base_url"].(string)
	ollamaBaseURL := isOllamaCloudBaseURL(baseURL)
	hasRefresh := false
	if refresh, ok := credentials["refresh_token"].(string); ok && strings.TrimSpace(refresh) != "" {
		hasRefresh = true
	}
	aad, err := c.marshalAAD(accountID, platform, fingerprint, apiKeyIndex, ollamaBaseURL, hasRefresh)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate account credential nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, plaintext, aad)
	storage := map[string]any{
		credentialEnvelopeKey: credentialEnvelope{
			Version:    credentialEnvelopeVersion,
			KeyID:      c.keyID,
			Algorithm:  credentialEnvelopeAlgorithm,
			Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
			Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		},
		credentialFingerprintKey:   fingerprint,
		credentialHasRefreshKey:    hasRefresh,
		credentialOllamaBaseURLKey: ollamaBaseURL,
	}
	if apiKeyIndex != "" {
		storage[credentialAPIKeyIndexKey] = apiKeyIndex
	}
	return storage, nil
}

// Decrypt supports legacy plaintext maps during the startup migration window.
// Once an envelope marker is present, all failures are fatal; malformed or
// partial metadata is never reinterpreted as plaintext.
func (c *CredentialCipher) Decrypt(accountID int64, platform string, storage map[string]any) (credentials map[string]any, legacy bool, err error) {
	if c == nil || c.aead == nil {
		cloned, cloneErr := cloneCredentialMap(storage)
		return cloned, true, cloneErr
	}
	rawEnvelope, hasEnvelope := storage[credentialEnvelopeKey]
	if !hasEnvelope {
		for _, reserved := range []string{
			credentialFingerprintKey,
			credentialAPIKeyIndexKey,
			credentialOllamaBaseURLKey,
			credentialHasRefreshKey,
		} {
			if _, exists := storage[reserved]; exists {
				return nil, false, fmt.Errorf("%w: partial reserved metadata", errCredentialEnvelopeInvalid)
			}
		}
		cloned, cloneErr := cloneCredentialMap(storage)
		return cloned, true, cloneErr
	}
	allowedStorageKeys := map[string]struct{}{
		credentialEnvelopeKey:      {},
		credentialFingerprintKey:   {},
		credentialAPIKeyIndexKey:   {},
		credentialOllamaBaseURLKey: {},
		credentialHasRefreshKey:    {},
	}
	for key := range storage {
		if _, allowed := allowedStorageKeys[key]; !allowed {
			return nil, false, fmt.Errorf("%w: unexpected top-level field %q", errCredentialEnvelopeInvalid, key)
		}
	}
	if accountID <= 0 || strings.TrimSpace(platform) == "" {
		return nil, false, fmt.Errorf("%w: missing record context", errCredentialEnvelopeInvalid)
	}
	envelopeJSON, err := json.Marshal(rawEnvelope)
	if err != nil {
		return nil, false, fmt.Errorf("%w: encode envelope", errCredentialEnvelopeInvalid)
	}
	var envelope credentialEnvelope
	decoder := json.NewDecoder(bytes.NewReader(envelopeJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, false, fmt.Errorf("%w: decode envelope", errCredentialEnvelopeInvalid)
	}
	if envelope.Version != credentialEnvelopeVersion || envelope.KeyID != c.keyID || envelope.Algorithm != credentialEnvelopeAlgorithm {
		return nil, false, fmt.Errorf("%w: unsupported version, key ID, or algorithm", errCredentialEnvelopeInvalid)
	}
	fingerprint, ok := storage[credentialFingerprintKey].(string)
	if !ok || !validCredentialHMAC(fingerprint) {
		return nil, false, fmt.Errorf("%w: missing fingerprint", errCredentialEnvelopeInvalid)
	}
	apiKeyIndex := ""
	if rawAPIKeyIndex, exists := storage[credentialAPIKeyIndexKey]; exists {
		var valid bool
		apiKeyIndex, valid = rawAPIKeyIndex.(string)
		if !valid || !validCredentialHMAC(apiKeyIndex) {
			return nil, false, fmt.Errorf("%w: invalid API-key index", errCredentialEnvelopeInvalid)
		}
	}
	ollamaBaseURL, ok := storage[credentialOllamaBaseURLKey].(bool)
	if !ok {
		return nil, false, fmt.Errorf("%w: invalid Ollama base URL projection", errCredentialEnvelopeInvalid)
	}
	hasRefresh, ok := storage[credentialHasRefreshKey].(bool)
	if !ok {
		return nil, false, fmt.Errorf("%w: invalid refresh-token projection", errCredentialEnvelopeInvalid)
	}
	aad, err := c.marshalAAD(accountID, platform, fingerprint, apiKeyIndex, ollamaBaseURL, hasRefresh)
	if err != nil {
		return nil, false, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return nil, false, fmt.Errorf("%w: invalid nonce", errCredentialEnvelopeInvalid)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < c.aead.Overhead() {
		return nil, false, fmt.Errorf("%w: invalid ciphertext", errCredentialEnvelopeInvalid)
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, false, fmt.Errorf("%w: authentication failed", errCredentialEnvelopeInvalid)
	}
	var decoded map[string]any
	if err := json.Unmarshal(plaintext, &decoded); err != nil || decoded == nil {
		return nil, false, fmt.Errorf("%w: decrypted document is not an object", errCredentialEnvelopeInvalid)
	}
	if !hmac.Equal([]byte(fingerprint), []byte(c.fingerprintBytes(plaintext))) {
		return nil, false, fmt.Errorf("%w: fingerprint mismatch", errCredentialEnvelopeInvalid)
	}
	return decoded, false, nil
}

func (c *CredentialCipher) Fingerprint(credentials map[string]any) (string, error) {
	if c == nil {
		return "", nil
	}
	plaintext, err := canonicalCredentialJSON(credentials)
	if err != nil {
		return "", err
	}
	return c.fingerprintBytes(plaintext), nil
}

func (c *CredentialCipher) fingerprintBytes(plaintext []byte) string {
	mac := hmac.New(sha256.New, c.indexKey[:])
	_, _ = mac.Write([]byte("credential-document\x00"))
	_, _ = mac.Write(plaintext)
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *CredentialCipher) APIKeyIndex(apiKey string) string {
	if c == nil {
		return apiKey
	}
	mac := hmac.New(sha256.New, c.indexKey[:])
	_, _ = mac.Write([]byte("api-key\x00"))
	_, _ = mac.Write([]byte(apiKey))
	return hex.EncodeToString(mac.Sum(nil))
}

func validCredentialHMAC(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (c *CredentialCipher) marshalAAD(accountID int64, platform, fingerprint, apiKeyIndex string, ollamaBaseURL, hasRefresh bool) ([]byte, error) {
	return json.Marshal(credentialAAD{
		Scope:         "sub2api/accounts/credentials",
		Version:       credentialEnvelopeVersion,
		KeyID:         c.keyID,
		Algorithm:     credentialEnvelopeAlgorithm,
		AccountID:     accountID,
		Platform:      platform,
		Fingerprint:   fingerprint,
		APIKeyIndex:   apiKeyIndex,
		OllamaBaseURL: ollamaBaseURL,
		HasRefresh:    hasRefresh,
	})
}

func isOllamaCloudBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "?#") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "ollama.com" && host != "www.ollama.com" {
		return false
	}
	authority := strings.ToLower(parsed.Host)
	if authority != host && authority != host+":443" {
		return false
	}
	if parsed.RawPath != "" {
		return false
	}
	return parsed.Path == "" || parsed.Path == "/v1"
}

func canonicalCredentialJSON(credentials map[string]any) ([]byte, error) {
	if credentials == nil {
		credentials = map[string]any{}
	}
	encoded, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("encode account credentials: %w", err)
	}
	return encoded, nil
}

func cloneCredentialMap(credentials map[string]any) (map[string]any, error) {
	encoded, err := canonicalCredentialJSON(credentials)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("clone account credentials: %w", err)
	}
	return cloned, nil
}

func hasCredentialEnvelope(storage map[string]any) bool {
	if storage == nil {
		return false
	}
	_, ok := storage[credentialEnvelopeKey]
	return ok
}

// migrateLegacyAccountCredentials atomically validates every existing
// envelope and encrypts every legacy plaintext row before the server starts.
// The transaction-scoped advisory lock serializes concurrent replica starts.
func migrateLegacyAccountCredentials(ctx context.Context, db *sql.DB, credentialCipher *CredentialCipher) error {
	if credentialCipher == nil || db == nil {
		return nil
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin account credential migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", credentialMigrationAdvisoryLockID); err != nil {
		return fmt.Errorf("lock account credential migration: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, platform, credentials
		FROM accounts
		ORDER BY id
		FOR UPDATE
	`)
	if err != nil {
		return fmt.Errorf("scan account credentials for migration: %w", err)
	}
	type row struct {
		id       int64
		platform string
		storage  map[string]any
		raw      []byte
	}
	var pending []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.platform, &item.raw); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan account credential row: %w", err)
		}
		if err := json.Unmarshal(item.raw, &item.storage); err != nil || item.storage == nil {
			_ = rows.Close()
			return fmt.Errorf("account %d has invalid credential JSON", item.id)
		}
		_, legacy, err := credentialCipher.Decrypt(item.id, item.platform, item.storage)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("validate encrypted credentials for account %d: %w", item.id, err)
		}
		if legacy {
			pending = append(pending, item)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate account credential rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close account credential rows: %w", err)
	}
	for _, item := range pending {
		storage, err := credentialCipher.Encrypt(item.id, item.platform, item.storage)
		if err != nil {
			return fmt.Errorf("encrypt legacy credentials for account %d: %w", item.id, err)
		}
		encoded, err := json.Marshal(storage)
		if err != nil {
			return fmt.Errorf("encode encrypted credentials for account %d: %w", item.id, err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET credentials = $1::jsonb
			WHERE id = $2 AND credentials = $3::jsonb
		`, string(encoded), item.id, string(item.raw))
		if err != nil {
			return fmt.Errorf("migrate credentials for account %d: %w", item.id, err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return fmt.Errorf("migrate credentials for account %d: concurrent change detected", item.id)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account credential migration: %w", err)
	}
	return nil
}
