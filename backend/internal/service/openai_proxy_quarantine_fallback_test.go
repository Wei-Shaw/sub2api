//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func quarantineFallbackFixture() (*OpenAIGatewayService, *Account) {
	proxy := proxyForTest(91, "primary.invalid", 8080)
	proxy.FallbackMode = FallbackModeDirect
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Proxy: proxy, ProxyID: i64(proxy.ID), Status: StatusActive, Schedulable: true}
	s := &OpenAIGatewayService{}
	s.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1, failureWindow: time.Minute, quarantineTTL: time.Minute, maxEntries: 16,
	})
	s.getOpenAIProxyStreamCircuit().recordFailure(proxy.ID, time.Now())
	return s, account
}

func TestQuarantineFallbackKeepsAccountWithAuthorizedEgress(t *testing.T) {
	s, account := quarantineFallbackFixture()
	require.False(t, s.isOpenAIProxyStreamQuarantined(context.Background(), account))
	require.True(t, s.isOpenAIProxyStreamQuarantined(
		withOpenAIProxyQuarantineTransport(context.Background(), OpenAIUpstreamTransportResponsesWebsocketV2Ingress), account),
		"native WebSocket selection must retain its existing circuit semantics")
	require.True(t, s.getOpenAIProxyStreamCircuit().isBlocked(account.Proxy.ID, time.Now()),
		"account eligibility must not heal the unhealthy egress")
	account.Proxy.FallbackMode = FallbackModeNone
	require.True(t, s.isOpenAIProxyStreamQuarantined(context.Background(), account))
	account.Proxy.FallbackMode = FallbackModeProxy
	account.Proxy.BackupProxyID = i64(92)
	backup := proxyForTest(92, "backup.invalid", 8080)
	s.proxyRepo = &fakeProxyLookup{byID: map[int64]*Proxy{92: backup}}
	require.False(t, s.isOpenAIProxyStreamQuarantined(context.Background(), account))
	s.getOpenAIProxyStreamCircuit().recordFailure(92, time.Now())
	require.True(t, s.isOpenAIProxyStreamQuarantined(context.Background(), account),
		"unusable backup must not bypass the scheduling gate")
	backup.FallbackMode = FallbackModeDirect
	require.False(t, s.isOpenAIProxyStreamQuarantined(context.Background(), account))
}

func TestQuarantineFallbackNewRequestsSkipPrimaryAndDoNotHealIt(t *testing.T) {
	s, account := quarantineFallbackFixture()
	var calls int
	s.httpUpstream = &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, id int64, _ int) (*http.Response, error) {
		calls++
		require.Empty(t, proxy, "the quarantined proxy must not receive even a connection attempt")
		require.Equal(t, account.ID, id)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "one-post", string(body))
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}}
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest(http.MethodPost, "https://example.invalid", io.NopCloser(strings.NewReader("one-post")))
		require.NoError(t, err)
		require.Nil(t, req.GetBody, "preselection does not require replayable body")
		resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
		require.NoError(t, err)
		id, proxied := openAIResponseEgressProxyID(account, resp)
		require.Zero(t, id)
		require.False(t, proxied)
		s.clearOpenAIProxyStreamDisconnect(account, resp)
		require.True(t, s.getOpenAIProxyStreamCircuit().isBlocked(91, time.Now()))
		require.Nil(t, account.TempUnschedulableUntil)
	}
	require.Equal(t, 3, calls)
	require.Equal(t, int64(91), *account.ProxyID)
}

func TestQuarantineFallbackActualBackupReceivesHealthObservations(t *testing.T) {
	s, account := quarantineFallbackFixture()
	account.Proxy.FallbackMode, account.Proxy.BackupProxyID = FallbackModeProxy, i64(92)
	backup := proxyForTest(92, "backup.invalid", 8080)
	s.proxyRepo = &fakeProxyLookup{byID: map[int64]*Proxy{92: backup}}
	s.httpUpstream = &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
		require.Equal(t, backup.URL(), proxy)
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}}
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", strings.NewReader("test"))
	resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
	require.NoError(t, err)
	s.recordOpenAIProxyStreamDisconnect(account, errors.New("unexpected EOF"), "", resp)
	require.True(t, s.getOpenAIProxyStreamCircuit().isBlocked(91, time.Now()))
	require.True(t, s.getOpenAIProxyStreamCircuit().isBlocked(92, time.Now()))
	s.clearOpenAIProxyStreamDisconnect(account, resp)
	require.False(t, s.getOpenAIProxyStreamCircuit().isBlocked(92, time.Now()))
	require.True(t, s.getOpenAIProxyStreamCircuit().isBlocked(91, time.Now()))
}

func TestQuarantineFallbackDirectDisconnectDoesNotBlameBoundProxy(t *testing.T) {
	s, account := quarantineFallbackFixture()
	s.getOpenAIProxyStreamCircuit().recordSuccess(91)
	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	resp := markOpenAIResponseEgress(&http.Response{}, req, 0)
	s.recordOpenAIProxyStreamDisconnect(account, errors.New("unexpected EOF"), "", resp)
	require.False(t, s.getOpenAIProxyStreamCircuit().isBlocked(91, time.Now()))
}

func TestQuarantineFallbackNoMidstreamReplay(t *testing.T) {
	s, account := quarantineFallbackFixture()
	var calls int
	s.httpUpstream = &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: &quarantineBrokenStream{}}, nil
	}}
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", strings.NewReader("test"))
	resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
	require.NoError(t, err)
	body, readErr := io.ReadAll(resp.Body)
	require.Equal(t, "partial-output", string(body))
	require.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
	require.Equal(t, 1, calls)
}

type quarantineBrokenStream struct{ read bool }

func (s *quarantineBrokenStream) Read(p []byte) (int, error) {
	if s.read {
		return 0, io.ErrUnexpectedEOF
	}
	s.read = true
	return copy(p, "partial-output"), nil
}
func (s *quarantineBrokenStream) Close() error { return nil }

func TestQuarantineFallbackConcurrentDirectSuccessDoesNotClearPrimary(t *testing.T) {
	s, account := quarantineFallbackFixture()
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
			resp := markOpenAIResponseEgress(&http.Response{}, req, 0)
			s.clearOpenAIProxyStreamDisconnect(account, resp)
			s.recordOpenAIProxyStreamDisconnect(account, io.ErrUnexpectedEOF, "", resp)
			s.isOpenAIProxyStreamQuarantined(context.Background(), account)
		}()
	}
	wg.Wait()
	require.True(t, s.getOpenAIProxyStreamCircuit().isBlocked(91, time.Now()))
}

func TestQuarantineFallbackTTLAllowsPrimaryRecovery(t *testing.T) {
	s, account := quarantineFallbackFixture()
	circuit := s.getOpenAIProxyStreamCircuit()
	circuit.mu.Lock()
	e := circuit.entries[91]
	e.blockedUntil = time.Now().Add(-time.Second)
	circuit.entries[91] = e
	circuit.mu.Unlock()
	var calls int
	s.httpUpstream = &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
		calls++
		require.Equal(t, account.Proxy.URL(), proxy)
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}}
	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
	require.NoError(t, err)
	id, ok := openAIResponseEgressProxyID(account, resp)
	require.True(t, ok)
	require.Equal(t, int64(91), id)
	s.clearOpenAIProxyStreamDisconnect(account, resp)
	require.Equal(t, 1, calls)
	require.False(t, circuit.isBlocked(91, time.Now()))
}
