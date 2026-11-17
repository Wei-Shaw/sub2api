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
	accountID int64
	platform  string
}

// fakeAnthropicFileOwnerCache 只实现归属读写，其余 GatewayCache 方法由嵌入的
// nil 接口占位——本文件不会调用它们。
type fakeAnthropicFileOwnerCache struct {
	GatewayCache
	owners  map[string]fakeAnthropicFileOwnerEntry
	ttls    map[string]time.Duration
	setErr  error
	getErr  error
	setCall int
	onSet   func()
}

func (c *fakeAnthropicFileOwnerCache) SetAnthropicFileOwner(_ context.Context, fileID string, accountID int64, platform string, ttl time.Duration) error {
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
	c.owners[fileID] = fakeAnthropicFileOwnerEntry{accountID: accountID, platform: platform}
	c.ttls[fileID] = ttl
	return nil
}

func (c *fakeAnthropicFileOwnerCache) GetAnthropicFileOwner(_ context.Context, fileID string) (int64, string, bool, error) {
	if c.getErr != nil {
		return 0, "", false, c.getErr
	}
	owner, ok := c.owners[fileID]
	if !ok {
		return 0, "", false, nil
	}
	return owner.accountID, owner.platform, true, nil
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

	require.NoError(t, svc.BindAnthropicFileOwner(ctx, &Account{ID: 901, Platform: PlatformDeepseek}, "file_a"))
	require.Equal(t, anthropicFileOwnerTTL, cache.ttls["file_a"], "TTL must be the fixed 7-day window")
	require.Equal(t, 7*24*time.Hour, anthropicFileOwnerTTL)

	// 绑定失败属于可观测错误，调用方只记日志。
	require.Error(t, svc.BindAnthropicFileOwner(ctx, nil, "file_a"))
	require.Error(t, svc.BindAnthropicFileOwner(ctx, &Account{ID: 0, Platform: PlatformDeepseek}, "file_a"))
	require.Error(t, svc.BindAnthropicFileOwner(ctx, &Account{ID: 901, Platform: PlatformDeepseek}, "  "))
	require.Error(t, (&OpenAIGatewayService{}).BindAnthropicFileOwner(ctx, &Account{ID: 901, Platform: PlatformDeepseek}, "file_a"))

	imageBody := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"file","file_id":"file_a"}}]}]}`)
	ownerID, platform, ok := svc.ResolveAnthropicFileOwner(ctx, imageBody)
	require.True(t, ok)
	require.Equal(t, int64(901), ownerID)
	require.Equal(t, PlatformDeepseek, platform)

	// 混合命中/未命中：命中的只有一个账号，仍算命中。
	mixedBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"document","source":{"type":"file","file_id":"file_missing"}}
	]}]}`)
	ownerID, platform, ok = svc.ResolveAnthropicFileOwner(ctx, mixedBody)
	require.True(t, ok)
	require.Equal(t, int64(901), ownerID)
	require.Equal(t, PlatformDeepseek, platform)

	// 全部未命中：退回原选号路径。
	ownerID, platform, ok = svc.ResolveAnthropicFileOwner(ctx, []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_missing"}}
	]}]}`))
	require.False(t, ok)
	require.Zero(t, ownerID)
	require.Empty(t, platform)

	// 多个 file 指向不同账号：无法同时满足，放弃硬钉。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, &Account{ID: 902, Platform: PlatformDeepseek}, "file_b"))
	inconsistentBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_b"}}
	]}]}`)
	ownerID, platform, ok = svc.ResolveAnthropicFileOwner(ctx, inconsistentBody)
	require.False(t, ok)
	require.Zero(t, ownerID)
	require.Empty(t, platform)

	// 同账号同平台重复出现仍然命中。
	require.NoError(t, svc.BindAnthropicFileOwner(ctx, &Account{ID: 901, Platform: PlatformDeepseek}, "file_c"))
	sameOwnerBody := []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"file","file_id":"file_a"}},
		{"type":"image","source":{"type":"file","file_id":"file_c"}}
	]}]}`)
	ownerID, platform, ok = svc.ResolveAnthropicFileOwner(ctx, sameOwnerBody)
	require.True(t, ok)
	require.Equal(t, int64(901), ownerID)
	require.Equal(t, PlatformDeepseek, platform)

	// 无 file 源（文本 / base64）不改变行为。
	_, _, ok = svc.ResolveAnthropicFileOwner(ctx, []byte(`{"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"x"}},
		{"type":"text","text":"hi"}
	]}]}`))
	require.False(t, ok)

	// 缓存读取失败按未命中处理（fail-open 到原选号），不 panic、不硬钉。
	broken := &OpenAIGatewayService{cache: &fakeAnthropicFileOwnerCache{getErr: errors.New("redis down")}}
	_, _, ok = broken.ResolveAnthropicFileOwner(ctx, imageBody)
	require.False(t, ok)
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
