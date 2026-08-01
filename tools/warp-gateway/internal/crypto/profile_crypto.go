package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const encPrefix = "enc:v1:"

// ProfileCipher encrypts WARP private keys at rest (AES-256-GCM).
type ProfileCipher struct {
	gcm cipher.AEAD
}

// NewProfileCipher derives a 32-byte key from secret material (token or explicit key).
func NewProfileCipher(secret string) (*ProfileCipher, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("profile encryption secret is empty")
	}
	sum := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ProfileCipher{gcm: gcm}, nil
}

// EncryptString encrypts plaintext; returns enc:v1:<base64> ciphertext.
func (c *ProfileCipher) EncryptString(plain string) (string, error) {
	if c == nil || plain == "" {
		return plain, nil
	}
	if strings.HasPrefix(plain, encPrefix) {
		return plain, nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := c.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(out), nil
}

// DecryptString decrypts enc:v1: payloads; passes through plaintext.
func (c *ProfileCipher) DecryptString(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil
	}
	if c == nil {
		return "", fmt.Errorf("encrypted profile key present but cipher not configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted key: %w", err)
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	plain, err := c.gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt profile key: %w", err)
	}
	return string(plain), nil
}

// IsEncrypted reports whether value looks like our ciphertext.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encPrefix)
}
