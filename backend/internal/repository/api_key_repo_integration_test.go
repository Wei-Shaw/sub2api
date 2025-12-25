//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ApiKeyRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *ApiKeyRepository
}

func (s *ApiKeyRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewApiKeyRepository(s.db)
}

func TestApiKeyRepoSuite(t *testing.T) {
	suite.Run(t, new(ApiKeyRepoSuite))
}

// --- Create / GetByID / GetByKey ---

func (s *ApiKeyRepoSuite) TestCreate() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "create@test.com"})

	key := &model.ApiKey{
		UserID: user.ID,
		Key:    "sk-create-test",
		Name:   "Test Key",
		Status: model.StatusActive,
	}

	err := s.repo.Create(s.ctx, key)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(key.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("sk-create-test", got.Key)
}

func (s *ApiKeyRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *ApiKeyRepoSuite) TestGetByKey() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "getbykey@test.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-key"})

	key := mustCreateApiKey(s.T(), s.db, &model.ApiKey{
		UserID:  user.ID,
		Key:     "sk-getbykey",
		Name:    "My Key",
		GroupID: &group.ID,
		Status:  model.StatusActive,
	})

	got, err := s.repo.GetByKey(s.ctx, key.Key)
	s.Require().NoError(err, "GetByKey")
	s.Require().Equal(key.ID, got.ID)
	s.Require().NotNil(got.User, "expected User preload")
	s.Require().Equal(user.ID, got.User.ID)
	s.Require().NotNil(got.Group, "expected Group preload")
	s.Require().Equal(group.ID, got.Group.ID)
}

func (s *ApiKeyRepoSuite) TestGetByKey_NotFound() {
	_, err := s.repo.GetByKey(s.ctx, "non-existent-key")
	s.Require().Error(err, "expected error for non-existent key")
}

// --- Update ---

func (s *ApiKeyRepoSuite) TestUpdate() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "update@test.com"})
	key := mustCreateApiKey(s.T(), s.db, &model.ApiKey{
		UserID: user.ID,
		Key:    "sk-update",
		Name:   "Original",
		Status: model.StatusActive,
	})

	key.Name = "Renamed"
	key.Status = model.StatusDisabled
	err := s.repo.Update(s.ctx, key)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("sk-update", got.Key, "Update should not change key")
	s.Require().Equal(user.ID, got.UserID, "Update should not change user_id")
	s.Require().Equal("Renamed", got.Name)
	s.Require().Equal(model.StatusDisabled, got.Status)
}

func (s *ApiKeyRepoSuite) TestUpdate_ClearGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "cleargroup@test.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-clear"})
	key := mustCreateApiKey(s.T(), s.db, &model.ApiKey{
		UserID:  user.ID,
		Key:     "sk-clear-group",
		Name:    "Group Key",
		GroupID: &group.ID,
	})

	key.GroupID = nil
	err := s.repo.Update(s.ctx, key)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Nil(got.GroupID, "expected GroupID to be cleared")
}

// --- Delete ---

func (s *ApiKeyRepoSuite) TestDelete() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "delete@test.com"})
	key := mustCreateApiKey(s.T(), s.db, &model.ApiKey{
		UserID: user.ID,
		Key:    "sk-delete",
		Name:   "Delete Me",
	})

	err := s.repo.Delete(s.ctx, key.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, key.ID)
	s.Require().Error(err, "expected error after delete")
}

// --- ListByUserID / CountByUserID ---

func (s *ApiKeyRepoSuite) TestListByUserID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listbyuser@test.com"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-list-1", Name: "Key 1"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-list-2", Name: "Key 2"})

	keys, page, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByUserID")
	s.Require().Len(keys, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *ApiKeyRepoSuite) TestListByUserID_Pagination() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "paging@test.com"})
	for i := 0; i < 5; i++ {
		mustCreateApiKey(s.T(), s.db, &model.ApiKey{
			UserID: user.ID,
			Key:    "sk-page-" + string(rune('a'+i)),
			Name:   "Key",
		})
	}

	keys, page, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 2})
	s.Require().NoError(err)
	s.Require().Len(keys, 2)
	s.Require().Equal(int64(5), page.Total)
	s.Require().Equal(3, page.Pages)
}

func (s *ApiKeyRepoSuite) TestCountByUserID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "count@test.com"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-count-1", Name: "K1"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-count-2", Name: "K2"})

	count, err := s.repo.CountByUserID(s.ctx, user.ID)
	s.Require().NoError(err, "CountByUserID")
	s.Require().Equal(int64(2), count)
}

// --- ListByGroupID / CountByGroupID ---

func (s *ApiKeyRepoSuite) TestListByGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listbygroup@test.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-list"})

	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-grp-1", Name: "K1", GroupID: &group.ID})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-grp-2", Name: "K2", GroupID: &group.ID})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-grp-3", Name: "K3"}) // no group

	keys, page, err := s.repo.ListByGroupID(s.ctx, group.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByGroupID")
	s.Require().Len(keys, 2)
	s.Require().Equal(int64(2), page.Total)
	// User preloaded
	s.Require().NotNil(keys[0].User)
}

func (s *ApiKeyRepoSuite) TestCountByGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "countgroup@test.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-count"})

	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-gc-1", Name: "K1", GroupID: &group.ID})

	count, err := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountByGroupID")
	s.Require().Equal(int64(1), count)
}

// --- ExistsByKey ---

func (s *ApiKeyRepoSuite) TestExistsByKey() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "exists@test.com"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-exists", Name: "K"})

	exists, err := s.repo.ExistsByKey(s.ctx, "sk-exists")
	s.Require().NoError(err, "ExistsByKey")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByKey(s.ctx, "sk-not-exists")
	s.Require().NoError(err)
	s.Require().False(notExists)
}

// --- SearchApiKeys ---

func (s *ApiKeyRepoSuite) TestSearchApiKeys() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "search@test.com"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-search-1", Name: "Production Key"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-search-2", Name: "Development Key"})

	found, err := s.repo.SearchApiKeys(s.ctx, user.ID, "prod", 10)
	s.Require().NoError(err, "SearchApiKeys")
	s.Require().Len(found, 1)
	s.Require().Contains(found[0].Name, "Production")
}

func (s *ApiKeyRepoSuite) TestSearchApiKeys_NoKeyword() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "searchnokw@test.com"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-nk-1", Name: "K1"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-nk-2", Name: "K2"})

	found, err := s.repo.SearchApiKeys(s.ctx, user.ID, "", 10)
	s.Require().NoError(err)
	s.Require().Len(found, 2)
}

func (s *ApiKeyRepoSuite) TestSearchApiKeys_NoUserID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "searchnouid@test.com"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-nu-1", Name: "TestKey"})

	found, err := s.repo.SearchApiKeys(s.ctx, 0, "testkey", 10)
	s.Require().NoError(err)
	s.Require().Len(found, 1)
}

// --- ClearGroupIDByGroupID ---

func (s *ApiKeyRepoSuite) TestClearGroupIDByGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "cleargrp@test.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-clear-bulk"})

	k1 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-clr-1", Name: "K1", GroupID: &group.ID})
	k2 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-clr-2", Name: "K2", GroupID: &group.ID})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-clr-3", Name: "K3"}) // no group

	affected, err := s.repo.ClearGroupIDByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ClearGroupIDByGroupID")
	s.Require().Equal(int64(2), affected)

	got1, _ := s.repo.GetByID(s.ctx, k1.ID)
	got2, _ := s.repo.GetByID(s.ctx, k2.ID)
	s.Require().Nil(got1.GroupID)
	s.Require().Nil(got2.GroupID)

	count, _ := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().Zero(count)
}

// --- Combined CRUD/Search/ClearGroupID (original test preserved as integration) ---

func (s *ApiKeyRepoSuite) TestCRUD_Search_ClearGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "k@example.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-k"})

	key := mustCreateApiKey(s.T(), s.db, &model.ApiKey{
		UserID:  user.ID,
		Key:     "sk-test-1",
		Name:    "My Key",
		GroupID: &group.ID,
		Status:  model.StatusActive,
	})

	got, err := s.repo.GetByKey(s.ctx, key.Key)
	s.Require().NoError(err, "GetByKey")
	s.Require().Equal(key.ID, got.ID)
	s.Require().NotNil(got.User)
	s.Require().Equal(user.ID, got.User.ID)
	s.Require().NotNil(got.Group)
	s.Require().Equal(group.ID, got.Group.ID)

	key.Name = "Renamed"
	key.Status = model.StatusDisabled
	key.GroupID = nil
	s.Require().NoError(s.repo.Update(s.ctx, key), "Update")

	got2, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("sk-test-1", got2.Key, "Update should not change key")
	s.Require().Equal(user.ID, got2.UserID, "Update should not change user_id")
	s.Require().Equal("Renamed", got2.Name)
	s.Require().Equal(model.StatusDisabled, got2.Status)
	s.Require().Nil(got2.GroupID)

	keys, page, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByUserID")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(keys, 1)

	exists, err := s.repo.ExistsByKey(s.ctx, "sk-test-1")
	s.Require().NoError(err, "ExistsByKey")
	s.Require().True(exists, "expected key to exist")

	found, err := s.repo.SearchApiKeys(s.ctx, user.ID, "renam", 10)
	s.Require().NoError(err, "SearchApiKeys")
	s.Require().Len(found, 1)
	s.Require().Equal(key.ID, found[0].ID)

	// ClearGroupIDByGroupID
	k2 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{
		UserID:  user.ID,
		Key:     "sk-test-2",
		Name:    "Group Key",
		GroupID: &group.ID,
	})

	countBefore, err := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountByGroupID")
	s.Require().Equal(int64(1), countBefore, "expected 1 key in group before clear")

	affected, err := s.repo.ClearGroupIDByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ClearGroupIDByGroupID")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")

	got3, err := s.repo.GetByID(s.ctx, k2.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Nil(got3.GroupID, "expected GroupID cleared")

	countAfter, err := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountByGroupID after clear")
	s.Require().Equal(int64(0), countAfter, "expected 0 keys in group after clear")
}
