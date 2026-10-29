//go:build integration

package repository

import (
	"time"

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

func (s *ProxyRepoSuite) TestListWithFiltersAndAccountCount_SortByExpiry() {
	now := time.Now()
	past := now.Add(-24 * time.Hour)
	soon := now.Add(72 * time.Hour)
	later := now.Add(100 * 24 * time.Hour)

	// 创建顺序(=ID 升序)刻意打乱,使其不同于任何有效期顺序:
	// 一旦排序退回按 id(没加 case "expiry"),此测试必然失败。
	pLater := s.mustCreateProxy(&service.Proxy{Name: "p-later", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive, ExpiresAt: &later})
	pNever := s.mustCreateProxy(&service.Proxy{Name: "p-never", Protocol: "http", Host: "127.0.0.1", Port: 8081, Status: service.StatusActive, ExpiresAt: nil})
	pExpired := s.mustCreateProxy(&service.Proxy{Name: "p-expired", Protocol: "http", Host: "127.0.0.1", Port: 8082, Status: service.StatusActive, ExpiresAt: &past})
	pSoon := s.mustCreateProxy(&service.Proxy{Name: "p-soon", Protocol: "http", Host: "127.0.0.1", Port: 8083, Status: service.StatusActive, ExpiresAt: &soon})

	// 升序:最快到期在前,NULL(永不过期)垫底
	asc, _, err := s.repo.ListWithFiltersAndAccountCount(s.ctx, pagination.PaginationParams{
		Page: 1, PageSize: 10, SortBy: "expiry", SortOrder: "asc",
	}, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(asc, 4)
	s.Require().Equal(
		[]int64{pExpired.ID, pSoon.ID, pLater.ID, pNever.ID},
		[]int64{asc[0].ID, asc[1].ID, asc[2].ID, asc[3].ID},
		"asc: 过期→快到期→远期→永不过期(垫底)",
	)

	// 降序:NULL(永不过期)置顶
	desc, _, err := s.repo.ListWithFiltersAndAccountCount(s.ctx, pagination.PaginationParams{
		Page: 1, PageSize: 10, SortBy: "expiry", SortOrder: "desc",
	}, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(desc, 4)
	s.Require().Equal(
		[]int64{pNever.ID, pLater.ID, pSoon.ID, pExpired.ID},
		[]int64{desc[0].ID, desc[1].ID, desc[2].ID, desc[3].ID},
		"desc: 永不过期(置顶)→远期→快到期→过期",
	)
}
