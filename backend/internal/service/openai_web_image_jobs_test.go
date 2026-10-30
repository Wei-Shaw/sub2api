package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func jobTestKey(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text))) }
func jobTestStore(t *testing.T) *webImageJobStore {
	return &webImageJobStore{dir: t.TempDir(), active: make(map[string]*webImageRunningJob)}
}

func TestWebImageJobSurvivesClientDisconnectAndReplaysAfterRestart(t *testing.T) {
	store := jobTestStore(t)
	key := jobTestKey("turn one")
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fn := func(ctx context.Context) (chatgptwebimage.Result, error) {
		calls.Add(1)
		close(started)
		<-release
		require.NoError(t, ctx.Err())
		return chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: []byte("same image"), Format: "png"}}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := store.run(ctx, key, fn); done <- err }()
	<-started
	cancel()
	require.Equal(t, "image_generation_outcome_unknown", requireWebImageTestValue[*OpenAIImagesUpstreamError](t, (<-done)).Code)
	close(release)
	result, err := store.run(context.Background(), key, fn)
	require.NoError(t, err)
	require.Equal(t, []byte("same image"), result.Images[0].Bytes)
	restarted := &webImageJobStore{dir: store.dir, active: make(map[string]*webImageRunningJob)}
	result, err = restarted.run(context.Background(), key, fn)
	require.NoError(t, err)
	require.Equal(t, []byte("same image"), result.Images[0].Bytes)
	require.Equal(t, int32(1), calls.Load())
	info, err := os.Stat(filepath.Join(store.dir, key+".json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestWebImageJobCrashClaimAndBrokenRecordNeverRegenerate(t *testing.T) {
	for _, record := range []string{`{"state":"pending"}`, `{"state":`, ""} {
		t.Run(fmt.Sprint(len(record)), func(t *testing.T) {
			store := jobTestStore(t)
			key := jobTestKey("claimed")
			require.NoError(t, os.WriteFile(filepath.Join(store.dir, key+".json"), []byte(record), 0600))
			_, err := store.run(context.Background(), key, func(context.Context) (chatgptwebimage.Result, error) {
				t.Fatal("must not regenerate claimed image")
				return chatgptwebimage.Result{}, nil
			})
			require.Equal(t, "image_generation_outcome_unknown", requireWebImageTestValue[*OpenAIImagesUpstreamError](t, err).Code)
		})
	}
}

func TestWebImageJobConcurrentCallersGenerateOnce(t *testing.T) {
	store := jobTestStore(t)
	key := jobTestKey("concurrent")
	var calls atomic.Int32
	start := make(chan struct{})
	results := make(chan error, 8)
	fn := func(context.Context) (chatgptwebimage.Result, error) {
		calls.Add(1)
		<-start
		return chatgptwebimage.Result{}, nil
	}
	for i := 0; i < 8; i++ {
		go func() { _, err := store.run(context.Background(), key, fn); results <- err }()
	}
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond)
	close(start)
	for i := 0; i < 8; i++ {
		require.NoError(t, <-results)
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestWebImageJobRequestScopeAndCanonicalJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: 1})
	c.Request.Header.Set("session_id", "one")
	a, err := webImageJobKey(c, []byte(`{"model":"gpt-6-luna","input":"draw"}`))
	require.NoError(t, err)
	b, err := webImageJobKey(c, []byte(`{ "input": "draw", "model":"gpt-6-luna" }`))
	require.NoError(t, err)
	require.Equal(t, a, b)
	c.Set("api_key", &APIKey{ID: 2})
	otherUser, err := webImageJobKey(c, []byte(`{"model":"gpt-6-luna","input":"draw"}`))
	require.NoError(t, err)
	require.NotEqual(t, a, otherUser)
	c.Set("api_key", &APIKey{ID: 1})
	c.Request.Header.Set("session_id", "two")
	otherSession, err := webImageJobKey(c, []byte(`{"model":"gpt-6-luna","input":"draw"}`))
	require.NoError(t, err)
	require.NotEqual(t, a, otherSession)
	c.Request.Header.Set("thread-id", "current-client-thread")
	thread, err := webImageJobKey(c, []byte(`{"model":"gpt-6-luna","input":"draw"}`))
	require.NoError(t, err)
	c.Request.Header.Set("session-id", "another-connection")
	reconnected, err := webImageJobKey(c, []byte(`{"model":"gpt-6-luna","input":"draw"}`))
	require.NoError(t, err)
	require.Equal(t, thread, reconnected)
	c.Request.Header.Set("thread-id", "new-chat")
	newChat, err := webImageJobKey(c, []byte(`{"model":"gpt-6-luna","input":"draw"}`))
	require.NoError(t, err)
	require.NotEqual(t, thread, newChat)
}

func TestWebImageJobRetainsPublicFailureWithoutRetry(t *testing.T) {
	store := jobTestStore(t)
	key := jobTestKey("failed")
	calls := 0
	fn := func(context.Context) (chatgptwebimage.Result, error) {
		calls++
		return chatgptwebimage.Result{}, &chatgptwebimage.Error{Accepted: true, Kind: "timeout", Stage: "poll"}
	}
	for i := 0; i < 2; i++ {
		_, err := store.run(context.Background(), key, fn)
		require.Equal(t, "image_generation_outcome_unknown", requireWebImageTestValue[*OpenAIImagesUpstreamError](t, err).Code)
	}
	require.Equal(t, 1, calls)
}

func completedJobBillingRef(t *testing.T, store *webImageJobStore, key string) *webImageJobBillingRef {
	t.Helper()
	_, err := store.run(context.Background(), key, func(context.Context) (chatgptwebimage.Result, error) {
		return chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: []byte("image"), Format: "png"}}}, nil
	})
	require.NoError(t, err)
	ref, err := store.billingRef(key)
	require.NoError(t, err)
	return ref
}

func TestWebImageJobDeliveryBillsOnceAcrossConcurrencyAndRestart(t *testing.T) {
	store := jobTestStore(t)
	ref := completedJobBillingRef(t, store, jobTestKey("billing"))
	var calls atomic.Int32
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { results <- store.billOnce(ref, func() error { calls.Add(1); return nil }) }()
	}
	for i := 0; i < 8; i++ {
		require.NoError(t, <-results)
	}
	require.Equal(t, int32(1), calls.Load())
	restarted := &webImageJobStore{dir: store.dir, active: make(map[string]*webImageRunningJob)}
	ref, err := restarted.billingRef(ref.key)
	require.NoError(t, err)
	require.NoError(t, restarted.billOnce(ref, func() error { t.Fatal("cached image must not be billed again"); return nil }))
}

func TestWebImageJobGenerationDoesNotClaimBillingAndFailureCanRetry(t *testing.T) {
	store := jobTestStore(t)
	ref := completedJobBillingRef(t, store, jobTestKey("disconnected-before-delivery"))
	// Background generation alone does not bill: a retry after a disconnected
	// original request must still pass through the normal first delivery bill.
	record, err := readWebImageJob(filepath.Join(store.dir, ref.key+".json"))
	require.NoError(t, err)
	require.False(t, record.Billed)
	attempts := 0
	require.EqualError(t, store.billOnce(ref, func() error { attempts++; return errors.New("database unavailable") }), "database unavailable")
	require.NoError(t, store.billOnce(ref, func() error { attempts++; return nil }))
	require.NoError(t, store.billOnce(ref, func() error { t.Fatal("second delivery must skip billing"); return nil }))
	require.Equal(t, 2, attempts)
}

func TestWebImageJobExpiryCreatesNewBillingGeneration(t *testing.T) {
	store := jobTestStore(t)
	key := jobTestKey("same-request-after-retention")
	old := completedJobBillingRef(t, store, key)
	require.NoError(t, store.billOnce(old, func() error { return nil }))
	past := time.Now().Add(-webImageJobRetention - time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(store.dir, key+".json"), past, past))
	fresh := completedJobBillingRef(t, store, key)
	require.NotEqual(t, old.billingID, fresh.billingID)
	require.Error(t, store.billOnce(old, func() error { t.Fatal("stale billing ref must not claim new image"); return nil }))
	called := false
	require.NoError(t, store.billOnce(fresh, func() error { called = true; return nil }))
	require.True(t, called)
}

func TestWebImageJobBillingContextIsInternalAndReplayedUsageSkipsEntirePath(t *testing.T) {
	store := jobTestStore(t)
	ref := completedJobBillingRef(t, store, jobTestKey("internal-context"))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set("X-Sub2API-Web-Image-Billing", ref.billingID)
	result := &OpenAIForwardResult{RequestID: "upstream-attempt"}
	applyChatGPTWebImageJobBilling(c, result)
	require.Nil(t, result.webImageJobBilling)
	require.Equal(t, "upstream-attempt", result.RequestID)
	c.Set(webImageJobBillingContextKey, ref)
	applyChatGPTWebImageJobBilling(c, result)
	require.Equal(t, ref.billingID, result.RequestID)
	require.Same(t, ref, result.webImageJobBilling)
	require.NoError(t, store.billOnce(ref, func() error { return nil }))
	// No API key/account/repositories are supplied. Any attempt to enter the
	// normal usage/account-health/billing path would fail instead of skipping.
	require.NoError(t, (&OpenAIGatewayService{}).RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result}))
}

func TestWebImageJobStableLedgerIdentitySurvivesPlannerAndAccountChanges(t *testing.T) {
	ref := completedJobBillingRef(t, jobTestStore(t), jobTestKey("ledger-recovery"))
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "new-client-retry")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "different-server-attempt")
	require.Equal(t, ref.billingID, resolveUsageBillingRequestID(ctx, ref.billingID))
	params := &postUsageBillingParams{
		webImageBillingID: ref.billingID,
		Cost:              &CostBreakdown{ActualCost: 1, TotalCost: 1},
		User:              &User{ID: 1}, APIKey: &APIKey{ID: 2}, Account: &Account{ID: 3, Type: AccountTypeOAuth},
	}
	first := buildUsageBillingCommand(ref.billingID, &UsageLog{Model: "gpt-6-luna", ImageCount: 1, InputTokens: 100}, params)
	params.Account = &Account{ID: 4, Type: AccountTypeOAuth}
	params.Cost = &CostBreakdown{ActualCost: 2, TotalCost: 2}
	retry := buildUsageBillingCommand(ref.billingID, &UsageLog{Model: "gpt-6-luna", ImageCount: 1, InputTokens: 250}, params)
	require.Equal(t, first.RequestID, retry.RequestID)
	require.Equal(t, first.RequestFingerprint, retry.RequestFingerprint)
	params.APIKey = &APIKey{ID: 5}
	other := buildUsageBillingCommand(ref.billingID, &UsageLog{ImageCount: 1}, params)
	require.NotEqual(t, first.RequestFingerprint, other.RequestFingerprint)
	// Ordinary requests retain payload-sensitive money-event fingerprinting.
	params.APIKey = &APIKey{ID: 2}
	ordinary := buildUsageBillingCommand("ordinary", &UsageLog{InputTokens: 100}, params)
	changed := buildUsageBillingCommand("ordinary", &UsageLog{InputTokens: 101}, params)
	require.NotEqual(t, ordinary.RequestFingerprint, changed.RequestFingerprint)
	// Even a valid-looking upstream prefix cannot request the special
	// fingerprint without the server's internal job ref in billing params.
	params.webImageBillingID = ""
	untrusted := buildUsageBillingCommand(ref.billingID, &UsageLog{InputTokens: 100}, params)
	untrustedChanged := buildUsageBillingCommand(ref.billingID, &UsageLog{InputTokens: 101}, params)
	require.NotEqual(t, untrusted.RequestFingerprint, untrustedChanged.RequestFingerprint)
}

func TestWebImageJobPendingBillingIdentityStillReservesQuota(t *testing.T) {
	store := jobTestStore(t)
	for i := 0; i < int(webImageJobMaxDiskBytes/webImageJobMaxFileBytes); i++ {
		// A full-sized new claim includes billing metadata and is > 64 bytes.
		claim := webImageJobRecord{State: "pending", BillingID: "web-image:00000000-0000-4000-8000-000000000000"}
		require.NoError(t, writeWebImageJob(filepath.Join(store.dir, jobTestKey(fmt.Sprint(i))+".json"), claim))
	}
	require.ErrorContains(t, store.makeRoom(), "storage full")
}
