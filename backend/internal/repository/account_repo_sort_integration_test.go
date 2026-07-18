//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *AccountRepoSuite) TestList_DefaultSortByNameAsc() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "z-account"})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-account"})

	accounts, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Len(accounts, 2)
	s.Require().Equal("a-account", accounts[0].Name)
	s.Require().Equal("z-account", accounts[1].Name)
}

func (s *AccountRepoSuite) TestListWithFilters_SortByPriorityDesc() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "low-priority", Priority: 10})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "high-priority", Priority: 90})

	accounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "priority",
		SortOrder: "desc",
	}, "", "", "", "", 0, "")
	s.Require().NoError(err)
	s.Require().Len(accounts, 2)
	s.Require().Equal("high-priority", accounts[0].Name)
	s.Require().Equal("low-priority", accounts[1].Name)
}

func (s *AccountRepoSuite) TestListWithFilters_SortByLastUsedAtNullsLast() {
	older := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	recent := older.Add(time.Minute)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "never-used"})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "older-used", LastUsedAt: &older})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "recent-used", LastUsedAt: &recent})

	descAccounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "last_used_at",
		SortOrder: "desc",
	}, "", "", "", "", 0, "")
	s.Require().NoError(err)
	s.Require().Len(descAccounts, 3)
	s.Require().Equal(
		[]string{"recent-used", "older-used", "never-used"},
		[]string{descAccounts[0].Name, descAccounts[1].Name, descAccounts[2].Name},
	)

	ascAccounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "last_used_at",
		SortOrder: "asc",
	}, "", "", "", "", 0, "")
	s.Require().NoError(err)
	s.Require().Len(ascAccounts, 3)
	s.Require().Equal(
		[]string{"older-used", "recent-used", "never-used"},
		[]string{ascAccounts[0].Name, ascAccounts[1].Name, ascAccounts[2].Name},
	)
}
