//go:build realsmoke

// FORM A — single-shot REAL Seedance smoke through the FULL PRODUCT CHAIN.
//
// !!! THIS FILE MAKES A REAL, BILLED UPSTREAM CALL WHEN ARMED !!!
//
// Unlike Form B (video_gateway_realsmoke_test.go), which calls the seedanceVideoAdapter
// struct directly and bypasses the service/worker/DB/budget gate, Form A drives the
// SAME path production uses:
//
//	svc.CreateTask → VA1 budget gate → DB persist → worker submit → worker poll → result
//
// so it is the harness that proves "the real budget gate ADMITS the single smoke" — the
// thing only Form A can show (the gate lives only on the Form A path). It is excluded from
// the normal test binary by the `realsmoke` build tag, so `go test ./...` can NEVER compile
// or run it.
//
// This is the B-2 "switch": built here, INERT until armed. Arming requires ALL of:
//
//	1. compile WITH the tag:   go test -tags realsmoke ...
//	2. set the run flag:        SUB2API_SEEDANCE_FORMA_REAL_SMOKE_RUN=1   (else t.Skip)
//	3. the real key in env:     SUB2API_SEEDANCE_SMOKE_API_KEY=...        (read, never logged)
//	4. the adapter smoke gates (asserted, not set, inside the chain):
//	     SUB2API_VIDEO_REAL_SMOKE_ENABLED=1
//	     SUB2API_VIDEO_REDACTED_EVENT_LOG=<git-ignored path>
//	     SUB2API_VIDEO_URL_ALLOWLIST=<trusted Ark CDN suffix(es)>
//
// Absent ANY of 1-4 the test is inert: no socket is opened, no key is read. The budget
// gate is ARMED from cost/cap (defaults 1.5 / 30, env-overridable) so a normal single 5s
// clip is admitted while an over-budget burst is rejected.
//
// The plaintext key is read from the environment at runtime, is NEVER hard-coded here,
// never printed, and never logged (PlainAPIKey is json:"-" and is redacted by the account
// String/GoString/LogValue; the adapter redacts every upstream echo key-aware).
package service

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSeedanceSingleRealSmokeFormA(t *testing.T) {
	// --- Safety layer 2: explicit human run flag (layer 1 is the build tag). ---
	if strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_FORMA_REAL_SMOKE_RUN")) != "1" {
		t.Skip("form A real smoke disarmed: set SUB2API_SEEDANCE_FORMA_REAL_SMOKE_RUN=1 to arm the single billed call through the full product chain")
	}

	apiKey := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_API_KEY"))
	if apiKey == "" {
		t.Fatal("SUB2API_SEEDANCE_SMOKE_API_KEY is empty; export the real key before arming (never commit it)")
	}

	// Assert — do NOT set — the adapter smoke gates so a misconfiguration fails with a clear
	// message instead of a silent VIDEO_PROVIDER_DISABLED. The scholar opens these at the
	// smoke scene and resets them immediately afterwards.
	realSmokeRequireEnvEquals(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	realSmokeRequireEnvSet(t, "SUB2API_VIDEO_REDACTED_EVENT_LOG")
	realSmokeRequireEnvSet(t, "SUB2API_VIDEO_URL_ALLOWLIST")

	// The adapter (seedancePreArmRedactionSelfCheck) re-checks fail-closed that THIS key is
	// stripped in every echo shape before opening a socket; arming a sub-floor key aborts there.

	duration := 5
	if v := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_DURATION")); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			duration = d
		}
	}
	model := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_MODEL"))
	if model == "" {
		model = "doubao-seedance-2-0-260128"
	}

	// Budget gate ARMED from config (defaults match the B-2 production config: cost 1.5,
	// cap 30 — admits a single 5s clip at 7.5, rejects an over-budget burst).
	cost := envFloatOr(t, "SUB2API_SEEDANCE_FORMA_COST_PER_SECOND", 1.5)
	perCallCap := envFloatOr(t, "SUB2API_SEEDANCE_FORMA_PER_CALL_BUDGET", 30.0)
	// Refuse to fire a REAL billed call with the budget gate neutralized: a non-positive cap
	// leaves the guard nil (unarmed), and a non-positive cost makes every estimate 0 (gate can
	// never brake). Both must be strictly positive so the gate genuinely guards this real shot.
	if cost <= 0 || perCallCap <= 0 {
		t.Fatalf("refusing to arm a REAL smoke with a neutralized budget gate (cost/s=%.3f, per_call_cap=%.3f); both must be > 0", cost, perCallCap)
	}
	// The estimate (cost × duration) must be a positive number the cap can actually brake on,
	// and within the cap so the legitimate single clip is admitted (not silently rejected).
	if est := cost * float64(duration); est <= 0 || est > perCallCap {
		t.Fatalf("budget gate misconfigured for a single clip: estimate %.3f (cost %.3f × %ds) must be in (0, cap %.3f]", est, cost, duration, perCallCap)
	}

	// Poll cadence: real Ark must not be hammered. Default 5s, floor 2s.
	pollInterval := 5 * time.Second
	if v := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_POLL_INTERVAL_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 {
			pollInterval = time.Duration(n) * time.Second
		}
	}

	// baseURL "" => the seedance adapter's real Ark default. THIS is the only switch flip
	// vs the in-memory mock proof; everything else is identical production code.
	repo, svc, providerID := newFormAHarnessSvc(t, "" /* real Ark */, apiKey, cost, perCallCap)

	aspect := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_ASPECT"))

	t.Logf("=== FORM A SINGLE REAL SMOKE ARMED (full product chain) ===")
	t.Logf("model=%q duration=%ds aspect=%q cost/s=%.3f per_call_cap=%.3f poll_interval=%s url=%s",
		model, duration, aspect, cost, perCallCap, pollInterval, "https://ark.cn-beijing.volces.com/api/v3 (adapter default)")

	res := driveFormASeedanceChain(t, repo, svc, providerID, formAChainOpts{
		model:       model,
		duration:    duration,
		aspectRatio: aspect,
		maxTicks:    realSmokePollHardCeiling + 5, // worker also self-caps at video_gateway.max_poll_attempts
		tickDelay:   pollInterval,
	})
	if res.createErr != nil {
		// Budget-gate rejection or a fail-closed adapter abort (gate/SSRF/audit/self-check).
		t.Fatalf("Form A create failed (no real generation): %v", res.createErr)
	}

	t.Logf("=== FORM A SMOKE DONE: status=%q poll_count=%d ===", res.task.Status, res.task.PollCount)
	// result_url is the SSRF/allowlist-validated, playable Ark CDN URL (not a secret).
	t.Logf("RESULT URL (validated, playable): %q", res.task.ResultURL)
	if res.task.ErrorMessage != "" {
		t.Logf("LAST ERROR MESSAGE (already redacted): %q", res.task.ErrorMessage)
	}
	if !IsTerminalVideoStatus(res.task.Status) {
		t.Logf("NOTE task did not reach a terminal status within the poll window; stopped deliberately — confirm the RAW status token in the audit dump before treating as timeout.")
	}

	// Field-name verification channel: dump the redacted audit log so the scholar can confirm
	// the UNVERIFIED Ark field NAMES (request duration/resolution/aspect_ratio; create id/status;
	// poll id/status/content.video_url/error.message) against the real JSON keys.
	realSmokeDumpAuditLog(t)
}

// envFloatOr reads a float env var with a default, failing the test on a malformed value
// (so an armed run never silently mis-prices the gate).
func envFloatOr(t *testing.T, key string, def float64) float64 {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		t.Fatalf("%s is not a valid float: %v", key, err)
	}
	return f
}
