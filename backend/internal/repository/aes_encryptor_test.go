//go:build unit

package repository

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 测试辅助 ─────────────────────────────────────────────────────────────────

// aesHexKey 构造一个全填充为 b 的 n 字节密钥并以 hex 编码返回。
func aesHexKey(n int, b byte) string {
	raw := make([]byte, n)
	for i := range raw {
		raw[i] = b
	}
	return hex.EncodeToString(raw)
}

// aesTestCfg 用给定 hex 密钥字符串构造最小 Config。
func aesTestCfg(keyHex string) *config.Config {
	return &config.Config{
		Totp: config.TotpConfig{EncryptionKey: keyHex},
	}
}

// aesEncryptor 创建一个持有合法 32 字节密钥的加密器，测试失败时立即终止。
func aesEncryptor(t *testing.T) *AESEncryptor {
	t.Helper()
	enc, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0x42)))
	require.NoError(t, err)
	require.NotNil(t, enc)
	return enc.(*AESEncryptor)
}

// ── NewAESEncryptor ──────────────────────────────────────────────────────────

func TestNewAESEncryptor_ValidKey32Bytes(t *testing.T) {
	enc, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0x01)))
	require.NoError(t, err)
	require.NotNil(t, enc)
}

// 16 / 24 字节密钥在 AES 体系内合法，但本实现仅接受 AES-256（32 字节）。
func TestNewAESEncryptor_WrongKeyLength(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{"16_bytes_AES128", 16},
		{"24_bytes_AES192", 24},
		{"1_byte", 1},
		{"31_bytes", 31},
		{"33_bytes", 33},
		{"64_bytes", 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAESEncryptor(aesTestCfg(aesHexKey(tt.keySize, 0x00)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "32 bytes")
		})
	}
}

// "配置缺失"场景：空字符串与非法 hex 编码。
func TestNewAESEncryptor_MissingOrInvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		keyHex      string
		wantContain string
	}{
		{"empty_key", "", "32 bytes"},
		{"invalid_hex_odd_length", "abcde", "invalid totp encryption key"},
		{"invalid_hex_chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", "invalid totp encryption key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAESEncryptor(aesTestCfg(tt.keyHex))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantContain)
		})
	}
}

// ── 加解密往返（Roundtrip）───────────────────────────────────────────────────

func TestAESEncryptor_RoundTrip(t *testing.T) {
	enc := aesEncryptor(t)

	tests := []struct {
		name      string
		plaintext string
	}{
		{"ascii", "Hello, Sub2API!"},
		{"chinese_multibyte", "你好，世界！这是多字节 UTF-8 文本。"},
		{"empty_string", ""},
		{"long_string_gt_1KB", strings.Repeat("x", 2048)},
		{"special_chars", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := enc.Encrypt(tt.plaintext)
			require.NoError(t, err)
			require.NotEmpty(t, ct, "密文不应为空（即便明文为空字符串）")

			got, err := enc.Decrypt(ct)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, got)
		})
	}
}

// ── IV/Nonce 随机性 ──────────────────────────────────────────────────────────

func TestAESEncryptor_Encrypt_NonceRandomness(t *testing.T) {
	enc := aesEncryptor(t)
	const iterations = 30
	plaintext := "same plaintext for every iteration"

	seen := make(map[string]struct{}, iterations)
	for i := 0; i < iterations; i++ {
		ct, err := enc.Encrypt(plaintext)
		require.NoError(t, err)
		seen[ct] = struct{}{}
	}

	// 30 次加密相同明文，每次因随机 Nonce 应产生不同密文。
	assert.Len(t, seen, iterations,
		"每次加密应因随机 Nonce 产生唯一密文，共 %d 次", iterations)
}

// ── Decrypt 错误路径 ──────────────────────────────────────────────────────────

func TestAESDecrypt_InvalidBase64(t *testing.T) {
	enc := aesEncryptor(t)
	_, err := enc.Decrypt("!!!not-valid-base64!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode base64")
}

func TestAESDecrypt_TooShort(t *testing.T) {
	enc := aesEncryptor(t)
	// GCM Nonce 为 12 字节；仅提供 2 字节，必然短于 NonceSize。
	short := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02})
	_, err := enc.Decrypt(short)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestAESDecrypt_TamperedCiphertext(t *testing.T) {
	enc := aesEncryptor(t)

	ct, err := enc.Encrypt("sensitive payload")
	require.NoError(t, err)

	raw, err := base64.StdEncoding.DecodeString(ct)
	require.NoError(t, err)

	// Nonce 占前 12 字节；翻转其后第一个字节（密文体）。
	raw[12] ^= 0xFF
	_, err = enc.Decrypt(base64.StdEncoding.EncodeToString(raw))
	require.Error(t, err, "篡改密文体后解密应失败")
}

func TestAESDecrypt_TamperedTag(t *testing.T) {
	enc := aesEncryptor(t)

	ct, err := enc.Encrypt("sensitive payload")
	require.NoError(t, err)

	raw, err := base64.StdEncoding.DecodeString(ct)
	require.NoError(t, err)

	// GCM 认证标签占最后 16 字节；翻转最后一个字节。
	raw[len(raw)-1] ^= 0xFF
	_, err = enc.Decrypt(base64.StdEncoding.EncodeToString(raw))
	require.Error(t, err, "篡改 GCM 标签后解密应失败")
}

// ── 跨实例（Cross-instance）──────────────────────────────────────────────────

func TestAESEncryptor_CrossInstance_SameKey_CanDecrypt(t *testing.T) {
	keyHex := aesHexKey(32, 0xDE)

	enc1, err := NewAESEncryptor(aesTestCfg(keyHex))
	require.NoError(t, err)
	enc2, err := NewAESEncryptor(aesTestCfg(keyHex))
	require.NoError(t, err)

	plaintext := "cross-instance roundtrip"
	ct, err := enc1.Encrypt(plaintext)
	require.NoError(t, err)

	got, err := enc2.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got, "相同密钥构造的两个实例应可互相解密")
}

func TestAESEncryptor_CrossInstance_DifferentKey_CannotDecrypt(t *testing.T) {
	enc1, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0xAA)))
	require.NoError(t, err)
	enc2, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0xBB)))
	require.NoError(t, err)

	ct, err := enc1.Encrypt("secret message")
	require.NoError(t, err)

	_, err = enc2.Decrypt(ct)
	require.Error(t, err, "不同密钥的实例不应能解密对方的密文")
}
