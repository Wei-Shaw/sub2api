//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *ProxyRepoSuite) TestListWithFiltersAndAccountCount_SortByAccountCountDesc() {
	p1 := s.mustCreateProxy(&service.Proxy{Name: "p1", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive})
	p2 := s.mustCreateProxy(&service.Proxy{Name: "p2", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive})
	s.mustInsertAccount("a1", &p1.ID)
	s.mustInsertAccount("a2", &p1.ID)
	s.mustInsertAccount("a3", &p2.ID)

	proxies, _, err := s.repo.ListWithFiltersAndAccountCount(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "account_count",
		SortOrder: "desc",
	}, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(proxies, 2)
	s.Require().Equal(p1.ID, proxies[0].ID)
	s.Require().Equal(int64(2), proxies[0].AccountCount)
	s.Require().Equal(p2.ID, proxies[1].ID)
}
