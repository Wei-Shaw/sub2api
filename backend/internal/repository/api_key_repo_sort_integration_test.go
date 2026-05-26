//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *APIKeyRepoSuite) TestListByUserID_SortByNameAsc() {
	user := s.mustCreateUser("sort-name@example.com")
	s.mustCreateApiKey(user.ID, "sk-z", "z-key", nil)
	s.mustCreateApiKey(user.ID, "sk-a", "a-key", nil)

	keys, _, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "name",
		SortOrder: "asc",
	}, service.APIKeyListFilters{})
	s.Require().NoError(err)
	s.Require().Len(keys, 2)
	s.Require().Equal("a-key", keys[0].Name)
	s.Require().Equal("z-key", keys[1].Name)
}
