//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

type runtimeFallbackUpstream struct {
	HTTPUpstream
	do func(*http.Request, string, int64, int) (*http.Response, error)
}

func (u *runtimeFallbackUpstream) Do(r *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	return u.do(r, proxy, id, concurrency)
}

func TestRuntimeProxyFallbackRequestSafety(t *testing.T) {
	for _, tc := range []struct {
		name, mode                                                           string
		err                                                                  error
		sent                                                                 string
		noTrace, noBody, cancelled, httpResponse, cloneFailure, retryFailure bool
		wantCalls                                                            int
	}{
		{name: "refused direct", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, wantCalls: 2},
		{name: "refused backup", mode: FallbackModeProxy, err: syscall.ECONNREFUSED, wantCalls: 2},
		{name: "proxy auth", mode: FallbackModeDirect, err: errors.New("proxy authentication required"), wantCalls: 2},
		{name: "DNS", mode: FallbackModeDirect, err: &net.DNSError{IsNotFound: true}, wantCalls: 2},
		{name: "disabled fallback", mode: FallbackModeNone, err: syscall.ECONNREFUSED, wantCalls: 1},
		{name: "transient reset", mode: FallbackModeDirect, err: syscall.ECONNRESET, wantCalls: 1},
		{name: "timeout", mode: FallbackModeDirect, err: context.DeadlineExceeded, wantCalls: 1},
		{name: "no positive trace", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, noTrace: true, wantCalls: 1},
		{name: "connection handed to HTTP", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, sent: "got_conn", wantCalls: 1},
		{name: "headers written", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, sent: "headers", wantCalls: 1},
		{name: "POST written", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, sent: "request", wantCalls: 1},
		{name: "response started", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, sent: "response", wantCalls: 1},
		{name: "unrewindable body", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, noBody: true, wantCalls: 1},
		{name: "cancelled", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, cancelled: true, wantCalls: 1},
		{name: "HTTP502", mode: FallbackModeDirect, httpResponse: true, wantCalls: 1},
		{name: "response plus error", mode: FallbackModeDirect, httpResponse: true, err: syscall.ECONNREFUSED, wantCalls: 1},
		{name: "body clone failure", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, cloneFailure: true, wantCalls: 1},
		{name: "fallback failure bounded", mode: FallbackModeDirect, err: syscall.ECONNREFUSED, retryFailure: true, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			primary := proxyForTest(1, "primary.invalid", 8080)
			primary.FallbackMode, primary.BackupProxyID = tc.mode, i64(2)
			backup := proxyForTest(2, "backup.invalid", 8080)
			account := &Account{ID: 1, Type: AccountTypeAPIKey, Concurrency: 3, Proxy: primary}
			calls, outerTraceCalls := 0, 0
			ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GetConn: func(string) { outerTraceCalls++ }})
			ctx = WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid/v1/responses", bytes.NewBufferString("test-body"))
			require.NoError(t, err)
			req.Header.Set("X-Test", "preserved")
			if tc.noBody {
				req.GetBody = nil
			}
			if tc.cloneFailure {
				req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("clone failure") }
			}
			upstream := &runtimeFallbackUpstream{do: func(r *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
				calls++
				require.Equal(t, account.ID, id)
				require.Equal(t, account.Concurrency, concurrency)
				if calls == 1 {
					require.Equal(t, primary.URL(), proxy)
					if !tc.noTrace {
						tr := httptrace.ContextClientTrace(r.Context())
						tr.GetConn("test.invalid")
						switch tc.sent {
						case "got_conn":
							tr.GotConn(httptrace.GotConnInfo{})
						case "headers":
							tr.WroteHeaders()
						case "request":
							tr.WroteRequest(httptrace.WroteRequestInfo{})
						case "response":
							tr.GotFirstResponseByte()
						}
					}
					if tc.cancelled {
						cancel()
					}
					if tc.httpResponse {
						return &http.Response{StatusCode: 502, Body: http.NoBody}, tc.err
					}
					return nil, tc.err
				}
				wantProxy := ""
				if tc.mode == FallbackModeProxy {
					wantProxy = backup.URL()
				}
				require.Equal(t, wantProxy, proxy)
				require.Equal(t, "preserved", r.Header.Get("X-Test"))
				body, readErr := io.ReadAll(r.Body)
				require.NoError(t, readErr)
				require.Equal(t, "test-body", string(body))
				_ = r.Body.Close()
				if tc.retryFailure {
					return nil, syscall.ECONNREFUSED
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody, Header: make(http.Header)}, nil
			}}
			s := &OpenAIGatewayService{httpUpstream: upstream, proxyRepo: &fakeProxyLookup{byID: map[int64]*Proxy{2: backup}}}
			resp, gotErr := s.doOpenAIUpstream(req, primary.URL(), account)
			require.Equal(t, tc.wantCalls, calls)
			if tc.wantCalls == 2 && !tc.retryFailure {
				require.NoError(t, gotErr)
				require.Equal(t, 200, resp.StatusCode)
			} else {
				require.ErrorIs(t, gotErr, tc.err)
			}
			if !tc.noTrace {
				require.Equal(t, 1, outerTraceCalls)
			}
			require.Equal(t, primary, account.Proxy, "runtime fallback must not rewrite the binding")
		})
	}
}

func TestRuntimeProxyFallbackRealRefusedConnection(t *testing.T) {
	var postCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "one-post" {
			w.WriteHeader(400)
			return
		}
		postCount.Add(1)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()
	deadListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	dead := deadListener.Addr().(*net.TCPAddr)
	require.NoError(t, deadListener.Close())
	primary := proxyForTest(1, "127.0.0.1", dead.Port)
	primary.FallbackMode = FallbackModeDirect
	account := &Account{ID: 1, Proxy: primary, Type: AccountTypeAPIKey}
	var calls int
	upstream := &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
		calls++
		transport := &http.Transport{}
		defer transport.CloseIdleConnections()
		if proxy != "" {
			parsed, parseErr := url.Parse(proxy)
			require.NoError(t, parseErr)
			transport.Proxy = http.ProxyURL(parsed)
		}
		return (&http.Client{Transport: transport}).Do(req)
	}}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("one-post"))
	require.NoError(t, err)
	s := &OpenAIGatewayService{httpUpstream: upstream}
	resp, err := s.doOpenAIUpstream(req, primary.URL(), account)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, 2, calls)
	require.Equal(t, int32(1), postCount.Load(), "only one POST reaches the target")
}

func TestRuntimeProxyFallbackChainBound(t *testing.T) {
	byID := map[int64]*Proxy{}
	for id := int64(1); id <= 6; id++ {
		p := proxyForTest(id, "unused.invalid", 8080)
		p.Status = StatusDisabled
		p.FallbackMode, p.BackupProxyID = FallbackModeProxy, i64(id+1)
		byID[id] = p
	}
	byID[6].Status = StatusActive
	s := &OpenAIGatewayService{proxyRepo: &fakeProxyLookup{byID: byID}}
	_, ok := s.resolveRuntimeProxyFallback(context.Background(), &Account{Proxy: byID[1]})
	require.False(t, ok, "do not walk beyond four backup hops")
}
