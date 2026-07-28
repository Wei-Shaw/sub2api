//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokVideoOwnerCacheStub struct {
	bindings map[string]int64
	ttls     map[string]time.Duration
	setErr   error
}

func (c *grokVideoOwnerCacheStub) GetSessionAccountID(_ context.Context, _ int64, key string) (int64, error) {
	if id, ok := c.bindings[key]; ok {
		return id, nil
	}
	return 0, errors.New("cache miss")
}

func (c *grokVideoOwnerCacheStub) SetSessionAccountID(_ context.Context, _ int64, key string, accountID int64, ttl time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	if c.ttls == nil {
		c.ttls = make(map[string]time.Duration)
	}
	c.bindings[key] = accountID
	c.ttls[key] = ttl
	return nil
}

func (c *grokVideoOwnerCacheStub) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *grokVideoOwnerCacheStub) DeleteSessionAccountID(_ context.Context, _ int64, key string) error {
	delete(c.bindings, key)
	return nil
}

type grokVideoOwnerRepoStub struct {
	cleanupMu         sync.Mutex
	owners            map[string]GrokMediaVideoRequestOwner
	creates           map[string]GrokMediaVideoCreateRecord
	bindErr           error
	getErr            error
	cleanupLimit      int
	ownerCleanupErr   error
	videoCleanupErr   error
	ownerCleanupCalls int
	videoCleanupCalls int
}

func grokVideoOwnerRepoKey(requestID string, userID, apiKeyID, groupID int64) string {
	return fmt.Sprintf("%d:%d:%d:%s", groupID, userID, apiKeyID, requestID)
}

func (r *grokVideoOwnerRepoStub) Bind(_ context.Context, owner GrokMediaVideoRequestOwner) error {
	if r.bindErr != nil {
		return r.bindErr
	}
	if r.owners == nil {
		r.owners = make(map[string]GrokMediaVideoRequestOwner)
	}
	key := grokVideoOwnerRepoKey(owner.RequestID, owner.UserID, owner.APIKeyID, owner.GroupID)
	if existing, ok := r.owners[key]; ok && existing.AccountID != owner.AccountID {
		return ErrGrokMediaVideoRequestOwnerConflict
	}
	r.owners[key] = owner
	return nil
}

func (r *grokVideoOwnerRepoStub) Resolve(_ context.Context, requestID string, userID, apiKeyID, groupID int64, refreshUntil time.Time) (*GrokMediaVideoRequestOwner, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	owner, ok := r.owners[grokVideoOwnerRepoKey(requestID, userID, apiKeyID, groupID)]
	if !ok || !time.Now().Before(owner.ExpiresAt) {
		return nil, ErrGrokMediaVideoRequestOwnerNotFound
	}
	if refreshUntil.After(owner.ExpiresAt) {
		owner.ExpiresAt = refreshUntil
		r.owners[grokVideoOwnerRepoKey(requestID, userID, apiKeyID, groupID)] = owner
	}
	return &owner, nil
}

func (r *grokVideoOwnerRepoStub) MarkTerminal(_ context.Context, requestID string, userID, apiKeyID, groupID int64, terminalAt, retainUntil time.Time) error {
	key := grokVideoOwnerRepoKey(requestID, userID, apiKeyID, groupID)
	owner, ok := r.owners[key]
	if !ok || !time.Now().Before(owner.ExpiresAt) {
		return ErrGrokMediaVideoRequestOwnerNotFound
	}
	if owner.TerminalAt == nil {
		value := terminalAt
		owner.TerminalAt = &value
	}
	if retainUntil.After(owner.ExpiresAt) {
		owner.ExpiresAt = retainUntil
	}
	r.owners[key] = owner
	return nil
}

func (r *grokVideoOwnerRepoStub) DeleteExpired(_ context.Context, before time.Time, limit int) (int64, error) {
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	r.ownerCleanupCalls++
	r.cleanupLimit = limit
	if r.ownerCleanupErr != nil {
		return 0, r.ownerCleanupErr
	}
	var deleted int64
	for key, owner := range r.owners {
		if deleted >= int64(limit) {
			break
		}
		if !owner.ExpiresAt.After(before) {
			delete(r.owners, key)
			deleted++
		}
	}
	return deleted, nil
}

func (r *grokVideoOwnerRepoStub) DeleteExpiredVideoCreates(_ context.Context, before time.Time, limit int) (int64, error) {
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	r.videoCleanupCalls++
	if r.videoCleanupErr != nil {
		return 0, r.videoCleanupErr
	}
	var deleted int64
	for key, record := range r.creates {
		if deleted >= int64(limit) {
			break
		}
		if !record.ExpiresAt.After(before) {
			delete(r.creates, key)
			deleted++
		}
	}
	return deleted, nil
}

func grokVideoCreateRepoKey(record GrokMediaVideoCreateRecord) string {
	return fmt.Sprintf("%d:%d:%d:%s:%s", record.GroupID, record.UserID, record.APIKeyID, record.Endpoint, record.IdempotencyKeyHash)
}

func (r *grokVideoOwnerRepoStub) ClaimVideoCreate(_ context.Context, record GrokMediaVideoCreateRecord) (*GrokMediaVideoCreateRecord, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.creates == nil {
		r.creates = make(map[string]GrokMediaVideoCreateRecord)
	}
	key := grokVideoCreateRepoKey(record)
	if existing, ok := r.creates[key]; ok {
		if existing.RequestHash != record.RequestHash {
			return nil, ErrGrokMediaVideoIdempotencyConflict
		}
		copy := existing
		copy.ResponseBody = append([]byte(nil), existing.ResponseBody...)
		return &copy, nil
	}
	r.creates[key] = record
	copy := record
	return &copy, nil
}

func (r *grokVideoOwnerRepoStub) BindVideoCreateAccount(_ context.Context, record GrokMediaVideoCreateRecord, accountID int64) (int64, error) {
	key := grokVideoCreateRepoKey(record)
	existing, ok := r.creates[key]
	if !ok {
		return 0, ErrGrokMediaVideoIdempotencyUnavailable
	}
	if existing.AccountID == 0 {
		existing.AccountID = accountID
		r.creates[key] = existing
	}
	return existing.AccountID, nil
}

func (r *grokVideoOwnerRepoStub) ReleaseVideoCreateAccount(_ context.Context, record GrokMediaVideoCreateRecord, accountID int64) (bool, error) {
	key := grokVideoCreateRepoKey(record)
	existing, ok := r.creates[key]
	if !ok || existing.AccountID != accountID {
		return false, nil
	}
	existing.AccountID = 0
	r.creates[key] = existing
	return true, nil
}

func (r *grokVideoOwnerRepoStub) CompleteVideoCreate(ctx context.Context, record GrokMediaVideoCreateRecord, owner GrokMediaVideoRequestOwner) error {
	if err := r.Bind(ctx, owner); err != nil {
		return err
	}
	key := grokVideoCreateRepoKey(record)
	r.creates[key] = record
	return nil
}

func TestGrokMediaVideoOwnerBindingDBAuthoritativeIsolatedAndRecoverable(t *testing.T) {
	ctx := context.Background()
	groupID := int64(7)
	const userID int64 = 41
	const apiKeyID int64 = 51
	const ownerID int64 = 63
	repo := &grokVideoOwnerRepoStub{}
	cache := &grokVideoOwnerCacheStub{}
	svc := &OpenAIGatewayService{cache: cache, grokMediaVideoOwnerRepo: repo}
	requestID := "video-request-123"
	ownerKey := grokMediaVideoRequestOwnerCacheKey(requestID, userID, apiKeyID, groupID)

	require.NoError(t, svc.BindGrokMediaVideoRequestAccount(ctx, &groupID, requestID, userID, apiKeyID, ownerID))
	require.Equal(t, grokMediaVideoRequestOwnerRecoveryWindow, cache.ttls[ownerKey])
	stored := repo.owners[grokVideoOwnerRepoKey(requestID, userID, apiKeyID, groupID)]
	require.WithinDuration(t, time.Now().Add(grokMediaVideoRequestOwnerRecoveryWindow), stored.ExpiresAt, 2*time.Second)

	// A corrupted Redis value cannot override the durable record.
	cache.bindings[ownerKey] = 99
	resolved, err := svc.ResolveGrokMediaVideoRequestAccount(ctx, &groupID, requestID, userID, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, ownerID, resolved)

	// A fresh process with an empty cache recovers from the database record.
	restarted := &OpenAIGatewayService{cache: &grokVideoOwnerCacheStub{}, grokMediaVideoOwnerRepo: repo}
	stored.ExpiresAt = time.Now().Add(4 * 24 * time.Hour)
	repo.owners[grokVideoOwnerRepoKey(requestID, userID, apiKeyID, groupID)] = stored
	resolved, err = restarted.ResolveGrokMediaVideoRequestAccount(ctx, &groupID, requestID, userID, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, ownerID, resolved)
	renewed := repo.owners[grokVideoOwnerRepoKey(requestID, userID, apiKeyID, groupID)]
	require.WithinDuration(t, time.Now().Add(grokMediaVideoRequestOwnerRecoveryWindow), renewed.ExpiresAt, 2*time.Second)

	otherGroup := int64(8)
	_, err = restarted.ResolveGrokMediaVideoRequestAccount(ctx, &otherGroup, requestID, userID, apiKeyID)
	require.ErrorIs(t, err, ErrGrokMediaVideoRequestOwnerNotFound)
	_, err = restarted.ResolveGrokMediaVideoRequestAccount(ctx, &groupID, requestID, userID, apiKeyID+1)
	require.ErrorIs(t, err, ErrGrokMediaVideoRequestOwnerNotFound)
}

func TestGrokMediaVideoOwnerTerminalRetentionAndBoundedCleanup(t *testing.T) {
	groupID := int64(7)
	repo := &grokVideoOwnerRepoStub{owners: map[string]GrokMediaVideoRequestOwner{}}
	svc := &OpenAIGatewayService{grokMediaVideoOwnerRepo: repo}
	require.NoError(t, svc.BindGrokMediaVideoRequestAccount(context.Background(), &groupID, "video-terminal", 41, 51, 63))
	require.NoError(t, svc.MarkGrokMediaVideoRequestTerminal(context.Background(), &groupID, "video-terminal", 41, 51))

	owner := repo.owners[grokVideoOwnerRepoKey("video-terminal", 41, 51, groupID)]
	require.NotNil(t, owner.TerminalAt)
	require.WithinDuration(t, time.Now().Add(grokMediaVideoTerminalRetention), owner.ExpiresAt, 2*time.Second)
	require.Equal(t, grokMediaExpiredCleanupBatchSize, repo.cleanupLimit)
}

func TestGrokMediaVideoOwnerCacheFailureDoesNotUndoDurableBind(t *testing.T) {
	groupID := int64(9)
	repo := &grokVideoOwnerRepoStub{}
	svc := &OpenAIGatewayService{
		cache:                   &grokVideoOwnerCacheStub{setErr: errors.New("redis unavailable")},
		grokMediaVideoOwnerRepo: repo,
	}
	require.NoError(t, svc.BindGrokMediaVideoRequestAccount(context.Background(), &groupID, "video", 1, 2, 3))
	require.NotEmpty(t, repo.owners)
}

func TestGrokMediaImageEndpointsNeverUseVideoOwnerBinding(t *testing.T) {
	require.False(t, GrokMediaEndpointImagesGenerations.IsVideoGenerationRequest())
	require.False(t, GrokMediaEndpointImagesEdits.IsVideoGenerationRequest())
	require.True(t, GrokMediaEndpointVideosGenerations.IsVideoGenerationRequest())
	require.True(t, GrokMediaEndpointVideosEdits.IsVideoGenerationRequest())
	require.True(t, GrokMediaEndpointVideosExtensions.IsVideoGenerationRequest())
}

func TestGrokMediaVideoResponseBufferedUntilOwnerCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	upstream := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"upstream-1"}},
	}
	bufferGrokMediaResponse(c, upstream, []byte(`{"request_id":"video-1"}`), nil)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())

	require.NoError(t, CommitBufferedGrokMediaResponse(c))
	require.True(t, c.Writer.Written())
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, `{"request_id":"video-1"}`, recorder.Body.String())
}

type grokVideoIdempotentUpstream struct {
	requests      int
	uniqueCreates int
	byKey         map[string]string
}

func (u *grokVideoIdempotentUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.requests++
	key := req.Header.Get("Idempotency-Key")
	requestID, ok := u.byKey[key]
	if !ok {
		u.uniqueCreates++
		requestID = fmt.Sprintf("video-external-%d", u.uniqueCreates)
		u.byKey[key] = requestID
	}
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"request_id":%q}`, requestID))),
	}, nil
}

func (u *grokVideoIdempotentUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestGrokMediaVideoCreateIdempotencyClosesAcceptedBeforePersistenceCrashGap(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	repo := &grokVideoOwnerRepoStub{}
	upstream := &grokVideoIdempotentUpstream{byKey: make(map[string]string)}
	groupID := int64(7)
	account := &Account{
		ID: 63, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://xai.test/v1"},
	}
	body := []byte(`{"model":"grok-imagine-video","prompt":"waves","duration":10}`)
	svc := &OpenAIGatewayService{grokMediaVideoOwnerRepo: repo, httpUpstream: upstream}
	first, err := svc.ClaimGrokMediaVideoCreate(
		context.Background(), &groupID, GrokMediaEndpointVideosGenerations,
		"wj-video-42", "application/json; charset=utf-8", body, 41, 51,
	)
	require.NoError(t, err)
	boundID, err := svc.BindGrokMediaVideoCreateAccount(context.Background(), first, account.ID)
	require.NoError(t, err)
	require.Equal(t, account.ID, boundID)

	firstRecorder := httptest.NewRecorder()
	firstContext, _ := gin.CreateTestContext(firstRecorder)
	firstContext.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewReader(body))
	SetGrokMediaUpstreamIdempotencyKey(firstContext, first.UpstreamIdempotencyKey)
	firstResult, err := svc.ForwardGrokMedia(
		context.Background(), firstContext, account, GrokMediaEndpointVideosGenerations, "", body, "application/json",
	)
	require.NoError(t, err)
	require.Equal(t, "video-external-1", firstResult.ResponseID)
	require.Equal(t, 1, upstream.uniqueCreates)

	// Simulate process loss after upstream acceptance but before the response and
	// owner transaction commits. Fifteen minutes later, a fresh service claims
	// the same canonical request and replays the same derived upstream key on the
	// already-persisted account.
	stored := repo.creates[grokVideoCreateRepoKey(*first)]
	stored.ExpiresAt = time.Now().Add(grokMediaVideoCreateIdempotencyTTL - 15*time.Minute)
	repo.creates[grokVideoCreateRepoKey(*first)] = stored
	restarted := &OpenAIGatewayService{grokMediaVideoOwnerRepo: repo, httpUpstream: upstream}
	reorderedBody := []byte(`{"duration":10,"prompt":"waves","model":"grok-imagine-video"}`)
	recovered, err := restarted.ClaimGrokMediaVideoCreate(
		context.Background(), &groupID, GrokMediaEndpointVideosGenerations,
		"wj-video-42", "application/json", reorderedBody, 41, 51,
	)
	require.NoError(t, err)
	require.Equal(t, account.ID, recovered.AccountID)
	require.Equal(t, first.RequestHash, recovered.RequestHash)
	require.Equal(t, first.UpstreamIdempotencyKey, recovered.UpstreamIdempotencyKey)

	secondRecorder := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(secondRecorder)
	secondContext.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewReader(reorderedBody))
	SetGrokMediaUpstreamIdempotencyKey(secondContext, recovered.UpstreamIdempotencyKey)
	secondResult, err := restarted.ForwardGrokMedia(
		context.Background(), secondContext, account, GrokMediaEndpointVideosGenerations, "", reorderedBody, "application/json",
	)
	require.NoError(t, err)
	require.Equal(t, firstResult.ResponseID, secondResult.ResponseID)
	require.Equal(t, 2, upstream.requests)
	require.Equal(t, 1, upstream.uniqueCreates, "upstream create must remain exactly once")

	status, responseContentType, responseBody, err := BufferedGrokMediaResponse(secondContext)
	require.NoError(t, err)
	require.NoError(t, restarted.CompleteGrokMediaVideoCreate(
		context.Background(), recovered, secondResult.ResponseID, account.ID,
		status, responseContentType, responseBody,
	))
	require.Len(t, repo.owners, 1)
	require.Equal(t, account.ID, repo.owners[grokVideoOwnerRepoKey(secondResult.ResponseID, 41, 51, groupID)].AccountID)

	completed, err := restarted.ClaimGrokMediaVideoCreate(
		context.Background(), &groupID, GrokMediaEndpointVideosGenerations,
		"wj-video-42", "application/json", body, 41, 51,
	)
	require.NoError(t, err)
	require.True(t, completed.Completed())
	replayRecorder := httptest.NewRecorder()
	replayContext, _ := gin.CreateTestContext(replayRecorder)
	require.NoError(t, WriteGrokMediaVideoCreateReplay(replayContext, completed))
	require.Equal(t, http.StatusAccepted, replayRecorder.Code)
	require.Contains(t, replayRecorder.Body.String(), secondResult.ResponseID)
	require.Equal(t, "true", replayRecorder.Header().Get("Idempotent-Replayed"))

	_, err = restarted.ClaimGrokMediaVideoCreate(
		context.Background(), &groupID, GrokMediaEndpointVideosGenerations,
		"wj-video-42", "application/json", []byte(`{"model":"grok-imagine-video","prompt":"different"}`), 41, 51,
	)
	require.ErrorIs(t, err, ErrGrokMediaVideoIdempotencyConflict)

	otherCaller, err := restarted.ClaimGrokMediaVideoCreate(
		context.Background(), &groupID, GrokMediaEndpointVideosGenerations,
		"wj-video-42", "application/json", body, 42, 51,
	)
	require.NoError(t, err)
	require.NotEqual(t, completed.UpstreamIdempotencyKey, otherCaller.UpstreamIdempotencyKey)
}

func TestSelectGrokMediaVideoRequestOwnerBusyNeverSwitches(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9)
	owner := &Account{ID: 11, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 5, GroupIDs: []int64{groupID}}
	other := &Account{ID: 22, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 5, GroupIDs: []int64{groupID}}
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{owner.ID: owner, other.ID: other}}
	acquiredIDs := make([]int64, 0, 1)
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 90 * time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 2
	svc := &OpenAIGatewayService{
		cfg:                cfg,
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{owner.ID: false, other.ID: true}, acquiredIDs: &acquiredIDs}),
	}

	selection, err := svc.SelectGrokMediaVideoRequestOwner(ctx, &groupID, owner.ID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, selection.Account.ID)
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, owner.ID, selection.WaitPlan.AccountID)
	require.Equal(t, 1, selection.WaitPlan.MaxConcurrency)
	require.Equal(t, 90*time.Second, selection.WaitPlan.Timeout)
	require.Equal(t, []int64{owner.ID}, acquiredIDs)
}

func TestSelectGrokMediaVideoRequestOwnerUnavailable(t *testing.T) {
	groupID := int64(11)
	disabled := &Account{ID: 41, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusDisabled, Schedulable: false, GroupIDs: []int64{groupID}}
	svc := &OpenAIGatewayService{
		cfg:         &config.Config{RunMode: config.RunModeStandard},
		accountRepo: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{disabled.ID: disabled}},
	}

	_, err := svc.SelectGrokMediaVideoRequestOwner(context.Background(), &groupID, disabled.ID)
	require.True(t, errors.Is(err, ErrGrokMediaVideoRequestOwnerUnavailable))
}

func TestSelectGrokMediaVideoRequestOwnerStatusAndContentUseSameOwnerAndRelease(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10)
	owner := &Account{ID: 31, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 3, GroupIDs: []int64{groupID}}
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{owner.ID: owner}}
	releasedIDs := make([]int64, 0, 2)
	svc := &OpenAIGatewayService{
		cfg:                &config.Config{RunMode: config.RunModeStandard},
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{releasedIDs: &releasedIDs}),
	}

	statusSelection, err := svc.SelectGrokMediaVideoRequestOwner(ctx, &groupID, owner.ID)
	require.NoError(t, err)
	contentSelection, err := svc.SelectGrokMediaVideoRequestOwner(ctx, &groupID, owner.ID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, statusSelection.Account.ID)
	require.Equal(t, owner.ID, contentSelection.Account.ID)
	require.Equal(t, 1, statusSelection.Account.SchedulingSlotConcurrency())
	require.NotNil(t, statusSelection.ReleaseFunc)
	require.NotNil(t, contentSelection.ReleaseFunc)
	statusSelection.ReleaseFunc()
	contentSelection.ReleaseFunc()
	require.Equal(t, []int64{owner.ID, owner.ID}, releasedIDs)
}
