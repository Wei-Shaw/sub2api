//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func runtimeChainFixture(quarantined bool) (*OpenAIGatewayService, *Account, *Proxy) {
	s, account := quarantineFallbackFixture()
	if !quarantined {
		s.getOpenAIProxyStreamCircuit().recordSuccess(account.Proxy.ID)
	}
	account.Proxy.FallbackMode, account.Proxy.BackupProxyID = FallbackModeProxy, i64(92)
	backup := proxyForTest(92, "backup.invalid", 8080)
	backup.Name, backup.FallbackMode = "backup", FallbackModeDirect
	s.proxyRepo = &fakeProxyLookup{byID: map[int64]*Proxy{92: backup}}
	return s, account, backup
}

func runtimeChainRefused(req *http.Request) (*http.Response, error) {
	trace := httptrace.ContextClientTrace(req.Context())
	trace.GetConn("fixture.invalid")
	return nil, syscall.ECONNREFUSED
}

func TestRuntimeProxyChainBackupRefusalContinuesToDirect(t *testing.T) {
	for _, quarantine := range []bool{false, true} {
		t.Run(fmt.Sprint(quarantine), func(t *testing.T) {
			s, account, backup := runtimeChainFixture(quarantine)
			var attempted []string
			var outerTraceCalls int
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
				GetConn: func(string) { outerTraceCalls++ },
			})
			s.httpUpstream = &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, id int64, _ int) (*http.Response, error) {
				attempted = append(attempted, proxy)
				require.Equal(t, account.ID, id)
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, "one-post", string(body))
				require.Equal(t, "preserved", req.Header.Get("X-Test"))
				if proxy != "" {
					return runtimeChainRefused(req)
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			}}
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid", strings.NewReader("one-post"))
			req.Header.Set("X-Test", "preserved")
			resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
			require.NoError(t, err)
			want := []string{backup.URL(), ""}
			if !quarantine {
				want = append([]string{account.Proxy.URL()}, want...)
			}
			require.Equal(t, want, attempted)
			require.Equal(t, len(want)-1, outerTraceCalls)
			id, proxied := openAIResponseEgressProxyID(account, resp)
			require.Zero(t, id)
			require.False(t, proxied)
			s.clearOpenAIProxyStreamDisconnect(account, resp)
			require.Equal(t, quarantine, s.getOpenAIProxyStreamCircuit().isBlocked(account.Proxy.ID, time.Now()))
			require.Equal(t, int64(91), *account.ProxyID)
			require.Nil(t, account.TempUnschedulableUntil)
		})
	}
}

func TestRuntimeProxyChainEveryHopMustProveUnsent(t *testing.T) {
	for _, tc := range []string{
		"no-trace", "got-conn", "header-field", "wrote-headers", "wrote-request", "response-byte",
		"response-and-error", "http-502", "cancelled", "timeout", "reset",
		"plugin-sent", "plugin-without-trace", "body-clone-fails",
	} {
		t.Run(tc, func(t *testing.T) {
			s, account, backup := runtimeChainFixture(false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid", strings.NewReader("test"))
			var calls int
			s.httpUpstream = &runtimeFallbackUpstream{do: func(r *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
				calls++
				if calls == 1 {
					return runtimeChainRefused(r)
				}
				require.Equal(t, backup.URL(), proxy, "must not proceed to direct")
				tr := httptrace.ContextClientTrace(r.Context())
				if tc != "no-trace" && tc != "plugin-without-trace" {
					tr.GetConn("backup.invalid")
				}
				switch tc {
				case "got-conn":
					tr.GotConn(httptrace.GotConnInfo{})
				case "header-field":
					tr.WroteHeaderField("X-Test", []string{"value"})
				case "wrote-headers":
					tr.WroteHeaders()
				case "wrote-request":
					tr.WroteRequest(httptrace.WroteRequestInfo{})
				case "response-byte":
					tr.GotFirstResponseByte()
				case "response-and-error":
					return &http.Response{StatusCode: 502, Body: http.NoBody}, syscall.ECONNREFUSED
				case "http-502":
					return &http.Response{StatusCode: 502, Body: http.NoBody}, nil
				case "cancelled":
					cancel()
				case "timeout":
					return nil, context.DeadlineExceeded
				case "reset":
					return nil, syscall.ECONNRESET
				case "plugin-sent", "plugin-without-trace":
					return nil, &PluginTransportError{Message: "connection refused", RequestSent: tc == "plugin-sent"}
				case "body-clone-fails":
					req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("cannot clone") }
				}
				return nil, syscall.ECONNREFUSED
			}}
			_, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
			if tc != "http-502" {
				require.Error(t, err)
				id, name := runtimeProxyErrorAttribution(account, err)
				require.Equal(t, backup.ID, *id)
				require.Equal(t, "backup", name)
			}
			require.Equal(t, 2, calls)
		})
	}
}

func TestRuntimeProxyChainQuarantinedNonReplayableBodyOnlyAttemptsBackupOnce(t *testing.T) {
	s, account, _ := runtimeChainFixture(true)
	var calls int
	s.httpUpstream = &runtimeFallbackUpstream{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls++
		return runtimeChainRefused(r)
	}}
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", io.NopCloser(strings.NewReader("test")))
	require.Nil(t, req.GetBody)
	_, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
	require.ErrorIs(t, err, syscall.ECONNREFUSED)
	require.Equal(t, 1, calls)
}

func TestRuntimeProxyChainDoesNotReplayBrokenBackupStream(t *testing.T) {
	s, account, backup := runtimeChainFixture(true)
	var calls int
	s.httpUpstream = &runtimeFallbackUpstream{do: func(r *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
		calls++
		require.Equal(t, backup.URL(), proxy)
		return &http.Response{StatusCode: 200, Body: &quarantineBrokenStream{}}, nil
	}}
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", strings.NewReader("test"))
	resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.Equal(t, "partial-output", string(body))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, 1, calls)
	id, proxied := openAIResponseEgressProxyID(account, resp)
	require.True(t, proxied)
	require.Equal(t, backup.ID, id)
}

func TestRuntimeProxyChainSharedHopBudgetCyclesAliasesAndDirectAuthorization(t *testing.T) {
	for _, tc := range []string{"budget", "budget-with-skips", "cycle", "alias", "direct-not-authorized", "direct-fails"} {
		t.Run(tc, func(t *testing.T) {
			s, account, backup := runtimeChainFixture(false)
			repo := s.proxyRepo.(*fakeProxyLookup)
			wantAttempts := 2
			switch tc {
			case "budget", "budget-with-skips":
				for id := int64(92); id <= 97; id++ {
					p := proxyForTest(id, fmt.Sprintf("proxy-%d.invalid", id), 8080)
					p.FallbackMode, p.BackupProxyID = FallbackModeProxy, i64(id+1)
					repo.byID[id] = p
				}
				wantAttempts = 5 // primary + four records, not four per attempt
				if tc == "budget-with-skips" {
					repo.byID[93].Status = StatusDisabled
					past := time.Now().Add(-time.Hour)
					repo.byID[94].ExpiresAt = &past
					wantAttempts = 3
				}
			case "cycle":
				backup.FallbackMode, backup.BackupProxyID = FallbackModeProxy, account.ProxyID
			case "alias":
				backup.Host, backup.Port = account.Proxy.Host, account.Proxy.Port
				wantAttempts = 2 // primary then direct, never the alias
			case "direct-not-authorized":
				backup.FallbackMode = FallbackModeNone
			case "direct-fails":
				wantAttempts = 3
			}
			var attempts []string
			s.httpUpstream = &runtimeFallbackUpstream{do: func(r *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
				attempts = append(attempts, proxy)
				require.LessOrEqual(t, len(attempts), wantAttempts)
				return runtimeChainRefused(r)
			}}
			req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", strings.NewReader("test"))
			_, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			require.Len(t, attempts, wantAttempts)
			seen := map[string]bool{}
			for _, endpoint := range attempts {
				require.False(t, seen[endpoint], "an endpoint must not be attempted twice")
				seen[endpoint] = true
			}
			if tc == "direct-fails" || tc == "alias" {
				id, name := runtimeProxyErrorAttribution(account, err)
				require.Nil(t, id)
				require.Equal(t, opsProxyNameDirect, name)
			} else {
				require.NotContains(t, attempts, "")
			}
		})
	}
}

func TestRuntimeProxyChainRealTwoRefusalsOnlySendOnePOST(t *testing.T) {
	for _, quarantined := range []bool{false, true} {
		t.Run(fmt.Sprint(quarantined), func(t *testing.T) {
			var posts atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, "one-post", string(body))
				posts.Add(1)
				_, _ = w.Write([]byte("OK"))
			}))
			defer target.Close()
			s, account, backup := runtimeChainFixture(quarantined)
			var listeners []net.Listener
			for _, p := range []*Proxy{account.Proxy, backup} {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				p.Host, p.Port = "127.0.0.1", listener.Addr().(*net.TCPAddr).Port
				listeners = append(listeners, listener)
			}
			for _, listener := range listeners {
				require.NoError(t, listener.Close())
			}
			var attempts int
			s.httpUpstream = &runtimeFallbackUpstream{do: func(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
				attempts++
				tr := &http.Transport{}
				defer tr.CloseIdleConnections()
				if proxy != "" {
					u, err := url.Parse(proxy)
					require.NoError(t, err)
					tr.Proxy = http.ProxyURL(u)
				}
				return (&http.Client{Transport: tr, Timeout: 2 * time.Second}).Do(req)
			}}
			req, _ := http.NewRequest(http.MethodPost, target.URL, strings.NewReader("one-post"))
			resp, err := s.doOpenAIUpstream(req, account.Proxy.URL(), account)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			require.Equal(t, int32(1), posts.Load())
			want := 3
			if quarantined {
				want = 2
			}
			require.Equal(t, want, attempts)
		})
	}
}

func TestRuntimeProxyChainTerminalErrorOpsUsesActualEgress(t *testing.T) {
	for _, id := range []int64{0, 92, -1} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			s, account, _ := runtimeChainFixture(false)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			err := &runtimeProxyEgressError{
				error: context.Canceled, target: runtimeProxyEgress{proxyID: id, proxyName: "actual-backup"},
			}
			result := s.handleOpenAIUpstreamTransportError(context.Background(), c, account, err, false)
			require.ErrorIs(t, result, context.Canceled)
			value, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events := value.([]*OpsUpstreamErrorEvent)
			require.Len(t, events, 1)
			if id == 0 {
				require.Nil(t, events[0].ProxyID)
				require.Equal(t, opsProxyNameDirect, events[0].ProxyName)
			} else if id > 0 {
				require.Equal(t, id, *events[0].ProxyID)
				require.Equal(t, "actual-backup", events[0].ProxyName)
			} else {
				require.Equal(t, account.Proxy.ID, *events[0].ProxyID)
			}
			require.Equal(t, int64(91), *account.ProxyID)
			require.Nil(t, account.TempUnschedulableUntil)
		})
	}
}
