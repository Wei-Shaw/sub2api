//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type ProxyPoolRepoSuite struct {
	suite.Suite
	ctx       context.Context
	tx        *dbent.Tx
	repo      *proxyPoolRepository
	proxyRepo *proxyRepository
}

func (s *ProxyPoolRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.repo = newProxyPoolRepositoryWithSQL(tx.Client(), tx)
	s.proxyRepo = newProxyRepositoryWithSQL(tx.Client(), tx)
}

func TestProxyPoolRepoSuite(t *testing.T) {
	suite.Run(t, new(ProxyPoolRepoSuite))
}

func (s *ProxyPoolRepoSuite) TestPoolLifecycle() {
	pool, err := s.repo.CreatePool(s.ctx, &service.ProxyPool{
		Name:                  "pool-lifecycle",
		Status:                service.StatusActive,
		HealthIntervalSeconds: 60,
		FailureThreshold:      2,
		AutoRebind:            true,
	})
	s.Require().NoError(err, "CreatePool")
	s.Require().NotZero(pool.ID)

	got, err := s.repo.GetPoolByID(s.ctx, pool.ID)
	s.Require().NoError(err)
	s.Require().Equal("pool-lifecycle", got.Name)
	s.Require().Equal(60, got.HealthIntervalSeconds)

	// 更新
	err = s.repo.UpdatePool(s.ctx, &service.ProxyPool{
		ID:                    pool.ID,
		Name:                  "pool-lifecycle-renamed",
		Status:                service.StatusDisabled,
		HealthIntervalSeconds: 120,
		FailureThreshold:      3,
		AutoRebind:            false,
	})
	s.Require().NoError(err, "UpdatePool")
	got, err = s.repo.GetPoolByID(s.ctx, pool.ID)
	s.Require().NoError(err)
	s.Require().Equal("pool-lifecycle-renamed", got.Name)
	s.Require().Equal(service.StatusDisabled, got.Status)
	s.Require().False(got.AutoRebind)
}

func (s *ProxyPoolRepoSuite) TestAssignAndListPoolProxies() {
	pool, err := s.repo.CreatePool(s.ctx, &service.ProxyPool{Name: "pool-assign", Status: service.StatusActive})
	s.Require().NoError(err)

	p1 := &service.Proxy{Name: "pool-p1", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive}
	p2 := &service.Proxy{Name: "pool-p2", Protocol: "http", Host: "127.0.0.1", Port: 8082, Status: service.StatusActive}
	s.Require().NoError(s.proxyRepo.Create(s.ctx, p1))
	s.Require().NoError(s.proxyRepo.Create(s.ctx, p2))

	affected, err := s.repo.AssignProxiesToPool(s.ctx, pool.ID, []int64{p1.ID, p2.ID})
	s.Require().NoError(err)
	s.Require().Equal(int64(2), affected)

	proxies, err := s.repo.ListPoolProxies(s.ctx, pool.ID)
	s.Require().NoError(err)
	s.Require().Len(proxies, 2)
	for _, p := range proxies {
		s.Require().NotNil(p.PoolID)
		s.Require().Equal(pool.ID, *p.PoolID)
		s.Require().Equal(service.PoolHealthUnknown, p.PoolHealth)
	}

	// 移除
	removed, err := s.repo.RemoveProxiesFromPool(s.ctx, []int64{p1.ID})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), removed)
	proxies, err = s.repo.ListPoolProxies(s.ctx, pool.ID)
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Equal(p2.ID, proxies[0].ID)
}

func (s *ProxyPoolRepoSuite) TestUpdateProxyPoolHealth() {
	proxy := &service.Proxy{Name: "pool-health", Protocol: "http", Host: "127.0.0.1", Port: 8083, Status: service.StatusActive}
	s.Require().NoError(s.proxyRepo.Create(s.ctx, proxy))

	checkedAt := time.Now()
	err := s.repo.UpdateProxyPoolHealth(s.ctx, proxy.ID, service.PoolHealthUnhealthy, 3, checkedAt)
	s.Require().NoError(err)

	got, err := s.proxyRepo.GetByID(s.ctx, proxy.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.PoolHealthUnhealthy, got.PoolHealth)
	s.Require().Equal(3, got.PoolFailures)
	s.Require().NotNil(got.PoolCheckedAt)
}

func (s *ProxyPoolRepoSuite) TestRebindAccountsOffProxy() {
	pool, err := s.repo.CreatePool(s.ctx, &service.ProxyPool{Name: "pool-rebind", Status: service.StatusActive})
	s.Require().NoError(err)

	from := &service.Proxy{Name: "pool-from", Protocol: "http", Host: "127.0.0.1", Port: 8084, Status: service.StatusActive}
	to := &service.Proxy{Name: "pool-to", Protocol: "http", Host: "127.0.0.1", Port: 8085, Status: service.StatusActive}
	s.Require().NoError(s.proxyRepo.Create(s.ctx, from))
	s.Require().NoError(s.proxyRepo.Create(s.ctx, to))
	_, err = s.repo.AssignProxiesToPool(s.ctx, pool.ID, []int64{from.ID, to.ID})
	s.Require().NoError(err)

	// 3 个账号绑定 from 代理
	accounts := make([]*service.Account, 0, 3)
	for i := 0; i < 3; i++ {
		a := mustCreateAccount(s.T(), s.tx.Client(), &service.Account{
			Name:    "pool-rebind-acc",
			ProxyID: &from.ID,
		})
		accounts = append(accounts, a)
	}

	// 账号数统计
	counts, err := s.repo.CountAccountsByProxyIDs(s.ctx, []int64{from.ID, to.ID})
	s.Require().NoError(err)
	s.Require().Equal(int64(3), counts[from.ID])
	s.Require().Equal(int64(0), counts[to.ID])

	// 改投
	changed, err := s.repo.RebindAccountsOffProxy(s.ctx, from.ID, &to.ID)
	s.Require().NoError(err)
	s.Require().Len(changed, 3)

	counts, err = s.repo.CountAccountsByProxyIDs(s.ctx, []int64{from.ID, to.ID})
	s.Require().NoError(err)
	s.Require().Equal(int64(0), counts[from.ID])
	s.Require().Equal(int64(3), counts[to.ID])

	// 再跑一轮不应有变化（无账号绑定 from）
	changed, err = s.repo.RebindAccountsOffProxy(s.ctx, from.ID, &to.ID)
	s.Require().NoError(err)
	s.Require().Empty(changed)
	_ = accounts
}

func (s *ProxyPoolRepoSuite) TestRebindLogs() {
	pool, err := s.repo.CreatePool(s.ctx, &service.ProxyPool{Name: "pool-logs", Status: service.StatusActive})
	s.Require().NoError(err)

	fromID := int64(101)
	toID := int64(102)
	for i := 0; i < 3; i++ {
		s.Require().NoError(s.repo.RecordRebindLog(s.ctx, &service.ProxyPoolRebindLog{
			PoolID:       pool.ID,
			FromProxyID:  &fromID,
			ToProxyID:    &toID,
			AccountCount: i + 1,
			Reason:       "unhealthy",
		}))
	}

	logs, err := s.repo.ListRebindLogs(s.ctx, pool.ID, 10)
	s.Require().NoError(err)
	s.Require().Len(logs, 3)
	// desc 排序：最新在前
	s.Require().Equal(3, logs[0].AccountCount)
	s.Require().Equal(int64(101), *logs[0].FromProxyID)
	s.Require().Equal(int64(102), *logs[0].ToProxyID)
	s.Require().Equal("unhealthy", logs[0].Reason)

	// limit 生效
	logs, err = s.repo.ListRebindLogs(s.ctx, pool.ID, 2)
	s.Require().NoError(err)
	s.Require().Len(logs, 2)
}

func (s *ProxyPoolRepoSuite) TestListPoolsWithStats() {
	pool, err := s.repo.CreatePool(s.ctx, &service.ProxyPool{Name: "pool-stats", Status: service.StatusActive})
	s.Require().NoError(err)

	p1 := &service.Proxy{Name: "pool-stats-p1", Protocol: "http", Host: "127.0.0.1", Port: 8086, Status: service.StatusActive}
	s.Require().NoError(s.proxyRepo.Create(s.ctx, p1))
	_, err = s.repo.AssignProxiesToPool(s.ctx, pool.ID, []int64{p1.ID})
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateProxyPoolHealth(s.ctx, p1.ID, service.PoolHealthHealthy, 0, time.Now()))

	mustCreateAccount(s.T(), s.tx.Client(), &service.Account{Name: "pool-stats-acc", ProxyID: &p1.ID})

	stats, err := s.repo.ListPoolsWithStats(s.ctx)
	s.Require().NoError(err)
	var found *service.ProxyPoolWithStats
	for i := range stats {
		if stats[i].ID == pool.ID {
			found = &stats[i]
			break
		}
	}
	s.Require().NotNil(found, "pool should appear in stats")
	s.Require().Equal(int64(1), found.ProxyCount)
	s.Require().Equal(int64(1), found.HealthyCount)
	s.Require().Equal(int64(0), found.UnhealthyCount)
	s.Require().Equal(int64(1), found.BoundAccountSum)
}
