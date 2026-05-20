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
REDACTED
	return hex.EncodeToString(raw)
REDACTED

// aesTestCfg 用给定 hex 密钥字符串构造最小 Config。
func aesTestCfg(keyHex string) *config.Config {
	return &config.Config{
		Totp: config.TotpConfig{EncryptionKey: keyHexREDACTED,
REDACTED
REDACTED

// aesEncryptor 创建一个持有合法 32 字节密钥的加密器，测试失败时立即终止。
func aesEncryptor(t *testing.T) *AESEncryptor {
REDACTED
	enc, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0x42)))
REDACTED
	require.NotNil(t, enc)
	return enc.(*AESEncryptor)
REDACTED

// ── NewAESEncryptor ──────────────────────────────────────────────────────────

func TestNewAESEncryptor_ValidKey32Bytes(t *testing.T) {
	enc, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0x01)))
REDACTED
	require.NotNil(t, enc)
REDACTED

// 16 / 24 字节密钥在 AES 体系内合法，但本实现仅接受 AES-256（32 字节）。
func TestNewAESEncryptor_WrongKeyLength(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
REDACTED{
		{"16_bytes_AES128", 16REDACTED,
		{"24_bytes_AES192", 24REDACTED,
		{"1_byte", 1REDACTED,
		{"31_bytes", 31REDACTED,
		{"33_bytes", 33REDACTED,
		{"64_bytes", 64REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAESEncryptor(aesTestCfg(aesHexKey(tt.keySize, 0x00)))
		REDACTED
			assert.Contains(t, err.Error(), "32 bytes")
	REDACTED)
REDACTED
REDACTED

// "配置缺失"场景：空字符串与非法 hex 编码。
func TestNewAESEncryptor_MissingOrInvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		keyHex      string
		wantContain string
REDACTED{
		{"empty_key", "", "32 bytes"REDACTED,
		{"invalid_hex_odd_length", "abcde", "invalid totp encryption key"REDACTED,
		{"invalid_hex_chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", "invalid totp encryption key"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAESEncryptor(aesTestCfg(tt.keyHex))
		REDACTED
			assert.Contains(t, err.Error(), tt.wantContain)
	REDACTED)
REDACTED
REDACTED

// ── 加解密往返（Roundtrip）───────────────────────────────────────────────────

func TestAESEncryptor_RoundTrip(t *testing.T) {
	enc := aesEncryptor(t)

	tests := []struct {
		name      string
		plaintext string
REDACTED{
		{"ascii", "Hello, Sub2API!"REDACTED,
		{"chinese_multibyte", "你好，世界！这是多字节 UTF-8 文本。"REDACTED,
		{"empty_string", ""REDACTED,
		{"long_string_gt_1KB", strings.Repeat("x", 2048)REDACTED,
		{"special_chars", "!@#$%^&*()_+-=[]{REDACTED|;':\",./<>?"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := enc.Encrypt(tt.plaintext)
		REDACTED
			require.NotEmpty(t, ct, "密文不应为空（即便明文为空字符串）")

			got, err := enc.Decrypt(ct)
		REDACTED
			assert.Equal(t, tt.plaintext, got)
	REDACTED)
REDACTED
REDACTED

// ── IV/Nonce 随机性 ──────────────────────────────────────────────────────────

func TestAESEncryptor_Encrypt_NonceRandomness(t *testing.T) {
	enc := aesEncryptor(t)
	const iterations = 30
	plaintext := "same plaintext for every iteration"

	seen := make(map[string]struct{REDACTED, iterations)
	for i := 0; i < iterations; i++ {
		ct, err := enc.Encrypt(plaintext)
	REDACTED
		seen[ct] = struct{REDACTED{REDACTED
REDACTED

	// 30 次加密相同明文，每次因随机 Nonce 应产生不同密文。
	assert.Len(t, seen, iterations,
		"每次加密应因随机 Nonce 产生唯一密文，共 %d 次", iterations)
REDACTED

// ── Decrypt 错误路径 ──────────────────────────────────────────────────────────

func TestAESDecrypt_InvalidBase64(t *testing.T) {
	enc := aesEncryptor(t)
	_, err := enc.Decrypt("!!!not-valid-base64!!!")
REDACTED
	assert.Contains(t, err.Error(), "decode base64")
REDACTED

func TestAESDecrypt_TooShort(t *testing.T) {
	enc := aesEncryptor(t)
	// GCM Nonce 为 12 字节；仅提供 2 字节，必然短于 NonceSize。
	short := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02REDACTED)
	_, err := enc.Decrypt(short)
REDACTED
	assert.Contains(t, err.Error(), "too short")
REDACTED

func TestAESDecrypt_TamperedCiphertext(t *testing.T) {
	enc := aesEncryptor(t)

	ct, err := enc.Encrypt("sensitive payload")
REDACTED

	raw, err := base64.StdEncoding.DecodeString(ct)
REDACTED

	// Nonce 占前 12 字节；翻转其后第一个字节（密文体）。
	raw[12] ^= 0xFF
	_, err = enc.Decrypt(base64.StdEncoding.EncodeToString(raw))
	require.Error(t, err, "篡改密文体后解密应失败")
REDACTED

func TestAESDecrypt_TamperedTag(t *testing.T) {
	enc := aesEncryptor(t)

	ct, err := enc.Encrypt("sensitive payload")
REDACTED

	raw, err := base64.StdEncoding.DecodeString(ct)
REDACTED

	// GCM 认证标签占最后 16 字节；翻转最后一个字节。
	raw[len(raw)-1] ^= 0xFF
	_, err = enc.Decrypt(base64.StdEncoding.EncodeToString(raw))
	require.Error(t, err, "篡改 GCM 标签后解密应失败")
REDACTED

// ── 跨实例（Cross-instance）──────────────────────────────────────────────────

func TestAESEncryptor_CrossInstance_SameKey_CanDecrypt(t *testing.T) {
	keyHex := aesHexKey(32, 0xDE)

	enc1, err := NewAESEncryptor(aesTestCfg(keyHex))
REDACTED
	enc2, err := NewAESEncryptor(aesTestCfg(keyHex))
REDACTED

	plaintext := "cross-instance roundtrip"
	ct, err := enc1.Encrypt(plaintext)
REDACTED

	got, err := enc2.Decrypt(ct)
REDACTED
	assert.Equal(t, plaintext, got, "相同密钥构造的两个实例应可互相解密")
REDACTED

func TestAESEncryptor_CrossInstance_DifferentKey_CannotDecrypt(t *testing.T) {
	enc1, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0xAA)))
REDACTED
	enc2, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0xBB)))
REDACTED

	ct, err := enc1.Encrypt("secret message")
REDACTED

	_, err = enc2.Decrypt(ct)
	require.Error(t, err, "不同密钥的实例不应能解密对方的密文")
REDACTED
