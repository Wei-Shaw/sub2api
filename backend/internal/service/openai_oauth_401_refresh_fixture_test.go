//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

const oauth401RequestBody = `{"model":"gpt-5.5","input":"hello","stream":false}`

const oauth401OKBody = `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`

func oauth401UnauthorizedResponse(rejected string) *http.Response {
	body := `{"error":{"message":"Invalid or expired token","type":"invalid_request_error","code":"invalid_api_key","rejected":"` + rejected + `"}}`
	return oauth401Response(http.StatusUnauthorized, body)
}

func oauth401Response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type oauth401BranchUpstream struct {
	calls atomic.Int32
}

func (u *oauth401BranchUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	if req.Header.Get("Authorization") == "Bearer A" {
		return oauth401UnauthorizedResponse("A"), nil
	}
	return oauth401Response(http.StatusOK, oauth401OKBody), nil
}

func (u *oauth401BranchUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

// oauth401Repo serializes the account state: the refresh attempt and the test
// goroutines touch it concurrently, including under -race.
type oauth401Repo struct {
	refreshAPIAccountRepo
	mu        sync.Mutex
	setErrors []string
	setErrIDs []int64
	// beforeGetByID, when set, runs once before the next read.
	beforeGetByID func()
	// onGetByID, when set, runs before every read.
	onGetByID func()
	// onSetError, when set, runs on every error write.
	onSetError func()
	// extraAccounts, when set, answers reads for these IDs before the primary account.
	extraAccounts map[int64]*Account
}

// GetByID returns a snapshot, like a database read: callers read it outside the lock.
func (r *oauth401Repo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if hook := r.onGetByID; hook != nil {
		hook()
	}
	if hook := r.beforeGetByID; hook != nil {
		r.beforeGetByID = nil
		hook()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if extra := r.extraAccounts[id]; extra != nil {
		return oauth401AccountSnapshot(extra), nil
	}
	account, err := r.refreshAPIAccountRepo.GetByID(ctx, id)
	if account == nil {
		return nil, err
	}
	return oauth401AccountSnapshot(account), err
}

func oauth401AccountSnapshot(account *Account) *Account {
	snapshot := *account
	snapshot.Credentials = shallowCopyMap(account.Credentials)
	if account.ProxyID != nil {
		proxyID := *account.ProxyID
		snapshot.ProxyID = &proxyID
	}
	return &snapshot
}

func (r *oauth401Repo) UpdateOpenAIOAuthCredentialsIfUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	credentials map[string]any,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refreshAPIAccountRepo.UpdateOpenAIOAuthCredentialsIfUnchanged(ctx, id, expectedCredentials, expectedProxyID, credentials)
}

func (r *oauth401Repo) SetError(_ context.Context, id int64, message string) error {
	r.mu.Lock()
	r.setErrors = append(r.setErrors, message)
	r.setErrIDs = append(r.setErrIDs, id)
	hook := r.onSetError
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (r *oauth401Repo) token() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.account.GetCredential("access_token")
}

func (r *oauth401Repo) getByIDCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refreshAPIAccountRepo.getByIDCalls
}

func (r *oauth401Repo) accountErrors() ([]string, []int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.setErrors...), append([]int64(nil), r.setErrIDs...)
}

func (r *oauth401Repo) accountErrorCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.setErrors)
}

func (r *oauth401Repo) setToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account.Credentials["access_token"] = token
}

// snapshot returns a copy to hold as the request's account, like a repository read.
func (r *oauth401Repo) snapshot() *Account {
	r.mu.Lock()
	defer r.mu.Unlock()
	return oauth401AccountSnapshot(r.account)
}

func (r *oauth401Repo) update(fn func(*Account)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(r.account)
}

// oauth401AlwaysUpstream answers 401 to every request and runs hook before the first one.
type oauth401AlwaysUpstream struct {
	calls atomic.Int32
	body  string
	hook  func()
}

func (u *oauth401AlwaysUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if u.calls.Add(1) == 1 && u.hook != nil {
		u.hook()
	}
	if u.body == "" {
		return oauth401UnauthorizedResponse("A"), nil
	}
	return oauth401Response(http.StatusUnauthorized, u.body), nil
}

func (u *oauth401AlwaysUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

// oauth401LockCache reports the refresh lock as held while held is set, or as unavailable
// while lockErr is set. onAcquire, when set, runs on every acquire.
type oauth401LockCache struct {
	refreshAPICacheStub
	held      atomic.Bool
	onAcquire func()
}

func (c *oauth401LockCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	acquired, err := !c.held.Load(), error(nil)
	if c.lockErr != nil {
		acquired, err = false, c.lockErr
	}
	// onAcquire runs once the attempt's result is settled, so a hook can release the lock
	// without turning this attempt into the acquirer.
	if hook := c.onAcquire; hook != nil {
		hook()
	}
	return acquired, err
}

// oauth401BlockingExecutor is refreshAPIExecutorStub whose Refresh blocks on release when
// it is set, until release is closed or ctx is done.
type oauth401BlockingExecutor struct {
	refreshAPIExecutorStub
	release chan struct{}
}

func (e *oauth401BlockingExecutor) Refresh(ctx context.Context, _ *Account) (map[string]any, error) {
	e.refreshCalls++
	if e.delay > 0 {
		time.Sleep(e.delay)
	}
	if e.onRefresh != nil {
		e.onRefresh()
	}
	if e.onRefreshContext != nil {
		e.onRefreshContext(ctx)
	}
	if e.release != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-e.release:
		}
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.credentials, nil
}

type oauth401Fixture struct {
	svc      *OpenAIGatewayService
	repo     *oauth401Repo
	provider *OpenAITokenProvider
	executor *oauth401BlockingExecutor
}

func newOAuth401Fixture(t *testing.T, upstream HTTPUpstream, cache GeminiTokenCache, account *Account) *oauth401Fixture {
	t.Helper()
	fixture := &oauth401Fixture{
		repo:     &oauth401Repo{refreshAPIAccountRepo: refreshAPIAccountRepo{account: account}},
		executor: &oauth401BlockingExecutor{refreshAPIExecutorStub: refreshAPIExecutorStub{credentials: map[string]any{"access_token": "B", "refresh_token": "rt-rotated"}}},
	}
	provider := NewOpenAITokenProvider(fixture.repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(fixture.repo, cache), fixture.executor)
	fixture.provider = provider
	fixture.svc = &OpenAIGatewayService{accountRepo: fixture.repo, openAITokenProvider: provider, httpUpstream: upstream}
	return fixture
}

func oauth401Request(t *testing.T, ctx context.Context, token string, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-request-marker", "keep")
	return req
}

func (f *oauth401Fixture) sendCtx(t *testing.T, ctx context.Context, account *Account, token string) (*http.Response, error) {
	t.Helper()
	return f.svc.doOpenAIUpstream(oauth401Request(t, ctx, token, oauth401RequestBody), "", account)
}

func (f *oauth401Fixture) send(t *testing.T, account *Account, token string) (*http.Response, error) {
	t.Helper()
	return f.sendCtx(t, context.Background(), account, token)
}

func oauth401Account(id int64) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":       "A",
			"refresh_token":      "rt",
			"expires_at":         time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339),
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
}

func oauth401ShadowAccount(id int64, parentID int64) *Account {
	return &Account{
		ID:              id,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		ParentAccountID: &parentID,
		Credentials:     map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}
}

// awaitOAuth401 waits for a signal the code under test is expected to send and fails the
// test instead of blocking the whole run when it never comes.
func awaitOAuth401(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func readOAuth401Body(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return string(body)
}

// oauth401EventLog records the order of the side effects of a failed refresh.
type oauth401EventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *oauth401EventLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *oauth401EventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// oauth401RecordingBlocker records block calls by account and reason.
type oauth401RecordingBlocker struct {
	log *oauth401EventLog
}

func (b *oauth401RecordingBlocker) BlockAccountScheduling(account *Account, _ time.Time, reason string) {
	b.log.add(fmt.Sprintf("block:%d:%s", account.ID, reason))
}

func (b *oauth401RecordingBlocker) ClearAccountSchedulingBlock(int64) {}

// oauth401RecordingCache records cache deletions, and acquires the refresh lock normally.
type oauth401RecordingCache struct {
	refreshAPICacheStub
	mu  sync.Mutex
	log *oauth401EventLog
}

func (c *oauth401RecordingCache) DeleteAccessToken(ctx context.Context, key string) error {
	c.log.add("delete:" + key)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshAPICacheStub.DeleteAccessToken(ctx, key)
}
