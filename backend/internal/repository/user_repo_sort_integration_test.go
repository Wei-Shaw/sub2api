//go:build integration

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *UserRepoSuite) TestListWithFilters_SortByEmailAsc() {
	s.mustCreateUser(&service.User{Email: "z-last@example.com", Username: "z-user"REDACTED)
	s.mustCreateUser(&service.User{Email: "a-first@example.com", Username: "a-user"REDACTED)

	users, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "email",
		SortOrder: "asc",
REDACTED, service.UserListFilters{REDACTED)
	s.Require().NoError(err)
	s.Require().Len(users, 2)
	s.Require().Equal("a-first@example.com", users[0].Email)
	s.Require().Equal("z-last@example.com", users[1].Email)
REDACTED

func (s *UserRepoSuite) TestList_DefaultSortByNewestFirst() {
	first := s.mustCreateUser(&service.User{Email: "first@example.com"REDACTED)
	second := s.mustCreateUser(&service.User{Email: "second@example.com"REDACTED)

	users, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err)
	s.Require().Len(users, 2)
	s.Require().Equal(second.ID, users[0].ID)
	s.Require().Equal(first.ID, users[1].ID)
REDACTED

func TestUserRepoSortSuiteSmoke(_ *testing.T) {REDACTED
