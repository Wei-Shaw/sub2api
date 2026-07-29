package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// ConnectionSignal is one authenticated gateway request fingerprint.
type ConnectionSignal struct {
	UserID    int64
	APIKeyID  int64
	IP        string // already NormalizeClientIPForSecurity
	UAHash    string
	KeyPrefix string // optional short display prefix (non-secret)
	NowUnix   int64  // optional; cache fills from Redis TIME when 0
}

// ConnectionSignalCache writes/reads cr:* Redis structures for connection risk.
type ConnectionSignalCache interface {
	// EmitAlwaysOn writes Tier A always-on signals (rules authority).
	// cmdCount is the number of Redis commands enqueued (for budget tests).
	EmitAlwaysOn(ctx context.Context, sig ConnectionSignal, maxActive int, pruneEveryN int, emitSeq uint64) (cmdCount int, err error)
	// EmitEvidence writes Tier B sampled evidence sets (non-authoritative).
	EmitEvidence(ctx context.Context, sig ConnectionSignal) error
	// IncrSessionMismatch increments R7 sb_mismatch counter for a user.
	IncrSessionMismatch(ctx context.Context, userID int64) error
	// PruneActive removes stale members and enforces ZCARD cap.
	PruneActive(ctx context.Context, maxActive int, olderThan time.Duration) error
	// ActiveCards returns ZCARD of active key/user sets.
	ActiveCards(ctx context.Context) (keys int64, users int64, err error)
	// ReadKeyWindowMetrics loads rule inputs for one API key (worker path).
	ReadKeyWindowMetrics(ctx context.Context, keyID, userID int64, nowUnix int64) (*ConnectionRiskSubjectMetrics, error)
	// TryDedupe acquires a short-lived dedupe key; true means first writer.
	TryDedupe(ctx context.Context, dedupeKey string, ttl time.Duration) (bool, error)
	// IsExempt reports whether subject is temporarily exempt in Redis.
	IsExempt(ctx context.Context, scope string, id int64) (bool, error)
	// SetExempt stores an exemption with optional TTL (0 = no expiry, use long default).
	SetExempt(ctx context.Context, scope string, id int64, reason string, ttl time.Duration) error
	// ClearExempt removes an exemption.
	ClearExempt(ctx context.Context, scope string, id int64) error
	// ListActiveKeys returns up to limit recently active API key IDs.
	ListActiveKeys(ctx context.Context, limit int) ([]int64, error)
	// ListActiveUsers returns up to limit recently active user IDs.
	ListActiveUsers(ctx context.Context, limit int) ([]int64, error)
	// GetKeyOwner returns the userID last seen using this API key (0 if unknown).
	GetKeyOwner(ctx context.Context, keyID int64) (int64, error)
	// GetKeyPrefix returns a cached non-secret key display prefix.
	GetKeyPrefix(ctx context.Context, keyID int64) (string, error)
	// TrimUAWindow forces ZREMRANGEBYSCORE on uas:1h before scoring.
	TrimUAWindow(ctx context.Context, keyID int64, nowUnix int64) error
	// SetThrottle marks a key for absolute RPM cap (Phase B).
	SetThrottle(ctx context.Context, keyID int64, capRPM int, untilUnix int64) error
	// ClearThrottle removes throttle mark.
	ClearThrottle(ctx context.Context, keyID int64) error
	// GetThrottle returns cap and until; ok=false if not throttled.
	GetThrottle(ctx context.Context, keyID int64) (capRPM int, untilUnix int64, ok bool, err error)
	// IncrThrottleCount increments per-key throttle counter for current minute.
	IncrThrottleCount(ctx context.Context, keyID int64) (int, error)
	// SnapshotBaselineDay stores PFCOUNT 24h for Phase B R3 (day key YYYYMMDD).
	SnapshotBaselineDay(ctx context.Context, keyID int64, day string, count int64) error
	// LoadBaselineSamples returns last N day PFCOUNT snapshots for a key.
	LoadBaselineSamples(ctx context.Context, keyID int64, days []string) ([]int64, error)
	// SetBaselineP95 stores computed p95.
	SetBaselineP95(ctx context.Context, keyID int64, p95 float64, sampleDays int) error
	// GetBaselineP95 loads stored p95; ok=false if missing.
	GetBaselineP95(ctx context.Context, keyID int64) (p95 float64, sampleDays int, ok bool, err error)
}

// HashUserAgent returns a stable 16-hex-char fingerprint of the User-Agent.
func HashUserAgent(ua string) string {
	trimmed := strings.ToLower(strings.TrimSpace(ua))
	if trimmed == "" {
		return "empty"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])[:16]
}
