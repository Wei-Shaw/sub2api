package service

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/safe"
)

// maybeInjectCountTokensSidecar fires a fire-and-forget count_tokens request
// to upstream for Claude OAuth accounts, so the account's endpoint mix
// includes /v1/messages/count_tokens at a rate that tracks /v1/messages.
//
// Why this exists: real Claude Code CLI calls count_tokens for every message
// the user composes (the editor uses the result for the "N tokens" hint in
// the status area). A gateway that only forwards /v1/messages produces a
// traffic shape that is trivially distinguishable from real CLI usage, and
// Anthropic's subscription abuse heuristics flag that shape and silently cap
// the account's weekly limit to a tiny fraction of its nominal value.
//
// Gating (all must be true, else no-op):
//   - cfg.SidecarProbe.CountTokensInject.Enabled
//   - account is a Claude OAuth / setup-token account on the Anthropic platform
//   - downstream client is NOT already Claude Code (which sends its own
//     count_tokens; adding a second one would double-dip)
//
// Execution shape: spawns a goroutine with a detached context bounded by
// CountTokensInject.TimeoutMilliseconds. Errors are logged but never
// surfaced — sidecar traffic is best-effort and must not affect the real
// user request. The body is cloned before the goroutine because the main
// path's retry loop may mutate the original buffer.
func (s *GatewayService) maybeInjectCountTokensSidecar(
	account *Account,
	body []byte,
	reqModel string,
	token, tokenType string,
	mimicClaudeCode bool,
	isClaudeCode bool,
	proxyURL string,
) {
	if s == nil || s.cfg == nil {
		return
	}
	if !s.cfg.SidecarProbe.CountTokensInject.Enabled {
		return
	}
	if account == nil || account.Platform != PlatformAnthropic || !account.IsOAuth() {
		return
	}
	if isClaudeCode {
		// Real Claude Code already sends count_tokens through the gateway's
		// /v1/messages/count_tokens passthrough route. Injecting another one
		// would produce a 2:1 count_tokens:messages ratio per account,
		// which is itself anomalous.
		return
	}
	if len(body) == 0 {
		return
	}

	timeoutMs := s.cfg.SidecarProbe.CountTokensInject.TimeoutMilliseconds
	if timeoutMs <= 0 {
		timeoutMs = countTokensSidecarDefaultTimeoutMs
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// Clone body: the main path's retry loop mutates its buffer, and the
	// goroutine may read it long after Forward() has returned.
	bodyCopy := append([]byte(nil), body...)

	accountID := account.ID
	safe.Go("sidecar_probe.count_tokens", []slog.Attr{slog.Int64("account_id", accountID)}, func() {
		// Detached context: the goroutine must outlive the caller's ctx,
		// which gets cancelled when the downstream client disconnects.
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := s.buildCountTokensRequest(ctx, nil, account, bodyCopy, token, tokenType, reqModel, mimicClaudeCode)
		if err != nil {
			slog.Warn("sidecar_probe.count_tokens.build_failed", "account_id", accountID, "error", err)
			return
		}

		tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)
		resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
		if err != nil {
			slog.Warn("sidecar_probe.count_tokens.request_failed", "account_id", accountID, "error", err)
			return
		}
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	})
}
