package service

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ticketproxy"
	"github.com/stretchr/testify/require"
)

type codexTicketTestRefiller func(context.Context) (*ticketproxy.FetchResult, error)

func (f codexTicketTestRefiller) RefillIfEmpty(ctx context.Context) (*ticketproxy.FetchResult, error) {
	return f(ctx)
}

func TestCodexTicketAutoRefillResumesHarvestingInSameCycle(t *testing.T) {
	pool := ticketproxy.NewStore(filepath.Join(t.TempDir(), "pool.json"))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{codexTicketResponse()}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, Models: []string{"gpt-6-astra"}}, upstream)
	svc.codexTicketProxyPool = pool
	account := ticketTestAccount(41)
	account.Status = StatusActive
	svc.accountRepo = &codexTicketLifecycleRepo{account: *account}
	refills := 0
	svc.codexTicketProxyRefiller = codexTicketTestRefiller(func(context.Context) (*ticketproxy.FetchResult, error) {
		refills++
		proxies, duplicates, err := ticketproxy.Parse([]string{"192.0.2.1:8080"})
		require.NoError(t, err)
		result, err := pool.Import(proxies, duplicates)
		return &ticketproxy.FetchResult{ImportResult: result}, err
	})
	svc.refreshOpenAICodexTickets(context.Background())
	require.Equal(t, 1, refills)
	require.Equal(t, "http://192.0.2.1:8080", upstream.lastProxyURL)
	require.Len(t, upstream.requests, 1)
	require.NotNil(t, svc.lookupOpenAICodexTicket(account, "gpt-6-astra"))
}

func TestCodexTicketDisabledOrCancelledDoesNotRefill(t *testing.T) {
	for _, disabled := range []bool{true, false} {
		svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: !disabled}, &httpUpstreamRecorder{})
		svc.accountRepo = &codexTicketLifecycleRepo{}
		calls := 0
		svc.codexTicketProxyRefiller = codexTicketTestRefiller(func(context.Context) (*ticketproxy.FetchResult, error) {
			calls++
			return nil, nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		if !disabled {
			cancel()
		}
		svc.refreshOpenAICodexTickets(ctx)
		cancel()
		require.Zero(t, calls)
	}
}

func TestCodexTicketStopCancelsAutomaticRefill(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, &httpUpstreamRecorder{})
	svc.accountRepo = &codexTicketLifecycleRepo{}
	started := make(chan struct{})
	svc.codexTicketProxyRefiller = codexTicketTestRefiller(func(ctx context.Context) (*ticketproxy.FetchResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	svc.StartOpenAICodexTicketHarvester()
	t.Cleanup(svc.StopOpenAICodexTicketHarvester)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("automatic refill did not start")
	}
	stopped := make(chan struct{})
	go func() { svc.StopOpenAICodexTicketHarvester(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel automatic refill")
	}
}

func TestCodexTicketPoolUsesActualTicketResults(t *testing.T) {
	for _, failure := range []string{"missing", "wrong-length", "wrong-prefix", "http-error", "network-error"} {
		t.Run(failure, func(t *testing.T) {
			pool := ticketproxy.NewStore(filepath.Join(t.TempDir(), "pool.json"))
			proxies, d, err := ticketproxy.Parse([]string{"192.0.2.1:8080"})
			require.NoError(t, err)
			_, err = pool.Import(proxies, d)
			require.NoError(t, err)
			calls := 0
			upstream := &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return codexTicketResponse(), nil
				}
				response := codexTicketResponse()
				switch failure {
				case "missing":
					response.Header.Del(openAICodexTurnStateHeader)
				case "wrong-length":
					response.Header.Set(openAICodexTurnStateHeader, fakeCodexTicketState(312))
				case "wrong-prefix":
					response.Header.Set(openAICodexTurnStateHeader, "x"+fakeCodexTicketState(291))
				case "http-error":
					response.StatusCode = http.StatusBadGateway
				case "network-error":
					return nil, errors.New("proxy connection failed")
				}
				return response, nil
			}}
			svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, HarvestProxyURL: "http://old-rotator:1234"}, upstream)
			svc.codexTicketProxyPool = pool
			account := ticketTestAccount(41)
			svc.probeOnceOpenAICodexTicket(context.Background(), account, "gpt-6-astra")
			status, err := pool.Status()
			require.NoError(t, err)
			require.Equal(t, 1, status.Count)
			svc.probeOnceOpenAICodexTicket(context.Background(), account, "gpt-6-astra")
			status, err = pool.Status()
			require.NoError(t, err)
			require.Zero(t, status.Count)
			require.Equal(t, 1, status.Removed)
			svc.probeOnceOpenAICodexTicket(context.Background(), account, "gpt-6-astra")
			require.Equal(t, 2, calls, "first failure removes the proxy; empty managed pool must not use the old proxy")
		})
	}
}

func TestCodexTicketPoolUsesImportedProxyAndIgnoresCancellation(t *testing.T) {
	pool := ticketproxy.NewStore(filepath.Join(t.TempDir(), "pool.json"))
	proxies, d, err := ticketproxy.Parse([]string{"192.0.2.1:8080", "192.0.2.2:8080"})
	require.NoError(t, err)
	_, err = pool.Import(proxies, d)
	require.NoError(t, err)
	recorder := &httpUpstreamRecorder{responses: []*http.Response{codexTicketResponse(), codexTicketResponse()}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, HarvestProxyURL: "http://old-rotator:1234"}, recorder)
	svc.codexTicketProxyPool = pool
	account := ticketTestAccount(41)
	svc.probeOnceOpenAICodexTicket(context.Background(), account, "gpt-6-astra")
	require.Equal(t, "http://192.0.2.1:8080", recorder.lastProxyURL)
	svc.probeOnceOpenAICodexTicket(context.Background(), account, "gpt-6-astra")
	require.Equal(t, "http://192.0.2.2:8080", recorder.lastProxyURL)
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		svc.httpUpstream = &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled }}
		svc.probeOnceOpenAICodexTicket(ctx, account, "gpt-6-astra")
		cancel()
	}
	status, err := pool.Status()
	require.NoError(t, err)
	require.Equal(t, 2, status.Count)
	require.Zero(t, status.Removed)
}
