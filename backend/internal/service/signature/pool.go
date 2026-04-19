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

// Account types that do not participate in the signature pool.
const (
	accountTypeBedrock  = "bedrock"
	accountTypeUpstream = "upstream"

	platformAnthropicDirect = "anthropic"
)

// BucketFor maps (accountType, platform, accountID) to a pool bucket key.
// OAuth/setup-token accounts share a per-platform pool (anthropic signatures
// must not mix with antigravity signatures). API Key accounts get a
// per-account pool regardless of platform. Returns "" for types that do not
// participate (bedrock, upstream).
func BucketFor(accountType, platform string, accountID int64) string {
	switch accountType {
	case accountTypeOAuth, accountTypeSetupToken:
		if platform == "" {
			platform = platformAnthropicDirect
		}
		return fmt.Sprintf("oauth:%s", platform)
	case accountTypeAPIKey:
		return fmt.Sprintf("apikey:%d", accountID)
	case accountTypeBedrock, accountTypeUpstream:
		return ""
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
