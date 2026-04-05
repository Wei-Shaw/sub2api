package payment

import (
	"encoding/hex"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/wire"
)

// EncryptionKey is a named type for the payment encryption key (AES-256, 32 bytes).
// Using a named type avoids Wire ambiguity with other []byte parameters.
type EncryptionKey []byte

// ProvideEncryptionKey derives the payment encryption key from the TOTP encryption key in config.
func ProvideEncryptionKey(cfg *config.Config) EncryptionKey {
	key, _ := hex.DecodeString(cfg.Totp.EncryptionKey)
	return EncryptionKey(key)
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
