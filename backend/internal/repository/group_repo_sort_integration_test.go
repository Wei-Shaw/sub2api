//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestListWithAccountCountSort_AttachesActiveCount 验证通过 account_count 排序时，
// ActiveAccountCount 与 AccountCount 都被正确附加到返回结果中，
// 且排序基于 total 账号数而非 active 账号数。
func (s *GroupRepoSuite) TestListWithAccountCountSort_AttachesActiveCount() {
	// Group A: 2 total, 1 active (1 disabled account)
	gA := &service.Group{Name: "sort-count-a", Platform: service.PlatformAnthropic, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	// Group B: 1 total, 1 active
	gB := &service.Group{Name: "sort-count-b", Platform: service.PlatformAnthropic, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	s.Require().NoError(s.repo.Create(s.ctx, gA))
	s.Require().NoError(s.repo.Create(s.ctx, gB))

	insertAccount := func(name, status string) int64 {
		var id int64
		s.Require().NoError(scanSingleRow(s.ctx, s.tx,
			"INSERT INTO accounts (name, platform, type, status) VALUES ($1, $2, $3, $4) RETURNING id",
			[]any{name, service.PlatformAnthropic, service.AccountTypeOAuth, status},
			&id))
		return id
	}
	link := func(accountID, groupID int64, priority int) {
		_, err := s.tx.ExecContext(s.ctx,
			"INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())",
			accountID, groupID, priority)
		s.Require().NoError(err)
	}

	// gA: 1 active + 1 disabled → total=2, active=1
	link(insertAccount("sa-active", service.StatusActive), gA.ID, 1)
	link(insertAccount("sa-disabled", service.StatusDisabled), gA.ID, 2)
	// gB: 1 active → total=1, active=1
	link(insertAccount("sb-active", service.StatusActive), gB.ID, 1)

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page: 1, PageSize: 100, SortBy: "account_count", SortOrder: "desc",
	}, service.PlatformAnthropic, service.StatusActive, "", nil)
	s.Require().NoError(err)

	byID := make(map[int64]service.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}

	s.Require().Contains(byID, gA.ID, "gA must appear in results")
	s.Require().Contains(byID, gB.ID, "gB must appear in results")

	cA := byID[gA.ID]
	s.Assert().Equal(int64(2), cA.AccountCount, "gA AccountCount must be 2")
	s.Assert().Equal(int64(1), cA.ActiveAccountCount, "gA ActiveAccountCount must be 1")

	cB := byID[gB.ID]
	s.Assert().Equal(int64(1), cB.AccountCount, "gB AccountCount must be 1")
	s.Assert().Equal(int64(1), cB.ActiveAccountCount, "gB ActiveAccountCount must be 1")

	// Sort is by total (not active): gA (total=2) must rank higher than gB (total=1) in desc order
	indexByID := make(map[int64]int, len(groups))
	for i, g := range groups {
		indexByID[g.ID] = i
	}
	s.Assert().Less(indexByID[gA.ID], indexByID[gB.ID], "gA (total=2) must rank above gB (total=1) with account_count desc")
}

func (s *GroupRepoSuite) TestList_DefaultSortBySortOrderAsc() {
	g1 := &service.Group{Name: "g1", Platform: service.PlatformAnthropic, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, SortOrder: 20}
	g2 := &service.Group{Name: "g2", Platform: service.PlatformAnthropic, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, SortOrder: 10}
	s.Require().NoError(s.repo.Create(s.ctx, g1))
	s.Require().NoError(s.repo.Create(s.ctx, g2))

	groups, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 100})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(groups), 2)
	indexByID := make(map[int64]int, len(groups))
	for i, g := range groups {
		indexByID[g.ID] = i
	}
	s.Require().Contains(indexByID, g1.ID)
	s.Require().Contains(indexByID, g2.ID)
	// g2 has SortOrder=10, g1 has SortOrder=20; ascending means g2 comes first
	s.Require().Less(indexByID[g2.ID], indexByID[g1.ID])
}

func (s *GroupRepoSuite) TestList_SortBySortOrderDesc() {
	g1 := &service.Group{Name: "g1", Platform: service.PlatformAnthropic, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, SortOrder: 40}
	g2 := &service.Group{Name: "g2", Platform: service.PlatformAnthropic, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, SortOrder: 50}
	s.Require().NoError(s.repo.Create(s.ctx, g1))
	s.Require().NoError(s.repo.Create(s.ctx, g2))

	groups, _, err := s.repo.List(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "sort_order",
		SortOrder: "desc",
	})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(groups), 2)
	indexByID := make(map[int64]int, len(groups))
	for i, group := range groups {
		indexByID[group.ID] = i
	}
	s.Require().Contains(indexByID, g1.ID)
	s.Require().Contains(indexByID, g2.ID)
	s.Require().Less(indexByID[g2.ID], indexByID[g1.ID])
}
