package service

import (
	"fmt"
	"strings"
	"time"
)

// Default Grok stream idle when gateway.stream_data_interval_timeout is 0.
// Long enough for slow thinking models, short enough to release hung sockets.
const defaultGrokStreamIdleTimeout = 180 * time.Second

// Shorter cool after a Grok stream-idle failure so the account can re-enter soon
// but is not immediately re-picked in a tight failover loop.
const grokStreamIdleCooldown = 2 * time.Minute

// resolveGrokStreamIdleTimeout returns the effective upstream-read idle timeout
// for Grok streams. Prefers the global gateway setting when positive; otherwise
// applies a Grok-only default so hung SSE bodies still fail over.
func resolveGrokStreamIdleTimeout(cfgStreamIntervalSec int) time.Duration {
	if cfgStreamIntervalSec > 0 {
		return time.Duration(cfgStreamIntervalSec) * time.Second
	}
	return defaultGrokStreamIdleTimeout
}

// grokStreamIdleFailoverError builds a pre-commit/handler-visible failover so
// the gateway can switch OAuth accounts after a hung Grok upstream stream.
func grokStreamIdleFailoverError(account *Account, idle time.Duration) *UpstreamFailoverError {
	msg := fmt.Sprintf("Grok stream idle timeout after %s with no upstream data", idle.Round(time.Second))
	return &UpstreamFailoverError{
		StatusCode:               502,
		ResponseBody:             []byte(`{"error":{"code":"empty_upstream","message":"` + strings.ReplaceAll(msg, `"`, `'`) + `"}}`),
		SafeToFailoverAfterWrite: true,
		// An idle upstream stream is transient and should get the configured
		// same-account retry budget before switching credentials. This applies
		// to both pooled and dedicated Grok accounts; the handler still enforces
		// the request's retry limit.
		RetryableOnSameAccount: account != nil && account.Platform == PlatformGrok,
		RequestScopedTransient: true,
		SameAccountRetryMax:    1,
		// Permit at most one same-account replay after the idle failure. The
		// deadline is anchored at failure time, so a hung stream cannot consume
		// the normal three-attempt budget before failover.
		SameAccountRetryDeadline: time.Now().Add(idle),
	}
}
