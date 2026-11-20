package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type fakeAnthropicFileOwnerEntry struct {
	ownerUserID int64
	accountID   int64
	platform    string
}

// fakeAnthropicFileOwnerCache 在调度器测试用缓存的基础上只覆写归属读写，其余
// GatewayCache 方法直接复用 schedulerTestGatewayCache 的实现（普通选号会读取
// 会话绑定，nil 接口会 panic）。
type fakeAnthropicFileOwnerCache struct {
	schedulerTestGatewayCache
	owners  map[string]fakeAnthropicFileOwnerEntry
	ttls    map[string]time.Duration
	setErr  error
	getErr  error
	setCall int
	onSet   func()
}

func (c *fakeAnthropicFileOwnerCache) SetAnthropicFileOwner(_ context.Context, fileID string, ownerUserID int64, accountID int64, platform string, ttl time.Duration) error {
	c.setCall++
	if c.onSet != nil {
		c.onSet()
	}
	if c.setErr != nil {
		return c.setErr
	}
	if c.owners == nil {
		c.owners = make(map[string]fakeAnthropicFileOwnerEntry)
	}
	if c.ttls == nil {
		c.ttls = make(map[string]time.Duration)
	}
	c.owners[fileID] = fakeAnthropicFileOwnerEntry{ownerUserID: ownerUserID, accountID: accountID, platform: platform}
	c.ttls[fileID] = ttl
	return nil
}

func (c *fakeAnthropicFileOwnerCache) GetAnthropicFileOwner(_ context.Context, fileID string) (int64, int64, string, bool, error) {
	if c.getErr != nil {
		return 0, 0, "", false, c.getErr
	}
	owner, ok := c.owners[fileID]
	if !ok {
		return 0, 0, "", false, nil
	}
	return owner.ownerUserID, owner.accountID, owner.platform, true, nil
}

func TestAnthropicFileIDs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "direct image file source",
			body: `{"messages":[{"role":"user","content":[
				{"type":"text","text":"look"},
				{"type":"image","source":{"type":"file","file_id":"file_a"}}
			]}]}`,
			want: []string{"file_a"},
		},
		{
			name: "tool_result nested image file source",
			body: `{"messages":[{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_1","content":[
					{"type":"image","source":{"type":"file","file_id":"file_b"}}
				]}
			]}]}`,
			want: []string{"file_b"},
		},
		{
			name: "document file source",
			body: `{"messages":[{"role":"user","content":[
				{"type":"document","source":{"type":"file","file_id":"file_c"}}
			]}]}`,
			want: []string{"file_c"},
		},
		{
			name: "base64 image source is ignored",
			body: `{"messages":[{"role":"user","content":[
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
			]}]}`,
			want: nil,
		},
		{
			name: "text only content",
			body: `{"messages":[{"role":"user","content":"plain text"}]}`,
			want: nil,
		},
		{
			name: "mixed blocks dedupe preserving order",
			body: `{"messages":[
				{"role":"user","content":[
					{"type":"image","source":{"type":"file","file_id":"file_b"}},
					{"type":"document","source":{"type":"file","file_id":"file_a"}}
				]},
				{"role":"assistant","content":[
					{"type":"tool_result","tool_use_id":"toolu_2","content":[
						{"type":"image","source":{"type":"file","file_id":"file_b"}},
						{"type":"image","source":{"type":"base64","data":"x"}}
					]}
				]}
			]}`,
			want: []string{"file_b", "file_a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, AnthropicFileIDs([]byte(tt.body)))
		})
	}
}

func TestBindAndResolveAnthropicFileOwner(t *testing.T) {
	ctx := context.Background()
	cache := &fakeAnthropicFileOwnerCache{}
	svc := &OpenAIGatewayService{cache: cache}

	require.NoError(t, svc.BindAnthropicFileOwner(ctx, 1, &Account{ID: 901, Platform: PlatformDeepseek}, "file_a"))
	require.Equal(t, anthropicFileOwnerTTL, cache.ttls["file_a"], "TTL must be the fixed 7-day window")
	require.Equal(t, 7*24*time.Hour, anthropicFileOwnerTTL)
	require.Equal(t, fakeAnthropicFileOwnerEntry{ownerUserID: 1, accountID: 901, platform: PlatformDeepseek}, cache.owners["file_a"])

	// 绑定失败属于可观测错误，调用方只记日志。
	require.Error(t, svc.BindAnthropicFileOwner(ctx, 1, nil, "file_a"))
	require.Error(t, svc.BindAnthropicFileOwner(ctx, 1, &Account{ID: 0, Platform: PlatformDeepseek}, "file_a"))
	require.Error(t, svc.BindAnthropicFileOwner(ctx, 1, &Account{ID: 901, Platform: PlatformDeepseek}, "  "))
	require.Error(t, (&OpenAIGatewayService{}).BindAnthropicFileOwner(ctx, 1, &Account{ID: 901, Platform: PlatformDeepseek}, "file_a"))

	// userID<=0 无法确定归属用户：不写；带该 file 的请求会判定 unknown。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, 0, &Account{ID: 903, Platform: PlatformDeepseek}, "file_zero_user"))
	require.NotContains(t, cache.owners, "file_zero_user")

	imageBody := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"file","file_id":"file_a"}}]}]}`)

	// 无 file 源 → none，保持原选号路径。
	textBody := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	_, _, decision, resolveErr := svc.ResolveAnthropicFileOwner(ctx, 1, textBody)
	require.Equal(t, AnthropicFileOwnerNone, decision)
	require.NoError(t, resolveErr)

	// 同用户命中同一账号+平台 → pinned。
	ownerID, platform, decision, resolveErr := svc.ResolveAnthropicFileOwner(ctx, 1, imageBody)
	require.Equal(t, AnthropicFileOwnerPinned, decision)
	require.NoError(t, resolveErr)
	require.Equal(t, int64(901), ownerID)
	require.Equal(t, PlatformDeepseek, platform)

	// 其他用户引用同一 file_id → foreign（handler 必须 403）。
	ownerID, platform, decision, resolveErr = svc.ResolveAnthropicFileOwner(ctx, 2, imageBody)
	require.Equal(t, AnthropicFileOwnerForeign, decision)
	require.NoError(t, resolveErr)
	require.Zero(t, ownerID)
	require.Empty(t, platform)

	// 无法识别请求用户（userID<=0）且有 file 源 → unknown，不得当成 none 放行。
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 0, imageBody)
	require.Equal(t, AnthropicFileOwnerUnknown, decision)

	// 自有已绑定 file + 未知 file 混合 → unknown：不得 pinned 后把未授权文件一起转发。
	ownPlusUnknownBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"document","source":{"type":"file","file_id":"file_missing"}}
	]}]}`)
	ownerID, platform, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 1, ownPlusUnknownBody)
	require.Equal(t, AnthropicFileOwnerUnknown, decision)
	require.Zero(t, ownerID)
	require.Empty(t, platform)

	// 全部未命中 → unknown（不是 none，必须要求重新上传）。
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 1, []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_missing"}}
	]}]}`))
	require.Equal(t, AnthropicFileOwnerUnknown, decision)

	// 同用户不同账号 → conflict。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, 1, &Account{ID: 902, Platform: PlatformDeepseek}, "file_b"))
	inconsistentBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_b"}}
	]}]}`)
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 1, inconsistentBody)
	require.Equal(t, AnthropicFileOwnerConflict, decision)

	// 自有 + 他人的 file 混合 → foreign（拒绝优先于 pinned）。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, 2, &Account{ID: 904, Platform: PlatformDeepseek}, "file_other"))
	foreignMixBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_other"}}
	]}]}`)
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 1, foreignMixBody)
	require.Equal(t, AnthropicFileOwnerForeign, decision)

	// 全部属于他人 → foreign。
	foreignOnlyBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_other"}}
	]}]}`)
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 3, foreignOnlyBody)
	require.Equal(t, AnthropicFileOwnerForeign, decision)

	// 自有 file 冲突 + 他人的 file 同时出现：拒绝优先于冲突，仍为 foreign。
	conflictForeignBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_b"}},
		{"type":"image","source":{"type":"file","file_id":"file_other"}}
	]}]}`)
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 1, conflictForeignBody)
	require.Equal(t, AnthropicFileOwnerForeign, decision)

	// 同账号同平台重复出现仍然 pinned。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, 1, &Account{ID: 901, Platform: PlatformDeepseek}, "file_c"))
	sameOwnerBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_c"}}
	]}]}`)
	ownerID, platform, decision, resolveErr = svc.ResolveAnthropicFileOwner(ctx, 1, sameOwnerBody)
	require.Equal(t, AnthropicFileOwnerPinned, decision)
	require.NoError(t, resolveErr)
	require.Equal(t, int64(901), ownerID)
	require.Equal(t, PlatformDeepseek, platform)

	// 无 file 源（base64 图片 / 文本）→ none。
	_, _, decision, _ = svc.ResolveAnthropicFileOwner(ctx, 1, []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"x"}},
		{"type":"text","text":"hi"}
	]}]}`))
	require.Equal(t, AnthropicFileOwnerNone, decision)

	// 缓存读取失败 → storeError（503，可重试），绝不当作 none/pinned。
	broken := &OpenAIGatewayService{cache: &fakeAnthropicFileOwnerCache{getErr: errors.New("redis down")}}
	_, _, decision, resolveErr = broken.ResolveAnthropicFileOwner(ctx, 1, imageBody)
	require.Equal(t, AnthropicFileOwnerStoreError, decision)
	require.Error(t, resolveErr)

	// 归属缓存不可用 → storeError，不得放行。
	_, _, decision, resolveErr = (&OpenAIGatewayService{}).ResolveAnthropicFileOwner(ctx, 1, imageBody)
	require.Equal(t, AnthropicFileOwnerStoreError, decision)
	require.Error(t, resolveErr)
}

// TestAnthropicFileOwnerPinsOwnerInCompositeGroup 复现评审场景：同一组合分组里
// 163 官方 DeepSeek（优先级 99）与 167 挂 ollama.com 的账号（优先级 1）。
// 注：仓库没有 "ollama_cloud" 平台常量——Ollama Cloud 账号就是 platform=deepseek
// 或 openai、base_url=https://ollama.com(/v1) 的普通账号（见 ollama 相关用例），
// 这里用同名账号 + base_url 模拟，调度行为与真实账号一致。
// 无 file 时普通选号按优先级选中 167（可能读到别人文件）；file 归属 163 时
// 必须硬钉回 163，而不是被普通选号落回 167。
func TestAnthropicFileOwnerPinsOwnerInCompositeGroup(t *testing.T) {
	ctx := context.Background()
	groupID := int64(163)
	official := Account{ID: 163, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 50, Priority: 99, GroupIDs: []int64{groupID}}
	ollama := official
	ollama.ID = 167
	ollama.Name = "ollama_cloud"
	ollama.Priority = 1
	ollama.Extra = map[string]any{"base_url": "https://ollama.com/v1"}

	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{official, ollama}},
		cache:       &fakeAnthropicFileOwnerCache{},
		cfg:         cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			acquireResults: map[int64]bool{163: true, 167: true},
		}),
	}

	// 无 file：普通选号按优先级选中 167（ollama.com），即本修复要避免的落点。
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "session-normal", "deepseek-flash", nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, PlatformDeepseek,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(167), selection.Account.ID, "plain scheduling must prefer the lower priority account")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	// file 归属 163：解析命中且硬钉回 163。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, 1, &official, "file-composite"))
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"file","file_id":"file-composite"}}]}]}`)
	ownerID, platform, fileOwnerDecision, resolveErr := svc.ResolveAnthropicFileOwner(ctx, 1, body)
	require.NoError(t, resolveErr)
	require.Equal(t, AnthropicFileOwnerPinned, fileOwnerDecision)
	selection, decision, err := svc.SelectAnthropicFileOwnerAccount(ctx, &groupID, ownerID, platform, "deepseek-flash")
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(163), selection.Account.ID, "file owner pin must override priority and select the uploading account")
	require.True(t, decision.StickySessionHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectAnthropicFileOwnerAccountPinsOwner(t *testing.T) {
	for _, state := range []string{"available", "unavailable", "missing", "wrong platform", "invalid id"} {
		t.Run(state, func(t *testing.T) {
			groupID := int64(24)
			ownerID := int64(1)
			owner := Account{ID: ownerID, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 50, GroupIDs: []int64{groupID}}
			other := owner
			other.ID = 2
			if state == "unavailable" {
				until := time.Now().Add(time.Minute)
				owner.TempUnschedulableUntil = &until
			}
			if state == "wrong platform" {
				owner.Platform = PlatformOpenAI
			}
			accounts := []Account{owner, other}
			if state == "missing" {
				accounts = []Account{other}
			}
			if state == "invalid id" {
				ownerID = 0
			}
			var acquired, released []int64
			cfg := &config.Config{}
			cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
			cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
			cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache:       &schedulerTestGatewayCache{},
				cfg:         cfg,
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					acquireResults: map[int64]bool{1: true, 2: true},
					acquiredIDs:    &acquired,
					releasedIDs:    &released,
				}),
			}
			ctx := context.Background()
			selection, decision, err := svc.SelectAnthropicFileOwnerAccount(ctx, &groupID, ownerID, PlatformDeepseek, "deepseek-flash")
			switch state {
			case "available":
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(1), selection.Account.ID)
				require.True(t, selection.Acquired)
				require.True(t, decision.StickySessionHit)
				selection.ReleaseFunc()
				require.Equal(t, []int64{1}, released)
			default:
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Nil(t, selection)
			}
			require.NotContains(t, acquired, int64(2), "file owner pin must never fall back to another account")
		})
	}
}

func TestAnthropicFileOwnerTTLIsSevenDays(t *testing.T) {
	require.Equal(t, 7*24*time.Hour, anthropicFileOwnerTTL)
}
