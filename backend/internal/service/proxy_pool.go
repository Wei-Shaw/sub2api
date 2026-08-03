package service

import (
	"context"
	"time"
)

// ProxyPool 代理池：一组代理的集合。
// 池服务周期探测池内代理健康度（PoolHealth/PoolFailures），
// 并在池内某代理不健康时把绑定账号自动改投到池内健康代理（auto_rebind）。
type ProxyPool struct {
	ID                    int64
	Name                  string
	Description           *string
	Status                string
	HealthIntervalSeconds int
	FailureThreshold      int
	AutoRebind            bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Pool 健康状态常量
const (
	PoolHealthUnknown   = "unknown"
	PoolHealthHealthy   = "healthy"
	PoolHealthUnhealthy = "unhealthy"
)

func (p *ProxyPool) IsActive() bool {
	return p != nil && p.Status == StatusActive
}

// HealthInterval 返回探测间隔，非正数时用默认值兜底。
func (p *ProxyPool) HealthInterval() time.Duration {
	if p == nil || p.HealthIntervalSeconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(p.HealthIntervalSeconds) * time.Second
}

// FailureThreshold 返回连续失败阈值，非正数时用默认值兜底。
func (p *ProxyPool) FailureThresholdValue() int {
	if p == nil || p.FailureThreshold <= 0 {
		return 2
	}
	return p.FailureThreshold
}

// ProxyPoolWithStats 池列表视图：附带池内代理统计。
type ProxyPoolWithStats struct {
	ProxyPool
	ProxyCount      int64 `json:"proxy_count"`
	HealthyCount    int64 `json:"healthy_count"`
	UnhealthyCount  int64 `json:"unhealthy_count"`
	UnknownCount    int64 `json:"unknown_count"`
	BoundAccountSum int64 `json:"bound_account_sum"`
}

// ProxyPoolRebindLog 池重绑操作日志。
type ProxyPoolRebindLog struct {
	ID           int64     `json:"id"`
	PoolID       int64     `json:"pool_id"`
	FromProxyID  *int64    `json:"from_proxy_id,omitempty"`
	ToProxyID    *int64    `json:"to_proxy_id,omitempty"`
	AccountCount int       `json:"account_count"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
	FromProxy    *Proxy    `json:"from_proxy,omitempty"`
	ToProxy      *Proxy    `json:"to_proxy,omitempty"`
}

// ProxyPoolRepository 代理池数据访问接口。
type ProxyPoolRepository interface {
	CreatePool(ctx context.Context, pool *ProxyPool) (*ProxyPool, error)
	GetPoolByID(ctx context.Context, id int64) (*ProxyPool, error)
	ListPools(ctx context.Context) ([]ProxyPool, error)
	ListPoolsWithStats(ctx context.Context) ([]ProxyPoolWithStats, error)
	UpdatePool(ctx context.Context, pool *ProxyPool) error
	DeletePool(ctx context.Context, id int64) error

	// ListPoolProxies 返回池内未删除代理（含健康字段）。
	ListPoolProxies(ctx context.Context, poolID int64) ([]Proxy, error)
	// AssignProxiesToPool 将 proxies 加入池；已属于其它池的会被改派。
	AssignProxiesToPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error)
	// RemoveProxiesFromPool 将 proxies 从池中移出（pool_id 置 NULL）。
	RemoveProxiesFromPool(ctx context.Context, proxyIDs []int64) (int64, error)

	// UpdateProxyPoolHealth 更新单个代理的池健康字段。
	UpdateProxyPoolHealth(ctx context.Context, proxyID int64, health string, failures int, checkedAt time.Time) error
	// CountAccountsByProxyIDs 统计各代理绑定的活跃账号数。
	CountAccountsByProxyIDs(ctx context.Context, proxyIDs []int64) (map[int64]int64, error)
	// RebindAccountsOffProxy 把绑定在 fromProxyID 上的账号改投到 toProxyID
	// （toProxyID 为 nil 时改投直连），记录 fallback origin 供手动回切。
	// 返回受影响账号 ID 列表。
	RebindAccountsOffProxy(ctx context.Context, fromProxyID int64, toProxyID *int64) ([]int64, error)

	// RecordRebindLog 记录一次池重绑操作（审计/管理端展示）。
	RecordRebindLog(ctx context.Context, log *ProxyPoolRebindLog) error
	// ListRebindLogs 返回池内最近的重绑日志（含代理名称，desc 排序）。
	ListRebindLogs(ctx context.Context, poolID int64, limit int) ([]ProxyPoolRebindLog, error)

	// ListPoolUnassignedAccountIDs 返回绑定该池但 proxy_id 为空或 proxy 不属于
	// 该池的账号 ID（供池服务补齐分配）。
	ListPoolUnassignedAccountIDs(ctx context.Context, poolID int64) ([]int64, error)
	// AssignAccountToProxy 把单个账号改投到指定代理（池服务分配用）。
	AssignAccountToProxy(ctx context.Context, accountID int64, proxyID int64) error
}
