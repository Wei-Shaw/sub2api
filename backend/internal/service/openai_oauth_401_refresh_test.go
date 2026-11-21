//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuth401ResendHandling(t *testing.T) {
	unauthorized := func(rejected string) []*http.Response {
		return []*http.Response{oauth401UnauthorizedResponse(rejected)}
	}
	ok := func() *http.Response { return oauth401Response(http.StatusOK, oauth401OKBody) }

	cases := []struct {
		name         string
		storedToken  string
		body         string
		responses    []*http.Response
		replay       bool
		executorErr  error
		wantSetError bool
		wantStatus   int
		wantRequests int
		wantSecondB  string
		wantBody     string
		wantExecutor int
		wantStored   string
	}{
		{name: "refreshes and resends", storedToken: "A", body: oauth401RequestBody, responses: []*http.Response{oauth401UnauthorizedResponse("A"), ok()}, replay: true, wantStatus: http.StatusOK, wantRequests: 2, wantSecondB: "Bearer B", wantExecutor: 1, wantStored: "B"},
		{name: "resends a streaming body", storedToken: "A", body: `{"model":"gpt-5.5","input":"hello","stream":true}`, responses: []*http.Response{oauth401UnauthorizedResponse("A"), ok()}, replay: true, wantStatus: http.StatusOK, wantRequests: 2, wantSecondB: "Bearer B", wantExecutor: 1, wantStored: "B"},
		{name: "second attempt unauthorized", storedToken: "A", responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401UnauthorizedResponse("B")}, replay: true, wantStatus: http.StatusUnauthorized, wantRequests: 2, wantSecondB: "Bearer B", wantBody: `"rejected":"B"`, wantExecutor: 1, wantStored: "B"},
		{name: "stored token already replaced", storedToken: "B", responses: []*http.Response{oauth401UnauthorizedResponse("A"), ok()}, replay: true, wantStatus: http.StatusOK, wantRequests: 2, wantSecondB: "Bearer B", wantStored: "B"},
		{name: "request body not replayable", storedToken: "A", responses: unauthorized("A"), wantStatus: http.StatusUnauthorized, wantRequests: 1, wantBody: `"rejected":"A"`, wantStored: "A"},
		{name: "invalid_grant", storedToken: "A", replay: true, executorErr: errors.New("invalid_grant: the refresh token is no longer valid"), responses: unauthorized("A"), wantSetError: true, wantStatus: http.StatusUnauthorized, wantRequests: 1, wantBody: `"rejected":"A"`, wantExecutor: 1, wantStored: "A"},
		{name: "refresh_token_reused", storedToken: "A", replay: true, executorErr: errors.New("refresh_token_reused: the refresh token is no longer valid"), responses: unauthorized("A"), wantSetError: true, wantStatus: http.StatusUnauthorized, wantRequests: 1, wantExecutor: 1, wantStored: "A"},
		{name: "refresh_token_invalidated", storedToken: "A", replay: true, executorErr: errors.New("refresh_token_invalidated: the refresh token is no longer valid"), responses: unauthorized("A"), wantSetError: true, wantStatus: http.StatusUnauthorized, wantRequests: 1, wantExecutor: 1, wantStored: "A"},
		{name: "retryable network error", storedToken: "A", replay: true, executorErr: errors.New("dial tcp 1.2.3.4:443: connection reset by peer"), responses: unauthorized("A"), wantStatus: http.StatusUnauthorized, wantRequests: 1, wantExecutor: 1, wantStored: "A"},
		{name: "shared provider error", storedToken: "A", replay: true, executorErr: errors.New("invalid_client: token endpoint rejected the client_id"), responses: unauthorized("A"), wantStatus: http.StatusUnauthorized, wantRequests: 1, wantExecutor: 1, wantStored: "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := oauth401Account(1)
			account.Credentials["access_token"] = tc.storedToken
			upstream := &httpUpstreamRecorder{responses: tc.responses}
			fixture := newOAuth401Fixture(t, upstream, nil, account)
			fixture.executor.err = tc.executorErr

			body := tc.body
			if body == "" {
				body = oauth401RequestBody
			}
			req := oauth401Request(t, context.Background(), "A", body)
			if !tc.replay {
				req.GetBody = nil
			}
			resp, err := fixture.svc.doOpenAIUpstream(req, "", account)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Len(t, upstream.requests, tc.wantRequests)
			require.Equal(t, tc.wantStatus, resp.StatusCode)
			respBody := readOAuth401Body(t, resp)
			if tc.wantBody != "" {
				require.Contains(t, respBody, tc.wantBody)
			}
			if tc.wantSecondB != "" {
				require.Equal(t, tc.wantSecondB, upstream.requests[1].Header.Get("Authorization"))
				require.Equal(t, []byte(body), upstream.bodies[1])
				require.Equal(t, "keep", upstream.requests[1].Header.Get("x-request-marker"))
			}
			require.Equal(t, tc.wantExecutor, fixture.executor.refreshCalls)
			require.Equal(t, tc.wantStored, fixture.repo.token())

			messages, ids := fixture.repo.setErrors, fixture.repo.setErrIDs
			if !tc.wantSetError {
				require.Empty(t, messages)
				return
			}
			require.Equal(t, []int64{1}, ids)
			require.Len(t, messages, 1)
			require.True(t, strings.HasPrefix(messages[0], "Token refresh failed (non-retryable): "), messages[0])
			require.Contains(t, messages[0], tc.executorErr.Error())
		})
	}
}

func TestOpenAIOAuth401SkipsWithoutRetry(t *testing.T) {
	version := time.Now().Add(-time.Minute).UnixMilli()
	earlier := time.UnixMilli(version - 1000)
	later := time.UnixMilli(version + 1000)

	cases := []struct {
		name         string
		account      func(*Account) *Account
		status       int
		body         string
		tokenVersion *int64
		lastUsedAt   *time.Time
		cooldownAt   *time.Time
		wantRefresh  bool
	}{
		{name: "api key", account: func(*Account) *Account {
			return &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Credentials: map[string]any{"api_key": "sk"}}
		}},
		{name: "personal access token", account: func(account *Account) *Account {
			account.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken
			return account
		}},
		{name: "agent identity", account: func(account *Account) *Account {
			account.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity
			return account
		}},
		{name: "oauth without refresh token", account: func(account *Account) *Account {
			delete(account.Credentials, "refresh_token")
			return account
		}},
		{name: "token invalidated", body: `{"error":{"message":"token invalidated","code":"token_invalidated"}}`},
		{name: "token revoked", body: `{"error":{"message":"token revoked","code":"token_revoked"}}`},
		{name: "detail unauthorized", body: `{"detail":"Unauthorized"}`},
		{name: "insufficient scope", body: `{"error":{"message":"scope missing","code":"insufficient_scope"}}`},
		{name: "missing scopes", body: `{"error":{"message":"Missing scopes: api.model.read"}}`},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "too many requests", status: http.StatusTooManyRequests},
		{name: "internal server error", status: http.StatusInternalServerError},
		{name: "token minted without any success", tokenVersion: &version, wantRefresh: true},
		{name: "token minted after the last success", tokenVersion: &version, lastUsedAt: &earlier, wantRefresh: true},
		{name: "token minted after the last success and cooled down", tokenVersion: &version, lastUsedAt: &earlier, cooldownAt: &later},
		{name: "cooldown ended before the token was minted", tokenVersion: &version, lastUsedAt: &earlier, cooldownAt: &earlier, wantRefresh: true},
		{name: "success after the mint with a later cooldown", tokenVersion: &version, lastUsedAt: &later, cooldownAt: &later, wantRefresh: true},
		{name: "no token version refreshes", wantRefresh: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := oauth401Account(1)
			if tc.account != nil {
				account = tc.account(account)
			}
			if tc.tokenVersion != nil {
				account.Credentials["_token_version"] = *tc.tokenVersion
			}
			account.LastUsedAt = tc.lastUsedAt
			account.TempUnschedulableUntil = tc.cooldownAt
			response := oauth401UnauthorizedResponse("A")
			if tc.body != "" || tc.status != 0 {
				status := tc.status
				if status == 0 {
					status = http.StatusUnauthorized
				}
				response = oauth401Response(status, tc.body)
			}
			responses := []*http.Response{response}
			if tc.wantRefresh {
				responses = append(responses, oauth401Response(http.StatusOK, oauth401OKBody))
			}
			upstream := &httpUpstreamRecorder{responses: responses}
			fixture := newOAuth401Fixture(t, upstream, nil, account)

			resp, err := fixture.send(t, account, "A")
			require.NoError(t, err)
			require.NotNil(t, resp)
			wantRequests := 1
			if tc.wantRefresh {
				wantRequests = 2
			}
			require.Len(t, upstream.requests, wantRequests)
			wantStatus := response.StatusCode
			if tc.wantRefresh {
				wantStatus = http.StatusOK
			}
			require.Equal(t, wantStatus, resp.StatusCode)
			body := readOAuth401Body(t, resp)
			if tc.wantRefresh {
				require.Equal(t, 1, fixture.executor.refreshCalls)
				return
			}
			if tc.body != "" {
				require.Equal(t, tc.body, body)
			}
			require.Equal(t, 0, fixture.executor.refreshCalls)
		})
	}
}

func TestOpenAIOAuth401ConcurrentRequestsRefreshOnce(t *testing.T) {
	account := oauth401Account(1)
	upstream := &oauth401BranchUpstream{}
	fixture := newOAuth401Fixture(t, upstream, nil, account)

	const goroutines = 10
	statuses := make([]int, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			// Each request holds its own account snapshot, like a repository read.
			resp, err := fixture.send(t, fixture.repo.snapshot(), "A")
			require.NoError(t, err)
			statuses[index] = resp.StatusCode
			readOAuth401Body(t, resp)
		}(i)
	}
	wg.Wait()

	require.Equal(t, 1, fixture.executor.refreshCalls)
	require.Equal(t, "B", fixture.repo.token())
	require.Equal(t, int32(2*goroutines), upstream.calls.Load())
	for _, status := range statuses {
		require.Equal(t, http.StatusOK, status)
	}
}

func TestOpenAIOAuth401HeldRefreshLock(t *testing.T) {
	account := oauth401Account(1)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401Response(http.StatusOK, oauth401OKBody)}}
	fixture := newOAuth401Fixture(t, upstream, &refreshAPICacheStub{lockResult: false}, account)
	// The other worker persists its token before the first read.
	fixture.repo.onGetByID = func() { fixture.repo.setToken("B") }

	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readOAuth401Body(t, resp)
	require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, 0, fixture.executor.refreshCalls)
}

func TestOpenAIOAuth401ClientCancel(t *testing.T) {
	t.Run("while waiting for the refresh lock", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A")}}
		fixture := newOAuth401Fixture(t, upstream, &refreshAPICacheStub{lockResult: false}, account)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		resp, err := fixture.sendCtx(t, ctx, account, "A")
		require.NoError(t, err)
		require.Less(t, time.Since(start), 250*time.Millisecond)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Len(t, upstream.requests, 1)
		require.Equal(t, 0, fixture.executor.refreshCalls)
	})

	t.Run("while the refresh runs", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401Response(http.StatusOK, oauth401OKBody)}}
		fixture := newOAuth401Fixture(t, upstream, nil, account)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entered := make(chan struct{})
		fixture.executor.onRefresh = func() { close(entered) }
		fixture.executor.release = make(chan struct{})
		var released sync.Once
		release := func() { released.Do(func() { close(fixture.executor.release) }) }
		time.AfterFunc(30*time.Second, release)
		go func() {
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
			}
			cancel()
		}()

		resp, err := fixture.sendCtx(t, ctx, account, "A")
		require.NoError(t, err)
		require.NotNil(t, resp)
		// The caller leaves at once and keeps the existing 401 handling.
		select {
		case <-fixture.executor.release:
			t.Fatal("the caller returned only after the refresh was released")
		default:
		}
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		readOAuth401Body(t, resp)
		// The refresh rotates the refresh_token at the provider, so the detached
		// attempt still persists it after the client left.
		release()
		require.Eventually(t, func() bool { return fixture.repo.token() == "B" }, 5*time.Second, 10*time.Millisecond)
		require.Equal(t, 1, fixture.executor.refreshCalls)
	})
}

func TestOpenAIOAuth401ShadowRefresh(t *testing.T) {
	parent := oauth401Account(10)
	shadow := oauth401ShadowAccount(20, parent.ID)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401Response(http.StatusOK, oauth401OKBody)}}
	fixture := newOAuth401Fixture(t, upstream, nil, parent)

	resp, err := fixture.send(t, shadow, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readOAuth401Body(t, resp)
	require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "B", fixture.repo.token())

	parent = oauth401Account(10)
	fixture = newOAuth401Fixture(t, &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A")}}, nil, parent)
	fixture.executor.err = errors.New("invalid_grant: refresh token already used")

	resp, err = fixture.send(t, shadow, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	readOAuth401Body(t, resp)
	require.Len(t, fixture.repo.setErrors, 1)
	require.Equal(t, []int64{10}, fixture.repo.setErrIDs)
}

// A success on the shadow counts as a use: the parent's LastUsedAt alone marks the
// rejected token as already rejected, the shadow succeeded after it was minted.
func TestOpenAIOAuth401ShadowSuccessCountsAsUse(t *testing.T) {
	minted := time.Now().Add(-time.Minute).UnixMilli()
	caution, usedByShadow := time.UnixMilli(minted+1000), time.UnixMilli(minted+2000)
	parent := oauth401Account(10)
	parent.Credentials["_token_version"] = minted
	parent.TempUnschedulableUntil = &caution
	shadow := oauth401ShadowAccount(20, parent.ID)
	shadow.LastUsedAt = &usedByShadow
	upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401Response(http.StatusOK, oauth401OKBody)}}
	fixture := newOAuth401Fixture(t, upstream, nil, parent)
	fixture.repo.extraAccounts = map[int64]*Account{shadow.ID: shadow}

	resp, err := fixture.send(t, shadow, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readOAuth401Body(t, resp)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, 1, fixture.executor.refreshCalls)
}

// A shadow is never a PAT or agent identity itself, its parent can be: the parent decides.
func TestOpenAIOAuth401ShadowParentAgentIdentity(t *testing.T) {
	parent := oauth401Account(10)
	parent.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity
	shadow := oauth401ShadowAccount(20, parent.ID)
	upstream := &oauth401AlwaysUpstream{}
	fixture := newOAuth401Fixture(t, upstream, nil, parent)
	fixture.repo.extraAccounts = map[int64]*Account{shadow.ID: shadow}

	resp, err := fixture.send(t, shadow, "A")
	require.NoError(t, err)
	require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Equal(t, 0, fixture.executor.refreshCalls)
}

func TestOpenAIOAuth401ChatCompletionsEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := oauth401Account(1)
	streamResponse := oauth401Response(http.StatusOK, strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}]},"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
	streamResponse.Header.Set("Content-Type", "text/event-stream")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), streamResponse}}
	fixture := newOAuth401Fixture(t, upstream, nil, account)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := fixture.svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok"`)
	require.Equal(t, "Bearer A", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
}

// A failed refresh is shared by every request that holds the same rejected token, and a
// caller that goes away does not wait for it.
func TestOpenAIOAuth401ConcurrentRequestsShareOneFailedRefresh(t *testing.T) {
	account := oauth401Account(1)
	upstream := &oauth401BranchUpstream{}
	fixture := newOAuth401Fixture(t, upstream, nil, account)
	fixture.executor.err = errors.New("dial tcp 104.18.0.1:443: i/o timeout")
	fixture.executor.release = make(chan struct{})

	const requests = 8
	sent := make([]*http.Request, requests)
	for i := range sent {
		sent[i] = oauth401Request(t, context.Background(), "A", oauth401RequestBody)
	}
	start := make(chan struct{})
	statuses := make([]int, requests)
	durations := make([]time.Duration, requests)
	var finished atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			request := fixture.repo.snapshot() // each request holds its own account snapshot
			begin := time.Now()
			resp, err := fixture.svc.doOpenAIUpstream(sent[index], "", request)
			durations[index] = time.Since(begin)
			finished.Add(1)
			require.NoError(t, err)
			statuses[index] = resp.StatusCode
			readOAuth401Body(t, resp)
		}(i)
	}
	close(start)

	// All requests reach the shared attempt, and none of them returns while it is blocked.
	require.Eventually(t, func() bool { return upstream.calls.Load() == requests }, 5*time.Second, 5*time.Millisecond)
	// Grace for the last caller's path from the 401 into the attempt.
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int32(0), finished.Load())
	close(fixture.executor.release)
	wg.Wait()

	require.Equal(t, int32(requests), finished.Load())
	require.Equal(t, 1, fixture.executor.refreshCalls)
	for i := range requests {
		require.Equal(t, http.StatusUnauthorized, statuses[i])
		require.Less(t, durations[i], 2*time.Second)
	}
	require.Equal(t, "A", fixture.repo.token())
}

func TestOpenAIOAuth401WaitingCallerLeavesOnClientDeadline(t *testing.T) {
	account := oauth401Account(1)
	upstream := &oauth401BranchUpstream{}
	fixture := newOAuth401Fixture(t, upstream, nil, account)
	fixture.executor.err = errors.New("dial tcp 104.18.0.1:443: i/o timeout")
	fixture.executor.release = make(chan struct{})
	entered := make(chan struct{})
	var enteredOnce sync.Once
	fixture.executor.onRefresh = func() { enteredOnce.Do(func() { close(entered) }) }
	leader := make(chan struct{})
	go func() {
		defer close(leader)
		resp, err := fixture.send(t, account, "A")
		if err == nil {
			readOAuth401Body(t, resp)
		}
	}()
	awaitOAuth401(t, entered, "the refresh to start")
	// Release from the background too, so a caller that ignores its cancellation cannot
	// deadlock the test.
	var released sync.Once
	release := func() { released.Do(func() { close(fixture.executor.release) }) }
	time.AfterFunc(30*time.Second, release)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	resp, err := fixture.sendCtx(t, ctx, account, "A")
	require.NoError(t, err)
	require.NotNil(t, resp)
	// It returned on its own deadline, while the refresh was still blocked.
	select {
	case <-fixture.executor.release:
		t.Fatal("the caller returned only after the refresh was released")
	default:
	}
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	readOAuth401Body(t, resp)

	release()
	awaitOAuth401(t, leader, "the leader request to finish")
	require.Equal(t, 1, fixture.executor.refreshCalls)
}

// A refresh lock held by another worker is read for its token at most
// openAILockMaxAttempts times; the request never refreshes itself a second time.
func TestOpenAIOAuth401WaitsOutHeldRefreshLock(t *testing.T) {
	// holdReads counts the held-lock reads, and optionally releases the lock on acquire.
	holdReads := func(fixture *oauth401Fixture, cache *oauth401LockCache, release bool) *atomic.Int32 {
		reads := &atomic.Int32{}
		var armed atomic.Bool
		cache.onAcquire = func() {
			armed.Store(true)
			cache.held.Store(!release)
		}
		fixture.repo.onGetByID = func() {
			if armed.Load() {
				reads.Add(1)
			}
		}
		return reads
	}

	t.Run("other worker holds the lock", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		cache := &oauth401LockCache{}
		cache.held.Store(true)
		fixture := newOAuth401Fixture(t, upstream, cache, account)
		reads := holdReads(fixture, cache, false)

		resp, err := fixture.send(t, account, "A")
		require.NoError(t, err)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Equal(t, int32(1), upstream.calls.Load())
		require.Equal(t, 0, fixture.executor.refreshCalls)
		require.Equal(t, int32(openAILockMaxAttempts), reads.Load())
		require.Empty(t, fixture.repo.setErrors)
	})

	t.Run("token persisted before the third read", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401Response(http.StatusOK, oauth401OKBody)}}
		cache := &oauth401LockCache{}
		cache.held.Store(true)
		fixture := newOAuth401Fixture(t, upstream, cache, account)
		reads := &atomic.Int32{}
		var armed atomic.Bool
		cache.onAcquire = func() { armed.Store(true) }
		fixture.repo.onGetByID = func() {
			if armed.Load() && reads.Add(1) == 3 {
				fixture.repo.setToken("B")
			}
		}

		resp, err := fixture.send(t, account, "A")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		readOAuth401Body(t, resp)
		require.Len(t, upstream.requests, 2)
		require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
		require.Equal(t, int32(3), reads.Load())
		require.Equal(t, 0, fixture.executor.refreshCalls)
	})

	t.Run("other worker persists a token into a disabled account", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &httpUpstreamRecorder{responses: []*http.Response{oauth401UnauthorizedResponse("A"), oauth401Response(http.StatusOK, oauth401OKBody)}}
		cache := &oauth401LockCache{}
		cache.held.Store(true)
		fixture := newOAuth401Fixture(t, upstream, cache, account)
		// The other worker stores the new token while the lock is held and the account
		// stops being active before the stored token is read.
		fixture.repo.beforeGetByID = func() {
			fixture.repo.update(func(account *Account) { account.Status, account.Credentials["access_token"] = StatusDisabled, "B" })
			cache.held.Store(false)
		}

		resp, err := fixture.send(t, account, "A")
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Len(t, upstream.requests, 1)
		require.Equal(t, 0, fixture.executor.refreshCalls)
		require.Empty(t, fixture.repo.setErrors)
	})

	t.Run("lock released without a new token", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		cache := &oauth401LockCache{}
		cache.held.Store(true)
		fixture := newOAuth401Fixture(t, upstream, cache, account)
		reads := holdReads(fixture, cache, true)

		resp, err := fixture.send(t, account, "A")
		require.NoError(t, err)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Equal(t, int32(1), upstream.calls.Load())
		require.False(t, cache.held.Load())
		require.Equal(t, 0, fixture.executor.refreshCalls)
		require.Equal(t, int32(openAILockMaxAttempts), reads.Load())
		require.Empty(t, fixture.repo.setErrors)
	})

	t.Run("client cancel while waiting", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		cache := &oauth401LockCache{}
		cache.held.Store(true)
		fixture := newOAuth401Fixture(t, upstream, cache, account)

		leader := make(chan struct{})
		go func() {
			defer close(leader)
			resp, err := fixture.send(t, account, "A")
			if err == nil {
				readOAuth401Body(t, resp)
			}
		}()
		// The first request is waiting on the held lock.
		require.Eventually(t, func() bool { return fixture.repo.getByIDCount() > 0 }, 5*time.Second, 5*time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		resp, err := fixture.sendCtx(t, ctx, account, "A")
		require.NoError(t, err)
		require.True(t, cache.held.Load(), "the caller returned while the lock was still held")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)

		// The waiting request finishes its bounded reads; the released lock alone does
		// not make it refresh again.
		cache.held.Store(false)
		awaitOAuth401(t, leader, "the waiting request to finish")
		require.Equal(t, int32(2), upstream.calls.Load())
		require.Equal(t, 0, fixture.executor.refreshCalls)
	})
}

// A lock backend error on the 401 path fails the attempt instead of degrading to an
// unlocked refresh: the request keeps the existing 401 handling, without an account error.
func TestOpenAIOAuth401LockBackendError(t *testing.T) {
	account := oauth401Account(1)
	upstream := &oauth401AlwaysUpstream{}
	cache := &oauth401LockCache{refreshAPICacheStub: refreshAPICacheStub{lockErr: errors.New("redis: connection refused")}}
	fixture := newOAuth401Fixture(t, upstream, cache, account)

	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Equal(t, 0, fixture.executor.refreshCalls)
	require.Equal(t, 0, fixture.repo.accountErrorCount())
}

// The lock requirement only changes the 401 path: without the marker a lock backend error
// still degrades to the unlocked refresh in the same API.
func TestOpenAIOAuth401LockRequiredMarker(t *testing.T) {
	lockErr := &oauth401LockCache{refreshAPICacheStub: refreshAPICacheStub{lockErr: errors.New("redis: connection refused")}}
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		degrades bool
	}{
		{name: "marked returns the lock error", ctx: withOAuthRefreshLockRequired(context.Background())},
		{name: "unmarked still degrades", ctx: context.Background(), degrades: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newOAuth401Fixture(t, &oauth401AlwaysUpstream{}, lockErr, oauth401Account(1))
			fixture.executor.needsRefresh = true
			result, err := fixture.svc.openAITokenProvider.refreshAPI.RefreshIfNeeded(tc.ctx, fixture.repo.snapshot(), fixture.executor, 0)
			if !tc.degrades {
				require.Nil(t, result)
				require.ErrorIs(t, err, errOAuthRefreshLockUnavailable)
				require.Equal(t, 0, fixture.executor.refreshCalls)
				require.Equal(t, "A", fixture.repo.token())
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Refreshed)
			require.Equal(t, 1, fixture.executor.refreshCalls)
			require.Equal(t, "B", fixture.repo.token())
		})
	}
}

// A panic becomes an error for every caller: all keep the original 401, both locks stay usable.
func TestOpenAIOAuth401RefreshPanic(t *testing.T) {
	account := oauth401Account(1)
	upstream := &oauth401BranchUpstream{}
	cache := &refreshAPICacheStub{lockResult: true}
	fixture := newOAuth401Fixture(t, upstream, cache, account)
	release := make(chan struct{})
	fixture.executor.onRefresh = func() {
		<-release
		panic("boom")
	}
	const requests = 6
	var wg sync.WaitGroup
	var unauthorized atomic.Int32
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := fixture.send(t, fixture.repo.snapshot(), "A")
			require.NoError(t, err)
			if resp.StatusCode == http.StatusUnauthorized {
				unauthorized.Add(1)
			}
			readOAuth401Body(t, resp)
		}()
	}
	// Every request has reached the shared attempt: let it panic.
	require.Eventually(t, func() bool { return upstream.calls.Load() == requests }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	require.Equal(t, int32(requests), unauthorized.Load())
	require.GreaterOrEqual(t, fixture.executor.refreshCalls, 1)
	require.Equal(t, "A", fixture.repo.token())
	require.Equal(t, 0, fixture.repo.accountErrorCount())
	require.GreaterOrEqual(t, cache.releaseCalls, 1)

	// Both locks are free again: the next request refreshes and resends.
	fixture.executor.onRefresh = nil
	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readOAuth401Body(t, resp)
	require.Equal(t, "B", fixture.repo.token())
}

func TestOpenAIOAuth401AccessStateBodyKeepsHandling(t *testing.T) {
	account := oauth401Account(1)
	body := `{"error":{"message":"Your OpenAI account has been deactivated.","type":"invalid_request_error","code":"account_deactivated"}}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		oauth401Response(http.StatusUnauthorized, body),
		oauth401Response(http.StatusOK, oauth401OKBody),
	}}
	fixture := newOAuth401Fixture(t, upstream, nil, account)

	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.Equal(t, body, readOAuth401Body(t, resp))
	require.Len(t, upstream.requests, 1)
	require.Equal(t, 0, fixture.executor.refreshCalls)
}

// An account that stopped being active before the re-read under the lock is left to the
// existing 401 handling, not refreshed and not put in error - even when another refresh
// already stored a different token.
func TestOpenAIOAuth401AccountDisabledBeforeReread(t *testing.T) {
	account := oauth401Account(1)
	upstream := &oauth401AlwaysUpstream{}
	fixture := newOAuth401Fixture(t, upstream, nil, account)
	upstream.hook = func() {
		fixture.repo.update(func(account *Account) {
			account.Status = StatusDisabled
			account.Credentials["access_token"] = "B"
		})
	}

	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Equal(t, 0, fixture.executor.refreshCalls)
	require.Empty(t, fixture.repo.setErrors)
}

// A token is only resent through the proxy the request used.
func TestOpenAIOAuth401ProxyMustMatch(t *testing.T) {
	one, two := int64(1), int64(2)

	t.Run("shadow proxy differs from the owner's", func(t *testing.T) {
		parent := oauth401Account(10)
		parent.ProxyID = &one
		shadow := oauth401ShadowAccount(20, parent.ID)
		shadow.ProxyID = &two
		upstream := &httpUpstreamRecorder{responses: []*http.Response{
			oauth401UnauthorizedResponse("A"),
			oauth401Response(http.StatusOK, oauth401OKBody),
		}}
		fixture := newOAuth401Fixture(t, upstream, nil, parent)

		resp, err := fixture.send(t, shadow, "A")
		require.NoError(t, err)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Len(t, upstream.requests, 1)
		require.Equal(t, 0, fixture.executor.refreshCalls)
	})

	t.Run("proxy changed before the reread", func(t *testing.T) {
		stored := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		fixture := newOAuth401Fixture(t, upstream, nil, stored)
		upstream.hook = func() {
			fixture.repo.update(func(account *Account) { account.ProxyID = &two })
		}
		request := *stored // the request still holds the pre-401 snapshot

		resp, err := fixture.send(t, &request, "A")
		require.NoError(t, err)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Equal(t, int32(1), upstream.calls.Load())
		require.Equal(t, 0, fixture.executor.refreshCalls)
	})

	t.Run("proxy changed with a new token before the reread", func(t *testing.T) {
		stored := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		fixture := newOAuth401Fixture(t, upstream, nil, stored)
		upstream.hook = func() {
			fixture.repo.update(func(account *Account) {
				account.ProxyID = &two
				account.Credentials["access_token"] = "B"
			})
		}
		request := *stored // the request still holds the pre-401 snapshot

		resp, err := fixture.send(t, &request, "A")
		require.NoError(t, err)
		require.Contains(t, readOAuth401Body(t, resp), `"rejected":"A"`)
		require.Equal(t, int32(1), upstream.calls.Load())
		require.Equal(t, 0, fixture.executor.refreshCalls)
	})
}

// Without any cooldown, a token this path minted itself is refreshed once, not once per
// request: the upstream rejects it for good.
func TestOpenAIOAuth401RefreshesEachMintedTokenOnce(t *testing.T) {
	account := oauth401Account(1)
	account.Credentials["_token_version"] = time.Now().Add(-time.Hour).UnixMilli()
	used := time.Now().Add(-2 * time.Hour)
	account.LastUsedAt = &used
	upstream := &oauth401AlwaysUpstream{body: `{"error":{"message":"Provided authentication token is expired.","code":"token_expired"}}`}
	fixture := newOAuth401Fixture(t, upstream, nil, account)

	token := "A"
	for i := 0; i < 5; i++ {
		resp, err := fixture.send(t, account, token)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		readOAuth401Body(t, resp)
		token = fixture.repo.token()
	}

	require.Equal(t, 1, fixture.executor.refreshCalls)
	require.Equal(t, int32(6), upstream.calls.Load())
}

// An administrator refresh can rotate both tokens just before ours and make the refresh fail
// on the consumed token; the stored token is then resent instead of failing the account.
func TestOpenAIOAuth401AdminRefreshRace(t *testing.T) {
	for _, refreshError := range []string{
		"refresh_token_reused: the refresh token is no longer valid",
		"invalid_grant: the refresh token has already been used",
	} {
		t.Run(strings.SplitN(refreshError, ":", 2)[0], func(t *testing.T) {
			account := oauth401Account(1)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				oauth401UnauthorizedResponse("A"),
				oauth401Response(http.StatusOK, oauth401OKBody),
			}}
			fixture := newOAuth401Fixture(t, upstream, nil, account)
			fixture.executor.err = errors.New(refreshError)
			fixture.executor.onRefresh = func() {
				fixture.repo.update(func(account *Account) {
					account.Credentials["access_token"] = "B"
					account.Credentials["refresh_token"] = "rt-admin"
				})
			}

			resp, err := fixture.send(t, account, "A")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			readOAuth401Body(t, resp)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
			require.Empty(t, fixture.repo.setErrors)
			require.Equal(t, 1, fixture.executor.refreshCalls)
		})
	}
}

// The first-output timeout ends Forward promptly while the refresh keeps running on its
// detached context and still persists the rotation.
func TestOpenAIOAuth401FirstOutputTimeoutDoesNotCutRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const timeoutSeconds = 1
	account := oauth401Account(1)
	upstream := &oauth401AlwaysUpstream{body: `{"error":{"message":"Provided authentication token is expired.","code":"token_expired"}}`}
	fixture := newOAuth401Fixture(t, upstream, nil, account)
	fixture.svc.cfg = &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:                     defaultMaxLineSize,
		OpenAIFirstOutputTimeoutSeconds: timeoutSeconds,
	}}
	fixture.svc.rateLimitService = NewRateLimitService(fixture.repo, nil, fixture.svc.cfg, nil, nil)
	fixture.executor.release = make(chan struct{})
	var released sync.Once
	release := func() { released.Do(func() { close(fixture.executor.release) }) }
	time.AfterFunc(30*time.Second, release)
	fixture.executor.onRefreshContext = func(ctx context.Context) {
		_, hasDeadline := ctx.Deadline()
		require.False(t, hasDeadline, "the refresh must not run under a deadline")
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","input":"hello","stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	start := time.Now()
	_, err := fixture.svc.Forward(context.Background(), c, account, body)
	elapsed := time.Since(start)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "first_output_timeout")
	// Forward came back while the refresh was still blocked, not after it.
	select {
	case <-fixture.executor.release:
		t.Fatal("Forward returned only after the refresh was released")
	default:
	}
	require.Equal(t, int32(1), upstream.calls.Load(), "the refresh was cut off before a resend")
	fixture.repo.update(func(account *Account) {
		require.Nil(t, account.TempUnschedulableUntil, "the stream timeout path runs, not the 401 cooldown")
	})

	release()
	require.Eventually(t, func() bool { return fixture.repo.token() == "B" }, 5*time.Second, 10*time.Millisecond)
	t.Logf("returned after %s with status %d, upstream calls %d, stored token %q", elapsed, failoverErr.StatusCode, upstream.calls.Load(), fixture.repo.token())
}

// The terminal failure of a shared attempt belongs to the attempt, not to a caller that may
// already be gone, and is written to the account exactly once.
func TestOpenAIOAuth401TerminalFailureReportedOnce(t *testing.T) {
	t.Run("lone caller leaves before the attempt ends", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		fixture := newOAuth401Fixture(t, upstream, nil, account)
		fixture.executor.err = errors.New("invalid_grant: the refresh token is no longer valid")
		fixture.executor.release = make(chan struct{})
		var released sync.Once
		release := func() { released.Do(func() { close(fixture.executor.release) }) }
		time.AfterFunc(30*time.Second, release)

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		resp, err := fixture.sendCtx(t, ctx, account, "A")
		require.NoError(t, err)
		require.NotNil(t, resp)
		// It returned on its own deadline, while the refresh was still blocked.
		select {
		case <-fixture.executor.release:
			t.Fatal("the caller returned only after the refresh was released")
		default:
		}
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		readOAuth401Body(t, resp)

		release()
		require.Eventually(t, func() bool { return fixture.repo.accountErrorCount() == 1 }, 5*time.Second, 10*time.Millisecond)
		messages, ids := fixture.repo.accountErrors()
		require.Equal(t, []int64{1}, ids)
		require.True(t, strings.HasPrefix(messages[0], "Token refresh failed (non-retryable): "), messages[0])
	})

	t.Run("concurrent callers", func(t *testing.T) {
		account := oauth401Account(1)
		upstream := &oauth401AlwaysUpstream{}
		fixture := newOAuth401Fixture(t, upstream, nil, account)
		fixture.executor.err = errors.New("refresh_token_reused: the refresh token is no longer valid")
		fixture.executor.release = make(chan struct{})

		const requests = 8
		var finished atomic.Int32
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < requests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				resp, err := fixture.send(t, fixture.repo.snapshot(), "A")
				finished.Add(1)
				if err == nil {
					readOAuth401Body(t, resp)
				}
			}()
		}
		close(start)

		require.Eventually(t, func() bool { return upstream.calls.Load() == requests }, 5*time.Second, 5*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, int32(0), finished.Load())
		close(fixture.executor.release)
		wg.Wait()

		require.Equal(t, int32(requests), finished.Load())
		require.Equal(t, 1, fixture.executor.refreshCalls)
		messages, _ := fixture.repo.accountErrors()
		require.Len(t, messages, 1)
		require.True(t, strings.HasPrefix(messages[0], "Token refresh failed (non-retryable): "), messages[0])
	})
}
