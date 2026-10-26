package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestOpenAICompactionTargetResolvesMappedPassthroughAndKindsOnce(t *testing.T) {
	svc := &OpenAIGatewayService{}
	mapped := &Account{
		ID: 201, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":               "sk-mapped",
			"api_protocol":          APIProtocolAdaptive,
			"api_base_urls":         map[string]any{APIProtocolAnthropic: "https://anthropic.example.test"},
			"protocol_rules":        []any{map[string]any{"pattern": "upstream-b", "protocol": APIProtocolAnthropic}},
			"model_mapping":         map[string]any{"public-a": "upstream-b", "upstream-b": "wrong-c"},
			"compact_model_mapping": map[string]any{"public-a": "legacy-c"},
		},
	}
	v2Target, err := svc.resolveOpenAICompactionTarget(context.Background(), mapped, "public-a", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if v2Target.Model != "upstream-b" || v2Target.Protocol != APIProtocolAnthropic {
		t.Fatalf("v2 target = %#v", v2Target)
	}
	if v2Target.Endpoint != "https://anthropic.example.test/v1/messages" {
		t.Fatalf("v2 endpoint = %q", v2Target.Endpoint)
	}
	legacyTarget, err := svc.resolveOpenAICompactionTarget(context.Background(), mapped, "public-a", openAICompactionKindLegacy, false)
	if err != nil {
		t.Fatal(err)
	}
	if legacyTarget.Model != "upstream-b" || legacyTarget.Protocol != APIProtocolAnthropic {
		t.Fatalf("legacy route target = %#v, want ordinary Anthropic upstream-b", legacyTarget)
	}
	if legacyTarget.Endpoint != "https://anthropic.example.test/v1/messages" {
		t.Fatalf("legacy Anthropic endpoint = %q", legacyTarget.Endpoint)
	}
	if legacyTarget.ID() == v2Target.ID() {
		t.Fatal("v2 and legacy targets must not share evidence identity")
	}

	passthrough := &Account{
		ID: 203, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test", "base_url": "https://passthrough.example.test",
			"model_mapping": map[string]any{"public-a": "upstream-b"},
		},
		Extra: map[string]any{"openai_passthrough": true},
	}
	passthroughTarget, err := svc.resolveOpenAICompactionTarget(context.Background(), passthrough, "public-a", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if passthroughTarget.Model != "public-a" {
		t.Fatalf("passthrough target model = %q, want original public-a", passthroughTarget.Model)
	}
}

func TestOpenAICompactionTargetFingerprintInvalidatesTargetConfiguration(t *testing.T) {
	svc := &OpenAIGatewayService{}
	base := &Account{
		ID: 202, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-secret", "account_mode": AccountModePayG,
			"api_protocol": APIProtocolResponses, "base_url": "https://one.example.test",
		},
	}
	target, err := svc.resolveOpenAICompactionTarget(context.Background(), base, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	changedEndpoint := base.CloneForTest()
	changedEndpoint.Credentials["base_url"] = "https://two.example.test"
	next, err := svc.resolveOpenAICompactionTarget(context.Background(), changedEndpoint, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.ConfigFingerprint == target.ConfigFingerprint || next.Endpoint == target.Endpoint {
		t.Fatal("endpoint change must invalidate target evidence")
	}

	changedHeader := base.CloneForTest()
	changedHeader.Credentials["header_override_enabled"] = true
	changedHeader.Credentials["header_overrides"] = map[string]any{"x-route": "two"}
	next, err = svc.resolveOpenAICompactionTarget(context.Background(), changedHeader, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.ConfigFingerprint == target.ConfigFingerprint {
		t.Fatal("header override change must invalidate target evidence")
	}

	changedProxy := base.CloneForTest()
	proxyID := int64(77)
	changedProxy.ProxyID = &proxyID
	next, err = svc.resolveOpenAICompactionTarget(context.Background(), changedProxy, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.ConfigFingerprint == target.ConfigFingerprint {
		t.Fatal("proxy change must invalidate target evidence")
	}

	shadow := base.CloneForTest()
	shadow.Type = AccountTypeUpstream
	parentID := int64(999)
	shadow.ParentAccountID = &parentID
	shadow.Credentials = nil
	parent := &Account{ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"access_token": "refresh-a", "chatgpt_account_id": "chatgpt-a",
	}}
	next, err = svc.resolveOpenAICompactionTargetWithCredential(context.Background(), shadow, parent, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.CredentialSource != "credential_shadow" || next.CredentialAccountID != parentID || next.ConfigFingerprint == target.ConfigFingerprint {
		t.Fatalf("shadow target identity = %#v", next)
	}

	refreshedParent := parent.CloneForTest()
	refreshedParent.Credentials["access_token"] = "refresh-b"
	refreshed, err := svc.resolveOpenAICompactionTargetWithCredential(context.Background(), shadow, refreshedParent, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ConfigFingerprint != next.ConfigFingerprint {
		t.Fatal("normal OAuth token refresh must not invalidate the target")
	}
	reauthoredParent := parent.CloneForTest()
	reauthoredParent.Credentials["chatgpt_account_id"] = "chatgpt-b"
	reauthored, err := svc.resolveOpenAICompactionTargetWithCredential(context.Background(), shadow, reauthoredParent, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if reauthored.ConfigFingerprint == next.ConfigFingerprint {
		t.Fatal("re-authorizing the same parent ID to another ChatGPT identity must invalidate evidence")
	}

	setupA := &Account{ID: 204, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Credentials: map[string]any{"access_token": "setup-a"}}
	setupTargetA, err := svc.resolveOpenAICompactionTarget(context.Background(), setupA, "gpt-5.4", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	setupB := setupA.CloneForTest()
	setupB.Credentials["access_token"] = "setup-b"
	setupTargetB, err := svc.resolveOpenAICompactionTarget(context.Background(), setupB, "gpt-5.4", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if setupTargetA.ConfigFingerprint == setupTargetB.ConfigFingerprint {
		t.Fatal("manual setup-token credential replacement must invalidate evidence")
	}

	key, privateKey := newTestAgentIdentityKey(t)
	agent := &Account{ID: 205, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"auth_mode":          OpenAIAuthModeAgentIdentity,
		"agent_runtime_id":   key.runtimeID,
		"agent_private_key":  privateKey,
		"task_id":            key.taskID,
		"chatgpt_account_id": "agent-account",
	}}
	agentTarget, err := svc.resolveOpenAICompactionTarget(context.Background(), agent, "gpt-5.4", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	agentPayload, err := json.Marshal(agentTarget)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(agentPayload), key.runtimeID) || strings.Contains(string(agentPayload), privateKey) {
		t.Fatal("agent identity material must not be persisted in target evidence")
	}

	changedRuntime := agent.CloneForTest()
	changedRuntime.Credentials["agent_runtime_id"] = "runtime-rotated"
	changedRuntimeTarget, err := svc.resolveOpenAICompactionTarget(context.Background(), changedRuntime, "gpt-5.4", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if changedRuntimeTarget.ConfigFingerprint == agentTarget.ConfigFingerprint {
		t.Fatal("agent runtime change must invalidate target evidence")
	}

	_, rotatedPrivateKey := newTestAgentIdentityKey(t)
	changedKey := agent.CloneForTest()
	changedKey.Credentials["agent_private_key"] = rotatedPrivateKey
	changedKeyTarget, err := svc.resolveOpenAICompactionTarget(context.Background(), changedKey, "gpt-5.4", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if changedKeyTarget.ConfigFingerprint == agentTarget.ConfigFingerprint {
		t.Fatal("agent private-key change must invalidate target evidence")
	}

	missingRuntime := agent.CloneForTest()
	missingRuntime.Credentials["agent_runtime_id"] = ""
	switch OpenAICompactionCredentialFingerprint(missingRuntime) {
	case "":
	default:
		t.Fatal("missing agent runtime must make the credential fingerprint unknown")
	}
	missingKey := agent.CloneForTest()
	missingKey.Credentials["agent_private_key"] = ""
	switch OpenAICompactionCredentialFingerprint(missingKey) {
	case "":
	default:
		t.Fatal("missing agent private key must make the credential fingerprint unknown")
	}
}

func TestOpenAICompactionTargetPersistsSanitizedEndpointAndHashesFullEndpoint(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		ID: 205, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test", "account_mode": AccountModePayG, "api_protocol": APIProtocolResponses,
			"base_url": "https://url-user:fixture-secret@relay.example.test/v1?api_key=fixture-secret",
		},
	}
	target, err := svc.resolveOpenAICompactionTarget(context.Background(), account, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(target.Endpoint, "fixture-secret") || strings.Contains(target.Endpoint, "url-user") || strings.Contains(target.Endpoint, "api_key=") {
		t.Fatalf("persisted endpoint leaks URL credentials: %q", target.Endpoint)
	}
	if !strings.Contains(target.outboundEndpoint, "fixture-secret") {
		t.Fatal("outbound endpoint must retain the configured URL")
	}
	updates, err := BuildOpenAICompactionProbeExtraUpdates(nil, target, OpenAICompactionProbeObservation{Verdict: OpenAICompactionProbeVerdictSupported, CheckedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(updates)
	if strings.Contains(string(serialized), "fixture-secret") {
		t.Fatalf("serialized probe evidence leaks URL credentials: %s", serialized)
	}
	changedAccount := account.CloneForTest()
	changedAccount.Credentials["base_url"] = "https://relay.example.test/v1?api_key=fixture-secret-two"
	changed, err := svc.resolveOpenAICompactionTarget(context.Background(), changedAccount, "deepseek-chat", openAICompactionKindV2, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Endpoint != target.Endpoint || changed.EndpointHash == target.EndpointHash || changed.ConfigFingerprint == target.ConfigFingerprint {
		t.Fatal("query credential change must keep display endpoint but invalidate target identity")
	}
}

func TestOpenAICompactionTargetColdCatalogRemainsUnknown(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		ID: 206, Platform: PlatformCommandCode, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test", "api_protocol": APIProtocolAdaptive,
			"api_base_urls": map[string]any{APIProtocolChatCompletions: "https://catalog-cold.example.test"},
		},
	}
	_, err := svc.resolveOpenAICompactionTarget(context.Background(), account, "cold-model", openAICompactionKindV2, false)
	if err == nil {
		t.Fatalf("cold catalog error = %v, want unknown target", err)
	}
	if errors.Is(err, ErrOpenAICompactionProtocolUnknown) {
	} else {
		t.Fatalf("cold catalog error = %v, want unknown target", err)
	}
}

func TestOpenAICompactionProbeFailureEvidenceDoesNotPersistRawErrors(t *testing.T) {
	target := OpenAICompactionProbeTarget{
		Kind: OpenAICompactionKindV2, Model: "m", Protocol: APIProtocolResponses,
		Endpoint: "https://relay.example.test/v1/responses", EndpointHash: "hash", ConfigFingerprint: "config",
	}
	obs := classifyOpenAICompactionProbeObservation(nil, nil, errors.New(`Post "https://user:fixture-secret@relay.example.test/v1/responses?api_key=fixture-secret": dial failed`), openAICompactProbeUnknown, time.Now())
	updates, err := BuildOpenAICompactionProbeExtraUpdates(nil, target, obs)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(updates)
	if strings.Contains(string(serialized), "fixture-secret") {
		t.Fatalf("probe failure evidence leaked URL credentials: %s", serialized)
	}
}

func TestOpenAICompactionTargetLegacyUsesGlobalFallbackModel(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-global-compact"}}}
	account := &Account{
		ID: 207, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://openai.example.test"},
	}
	target, err := svc.resolveOpenAICompactionTarget(context.Background(), account, "gpt-5.5", openAICompactionKindLegacy, false)
	if err != nil {
		t.Fatal(err)
	}
	if target.Model != "gpt-global-compact" {
		t.Fatalf("legacy final model = %q, want global compact model", target.Model)
	}

	specific := account.CloneForTest()
	specific.Credentials["model_mapping"] = map[string]any{"gpt-5.5": "ordinary-upstream"}
	specific.Credentials["compact_model_mapping"] = map[string]any{"gpt-5.5": "specific-compact"}
	specificTarget, err := svc.resolveOpenAICompactionTarget(context.Background(), specific, "gpt-5.5", openAICompactionKindLegacy, false)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case specificTarget.Model == "specific-compact" && specificTarget.Protocol == APIProtocolResponses:
	default:
		t.Fatalf("legacy compact-specific target = %#v", specificTarget)
	}
}

func TestOpenAICompactionProbeEvidenceKeepsTargetsAndUnknownDoesNotRefreshSuccess(t *testing.T) {
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	v2 := OpenAICompactionProbeTarget{Kind: openAICompactionKindV2, Model: "m", Protocol: APIProtocolResponses, Endpoint: "https://x/v1/responses", ConfigFingerprint: "f1"}
	legacy := OpenAICompactionProbeTarget{Kind: openAICompactionKindLegacy, Model: "m", Protocol: APIProtocolResponses, Endpoint: "https://x/v1/responses/compact", ConfigFingerprint: "f1"}
	extra := map[string]any{}
	updates, err := BuildOpenAICompactionProbeExtraUpdates(extra, v2, OpenAICompactionProbeObservation{Verdict: OpenAICompactionProbeVerdictSupported, CheckedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	extra = updates
	updates, err = BuildOpenAICompactionProbeExtraUpdates(extra, legacy, OpenAICompactionProbeObservation{Verdict: OpenAICompactionProbeVerdictUnsupported, CheckedAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	extra = updates
	records := decodeOpenAICompactionProbeRecords(extra)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2 independent targets", len(records))
	}
	before := records[v2.ID()]
	updates, err = BuildOpenAICompactionProbeExtraUpdates(extra, v2, OpenAICompactionProbeObservation{Verdict: OpenAICompactionProbeVerdictUnknown, CheckedAt: now.Add(2 * time.Minute), Status: http.StatusTooManyRequests, ErrorClass: openAICompactionErrorRateLimit})
	if err != nil {
		t.Fatal(err)
	}
	after := decodeOpenAICompactionProbeRecords(updates)[v2.ID()]
	if after.Verdict != OpenAICompactionProbeVerdictSupported || after.VerifiedAt != before.VerifiedAt {
		t.Fatalf("unknown observation refreshed verified evidence: before=%#v after=%#v", before, after)
	}
	if after.LastCheckedAt == before.LastCheckedAt || after.ErrorClass != openAICompactionErrorRateLimit {
		t.Fatalf("last attempt was not recorded: %#v", after)
	}
}

func TestOpenAICompactionProbeEvidenceCapacityIsBounded(t *testing.T) {
	extra := map[string]any{}
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	for i := 0; i < openAICompactionProbeMaxTargets+5; i++ {
		target := OpenAICompactionProbeTarget{Kind: openAICompactionKindV2, Model: fmt.Sprintf("m-%02d", i), Protocol: APIProtocolResponses, Endpoint: "https://x", ConfigFingerprint: fmt.Sprintf("f-%02d", i)}
		var err error
		extra, err = BuildOpenAICompactionProbeExtraUpdates(extra, target, OpenAICompactionProbeObservation{Verdict: OpenAICompactionProbeVerdictSupported, CheckedAt: now.Add(time.Duration(i) * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if records := decodeOpenAICompactionProbeRecords(extra); len(records) != openAICompactionProbeMaxTargets {
		t.Fatalf("records = %d, want cap %d", len(records), openAICompactionProbeMaxTargets)
	}
}

func TestClassifyOpenAICompactionProbeObservation(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		status     int
		body       string
		err        error
		result     openAICompactProbeResult
		want       string
		errorClass string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"bad token"}}`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorAuth},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorRateLimit},
		{"server error", http.StatusBadGateway, `bad gateway`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorUpstream5xx},
		{"timeout", 0, "", context.DeadlineExceeded, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorTimeout},
		{"truncated", http.StatusOK, `data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"x"}}`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorTruncated},
		{"model missing", http.StatusNotFound, `{"error":{"message":"model not found"}}`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorModelNotFound},
		{"model missing code", http.StatusNotFound, `{"error":{"code":"model_not_found","message":"does not exist"}}`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorModelNotFound},
		{"endpoint unsupported", http.StatusNotFound, `404 page not found`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnsupported, openAICompactionErrorEndpointUnsupported},
		{"supported", http.StatusOK, "ok", nil, openAICompactProbeSupported, OpenAICompactionProbeVerdictSupported, openAICompactionErrorNone},
		{"no response", 0, "", nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorTransport},
		{"transport failure", 0, "", errors.New("dial failed"), openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorTransport},
		{"unknown model", http.StatusBadRequest, `{"error":{"message":"unknown model gpt-5.4-openai-compact"}}`, nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorRequest},
		{"empty unavailable", http.StatusServiceUnavailable, "", nil, openAICompactProbeUnknown, OpenAICompactionProbeVerdictUnknown, openAICompactionErrorUpstream5xx},
		{"invalid terminal", http.StatusOK, "", nil, openAICompactProbeUnsupported, OpenAICompactionProbeVerdictUnsupported, openAICompactionErrorInvalidContract},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.status != 0 {
				resp = &http.Response{StatusCode: tt.status}
			}
			obs := classifyOpenAICompactionProbeObservation(resp, []byte(tt.body), tt.err, tt.result, now)
			if obs.Verdict != tt.want || obs.ErrorClass != tt.errorClass {
				t.Fatalf("observation = %#v, want verdict=%s class=%s", obs, tt.want, tt.errorClass)
			}
		})
	}
}

func TestOpenAICompactionProbeJSONAndTerminalOnlyAreNotV2Support(t *testing.T) {
	jsonBody := []byte(`{"status":"completed","output":[{"type":"compaction","encrypted_content":"blob"}]}`)
	if got := classifyOpenAICompactProbeSuccess(jsonBody); got != openAICompactProbeUnsupported {
		t.Fatalf("completed JSON result = %v, want unsupported for v2", got)
	}
	terminalOnly := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"compaction\",\"encrypted_content\":\"blob\"}]}}\n\n")
	if got := classifyOpenAICompactProbeSuccess(terminalOnly); got == openAICompactProbeSupported {
		t.Fatal("terminal-only compaction must not prove v2 support")
	}
	if got := classifyOpenAICompactProbeSuccess([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"boom\"}}\n\n")); got != openAICompactProbeUnknown {
		t.Fatalf("mid-stream error result = %v, want unknown", got)
	}
}

// CloneForTest provides an isolated account copy for target-resolution tests.
func (a *Account) CloneForTest() *Account {
	if a == nil {
		return nil
	}
	copy := *a
	copy.Credentials = cloneAnyMap(a.Credentials)
	copy.Extra = cloneAnyMap(a.Extra)
	return &copy
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
