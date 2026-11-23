package service

import (
	"context"
	"sync"
)

const AccountExtraUpstreamRequestZstd = "upstream_request_zstd"

func (a *Account) IsUpstreamRequestZstdEnabled() bool {
	return a != nil && a.Type == AccountTypeAPIKey && a.Extra[AccountExtraUpstreamRequestZstd] == true
}

type upstreamRequestZstdProbeKey struct{}

// UpstreamRequestZstdProbe overrides the saved policy during an admin test.
// A disabled probe provides a genuine uncompressed comparison even on allowlisted hosts.
type UpstreamRequestZstdProbe struct {
	Compress       bool
	accountEnabled bool
	mu             sync.Mutex
	attempts       []UpstreamRequestZstdAttempt
}

type UpstreamRequestZstdAttempt struct {
	Host      string `json:"host"`
	RawBytes  int    `json:"raw_bytes"`
	WireBytes int    `json:"wire_bytes"`
}

type upstreamRequestZstdReport struct {
	Attempts       []UpstreamRequestZstdAttempt `json:"attempts"`
	AccountEnabled bool                         `json:"account_enabled"`
}

func WithUpstreamRequestZstdProbe(ctx context.Context, probe *UpstreamRequestZstdProbe) context.Context {
	return context.WithValue(ctx, upstreamRequestZstdProbeKey{}, probe)
}

func UpstreamRequestZstdProbeFromContext(ctx context.Context) *UpstreamRequestZstdProbe {
	probe, _ := ctx.Value(upstreamRequestZstdProbeKey{}).(*UpstreamRequestZstdProbe)
	return probe
}

func (p *UpstreamRequestZstdProbe) Record(attempt UpstreamRequestZstdAttempt) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.attempts = append(p.attempts, attempt)
}

func (p *UpstreamRequestZstdProbe) Attempts() []UpstreamRequestZstdAttempt {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]UpstreamRequestZstdAttempt{}, p.attempts...)
}

func (p *UpstreamRequestZstdProbe) report() *upstreamRequestZstdReport {
	return &upstreamRequestZstdReport{Attempts: p.Attempts(), AccountEnabled: p.accountEnabled}
}
