//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ProxyRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *ProxyRepository
}

func (s *ProxyRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewProxyRepository(s.db)
}

func TestProxyRepoSuite(t *testing.T) {
	suite.Run(t, new(ProxyRepoSuite))
}

// --- Create / GetByID / Update / Delete ---

func (s *ProxyRepoSuite) TestCreate() {
	proxy := &model.Proxy{
		Name:     "test-create",
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Status:   model.StatusActive,
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
	proxy := mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "original"})

	proxy.Name = "updated"
	err := s.repo.Update(s.ctx, proxy)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, proxy.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
}

func (s *ProxyRepoSuite) TestDelete() {
	proxy := mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "to-delete"})

	err := s.repo.Delete(s.ctx, proxy.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, proxy.ID)
	s.Require().Error(err, "expected error after delete")
}

// --- List / ListWithFilters ---

func (s *ProxyRepoSuite) TestList() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p1"})
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p2"})

	proxies, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(proxies, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *ProxyRepoSuite) TestListWithFilters_Protocol() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p1", Protocol: "http"})
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p2", Protocol: "socks5"})

	proxies, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "socks5", "", "")
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Equal("socks5", proxies[0].Protocol)
}

func (s *ProxyRepoSuite) TestListWithFilters_Status() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p1", Status: model.StatusActive})
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p2", Status: model.StatusDisabled})

	proxies, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", model.StatusDisabled, "")
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Equal(model.StatusDisabled, proxies[0].Status)
}

func (s *ProxyRepoSuite) TestListWithFilters_Search() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "production-proxy"})
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "dev-proxy"})

	proxies, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "prod")
	s.Require().NoError(err)
	s.Require().Len(proxies, 1)
	s.Require().Contains(proxies[0].Name, "production")
}

// --- ListActive ---

func (s *ProxyRepoSuite) TestListActive() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "active1", Status: model.StatusActive})
	mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "inactive1", Status: model.StatusDisabled})

	proxies, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(proxies, 1)
	s.Require().Equal("active1", proxies[0].Name)
}

// --- ExistsByHostPortAuth ---

func (s *ProxyRepoSuite) TestExistsByHostPortAuth() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:     "p1",
		Protocol: "http",
		Host:     "1.2.3.4",
		Port:     8080,
		Username: "user",
		Password: "pass",
	})

	exists, err := s.repo.ExistsByHostPortAuth(s.ctx, "1.2.3.4", 8080, "user", "pass")
	s.Require().NoError(err, "ExistsByHostPortAuth")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByHostPortAuth(s.ctx, "1.2.3.4", 8080, "wrong", "creds")
	s.Require().NoError(err)
	s.Require().False(notExists)
}

func (s *ProxyRepoSuite) TestExistsByHostPortAuth_NoAuth() {
	mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:     "p-noauth",
		Protocol: "http",
		Host:     "5.6.7.8",
		Port:     8081,
		Username: "",
		Password: "",
	})

	exists, err := s.repo.ExistsByHostPortAuth(s.ctx, "5.6.7.8", 8081, "", "")
	s.Require().NoError(err)
	s.Require().True(exists)
}

// --- CountAccountsByProxyID ---

func (s *ProxyRepoSuite) TestCountAccountsByProxyID() {
	proxy := mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p-count"})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a1", ProxyID: &proxy.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a2", ProxyID: &proxy.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a3"}) // no proxy

	count, err := s.repo.CountAccountsByProxyID(s.ctx, proxy.ID)
	s.Require().NoError(err, "CountAccountsByProxyID")
	s.Require().Equal(int64(2), count)
}

func (s *ProxyRepoSuite) TestCountAccountsByProxyID_Zero() {
	proxy := mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p-zero"})

	count, err := s.repo.CountAccountsByProxyID(s.ctx, proxy.ID)
	s.Require().NoError(err)
	s.Require().Zero(count)
}

// --- GetAccountCountsForProxies ---

func (s *ProxyRepoSuite) TestGetAccountCountsForProxies() {
	p1 := mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p1"})
	p2 := mustCreateProxy(s.T(), s.db, &model.Proxy{Name: "p2"})

	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a1", ProxyID: &p1.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a2", ProxyID: &p1.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a3", ProxyID: &p2.ID})

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

	p1 := mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:      "p1",
		Status:    model.StatusActive,
		CreatedAt: base.Add(-1 * time.Hour),
	})
	p2 := mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:      "p2",
		Status:    model.StatusActive,
		CreatedAt: base,
	})
	mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:   "p3-inactive",
		Status: model.StatusDisabled,
	})

	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a1", ProxyID: &p1.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a2", ProxyID: &p1.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a3", ProxyID: &p2.ID})

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
	p1 := mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:      "p1",
		Protocol:  "http",
		Host:      "1.2.3.4",
		Port:      8080,
		Username:  "u",
		Password:  "p",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	})
	p2 := mustCreateProxy(s.T(), s.db, &model.Proxy{
		Name:      "p2",
		Protocol:  "http",
		Host:      "5.6.7.8",
		Port:      8081,
		Username:  "",
		Password:  "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	exists, err := s.repo.ExistsByHostPortAuth(s.ctx, "1.2.3.4", 8080, "u", "p")
	s.Require().NoError(err, "ExistsByHostPortAuth")
	s.Require().True(exists, "expected proxy to exist")

	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a1", ProxyID: &p1.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a2", ProxyID: &p1.ID})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a3", ProxyID: &p2.ID})

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
