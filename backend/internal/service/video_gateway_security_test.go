package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

// fakeSecret is a clearly-fake credential-shaped token used only to prove the
// redactor strips it. It is NOT a real key and never reaches a real provider.
const fakeSecret = "sk-livefakeSECRETtoken1234567890ABCDEF"

// --- B1a: upstream error/response body redaction ----------------------------

func TestRedactVideoUpstreamSecrets(t *testing.T) {
	body := `{"error":{"message":"invalid key Bearer ` + fakeSecret + `","authorization":"` + fakeSecret + `"}}`
	out := redactVideoUpstreamSecrets(body)
	if strings.Contains(out, fakeSecret) {
		t.Fatalf("redaction leaked the secret: %q", out)
	}
	if !strings.Contains(out, "已脱敏") {
		t.Fatalf("expected redaction marker in output, got %q", out)
	}

	// Volcengine-style delimiter-less access key: missed by the shared patterns
	// (no [_-] delimiter, <48 chars, non-hex), caught by the layered AKLT rule.
	aklt := "AKLTabc123DEF456ghi789Zxy"
	out2 := redactVideoUpstreamSecrets(`{"AccessKeyId":"` + aklt + `"}`)
	if strings.Contains(out2, aklt) {
		t.Fatalf("AKLT access key was not redacted: %q", out2)
	}
}

// --- B1a': opaque delimiter-less token redaction (latent gap) ----------------

// TestRedactVideoUpstreamSecretsOpaqueToken closes the adversarial finding
// redact-gap-opaque-token: an opaque, mixed-case, non-hex, delimiter-less token
// of ~20-47 chars with NO recognized prefix slips past every shared pattern
// (idx5 is hex-only; idx6/idx7 require >=48; the field-name idx1 anchor breaks on
// the quote in {"api_key":"..."}) and past the AKLT rule. The VIDEO-LOCAL
// opaque-token pass must strip it in every echo shape — bare, as a JSON value
// under an innocuous key, under a sensitive key, and buried in an error.message
// — WITHOUT over-redacting ordinary words, short ids, numeric ids, or the result
// video_url path segments that must stay inspectable for debugging.
func TestRedactVideoUpstreamSecretsOpaqueToken(t *testing.T) {
	// A 47-char delimiter-less, non-hex, mixed-case, digit-bearing token; the
	// shorter lengths are prefixes of it so every case shares one shape.
	const master = "Ab3Cd6Ef9Gh2Jk5Lm8Np1Qr4St7Uv0Wx3Yz6Ab9Cd2Ef5Gh"
	tok20 := master[:20]
	tok32 := master[:32]
	tok40 := master[:40]
	tok47 := master[:47]
	for _, n := range []struct {
		name string
		tok  string
		want int
	}{{"20", tok20, 20}, {"32", tok32, 32}, {"40", tok40, 40}, {"47", tok47, 47}} {
		if len(n.tok) != n.want {
			t.Fatalf("token %s has length %d, want %d (fix the master string)", n.name, len(n.tok), n.want)
		}
	}

	type tc struct {
		name string
		in   string
		// gone: a substring that MUST NOT survive (a leaked secret). "" => none.
		gone string
		// keep: a substring that MUST survive (inspectable). "" => none.
		keep string
		// marker: whether the redaction marker is expected in the output.
		marker bool
	}
	cases := []tc{
		// --- positives: the opaque token must be stripped --------------------
		{"bare token 20", tok20, tok20, "", true},
		{"bare token 32", tok32, tok32, "", true},
		{"bare token 40", tok40, tok40, "", true},
		{"bare token 47", tok47, tok47, "", true},
		{"json innocuous key", `{"k":"` + tok32 + `"}`, tok32, `"k":`, true},
		{"json api_key value (idx1 anchor gap)", `{"api_key":"` + tok40 + `"}`, tok40, `"api_key":`, true},
		{"nested error.message", `{"error":{"message":"upstream rejected credential ` + tok47 + ` please retry"}}`, tok47, "please retry", true},

		// --- negatives: nothing of value may be redacted ---------------------
		{"ordinary words", "the quick brown fox jumps over the lazy dog several times today", "", "quick brown fox", false},
		{"long dictionary word (no digit)", "antidisestablishmentarianism", "", "antidisestablishmentarianism", false},
		{"short ids and statuses", `{"id":"cgt-1234","status":"queued"}`, "", `"status":"queued"`, false},
		{"delimited task id stays readable", "cgt-20250615-7a8b9c", "", "cgt-20250615-7a8b9c", false},
		{"long pure-digit run (no letter)", "1234567890123456789012", "", "1234567890123456789012", false},
		{"video_url path segments inspectable", `{"video_url":"ark-content.cn-beijing.volces.com/v/tasks/cgt-20250615/ok.mp4"}`, "", "/v/tasks/cgt-20250615/ok.mp4", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := redactVideoUpstreamSecrets(c.in)
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Fatalf("secret survived redaction: in=%q out=%q", c.in, out)
			}
			if c.keep != "" && !strings.Contains(out, c.keep) {
				t.Fatalf("inspectable substring %q was over-redacted: in=%q out=%q", c.keep, c.in, out)
			}
			if c.marker && !strings.Contains(out, "已脱敏") {
				t.Fatalf("expected redaction marker, in=%q out=%q", c.in, out)
			}
			if !c.marker && strings.Contains(out, "已脱敏") {
				t.Fatalf("unexpected over-redaction (marker present), in=%q out=%q", c.in, out)
			}
		})
	}
}

// --- B1b: redacted event log writer -----------------------------------------

func TestAppendRedactedVideoEvent(t *testing.T) {
	logPath := t.TempDir() + "/redacted-events.log"
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", logPath)

	if err := appendRedactedVideoEvent("", "create", 401, `{"msg":"Bearer `+fakeSecret+`"}`); err != nil {
		t.Fatalf("append create: %v", err)
	}
	if err := appendRedactedVideoEvent("", "poll", 200, `{"status":"running"}`); err != nil {
		t.Fatalf("append poll: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	// On non-Windows the audit file must be 0600 (enforced even if pre-existing).
	if runtime.GOOS != "windows" {
		if fi, statErr := os.Stat(logPath); statErr == nil && fi.Mode().Perm() != 0o600 {
			t.Fatalf("redacted log perm = %v, want 0600", fi.Mode().Perm())
		}
	}
	content := string(raw)
	if strings.Contains(content, fakeSecret) {
		t.Fatalf("redacted event log leaked the secret: %q", content)
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d: %q", len(lines), content)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("line 0 is not valid JSON: %v (%q)", err, lines[0])
	}
	if rec["phase"] != "create" || rec["provider"] != VideoProviderSeedance {
		t.Fatalf("unexpected record: %+v", rec)
	}

	// Empty env => no-op (must not panic, must not create a file).
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", "")
	_ = appendRedactedVideoEvent("", "create", 200, "ignored")
}

// --- B3a: SSRF / allowlist URL validation -----------------------------------

func TestValidateExternalVideoURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https public", "https://ark-content.cn-beijing.volces.com/v/a.mp4", false},
		{"empty", "", true},
		{"http rejected", "http://cdn.example.com/a.mp4", true},
		{"file scheme", "file:///etc/passwd", true},
		{"localhost", "https://localhost/a.mp4", true},
		{"loopback ip", "https://127.0.0.1/a.mp4", true},
		{"private 10", "https://10.0.0.5/a.mp4", true},
		{"private 192", "https://192.168.1.10/a.mp4", true},
		{"cloud metadata", "https://169.254.169.254/latest/meta-data", true},
		{"ipv6 loopback", "https://[::1]/a.mp4", true},
		{"dot-internal host", "https://store.internal/a.mp4", true},
		{"decimal ip", "https://2130706433/a.mp4", true},
		{"hex ip", "https://0x7f000001/a.mp4", true},
		{"octal dotted ip", "https://0177.0.0.1/a.mp4", true},
		{"trailing dot loopback", "https://127.0.0.1./a.mp4", true},
		{"trailing dot localhost", "https://localhost./a.mp4", true},
		{"cgnat 100.64/10", "https://100.64.0.1/a.mp4", true},
		{"numeric-leading legit label ok", "https://007.cdn.example.com/a.mp4", false},
		{"backslash parser-differential", "https://169.254.169.254\\@x.cn-beijing.volces.com/a.mp4", true},
		{"userinfo confusion", "https://evil@volces.com/a.mp4", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateExternalVideoURL(c.url)
			if c.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", c.url)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected no error for %q, got %v", c.url, err)
			}
		})
	}
}

func TestValidateExternalVideoURLAllowlist(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com, volccdn.com")
	if err := validateExternalVideoURL("https://x.cn-beijing.volces.com/a.mp4"); err != nil {
		t.Fatalf("allowlisted host should pass: %v", err)
	}
	// A legitimate allowlisted FQDN with a trailing dot must NOT be rejected
	// (the trailing dot is normalized before the allowlist match).
	if err := validateExternalVideoURL("https://x.cn-beijing.volces.com./a.mp4"); err != nil {
		t.Fatalf("allowlisted host with trailing dot should pass: %v", err)
	}
	if err := validateExternalVideoURL("https://evil.example.com/a.mp4"); err == nil {
		t.Fatalf("non-allowlisted host should be rejected")
	}
}

func TestValidateExternalVideoURLMediaAllowlistOverridesLegacyVideoAllowlist(t *testing.T) {
	t.Setenv("SUB2API_MEDIA_URL_ALLOWLIST", "media.example.com")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	if err := validateExternalVideoURL("https://cdn.media.example.com/a.mp4"); err != nil {
		t.Fatalf("media allowlisted host should pass: %v", err)
	}
	if err := validateExternalVideoURL("https://ark-content.cn-beijing.volces.com/a.mp4"); err == nil {
		t.Fatalf("legacy video allowlist must not apply when media allowlist is set")
	}
}

// --- B1c: PlainAPIKey never rendered by fmt / slog --------------------------

func TestVideoProviderAccountRedactsPlainKey(t *testing.T) {
	acc := VideoProviderAccount{
		ID:               7,
		Provider:         VideoProviderSeedance,
		MaskedKey:        "sk-***tail",
		APIKeyConfigured: true,
		PlainAPIKey:      "sk-supersecret-PLAINTEXT-9999",
	}
	for _, s := range []string{
		fmt.Sprintf("%v", acc),
		fmt.Sprintf("%+v", acc),
		acc.String(),
		fmt.Sprintf("%v", &acc),
		acc.String(),
	} {
		if strings.Contains(s, "sk-supersecret-PLAINTEXT-9999") {
			t.Fatalf("fmt leaked PlainAPIKey: %q", s)
		}
		if !strings.Contains(s, "[REDACTED]") {
			t.Fatalf("expected [REDACTED] marker, got %q", s)
		}
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("acct", slog.Any("account", acc))
	logger.Info("acct_ptr", slog.Any("account", &acc))
	if strings.Contains(buf.String(), "sk-supersecret-PLAINTEXT-9999") {
		t.Fatalf("slog leaked PlainAPIKey: %q", buf.String())
	}

	// %#v (GoString) must also mask the key.
	if gs := fmt.Sprintf("%#v", acc); strings.Contains(gs, "sk-supersecret-PLAINTEXT-9999") {
		t.Fatalf("%%#v leaked PlainAPIKey: %q", gs)
	}
	// JSON marshaling must not serialize the plaintext key (json:"-").
	jb, err := json.Marshal(acc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(jb), "sk-supersecret-PLAINTEXT-9999") {
		t.Fatalf("json leaked PlainAPIKey: %s", jb)
	}
}

// --- B2 + B1a/B1b + B3a end-to-end via a localhost test server (never Ark) ---

func newSmokeGatedSeedanceFixture(t *testing.T, handler http.HandlerFunc) (*seedanceVideoAdapter, *VideoProviderAccount) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/redacted-events.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	acc := &VideoProviderAccount{
		ID:               1,
		Provider:         VideoProviderSeedance,
		Enabled:          true,
		APIKeyConfigured: true,
		PlainAPIKey:      "test-key-not-real",
		BaseURL:          srv.URL,
		Metadata:         map[string]any{"single_smoke_authorized": true},
	}
	return &seedanceVideoAdapter{}, acc
}

func TestSeedanceCreateSendsDurationAndAuditsRedacted(t *testing.T) {
	var captured map[string]any
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"task-xyz","status":"queued"}`))
	})

	task := &VideoTask{
		Model:       "doubao-seedance-2-0-260128",
		Prompt:      "a cat",
		Duration:    5,
		Resolution:  "720p",
		AspectRatio: "16:9",
	}
	res, err := adapter.CreateTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if res.UpstreamTaskID != "task-xyz" {
		t.Fatalf("UpstreamTaskID = %q, want task-xyz", res.UpstreamTaskID)
	}
	// B2: duration + resolution must be on the wire.
	if captured["duration"] != float64(5) {
		t.Fatalf("payload duration = %v, want 5; full payload=%+v", captured["duration"], captured)
	}
	if captured["resolution"] != "720p" {
		t.Fatalf("payload missing resolution: %+v", captured)
	}
	// B1: the aspect field is sent as Ark's `ratio` (NOT `aspect_ratio`).
	if captured["ratio"] != "16:9" {
		t.Fatalf("payload aspect must be sent as ratio=16:9, got ratio=%v; full payload=%+v", captured["ratio"], captured)
	}
	if _, leaked := captured["aspect_ratio"]; leaked {
		t.Fatalf("request must NOT send the legacy aspect_ratio field (Ark ignores it): %+v", captured)
	}

	// B1b: redacted event log was actually written.
	logPath := os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG")
	raw, err := os.ReadFile(logPath)
	if err != nil || len(raw) == 0 {
		t.Fatalf("expected redacted event log written, err=%v len=%d", err, len(raw))
	}
}

func TestSeedanceCreateRedactsUpstreamErrorBody(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"rejected token Bearer ` + fakeSecret + `"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3}

	_, err := adapter.CreateTask(context.Background(), acc, task)
	if err == nil {
		t.Fatalf("expected upstream error")
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Fatalf("error message leaked the secret: %v", err)
	}
	if !strings.Contains(err.Error(), "已脱敏") {
		t.Fatalf("expected redaction marker in error, got %v", err)
	}

	// The redacted event log must also not contain the secret.
	raw, _ := os.ReadFile(os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG"))
	if strings.Contains(string(raw), fakeSecret) {
		t.Fatalf("redacted event log leaked the secret: %q", string(raw))
	}
}

func TestSeedancePollRejectsUnsafeResultURL(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t1","status":"succeeded","content":{"video_url":"https://169.254.169.254/meta.mp4"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

	res, err := adapter.PollTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if res.ResultURL != "" {
		t.Fatalf("unsafe result_url must not be stored, got %q", res.ResultURL)
	}
	if res.Status != VideoStatusFailed {
		t.Fatalf("unsafe result_url should fail the task, status=%q", res.Status)
	}
	if res.Payload["result_url_rejected"] != true {
		t.Fatalf("expected result_url_rejected marker, payload=%+v", res.Payload)
	}
}

func TestSeedancePollAcceptsSafeResultURL(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t1","status":"succeeded","content":{"video_url":"https://ark-content.cn-beijing.volces.com/v/ok.mp4"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

	res, err := adapter.PollTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if res.ResultURL != "https://ark-content.cn-beijing.volces.com/v/ok.mp4" {
		t.Fatalf("safe result_url should be stored, got %q", res.ResultURL)
	}
	if res.Status != VideoStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", res.Status)
	}
}

// #1: a 200-OK body carrying an error.message must also be redacted (create).
func TestSeedanceCreateRedactsBusinessErrorMessage(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":{"message":"rejected: Bearer ` + fakeSecret + `"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3}

	_, err := adapter.CreateTask(context.Background(), acc, task)
	if err == nil {
		t.Fatalf("expected business error")
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Fatalf("200-OK business error leaked the secret: %v", err)
	}
	if !strings.Contains(err.Error(), "已脱敏") {
		t.Fatalf("expected redaction marker in business error, got %v", err)
	}
}

// The result_url SSRF-validation error must itself be redacted before it lands in
// task.ErrorMessage (DB) and the poll API response — closing the redaction-contract gap
// where the rejected upstream URL was echoed un-redacted. The unsafe host is shaped like a
// Volcengine AKLT access key so the "host is not allowed: <host>" echo carries a
// credential-shaped token that the key-aware redactor must strip.
func TestSeedancePollRedactsRejectedResultURLError(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t1","status":"succeeded","content":{"video_url":"https://akltsecret1234567890abcdef.evil.example.com/v.mp4"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

	res, err := adapter.PollTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if res.Status != VideoStatusFailed {
		t.Fatalf("an unsafe result_url must fail the task, got %s", res.Status)
	}
	if res.ResultURL != "" {
		t.Fatalf("an unsafe result_url must not be stored, got %q", res.ResultURL)
	}
	if strings.Contains(res.ErrorMessage, "akltsecret1234567890abcdef") {
		t.Fatalf("rejected result_url error leaked the credential-shaped host token: %q", res.ErrorMessage)
	}
	if !strings.Contains(res.ErrorMessage, "已脱敏") {
		t.Fatalf("expected the rejected-url error to be redacted, got %q", res.ErrorMessage)
	}
}

// #1: a 200-OK body carrying an error.message must also be redacted (poll).
func TestSeedancePollRedactsBusinessErrorMessage(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t1","status":"failed","error":{"message":"bad key ` + fakeSecret + `"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

	res, err := adapter.PollTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if strings.Contains(res.ErrorMessage, fakeSecret) {
		t.Fatalf("poll business error leaked the secret: %q", res.ErrorMessage)
	}
	if res.Status != VideoStatusFailed {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}

// #5: poll non-2xx error body must be redacted (mirror of the create test).
func TestSeedancePollRedactsUpstreamErrorBody(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"forbidden Bearer ` + fakeSecret + `"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

	_, err := adapter.PollTask(context.Background(), acc, task)
	if err == nil {
		t.Fatalf("expected upstream error")
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Fatalf("poll error body leaked the secret: %v", err)
	}
	raw, _ := os.ReadFile(os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG"))
	if strings.Contains(string(raw), fakeSecret) {
		t.Fatalf("redacted event log leaked the secret: %q", string(raw))
	}
}

// #6: an unsafe reference_image_url must be rejected BEFORE any upstream call.
func TestSeedanceCreateRejectsUnsafeReferenceURL(t *testing.T) {
	called := false
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","status":"queued"}`))
	})
	task := &VideoTask{
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "x",
		Duration:          3,
		ReferenceImageURL: "http://169.254.169.254/payload.png",
	}

	_, err := adapter.CreateTask(context.Background(), acc, task)
	if err == nil {
		t.Fatalf("expected reference_image_url to be rejected")
	}
	if !strings.Contains(err.Error(), "reference_image_url") {
		t.Fatalf("expected reference_image_url validation error, got %v", err)
	}
	if called {
		t.Fatalf("upstream must NOT be called when reference_image_url is unsafe")
	}
}
