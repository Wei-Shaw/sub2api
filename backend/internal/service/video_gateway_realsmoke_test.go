//go:build realsmoke

// FORM B — single-shot REAL Seedance (Volcengine Ark) smoke test.
//
// !!! THIS FILE MAKES A REAL, BILLED UPSTREAM CALL WHEN ARMED !!!
//
// It is excluded from the normal test binary by the `realsmoke` build tag, so
// `go test ./...` can NEVER compile or run it. Arming requires ALL of:
//
//	 1. compile WITH the tag:      go test -tags realsmoke ...
//	 2. set the run flag:          SUB2API_SEEDANCE_REAL_SMOKE_RUN=1   (else t.Skip)
//	 3. the real key in env:       SUB2API_SEEDANCE_SMOKE_API_KEY=...  (read, never logged)
//	 4. the adapter smoke gates (re-checked fail-closed inside the adapter):
//	      SUB2API_VIDEO_REAL_SMOKE_ENABLED=1
//	      SUB2API_VIDEO_REDACTED_EVENT_LOG=<git-ignored path>
//	      SUB2API_VIDEO_URL_ALLOWLIST=<trusted Ark CDN suffix(es)>
//
// Absent ANY of 1-4 the test is inert: no socket is opened, no key is read.
//
// Form B calls the seedanceVideoAdapter STRUCT directly — it does NOT go through
// VideoGatewayService / the worker / the Hono gateway. Consequences (by design):
//   - no DB write, no daily-limit accounting, no team-credit deduction (CB3);
//   - the poll loop is driven HERE with its own hard ceiling; the production
//     worker has its own per-task poll cap (VA2: video_gateway.max_poll_attempts);
//   - exactly ONE Create is issued; Poll count is bounded by a hard ceiling.
//
// The plaintext API key is read from the environment at runtime and is NEVER
// hard-coded here, never printed, and never logged (PlainAPIKey is json:"-" and
// is redacted by VideoProviderAccount.String/GoString/LogValue).
package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// realSmokePollHardCeiling is the absolute upper bound on poll iterations,
// independent of any env override. VA2: at the default 5s interval this is a
// 72×5s=360s poll window — ≥2× the observed ~170s Seedance generation time, so a
// real clip finishes before the cap (the prior 30×5s=150s gave up too early). The
// production worker now enforces its own per-task cap (video_gateway.max_poll_attempts);
// this manual loop keeps an independent hard ceiling that env can only lower.
const realSmokePollHardCeiling = 72

func TestSeedanceSingleRealSmokeFormB(t *testing.T) {
	// --- Safety layer 2: explicit human run flag (layer 1 is the build tag). ---
	if strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_REAL_SMOKE_RUN")) != "1" {
		t.Skip("real smoke disarmed: set SUB2API_SEEDANCE_REAL_SMOKE_RUN=1 to arm the single billed call")
	}

	// The scholar exports the real key into the environment; this process reads
	// it but never prints it. Empty => abort loudly (do NOT silently pass).
	apiKey := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_API_KEY"))
	if apiKey == "" {
		t.Fatal("SUB2API_SEEDANCE_SMOKE_API_KEY is empty; export the real key before arming (never commit it)")
	}

	// Pre-arm, fail-closed self-check (closes adversarial finding redact-gap-opaque-token).
	// The pattern passes have a known gap: an opaque, delimiter-less, sub-48-char mixed-case
	// token, or a 12-19 char / pure-letter / pure-digit key, passes through UNREDACTED. The
	// Ark key shape is UNVERIFIED. If a 401/403 body or a 200-OK error.message echoes the bare
	// key, it flows through the redactor into the audit log AND this test's output (the dump
	// and the LAST ERROR MESSAGE line). So prove, BEFORE opening any socket, that the KEY-AWARE
	// redactor (redactVideoUpstreamSecretsForKey — the same path the adapter now uses, which
	// strips the exact configured key shape-agnostically) actually removes THIS key in every
	// shape an upstream echo could take; otherwise abort. The adapter re-runs this same check
	// internally (seedancePreArmRedactionSelfCheck) as the authoritative fail-closed gate; this
	// is the earliest, loudest copy. The key value is NEVER printed here — only a generic abort.
	for _, probe := range []string{
		apiKey,
		"Bearer " + apiKey,
		`{"message":"` + apiKey + `"}`,
		`{"error":{"message":"rejected token ` + apiKey + `"}}`,
	} {
		if strings.Contains(redactVideoUpstreamSecretsForKey(probe, apiKey), apiKey) {
			t.Fatal("ABORT before any network call: the key-aware redactor does NOT strip the configured " +
				"key, so an upstream echo could leak the real key into the audit log / test output. " +
				"Use a key of length >= videoKnownSecretMinLen (or harden the redactor) before arming. " +
				"(key value intentionally not printed)")
		}
	}

	// Assert — do NOT set — the adapter smoke gates, so a misconfiguration fails
	// with a clear message instead of a silent VIDEO_PROVIDER_DISABLED. These
	// MUST be opened in the real environment by the scholar at the smoke scene
	// and reset immediately afterwards. The test never opens a gate for you.
	realSmokeRequireEnvEquals(t, "SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	realSmokeRequireEnvSet(t, "SUB2API_VIDEO_REDACTED_EVENT_LOG")
	realSmokeRequireEnvSet(t, "SUB2API_VIDEO_URL_ALLOWLIST")

	// duration: gate enforces 1..5; default 5 (a single tiny clip).
	duration := 5
	if v := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_DURATION")); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			duration = d
		}
	}

	// poll cap: env may LOWER it; it can never exceed the hard ceiling.
	maxPolls := realSmokePollHardCeiling
	if v := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_MAX_POLLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < maxPolls {
			maxPolls = n
		}
	}

	// poll interval: default 5s, floor 2s (avoid hammering upstream).
	pollInterval := 5 * time.Second
	if v := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_POLL_INTERVAL_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 {
			pollInterval = time.Duration(n) * time.Second
		}
	}

	model := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_MODEL"))
	if model == "" {
		model = "doubao-seedance-2-0-260128"
	}
	prompt := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_PROMPT"))
	if prompt == "" {
		prompt = "a single small cube slowly rotating on a plain neutral background"
	}

	baseURL := strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_BASE_URL")) // "" => real Ark default
	effectiveURL := baseURL
	if effectiveURL == "" {
		effectiveURL = "https://ark.cn-beijing.volces.com/api/v3 (adapter default)"
	}

	adapter := &seedanceVideoAdapter{}
	acc := &VideoProviderAccount{
		ID:               1,
		Provider:         VideoProviderSeedance,
		Enabled:          true,
		APIKeyConfigured: true,
		PlainAPIKey:      apiKey, // from env; never logged (json:"-" + redacted String/LogValue)
		BaseURL:          baseURL,
		Metadata:         map[string]any{"single_smoke_authorized": true},
	}
	task := &VideoTask{
		Provider:    VideoProviderSeedance,
		Model:       model,
		TaskType:    VideoTaskTypeTextToVideo,
		Prompt:      prompt,
		Duration:    duration,
		Resolution:  strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_RESOLUTION")), // optional
		AspectRatio: strings.TrimSpace(os.Getenv("SUB2API_SEEDANCE_SMOKE_ASPECT")),     // optional
	}

	t.Logf("=== FORM B SINGLE REAL SMOKE ARMED ===")
	t.Logf("model=%q task_type=%q duration=%ds maxPolls=%d interval=%s url=%s",
		model, task.TaskType, duration, maxPolls, pollInterval, effectiveURL)

	// ---------------- THE SINGLE BILLED CREATE (exactly one) ----------------
	createRes, err := adapter.CreateTask(context.Background(), acc, task)
	if err != nil {
		// CreateTask fails closed on any gate/SSRF/HTTP/audit problem and makes no
		// poll. A gate failure surfaces here as VIDEO_PROVIDER_DISABLED.
		t.Fatalf("CreateTask failed (no poll attempted): %v", err)
	}
	task.UpstreamTaskID = createRes.UpstreamTaskID
	task.Status = createRes.Status
	t.Logf("CREATE ok: upstream_id=%q normalized_status=%q", createRes.UpstreamTaskID, createRes.Status)
	if strings.TrimSpace(createRes.UpstreamTaskID) == "" {
		// json.Unmarshal silently leaves unmatched fields zero, so an empty id is
		// the signal that Ark's create-response 'id' field name differs from the
		// adapter's json:"id" tag. Confirm against the redacted audit log dump.
		t.Errorf("WARNING empty upstream_id: Ark create 'id' field name may differ from adapter json:\"id\"; inspect the audit log dump below")
	}

	// ------------- MANUAL, HARD-CAPPED POLL LOOP (VA2 guard) ----------------
	var lastStatus, resultURL, lastErrMsg string
	polls := 0
	for polls < maxPolls {
		polls++
		time.Sleep(pollInterval)
		pollRes, perr := adapter.PollTask(context.Background(), acc, task)
		if perr != nil {
			t.Fatalf("PollTask #%d failed: %v", polls, perr)
		}
		task.Status = pollRes.Status
		lastStatus = pollRes.Status
		lastErrMsg = pollRes.ErrorMessage
		if pollRes.ResultURL != "" {
			resultURL = pollRes.ResultURL
		}
		t.Logf("POLL #%d/%d: status=%q result_url_present=%t err_msg=%q",
			polls, maxPolls, pollRes.Status, pollRes.ResultURL != "", pollRes.ErrorMessage)
		if IsTerminalVideoStatus(pollRes.Status) {
			break
		}
	}

	t.Logf("=== SMOKE DONE: creates=1 polls=%d last_status=%q ===", polls, lastStatus)
	// result_url is the SSRF/allowlist-validated, playable Ark CDN URL (not a
	// secret) — print it so the scholar can confirm "result_url 可播".
	t.Logf("RESULT URL (validated, playable): %q", resultURL)
	if lastErrMsg != "" {
		t.Logf("LAST ERROR MESSAGE (already redacted): %q", lastErrMsg)
	}
	if polls >= maxPolls && !IsTerminalVideoStatus(lastStatus) {
		t.Logf("NOTE hit poll hard-cap (%d) before a terminal status; stopped deliberately — NOT a runaway.", maxPolls)
		// last_status is the NORMALIZED value; NormalizeStatus maps unknown/empty tokens to
		// "running" (never empty). Before calling this a timeout, read the RAW `status` value
		// in the audit dump below: if Ark returned a terminal token the adapter does not map
		// (e.g. done/finished/complete), the task likely FINISHED — extend NormalizeStatus and
		// DO NOT bill again. (adversarial findings: status-fieldname-mismatch-silent,
		// unknown-status-token-masquerades-as-running)
		t.Logf("      last_status=%q is NORMALIZED; confirm the RAW `status` token in the audit dump before treating as timeout.", lastStatus)
	}

	// Field-name verification channel: dump the redacted audit log so the scholar
	// can confirm the UNVERIFIED Ark field NAMES against the real JSON keys:
	//   request side : duration / resolution / aspect_ratio
	//   create resp  : id / status
	//   poll resp    : id / status / content.video_url (nested) / error.message
	// Values may be redacted, but the JSON KEYS survive and are what we verify.
	realSmokeDumpAuditLog(t)
}

// realSmokeRequireEnvEquals fails the test (with actionable guidance) unless the
// named env var equals want. It never sets the var — the scholar opens gates.
func realSmokeRequireEnvEquals(t *testing.T, key, want string) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(key)) != want {
		t.Fatalf("gate %s must equal %q at the smoke scene (open it in the env, reset after); got %q",
			key, want, os.Getenv(key))
	}
}

// realSmokeRequireEnvSet fails the test unless the named env var is non-empty.
func realSmokeRequireEnvSet(t *testing.T, key string) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(key)) == "" {
		t.Fatalf("gate %s must be set at the smoke scene (open it in the env, reset after)", key)
	}
}

// realSmokeDumpAuditLog prints the already-redacted JSONL audit trail written by
// appendRedactedVideoEvent. It is safe to print (secrets are stripped before the
// line is written) and is the scholar's evidence for both the audit-line count
// and the Ark field-name confirmation.
func realSmokeDumpAuditLog(t *testing.T) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG"))
	if path == "" {
		t.Logf("audit log path empty; cannot dump (gate should have caught this)")
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Logf("could not read audit log %q: %v", path, err)
		return
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	t.Logf("=== REDACTED AUDIT LOG (%d line(s); expect 1 create + N poll) ===", len(lines))
	for i, ln := range lines {
		t.Logf("audit[%d]: %s", i, ln)
		// The body is truncate(redact(rawBody), 1000): a 1000-char cap applied AFTER redaction.
		// If a needed field-name key (status / content.video_url / error.message) sits past the
		// cut it is replaced by "...(N more)" and field-name verification for that key is INVALID.
		// (adversarial finding: audit-truncation-can-drop-the-keys-being-verified)
		if strings.Contains(ln, "...(") {
			t.Log("  ^ WARNING this audit body was TRUNCATED (1000-char cap): JSON keys past the cut are NOT shown; " +
				"field-name verification for any missing key is INVALID. Re-run with a shorter prompt if a needed key is hidden.")
		}
	}
}
