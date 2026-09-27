//go:build unit

package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUnreachableDB 返回一个语法合法但从不连接的 *sql.DB。
// database/sql 是惰性的：Open 不会拨号，所以任何真正碰库的代码路径都会在这里
// 失败得很明显，而在库之前就返回的路径可以被干净地测出来。
func newUnreachableDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", "host=127.0.0.1 port=1 user=x password=x dbname=x sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func secretKeyCfg(keyHex string) *config.Config {
	cfg := &config.Config{}
	cfg.Security.SecretEncryptionKey = keyHex
	return cfg
}

// ── 明文 / 密文判别 ──────────────────────────────────────────────────────────

// 这是回填的核心判定。它必须是判定而不是猜测：明文是 json.Marshal(map[string]string)
// 的输出，密文是三段冒号分隔的 base64，两者在结构上互斥。
func TestIsPlaintextPaymentProviderConfig(t *testing.T) {
	t.Parallel()

	key := make([]byte, payment.AES256KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	ciphertext, err := payment.Encrypt(`{"secretKey":"sk_live_x","webhookSecret":"whsec_y"}`, key)
	require.NoError(t, err)

	tests := []struct {
		name          string
		stored        string
		wantPlaintext bool
	}{
		{"empty_string", "", false},
		{"json_null", "null", false},
		{"real_ciphertext", ciphertext, false},
		{"garbage", "not json and not ciphertext", false},
		{"json_array", `["a","b"]`, false},
		{"json_number", "42", false},
		{"empty_object", "{}", true},
		{"stripe_secrets_in_the_clear", `{"secretKey":"sk_live_x","webhookSecret":"whsec_y"}`, true},
		{"wxpay_secrets_in_the_clear", `{"apiV3Key":"k","privateKey":"-----BEGIN-----"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, got := isPlaintextPaymentProviderConfig(tt.stored)
			assert.Equal(t, tt.wantPlaintext, got)
		})
	}
}

// 密文永远不可能被误判为明文，因此回填绝不会二次加密。
// 随机 nonce 让这条性质值得多跑几轮而不是只试一个样本。
func TestCiphertextIsNeverMistakenForPlaintext(t *testing.T) {
	t.Parallel()

	key := make([]byte, payment.AES256KeySize)
	for i := range key {
		key[i] = 0x5A
	}
	plaintext := `{"pid":"1001","pkey":"secret-pkey"}`

	for i := 0; i < 50; i++ {
		ciphertext, err := payment.Encrypt(plaintext, key)
		require.NoError(t, err)
		_, isPlain := isPlaintextPaymentProviderConfig(ciphertext)
		require.False(t, isPlain, "ciphertext must never be re-encrypted (iteration %d)", i)
	}
}

// 判别为明文的行必须能被原样解析出来，回填才可能重加密出等价内容。
func TestPlaintextDetectionRoundTrip(t *testing.T) {
	t.Parallel()

	original := map[string]string{"apiKey": "ak_live", "webhookSecret": "whsec"}
	raw, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, ok := isPlaintextPaymentProviderConfig(string(raw))
	require.True(t, ok)
	assert.Equal(t, original, parsed)
}

// ── 前置检查 ─────────────────────────────────────────────────────────────────

// 密钥已配置时只做支付配置解密抽查，查询失败按未命中处理，不得阻断启动。
func TestEnsureSecretEncryptionKeyUsableSkipsProbesWhenKeyConfigured(t *testing.T) {
	t.Parallel()

	err := ensureSecretEncryptionKeyUsable(context.Background(), newUnreachableDB(t),
		secretKeyCfg(strings.Repeat("ab", 32)))
	require.NoError(t, err)
}

// 空密钥但库里已有支付服务商配置密文：必须阻断启动，否则这些配置会被静默当成空。
func TestEnsureSecretEncryptionKeyUsableBlocksEmptyKeyWithPaymentCiphertext(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	// 其它探测没有匹配的期望，会返回错误并按未命中处理。
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM payment_provider_instances WHERE`)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = ensureSecretEncryptionKeyUsable(context.Background(), db, secretKeyCfg(""))
	require.ErrorContains(t, err, "payment_provider_instances.config")
	require.NoError(t, mock.ExpectationsWereMet())
}

// 主密钥解不开已存的支付服务商配置时（密钥丢失或被换），必须拒绝启动；
// 设置应急开关后只记 ERROR 并放行。
func TestEnsureSecretEncryptionKeyUsableChecksPaymentConfigDecryption(t *testing.T) {
	keyHex := strings.Repeat("ab", 32)
	key, err := hex.DecodeString(keyHex)
	require.NoError(t, err)
	encryptedWith := func(k []byte) string {
		ciphertext, err := payment.Encrypt(`{"apiKey":"sk_live"}`, k)
		require.NoError(t, err)
		return ciphertext
	}
	otherKey := bytes.Repeat([]byte{0x11}, payment.AES256KeySize)

	tests := []struct {
		name     string
		rows     *sqlmock.Rows
		override string
		wantErr  bool
	}{
		{"decryptable", sqlmock.NewRows([]string{"config"}).AddRow(encryptedWith(key)), "", false},
		{"no_ciphertext_rows", sqlmock.NewRows([]string{"config"}), "", false},
		{"wrong_key_refuses_start", sqlmock.NewRows([]string{"config"}).AddRow(encryptedWith(otherKey)), "", true},
		{"wrong_key_with_override", sqlmock.NewRows([]string{"config"}).AddRow(encryptedWith(otherKey)), "true", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(allowUndecryptablePaymentConfigEnvVar, tt.override)
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			mock.ExpectQuery(regexp.QuoteMeta(
				`SELECT config FROM payment_provider_instances WHERE config <> '' AND config <> 'null' AND config NOT LIKE '{%' ORDER BY id LIMIT 1`,
			)).WillReturnRows(tt.rows)

			err = ensureSecretEncryptionKeyUsable(context.Background(), db, secretKeyCfg(keyHex))
			if tt.wantErr {
				require.ErrorContains(t, err, "payment_provider_instances.config")
				require.ErrorContains(t, err, allowUndecryptablePaymentConfigEnvVar)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestEnsureSecretEncryptionKeyUsableRejectsNilConfig(t *testing.T) {
	t.Parallel()

	err := ensureSecretEncryptionKeyUsable(context.Background(), nil, nil)
	require.Error(t, err)
}

// 空密钥 + 无数据库连接：所有探测都失败，按未命中处理并放行。
// 探测失败不得变成一种新的启动失败模式。
func TestEnsureSecretEncryptionKeyUsableTreatsProbeFailuresAsMisses(t *testing.T) {
	t.Parallel()

	err := ensureSecretEncryptionKeyUsable(context.Background(), newUnreachableDB(t), secretKeyCfg(""))
	require.NoError(t, err, "unreachable probes must not block startup")
}

// 每条探测都必须真正对应一个 SecretEncryptor 调用点，并且在错误信息里可读。
func TestSecretEncryptionProbesCoverEveryKnownConsumer(t *testing.T) {
	t.Parallel()

	joined := ""
	for _, probe := range secretEncryptionProbes {
		assert.NotEmpty(t, probe.name, "every probe needs a name that can appear in the startup error")
		assert.Contains(t, probe.query, "SELECT EXISTS",
			"probes must be cheap existence checks, not scans")
		joined += probe.name + "\n" + probe.query + "\n"
	}

	for _, needle := range []string{
		"backup_s3_config",             // backup_service.go
		"image_storage_config",         // image_storage_settings.go
		"ollama_cloud_usage_settings",  // ollama_cloud_usage.go
		"prompt_audit_config",          // securityaudit/prompt_config_store.go
		"channel_monitor_enabled",      // channel_monitor_service.go
		"plugin_management_enabled",    // plugin_manager.go
		"totp_enabled",                 // totp_service.go
		"channel_monitors",             // stored channel monitor ciphertext
		"sub2api_plugin_installations", // stored plugin ciphertext
		"users",                        // users with 2FA enabled
		"payment_provider_instances",   // payment_config_providers.go
	} {
		assert.Contains(t, joined, needle, "no probe covers %q", needle)
	}
}

// ── 回填 ─────────────────────────────────────────────────────────────────────

// 密钥为空时回填必须跳过，且不得阻断启动——「空密钥是否阻断启动」是前置检查的
// 职责，回填不为同一件事发明第二种失败方式。传一个从不连接的 db 证明它没碰库，
// 因而也不可能写下完成标记。
func TestReencryptPaymentProviderConfigsSkipsWithoutKey(t *testing.T) {
	t.Parallel()

	err := reencryptPaymentProviderConfigs(context.Background(), newUnreachableDB(t), secretKeyCfg(""))
	require.NoError(t, err)
}

// 密钥格式非法时同样静默跳过：NewAESEncryptor 已经会因此让启动失败，
// 这里再报一次只会让同一个配置错误产生两条互相矛盾的信息。
func TestReencryptPaymentProviderConfigsSkipsInvalidKey(t *testing.T) {
	t.Parallel()

	for _, keyHex := range []string{"zz", strings.Repeat("ab", 16)} {
		err := reencryptPaymentProviderConfigs(context.Background(), newUnreachableDB(t), secretKeyCfg(keyHex))
		require.NoError(t, err, "invalid key %q must not add a second failure mode", keyHex)
	}
}

func TestReencryptPaymentProviderConfigsRejectsNilArguments(t *testing.T) {
	t.Parallel()

	require.Error(t, reencryptPaymentProviderConfigs(context.Background(), nil, secretKeyCfg("")))
	require.Error(t, reencryptPaymentProviderConfigs(context.Background(), newUnreachableDB(t), nil))
}

// 回填必须用自己的 advisory lock ID：复用迁移运行器的 ID 会把回填和数据库迁移串在
// 同一把锁上，一个副本做回填时另一个副本连迁移都启动不了。
func TestBackfillLockIDIsDistinctFromMigrations(t *testing.T) {
	t.Parallel()

	assert.NotEqual(t, migrationsAdvisoryLockID, secretEncryptionBackfillLockID,
		"reusing the migrations lock would serialize the backfill against unrelated migrations")
}

// 完成标记的键名带版本后缀，且不会和迁移记录表混用。
func TestBackfillMarkerKeyIsVersioned(t *testing.T) {
	t.Parallel()

	assert.True(t, strings.HasSuffix(paymentConfigBackfillMarkerKey, "_v1"),
		"a future backfill with different semantics must use a new key, not reuse this one")
}
