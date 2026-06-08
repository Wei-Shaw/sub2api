//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type ProxyRepoSuite struct {
	suite.Suite
	ctx  context.Context
	tx   *dbent.Tx
	repo *proxyRepository
}

func (s *ProxyRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.repo = newProxyRepositoryWithSQL(tx.Client(), tx)
}

func TestProxyRepoSuite(t *testing.T) {
	suite.Run(t, new(ProxyRepoSuite))
}

// --- Create / GetByID / Update / Delete ---

func (s *ProxyRepoSuite) TestCreate() {
	proxy := &service.Proxy{
		Name:     "test-create",
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Status:   service.StatusActive,
	}

	err := s.repo.Create(s.ctx, proxy)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(proxy.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, proxy.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
}

func (s *ProxyRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *ProxyRepoSuite) TestUpdate() {
	proxy := &service.Proxy{
		Name:     "original",
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Status:   service.StatusActive,
	}
	s.Require().NoError(s.repo.Create(s.ctx, proxy))

	proxy.Name = "updated"
	err := s.repo.Update(s.ctx, proxy)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, proxy.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
}

func (s *ProxyRepoSuite) TestDelete() {
	proxy := &service.Proxy{
		Name:     "to-delete",
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Status:   service.StatusActive,
	}
	s.Require().NoError(s.repo.Create(s.ctx, proxy))

	err := s.repo.Delete(s.ctx, proxy.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, proxy.ID)
	s.Require().Error(err, "expected error after delete")
}

// --- List / ListWithFilters ---

func (s *ProxyRepoSuite) TestList() {
	s.mustCreateProxy(&service.Proxy{Name: "p1", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	s.mustCreateProxy(&service.Proxy{Name: "p2", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive})

	proxies, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(proxies, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *ProxyRepoSuite) TestListWithFilters_Protocol() {
	s.mustCreateProxy(&service.Proxy{Name: "p1", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	s.mustCreateProxy(&service.Proxy{Name: "p2", Protocol: "socks5", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive})

	proxies, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "socks5", "", "")
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Equal("socks5", proxies[0].Protocol)
}

func (s *ProxyRepoSuite) TestListWithFilters_Status() {
	s.mustCreateProxy(&service.Proxy{Name: "p1", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	s.mustCreateProxy(&service.Proxy{Name: "p2", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusDisabled})

	proxies, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", service.StatusDisabled, "")
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Equal(service.StatusDisabled, proxies[0].Status)
}

func (s *ProxyRepoSuite) TestListWithFilters_Search() {
	s.mustCreateProxy(&service.Proxy{Name: "production-proxy", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	s.mustCreateProxy(&service.Proxy{Name: "dev-proxy", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive})

	proxies, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "prod")
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Contains(proxies[0].Name, "production")
}

// --- ListActive ---

func (s *ProxyRepoSuite) TestListActive() {
	s.mustCreateProxy(&service.Proxy{Name: "active1", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	s.mustCreateProxy(&service.Proxy{Name: "inactive1", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusDisabled})

	proxies, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(proxies, 1)
	s.Require().Equal("active1", proxies[0].Name)
}

// --- ExistsByHostPortAuth ---

func (s *ProxyRepoSuite) TestExistsByHostPortAuth() {
	s.mustCreateProxy(&service.Proxy{
		Name:     "p1",
		Protocol: "http",
		Host:     "1.2.3.4",
		Port:     8080,
		Username: "user",
		Password: "pass",
		Status:   service.StatusActive,
	})

	exists, err := s.repo.ExistsByHostPortAuth(s.ctx, "1.2.3.4", 8080, "user", "pass")
	s.Require().NoError(err, "ExistsByHostPortAuth")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByHostPortAuth(s.ctx, "1.2.3.4", 8080, "wrong", "creds")
	s.Require().NoError(err)
	s.Require().False(notExists)
}

func (s *ProxyRepoSuite) TestExistsByHostPortAuth_NoAuth() {
	s.mustCreateProxy(&service.Proxy{
		Name:     "p-noauth",
		Protocol: "http",
		Host:     "5.6.7.8",
		Port:     8081,
		Username: "",
		Password: "",
		Status:   service.StatusActive,
	})

	exists, err := s.repo.ExistsByHostPortAuth(s.ctx, "5.6.7.8", 8081, "", "")
	s.Require().NoError(err)
	s.Require().True(exists)
}

// --- CountAccountsByProxyID ---

func (s *ProxyRepoSuite) TestCountAccountsByProxyID() {
	proxy := s.mustCreateProxy(&service.Proxy{Name: "p-count", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	s.mustInsertAccount("a1", &proxy.ID)
	s.mustInsertAccount("a2", &proxy.ID)
	s.mustInsertAccount("a3", nil) // no proxy

	count, err := s.repo.CountAccountsByProxyID(s.ctx, proxy.ID)
	s.Require().NoError(err, "CountAccountsByProxyID")
	s.Require().Equal(int64(2), count)
}

func (s *ProxyRepoSuite) TestCountAccountsByProxyID_Zero() {
	proxy := s.mustCreateProxy(&service.Proxy{Name: "p-zero", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})

	count, err := s.repo.CountAccountsByProxyID(s.ctx, proxy.ID)
	s.Require().NoError(err)
	s.Require().Zero(count)
}

// --- GetAccountCountsForProxies ---

func (s *ProxyRepoSuite) TestGetAccountCountsForProxies() {
	p1 := s.mustCreateProxy(&service.Proxy{Name: "p1", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	p2 := s.mustCreateProxy(&service.Proxy{Name: "p2", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive})

	s.mustInsertAccount("a1", &p1.ID)
	s.mustInsertAccount("a2", &p1.ID)
	s.mustInsertAccount("a3", &p2.ID)

	counts, err := s.repo.GetAccountCountsForProxies(s.ctx)
	s.Require().NoError(err, "GetAccountCountsForProxies")
	s.Require().Equal(int64(2), counts[p1.ID])
	s.Require().Equal(int64(1), counts[p2.ID])
}

func (s *ProxyRepoSuite) TestGetAccountCountsForProxies_Empty() {
	counts, err := s.repo.GetAccountCountsForProxies(s.ctx)
	s.Require().NoError(err)
	s.Require().Empty(counts)
}

// --- ListActiveWithAccountCount ---

func (s *ProxyRepoSuite) TestListActiveWithAccountCount() {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	p1 := s.mustCreateProxyWithTimes("p1", service.StatusActive, base.Add(-1*time.Hour))
	p2 := s.mustCreateProxyWithTimes("p2", service.StatusActive, base)
	s.mustCreateProxyWithTimes("p3-inactive", service.StatusDisabled, base.Add(1*time.Hour))

	s.mustInsertAccount("a1", &p1.ID)
	s.mustInsertAccount("a2", &p1.ID)
	s.mustInsertAccount("a3", &p2.ID)

	withCounts, err := s.repo.ListActiveWithAccountCount(s.ctx)
	s.Require().NoError(err, "ListActiveWithAccountCount")
	s.Require().Len(withCounts, 2, "expected 2 active proxies")

	// Sorted by created_at DESC, so p2 first
	s.Require().Equal(p2.ID, withCounts[0].ID)
	s.Require().Equal(int64(1), withCounts[0].AccountCount)
	s.Require().Equal(p1.ID, withCounts[1].ID)
	s.Require().Equal(int64(2), withCounts[1].AccountCount)
}

// --- Combined original test ---

func (s *ProxyRepoSuite) TestExistsByHostPortAuth_And_AccountCountAggregates() {
	p1 := s.mustCreateProxy(&service.Proxy{Name: "p1", Protocol: "http", Host: "1.2.3.4", Port: 8080, Username: "u", Password: "p", Status: service.StatusActive})
	p2 := s.mustCreateProxy(&service.Proxy{Name: "p2", Protocol: "http", Host: "5.6.7.8", Port: 8081, Username: "", Password: "", Status: service.StatusActive})

	exists, err := s.repo.ExistsByHostPortAuth(s.ctx, "1.2.3.4", 8080, "u", "p")
	s.Require().NoError(err, "ExistsByHostPortAuth")
	s.Require().True(exists, "expected proxy to exist")

	s.mustInsertAccount("a1", &p1.ID)
	s.mustInsertAccount("a2", &p1.ID)
	s.mustInsertAccount("a3", &p2.ID)

	count1, err := s.repo.CountAccountsByProxyID(s.ctx, p1.ID)
	s.Require().NoError(err, "CountAccountsByProxyID")
	s.Require().Equal(int64(2), count1, "expected 2 accounts for p1")

	counts, err := s.repo.GetAccountCountsForProxies(s.ctx)
	s.Require().NoError(err, "GetAccountCountsForProxies")
	s.Require().Equal(int64(2), counts[p1.ID])
	s.Require().Equal(int64(1), counts[p2.ID])

	withCounts, err := s.repo.ListActiveWithAccountCount(s.ctx)
	s.Require().NoError(err, "ListActiveWithAccountCount")
	s.Require().Len(withCounts, 2, "expected 2 proxies")
	for _, pc := range withCounts {
		switch pc.ID {
		case p1.ID:
			s.Require().Equal(int64(2), pc.AccountCount, "p1 count mismatch")
		case p2.ID:
			s.Require().Equal(int64(1), pc.AccountCount, "p2 count mismatch")
		default:
			s.Require().Fail("unexpected proxy id", pc.ID)
		}
	}
}

func (s *ProxyRepoSuite) mustCreateProxy(p *service.Proxy) *service.Proxy {
	s.T().Helper()
	s.Require().NoError(s.repo.Create(s.ctx, p), "create proxy")
	return p
}

func (s *ProxyRepoSuite) mustCreateProxyWithTimes(name, status string, createdAt time.Time) *service.Proxy {
	s.T().Helper()

	// Use the repository create for standard fields, then update timestamps via raw SQL to keep deterministic ordering.
	p := s.mustCreateProxy(&service.Proxy{
		Name:     name,
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Status:   status,
	})
	_, err := s.tx.ExecContext(s.ctx, "UPDATE proxies SET created_at = $1, updated_at = $1 WHERE id = $2", createdAt, p.ID)
	s.Require().NoError(err, "update proxy timestamps")
	return p
}

func (s *ProxyRepoSuite) mustInsertAccount(name string, proxyID *int64) {
	s.T().Helper()
	var pid any
	if proxyID != nil {
		pid = *proxyID
	}
	_, err := s.tx.ExecContext(
		s.ctx,
		"INSERT INTO accounts (name, platform, type, proxy_id) VALUES ($1, $2, $3, $4)",
		name,
		service.PlatformAnthropic,
		service.AccountTypeOAuth,
		pid,
	)
	s.Require().NoError(err, "insert account")
}
