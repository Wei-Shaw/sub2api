package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/safe"
)

// ClaudeStartupProber is the narrow interface TokenRefreshService uses to
// request a startup-handshake probe after successfully refreshing a token.
// It exists only to avoid a direct *GatewayService dependency inside
// TokenRefreshService — the refresh loop knows nothing about HTTP forwarding
// details and should not learn them.
type ClaudeStartupProber interface {
	// ProbeClaudeStartup sends a max_tokens=1 haiku /v1/messages request to
	// the given Claude OAuth account. Matches the "am I alive" probe real
	// Claude Code CLI sends at boot, so after a background token refresh the
	// upstream sees the same traffic shape a real login would produce. Runs
	// synchronously but should be called from a goroutine at the caller side
	// so a slow upstream doesn't block the refresh loop.
	ProbeClaudeStartup(ctx context.Context, account *Account) error
}

// claudeStartupProbeBody is the canonical body Claude Code's startup probe
// sends: haiku, max_tokens=1, a single user message, no tools, no system
// prompt customization. Mirrors validator logic in claude_code_validator.go
// that recognizes this exact pattern.
func claudeStartupProbeBody() []byte {
	body := map[string]any{
		"model":      startupProbeModel,
		"max_tokens": startupProbeMaxTokens,
		"messages": []map[string]any{
			{
				"role":    startupProbeUserRole,
				"content": startupProbeUserContent,
			},
		},
	}
	b, _ := json.Marshal(body)
	return b
}

// ProbeClaudeStartup implements ClaudeStartupProber on GatewayService.
//
// Uses the same credential resolution, TLS profile, proxy, and fingerprint
// plumbing the real Forward path uses, so upstream sees a request that is
// indistinguishable from the first /v1/messages a freshly-authenticated
// Claude Code CLI would send.
func (s *GatewayService) ProbeClaudeStartup(ctx context.Context, account *Account) error {
	if s == nil || account == nil {
		return fmt.Errorf("nil gateway or account")
	}
	if account.Platform != PlatformAnthropic || !account.IsOAuth() {
		return nil // nothing to probe
	}

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}
	if tokenType != "oauth" {
		// Setup-token / api-key: skip. Only real OAuth accounts get a
		// startup probe since that matches what `claude setup-token` sets
		// up on a developer machine.
		return nil
	}

	body := claudeStartupProbeBody()

	req, err := http.NewRequestWithContext(ctx, "POST", claudeAPIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	setHeaderRaw(req.Header, "content-type", "application/json")
	setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	// Claude Code's startup probe sends the Haiku-scoped beta header: OAuth
	// scope + interleaved thinking (no claude-code beta, matching what the
	// real CLI sends for its initial /v1/messages warm-up).
	setHeaderRaw(req.Header, "anthropic-beta", claude.HaikuBetaHeader)
	applyClaudeOAuthHeaderDefaults(req)

	// Apply the account's cached fingerprint so X-Stainless-* headers match
	// whatever identity was minted the first time this account ran.
	if s.identityService != nil {
		if fp, fpErr := s.identityService.GetOrCreateFingerprint(ctx, account.ID, http.Header{}); fpErr == nil && fp != nil {
			s.identityService.ApplyFingerprint(req, fp)
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return fmt.Errorf("send probe: %w", err)
	}
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	return nil
}

// runStartupProbeAsync is a convenience wrapper that TokenRefreshService
// uses to fire a probe in a detached goroutine with a bounded timeout.
// Logs failures under service.sidecar_probe and swallows errors.
func runStartupProbeAsync(prober ClaudeStartupProber, account *Account) {
	if prober == nil || account == nil {
		return
	}
	accountID := account.ID
	safe.Go("sidecar_probe.startup", []slog.Attr{slog.Int64("account_id", accountID)}, func() {
		ctx, cancel := context.WithTimeout(context.Background(), startupProbeDefaultTimeout)
		defer cancel()
		if err := prober.ProbeClaudeStartup(ctx, account); err != nil {
			slog.Warn("sidecar_probe.startup.failed", "account_id", accountID, "error", err)
		}
	})
}
