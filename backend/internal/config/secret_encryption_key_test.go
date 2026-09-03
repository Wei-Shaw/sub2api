//go:build unit

package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCanonicalSecretKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testLegacySecretKey    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// resolveSecretEncryptionKey 是 H5 的核心：这把密钥不只加密 TOTP，而是全站唯一的
// 静态数据加密密钥。它以前在为空时每次进程启动生成一把新的随机密钥，于是从未配置
// 过密钥的运维会在每次重启后静默失去解密全部已存密文的能力。
func TestResolveSecretEncryptionKey(t *testing.T) {
	tests := []struct {
		name           string
		canonical      string
		legacy         string
		wantKey        string
		wantConfigured bool
	}{
		{
			name:           "both_empty_stays_empty",
			wantKey:        "",
			wantConfigured: false,
		},
		{
			name:           "canonical_only",
			canonical:      testCanonicalSecretKey,
			wantKey:        testCanonicalSecretKey,
			wantConfigured: true,
		},
		{
			name:           "legacy_alias_only",
			legacy:         testLegacySecretKey,
			wantKey:        testLegacySecretKey,
			wantConfigured: true,
		},
		{
			name:           "both_set_and_identical",
			canonical:      testCanonicalSecretKey,
			legacy:         testCanonicalSecretKey,
			wantKey:        testCanonicalSecretKey,
			wantConfigured: true,
		},
		{
			// 规范名必须胜出：否则运维无法通过设置新变量覆盖一个遗留的旧变量。
			name:           "conflict_canonical_wins",
			canonical:      testCanonicalSecretKey,
			legacy:         testLegacySecretKey,
			wantKey:        testCanonicalSecretKey,
			wantConfigured: true,
		},
		{
			name:           "whitespace_only_is_empty",
			canonical:      "   ",
			legacy:         "\t\n",
			wantKey:        "",
			wantConfigured: false,
		},
		{
			name:           "values_are_trimmed",
			canonical:      "  " + testCanonicalSecretKey + "\n",
			wantKey:        testCanonicalSecretKey,
			wantConfigured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{}
			cfg.Security.SecretEncryptionKey = tt.canonical
			cfg.Totp.EncryptionKey = tt.legacy

			resolveSecretEncryptionKey(cfg)

			assert.Equal(t, tt.wantKey, cfg.Security.SecretEncryptionKey)
			assert.Equal(t, tt.wantConfigured, cfg.Security.SecretEncryptionKeyConfigured)

			// 历史字段是生效值的镜像，约十处既有读取点依赖这一点。
			assert.Equal(t, tt.wantKey, cfg.Totp.EncryptionKey,
				"cfg.Totp.EncryptionKey must mirror the resolved key")
			assert.Equal(t, tt.wantConfigured, cfg.Totp.EncryptionKeyConfigured,
				"cfg.Totp.EncryptionKeyConfigured must mirror the resolved state")
		})
	}
}

// 空密钥必须保持为空。以前这里会 generateJWTSecret(32)，两次启动得到两把不同的
// 密钥，第一次写入的密文在第二次启动后再也解不开。
func TestResolveSecretEncryptionKeyNeverAutoGenerates(t *testing.T) {
	t.Parallel()

	first := &Config{}
	resolveSecretEncryptionKey(first)

	second := &Config{}
	resolveSecretEncryptionKey(second)

	require.Empty(t, first.Security.SecretEncryptionKey,
		"an empty key must stay empty, not become a fresh random key")
	assert.Equal(t, first.Security.SecretEncryptionKey, second.Security.SecretEncryptionKey,
		"two loads of the same config must resolve to the same key")
	assert.False(t, first.Security.SecretEncryptionKeyConfigured)
}

// 归一是幂等的：对同一个 Config 反复调用不得改变结果（load 路径上不会重复调用，
// 但镜像写回让这个不变式值得钉住——否则第二次调用会把镜像当成"旧键也设置了"）。
func TestResolveSecretEncryptionKeyIsIdempotent(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.Security.SecretEncryptionKey = testCanonicalSecretKey

	resolveSecretEncryptionKey(cfg)
	firstKey := cfg.Security.SecretEncryptionKey

	resolveSecretEncryptionKey(cfg)
	assert.Equal(t, firstKey, cfg.Security.SecretEncryptionKey)
	assert.Equal(t, firstKey, cfg.Totp.EncryptionKey)
	assert.True(t, cfg.Security.SecretEncryptionKeyConfigured)
}

// viper.Unmarshal 只解码 AllKeys() 中的键；AutomaticEnv 只能覆盖已在其中的键，
// 从不新增。少了 SetDefault，纯环境变量部署下 SECURITY_SECRET_ENCRYPTION_KEY 会被
// 读进来然后静默丢弃——image_storage 凭据就是这么丢的。
func TestSecretEncryptionKeysAreEnvReachable(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setDefaults()

	registered := map[string]struct{}{}
	for _, key := range viper.AllKeys() {
		registered[strings.ToLower(key)] = struct{}{}
	}

	for _, key := range []string{"security.secret_encryption_key", "totp.encryption_key"} {
		_, ok := registered[key]
		assert.True(t, ok, "%s must be registered so AutomaticEnv can reach it", key)
	}
}

// 环境变量拼写由 AutomaticEnv + SetEnvKeyReplacer(".", "_") 决定，且本仓库未设置
// SetEnvPrefix。这个测试把两个名字钉死，因为它们出现在部署文档、compose 文件和
// 启动错误信息里，改动会静默地让运维设的变量失效。
func TestSecretEncryptionKeyEnvVarSpellings(t *testing.T) {
	t.Parallel()

	replacer := strings.NewReplacer(".", "_")
	assert.Equal(t, SecretEncryptionKeyEnvVar,
		strings.ToUpper(replacer.Replace("security.secret_encryption_key")))
	assert.Equal(t, LegacySecretEncryptionKeyEnvVar,
		strings.ToUpper(replacer.Replace("totp.encryption_key")))
}
