package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AESEncryptor implements SecretEncryptor using AES-256-GCM
type AESEncryptor struct {
	key []byte
}

// disabledSecretEncryptor 是主密钥未配置时使用的实现。
//
// 它存在的理由：主密钥为空曾经会被自动生成成一把每次重启都变的随机密钥，写入看似成功，
// 重启后密文静默变成无法解开的垃圾。现在空密钥不再伪造随机密钥，于是必须有一个既不返回
// nil（调用方会 panic）、也不阻断进程启动（全新安装还用不到它）的实现。
// 任何真正想写入或读取密文的调用都会当场拿到一条点名变量的错误。
type disabledSecretEncryptor struct{}

func (disabledSecretEncryptor) err() error {
	return fmt.Errorf(
		"secret encryption is unavailable: no application secret encryption key is configured; "+
			"generate one with `openssl rand -hex 32` and set %s (legacy alias: %s), then restart",
		config.SecretEncryptionKeyEnvVar, config.LegacySecretEncryptionKeyEnvVar,
	)
}

func (e disabledSecretEncryptor) Encrypt(string) (string, error) { return "", e.err() }
func (e disabledSecretEncryptor) Decrypt(string) (string, error) { return "", e.err() }

// NewAESEncryptor creates the application-wide SecretEncryptor.
//
// 密钥来源是 cfg.Security.SecretEncryptionKey（load() 已把历史别名 totp.encryption_key
// 归一进去）。密钥为空时返回 disabledSecretEncryptor 而不是 error：这是一个 Wire
// provider，返回 error 会让全新安装在一把它还不需要的密钥上起不来。
// 「空密钥是否应该阻断启动」由 ensureSecretEncryptionKeyUsable 在能看到数据库、
// 因而能判断是否真有东西可丢的地方决定。
func NewAESEncryptor(cfg *config.Config) (service.SecretEncryptor, error) {
	keyHex := ""
	if cfg != nil {
		keyHex = strings.TrimSpace(cfg.Security.SecretEncryptionKey)
		if keyHex == "" {
			// 别名回退。load() 已经把两个键归一并互相镜像，所以这条分支只对
			// 手工构造 Config 的调用方（主要是测试）生效。
			keyHex = strings.TrimSpace(cfg.Totp.EncryptionKey)
		}
	}
	if keyHex == "" {
		return disabledSecretEncryptor{}, nil
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: must be hex-encoded: %w", config.SecretEncryptionKeyEnvVar, err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("%s must be 32 bytes (64 hex chars), got %d bytes", config.SecretEncryptionKeyEnvVar, len(key))
	}

	return &AESEncryptor{key: key}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM
// Output format: base64(nonce + ciphertext + tag)
func (e *AESEncryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// Encrypt the plaintext
	// Seal appends the ciphertext and tag to the nonce
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode as base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func (e *AESEncryptor) Decrypt(ciphertext string) (string, error) {
	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertextData := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
