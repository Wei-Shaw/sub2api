//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *RedeemCodeRepoSuite) TestListWithFilters_SortByValueAsc() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "VALUE-20", Type: service.RedeemTypeBalance, Value: 20, Status: service.StatusUnused}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "VALUE-10", Type: service.RedeemTypeBalance, Value: 10, Status: service.StatusUnused}))

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "value",
		SortOrder: "asc",
	}, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(codes, 2)
	s.Require().Equal("VALUE-10", codes[0].Code)
	s.Require().Equal("VALUE-20", codes[1].Code)
}
