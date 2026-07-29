package service

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
)

// ConnectionSignalEmitter is the hot-path writer for connection-risk signals.
// All errors are fail-open: Emit never blocks the gateway on Redis failure.
type ConnectionSignalEmitter struct {
	cache    ConnectionSignalCache
	settings *SettingService
	cfg      *config.Config
	metrics  *ConnectionRiskMetrics
	emitSeq  atomic.Uint64
}

// NewConnectionSignalEmitter constructs the emitter (does not start anything).
func NewConnectionSignalEmitter(
	cache ConnectionSignalCache,
	settings *SettingService,
	cfg *config.Config,
	metrics *ConnectionRiskMetrics,
) *ConnectionSignalEmitter {
	if metrics == nil {
		metrics = &ConnectionRiskMetrics{}
	}
	return &ConnectionSignalEmitter{
		cache:    cache,
		settings: settings,
		cfg:      cfg,
		metrics:  metrics,
	}
}

// Metrics exposes process counters for runtime/admin.
func (e *ConnectionSignalEmitter) Metrics() *ConnectionRiskMetrics {
	if e == nil {
		return nil
	}
	return e.metrics
}

// IncrSessionMismatch implements SessionMismatchSignal for R7 (jwt/admin auth).
// Fail-open on any error.
func (e *ConnectionSignalEmitter) IncrSessionMismatch(ctx context.Context, userID int64) {
	if e == nil || e.cache == nil || userID <= 0 {
		return
	}
	if !e.masterEnabled() {
		return
	}
	s := e.cachedSettings(ctx)
	if !s.Enabled {
		return
	}
	timeout := e.emitTimeout()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := e.cache.IncrSessionMismatch(cctx, userID); err != nil {
		e.metrics.EmitError.Add(1)
	}
}

// Emit records one authenticated request. rawIP should be SecurityClientIP output.
// userAgent is the raw UA string (hashed internally).
func (e *ConnectionSignalEmitter) Emit(ctx context.Context, userID, apiKeyID int64, rawIP, userAgent string) {
	e.EmitWithPrefix(ctx, userID, apiKeyID, rawIP, userAgent, "")
}

// EmitWithPrefix is Emit plus an optional raw API key used only to derive a short display prefix.
func (e *ConnectionSignalEmitter) EmitWithPrefix(ctx context.Context, userID, apiKeyID int64, rawIP, userAgent, rawKey string) {
	if e == nil || e.cache == nil {
		return
	}
	if userID <= 0 || apiKeyID <= 0 {
		return
	}
	if !e.masterEnabled() {
		return
	}
	s := e.cachedSettings(ctx)
	if !s.Enabled || !s.EmitEnabled {
		return
	}

	normIP := ip.NormalizeClientIPForSecurity(rawIP)
	if normIP == "" {
		return
	}
	sig := ConnectionSignal{
		UserID:    userID,
		APIKeyID:  apiKeyID,
		IP:        normIP,
		UAHash:    HashUserAgent(userAgent),
		KeyPrefix: maskAPIKeyPrefix(rawKey),
	}

	timeout := e.emitTimeout()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	seq := e.emitSeq.Add(1)
	_, err := e.cache.EmitAlwaysOn(cctx, sig, s.MaxActiveMembers, s.ActivePruneEveryNEmits, seq)
	if err != nil {
		if cctx.Err() == context.DeadlineExceeded {
			e.metrics.EmitTimeout.Add(1)
		} else {
			e.metrics.EmitError.Add(1)
		}
		return
	}
	e.metrics.EmitOK.Add(1)

	// Tier B evidence (sampled) — never authority for rules
	rate := s.EmitSampleRateEvidence
	if rate > 0 && (rate >= 1 || rand.Float64() < rate) {
		_ = e.cache.EmitEvidence(cctx, sig)
	}
}

func (e *ConnectionSignalEmitter) masterEnabled() bool {
	if e.cfg == nil {
		return false
	}
	return e.cfg.ConnectionRisk.Enabled
}

func (e *ConnectionSignalEmitter) cachedSettings(ctx context.Context) ConnectionRiskSettings {
	if e.settings == nil {
		return *DefaultConnectionRiskSettings()
	}
	return e.settings.GetConnectionRiskSettingsCached(ctx)
}

func (e *ConnectionSignalEmitter) emitTimeout() time.Duration {
	ms := 8
	if e.cfg != nil && e.cfg.ConnectionRisk.EmitTimeoutMS > 0 {
		ms = e.cfg.ConnectionRisk.EmitTimeoutMS
	}
	return time.Duration(ms) * time.Millisecond
}

// CheckThrottle enforces Phase B absolute per-key RPM cap.
// Returns (blocked, message). Redis errors fail-open (blocked=false).
func (e *ConnectionSignalEmitter) CheckThrottle(ctx context.Context, apiKeyID int64) (bool, string) {
	if e == nil || e.cache == nil || apiKeyID <= 0 {
		return false, ""
	}
	if !e.masterEnabled() {
		return false, ""
	}
	// Soft-throttle marks are honored even when soft_throttle_enabled is later
	// turned off, so active caps continue until TTL expiry or resolve/exempt.
	timeout := e.emitTimeout()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	capRPM, _, ok, err := e.cache.GetThrottle(cctx, apiKeyID)
	if err != nil || !ok || capRPM <= 0 {
		return false, ""
	}
	n, err := e.cache.IncrThrottleCount(cctx, apiKeyID)
	if err != nil {
		return false, ""
	}
	if n > capRPM {
		return true, "Connection risk soft throttle active for this API key"
	}
	return false, ""
}

// ShouldIncludePath reports whether a request path should emit under current settings.
func (e *ConnectionSignalEmitter) ShouldIncludePath(ctx context.Context, isReadOnly bool) bool {
	if e == nil || !e.masterEnabled() {
		return false
	}
	s := e.cachedSettings(ctx)
	if !s.Enabled || !s.EmitEnabled {
		return false
	}
	if isReadOnly && !s.IncludeReadOnlyEndpoints {
		return false
	}
	return true
}
