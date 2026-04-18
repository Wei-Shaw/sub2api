package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// usage_poll loop tuning ---------------------------------------------------

// sidecarProbeMinIntervalFloor is the smallest min_interval_seconds wire.go
// will honor. Anything tighter risks correlating with token-refresh and
// account-health periodics, producing a synchronized spike pattern that's
// itself a fingerprintable signal.
const sidecarProbeMinIntervalFloor = 60 * time.Second

// sidecarProbeRequestTimeout bounds a single /api/oauth/usage probe
// (DB list + one upstream HTTPS round trip). Must be < sidecarProbeMinIntervalFloor
// so a stuck probe never overlaps the next tick.
const sidecarProbeRequestTimeout = 30 * time.Second

// count_tokens injection --------------------------------------------------

// countTokensSidecarDefaultTimeoutMs caps each fire-and-forget count_tokens
// sidecar request when the operator config leaves timeout_milliseconds
// unset/non-positive. 3000 matches the real Claude CLI's count_tokens
// budget (the editor blocks on it for the status-bar hint).
const countTokensSidecarDefaultTimeoutMs = 3000

// startup probe (post token-refresh) --------------------------------------

// startupProbeModel mirrors what real Claude Code sends as its first
// /v1/messages after auth. claude-haiku-4-5 is the short alias the CLI
// emits before model normalization.
const startupProbeModel = claude.HaikuModelShort

const (
	startupProbeUserRole       = "user"
	startupProbeUserContent    = "hi"
	startupProbeMaxTokens      = 1
	startupProbeDefaultTimeout = 10 * time.Second
)
