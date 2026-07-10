package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// blindSpotKey is a key shape that the PATTERN passes alone miss (the documented
// redaction blind spots: 12-19 chars / pure-letter >=20 / pure-digit >=20), plus a
// control shape the pattern passes DO catch. Every value here is >= videoKnownSecretMinLen
// so the key-aware strip covers it.
type blindSpotKey struct {
	name string
	key  string
	// patternAloneLeaks is true when the bare PATTERN redactor (no known key) fails to
	// strip this key from neutral prose — i.e. it is a genuine blind spot that ONLY the
	// key-aware strip closes. The control row sets this false (the pattern already catches it).
	patternAloneLeaks bool
}

func blindSpotKeyCases() []blindSpotKey {
	return []blindSpotKey{
		// 12-19 char band: below the videoOpaqueTokenMinLen(20) floor, no recognized prefix.
		{"mixed_14_chars", "Ab3Cd6Ef9Gh2Jk", true},
		{"mixed_19_chars", "Ab3Cd6Ef9Gh2Jk5Lm8N", true},
		// pure-letter, >=20, non-hex letters, <48 => no pattern fires (looksLikeOpaqueVideoSecret
		// needs a digit too).
		{"pure_letter_20", "AbCdGfGhJkLmNpQrStUv", true},
		// pure-digit, >=20 but <32 => below the hex idx5 floor; looksLikeOpaqueVideoSecret needs a letter.
		{"pure_digit_20", "12345678901234567890", true},
		// Control: mixed-case, digit+letter, >=20 => caught by the VIDEO-LOCAL opaque pass even
		// WITHOUT the known key. Proves the suite is not trivially passing.
		{"control_mixed_24", "Ab3Cd6Ef9Gh2Jk5Lm8Np1Qr4", false},
	}
}

// neutralEcho wraps a key in prose with no Bearer / field-name / URL trigger, so the
// pattern passes have nothing to anchor on for a blind-spot shape.
func neutralEcho(key string) string {
	return "upstream rejected credential " + key + " please retry later"
}

// TestKnownKeyRedactionClosesPatternBlindSpots proves two things per shape:
//  1. the PATTERN redactor alone LEAKS the blind-spot key (the gap is real), and
//  2. the KEY-AWARE redactor strips it in every echo shape (the gap is closed).
func TestKnownKeyRedactionClosesPatternBlindSpots(t *testing.T) {
	for _, bc := range blindSpotKeyCases() {
		t.Run(bc.name, func(t *testing.T) {
			echoes := []string{
				bc.key,                         // bare
				neutralEcho(bc.key),            // prose
				`{"k":"` + bc.key + `"}`,       // innocuous JSON value
				`{"api_key":"` + bc.key + `"}`, // sensitive JSON value (idx1 anchor gap)
				`{"error":{"message":"bad ` + bc.key + ` key"}}`, // nested error.message
			}

			// (1) pattern-alone behaviour on neutral prose: blind spots must leak; control must not.
			patternOnly := redactVideoUpstreamSecrets(neutralEcho(bc.key))
			leaked := strings.Contains(patternOnly, bc.key)
			if bc.patternAloneLeaks && !leaked {
				t.Fatalf("expected the pattern redactor alone to LEAK blind-spot key (gap proof), but it stripped it: %q", patternOnly)
			}
			if !bc.patternAloneLeaks && leaked {
				t.Fatalf("control key should be caught by the pattern passes, but leaked: %q", patternOnly)
			}

			// (2) key-aware path must strip the key in EVERY echo shape.
			for _, e := range echoes {
				out := redactVideoUpstreamSecretsForKey(e, bc.key)
				if strings.Contains(out, bc.key) {
					t.Fatalf("key-aware redaction leaked the key: in=%q out=%q", e, out)
				}
				if !strings.Contains(out, "已脱敏") {
					t.Fatalf("expected redaction marker after stripping the key: in=%q out=%q", e, out)
				}
			}
		})
	}
}

// TestKnownKeyRedactionDoesNotOverRedact guards the existing debuggability contract: the
// key-aware strip must touch ONLY the configured key, never ordinary words / ids / numeric
// runs that merely share a blind-spot shape. (Mirrors the negatives in
// TestRedactVideoUpstreamSecretsOpaqueToken, now under the key-aware entry point.)
func TestKnownKeyRedactionDoesNotOverRedact(t *testing.T) {
	const someOtherKey = "ZZdifferentkey99887766" // the configured key is NOT present in the bodies below
	keep := []string{
		"the quick brown fox jumps over the lazy dog several times today",
		"antidisestablishmentarianism",                                  // 28-char pure-letter word
		`{"id":"cgt-1234","status":"queued"}`,                           // short ids/statuses
		"1234567890123456789012",                                        // 22-digit run (timestamp/counter)
		"ark-content.cn-beijing.volces.com/v/tasks/cgt-20250615/ok.mp4", // result url path
	}
	for _, body := range keep {
		out := redactVideoUpstreamSecretsForKey(body, someOtherKey)
		if !strings.Contains(out, strings.Fields(body)[0]) && !strings.Contains(out, body) {
			// loose check: at least the leading token must survive
			t.Fatalf("over-redacted inspectable content: in=%q out=%q", body, out)
		}
		if strings.Contains(out, "已脱敏") {
			t.Fatalf("known-key strip over-redacted (marker present though the key is absent): in=%q out=%q", body, out)
		}
	}

	// A sub-floor key (< videoKnownSecretMinLen) is intentionally NOT stripped as an exact
	// substring (would chew through ordinary text); it is the pre-arm self-check that
	// fail-closes for such a key instead. Document that here.
	if got := stripKnownVideoSecret("xx short yy", "abc"); got != "xx short yy" {
		t.Fatalf("sub-floor key must not be stripped as a substring, got %q", got)
	}
}

// --- pre-arm self-check ------------------------------------------------------

// TestSeedancePreArmSelfCheckPassesForCredibleKeys: every blind-spot shape (all >= floor)
// passes the self-check because the key-aware redactor strips it.
func TestSeedancePreArmSelfCheckPassesForCredibleKeys(t *testing.T) {
	for _, bc := range blindSpotKeyCases() {
		acc := &VideoProviderAccount{PlainAPIKey: bc.key}
		if err := seedancePreArmRedactionSelfCheck(acc); err != nil {
			t.Fatalf("%s: credible key should pass the self-check, got %v", bc.name, err)
		}
	}
}

// TestSeedancePreArmSelfCheckFailsClosedForSubFloorKey: a key too short to strip as an exact
// substring AND in a pattern blind spot would leak — the self-check must fail closed.
func TestSeedancePreArmSelfCheckFailsClosedForSubFloorKey(t *testing.T) {
	acc := &VideoProviderAccount{PlainAPIKey: "abcde"} // 5 chars, pure-letter, < floor, no pattern fires
	err := seedancePreArmRedactionSelfCheck(acc)
	if err == nil {
		t.Fatal("expected the self-check to fail closed for a sub-floor blind-spot key")
	}
	if strings.Contains(err.Error(), "abcde") {
		t.Fatalf("self-check error must NOT echo the key value: %v", err)
	}
}

// TestSeedanceCreateAbortsBeforeNetworkWhenSelfCheckFails: the adapter must abort with NO
// socket opened when the configured key would leak.
func TestSeedanceCreateAbortsBeforeNetworkWhenSelfCheckFails(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","status":"queued"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	acc := &VideoProviderAccount{
		ID:               1,
		Provider:         VideoProviderSeedance,
		Enabled:          true,
		APIKeyConfigured: true,
		PlainAPIKey:      "abcde", // sub-floor blind-spot key
		BaseURL:          srv.URL,
		Metadata:         map[string]any{"single_smoke_authorized": true},
	}
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 5}

	_, err := (&seedanceVideoAdapter{}).CreateTask(context.Background(), acc, task)
	if err == nil {
		t.Fatal("expected fail-closed abort, got nil")
	}
	if called {
		t.Fatal("self-check failure must abort BEFORE any network call")
	}
	if strings.Contains(err.Error(), "abcde") {
		t.Fatalf("abort error must not echo the key: %v", err)
	}
}

// --- full Form A worker chain: a blind-spot key never leaks into any channel ---

// seedSmokeAuthorizedSeedanceProvider seeds a routable, smoke-authorized seedance account
// whose decrypted key is `key`, pointed at a hostile upstream `baseURL`.
func seedSmokeAuthorizedSeedanceProvider(repo *memoryVideoGatewayRepo, key, baseURL string) int64 {
	id := repo.nextProviderID
	repo.nextProviderID++
	now := time.Now().UTC()
	repo.providers[id] = &VideoProviderAccount{
		ID:              id,
		Provider:        VideoProviderSeedance,
		DisplayName:     "Seedance Leak Probe",
		Enabled:         true,
		EncryptedAPIKey: "enc:" + key, // noopVideoKeyEncryptor decrypts to `key`
		BaseURL:         baseURL,
		DefaultModel:    "doubao-seedance-2-0-260128",
		Metadata:        map[string]any{"single_smoke_authorized": true},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return id
}

// TestFormABlindSpotKeyNeverLeaksAcrossAllChannels drives the REAL Form A product chain
// (create -> budget gate -> DB -> worker -> submit -> adapter HTTP) against a HOSTILE upstream
// that echoes the plaintext key in a 401 body. For every blind-spot key shape it asserts the
// plaintext key never reaches ANY of: the returned error, the persisted task.ErrorMessage (DB),
// the persisted task-event messages/payloads (DB), or the redacted audit log file. No real
// network/cost: the "upstream" is a local httptest server.
func TestFormABlindSpotKeyNeverLeaksAcrossAllChannels(t *testing.T) {
	for _, bc := range blindSpotKeyCases() {
		t.Run(bc.name, func(t *testing.T) {
			key := bc.key
			// Hostile upstream: reflect the key in a 401 auth-error body (the worst echo channel).
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"message":"invalid api key ` + key + ` rejected"}}`))
			}))
			t.Cleanup(srv.Close)

			auditPath := t.TempDir() + "/audit.log"
			t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
			t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", auditPath)
			t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

			ctx := context.Background()
			repo := newMemoryVideoGatewayRepo()
			providerID := seedSmokeAuthorizedSeedanceProvider(repo, key, srv.URL)
			// Real production DI wrapper with the budget gate ARMED (proves the chain does not
			// bypass the gate): cost 1.5 × 5s = 7.5 <= cap 30 => admitted.
			svc := ProvideVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithVideoBudget(1.5, 30.0), nil, nil, nil)

			task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
				ProviderAccountID: providerID,
				TaskType:          VideoTaskTypeTextToVideo,
				Prompt:            "blind-spot leak probe",
				CreatedBy:         11,
				Duration:          5,
			})
			if err != nil {
				t.Fatalf("create (budget gate should admit): %v", err)
			}
			// Worker dispatches the queued task -> submit -> hostile 401 -> failTask (redacted).
			if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
				t.Fatalf("process: %v", err)
			}

			persisted, events, err := svc.GetTask(ctx, task.ID, 11, true)
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			if persisted.Status != VideoStatusFailed {
				t.Fatalf("expected the hostile 401 to fail the task, got %s", persisted.Status)
			}

			// Channel 1+2: DB task.ErrorMessage must be key-free and non-empty. The hostile body
			// echoes the key, so the adapter's fail-closed echoed-credential guard fires and the
			// reason is a generic, key-free abort — the marker is asserted on the audited body
			// below (where redaction actually ran), not on this generic failure string.
			if strings.Contains(persisted.ErrorMessage, key) {
				t.Fatalf("DB task.ErrorMessage leaked the key: %q", persisted.ErrorMessage)
			}
			if persisted.ErrorMessage == "" {
				t.Fatal("expected a recorded failure reason in task.ErrorMessage")
			}
			// Channel 3: every event (message + payload) must be key-free.
			for _, ev := range events {
				if strings.Contains(ev.Message, key) {
					t.Fatalf("event message leaked the key: %q", ev.Message)
				}
				if strings.Contains(fmtAny(ev.Payload), key) {
					t.Fatalf("event payload leaked the key: %#v", ev.Payload)
				}
			}
			// Channel 4: the redacted audit log file must be key-free, exist, AND show the
			// redaction marker (the audited body carried the key and must have been redacted).
			raw, readErr := os.ReadFile(auditPath)
			if readErr != nil {
				t.Fatalf("audit log not written: %v", readErr)
			}
			if strings.Contains(string(raw), key) {
				t.Fatalf("redacted audit log leaked the key: %q", string(raw))
			}
			if !strings.Contains(string(raw), "已脱敏") {
				t.Fatalf("expected the audited body to be redacted (marker), got %q", string(raw))
			}
		})
	}
}

// TestFormASuccessBodyEchoingKeyAsStructuralIDNeverLeaks closes the cross-family (Codex)
// blocker: a SUCCESS create response that echoes the configured key as the STRUCTURAL `id`
// field — {"id":"<key>","status":"submitted"} — would otherwise store the key raw as
// upstream_task_id (DB), in the routed/submitted event payload (upstream_id), and in the API
// response, bypassing the body/message redactor (which only covers error strings). The
// fail-closed echoed-credential guard must abort instead, so the key never reaches ANY
// persisted field. The key here is a 14-char pattern blind-spot shape, so only the
// known-value machinery (not the pattern passes) can catch it.
func TestFormASuccessBodyEchoingKeyAsStructuralIDNeverLeaks(t *testing.T) {
	const key = "Ab3Cd6Ef9Gh2Jk" // 14-char blind-spot key, ALSO returned as the upstream id
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"` + key + `","status":"submitted"}`)) // hostile: key as structural id
	}))
	t.Cleanup(srv.Close)

	auditPath := t.TempDir() + "/audit.log"
	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", auditPath)
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, key, srv.URL)
	svc := ProvideVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithVideoBudget(1.5, 30.0), nil, nil, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "structural id leak probe",
		CreatedBy:         12,
		Duration:          5,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
		t.Fatalf("process: %v", err)
	}

	persisted, events, err := svc.GetTask(ctx, task.ID, 12, true)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	// The key (echoed as the structural id) must NOT be persisted in ANY field.
	if strings.Contains(persisted.UpstreamTaskID, key) {
		t.Fatalf("upstream_task_id (DB) leaked the key: %q", persisted.UpstreamTaskID)
	}
	if strings.Contains(persisted.ErrorMessage, key) {
		t.Fatalf("task.ErrorMessage (DB) leaked the key: %q", persisted.ErrorMessage)
	}
	if persisted.Status != VideoStatusFailed {
		t.Fatalf("a key-echoing success body must fail closed (no persisted task id), got status %s", persisted.Status)
	}
	for _, ev := range events {
		if strings.Contains(ev.Message, key) || strings.Contains(fmtAny(ev.Payload), key) {
			t.Fatalf("event leaked the key: msg=%q payload=%#v", ev.Message, ev.Payload)
		}
	}
	raw, _ := os.ReadFile(auditPath)
	if strings.Contains(string(raw), key) {
		t.Fatalf("audit log leaked the key (structural id): %q", string(raw))
	}
}

// TestSeedanceCreateFailsClosedWhenEscapedKeyEchoedAsID proves the escape-proof structural
// guard: a JSON-unicode-escaped first char in the `id` means the RAW response bytes do NOT
// literally contain the key (defeating a raw-substring check), yet json decodes parsed.ID
// back to the key. The post-parse structural guard must still fail-closed so the decoded key
// never becomes upstream_task_id.
func TestSeedanceCreateFailsClosedWhenEscapedKeyEchoedAsID(t *testing.T) {
	const key = "Ab3Cd6Ef9Gh2Jk"
	// Build the body at runtime so no literal backslash-u sequence sits in source: a single
	// backslash (rune 92) + "u0041" is the JSON escape for 'A', so the bytes read
	// `{"id":"Ab3Cd6Ef9Gh2Jk",...}` — which does NOT contain the key — but json decodes
	// the id back to "Ab3Cd6Ef9Gh2Jk".
	escapedID := string(rune(92)) + "u0041" + key[1:] // A + "b3Cd6Ef9Gh2Jk"
	rawBody := `{"id":"` + escapedID + `","status":"submitted"}`
	if strings.Contains(rawBody, key) {
		t.Fatal("test setup error: the raw body must NOT literally contain the key (it must be JSON-escaped)")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rawBody))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	acc := &VideoProviderAccount{
		ID:               1,
		Provider:         VideoProviderSeedance,
		Enabled:          true,
		APIKeyConfigured: true,
		PlainAPIKey:      key,
		BaseURL:          srv.URL,
		Metadata:         map[string]any{"single_smoke_authorized": true},
	}
	res, err := (&seedanceVideoAdapter{}).CreateTask(context.Background(), acc, &VideoTask{
		Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 5,
	})
	if err == nil {
		t.Fatal("expected fail-closed when the escaped key decodes to the id")
	}
	if res != nil && strings.Contains(res.UpstreamTaskID, key) {
		t.Fatalf("escaped key leaked into upstream_task_id: %q", res.UpstreamTaskID)
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("abort error leaked the key: %v", err)
	}
}
