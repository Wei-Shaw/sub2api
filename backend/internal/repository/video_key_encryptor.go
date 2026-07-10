package repository

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NewVideoKeyEncryptor creates the dedicated reversible encryptor for video
// upstream API keys. video_gateway.encryption_key is REQUIRED.
//
// Previously an empty key silently fell back to totp.encryption_key with only a
// Warn. That is a key-domain-confusion risk: a production deployment that forgot
// to configure the dedicated key would encrypt real Seedance credentials under
// the TOTP key, and the Warn is easy to miss. We now hard-fail instead, so the
// misconfiguration cannot reach production.
func NewVideoKeyEncryptor(cfg *config.Config) (service.VideoKeyEncryptor, error) {
	keyHex := strings.TrimSpace(cfg.VideoGateway.EncryptionKey)
	if keyHex == "" {
		return nil, fmt.Errorf("video_gateway.encryption_key is required: refusing to fall back to totp.encryption_key (key-domain confusion); configure a dedicated 32-byte (64 hex char) key")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid video gateway encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("video gateway encryption key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return &AESEncryptor{key: key}, nil
}
