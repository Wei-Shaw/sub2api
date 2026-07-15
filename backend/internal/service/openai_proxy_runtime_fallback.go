package service

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// maxRuntimeProxyFallbackHops bounds the BackupProxyID chain walk performed on a
// transport failure. It guards against a misconfigured cycle and keeps the
// per-request cost of resolving a fallback proxy tiny.
const maxRuntimeProxyFallbackHops = 4

// resolveRuntimeProxyFallbackURL decides which egress a request should be
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
// cycle, or proxyRepo unavailable). The returned label is a human-readable
// endpoint used only for logging.
func (s *OpenAIGatewayService) resolveRuntimeProxyFallbackURL(ctx context.Context, account *Account) (proxyURL, label string, ok bool) {
	if account == nil || account.Proxy == nil {
		return "", "", false
	}
	start := account.Proxy
	switch start.FallbackMode {
	case FallbackModeDirect:
		return "", "direct", true
	case FallbackModeProxy:
		if s.proxyRepo == nil {
			return "", "", false
		}
		now := time.Now()
		visited := map[int64]struct{}{start.ID: {}}
		curID := start.BackupProxyID
		for hop := 0; hop < maxRuntimeProxyFallbackHops; hop++ {
			if curID == nil {
				return "", "", false
			}
			if _, seen := visited[*curID]; seen {
				return "", "", false
			}
			visited[*curID] = struct{}{}

			p, err := s.proxyRepo.GetByID(ctx, *curID)
			if err != nil || p == nil {
				return "", "", false
			}
			if p.IsActive() && !p.IsExpired(now) {
				return p.URL(), net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), true
			}
			// Unusable backup: continue along its own fallback chain, mirroring
			// ResolveProxyFallbackTarget's expiry-time behaviour.
			switch p.FallbackMode {
			case FallbackModeDirect:
				return "", "direct", true
			case FallbackModeProxy:
				curID = p.BackupProxyID
			default:
				return "", "", false
			}
		}
		return "", "", false
	default:
		return "", "", false
	}
}

// doUpstreamWithProxyFallback executes the upstream request through the account's
// primary proxy and, on a durable transport-level connection failure (the proxy
// is down / unreachable, so the request never reached the upstream), retries the
// same request once through the proxy's configured runtime fallback (backup proxy
// or direct). If no fallback is configured, or the failure is not a durable
// proxy/connection fault, it behaves exactly like a plain httpUpstream.Do call.
//
// Retrying is safe here precisely because a transport error means no bytes were
// delivered to the upstream; the retry only happens when the request body can be
// replayed (req.GetBody != nil, which stdlib sets for bytes/strings bodies).
func (s *OpenAIGatewayService) doUpstreamWithProxyFallback(ctx context.Context, req *http.Request, account *Account, primaryProxyURL string) (*http.Response, error) {
	resp, err := s.httpUpstream.Do(req, primaryProxyURL, account.ID, account.Concurrency)
	if err == nil {
		return resp, nil
	}

	// Only retry when: the client is still connected, the body is replayable, and
	// the failure is a durable proxy/connection fault (connection refused, no
	// route, DNS failure, rejected proxy credentials, ...). Transient blips such
	// as timeouts keep the existing account-failover behaviour.
	if ctx.Err() != nil || req.GetBody == nil {
		return resp, err
	}
	if !classifyOpenAITransportError(err).Persistent {
		return resp, err
	}

	fallbackURL, fallbackLabel, ok := s.resolveRuntimeProxyFallbackURL(ctx, account)
	if !ok || fallbackURL == primaryProxyURL {
		return resp, err
	}

	retryReq, cloneErr := cloneUpstreamRequestForRetry(ctx, req)
	if cloneErr != nil {
		return resp, err
	}

	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.proxy_runtime_fallback",
		zap.Int64("account_id", account.ID),
		zap.String("account_name", account.Name),
		zap.String("primary_proxy", primaryProxyLabel(account)),
		zap.String("fallback_proxy", fallbackLabel),
		zap.String("reason", sanitizeUpstreamErrorMessage(err.Error())),
	)

	retryResp, retryErr := s.httpUpstream.Do(retryReq, fallbackURL, account.ID, account.Concurrency)
	if retryErr != nil {
		// The fallback egress failed too: surface the retry error so the caller's
		// normal transport-error handler performs account-level failover.
		return retryResp, retryErr
	}
	return retryResp, nil
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

// primaryProxyLabel returns a human-readable endpoint for the account's primary
// proxy, used only for logging a fallback event.
func primaryProxyLabel(account *Account) string {
	if account == nil || account.Proxy == nil {
		return "none"
	}
	return net.JoinHostPort(account.Proxy.Host, strconv.Itoa(account.Proxy.Port))
}
