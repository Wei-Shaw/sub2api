package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbsecuritysecret "github.com/Wei-Shaw/sub2api/ent/securitysecret"
)

// GetSecuritySecret reads one persisted security secret without creating it.
// A missing row is reported as found=false so read-only deployments can keep
// serving ordinary traffic before a feature that owns the secret is enabled.
func (r *accountRepository) GetSecuritySecret(ctx context.Context, key string) (string, bool, error) {
	if r == nil || r.client == nil {
		return "", false, fmt.Errorf("security secret store is unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 100 {
		return "", false, fmt.Errorf("security secret key is invalid")
	}
	stored, err := r.client.SecuritySecret.Query().Where(dbsecuritysecret.KeyEQ(key)).Only(ctx)
	if dbent.IsNotFound(err) || isSQLNoRowsError(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(stored.Value)
	if len([]byte(value)) < 32 {
		return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	}
	return value, true, nil
}

// GetOrCreateGeneratedSecuritySecret reuses the existing cross-instance
// security_secrets bootstrap primitive. The returned value is the persisted
// hex representation, not raw key bytes.
func (r *accountRepository) GetOrCreateGeneratedSecuritySecret(ctx context.Context, key string, byteLength int) (string, bool, error) {
	if r == nil || r.client == nil {
		return "", false, fmt.Errorf("security secret store is unavailable")
	}
	return getOrCreateGeneratedSecuritySecret(ctx, r.client, strings.TrimSpace(key), byteLength)
}
