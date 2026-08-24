package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/anthropictokenizer"
	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func TestKiroCacheEmulationGroupDefaultsAndNonKiro(t *testing.T) {
	kiro := &Group{Platform: PlatformKiro, KiroCacheEmulationEnabled: true, KiroCacheEmulationRatio: 0.5}
	if !kiro.EffectiveKiroCacheEmulationEnabled() {
		t.Fatal("kiro group should enable cache emulation")
	}
	if got := kiro.EffectiveKiroCacheEmulationRatio(); got != 0.5 {
		t.Fatalf("ratio = %v, want 0.5", got)
	}
	nonKiro := &Group{Platform: PlatformAnthropic, KiroCacheEmulationEnabled: true, KiroCacheEmulationRatio: 1}
	NormalizeGroupRuntimeFields(nonKiro)
	if nonKiro.KiroCacheEmulationEnabled || nonKiro.KiroCacheEmulationRatio != 0 {
		t.Fatalf("non-kiro fields were not normalized: %+v", nonKiro)
	}
}

func TestKiroCacheEmulationUsesSnapshotGroupWithoutRepo(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 34, Platform: PlatformKiro}
	group := kiroCacheGroup(1)
	first := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, kiroCacheRequestBody("stable", false), "claude-sonnet-4-6", 2000)
	if first == nil || first.CacheCreationInputTokens != 2000 || first.CacheReadInputTokens != 0 || first.InputTokens != 0 {
		t.Fatalf("unexpected first usage: %+v", first)
	}
	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, kiroCacheRequestBody("stable", false), "claude-sonnet-4-6", 2000)
	if second == nil || second.CacheReadInputTokens != 2000 || second.CacheCreationInputTokens != 0 || second.InputTokens != 0 {
		t.Fatalf("unexpected second usage: %+v", second)
	}
}

// prepareKiroCacheEmulationUsage 在 commit() 之前不得改动 tracker：连续两次 prepare
// 且都不 commit，应当观察到完全相同的（未命中）状态，证明 prepare 从未写入缓存条目。
func TestKiroCacheEmulationPrepareDoesNotMutateUntilCommit(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 55, Platform: PlatformKiro}
	group := kiroCacheGroup(1)
	body := kiroCacheRequestBody("deferred commit", false)

	planA := svc.prepareKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, planA)
	usageA := planA.result()
	require.NotNil(t, usageA)
	require.Equal(t, 2000, usageA.CacheCreationInputTokens)
	require.Equal(t, 0, usageA.CacheReadInputTokens)

	// 未 commit：同样内容的第二次 prepare 仍应观察到未命中，
	// 证明第一次 prepare 没有写入 tracker。
	planB := svc.prepareKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, planB)
	usageB := planB.result()
	require.NotNil(t, usageB)
	require.Equal(t, 2000, usageB.CacheCreationInputTokens)
	require.Equal(t, 0, usageB.CacheReadInputTokens)

	// 提交后，后续 prepare 应观察到缓存命中。
	planB.commit()
	planC := svc.prepareKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, planC)
	usageC := planC.result()
	require.NotNil(t, usageC)
	require.Equal(t, 2000, usageC.CacheReadInputTokens)
	require.Equal(t, 0, usageC.CacheCreationInputTokens)
}

func TestKiroCacheEmulationRatioScalesTokens(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 78, Platform: PlatformKiro}
	usage := svc.buildKiroCacheEmulationUsage(context.Background(), account, kiroCacheGroup(0.5), kiroCacheRequestBody("ratio", false), "claude-sonnet-4-6", 2000)
	if usage == nil || usage.CacheCreationInputTokens != 1000 || usage.InputTokens != 1000 {
		t.Fatalf("unexpected scaled usage: %+v", usage)
	}
	disabled := kiroCacheGroup(1)
	disabled.KiroCacheEmulationEnabled = false
	if got := svc.buildKiroCacheEmulationUsage(context.Background(), account, disabled, kiroCacheRequestBody("disabled", false), "claude-sonnet-4-6", 2000); got != nil {
		t.Fatalf("disabled group should skip cache emulation, got %+v", got)
	}
}

func TestKiroCacheEmulationIndependentRatiosScaleSeparately(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 79, Platform: PlatformKiro}
	group := kiroCacheGroup(1)
	group.KiroCacheEmulationMode = KiroCacheEmulationModeIndependent
	group.KiroCacheCreationEmulationRatio = 0.75
	group.KiroCacheReadEmulationRatio = 0.25
	body := kiroCacheRequestBody("independent ratios", false)

	first := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, first)
	require.Equal(t, 1500, first.CacheCreationInputTokens)
	require.Zero(t, first.CacheReadInputTokens)
	require.Equal(t, 500, first.InputTokens)

	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, second)
	require.Zero(t, second.CacheCreationInputTokens)
	require.Equal(t, 500, second.CacheReadInputTokens)
	require.Equal(t, 1500, second.InputTokens)
}

func TestScaleKiroCacheCreationTTLTokensPreservesScaledTotal(t *testing.T) {
	tokens5m, tokens1h := scaleKiroCacheCreationTTLTokens(2000, 0, 1000, 0.5)
	require.Equal(t, 1000, tokens5m+tokens1h)
	require.Equal(t, 1000, tokens5m)
	require.Zero(t, tokens1h)

	tokens5m, tokens1h = scaleKiroCacheCreationTTLTokens(0, 2000, 1000, 0.5)
	require.Equal(t, 1000, tokens5m+tokens1h)
	require.Zero(t, tokens5m)
	require.Equal(t, 1000, tokens1h)

	tokens5m, tokens1h = scaleKiroCacheCreationTTLTokens(0, 0, 1000, 0.5)
	require.Zero(t, tokens5m)
	require.Zero(t, tokens1h)
}

func TestScaleKiroCacheCreationTTLTokensHandlesFutureMixedBuckets(t *testing.T) {
	tokens5m, tokens1h := scaleKiroCacheCreationTTLTokens(1001, 999, 1000, 0.5)
	require.Equal(t, 1000, tokens5m+tokens1h)
	require.Equal(t, 501, tokens5m)
	require.Equal(t, 499, tokens1h)

	tokens5m, tokens1h = scaleKiroCacheCreationTTLTokens(3000, 1, 1000, 0.5)
	require.Equal(t, 1000, tokens5m)
	require.Zero(t, tokens1h)
}

func TestKiroCacheEmulationAccountIsolation(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	group := kiroCacheGroup(1)
	body := kiroCacheRequestBody("account isolation", false)
	first := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(1, "refresh-a", "access-a"), group, body, "claude-sonnet-4-6", 2000)
	if first == nil || first.CacheCreationInputTokens != 2000 {
		t.Fatalf("unexpected first usage: %+v", first)
	}
	otherAccount := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(2, "refresh-b", "access-b"), group, body, "claude-sonnet-4-6", 2000)
	if otherAccount == nil || otherAccount.CacheCreationInputTokens != 2000 || otherAccount.CacheReadInputTokens != 0 {
		t.Fatalf("cache should be isolated by account: %+v", otherAccount)
	}
}

// 缓存命名空间按 account.ID 隔离，必须对「凭证轮转」完全免疫：access_token 与
// refresh_token 都会在正常刷新中被覆盖写回（每小时至少一次），若参与键计算就会把该账号
// 的全部前缀指纹作废、退回冷启动。理由详见 kiroCacheCredentialKey 的 godoc。
func TestKiroCacheEmulationKeyIsImmuneToCredentialRotation(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	group := kiroCacheGroup(1)
	body := kiroCacheRequestBody("credential rotation", false)

	first := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(7, "refresh-same", "access-a"), group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, first)
	require.Equal(t, 2000, first.CacheCreationInputTokens)

	rotatedAccessToken := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(7, "refresh-same", "access-b"), group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, rotatedAccessToken)
	require.Equal(t, 2000, rotatedAccessToken.CacheReadInputTokens, "access token rotation must not break cache")
	require.Zero(t, rotatedAccessToken.CacheCreationInputTokens)

	// refresh_token 轮转是每小时都会发生的正常刷新，必须仍命中同一命名空间。
	rotatedRefreshToken := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(7, "refresh-rotated", "access-c"), group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, rotatedRefreshToken)
	require.Equal(t, 2000, rotatedRefreshToken.CacheReadInputTokens, "refresh token rotation must not break cache")
	require.Zero(t, rotatedRefreshToken.CacheCreationInputTokens)
}

// 共用同一 OAuth 客户端应用（client_id / client_id_hash 相同）的不同账号之间不得串缓存。
// 跨账号误命中会让 cache_read 按 1/10 价计费而上游实际全价，属少计费。
func TestKiroCacheEmulationDoesNotShareAcrossAccountsWithSameClientID(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	group := kiroCacheGroup(1)
	body := kiroCacheRequestBody("shared client id", false)

	first := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(11, "refresh-a", "access-a"), group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, first)
	require.Equal(t, 2000, first.CacheCreationInputTokens)

	// kiroCacheAccount 固定写入同一个 client_id，模拟共用 OAuth 应用注册的两个账号。
	otherAccount := svc.buildKiroCacheEmulationUsage(context.Background(), kiroCacheAccount(12, "refresh-a", "access-a"), group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, otherAccount)
	require.Zero(t, otherAccount.CacheReadInputTokens, "accounts sharing an OAuth client must not share cache")
	require.Equal(t, 2000, otherAccount.CacheCreationInputTokens)
}

func TestKiroCacheEmulationContentChangeMisses(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 3, Platform: PlatformKiro}
	group := kiroCacheGroup(1)
	_ = svc.buildKiroCacheEmulationUsage(context.Background(), account, group, kiroCacheRequestBody("before", false), "claude-sonnet-4-6", 2000)
	changed := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, kiroCacheRequestBody("after", false), "claude-sonnet-4-6", 2000)
	if changed == nil || changed.CacheCreationInputTokens != 2000 || changed.CacheReadInputTokens != 0 {
		t.Fatalf("changed content should miss: %+v", changed)
	}
}

func TestKiroCacheEmulationTTLExpiry(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 4, Platform: PlatformKiro}
	group := kiroCacheGroup(1)
	body := kiroCacheRequestBody("ttl", false)
	_ = svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	globalKiroCacheTracker.mu.Lock()
	for accountID, entries := range globalKiroCacheTracker.entries {
		for fp, entry := range entries {
			entry.expiresAt = time.Now().Add(-time.Second)
			globalKiroCacheTracker.entries[accountID][fp] = entry
		}
	}
	globalKiroCacheTracker.mu.Unlock()
	afterExpiry := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	if afterExpiry == nil || afterExpiry.CacheCreationInputTokens != 2000 || afterExpiry.CacheReadInputTokens != 0 {
		t.Fatalf("expired cache should be recreated: %+v", afterExpiry)
	}
}

func TestKiroCacheEmulationOneHourBucket(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	usage := svc.buildKiroCacheEmulationUsage(context.Background(), &Account{ID: 5, Platform: PlatformKiro}, kiroCacheGroup(1), kiroCacheRequestBody("1h", true), "claude-sonnet-4-6", 2000)
	if usage == nil || usage.CacheCreationInputTokens != 2000 || usage.CacheCreation1hInputTokens != 2000 || usage.CacheCreation5mInputTokens != 0 {
		t.Fatalf("unexpected 1h bucket usage: %+v", usage)
	}
}

func TestKiroCacheEmulationPrefixPartialHit(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := &Account{ID: 6, Platform: PlatformKiro}
	group := kiroCacheGroup(1)
	firstBody := kiroCacheMultiMessageBody("cached prefix", "tail one")
	secondBody := kiroCacheMultiMessageBody("cached prefix", "tail two")
	first := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, firstBody, "claude-sonnet-4-6", 6000)
	if first == nil || first.CacheCreationInputTokens <= 0 {
		t.Fatalf("unexpected first usage: %+v", first)
	}
	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, secondBody, "claude-sonnet-4-6", 6000)
	if second == nil || second.CacheReadInputTokens <= 0 || second.CacheReadInputTokens >= first.CacheCreationInputTokens || second.CacheCreationInputTokens <= 0 {
		t.Fatalf("expected partial prefix hit: %+v", second)
	}
}

func TestKiroInputTokenEstimateIgnoresClientMetadata(t *testing.T) {
	bodyWithoutMetadata := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello world"}]}`)
	bodyWithMetadata := []byte(`{"model":"claude-sonnet-4-6","metadata":{"input_tokens":999999},"messages":[{"role":"user","content":"hello world"}]}`)
	withoutMetadata := estimateKiroInputTokens(context.Background(), bodyWithoutMetadata)
	withMetadata := estimateKiroInputTokens(context.Background(), bodyWithMetadata)
	if withMetadata == 999999 {
		t.Fatal("client metadata.input_tokens must not be trusted")
	}
	if withMetadata <= 0 || withoutMetadata <= 0 || withMetadata > withoutMetadata*2 {
		t.Fatalf("unexpected estimates without=%d with=%d", withoutMetadata, withMetadata)
	}
}

func TestKiroTokenCountersMatchReferenceRules(t *testing.T) {
	if got := anthropictokenizer.CountTokens("abc def"); got != 1 {
		t.Fatalf("english tokens = %d, want 1", got)
	}
	if got := anthropictokenizer.CountTokens("你好世界"); got != 1 {
		t.Fatalf("cjk tokens = %d, want 1", got)
	}
	if kiroTokensPerTool != 150 {
		t.Fatalf("tool tokens = %d, want 150", kiroTokensPerTool)
	}
	if got := countKiroMessageContentTokens(context.Background(), map[string]any{"thinking": "abc def"}); got != 1 {
		t.Fatalf("thinking tokens = %d, want 1", got)
	}
	if got := countKiroMessageContentTokens(context.Background(), map[string]any{"input": map[string]any{"path": "/tmp/a.txt"}}); got <= 0 {
		t.Fatalf("tool input tokens should be positive, got %d", got)
	}
	if got := countKiroMessageContentTokens(context.Background(), map[string]any{"content": []any{map[string]any{"text": "abc"}, map[string]any{"text": "你好"}}}); got != 2 {
		t.Fatalf("tool result content tokens = %d, want 2", got)
	}
}

func TestKiroInputTokenEstimateSeparatesVisualTokensFromBase64(t *testing.T) {
	dataURL := kiroPNGDataURL(t, 512, 512, color.RGBA{R: 37, G: 89, B: 151, A: 255})
	body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":%q}}]}]}`, strings.TrimPrefix(dataURL, "data:image/png;base64,")))

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	sanitized, imageTokens := sanitizeKiroImagesForTokenEstimate(context.Background(), payload["messages"])
	canonical, err := canonicalJSON(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(canonical, []byte(strings.TrimPrefix(dataURL, "data:image/png;base64,"))) {
		t.Fatal("sanitized token payload must not retain image base64")
	}
	if imageTokens != 350 {
		t.Fatalf("image tokens = %d, want 350", imageTokens)
	}

	got := estimateKiroInputTokens(context.Background(), body)
	want := anthropictokenizer.CountTokens("describe") + imageTokens
	if got < want || got > want+50 {
		t.Fatalf("input token estimate = %d, expected visual-aware estimate near %d", got, want)
	}
}

func TestKiroImageTokenSourcesSupportAnthropicAndOpenAIShapes(t *testing.T) {
	dataURL := kiroPNGDataURL(t, 200, 200, color.RGBA{A: 255})
	base64Data := strings.TrimPrefix(dataURL, "data:image/png;base64,")
	tests := []map[string]any{
		{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": base64Data}},
		{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
		{"type": "input_image", "image_url": dataURL},
	}
	for _, block := range tests {
		if got := countKiroMessageContentTokens(context.Background(), block); got != 54 {
			t.Fatalf("image block %#v tokens = %d, want 54", block, got)
		}
	}
}

func TestKiroCacheEmulationIncludesImageTokensAndKeepsImageFingerprint(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(91, "refresh-image", "access-image")
	group := kiroCacheGroup(1)
	prefix := strings.Repeat("cacheable visual prompt ", 700)
	body := kiroCacheImageRequestBody(t, prefix, color.RGBA{R: 1, A: 255})
	inputTokens := estimateKiroInputTokens(context.Background(), body)

	first := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", inputTokens)
	if first == nil || first.CacheCreationInputTokens <= 0 || first.CacheReadInputTokens != 0 {
		t.Fatalf("unexpected first image cache usage: %+v", first)
	}
	if first.InputTokens+first.CacheCreationInputTokens+first.CacheReadInputTokens != inputTokens {
		t.Fatalf("first image cache token totals do not balance: usage=%+v total=%d", first, inputTokens)
	}

	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", inputTokens)
	if second == nil || second.CacheReadInputTokens <= 0 {
		t.Fatalf("same image should hit cache: %+v", second)
	}
	if second.InputTokens+second.CacheCreationInputTokens+second.CacheReadInputTokens != inputTokens {
		t.Fatalf("second image cache token totals do not balance: usage=%+v total=%d", second, inputTokens)
	}

	changedBody := kiroCacheImageRequestBody(t, prefix, color.RGBA{G: 1, A: 255})
	changedTokens := estimateKiroInputTokens(context.Background(), changedBody)
	changed := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, changedBody, "claude-sonnet-4-6", changedTokens)
	if changed == nil || changed.CacheReadInputTokens != 0 || changed.CacheCreationInputTokens <= 0 {
		t.Fatalf("different image must miss cache: %+v", changed)
	}
}

func TestKiroResponsesCacheEmulationUsesFullInputPrefix(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(101, "refresh-responses", "access-responses")
	group := kiroCacheGroup(1)
	body := kiroResponsesCacheRequestBody("stable", "workspace-a", "resp-a")

	first := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, body, "gpt-5", 2400)
	if first == nil || first.CacheCreationInputTokens != 2400 || first.CacheReadInputTokens != 0 || first.InputTokens != 0 {
		t.Fatalf("unexpected first responses usage: %+v", first)
	}

	second := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, body, "gpt-5", 2400)
	if second == nil || second.CacheReadInputTokens != 2400 || second.CacheCreationInputTokens != 0 || second.InputTokens != 0 {
		t.Fatalf("unexpected second responses usage: %+v", second)
	}
}

func TestKiroResponsesCacheEmulationHitsStableHistoryPrefix(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(106, "refresh-responses-history", "access-responses-history")
	group := kiroCacheGroup(1)
	prefixText := strings.Repeat("stable codex history prefix ", 640)
	firstBody := kiroResponsesConversationRequestBody("workspace-history", []string{prefixText})
	secondBody := kiroResponsesConversationRequestBody("workspace-history", []string{prefixText, strings.Repeat("new codex tail ", 160)})

	first := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, firstBody, "gpt-5", 2600)
	if first == nil || first.CacheReadInputTokens != 0 || first.CacheCreationInputTokens <= 0 {
		t.Fatalf("first request should create cache: %+v", first)
	}

	second := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, secondBody, "gpt-5", 3200)
	if second == nil || second.CacheReadInputTokens <= 0 || second.CacheCreationInputTokens <= 0 {
		t.Fatalf("grown conversation should read stable prefix and create tail: %+v", second)
	}
	if second.CacheReadInputTokens >= 3200 {
		t.Fatalf("grown conversation should not treat the whole request as cache read: %+v", second)
	}
}

func TestKiroResponsesCacheEmulationDoesNotReadChangedLatestItem(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(107, "refresh-responses-tail", "access-responses-tail")
	group := kiroCacheGroup(1)
	stablePrefix := strings.Repeat("stable codex history prefix ", 640)
	firstBody := kiroResponsesConversationRequestBody("workspace-tail", []string{stablePrefix, strings.Repeat("first latest item ", 180)})
	secondBody := kiroResponsesConversationRequestBody("workspace-tail", []string{stablePrefix, strings.Repeat("changed latest item ", 180)})

	first := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, firstBody, "gpt-5", 3200)
	if first == nil || first.CacheCreationInputTokens <= 0 || first.CacheReadInputTokens != 0 {
		t.Fatalf("first request should create cache: %+v", first)
	}

	second := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, secondBody, "gpt-5", 3200)
	if second == nil || second.CacheReadInputTokens <= 0 || second.CacheCreationInputTokens <= 0 {
		t.Fatalf("changed latest item should read stable prefix and create changed tail: %+v", second)
	}
	if second.CacheReadInputTokens >= first.CacheCreationInputTokens {
		t.Fatalf("changed latest item should not be treated as a full cache read: first=%+v second=%+v", first, second)
	}
}

func TestKiroResponsesCacheEmulationPromptCacheKeyIsolatesNamespaces(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(102, "refresh-responses-key", "access-responses-key")
	group := kiroCacheGroup(1)
	bodyA := kiroResponsesCacheRequestBody("same", "workspace-a", "resp-a")
	bodyB := kiroResponsesCacheRequestBody("same", "workspace-b", "resp-a")

	_ = svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, bodyA, "gpt-5", 2400)
	otherNamespace := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, bodyB, "gpt-5", 2400)
	if otherNamespace == nil || otherNamespace.CacheReadInputTokens != 0 || otherNamespace.CacheCreationInputTokens != 2400 {
		t.Fatalf("different prompt_cache_key should miss: %+v", otherNamespace)
	}
}

func TestKiroResponsesCacheEmulationPreviousResponseIDIsolatesNamespaces(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(103, "refresh-responses-prev", "access-responses-prev")
	group := kiroCacheGroup(1)
	bodyA := kiroResponsesCacheRequestBody("same", "workspace-a", "resp-a")
	bodyB := kiroResponsesCacheRequestBody("same", "workspace-a", "resp-b")

	_ = svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, bodyA, "gpt-5", 2400)
	otherPrevious := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, bodyB, "gpt-5", 2400)
	if otherPrevious == nil || otherPrevious.CacheReadInputTokens != 0 || otherPrevious.CacheCreationInputTokens != 2400 {
		t.Fatalf("different previous_response_id should miss: %+v", otherPrevious)
	}
}

func TestKiroResponsesCacheEmulationPreludeFieldsAffectFingerprint(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(104, "refresh-responses-prelude", "access-responses-prelude")
	group := kiroCacheGroup(1)
	base := kiroResponsesCacheRequestBodyWithOptions("same", "workspace-a", "resp-a", "gpt-5", "auto", "medium", `{"type":"json_object"}`, "lookup")
	changed := kiroResponsesCacheRequestBodyWithOptions("same", "workspace-a", "resp-a", "gpt-5-mini", "required", "high", `{"type":"text"}`, "search")

	_ = svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, base, "gpt-5", 2400)
	miss := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, changed, "gpt-5-mini", 2400)
	if miss == nil || miss.CacheReadInputTokens != 0 || miss.CacheCreationInputTokens != 2400 {
		t.Fatalf("model/tools/tool_choice/reasoning/text changes should miss: %+v", miss)
	}
}

func TestKiroResponsesCacheEmulationIncludesImageFingerprint(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(105, "refresh-responses-image", "access-responses-image")
	group := kiroCacheGroup(1)
	body := kiroResponsesImageCacheRequestBody(t, "same", color.RGBA{R: 1, A: 255})

	first := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, body, "gpt-5", 2400)
	if first == nil || first.CacheCreationInputTokens != 2400 || first.CacheReadInputTokens != 0 {
		t.Fatalf("unexpected first responses image usage: %+v", first)
	}
	second := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, body, "gpt-5", 2400)
	if second == nil || second.CacheReadInputTokens != 2400 || second.CacheCreationInputTokens != 0 {
		t.Fatalf("same responses image should hit: %+v", second)
	}

	changed := kiroResponsesImageCacheRequestBody(t, "same", color.RGBA{G: 1, A: 255})
	miss := svc.buildKiroResponsesCacheEmulationUsage(context.Background(), account, group, changed, "gpt-5", 2400)
	if miss == nil || miss.CacheReadInputTokens != 0 || miss.CacheCreationInputTokens != 2400 {
		t.Fatalf("different responses image should miss: %+v", miss)
	}
}

func TestKiroChatCompletionsCacheEmulationHitsStableHistoryPrefix(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(701, "refresh-chat", "access-chat")
	group := kiroCacheGroup(1)

	firstMessage := strings.Repeat("stable chat history chunk ", 700)
	secondMessage := strings.Repeat("latest chat turn chunk one ", 180)
	thirdMessage := strings.Repeat("latest chat turn chunk two ", 180)

	mappedModel := "claude-sonnet-4-6"
	inputTokens := 6000
	firstBody := kiroChatCompletionsConversationBody([]string{firstMessage, secondMessage})
	first := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, firstBody, mappedModel, inputTokens)
	require.NotNil(t, first)
	require.Equal(t, 0, first.CacheReadInputTokens)
	require.Greater(t, first.CacheCreationInputTokens, 0)

	secondBody := kiroChatCompletionsConversationBody([]string{firstMessage, thirdMessage})
	second := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, secondBody, mappedModel, inputTokens)
	require.NotNil(t, second)
	require.Greater(t, second.CacheReadInputTokens, 0)
	require.Greater(t, second.CacheCreationInputTokens, 0)
	require.Less(t, second.CacheCreationInputTokens, first.CacheCreationInputTokens)
}

func TestKiroChatCompletionsCacheEmulationDoesNotReadChangedHistory(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(702, "refresh-chat", "access-chat")
	group := kiroCacheGroup(1)

	stable := strings.Repeat("stable chat history chunk ", 700)
	latest := strings.Repeat("latest chat turn chunk ", 180)

	mappedModel := "claude-sonnet-4-6"
	inputTokens := 6000
	firstBody := kiroChatCompletionsConversationBody([]string{stable, latest})
	first := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, firstBody, mappedModel, inputTokens)
	require.NotNil(t, first)
	require.Equal(t, 0, first.CacheReadInputTokens)

	changedHistory := strings.Repeat("changed chat history chunk ", 700)
	secondBody := kiroChatCompletionsConversationBody([]string{changedHistory, latest})
	second := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, secondBody, mappedModel, inputTokens)
	require.NotNil(t, second)
	require.Equal(t, 0, second.CacheReadInputTokens)
	require.Greater(t, second.CacheCreationInputTokens, 0)
}

func TestKiroChatCompletionsCacheEmulationIncludesModelAndToolsInIdentity(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(703, "refresh-chat", "access-chat")
	group := kiroCacheGroup(1)

	stable := strings.Repeat("stable chat history chunk ", 700)
	latest := strings.Repeat("latest chat turn chunk ", 180)
	body := kiroChatCompletionsConversationBody([]string{stable, latest})

	mappedModel := "claude-sonnet-4-6"
	inputTokens := 6000
	first := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, body, mappedModel, inputTokens)
	require.NotNil(t, first)

	otherModel := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, body, "claude-opus-4-1", inputTokens)
	require.NotNil(t, otherModel)
	require.Equal(t, 0, otherModel.CacheReadInputTokens)

	changedToolsBody := []byte(strings.Replace(string(body), `"name":"lookup"`, `"name":"search"`, 1))
	changedTools := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, changedToolsBody, mappedModel, inputTokens)
	require.NotNil(t, changedTools)
	require.Equal(t, 0, changedTools.CacheReadInputTokens)
}

func TestKiroChatCompletionsCacheEmulationIncludesMessageNameInIdentity(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(704, "refresh-chat", "access-chat")
	group := kiroCacheGroup(1)

	stable := strings.Repeat("stable chat history chunk ", 700)
	latest := strings.Repeat("latest chat turn chunk ", 180)
	mappedModel := "claude-sonnet-4-6"
	inputTokens := 6000

	firstBody := []byte(fmt.Sprintf(`{"model":"gpt-5","messages":[{"role":"user","name":"alice","content":%q},{"role":"user","content":%q}]}`, stable, latest))
	first := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, firstBody, mappedModel, inputTokens)
	require.NotNil(t, first)
	require.Equal(t, 0, first.CacheReadInputTokens)

	changedNameBody := []byte(fmt.Sprintf(`{"model":"gpt-5","messages":[{"role":"user","name":"bob","content":%q},{"role":"user","content":%q}]}`, stable, latest))
	changedName := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, changedNameBody, mappedModel, inputTokens)
	require.NotNil(t, changedName)
	require.Equal(t, 0, changedName.CacheReadInputTokens)
	require.Greater(t, changedName.CacheCreationInputTokens, 0)
}

func TestKiroChatCompletionsCacheEmulationDoesNotReadInstructionsOnlyPrefix(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(705, "refresh-chat", "access-chat")
	group := kiroCacheGroup(1)

	instructions := strings.Repeat("stable instruction chunk ", 700)
	firstHistory := strings.Repeat("first chat history chunk ", 700)
	secondHistory := strings.Repeat("second chat history chunk ", 700)
	latest := strings.Repeat("latest chat turn chunk ", 180)
	mappedModel := "claude-sonnet-4-6"
	inputTokens := 9000

	firstBody := []byte(fmt.Sprintf(`{"model":"gpt-5","instructions":%q,"messages":[{"role":"user","content":%q},{"role":"user","content":%q}]}`, instructions, firstHistory, latest))
	first := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, firstBody, mappedModel, inputTokens)
	require.NotNil(t, first)
	require.Equal(t, 0, first.CacheReadInputTokens)

	secondBody := []byte(fmt.Sprintf(`{"model":"gpt-5","instructions":%q,"messages":[{"role":"user","content":%q},{"role":"user","content":%q}]}`, instructions, secondHistory, latest))
	second := svc.buildKiroChatCompletionsCacheEmulationUsage(context.Background(), account, group, secondBody, mappedModel, inputTokens)
	require.NotNil(t, second)
	require.Equal(t, 0, second.CacheReadInputTokens)
	require.Greater(t, second.CacheCreationInputTokens, 0)
}

func resetKiroCacheTracker() {
	globalKiroCacheTracker = &kiroCacheTracker{entries: make(map[uint64]map[[32]byte]kiroCacheEntry)}
}

func kiroPNGDataURL(t *testing.T, width, height int, fill color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func kiroCacheImageRequestBody(t *testing.T, text string, fill color.RGBA) []byte {
	t.Helper()
	dataURL := kiroPNGDataURL(t, 200, 200, fill)
	return []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":%q},{"type":"image","source":{"type":"base64","media_type":"image/png","data":%q},"cache_control":{"type":"ephemeral"}}]}]}`, text, strings.TrimPrefix(dataURL, "data:image/png;base64,")))
}

// kiroMinimumCacheableTokens 决定「一个前缀至少要多少 token 才值得记进缓存」，直接
// 影响模拟出的 cache_creation / cache_read 分布，进而影响账单。这里把每个 Kiro 实际
// 暴露的模型的阈值钉死：GPT-5.6 三兄弟必须是 1024（对齐 OpenAI 官方最小缓存粒度），
// opus 系必须是 4096。特例断言写字面量、默认档断言写常量名，这样任何一侧被单独改动
// 都会让测试失败，而不是静默漂移。
func TestKiroMinimumCacheableTokens(t *testing.T) {
	t.Parallel()

	// GPT-5.6 是本用例的核心诉求：1024 是显式契约，不是「恰好等于默认档」。
	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		require.Equal(t, 1024, kiroMinimumCacheableTokens(model), model)
	}

	// opus 系走 4096，含 -thinking 变体与带日期后缀的 4.5。
	for _, model := range []string{
		"claude-opus-5", "claude-opus-5-thinking",
		"claude-opus-4-8", "claude-opus-4-8-thinking",
		"claude-opus-4-5-20251101", "claude-opus-4-5-20251101-thinking",
	} {
		require.Equal(t, 4096, kiroMinimumCacheableTokens(model), model)
	}

	// 非 opus 的 Claude 走默认档。断言常量而非字面量：默认档本身允许调整，
	// 但调整时必须同步 GPT 的显式 case（GPT 那侧断言的是字面量 1024）。
	for _, model := range []string{
		"claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-4-5-20251001",
	} {
		require.Equal(t, kiroCacheMinTokensDefault, kiroMinimumCacheableTokens(model), model)
	}

	// 遍历 Kiro 实际暴露的全量模型，确保没有未归类的漏网之鱼。上游新增模型时，
	// 这里会立刻因缺少 expected 条目而失败，迫使新模型被显式归类。
	expected := map[string]int{
		"gpt-5.6-sol":                         1024,
		"gpt-5.6-terra":                       1024,
		"gpt-5.6-luna":                        1024,
		"claude-opus-4-8":                     4096,
		"claude-opus-4-8-thinking":            4096,
		"claude-opus-4-7":                     4096,
		"claude-opus-4-7-thinking":            4096,
		"claude-opus-4-6":                     4096,
		"claude-opus-4-6-thinking":            4096,
		"claude-opus-5":                       4096,
		"claude-opus-5-thinking":              4096,
		"claude-opus-4-5-20251101":            4096,
		"claude-opus-4-5-20251101-thinking":   4096,
		"claude-sonnet-5":                     1024,
		"claude-sonnet-5-thinking":            1024,
		"claude-sonnet-4-6":                   1024,
		"claude-sonnet-4-6-thinking":          1024,
		"claude-sonnet-4-5-20250929":          1024,
		"claude-sonnet-4-5-20250929-thinking": 1024,
		"claude-haiku-4-5-20251001":           1024,
		"claude-haiku-4-5-20251001-thinking":  1024,
	}
	for _, model := range kiropkg.DefaultModels {
		want, ok := expected[model.ID]
		require.Truef(t, ok, "model %q 未在本测试中归类，请为其显式指定最小可缓存 token 数", model.ID)
		require.Equal(t, want, kiroMinimumCacheableTokens(model.ID), model.ID)
	}
}

func kiroCacheGroup(ratio float64) *Group {
	return &Group{ID: 12, Platform: PlatformKiro, KiroCacheEmulationEnabled: true, KiroCacheEmulationRatio: ratio}
}

func kiroCacheAccount(id int64, refreshToken string, accessToken string) *Account {
	return &Account{ID: id, Platform: PlatformKiro, Type: AccountTypeOAuth, Credentials: map[string]any{
		"client_id":     "client-id",
		"refresh_token": refreshToken,
		"access_token":  accessToken,
	}}
}

func kiroCacheRequestBody(label string, oneHour bool) []byte {
	ttl := ""
	if oneHour {
		ttl = `,"ttl":"1h"`
	}
	return []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":%q,"cache_control":{"type":"ephemeral"%s}}]}]}`, strings.Repeat("cacheable prompt chunk "+label+" ", 512), ttl))
}

func kiroCacheMultiMessageBody(prefixLabel, tailLabel string) []byte {
	prefix := strings.Repeat("cacheable prompt chunk "+prefixLabel+" ", 512)
	tail := strings.Repeat("conversation growth chunk "+tailLabel+" ", 160)
	return []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":%q,"cache_control":{"type":"ephemeral"}}]},{"role":"user","content":[{"type":"text","text":%q}]}]}`, prefix, tail))
}

func kiroChatCompletionsConversationBody(messages []string) []byte {
	items := make([]string, 0, len(messages)+1)
	items = append(items, `{"role":"system","content":"You are a precise assistant."}`)
	for _, message := range messages {
		items = append(items, fmt.Sprintf(`{"role":"user","content":%q}`, message))
	}
	return []byte(fmt.Sprintf(`{"model":"gpt-5","tool_choice":"auto","tools":[{"type":"function","function":{"name":"lookup","description":"lookup data","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}}],"messages":[%s]}`, strings.Join(items, ",")))
}

func kiroResponsesCacheRequestBody(label, promptCacheKey, previousResponseID string) []byte {
	return kiroResponsesCacheRequestBodyWithOptions(label, promptCacheKey, previousResponseID, "gpt-5", "auto", "medium", `{"type":"json_object"}`, "lookup")
}

func kiroResponsesCacheRequestBodyWithOptions(label, promptCacheKey, previousResponseID, model, toolChoice, effort, textFormat, toolName string) []byte {
	prompt := strings.Repeat("cacheable responses prompt chunk "+label+" ", 512)
	return []byte(fmt.Sprintf(`{"model":%q,"instructions":"You are a precise assistant.","prompt_cache_key":%q,"previous_response_id":%q,"tool_choice":%q,"reasoning":{"effort":%q},"text":{"format":%s},"tools":[{"type":"function","name":%q,"description":"lookup data","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}],"input":[{"role":"user","content":[{"type":"input_text","text":%q}]}]}`, model, promptCacheKey, previousResponseID, toolChoice, effort, textFormat, toolName, prompt))
}

func kiroResponsesConversationRequestBody(promptCacheKey string, messages []string) []byte {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		items = append(items, fmt.Sprintf(`{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}`, message))
	}
	return []byte(fmt.Sprintf(`{"model":"gpt-5","instructions":"You are a precise assistant.","prompt_cache_key":%q,"tool_choice":"auto","tools":[{"type":"function","name":"lookup","description":"lookup data","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}],"input":[%s]}`, promptCacheKey, strings.Join(items, ",")))
}

func kiroResponsesImageCacheRequestBody(t *testing.T, label string, fill color.RGBA) []byte {
	prompt := strings.Repeat("cacheable responses visual prompt "+label+" ", 512)
	imageURL := kiroPNGDataURL(t, 384, 256, fill)
	return []byte(fmt.Sprintf(`{"model":"gpt-5","instructions":"Describe visual changes precisely.","prompt_cache_key":"workspace-image","previous_response_id":"resp-image","input":[{"role":"user","content":[{"type":"input_text","text":%q},{"type":"input_image","image_url":%q}]}]}`, prompt, imageURL))
}

// 客户端完全不下发 cache_control 时（curl、Cherry Studio 等 OpenAI 风格客户端），
// Anthropic 路径必须兜底补断点。否则该请求零 cache_read 却仍计入命中率分母。
func TestKiroCacheEmulationFallsBackWhenClientSendsNoCacheControl(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(301, "refresh-nocc", "access-nocc")
	group := kiroCacheGroup(1)
	// 注意：整个 body 不含任何 cache_control。
	body := []byte(fmt.Sprintf(
		`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":%q}]}]}`,
		strings.Repeat("no cache control marker anywhere in this body ", 512)))

	first := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, first, "body without cache_control must still produce usage")
	require.Equal(t, 2000, first.CacheCreationInputTokens)

	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 2000)
	require.NotNil(t, second)
	require.Equal(t, 2000, second.CacheReadInputTokens, "repeat request must hit the fallback breakpoint")
}

// 客户端显式的 ttl:"1h" 不得被兜底逻辑降级成默认 5m。
func TestKiroCacheEmulationFallbackDoesNotDowngradeClientTTL(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	usage := svc.buildKiroCacheEmulationUsage(context.Background(),
		kiroCacheAccount(302, "refresh-ttl", "access-ttl"), kiroCacheGroup(1),
		kiroCacheRequestBody("fallback ttl", true), "claude-sonnet-4-6", 2000)
	require.NotNil(t, usage)
	require.Equal(t, 2000, usage.CacheCreation1hInputTokens, "client 1h TTL must survive")
	require.Zero(t, usage.CacheCreation5mInputTokens)
}

// 带真实体积 tools 的 opus 短请求不得被 minCacheable 误拒。
// 工具块若按 kiroTokensPerTool=150 拍平，累计值会卡在 opus 阈值 4096 之下导致 profile 被拒。
func TestKiroCacheEmulationOpusShortRequestWithToolsIsCacheable(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(303, "refresh-opus", "access-opus")
	group := kiroCacheGroup(1)
	body := kiroToolHeavyShortRequestBody()

	inputTokens := estimateKiroInputTokens(context.Background(), body)
	first := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-opus-4-8", inputTokens)
	require.NotNil(t, first, "opus short request with real-sized tools must be cacheable")
	require.Positive(t, first.CacheCreationInputTokens)

	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-opus-4-8", inputTokens)
	require.NotNil(t, second)
	require.Positive(t, second.CacheReadInputTokens, "repeat opus request must hit")
}

// commit() → update() 会遍历当前 profile 的全部可缓存断点并推后 expiresAt，
// 因此命中后无需在 compute() 里额外遍历前缀链。这条用例把该不变量钉住。
func TestKiroCacheEmulationCommitRenewsAllBreakpoints(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(304, "refresh-chain", "access-chain")
	group := kiroCacheGroup(1)
	body := kiroCacheMultiMessageBody("chain prefix", "chain tail")

	require.NotNil(t, svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 6000))

	cacheKey := kiroCacheCredentialKey(account)
	nearExpiry := time.Now().Add(2 * time.Second)
	globalKiroCacheTracker.mu.Lock()
	entries := globalKiroCacheTracker.entries[cacheKey]
	require.GreaterOrEqual(t, len(entries), 2, "need multiple breakpoints to exercise renewal")
	for fp, entry := range entries {
		entry.expiresAt = nearExpiry
		entries[fp] = entry
	}
	globalKiroCacheTracker.mu.Unlock()

	second := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", 6000)
	require.NotNil(t, second)
	require.Positive(t, second.CacheReadInputTokens)

	globalKiroCacheTracker.mu.Lock()
	defer globalKiroCacheTracker.mu.Unlock()
	renewedThreshold := nearExpiry.Add(time.Minute)
	for fp, entry := range globalKiroCacheTracker.entries[cacheKey] {
		require.True(t, entry.expiresAt.After(renewedThreshold),
			"entry %x was not renewed: expiresAt=%s", fp[:4], entry.expiresAt)
	}
}

// kiroToolHeavyShortRequestBody 构造「工具多且 schema 大、但对话很短」的请求，
// 即 Claude Code 会话刚开始时的形态。
func kiroToolHeavyShortRequestBody() []byte {
	desc := strings.Repeat("Detailed behavioural contract for this tool. ", 40)
	tools := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		tools = append(tools, fmt.Sprintf(
			`{"name":"tool_%d","description":%q,"input_schema":{"type":"object","properties":{"path":{"type":"string","description":%q}},"required":["path"]}}`,
			i, desc, desc))
	}
	return []byte(fmt.Sprintf(
		`{"model":"claude-opus-4-8","system":[{"type":"text","text":%q}],"tools":[%s],"messages":[{"role":"user","content":[{"type":"text","text":%q,"cache_control":{"type":"ephemeral"}}]}]}`,
		strings.Repeat("short system. ", 8), strings.Join(tools, ","),
		strings.Repeat("brief question. ", 10)))
}

// kiroClaudeCodeConversationBody 构造贴近真实 Claude Code 长会话的 Anthropic 请求体。
//
// 形态要点（决定这个用例是否具备判别力，两个约束互相拉扯，必须同时满足）：
//   - 轮数要足够多，让 messages 在总量中占主导。若 system 相对 messages 过大，逐块计数与
//     整体 JSON 计数之间的口径差异会被稀释，用例就测不出问题了。
//   - 但 system 也不能过小。单轮命中率上限 = (base+g)/(base+2g)，base 为 system+tools、
//     g 为每轮增量；base/g < 8 时前几轮天然到不了 90%。这是 prompt caching 的冷启动爬坡，
//     真实 Anthropic 缓存同样如此，不是缺陷。真实 Claude Code 的 system + CLAUDE.md + tools
//     通常 10-25k token，每轮增量 200-2000，base/g 约 10-50:1；这里取 ~3k token 的 system
//     使 base/g ≈ 10，兼顾上述两个约束。
//   - 每轮 assistant 带 text + 2 个 tool_use，user 回 2 个 tool_result，即大量小块。
//     tool_use 只有 input 被计数、tool_result 只有 content 被计数，而 type/id/name 等
//     结构字段一律计 0——块越多越碎，缺口越大，正是线上流量的形态。
//   - cache_control 只打在 system 末块，模拟客户端最保守的断点策略；后续每个消息末尾的
//     断点由 buildKiroCacheProfileFromBlocks 的传播逻辑补齐。
func kiroClaudeCodeConversationBody(turns int) []byte {
	system := strings.Repeat("You are a coding agent operating in a real repository. ", 220)
	tools := `[{"name":"read_file","description":"Read a file from disk","input_schema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}},{"name":"edit_file","description":"Apply an edit","input_schema":{"type":"object","properties":{"path":{"type":"string"},"patch":{"type":"string"}},"required":["path","patch"]}},{"name":"grep","description":"Search contents","input_schema":{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}}]`

	messages := make([]string, 0, turns*2)
	messages = append(messages, fmt.Sprintf(`{"role":"user","content":[{"type":"text","text":%q}]}`,
		strings.Repeat("Please refactor the billing module carefully. ", 12)))
	for turn := 1; turn <= turns; turn++ {
		messages = append(messages, fmt.Sprintf(`{"role":"assistant","content":[{"type":"text","text":%q},{"type":"tool_use","id":"toolu_%d_a","name":"read_file","input":{"path":"internal/service/billing_%d.go"}},{"type":"tool_use","id":"toolu_%d_b","name":"grep","input":{"pattern":"CalculateCost_%d"}}]}`,
			strings.Repeat("Inspecting the next call site. ", 6), turn, turn, turn, turn))
		messages = append(messages, fmt.Sprintf(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_%d_a","content":[{"type":"text","text":%q}]},{"type":"tool_result","tool_use_id":"toolu_%d_b","content":[{"type":"text","text":%q}]}]}`,
			turn, strings.Repeat("func CalculateCost(tokens UsageTokens) float64 { return 0 } ", 5),
			turn, strings.Repeat("billing_service.go:120: CalculateCost invoked here ", 5)))
	}

	return []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","system":[{"type":"text","text":%q,"cache_control":{"type":"ephemeral"}}],"tools":%s,"messages":[%s]}`,
		system, tools, strings.Join(messages, ",")))
}

// Kiro 分组模拟缓存在多轮追加式会话下的稳态命中率必须 >= 90%。
//
// 命中率口径与前端面板一致（TokenUsageTrend.vue）：
//
//	cache_read / (input + cache_read + cache_creation)
//
// 而 prepareKiroCacheEmulationPlanFromProfile 令三者之和恒等于 inputTokens，故等价于
// cache_read / inputTokens。这要求断点累计值与 inputTokens 处于同一 token 空间，即
// profile.scaleBreakpointsToInputTokens 必须为 true——该标志缺失时，分子只统计正文、
// 分母含整体 JSON 结构开销，命中率会被系统性压到 90% 以下。
func TestKiroCacheEmulationSteadyStateHitRateMeetsTarget(t *testing.T) {
	resetKiroCacheTracker()
	svc := &GatewayService{}
	account := kiroCacheAccount(4201, "refresh-steady", "access-steady")
	group := kiroCacheGroup(1)

	const totalTurns = 30
	const minHitRate = 0.90

	var aggregateRead, aggregateTotal int
	for turn := 1; turn <= totalTurns; turn++ {
		body := kiroClaudeCodeConversationBody(turn)
		inputTokens := estimateKiroInputTokens(context.Background(), body)
		usage := svc.buildKiroCacheEmulationUsage(context.Background(), account, group, body, "claude-sonnet-4-6", inputTokens)
		require.NotNil(t, usage, "turn %d: cache emulation must produce usage", turn)

		total := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
		require.Equal(t, inputTokens, total, "turn %d: token buckets must sum to inputTokens", turn)

		aggregateRead += usage.CacheReadInputTokens
		aggregateTotal += total

		hitRate := float64(usage.CacheReadInputTokens) / float64(total)
		t.Logf("turn %2d: input=%6d read=%6d creation=%6d hit_rate=%.4f",
			turn, usage.InputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens, hitRate)

		if turn == 1 {
			// 首轮无历史前缀，必然全量创建。
			require.Zero(t, usage.CacheReadInputTokens, "turn 1 must be a cold miss")
			continue
		}
		require.GreaterOrEqual(t, hitRate, minHitRate,
			"turn %d: hit rate %.4f below target %.2f", turn, hitRate, minHitRate)
	}

	// 面板命中率是 token 加权聚合（见 TokenUsageTrend.vue），含首轮冷启动在内。
	aggregateHitRate := float64(aggregateRead) / float64(aggregateTotal)
	t.Logf("aggregate over %d turns: read=%d total=%d hit_rate=%.4f",
		totalTurns, aggregateRead, aggregateTotal, aggregateHitRate)
	require.GreaterOrEqual(t, aggregateHitRate, minHitRate,
		"aggregate hit rate %.4f below target %.2f", aggregateHitRate, minHitRate)
}
