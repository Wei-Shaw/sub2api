//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type liveTransferCacheStub struct {
	stubConcurrencyCacheForTest
	transferCalls   int
	transferredKeys []string
	lastTransfer    LiveLeaseTransferRequest
	transferResult  bool
	transferErr     error
	legacyCalls     int
	legacyResult    bool
	abortCalls      int
	abortErr        error
	onTransfer      func()

	migrateCalls     int
	migratedAccounts []int64
	migrateRequest   LiveLeaseTransferRequest
	migrateResult    bool
	migrateErr       error
	accountRelease   int
	releaseLeaseCall int
	releasedLeaseID  string
}

func (c *liveTransferCacheStub) AcquireLiveLease(context.Context, int64, int, int64, int, int64, int, string, bool) (bool, error) {
	c.legacyCalls++
	return c.legacyResult, nil
}
func (c *liveTransferCacheStub) AcquireLiveLeaseTransferring(_ context.Context, request LiveLeaseTransferRequest) (bool, error) {
	c.transferCalls++
	c.lastTransfer = request
	c.transferredKeys = append(c.transferredKeys, request.KeyRequestID)
	if c.onTransfer != nil {
		c.onTransfer()
	}
	return c.transferResult, c.transferErr
}
func (c *liveTransferCacheStub) MigrateLiveLeaseAccount(_ context.Context, request LiveLeaseTransferRequest) (bool, error) {
	c.migrateCalls++
	c.migrateRequest = request
	c.migratedAccounts = append(c.migratedAccounts, request.AccountID)
	return c.migrateResult, c.migrateErr
}
func (c *liveTransferCacheStub) ReleaseLiveLeaseAccount(context.Context, int64, string) error {
	c.accountRelease++
	return nil
}
func (c *liveTransferCacheStub) AbortLiveLeaseTransfer(context.Context, LiveLeaseTransferRequest) error {
	c.abortCalls++
	return c.abortErr
}
func (c *liveTransferCacheStub) RefreshLiveLease(context.Context, int64, int64, int64, string) (bool, error) {
	return true, nil
}
func (c *liveTransferCacheStub) ReleaseLiveLease(_ context.Context, _, _, _ int64, leaseID string) error {
	c.releaseLeaseCall++
	c.releasedLeaseID = leaseID
	return nil
}

// liveLeaseOnlyCacheStub intentionally has no transfer support so the fallback
// branch is exercised.
type liveLeaseOnlyCacheStub struct {
	stubConcurrencyCacheForTest
	legacyCalls int
}

func (c *liveLeaseOnlyCacheStub) AcquireLiveLease(context.Context, int64, int, int64, int, int64, int, string, bool) (bool, error) {
	c.legacyCalls++
	return false, nil
}
func (c *liveLeaseOnlyCacheStub) RefreshLiveLease(context.Context, int64, int64, int64, string) (bool, error) {
	return true, nil
}
func (c *liveLeaseOnlyCacheStub) ReleaseLiveLease(context.Context, int64, int64, int64, string) error {
	return nil
}

func TestAcquireLiveLeaseForIdentityTransfersExactMembers(t *testing.T) {
	cache := &liveTransferCacheStub{transferResult: true}
	paused := 0
	pausedBeforeTransfer := false
	cache.onTransfer = func() { pausedBeforeTransfer = paused > 0 }
	reservation := &APIKeySlotReservation{
		apiKeyID:     9,
		requestID:    "key-req-1",
		limit:        1,
		release:      func() {},
		pauseRenewal: func() { paused++ },
	}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{KeyReservation: reservation},
		1, 2, "acct-req", 2, 3, "user-req", 9, 1, "live-lease",
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.True(t, pausedBeforeTransfer, "ordinary key renewal stops before the atomic transfer")
	require.Equal(t, []string{"key-req-1"}, cache.transferredKeys)
	require.Zero(t, cache.legacyCalls, "reservation paths must not double count capacity")
	require.Empty(t, reservation.RequestID(), "consumed reservation cannot be transferred twice")
	require.Equal(t, "acct-req", cache.lastTransfer.AccountRequestID)
	require.Equal(t, "user-req", cache.lastTransfer.UserRequestID)
	require.Zero(t, cache.abortCalls)
}

func TestAcquireLiveLeaseForIdentityTransfersStatsOnlyMemberAtomically(t *testing.T) {
	cache := &liveTransferCacheStub{transferResult: true}
	released := 0
	paused := 0
	reservation := &APIKeySlotReservation{
		apiKeyID:     9,
		requestID:    "stats-member",
		limit:        0,
		release:      func() { released++ },
		pauseRenewal: func() { paused++ },
	}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{KeyReservation: reservation},
		1, 0, "", 2, 0, "", 9, 0, "live-lease",
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, "stats-member", cache.lastTransfer.KeyRequestID,
		"stats-only member is moved by the same atomic transfer")
	require.Empty(t, reservation.RequestID())
	require.Positive(t, paused, "stats renewal worker is stopped on consume")
	// The original release cannot delete the transferred Live member.
	reservation.Release()
	require.Zero(t, released)
}

func TestAcquireLiveLeaseForIdentityLimitedKeyWithoutReservationFailsClosed(t *testing.T) {
	cache := &liveTransferCacheStub{transferResult: true}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{},
		1, 0, "", 2, 0, "", 9, 2, "live-lease",
	)
	require.False(t, acquired)
	require.ErrorIs(t, err, ErrAPIKeyReservationLost)
	require.Zero(t, cache.transferCalls)
	require.Zero(t, cache.legacyCalls)
}

func TestAcquireLiveLeaseForIdentityUncertainAbortsExactTransfer(t *testing.T) {
	cache := &liveTransferCacheStub{transferErr: ErrLiveLeaseTransferUncertain}
	reservation := &APIKeySlotReservation{
		apiKeyID:     9,
		requestID:    "key-req-2",
		limit:        1,
		release:      func() {},
		pauseRenewal: func() {},
	}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{KeyReservation: reservation},
		1, 0, "", 2, 0, "", 9, 1, "live-lease",
	)
	require.False(t, acquired)
	require.ErrorIs(t, err, ErrLiveUnavailable, "unknown transfer acknowledgement must not start SDP")
	require.Equal(t, 1, cache.abortCalls, "unknown acknowledgement must be fenced and cleaned")
	require.Empty(t, reservation.RequestID(), "reservation belongs to the aborted successor")
}

func TestAcquireLiveLeaseForIdentityDefiniteFailureKeepsSourcesReleasable(t *testing.T) {
	cache := &liveTransferCacheStub{transferErr: ErrLiveConcurrencyFull}
	released := 0
	reservation := &APIKeySlotReservation{
		apiKeyID:     9,
		requestID:    "key-req-3",
		limit:        1,
		release:      func() { released++ },
		pauseRenewal: func() {},
	}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{KeyReservation: reservation},
		1, 0, "", 2, 0, "", 9, 1, "live-lease",
	)
	require.False(t, acquired)
	require.ErrorIs(t, err, ErrLiveConcurrencyFull)
	require.Zero(t, cache.abortCalls, "a definite refusal did not change Redis state")
	require.Equal(t, "key-req-3", reservation.RequestID())
	reservation.Release()
	require.Equal(t, 1, released, "ordinary sources stay owned by the caller")
}

func TestAcquireLiveLeaseForIdentityFallsBackWithRelease(t *testing.T) {
	cache := &liveLeaseOnlyCacheStub{}
	released := 0
	reservation := &APIKeySlotReservation{apiKeyID: 9, requestID: "key-req-4", limit: 1, release: func() { released++ }}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{KeyReservation: reservation},
		1, 1, "acct-req", 2, 1, "user-req", 9, 1, "live-lease",
	)
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, 1, cache.legacyCalls)
	require.Equal(t, 1, released, "a cache without transfer support must release the reservation")
}

func TestAcquireLiveLeaseForIdentityUncertainAbortFailureSurfacesError(t *testing.T) {
	cache := &liveTransferCacheStub{
		transferErr: ErrLiveLeaseTransferUncertain,
		abortErr:    errors.New("redis down"),
	}
	reservation := &APIKeySlotReservation{
		apiKeyID:     9,
		requestID:    "key-req-5",
		limit:        1,
		release:      func() {},
		pauseRenewal: func() {},
	}

	acquired, err := acquireLiveLeaseForIdentity(
		context.Background(), cache,
		LiveCallIdentity{KeyReservation: reservation},
		1, 0, "", 2, 0, "", 9, 1, "live-lease",
	)
	require.False(t, acquired)
	require.ErrorIs(t, err, ErrLiveUnavailable)
	require.Contains(t, err.Error(), "abort Live transfer")
	require.Empty(t, reservation.RequestID())
}

func TestMigrateLiveLeaseAccountKeepsJointLease(t *testing.T) {
	cache := &liveTransferCacheStub{migrateResult: true}
	acquired, err := migrateLiveLeaseAccount(
		context.Background(), cache,
		LiveCallIdentity{UserID: 2, APIKeyID: 9},
		&Account{ID: 77, Concurrency: 1},
		"acct-new",
		"joint-lease",
		55,
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, 1, cache.migrateCalls)
	require.Equal(t, int64(77), cache.migrateRequest.AccountID)
	require.Equal(t, "acct-new", cache.migrateRequest.AccountRequestID)
	require.Equal(t, "joint-lease", cache.migrateRequest.LeaseID)
	require.Equal(t, int64(55), cache.migrateRequest.ReplacedAccountID)
	require.Zero(t, cache.transferCalls, "retry must not re-enter the Key transfer")
}

func TestMigrateLiveLeaseAccountUncertainAbortsAndFailsClosed(t *testing.T) {
	cache := &liveTransferCacheStub{migrateErr: ErrLiveLeaseTransferUncertain}
	acquired, err := migrateLiveLeaseAccount(
		context.Background(), cache,
		LiveCallIdentity{UserID: 2, APIKeyID: 9},
		&Account{ID: 77, Concurrency: 1},
		"acct-new",
		"joint-lease",
		0,
	)
	require.False(t, acquired)
	require.ErrorIs(t, err, ErrLiveUnavailable)
	require.Equal(t, 1, cache.abortCalls)
}

// --- CreateLiveCall retry-loop joint lease cleanup regression ---

type liveLoopStoreStub struct {
	stubGatewayCache
	saves int
}

func (s *liveLoopStoreStub) SaveLiveCall(context.Context, *LiveCallRecord, time.Duration) error {
	s.saves++
	return nil
}
func (s *liveLoopStoreStub) GetLiveCall(context.Context, string) (*LiveCallRecord, error) {
	return nil, ErrLiveCallNotFound
}
func (s *liveLoopStoreStub) ClaimLiveController(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (s *liveLoopStoreStub) ReleaseLiveController(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *liveLoopStoreStub) GetLiveController(context.Context, string) (string, error) {
	return "", nil
}
func (s *liveLoopStoreStub) MarkLiveCallClosed(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

type liveLoopUpstreamStub struct {
	calls    int
	failures int
}

func (s *liveLoopUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	s.calls++
	if s.calls <= s.failures {
		return nil, errors.New("live sdp transport failure")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Location": {fmt.Sprintf("/backend-api/codex/call_%d", s.calls)}},
		Body:       io.NopCloser(strings.NewReader("v=0\r\n")),
	}, nil
}

func (s *liveLoopUpstreamStub) DoWithTLS(request *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, accountConcurrency)
}

func newLiveLoopFixture(t *testing.T, upstream *liveLoopUpstreamStub, withAccessToken bool) (*OpenAIGatewayService, *liveTransferCacheStub, *liveLoopStoreStub) {
	t.Helper()
	snapshot := &snapshotHydrationCache{accounts: map[int64]*Account{}}
	for i := int64(1); i <= 6; i++ {
		credentials := map[string]any{
			"chatgpt_account_id": fmt.Sprintf("acct-%d", i),
			"model_mapping":      map[string]any{"gpt-live-test": "gpt-live-test"},
		}
		if withAccessToken {
			credentials["access_token"] = fmt.Sprintf("token-%d", i)
		}
		account := &Account{
			ID:          i,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    int(i),
			Credentials: credentials,
		}
		snapshot.accounts[i] = account
		snapshot.snapshot = append(snapshot.snapshot, account)
	}

	cacheStub := &liveTransferCacheStub{
		stubConcurrencyCacheForTest: stubConcurrencyCacheForTest{acquireResult: true},
		transferResult:              true,
		migrateResult:               true,
	}
	store := &liveLoopStoreStub{}
	svc := &OpenAIGatewayService{
		// nil cfg keeps schedulingConfig() defaults so the load-aware selection
		// path (matching production) is exercised; nil accountRepo keeps the
		// scheduler-snapshot recheck from dropping the fixture accounts.
		cfg:                   nil,
		cache:                 store,
		schedulerSnapshot:     NewSchedulerSnapshotService(snapshot, nil, stubOpenAIAccountRepo{}, nil, nil),
		concurrencyService:    NewConcurrencyService(cacheStub),
		httpUpstream:          upstream,
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: newLiveAttestationCipher(&config.Config{JWT: config.JWTConfig{Secret: "live-loop-secret"}}),
	}
	return svc, cacheStub, store
}

func liveLoopIdentity() LiveCallIdentity {
	return LiveCallIdentity{
		UserID:                 20,
		APIKeyID:               9,
		APIKeyConcurrencyLimit: 1,
		UserRequestID:          "user-req",
		KeyReservation: &APIKeySlotReservation{
			apiKeyID:     9,
			requestID:    "key-req",
			limit:        1,
			release:      func() {},
			pauseRenewal: func() {},
		},
	}
}

func liveLoopRequest() *LiveCallRequest {
	return &LiveCallRequest{
		SDP:     "v=0\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test"}`),
	}
}

// TestCreateLiveCallExhaustedRetriesReleaseJointLease proves four retryable SDP
// failures release the retained Key/user members immediately (no TTL wait).
func TestCreateLiveCallExhaustedRetriesReleaseJointLease(t *testing.T) {
	svc, cacheStub, store := newLiveLoopFixture(t, &liveLoopUpstreamStub{}, false)

	created, err := svc.CreateLiveCall(context.Background(), liveLoopRequest(), liveLoopIdentity(), 1)
	require.Nil(t, created)
	require.Error(t, err, "all four attempts must fail on the missing access token")

	require.Equal(t, 1, cacheStub.transferCalls, "first attempt transfers the joint lease")
	require.Equal(t, 3, cacheStub.migrateCalls, "retries migrate the account member")
	require.Equal(t, 4, cacheStub.accountRelease, "each failure releases only its account member")
	require.Equal(t, 1, cacheStub.releaseLeaseCall,
		"exhausting the retries must release the retained joint Key/user members once")
	require.NotEmpty(t, cacheStub.releasedLeaseID)
	require.Equal(t, cacheStub.lastTransfer.LeaseID, cacheStub.releasedLeaseID,
		"the released lease is the retained joint lease")
	require.Zero(t, store.saves)
}

// TestCreateLiveCallSafeRetryRetainsJointLease proves the joint members survive
// a retryable failure until the call reaches a terminal outcome, and no
// terminal release happens while the live call continues.
func TestCreateLiveCallSafeRetryRetainsJointLease(t *testing.T) {
	upstream := &liveLoopUpstreamStub{failures: 1}
	svc, cacheStub, store := newLiveLoopFixture(t, upstream, true)

	created, err := svc.CreateLiveCall(context.Background(), liveLoopRequest(), liveLoopIdentity(), 1)
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEmpty(t, created.CallID)

	require.Equal(t, 1, cacheStub.transferCalls)
	require.Equal(t, 1, cacheStub.migrateCalls)
	require.Equal(t, 1, cacheStub.accountRelease, "only the failed account member is released")
	require.Zero(t, cacheStub.releaseLeaseCall,
		"a live call keeps its joint Key/user lease; terminal release comes from finalize")
	require.Equal(t, 1, store.saves)
}
