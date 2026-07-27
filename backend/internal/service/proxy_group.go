package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	ProxyGroupStrategyRoundRobin = "round_robin"
	ProxyGroupStrategyRandom     = "random"
	ProxyGroupStrategySticky     = "sticky"
)

var (
	ErrProxyGroupNotFound = infraerrors.NotFound("PROXY_GROUP_NOT_FOUND", "proxy group not found")
	ErrProxyGroupInUse    = infraerrors.Conflict("PROXY_GROUP_IN_USE", "proxy group is in use by proxies or accounts")
)

// ProxyGroup 代理池/分组领域模型。
type ProxyGroup struct {
	ID              int64
	Name            string
	Description     string
	Strategy        string
	StickyByAccount bool
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (g *ProxyGroup) IsActive() bool {
	return g != nil && g.Status == StatusActive
}

// EffectiveStrategy 返回实际选择策略。
// sticky_by_account 开启时强制 sticky；未知 strategy 回退 round_robin。
func (g *ProxyGroup) EffectiveStrategy() string {
	if g == nil {
		return ProxyGroupStrategyRoundRobin
	}
	if g.StickyByAccount {
		return ProxyGroupStrategySticky
	}
	switch g.Strategy {
	case ProxyGroupStrategyRoundRobin, ProxyGroupStrategyRandom, ProxyGroupStrategySticky:
		return g.Strategy
	default:
		return ProxyGroupStrategyRoundRobin
	}
}

// ProxyGroupWithProxies 组 + 成员列表（管理端展示用）。
type ProxyGroupWithProxies struct {
	ProxyGroup
	Proxies    []Proxy
	ProxyCount int64
}

// ProxyGroupRepository 定义在 service 包（依赖倒置），由 repository 包实现。
type ProxyGroupRepository interface {
	Create(ctx context.Context, group *ProxyGroup) error
	GetByID(ctx context.Context, id int64) (*ProxyGroup, error)
	Update(ctx context.Context, group *ProxyGroup) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams) ([]ProxyGroup, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]ProxyGroup, error)
	CountProxiesByGroupID(ctx context.Context, groupID int64) (int64, error)
	CountAccountsByGroupID(ctx context.Context, groupID int64) (int64, error)
	// SetGroupMembers 用 proxyIDs 全量替换组成员（空切片 = 清空）。
	SetGroupMembers(ctx context.Context, groupID int64, proxyIDs []int64) error
}

// ProxyGroupResolver 在账号 hydration 时按组选出一个代理。
// 实现 MUST NOT 写回 account.ProxyID。
type ProxyGroupResolver interface {
	// ResolveProxy 返回组内选出的代理；无健康成员时返回 (nil, nil)。
	ResolveProxy(ctx context.Context, groupID, accountID int64) (*Proxy, error)
	// InvalidateGroup 使组成员缓存失效。
	InvalidateGroup(groupID int64)
}
