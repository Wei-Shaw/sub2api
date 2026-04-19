package signature

import (
	"context"
	"fmt"
	"time"
)

// Account type identifiers used by Bucket.
// These mirror the existing AccountType* constants in the service package
// but are duplicated here as primitives to keep this package dependency-free.
const (
	accountTypeOAuth      = "oauth"
	accountTypeSetupToken = "setup-token"
	accountTypeAPIKey     = "apikey"
)

// Bucket keys used by the SignaturePool.
//   - BucketOAuthShared: shared pool across all max OAuth and setup-token accounts
//   - BucketAPIKey(id):  per-account pool for API Key accounts
//
// Bedrock / Upstream / other types currently do not participate in the pool
// (they never emit thinking signatures we can reuse).
const (
	BucketOAuthShared = "oauth"
)

// BucketAPIKey returns the bucket key for an API Key account.
func BucketAPIKey(accountID int64) string {
	return fmt.Sprintf("apikey:%d", accountID)
}

// BucketFor maps an account type+id to its pool bucket.
// Returns empty string for types that do not participate in the pool.
func BucketFor(accountType string, accountID int64) string {
	switch accountType {
	case accountTypeOAuth, accountTypeSetupToken:
		return BucketOAuthShared
	case accountTypeAPIKey:
		return BucketAPIKey(accountID)
	}
	return ""
}

// SignaturePool stores thinking-block signatures harvested from successful
// upstream responses and returns the freshest N on demand for pool-replace retry.
//
// Implementations are expected to be safe for concurrent use across goroutines.
type SignaturePool interface {
	// Add stores a signature in the given bucket with the provided timestamp.
	// Expired entries (older than the configured soft TTL) and entries beyond
	// the configured capacity are evicted lazily in the same operation.
	Add(ctx context.Context, bucket string, signature string, at time.Time, capacity int) error

	// TopN returns up to n most recently added signatures in the bucket,
	// ordered newest-first. Expired entries are skipped (lazy expiry).
	TopN(ctx context.Context, bucket string, n int) ([]string, error)

	// Size returns the current number of live (non-expired) entries in the bucket.
	Size(ctx context.Context, bucket string) (int64, error)
}

// DefaultSignatureTTL is the soft expiry applied to pool entries.
// Entries older than this are evicted lazily on the next Add call for the bucket.
const DefaultSignatureTTL = time.Hour
