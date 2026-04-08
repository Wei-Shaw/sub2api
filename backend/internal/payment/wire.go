package payment

import (
	"encoding/hex"
	"fmt"
	"log/slog"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/wire"
)

// EncryptionKey is a named type for the payment encryption key (AES-256, 32 bytes).
// Using a named type avoids Wire ambiguity with other []byte parameters.
type EncryptionKey []byte

// ProvideEncryptionKey derives the payment encryption key from the TOTP encryption key in config.
// When the key is empty, nil is returned (payment features that need encryption will be disabled).
// When the key is non-empty but invalid (bad hex or wrong length), an error is returned
// to prevent startup with a misconfigured encryption key.
func ProvideEncryptionKey(cfg *config.Config) (EncryptionKey, error) {
	if cfg.Totp.EncryptionKey == "" {
		slog.Warn("payment encryption key not configured — encrypted payment config will be unavailable")
		return nil, nil
	}
	key, err := hex.DecodeString(cfg.Totp.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid payment encryption key (hex decode): %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("payment encryption key must be 32 bytes, got %d", len(key))
	}
	return EncryptionKey(key), nil
}

// ProvideRegistry creates an empty payment provider registry.
// Providers are registered at runtime after application startup.
func ProvideRegistry() *Registry {
	return NewRegistry()
}

// ProvideDefaultLoadBalancer creates a DefaultLoadBalancer backed by the ent client.
func ProvideDefaultLoadBalancer(client *dbent.Client, key EncryptionKey) *DefaultLoadBalancer {
	return NewDefaultLoadBalancer(client, []byte(key))
}

// ProviderSet is the Wire provider set for the payment package.
var ProviderSet = wire.NewSet(
	ProvideEncryptionKey,
	ProvideRegistry,
	ProvideDefaultLoadBalancer,
	wire.Bind(new(LoadBalancer), new(*DefaultLoadBalancer)),
)
