//go:build integration

package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/signature"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SignaturePoolCacheSuite struct {
	IntegrationRedisSuite
	pool signature.SignaturePool
}

func TestSignaturePoolCacheSuite(t *testing.T) {
	suite.Run(t, new(SignaturePoolCacheSuite))
}

func (s *SignaturePoolCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.pool = NewSignaturePoolCache(s.rdb, signature.DefaultSignatureTTL)
}

// uniqueBucket returns a test-isolated bucket to avoid cross-test pollution.
func (s *SignaturePoolCacheSuite) uniqueBucket(suffix string) string {
	return fmt.Sprintf("test:%s:%d", suffix, time.Now().UnixNano())
}

// --- Basic flow ---

func (s *SignaturePoolCacheSuite) TestAddAndTopN_BasicFlow() {
	bucket := s.uniqueBucket("basic")
	now := time.Now()

	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig-A", now, 10))
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig-B", now.Add(time.Second), 10))
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig-C", now.Add(2*time.Second), 10))

	sigs, err := s.pool.TopN(s.ctx, bucket, 3)
	s.RequireNoError(err)
	// Newest first: C, B, A
	require.Equal(s.T(), []string{"sig-C", "sig-B", "sig-A"}, sigs)
}

func (s *SignaturePoolCacheSuite) TestTopN_ReturnsFewerWhenPoolSmall() {
	bucket := s.uniqueBucket("fewer")
	now := time.Now()

	s.RequireNoError(s.pool.Add(s.ctx, bucket, "only-one", now, 10))

	sigs, err := s.pool.TopN(s.ctx, bucket, 5)
	s.RequireNoError(err)
	require.Equal(s.T(), []string{"only-one"}, sigs)
}

func (s *SignaturePoolCacheSuite) TestTopN_EmptyBucketReturnsNil() {
	sigs, err := s.pool.TopN(s.ctx, s.uniqueBucket("empty"), 10)
	s.RequireNoError(err)
	require.Empty(s.T(), sigs)
}

// --- Capacity trimming ---

func (s *SignaturePoolCacheSuite) TestAdd_TrimsToCapacity() {
	bucket := s.uniqueBucket("cap")
	now := time.Now()
	cap := 3

	for i := 0; i < 6; i++ {
		sig := fmt.Sprintf("sig-%d", i)
		s.RequireNoError(s.pool.Add(s.ctx, bucket, sig, now.Add(time.Duration(i)*time.Second), cap))
	}

	// Only the newest 3 should survive.
	sigs, err := s.pool.TopN(s.ctx, bucket, 10)
	s.RequireNoError(err)
	require.Equal(s.T(), []string{"sig-5", "sig-4", "sig-3"}, sigs)

	sz, err := s.pool.Size(s.ctx, bucket)
	s.RequireNoError(err)
	require.Equal(s.T(), int64(3), sz)
}

// --- Soft TTL lazy expiry ---

func (s *SignaturePoolCacheSuite) TestAdd_EvictsExpiredEntries() {
	bucket := s.uniqueBucket("ttl")
	ttl := signature.DefaultSignatureTTL // 1h

	// Add an entry timestamped 2 hours in the past — already expired.
	oldTime := time.Now().Add(-2 * ttl)
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "old-sig", oldTime, 100))

	// Verify it exists before a new Add triggers cleanup.
	sz, err := s.pool.Size(s.ctx, bucket)
	s.RequireNoError(err)
	require.Equal(s.T(), int64(1), sz, "old entry should exist before lazy cleanup")

	// Add a fresh entry — lazy cleanup should remove the expired one.
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "new-sig", time.Now(), 100))

	sigs, err := s.pool.TopN(s.ctx, bucket, 10)
	s.RequireNoError(err)
	require.Equal(s.T(), []string{"new-sig"}, sigs, "expired entry should be evicted by lazy cleanup")

	sz, err = s.pool.Size(s.ctx, bucket)
	s.RequireNoError(err)
	require.Equal(s.T(), int64(1), sz)
}

func (s *SignaturePoolCacheSuite) TestTopN_DoesNotFilterByTTL() {
	// Per design: "避免没有签名可用" — expired entries survive until the next
	// Add evicts them.
	bucket := s.uniqueBucket("no-filter")
	ttl := signature.DefaultSignatureTTL

	oldTime := time.Now().Add(-2 * ttl)
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "stale-but-valid", oldTime, 100))

	sigs, err := s.pool.TopN(s.ctx, bucket, 10)
	s.RequireNoError(err)
	require.Equal(s.T(), []string{"stale-but-valid"}, sigs,
		"TopN must NOT filter by TTL — lazy expiry only happens on Add")
}

// --- De-duplication (ZADD score update) ---

func (s *SignaturePoolCacheSuite) TestAdd_SameSignatureUpdatesScore() {
	bucket := s.uniqueBucket("dedup")
	now := time.Now()

	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig-A", now, 10))
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig-B", now.Add(time.Second), 10))
	// Re-add sig-A with a newer timestamp — it should move to the front.
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig-A", now.Add(2*time.Second), 10))

	sigs, err := s.pool.TopN(s.ctx, bucket, 10)
	s.RequireNoError(err)
	// sig-A is now newest (score updated).
	require.Equal(s.T(), []string{"sig-A", "sig-B"}, sigs)

	sz, err := s.pool.Size(s.ctx, bucket)
	s.RequireNoError(err)
	require.Equal(s.T(), int64(2), sz, "duplicate should not increase pool size")
}

// --- Redis key-level TTL ---

func (s *SignaturePoolCacheSuite) TestAdd_SetsKeyTTL() {
	bucket := s.uniqueBucket("keyttl")
	key := signaturePoolKey(bucket)

	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig", time.Now(), 10))

	ttl, err := s.rdb.TTL(s.ctx, key).Result()
	s.RequireNoError(err)
	// key TTL = softTTL * signaturePoolKeyTTLFactor = 1h * 24 = 24h.
	// Allow a generous window for test latency.
	s.AssertTTLWithin(ttl, 23*time.Hour, 25*time.Hour)
}

// --- Size ---

func (s *SignaturePoolCacheSuite) TestSize_ReflectsCurrentEntries() {
	bucket := s.uniqueBucket("size")
	now := time.Now()

	sz, err := s.pool.Size(s.ctx, bucket)
	s.RequireNoError(err)
	require.Equal(s.T(), int64(0), sz, "empty bucket")

	for i := 0; i < 5; i++ {
		s.RequireNoError(s.pool.Add(s.ctx, bucket, fmt.Sprintf("s%d", i), now.Add(time.Duration(i)*time.Second), 100))
	}
	sz, err = s.pool.Size(s.ctx, bucket)
	s.RequireNoError(err)
	require.Equal(s.T(), int64(5), sz)
}

// --- Edge cases ---

func (s *SignaturePoolCacheSuite) TestAdd_EmptyBucketOrSignatureIsNoop() {
	require.NoError(s.T(), s.pool.Add(s.ctx, "", "sig", time.Now(), 10))
	require.NoError(s.T(), s.pool.Add(s.ctx, "b", "", time.Now(), 10))
}

func (s *SignaturePoolCacheSuite) TestTopN_EmptyBucketStringReturnsNil() {
	sigs, err := s.pool.TopN(s.ctx, "", 10)
	s.RequireNoError(err)
	require.Empty(s.T(), sigs)
}

func (s *SignaturePoolCacheSuite) TestTopN_ZeroNReturnsNil() {
	bucket := s.uniqueBucket("zeron")
	s.RequireNoError(s.pool.Add(s.ctx, bucket, "sig", time.Now(), 10))

	sigs, err := s.pool.TopN(s.ctx, bucket, 0)
	s.RequireNoError(err)
	require.Empty(s.T(), sigs)
}
