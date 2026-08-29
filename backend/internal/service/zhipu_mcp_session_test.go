//go:build unit

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// zhipuMCPStickyRepoStub 只覆盖 LoadZhipuMCPStickyAccount 需要的 GetByID。
type zhipuMCPStickyRepoStub struct {
	service.AccountRepository

	accountsByID map[int64]*service.Account
}

func (s *zhipuMCPStickyRepoStub) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if acc, ok := s.accountsByID[id]; ok {
		return acc, nil
	}
	return nil, errors.New("account not found")
}

func zhipuMCPTestService(repo service.AccountRepository, store service.ZhipuMCPSessionStore) *service.GatewayService {
	svc := service.NewGatewayService(
		repo,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return svc.WithZhipuMCPSessionStore(store)
}

func zhipuMCPCapableAccount(id int64) *service.Account {
	return &service.Account{
		ID:          id,
		Platform:    service.PlatformZhipu,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 5,
		Credentials: map[string]any{"api_key": "zp-key-" + strings.Repeat("x", 4), "account_mode": service.AccountModeCoding},
		Extra:       map[string]any{service.ZhipuMCPCapabilityExtraKey: true},
	}
}

func TestZhipuMCPSessionBinding_TTLAndLifecycle(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := zhipuMCPTestService(nil, repository.NewZhipuMCPCache(rdb))
	ctx := context.Background()

	// 绑定后可查回，且条目带 TTL（miniredis TTL > 0）。
	require.NoError(t, svc.BindZhipuMCPSession(ctx, "sess-abc-123", 42))
	accountID, ok := svc.LookupZhipuMCPSession(ctx, "sess-abc-123")
	require.True(t, ok)
	require.Equal(t, int64(42), accountID)

	key := "zhipu_mcp:session:sess-abc-123"
	ttl := mr.TTL(key)
	require.Greater(t, ttl, time.Duration(0), "粘表条目必须带 TTL")
	require.LessOrEqual(t, ttl, service.ZhipuMCPSessionTTL)

	// TTL 过期后视为未绑定。
	mr.FastForward(service.ZhipuMCPSessionTTL + time.Minute)
	_, ok = svc.LookupZhipuMCPSession(ctx, "sess-abc-123")
	require.False(t, ok)

	// 重新绑定后 DELETE 清理。
	require.NoError(t, svc.BindZhipuMCPSession(ctx, "sess-abc-123", 42))
	require.NoError(t, svc.UnbindZhipuMCPSession(ctx, "sess-abc-123"))
	require.False(t, mr.Exists(key), "DELETE 后 Redis key 应被清理")
	_, ok = svc.LookupZhipuMCPSession(ctx, "sess-abc-123")
	require.False(t, ok)
}

func TestZhipuMCPSessionBinding_TombstoneAfterDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := zhipuMCPTestService(nil, repository.NewZhipuMCPCache(rdb))
	ctx := context.Background()

	// 正常绑定 → DELETE 终止语义 → tombstone（值 0，key 保留并带 TTL）。
	require.NoError(t, svc.BindZhipuMCPSession(ctx, "sess-tomb-1", 42))
	require.NoError(t, svc.MarkZhipuMCPSessionDeleted(ctx, "sess-tomb-1"))

	accountID, ok := svc.LookupZhipuMCPSession(ctx, "sess-tomb-1")
	require.True(t, ok, "tombstone 必须命中（区别于未绑定）")
	require.Equal(t, service.ZhipuMCPSessionDeletedAccountID, accountID)
	ttl := mr.TTL("zhipu_mcp:session:sess-tomb-1")
	require.Greater(t, ttl, time.Duration(0), "tombstone 条目必须带 TTL")

	// 重新 initialize 用同一 SID 覆盖 tombstone，恢复正常绑定。
	require.NoError(t, svc.BindZhipuMCPSession(ctx, "sess-tomb-1", 43))
	accountID, ok = svc.LookupZhipuMCPSession(ctx, "sess-tomb-1")
	require.True(t, ok)
	require.Equal(t, int64(43), accountID)

	// tombstone 随 TTL 过期后回到未绑定状态（客户端早已重新 initialize）。
	require.NoError(t, svc.MarkZhipuMCPSessionDeleted(ctx, "sess-tomb-1"))
	mr.FastForward(service.ZhipuMCPSessionTTL + time.Minute)
	_, ok = svc.LookupZhipuMCPSession(ctx, "sess-tomb-1")
	require.False(t, ok)
}

func TestZhipuMCPSessionBinding_SanitizeRejectsUnsafeSessionID(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := zhipuMCPTestService(nil, repository.NewZhipuMCPCache(rdb))
	ctx := context.Background()

	// key 注入与日志注入向量全部拒绝，不落 Redis。
	unsafeIDs := []string{
		"",
		"   ",
		"a b",
		"sess\n:1",               // 控制字符
		"sess\r\tx",              // 控制字符
		"中文-session",             // 非 ASCII
		strings.Repeat("s", 201), // 超长
	}
	for _, sessionID := range unsafeIDs {
		require.Error(t, svc.BindZhipuMCPSession(ctx, sessionID, 1), "sessionID=%q", sessionID)
		_, ok := svc.LookupZhipuMCPSession(ctx, sessionID)
		require.False(t, ok, "sessionID=%q", sessionID)
		require.NoError(t, svc.UnbindZhipuMCPSession(ctx, sessionID))
	}
	require.Equal(t, 0, len(mr.Keys()), "非法 sessionID 不应产生任何 Redis key")

	// 边界内值可用：可见 ASCII、长度 1..200、允许 trim 前后空白。
	require.NoError(t, svc.BindZhipuMCPSession(ctx, strings.Repeat("a", 200), 7))
	_, ok := svc.LookupZhipuMCPSession(ctx, strings.Repeat("a", 200))
	require.True(t, ok)

	// accountID 非法时不绑定。
	require.NoError(t, svc.BindZhipuMCPSession(ctx, "sess-zero", 0))
	_, ok = svc.LookupZhipuMCPSession(ctx, "sess-zero")
	require.False(t, ok)
}

func TestZhipuMCPSessionBinding_NilStoreDegradesGracefully(t *testing.T) {
	// 未接线（单测/降级环境）时粘表方法按 best-effort 降级，不允许 panic。
	svc := zhipuMCPTestService(nil, nil)
	ctx := context.Background()

	require.NoError(t, svc.BindZhipuMCPSession(ctx, "sess-1", 1))
	_, ok := svc.LookupZhipuMCPSession(ctx, "sess-1")
	require.False(t, ok)
	require.NoError(t, svc.UnbindZhipuMCPSession(ctx, "sess-1"))
}

func TestResolveZhipuMCPServerURL(t *testing.T) {
	url, ok := service.ResolveZhipuMCPServerURL("web_search_prime")
	require.True(t, ok)
	require.Equal(t, "https://open.bigmodel.cn/api/mcp/web_search_prime/mcp", url)

	url, ok = service.ResolveZhipuMCPServerURL("zread")
	require.True(t, ok)
	require.Equal(t, "https://open.bigmodel.cn/api/mcp/zread/mcp", url)

	url, ok = service.ResolveZhipuMCPServerURL("web_reader")
	require.True(t, ok)
	require.Equal(t, "https://open.bigmodel.cn/api/mcp/web_reader/mcp", url)

	// 表外 slug 一律拒绝，由调用方映射 404。vision 是 Local MCP（无远程端点），天然表外。
	for _, slug := range []string{"", "  ", "vision", "../etc", "WEB_SEARCH_PRIME"} {
		_, ok = service.ResolveZhipuMCPServerURL(slug)
		require.False(t, ok, "slug=%q 不应在白名单内", slug)
	}
}

func TestLoadZhipuMCPStickyAccount_Validation(t *testing.T) {
	valid := zhipuMCPCapableAccount(11)

	notCapable := zhipuMCPCapableAccount(12)
	notCapable.Extra = map[string]any{service.ZhipuMCPCapabilityExtraKey: false}

	notZhipu := zhipuMCPCapableAccount(13)
	notZhipu.Platform = service.PlatformOpenAI

	inactive := zhipuMCPCapableAccount(14)
	inactive.Status = "error"

	repo := &zhipuMCPStickyRepoStub{accountsByID: map[int64]*service.Account{
		11: valid,
		12: notCapable,
		13: notZhipu,
		14: inactive,
	}}
	svc := zhipuMCPTestService(repo, nil)
	ctx := context.Background()

	account, err := svc.LoadZhipuMCPStickyAccount(ctx, 11)
	require.NoError(t, err)
	require.Equal(t, int64(11), account.ID)

	// 校验不通过（能力关闭 / 平台不符 / 停用 / 不存在）返回错误，调用方据此清粘表走正常调度。
	for _, id := range []int64{12, 13, 14, 99} {
		_, err = svc.LoadZhipuMCPStickyAccount(ctx, id)
		require.Error(t, err, "account %d 不应通过粘性校验", id)
	}

	_, err = svc.LoadZhipuMCPStickyAccount(ctx, 0)
	require.Error(t, err)
}
