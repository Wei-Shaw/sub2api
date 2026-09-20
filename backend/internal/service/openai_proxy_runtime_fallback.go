package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// One request visits at most four backup records (including skipped records).
// The primary and a final explicitly authorized direct attempt are additional.
const maxRuntimeProxyFallbackHops = 4

type runtimeProxyEgress struct {
	url       string
	proxyID   int64 // zero explicitly means direct, not the account's bound proxy
	proxyName string
}

// A request-local cursor shares the lookup/cycle budget across all attempts.
// Resolving a new chain from each failed backup would reset that budget and
// could resend to an earlier failed or quarantined endpoint.
type runtimeProxyFallbackChain struct {
	current *Proxy
	visited map[int64]bool
	urls    map[string]bool
	hops    int
}

func newRuntimeProxyFallbackChain(primary *Proxy) *runtimeProxyFallbackChain {
	chain := &runtimeProxyFallbackChain{
		current: primary, visited: make(map[int64]bool), urls: make(map[string]bool),
	}
	if primary != nil {
		chain.visited[primary.ID] = true
		chain.urls[primary.URL()] = true
	}
	return chain
}

func (chain *runtimeProxyFallbackChain) next(ctx context.Context, s *OpenAIGatewayService) (runtimeProxyEgress, bool) {
	for chain.current != nil && ctx.Err() == nil {
		current := chain.current
		chain.current = nil
		switch current.FallbackMode {
		case FallbackModeDirect:
			return runtimeProxyEgress{}, true
		case FallbackModeProxy:
			id := current.BackupProxyID
			if s.proxyRepo == nil || id == nil || *id <= 0 || chain.visited[*id] ||
				chain.hops >= maxRuntimeProxyFallbackHops {
				return runtimeProxyEgress{}, false
			}
			chain.visited[*id] = true
			chain.hops++
			p, err := s.proxyRepo.GetByID(ctx, *id)
			if err != nil || p == nil || p.ID != *id {
				return runtimeProxyEgress{}, false
			}
			chain.current = p
			url := p.URL()
			duplicate := chain.urls[url]
			chain.urls[url] = true
			now := time.Now()
			if !duplicate && p.IsActive() && !p.IsExpired(now) &&
				!s.getOpenAIProxyStreamCircuit().isBlocked(p.ID, now) {
				return runtimeProxyEgress{url: url,
					proxyID: p.ID, proxyName: p.Name}, true
			}
			// A skipped record consumes the same budget as an attempted one.
		default:
			return runtimeProxyEgress{}, false
		}
	}
	return runtimeProxyEgress{}, false
}

// resolveRuntimeProxyFallback decides which egress a request should be
// retried through when the account's primary proxy connection fails at the
// transport layer (proxy down / unreachable). It reuses the per-proxy
// FallbackMode / BackupProxyID configuration that already drives proxy-expiry
// reassignment, so no new config is required:
//
//   - FallbackModeDirect: retry with a direct connection (empty proxy URL).
//   - FallbackModeProxy:  walk the BackupProxyID chain and return the first
//     active, non-expired backup proxy's URL.
//   - otherwise:          no runtime fallback.
//
// ok=false means there is nothing to fall back to (mode none, chain unresolved,
// cycle, or proxyRepo unavailable). The target carries the actual proxy identity
// for health/error attribution; zero explicitly identifies direct networking.
func (s *OpenAIGatewayService) resolveRuntimeProxyFallback(ctx context.Context, account *Account) (runtimeProxyEgress, bool) {
	if account == nil || account.Proxy == nil {
		return runtimeProxyEgress{}, false
	}
	return newRuntimeProxyFallbackChain(account.Proxy).next(ctx, s)
}

// Preserve errors.Is/As and the original message, while attributing failures
// without a Response to the actual egress, not the account's primary binding.
type runtimeProxyEgressError struct {
	error
	target runtimeProxyEgress
}

func (e *runtimeProxyEgressError) Unwrap() error { return e.error }

func runtimeProxyErrorAttribution(account *Account, err error) (*int64, string) {
	var attemptErr *runtimeProxyEgressError
	if !errors.As(err, &attemptErr) || attemptErr.target.proxyID < 0 {
		return opsUpstreamProxyAttribution(account)
	}
	if attemptErr.target.proxyID == 0 {
		return nil, opsProxyNameDirect
	}
	id := attemptErr.target.proxyID
	name := strings.TrimSpace(attemptErr.target.proxyName)
	if name == "" {
		name = opsProxyNameUnnamed
	}
	return &id, name
}

// Keep plugin routing inside each attempt, including a preselected healthy
// egress. No request is sent to both a plugin and the native transport.
func (s *OpenAIGatewayService) doOpenAIProxyAttempt(req *http.Request, account *Account, target runtimeProxyEgress) (resp *http.Response, err error) {
	defer func() {
		if err != nil && target.proxyID >= 0 {
			err = &runtimeProxyEgressError{error: err, target: target}
		}
	}()
	if s.pluginManager != nil {
		resp, handled, err := s.pluginManager.RoundTripOpenAIOAuth(req.Context(), req, target.url, account)
		if handled {
			return markOpenAIResponseEgress(resp, req, target.proxyID), err
		}
	}
	resp, err = s.httpUpstream.Do(req, target.url, account.ID, account.Concurrency)
	return markOpenAIResponseEgress(resp, req, target.proxyID), err
}

// doUpstreamWithProxyFallback executes the upstream request through the account's
// configured, bounded egress chain. Every failed attempt must independently prove
// that HTTP never received a connection before another egress may be tried.
// A response (including its later stream failures) always ends this loop.
func (s *OpenAIGatewayService) doUpstreamWithProxyFallback(ctx context.Context, req *http.Request, account *Account, primaryProxyURL string) (*http.Response, error) {
	primary := runtimeProxyEgress{url: primaryProxyURL, proxyID: -1}
	if primaryProxyURL == "" {
		primary.proxyID = 0
	} else if account.Proxy != nil && primaryProxyURL == account.Proxy.URL() {
		primary.proxyID = account.Proxy.ID
		primary.proxyName = account.Proxy.Name
	}
	if account.Proxy == nil || primaryProxyURL == "" ||
		primaryProxyURL != account.Proxy.URL() ||
		(account.Proxy.FallbackMode != FallbackModeDirect && account.Proxy.FallbackMode != FallbackModeProxy) {
		return s.doOpenAIProxyAttempt(req, account, primary)
	}
	chain := newRuntimeProxyFallbackChain(account.Proxy)
	target := primary
	// A new request can select an authorized alternative before sending any
	// bytes. This is not a replay and also works for non-rewindable bodies.
	if account.Platform == PlatformOpenAI && primary.proxyID > 0 &&
		s.getOpenAIProxyStreamCircuit().isBlocked(primary.proxyID, time.Now()) {
		if fallback, ok := chain.next(ctx, s); ok {
			target = fallback
			logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
				"openai.proxy_quarantine_fallback",
				zap.Int64("account_id", account.ID),
				zap.Int64("primary_proxy_id", primary.proxyID),
				zap.Int64("egress_proxy_id", target.proxyID),
				zap.Bool("fallback_direct", target.proxyID == 0),
			)
		}
	}
	attemptReq := req
	for {
		resp, err, unsent := s.doOpenAIProxyAttemptWithTrace(attemptReq, account, target)
		if err == nil || resp != nil || !unsent || target.proxyID == 0 ||
			ctx.Err() != nil || req.Context().Err() != nil || req.GetBody == nil ||
			!classifyUpstreamTransportError(err).Persistent {
			return resp, err
		}
		// A plugin may report having sent a request even when it did not emit
		// standard net/http trace callbacks. Its positive signal is a veto.
		var pluginErr *PluginTransportError
		if errors.As(err, &pluginErr) && pluginErr.RequestSent {
			return resp, err
		}
		next, ok := chain.next(ctx, s)
		if !ok {
			return resp, err
		}
		retry, cloneErr := cloneUpstreamRequestForRetry(req.Context(), req)
		if cloneErr != nil {
			return resp, err
		}
		logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
			"openai.proxy_runtime_fallback",
			zap.Int64("account_id", account.ID),
			zap.Int64("primary_proxy_id", account.Proxy.ID),
			zap.Int64("failed_proxy_id", target.proxyID),
			zap.Int64("egress_proxy_id", next.proxyID),
			zap.Bool("fallback_direct", next.proxyID == 0),
			zap.String("reason", "persistent_connection_failure_before_http"),
		)
		attemptReq, target = retry, next
	}
}

// Never reuse the previous attempt's trace state or context. Otherwise a
// failure before GetConn on a later hop could inherit an earlier safety proof.
func (s *OpenAIGatewayService) doOpenAIProxyAttemptWithTrace(req *http.Request, account *Account, target runtimeProxyEgress) (*http.Response, error, bool) {
	var mu sync.Mutex
	var connecting, handedToHTTP bool
	markHTTP := func() {
		mu.Lock()
		handedToHTTP = true
		mu.Unlock()
	}
	trace := &httptrace.ClientTrace{
		GetConn: func(string) {
			mu.Lock()
			connecting = true
			mu.Unlock()
		},
		GotConn:              func(httptrace.GotConnInfo) { markHTTP() },
		WroteHeaderField:     func(string, []string) { markHTTP() },
		WroteHeaders:         markHTTP,
		WroteRequest:         func(httptrace.WroteRequestInfo) { markHTTP() },
		GotFirstResponseByte: markHTTP,
	}
	attempt := req.Clone(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := s.doOpenAIProxyAttempt(attempt, account, target)
	mu.Lock()
	safeToRetry := connecting && !handedToHTTP
	mu.Unlock()
	return resp, err, safeToRetry
}

// cloneUpstreamRequestForRetry produces an independent copy of req with a fresh,
// rewound body so it can be re-sent through a different proxy. The caller must
// have verified req.GetBody != nil.
func cloneUpstreamRequestForRetry(ctx context.Context, req *http.Request) (*http.Request, error) {
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone := req.Clone(ctx)
	clone.Body = body
	return clone, nil
}
