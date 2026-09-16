package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testCodexTelemetryProfile() codexTelemetryProfile {
	return codexTelemetryProfile{
		client: codexTelemetryClient{
			localID: 7, accessToken: "test-token", accountID: "acct-test",
			userAgent:  "codex-tui/0.153.4 (Mac OS 15.5.0; arm64) xterm-256color (codex-tui; 0.153.4)",
			originator: "codex_cli_rs", version: "0.153.4",
		},
		sessionID: "session-1", threadID: "thread-1", turnID: "turn-1", rootTurnID: "turn-1",
		model: "gpt-6-astra", effort: "high", serviceTier: "default", started: time.Now().Add(-time.Second),
		firstThread: true, dynamicTool: true, command: true, fileChange: true,
	}
}

func isolatedCodexTelemetryManager(t *testing.T) *codexTelemetryManager {
	t.Helper()
	m := newCodexTelemetryManager()
	m.once.Do(func() {})
	prev := codexTelemetryGlobal
	codexTelemetryGlobal = m
	t.Cleanup(func() { codexTelemetryGlobal = prev })
	return m
}

func testCodexTelemetryAccount() *Account {
	return &Account{
		ID:       7,
		Name:     "codex-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-token",
			"chatgpt_account_id": "acct-test",
		},
		Extra: map[string]any{
			codexTelemetryEnabledExtraKey: true,
		},
	}
}

func testCodexTelemetryService() *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{CodexTelemetryEnabled: true}},
	}
}

func testCodexTelemetryHeaders() http.Header {
	headers := make(http.Header)
	headers.Set("user-agent", "codex-tui/0.153.4 (Mac OS 15.5.0; arm64) xterm-256color (codex-tui; 0.153.4)")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("version", "0.153.4")
	headers.Set("session_id", "upstream-session")
	return headers
}

func testCodexTelemetryGin(path string, headers http.Header) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if headers != nil {
		req.Header = headers.Clone()
	}
	c.Request = req
	return c
}

func TestCodexTelemetryEventContract(t *testing.T) {
	profile := testCodexTelemetryProfile()
	terminal := []byte(`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":12,"output_tokens":8,"total_tokens":20}}}`)
	events := append(codexInitializationEvents(profile), codexTerminalEvents(profile, codexTelemetryTerminal{status: "completed", body: terminal})...)
	counts := make(map[string]int)
	var mainTurn, accepted map[string]any
	for _, event := range events {
		counts[event.EventType]++
		if event.EventType == "codex_turn_steer_event" {
			t.Fatal("Responses telemetry must not infer native turn/steer RPCs")
		}
		if event.EventType == "codex_turn_event" && event.EventParams["thread_source"] == "user" {
			mainTurn = event.EventParams
		}
		if event.EventType == "codex_turn_event" && event.EventParams["thread_source"] == "thread_title" {
			if event.EventParams["root_turn_id"] != event.EventParams["turn_id"] || event.EventParams["total_tool_call_count"] != 0 {
				t.Fatalf("title turn params = %#v", event.EventParams)
			}
		}
		if event.EventType == "codex_thread_initialized" && event.EventParams["thread_source"] == "guardian_review" {
			if event.EventParams["parent_thread_id"] != profile.threadID || event.EventParams["subagent_source"] != "guardian" || event.EventParams["ephemeral"] != false {
				t.Fatalf("guardian thread params = %#v", event.EventParams)
			}
		}
		if event.EventType == "codex_accepted_line_fingerprints" {
			accepted = event.EventParams
		}
	}
	if counts["codex_thread_initialized"] != 3 || counts["codex_turn_event"] != 2 || counts["codex_hook_run"] != 4 {
		t.Fatalf("event counts = %#v", counts)
	}
	if counts["codex_dynamic_tool_call_event"] != 1 || counts["codex_command_execution_event"] != 1 || counts["codex_file_change_event"] != 1 || counts["codex_accepted_line_fingerprints"] != 1 {
		t.Fatalf("random event counts = %#v", counts)
	}
	if mainTurn["initialization_mode"] != "new" || mainTurn["steer_count"] != 0 || mainTurn["total_tokens"] != int64(20) {
		t.Fatalf("main turn params = %#v", mainTurn)
	}
	if accepted["repo_hash"] != nil || len(accepted["line_fingerprints"].([]any)) != 0 {
		t.Fatalf("accepted fingerprint params = %#v", accepted)
	}
}

func TestCodexTelemetryDoesNotInferResume(t *testing.T) {
	profile := testCodexTelemetryProfile()
	body := []byte(`{"model":"gpt-6-astra","previous_response_id":"resp_old","input":[{"role":"assistant"},{"type":"function_call_output"}]}`)
	profile = buildCodexTelemetryProfile(profile.client, codexTelemetryRequest{body: body, sessionID: "session-1", headers: http.Header{}})
	params := codexMainTurnEvent(profile, codexTelemetryTerminal{status: "completed"}).EventParams
	if params["initialization_mode"] != "new" || params["steer_count"] != 0 {
		t.Fatalf("history changed turn classification: %#v", params)
	}
}

func testCodexMetricPoint(descriptor codexMetricDescriptor, value float64) *codexMetricPoint {
	return newCodexMetricPoint(descriptor, codexMetricAttributeMap(testCodexTelemetryProfile(), descriptor), value)
}

func TestCodexTelemetryMetricsContract(t *testing.T) {
	if len(codexMetricDescriptors) != 66 {
		t.Fatalf("metric descriptor count = %d, want 66", len(codexMetricDescriptors))
	}
	names := make(map[string]bool, len(codexMetricDescriptors))
	points := make([]*codexMetricPoint, 0, len(codexMetricDescriptors))
	for _, descriptor := range codexMetricDescriptors {
		if names[descriptor.name] {
			t.Fatalf("duplicate metric %q", descriptor.name)
		}
		names[descriptor.name] = true
		points = append(points, testCodexMetricPoint(descriptor, 1))
	}
	for _, name := range []string{"codex.hooks.run", "codex.hooks.run.duration_ms", "codex.external_agent_config.detect", "codex.rollout.size_bytes"} {
		if !names[name] {
			t.Fatalf("missing dynamic metric %q", name)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(buildCodexMetricsPayload(testCodexTelemetryProfile(), time.Now(), points), &payload); err != nil {
		t.Fatalf("decode OTLP payload: %v", err)
	}
	resourceMetrics := payload["resourceMetrics"].([]any)
	scopeMetrics := resourceMetrics[0].(map[string]any)["scopeMetrics"].([]any)
	metrics := scopeMetrics[0].(map[string]any)["metrics"].([]any)
	if len(metrics) != 66 {
		t.Fatalf("OTLP metric count = %d, want 66", len(metrics))
	}
	for _, raw := range metrics {
		metric := raw.(map[string]any)
		for _, kind := range []string{"sum", "histogram"} {
			if aggregation, ok := metric[kind].(map[string]any); ok && aggregation["aggregationTemporality"] != float64(1) {
				t.Fatalf("metric %q is not delta temporality", metric["name"])
			}
		}
		histogram, ok := metric["histogram"].(map[string]any)
		if !ok {
			continue
		}
		dataPoint := histogram["dataPoints"].([]any)[0].(map[string]any)
		buckets := dataPoint["bucketCounts"].([]any)
		var bucketTotal float64
		for _, bucket := range buckets {
			bucketTotal += bucket.(float64)
		}
		if bucketTotal != dataPoint["count"].(float64) {
			t.Fatalf("metric %q bucket total %v != count %v", metric["name"], bucketTotal, dataPoint["count"])
		}
		if len(dataPoint["explicitBounds"].([]any)) != len(buckets)-1 {
			t.Fatalf("metric %q bounds/buckets mismatch", metric["name"])
		}
	}
}

func TestCodexTelemetryMetricAggregationKeepsObservations(t *testing.T) {
	m := newCodexTelemetryManager()
	m.once.Do(func() {})
	base := testCodexTelemetryProfile()
	accountID := base.client.localID

	profileA := base
	profileA.model = "gpt-6-astra"
	profileA.started = time.Now().Add(-200 * time.Millisecond)
	profileA.dynamicTool, profileA.command, profileA.fileChange = false, false, false
	m.recordTurnMetrics(profileA, codexTelemetryTerminal{status: "completed"})

	profileB := base
	profileB.model = "gpt-5.6-sol"
	profileB.turnID = "turn-2"
	profileB.started = time.Now().Add(-4 * time.Second)
	profileB.dynamicTool, profileB.command, profileB.fileChange = false, false, false
	m.recordTurnMetrics(profileB, codexTelemetryTerminal{status: "completed"})

	state := m.metrics[accountID]
	if state == nil {
		t.Fatal("metric state missing")
	}
	byModel := make(map[string]*codexMetricPoint)
	for _, point := range state.points {
		if point.descriptor.name != "codex.turn.e2e_duration_ms" {
			continue
		}
		if point.count != 1 {
			t.Fatalf("per-model point count = %d, want 1", point.count)
		}
		byModel[point.attributes["model"]] = point
	}
	fast, slow := byModel["gpt-6-astra"], byModel["gpt-5.6-sol"]
	if fast == nil || slow == nil {
		t.Fatalf("per-model attribute points missing: %#v", byModel)
	}
	if slow.sum <= fast.sum {
		t.Fatalf("durations collapsed across profiles: fast=%v slow=%v", fast.sum, slow.sum)
	}

	m.recordTurnMetrics(profileA, codexTelemetryTerminal{status: "completed"})
	if fast.count != 2 {
		t.Fatalf("same-profile observations must accumulate, count = %d", fast.count)
	}
	if len(byModel) != 2 {
		t.Fatalf("aggregation created extra points: %d", len(byModel))
	}
}

func TestCodexTelemetryHookHistogramCountsObservations(t *testing.T) {
	m := newCodexTelemetryManager()
	m.once.Do(func() {})
	profile := testCodexTelemetryProfile()
	m.recordTurnMetrics(profile, codexTelemetryTerminal{status: "completed"})
	state := m.metrics[profile.client.localID]
	if state == nil {
		t.Fatal("metric state missing")
	}
	for _, point := range state.points {
		if point.descriptor.name != "codex.hooks.run.duration_ms" {
			continue
		}
		if point.count != 4 {
			t.Fatalf("hook duration histogram count = %d, want 4", point.count)
		}
		var bucketTotal uint64
		for _, bucket := range point.bucketCounts {
			bucketTotal += bucket
		}
		if bucketTotal != point.count {
			t.Fatalf("hook bucket total = %d, want %d", bucketTotal, point.count)
		}
		return
	}
	t.Fatal("hook duration histogram missing")
}

func TestCodexTelemetryRedirectRejected(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	profile := testCodexTelemetryProfile()
	err := sendCodexTelemetryJob(codexTelemetryJob{client: profile.client, url: redirector.URL, body: []byte(`{}`)})
	if err == nil {
		t.Fatal("telemetry client must not follow redirects")
	}
	if targetHits != 0 {
		t.Fatalf("redirect target received credentials %d times", targetHits)
	}
}

func TestCodexTelemetryAcceptedLinesIsolatedRequest(t *testing.T) {
	m := newCodexTelemetryManager()
	m.once.Do(func() {})
	profile := testCodexTelemetryProfile()
	events := codexTerminalEvents(profile, codexTelemetryTerminal{status: "completed"})
	m.enqueueAnalytics(events)

	var batches [][]map[string]any
	for len(m.queue) > 0 {
		job := <-m.queue
		var payload struct {
			Events []map[string]any `json:"events"`
		}
		if err := json.Unmarshal(job.body, &payload); err != nil {
			t.Fatalf("decode analytics batch: %v", err)
		}
		batches = append(batches, payload.Events)
	}
	if len(batches) < 2 {
		t.Fatalf("analytics batch count = %d, want accepted-lines split out", len(batches))
	}
	isolatedAccepted := 0
	for _, batch := range batches {
		hasAccepted := false
		for _, event := range batch {
			if event["event_type"] == "codex_accepted_line_fingerprints" {
				hasAccepted = true
			}
		}
		if !hasAccepted {
			continue
		}
		if len(batch) != 1 {
			t.Fatalf("accepted-line-fingerprints must be isolated, batch = %#v", batch)
		}
		isolatedAccepted++
	}
	if isolatedAccepted != 1 {
		t.Fatalf("isolated accepted-line-fingerprints batches = %d, want 1", isolatedAccepted)
	}
}

func TestCodexTelemetryStatsigGuard(t *testing.T) {
	for _, name := range []string{"codex.tool.call", "codex.tool.call.duration_ms", "codex.turn.token_usage", "codex.api_request"} {
		if codexTelemetryStatsigAllowed(name) {
			t.Fatalf("metric %q must be blocked from Statsig", name)
		}
	}
	if !codexTelemetryStatsigAllowed("codex.turn.e2e_duration_ms") {
		t.Fatal("normal metric must stay allowed")
	}
}

func TestCodexDesktopMetricIdentity(t *testing.T) {
	profile := testCodexTelemetryProfile()
	profile.client.userAgent = "Codex Desktop/0.153.4 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.903.61454)"
	profile.client.originator = "Codex Desktop"
	if got := codexMetricAttributeValue(profile, "codex.turn.e2e_duration_ms", "originator"); got != "Codex_Desktop" {
		t.Fatalf("metric originator = %q", got)
	}
	if got := codexMetricAttributeValue(profile, "codex.turn.e2e_duration_ms", "service_name"); got != "codex_desktop" {
		t.Fatalf("metric service name = %q", got)
	}
	if got := codexMetricAttributeValue(profile, "codex.process.start", "originator"); got != "codex-app-server" {
		t.Fatalf("process originator = %q", got)
	}
}

func TestCodexTelemetryTransportHeaders(t *testing.T) {
	requests := make(chan http.Header, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		requests <- request.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	profile := testCodexTelemetryProfile()
	profile.client.statsigAPIKey = "test-statsig-key"
	jobs := []codexTelemetryJob{
		{client: profile.client, url: server.URL, body: []byte(`{}`)},
		{client: profile.client, url: server.URL, body: []byte(`{}`), metrics: true},
	}
	for _, job := range jobs {
		if err := sendCodexTelemetryJob(job); err != nil {
			t.Fatalf("send telemetry: %v", err)
		}
	}
	analytics, metrics := <-requests, <-requests
	if analytics.Get("Authorization") != "Bearer test-token" || analytics.Get("Chatgpt-Account-Id") != "acct-test" || analytics.Get("Originator") != "codex_cli_rs" || analytics.Get("User-Agent") != profile.client.userAgent {
		t.Fatalf("analytics headers = %#v", analytics)
	}
	if metrics.Get("Authorization") != "" || metrics.Get("statsig-api-key") != "test-statsig-key" || metrics.Get("User-Agent") != "OTel-OTLP-Exporter-Rust/0.31.0" {
		t.Fatalf("metrics headers = %#v", metrics)
	}
}

func TestCodexTelemetryStartupMetricPartition(t *testing.T) {
	catalog := make(map[string]bool, len(codexMetricDescriptors))
	startup := 0
	for _, descriptor := range codexMetricDescriptors {
		catalog[descriptor.name] = true
		if codexStartupMetric(descriptor.name) {
			startup++
		}
	}
	for name := range codexDynamicMetricNames {
		if !catalog[name] {
			t.Fatalf("dynamic metric %q missing from catalog", name)
		}
	}
	if startup != len(codexMetricDescriptors)-len(codexDynamicMetricNames) {
		t.Fatalf("startup=%d catalog=%d dynamic=%d", startup, len(codexMetricDescriptors), len(codexDynamicMetricNames))
	}
	if startup != 62 {
		t.Fatalf("startup metric count = %d, want 62", startup)
	}
}

func TestCodexTelemetryParsesWebsocketSSE(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	attempt := &codexTelemetryAttempt{profile: testCodexTelemetryProfile()}
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ws\"}}\n\n"
	body := &codexTelemetryBody{ReadCloser: io.NopCloser(strings.NewReader(stream)), attempt: attempt}
	buf := make([]byte, 128)
	for {
		if _, err := body.Read(buf); err != nil {
			break
		}
	}
	if attempt.firstEvent.IsZero() {
		t.Fatal("WebSocket-shaped SSE stream did not record first event")
	}
	if attempt.firstToken.IsZero() {
		t.Fatal("WebSocket-shaped SSE stream did not record first token")
	}
}

func TestCodexTelemetryAccountAndProcessDefaultOff(t *testing.T) {
	account := testCodexTelemetryAccount()
	account.Extra = nil
	require.False(t, account.IsCodexTelemetryEnabled())

	s := &OpenAIGatewayService{cfg: &config.Config{}}
	headers := testCodexTelemetryHeaders()
	c := testCodexTelemetryGin("/v1/responses", headers)
	require.Nil(t, s.beginCodexTelemetry(c, testCodexTelemetryAccount(), []byte(`{"model":"gpt-6-astra"}`), headers))
}

func TestCodexTelemetryEligibilityMatrix(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	body := []byte(`{"model":"gpt-6-astra","input":[{"type":"message","role":"user","content":"hi"}]}`)
	headers := testCodexTelemetryHeaders()
	s := testCodexTelemetryService()
	account := testCodexTelemetryAccount()

	t.Run("eligible oauth", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.NotNil(t, s.beginCodexTelemetry(c, account, body, headers))
	})
	t.Run("kill switch", func(t *testing.T) {
		disabled := testCodexTelemetryService()
		disabled.cfg.Gateway.CodexTelemetryEnabled = false
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, disabled.beginCodexTelemetry(c, account, body, headers))
	})
	t.Run("extra off", func(t *testing.T) {
		off := testCodexTelemetryAccount()
		off.Extra = map[string]any{}
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, s.beginCodexTelemetry(c, off, body, headers))
	})
	t.Run("setup token", func(t *testing.T) {
		setupToken := testCodexTelemetryAccount()
		setupToken.Type = AccountTypeSetupToken
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, s.beginCodexTelemetry(c, setupToken, body, headers))
	})
	t.Run("api key", func(t *testing.T) {
		apiKey := testCodexTelemetryAccount()
		apiKey.Type = AccountTypeAPIKey
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, s.beginCodexTelemetry(c, apiKey, body, headers))
	})
	t.Run("agent identity", func(t *testing.T) {
		agent := testCodexTelemetryAccount()
		agent.Credentials[openAIAuthModeCredentialKey] = OpenAIAuthModeAgentIdentity
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, s.beginCodexTelemetry(c, agent, body, headers))
	})
	t.Run("compact path", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/responses/compact", headers)
		require.Nil(t, s.beginCodexTelemetry(c, account, body, headers))
	})
	t.Run("count tokens", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/messages/count_tokens", headers)
		require.Nil(t, s.beginCodexTelemetry(c, account, body, headers))
	})
	t.Run("embeddings", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/embeddings", headers)
		require.Nil(t, s.beginCodexTelemetry(c, account, body, headers))
	})
	t.Run("images", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/images/generations", headers)
		require.Nil(t, s.beginCodexTelemetry(c, account, body, headers))
	})
	t.Run("image generation intent", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/responses", headers)
		imageBody := []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation"}]}`)
		require.Nil(t, s.beginCodexTelemetry(c, account, imageBody, headers))
	})
	t.Run("compaction metadata", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/responses", headers)
		compactBody := []byte(`{"model":"gpt-6-astra","client_metadata":{"x-codex-turn-metadata":"{\"compaction\":true}"}}`)
		require.Nil(t, s.beginCodexTelemetry(c, account, compactBody, headers))
	})
	t.Run("grok", func(t *testing.T) {
		grok := testCodexTelemetryAccount()
		grok.Platform = PlatformGrok
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, s.beginCodexTelemetry(c, grok, body, headers))
	})
	t.Run("missing token", func(t *testing.T) {
		missing := testCodexTelemetryAccount()
		missing.Credentials = map[string]any{"chatgpt_account_id": "acct-test"}
		c := testCodexTelemetryGin("/v1/responses", headers)
		require.Nil(t, s.beginCodexTelemetry(c, missing, body, headers))
	})
}

func TestCodexTelemetryShadowFollowsParentExtra(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	parent := testCodexTelemetryAccount()
	parent.ID = 11
	parentID := int64(11)
	shadow := &Account{
		ID:              12,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		Credentials:     map[string]any{},
		Extra:           map[string]any{},
	}
	headers := testCodexTelemetryHeaders()
	c := testCodexTelemetryGin("/v1/responses", headers)
	c.Set(codexAccountIdentitySourceContextKey, parent)
	s := testCodexTelemetryService()
	require.NotNil(t, s.beginCodexTelemetry(c, shadow, []byte(`{"model":"gpt-6-astra"}`), headers))
}

func TestCodexTelemetryFingerprintIDsPreferred(t *testing.T) {
	client := testCodexTelemetryProfile().client
	ids := &codexFingerprintIDs{sessionID: "fp-session", threadID: "fp-thread", turnID: "fp-turn"}
	headers := testCodexTelemetryHeaders()
	headers.Set(openCodeSessionAffinityHeader, "tenant-secret-42")
	profile := buildCodexTelemetryProfile(client, codexTelemetryRequest{
		body:      []byte(`{"model":"gpt-6-astra","client_metadata":{"session_id":"body-session"}}`),
		headers:   headers,
		sessionID: "header-session",
		ids:       ids,
	})
	require.Equal(t, "fp-session", profile.sessionID)
	require.Equal(t, "fp-thread", profile.threadID)
	require.Equal(t, "fp-turn", profile.turnID)
}

func TestCodexTelemetryProfileIgnoresLocalAffinityKey(t *testing.T) {
	profile := testCodexTelemetryProfile()
	headers := http.Header{}
	headers.Set(openCodeSessionAffinityHeader, "tenant-secret-42")
	profile = buildCodexTelemetryProfile(profile.client, codexTelemetryRequest{
		body:      []byte(`{"model":"gpt-6-astra"}`),
		sessionID: "upstream-session",
		headers:   headers,
	})
	if profile.sessionID != "upstream-session" || profile.threadID != "upstream-session" {
		t.Fatalf("session identity = %q/%q, must not derive from local affinity key", profile.sessionID, profile.threadID)
	}
}

func TestCodexTelemetryCLIVersusDesktopIdentity(t *testing.T) {
	cli := testCodexTelemetryProfile()
	require.Equal(t, "codex-tui", codexClientName(cli))
	require.Equal(t, "cli", codexMetricSessionSource(cli))

	desktop := testCodexTelemetryProfile()
	desktop.client.userAgent = "Codex Desktop/0.153.4 (Mac OS 15.5.0; arm64) unknown (Codex Desktop; 26.903.61454)"
	desktop.client.originator = "Codex Desktop"
	desktop.client.version = "26.903.61454"
	require.Equal(t, "Codex Desktop", codexClientName(desktop))
	require.Equal(t, "vscode", codexMetricSessionSource(desktop))
	require.Equal(t, "stdio", codexAppServerClient(desktop)["rpc_transport"])
}

func TestCodexTelemetryObserveResultDoesNotAffectCaller(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	attempt := &codexTelemetryAttempt{profile: testCodexTelemetryProfile()}
	upstreamErr := io.ErrUnexpectedEOF
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload"))}
	attempt.observeResult(resp, upstreamErr)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "payload", string(body))

	success := &codexTelemetryAttempt{profile: testCodexTelemetryProfile()}
	stream := httptest.NewRecorder()
	okResp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n"))}
	success.observeResult(okResp, nil)
	require.Equal(t, http.StatusOK, okResp.StatusCode)
	copied, err := io.ReadAll(okResp.Body)
	require.NoError(t, err)
	require.Contains(t, string(copied), "response.completed")
	_ = stream
}

func TestCodexTelemetryBodyCloseRacesRead(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	pr, pw := io.Pipe()
	attempt := &codexTelemetryAttempt{profile: testCodexTelemetryProfile()}
	body := &codexTelemetryBody{ReadCloser: pr, attempt: attempt}
	go func() {
		for i := 0; i < 50; i++ {
			_, _ = pw.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n"))
		}
		_ = pw.Close()
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		for {
			if _, err := body.Read(buf); err != nil {
				return
			}
		}
	}()
	time.Sleep(time.Millisecond)
	_ = body.Close()
	<-done
}

func TestCodexTelemetryRecordTurnWithoutStateDoesNotPanic(t *testing.T) {
	m := newCodexTelemetryManager()
	m.once.Do(func() {})
	profile := testCodexTelemetryProfile()
	profile.started = time.Now().Add(-2 * codexTelemetryStateTTL)
	m.recordTurnMetrics(profile, codexTelemetryTerminal{status: "completed"})
	if len(m.queue) != 1 {
		t.Fatalf("startup batch count = %d, want 1", len(m.queue))
	}
	state := m.metrics[profile.client.localID]
	if state == nil || time.Since(state.lastSeen) > time.Minute {
		t.Fatalf("state after long turn = %#v", state)
	}
	m.flushMetrics(time.Now())
	if m.metrics[profile.client.localID] == nil {
		t.Fatal("state evicted right after a long turn")
	}
	m.recordTurnMetrics(profile, codexTelemetryTerminal{status: "completed"})
	if len(m.queue) != 2 {
		t.Fatalf("queue after second turn = %d, want 2", len(m.queue))
	}
}

// drainCodexTelemetryEvents 排空隔离队列并按 event_type 计数。
// 隔离 manager 没有 worker 消费 queue，所以入队的批次留在 channel 里可同步读。
func drainCodexTelemetryEvents(t *testing.T, m *codexTelemetryManager) ([]map[string]any, map[string]int) {
	t.Helper()
	events := make([]map[string]any, 0, 16)
	counts := make(map[string]int)
	for len(m.queue) > 0 {
		job := <-m.queue
		if job.metrics {
			continue
		}
		var payload struct {
			Events []map[string]any `json:"events"`
		}
		require.NoError(t, json.Unmarshal(job.body, &payload))
		for _, event := range payload.Events {
			events = append(events, event)
			eventType, _ := event["event_type"].(string)
			counts[eventType]++
		}
	}
	return events, counts
}

// mainCodexTurnEvents 只保留主回合事件，排除 thread_title 那条模拟的标题回合。
func mainCodexTurnEvents(events []map[string]any) []map[string]any {
	main := make([]map[string]any, 0, 2)
	for _, event := range events {
		if event["event_type"] != "codex_turn_event" {
			continue
		}
		params, _ := event["event_params"].(map[string]any)
		if params == nil || params["thread_source"] != "user" {
			continue
		}
		main = append(main, params)
	}
	return main
}

// fixCodexTelemetryDice 固定 dynamicTool/command/fileChange 的掷骰子结果，
// 让终止批次的事件条数可精确断言。
func fixCodexTelemetryDice(t *testing.T) {
	t.Helper()
	prev := codexTelemetryRandIntN
	codexTelemetryRandIntN = func(int) int { return 0 }
	t.Cleanup(func() { codexTelemetryRandIntN = prev })
}

func TestCodexTelemetryTurnMemoizedAcrossAttempts(t *testing.T) {
	m := isolatedCodexTelemetryManager(t)
	s := testCodexTelemetryService()
	account := testCodexTelemetryAccount()
	headers := testCodexTelemetryHeaders()
	body := []byte(`{"model":"gpt-6-astra"}`)
	c := testCodexTelemetryGin("/v1/responses", headers)

	first := s.beginCodexTelemetry(c, account, body, headers)
	require.NotNil(t, first)
	queuedAfterFirst := len(m.queue)

	second := s.beginCodexTelemetry(c, account, body, headers)
	require.Same(t, first, second, "网关内部重试必须复用同一个回合")
	require.Equal(t, queuedAfterFirst, len(m.queue), "复用回合不得重发初始化批次")
}

func TestCodexTelemetryTurnRestartsOnAccountSwitch(t *testing.T) {
	m := isolatedCodexTelemetryManager(t)
	s := testCodexTelemetryService()
	headers := testCodexTelemetryHeaders()
	body := []byte(`{"model":"gpt-6-astra"}`)
	c := testCodexTelemetryGin("/v1/responses", headers)

	first := s.beginCodexTelemetry(c, testCodexTelemetryAccount(), body, headers)
	require.NotNil(t, first)
	queuedAfterFirst := len(m.queue)

	// handler 层 failover 换号：同一个 gin.Context、不同账号。事件要用各自账号的
	// Bearer 发出，所以必须是两个独立回合，不能复用 A 号的 attempt。
	next := testCodexTelemetryAccount()
	next.ID = 99
	second := s.beginCodexTelemetry(c, next, body, headers)
	require.NotNil(t, second)
	require.NotSame(t, first, second)
	require.Greater(t, len(m.queue), queuedAfterFirst)
}

func TestCodexTelemetryFailedAttemptsEmitSingleTurnEvent(t *testing.T) {
	m := isolatedCodexTelemetryManager(t)
	fixCodexTelemetryDice(t)
	s := testCodexTelemetryService()
	account := testCodexTelemetryAccount()
	headers := testCodexTelemetryHeaders()
	body := []byte(`{"model":"gpt-6-astra"}`)
	c := testCodexTelemetryGin("/v1/responses", headers)

	// 三次上游 attempt 全失败（transport error、429、500），模拟网关内部重试循环。
	attempt := s.beginCodexTelemetry(c, account, body, headers)
	require.NotNil(t, attempt)
	attempt.observeResult(nil, io.ErrUnexpectedEOF)
	require.Same(t, attempt, s.beginCodexTelemetry(c, account, body, headers))
	attempt.observeResult(&http.Response{StatusCode: http.StatusTooManyRequests}, nil)
	require.Same(t, attempt, s.beginCodexTelemetry(c, account, body, headers))
	attempt.observeResult(&http.Response{StatusCode: http.StatusInternalServerError}, nil)

	// 初始化批次里的 thread_title 模拟回合是正常的，这里只看主回合。
	beforeFinish, counts := drainCodexTelemetryEvents(t, m)
	require.Empty(t, mainCodexTurnEvents(beforeFinish), "失败的 attempt 不得各自发终止事件")
	require.Zero(t, counts["codex_hook_run"])

	finishCodexTelemetryTurn(c)
	events, counts := drainCodexTelemetryEvents(t, m)
	main := mainCodexTurnEvents(events)
	require.Len(t, main, 1, "一个用户回合只能发一条主 turn 事件")
	require.Equal(t, "failed", main[0]["status"])
	require.Equal(t, 4, counts["codex_hook_run"], "hook 事件同样只发一轮")
}

func TestCodexTelemetryCompletedStreamIgnoresDeferredFinish(t *testing.T) {
	m := isolatedCodexTelemetryManager(t)
	fixCodexTelemetryDice(t)
	s := testCodexTelemetryService()
	account := testCodexTelemetryAccount()
	headers := testCodexTelemetryHeaders()
	body := []byte(`{"model":"gpt-6-astra"}`)
	c := testCodexTelemetryGin("/v1/responses", headers)

	attempt := s.beginCodexTelemetry(c, account, body, headers)
	require.NotNil(t, attempt)
	sse := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":7,\"total_tokens\":18}}}\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(sse))}
	attempt.observeResult(resp, nil)
	_, err := io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)

	// 收尾 defer 绝不能覆盖流已经发出的真实终止状态。
	finishCodexTelemetryTurn(c)
	events, _ := drainCodexTelemetryEvents(t, m)
	main := mainCodexTurnEvents(events)
	require.Len(t, main, 1)
	require.Equal(t, "completed", main[0]["status"])
	require.EqualValues(t, 18, main[0]["total_tokens"])
}

func TestCodexTelemetryFinishClearsTurnForNextBridgeTurn(t *testing.T) {
	m := isolatedCodexTelemetryManager(t)
	fixCodexTelemetryDice(t)
	s := testCodexTelemetryService()
	account := testCodexTelemetryAccount()
	headers := testCodexTelemetryHeaders()
	body := []byte(`{"model":"gpt-6-astra"}`)
	c := testCodexTelemetryGin("/v1/responses", headers)

	// WS HTTP bridge 的第 N 个回合，以及 handler 的同账号重试，都会在同一个
	// gin.Context 上重新开始：收尾必须清掉 memo，否则第二次拿到已 done 的
	// attempt，真正成功的回合会被永久记成失败。
	first := s.beginCodexTelemetry(c, account, body, headers)
	require.NotNil(t, first)
	first.observeResult(nil, io.ErrUnexpectedEOF)
	finishCodexTelemetryTurn(c)

	second := s.beginCodexTelemetry(c, account, body, headers)
	require.NotNil(t, second)
	require.NotSame(t, first, second)
	finishCodexTelemetryTurn(c)

	events, _ := drainCodexTelemetryEvents(t, m)
	require.Len(t, mainCodexTurnEvents(events), 2)
}

func TestCodexTelemetryIneligibleDecisionIsMemoized(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	s := testCodexTelemetryService()
	account := testCodexTelemetryAccount()
	headers := testCodexTelemetryHeaders()
	body := []byte(`{"model":"gpt-6-astra"}`)
	c := testCodexTelemetryGin("/v1/responses/compact", headers)

	require.Nil(t, s.beginCodexTelemetry(c, account, body, headers))
	turn, ok := stagedCodexTelemetryTurn(c, account)
	require.True(t, ok, "「不发遥测」的结论也要缓存，避免每次重试重跑 body 扫描")
	require.Nil(t, turn.attempt)
	require.Nil(t, s.beginCodexTelemetry(c, account, body, headers))
}

func TestCodexTelemetryEligiblePredicateMatchesBeginGuards(t *testing.T) {
	isolatedCodexTelemetryManager(t)
	body := []byte(`{"model":"gpt-6-astra","input":[{"type":"message","role":"user","content":"hi"}]}`)
	headers := testCodexTelemetryHeaders()

	agentIdentity := testCodexTelemetryAccount()
	agentIdentity.Credentials[openAIAuthModeCredentialKey] = OpenAIAuthModeAgentIdentity
	extraOff := testCodexTelemetryAccount()
	extraOff.Extra = map[string]any{}
	apiKey := testCodexTelemetryAccount()
	apiKey.Type = AccountTypeAPIKey
	setupToken := testCodexTelemetryAccount()
	setupToken.Type = AccountTypeSetupToken
	grok := testCodexTelemetryAccount()
	grok.Platform = PlatformGrok
	killSwitch := testCodexTelemetryService()
	killSwitch.cfg.Gateway.CodexTelemetryEnabled = false

	cases := []struct {
		name    string
		service *OpenAIGatewayService
		account *Account
	}{
		{"kill switch", killSwitch, testCodexTelemetryAccount()},
		{"extra off", testCodexTelemetryService(), extraOff},
		{"api key", testCodexTelemetryService(), apiKey},
		{"setup token", testCodexTelemetryService(), setupToken},
		{"agent identity", testCodexTelemetryService(), agentIdentity},
		{"grok", testCodexTelemetryService(), grok},
	}
	// 谓词必须是 begin 的 body 无关守卫的超集：一旦退化成真子集，
	// WS 路径靠它跳过序列化就会连带丢掉真实遥测。
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testCodexTelemetryGin("/v1/responses", headers)
			require.False(t, tc.service.codexTelemetryEligible(c, tc.account))
			require.Nil(t, tc.service.beginCodexTelemetry(c, tc.account, body, headers))
		})
	}

	t.Run("eligible oauth", func(t *testing.T) {
		c := testCodexTelemetryGin("/v1/responses", headers)
		s := testCodexTelemetryService()
		require.True(t, s.codexTelemetryEligible(c, testCodexTelemetryAccount()))
	})
}
