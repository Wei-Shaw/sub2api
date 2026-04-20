//go:build integration

package repository

import (
	"testing"
	"time"

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

func (s *UserRepoSuite) TestCreateAndRead_PreservesSignupSourceAndActivityTimestamps() {
	lastLoginAt := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond)
	lastActiveAt := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Microsecond)

	created := s.mustCreateUser(&service.User{
		Email:        "identity-meta@example.com",
		SignupSource: "github",
		LastLoginAt:  &lastLoginAt,
		LastActiveAt: &lastActiveAt,
REDACTED)

	got, err := s.repo.GetByID(s.ctx, created.ID)
	s.Require().NoError(err)
	s.Require().Equal("github", got.SignupSource)
	s.Require().NotNil(got.LastLoginAt)
	s.Require().NotNil(got.LastActiveAt)
	s.Require().True(got.LastLoginAt.Equal(lastLoginAt))
	s.Require().True(got.LastActiveAt.Equal(lastActiveAt))
REDACTED

func (s *UserRepoSuite) TestUpdate_PersistsSignupSourceAndActivityTimestamps() {
	created := s.mustCreateUser(&service.User{Email: "identity-update@example.com"REDACTED)
	lastLoginAt := time.Now().Add(-90 * time.Minute).UTC().Truncate(time.Microsecond)
	lastActiveAt := time.Now().Add(-15 * time.Minute).UTC().Truncate(time.Microsecond)

	created.SignupSource = "oidc"
	created.LastLoginAt = &lastLoginAt
	created.LastActiveAt = &lastActiveAt

	s.Require().NoError(s.repo.Update(s.ctx, created))

	got, err := s.repo.GetByID(s.ctx, created.ID)
	s.Require().NoError(err)
	s.Require().Equal("oidc", got.SignupSource)
	s.Require().NotNil(got.LastLoginAt)
	s.Require().NotNil(got.LastActiveAt)
	s.Require().True(got.LastLoginAt.Equal(lastLoginAt))
	s.Require().True(got.LastActiveAt.Equal(lastActiveAt))
REDACTED

func (s *UserRepoSuite) TestListWithFilters_SortByLastLoginAtDesc() {
	older := time.Now().Add(-4 * time.Hour).UTC().Truncate(time.Microsecond)
	newer := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Microsecond)

	s.mustCreateUser(&service.User{Email: "nil-login@example.com"REDACTED)
	s.mustCreateUser(&service.User{Email: "older-login@example.com", LastLoginAt: &olderREDACTED)
	s.mustCreateUser(&service.User{Email: "newer-login@example.com", LastLoginAt: &newerREDACTED)

	users, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "last_login_at",
		SortOrder: "desc",
REDACTED, service.UserListFilters{REDACTED)
	s.Require().NoError(err)
	s.Require().Len(users, 3)
	s.Require().Equal("newer-login@example.com", users[0].Email)
	s.Require().Equal("older-login@example.com", users[1].Email)
	s.Require().Equal("nil-login@example.com", users[2].Email)
REDACTED

func (s *UserRepoSuite) TestListWithFilters_SortByLastActiveAtAsc() {
	earlier := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Microsecond)
	later := time.Now().Add(-45 * time.Minute).UTC().Truncate(time.Microsecond)

	s.mustCreateUser(&service.User{Email: "nil-active@example.com"REDACTED)
	s.mustCreateUser(&service.User{Email: "later-active@example.com", LastActiveAt: &laterREDACTED)
	s.mustCreateUser(&service.User{Email: "earlier-active@example.com", LastActiveAt: &earlierREDACTED)

	users, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "last_active_at",
		SortOrder: "asc",
REDACTED, service.UserListFilters{REDACTED)
	s.Require().NoError(err)
	s.Require().Len(users, 3)
	s.Require().Equal("earlier-active@example.com", users[0].Email)
	s.Require().Equal("later-active@example.com", users[1].Email)
	s.Require().Equal("nil-active@example.com", users[2].Email)
REDACTED

func TestUserRepoSortSuiteSmoke(_ *testing.T) {REDACTED
