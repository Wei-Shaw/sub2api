//go:build unit

package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A terminal refresh failure blocks scheduling of the credential account before writing the
// account error, and drops the cached token afterwards. A shadow request still blocks the
// parent it resolved to.
func TestOpenAIOAuth401TerminalFailureBlocksThenDropsCache(t *testing.T) {
	parent := oauth401Account(10)
	shadow := oauth401ShadowAccount(20, parent.ID)
	events := &oauth401EventLog{}
	cache := &oauth401RecordingCache{refreshAPICacheStub: refreshAPICacheStub{lockResult: true}, log: events}
	fixture := newOAuth401Fixture(t, &oauth401AlwaysUpstream{}, cache, parent)
	fixture.provider.tokenCache = cache
	fixture.provider.SetAccountRuntimeBlocker(&oauth401RecordingBlocker{log: events})
	fixture.repo.onSetError = func() { events.add("set_error") }
	fixture.executor.err = errors.New("invalid_grant: the refresh token is no longer valid")

	resp, err := fixture.send(t, shadow, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	readOAuth401Body(t, resp)

	require.Equal(t, []string{
		"block:10:token_refresh_non_retryable",
		"set_error",
		"delete:" + OpenAITokenCacheKey(parent),
	}, events.snapshot())
	messages, ids := fixture.repo.accountErrors()
	require.Equal(t, []int64{10}, ids)
	require.Len(t, messages, 1)
	require.True(t, strings.HasPrefix(messages[0], "Token refresh failed (non-retryable): "), messages[0])
	require.Equal(t, OpenAITokenCacheKey(parent), cache.deleteKey)
}

// A retryable failure leaves scheduling, the account state and the token cache alone.
func TestOpenAIOAuth401RetryableFailureLeavesBlockerAndCacheAlone(t *testing.T) {
	account := oauth401Account(1)
	events := &oauth401EventLog{}
	cache := &oauth401RecordingCache{refreshAPICacheStub: refreshAPICacheStub{lockResult: true}, log: events}
	fixture := newOAuth401Fixture(t, &oauth401AlwaysUpstream{}, cache, account)
	fixture.provider.tokenCache = cache
	fixture.provider.SetAccountRuntimeBlocker(&oauth401RecordingBlocker{log: events})
	fixture.repo.onSetError = func() { events.add("set_error") }
	fixture.executor.err = errors.New("dial tcp 1.2.3.4:443: connection reset by peer")

	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	readOAuth401Body(t, resp)

	require.Empty(t, events.snapshot())
	require.Empty(t, fixture.repo.setErrors)
	require.Equal(t, 1, fixture.executor.refreshCalls)
	require.Equal(t, 0, cache.deleteCalls)
	require.Equal(t, "A", fixture.repo.token())
}

// A terminal failure returns the token another worker persisted instead of blocking or
// touching the account and its cache, and resends with it.
func TestOpenAIOAuth401TerminalFailureUsesStoredReplacement(t *testing.T) {
	account := oauth401Account(1)
	events := &oauth401EventLog{}
	cache := &oauth401RecordingCache{refreshAPICacheStub: refreshAPICacheStub{lockResult: true}, log: events}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		oauth401UnauthorizedResponse("A"),
		oauth401Response(http.StatusOK, oauth401OKBody),
	}}
	fixture := newOAuth401Fixture(t, upstream, cache, account)
	fixture.provider.tokenCache = cache
	fixture.provider.SetAccountRuntimeBlocker(&oauth401RecordingBlocker{log: events})
	fixture.repo.onSetError = func() { events.add("set_error") }
	fixture.executor.err = errors.New("invalid_grant: the refresh token is no longer valid")
	fixture.executor.onRefresh = func() { fixture.repo.setToken("B") }

	resp, err := fixture.send(t, account, "A")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readOAuth401Body(t, resp)

	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Bearer B", upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, events.snapshot())
	require.Empty(t, fixture.repo.setErrors)
	require.Equal(t, 0, cache.deleteCalls)
	require.Equal(t, "B", fixture.repo.token())
}
