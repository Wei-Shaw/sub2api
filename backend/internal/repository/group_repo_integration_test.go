//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type GroupRepoSuite struct {
	suite.Suite
	ctx  context.Context
	tx   *dbent.Tx
	repo *groupRepository
}

type forbidSQLExecutor struct {
	called bool
}

func (s *forbidSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	s.called = true
	return nil, errors.New("unexpected sql exec")
}

func (s *forbidSQLExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	s.called = true
	return nil, errors.New("unexpected sql query")
}

func (s *GroupRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.repo = newGroupRepositoryWithSQL(tx.Client(), tx)
}

func TestGroupRepoSuite(t *testing.T) {
	suite.Run(t, new(GroupRepoSuite))
}

// --- Create / GetByID / Update / Delete ---

func (s *GroupRepoSuite) TestCreate() {
	group := &service.Group{
		Name:             "test-create",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}

	err := s.repo.Create(s.ctx, group)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(group.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
}

func (s *GroupRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
	s.Require().ErrorIs(err, service.ErrGroupNotFound)
}

func (s *GroupRepoSuite) TestGetByIDLite_DoesNotUseAccountCount() {
	group := &service.Group{
		Name:             "lite-group",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	spy := &forbidSQLExecutor{}
	repo := newGroupRepositoryWithSQL(s.tx.Client(), spy)

	got, err := repo.GetByIDLite(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Equal(group.ID, got.ID)
	s.Require().False(spy.called, "expected no direct sql executor usage")
}

func (s *GroupRepoSuite) TestUpdate() {
	group := &service.Group{
		Name:             "original",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	group.Name = "updated"
	err := s.repo.Update(s.ctx, group)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
}

func (s *GroupRepoSuite) TestGetByID_PreservesMessagesDispatchModelConfig() {
	group := &service.Group{
		Name:                  "openai-dispatch",
		Platform:              service.PlatformOpenAI,
		RateMultiplier:        1.0,
		IsExclusive:           false,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		},
	}

	s.Require().NoError(s.repo.Create(s.ctx, group))

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Equal(group.MessagesDispatchModelConfig, got.MessagesDispatchModelConfig)
}

func (s *GroupRepoSuite) TestDelete() {
	group := &service.Group{
		Name:             "to-delete",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	err := s.repo.Delete(s.ctx, group.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().Error(err, "expected error after delete")
	s.Require().ErrorIs(err, service.ErrGroupNotFound)
}

// --- List / ListWithFilters ---

func (s *GroupRepoSuite) TestList() {
	baseGroups, basePage, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List base")

	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	groups, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(groups, len(baseGroups)+2)
	s.Require().Equal(basePage.Total+2, page.Total)
}

func (s *GroupRepoSuite) TestListWithFilters_Platform() {
	baseGroups, _, err := s.repo.ListWithFilters(
		s.ctx,
		pagination.PaginationParams{Page: 1, PageSize: 10},
		service.PlatformOpenAI,
		"",
		"",
		nil,
	)
	s.Require().NoError(err, "ListWithFilters base")

	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformOpenAI,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, service.PlatformOpenAI, "", "", nil)
	s.Require().NoError(err)
	s.Require().Len(groups, len(baseGroups)+1)
	// Verify all groups are OpenAI platform
	for _, g := range groups {
		s.Require().Equal(service.PlatformOpenAI, g.Platform)
	}
}

func (s *GroupRepoSuite) TestListWithFilters_Status() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusDisabled,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", service.StatusDisabled, "", nil)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().Equal(service.StatusDisabled, groups[0].Status)
}

func (s *GroupRepoSuite) TestListWithFilters_IsExclusive() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      true,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	isExclusive := true
	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "", &isExclusive)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().True(groups[0].IsExclusive)
}

func (s *GroupRepoSuite) TestListWithFilters_Search() {
	newRepo := func() (*groupRepository, context.Context) {
		tx := testEntTx(s.T())
		return newGroupRepositoryWithSQL(tx.Client(), tx), context.Background()
	}

	containsID := func(groups []service.Group, id int64) bool {
		for i := range groups {
			if groups[i].ID == id {
				return true
			}
		}
		return false
	}

	mustCreate := func(repo *groupRepository, ctx context.Context, g *service.Group) *service.Group {
		s.Require().NoError(repo.Create(ctx, g))
		s.Require().NotZero(g.ID)
		return g
	}

	newGroup := func(name string) *service.Group {
		return &service.Group{
			Name:             name,
			Platform:         service.PlatformAnthropic,
			RateMultiplier:   1.0,
			IsExclusive:      false,
			Status:           service.StatusActive,
			SubscriptionType: service.SubscriptionTypeStandard,
		}
	}

	s.Run("search_name_should_match", func() {
		repo, ctx := newRepo()

		target := mustCreate(repo, ctx, newGroup("it-group-search-name-target"))
		other := mustCreate(repo, ctx, newGroup("it-group-search-name-other"))

		groups, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, "", "", "name-target", nil)
		s.Require().NoError(err)
		s.Require().True(containsID(groups, target.ID), "expected target group to match by name")
		s.Require().False(containsID(groups, other.ID), "expected other group to be filtered out")
	})

	s.Run("search_description_should_match", func() {
		repo, ctx := newRepo()

		target := newGroup("it-group-search-desc-target")
		target.Description = "something about desc-needle in here"
		target = mustCreate(repo, ctx, target)

		other := newGroup("it-group-search-desc-other")
		other.Description = "nothing to see here"
		other = mustCreate(repo, ctx, other)

		groups, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, "", "", "desc-needle", nil)
		s.Require().NoError(err)
		s.Require().True(containsID(groups, target.ID), "expected target group to match by description")
		s.Require().False(containsID(groups, other.ID), "expected other group to be filtered out")
	})

	s.Run("search_nonexistent_should_return_empty", func() {
		repo, ctx := newRepo()

		_ = mustCreate(repo, ctx, newGroup("it-group-search-nonexistent-baseline"))

		search := s.T().Name() + "__no_such_group__"
		groups, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, "", "", search, nil)
		s.Require().NoError(err)
		s.Require().Empty(groups)
	})

	s.Run("search_should_be_case_insensitive", func() {
		repo, ctx := newRepo()

		target := mustCreate(repo, ctx, newGroup("MiXeDCaSe-Needle"))
		other := mustCreate(repo, ctx, newGroup("it-group-search-case-other"))

		groups, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, "", "", "mixedcase-needle", nil)
		s.Require().NoError(err)
		s.Require().True(containsID(groups, target.ID), "expected case-insensitive match")
		s.Require().False(containsID(groups, other.ID), "expected other group to be filtered out")
	})

	s.Run("search_should_escape_like_wildcards", func() {
		repo, ctx := newRepo()

		percentTarget := mustCreate(repo, ctx, newGroup("it-group-search-100%-target"))
		percentOther := mustCreate(repo, ctx, newGroup("it-group-search-100X-other"))

		groups, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, "", "", "100%", nil)
		s.Require().NoError(err)
		s.Require().True(containsID(groups, percentTarget.ID), "expected literal %% match")
		s.Require().False(containsID(groups, percentOther.ID), "expected %% not to act as wildcard")

		underscoreTarget := mustCreate(repo, ctx, newGroup("it-group-search-ab_cd-target"))
		underscoreOther := mustCreate(repo, ctx, newGroup("it-group-search-abXcd-other"))

		groups, _, err = repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, "", "", "ab_cd", nil)
		s.Require().NoError(err)
		s.Require().True(containsID(groups, underscoreTarget.ID), "expected literal _ match")
		s.Require().False(containsID(groups, underscoreOther.ID), "expected _ not to act as wildcard")
	})
}

func (s *GroupRepoSuite) TestUpdateSortOrders_BatchCaseWhen() {
	g1 := &service.Group{
		Name:             "sort-g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	g2 := &service.Group{
		Name:             "sort-g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	g3 := &service.Group{
		Name:             "sort-g3",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, g1))
	s.Require().NoError(s.repo.Create(s.ctx, g2))
	s.Require().NoError(s.repo.Create(s.ctx, g3))

	err := s.repo.UpdateSortOrders(s.ctx, []service.GroupSortOrderUpdate{
		{ID: g1.ID, SortOrder: 30},
		{ID: g2.ID, SortOrder: 10},
		{ID: g3.ID, SortOrder: 20},
		{ID: g2.ID, SortOrder: 15}, // 重复 ID 应以最后一次为准
	})
	s.Require().NoError(err)

	got1, err := s.repo.GetByID(s.ctx, g1.ID)
	s.Require().NoError(err)
	got2, err := s.repo.GetByID(s.ctx, g2.ID)
	s.Require().NoError(err)
	got3, err := s.repo.GetByID(s.ctx, g3.ID)
	s.Require().NoError(err)
	s.Require().Equal(30, got1.SortOrder)
	s.Require().Equal(15, got2.SortOrder)
	s.Require().Equal(20, got3.SortOrder)
}

func (s *GroupRepoSuite) TestUpdateSortOrders_MissingGroupNoPartialUpdate() {
	g1 := &service.Group{
		Name:             "sort-no-partial",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, g1))

	before, err := s.repo.GetByID(s.ctx, g1.ID)
	s.Require().NoError(err)
	beforeSort := before.SortOrder

	err = s.repo.UpdateSortOrders(s.ctx, []service.GroupSortOrderUpdate{
		{ID: g1.ID, SortOrder: 99},
		{ID: 99999999, SortOrder: 1},
	})
	s.Require().Error(err)
	s.Require().ErrorIs(err, service.ErrGroupNotFound)

	after, err := s.repo.GetByID(s.ctx, g1.ID)
	s.Require().NoError(err)
	s.Require().Equal(beforeSort, after.SortOrder)
}

func (s *GroupRepoSuite) TestListWithFilters_AccountCount() {
	g1 := &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	g2 := &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      true,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, g1))
	s.Require().NoError(s.repo.Create(s.ctx, g2))

	var accountID int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"acc1", service.PlatformAnthropic, service.AccountTypeOAuth},
		&accountID,
	))
	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", accountID, g1.ID, 1)
	s.Require().NoError(err)
	_, err = s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", accountID, g2.ID, 1)
	s.Require().NoError(err)

	isExclusive := true
	groups, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, service.PlatformAnthropic, service.StatusActive, "", &isExclusive)
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(groups, 1)
	s.Require().Equal(g2.ID, groups[0].ID, "ListWithFilters returned wrong group")
	s.Require().Equal(int64(1), groups[0].AccountCount, "AccountCount mismatch")
}

// --- ListActive / ListActiveByPlatform ---

func (s *GroupRepoSuite) TestListActive() {
	baseGroups, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive base")

	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "active1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "inactive1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusDisabled,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	groups, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(groups, len(baseGroups)+1)
	// Verify our test group is in the results
	var found bool
	for _, g := range groups {
		if g.Name == "active1" {
			found = true
			break
		}
	}
	s.Require().True(found, "active1 group should be in results")
}

func (s *GroupRepoSuite) TestListActiveByPlatform() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformOpenAI,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g3",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusDisabled,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	groups, err := s.repo.ListActiveByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err, "ListActiveByPlatform")
	// 1 default anthropic group + 1 test active anthropic group = 2 total
	s.Require().Len(groups, 2)
	// Verify our test group is in the results
	var found bool
	for _, g := range groups {
		if g.Name == "g1" {
			found = true
			break
		}
	}
	s.Require().True(found, "g1 group should be in results")
}

// --- ExistsByName ---

func (s *GroupRepoSuite) TestExistsByName() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "existing-group",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}))

	exists, err := s.repo.ExistsByName(s.ctx, "existing-group")
	s.Require().NoError(err, "ExistsByName")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByName(s.ctx, "non-existing")
	s.Require().NoError(err)
	s.Require().False(notExists)
}

// --- GetAccountCount ---

func (s *GroupRepoSuite) TestGetAccountCount() {
	group := &service.Group{
		Name:             "g-count",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	var a1 int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"a1", service.PlatformAnthropic, service.AccountTypeOAuth},
		&a1,
	))
	var a2 int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"a2", service.PlatformAnthropic, service.AccountTypeOAuth},
		&a2,
	))

	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a1, group.ID, 1)
	s.Require().NoError(err)
	_, err = s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a2, group.ID, 2)
	s.Require().NoError(err)

	count, _, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(2), count)
}

func (s *GroupRepoSuite) TestGetAccountCount_Empty() {
	group := &service.Group{
		Name:             "g-empty",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	count, _, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Zero(count)
}

// --- DeleteAccountGroupsByGroupID ---

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID() {
	g := &service.Group{
		Name:             "g-del",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, g))
	var accountID int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"acc-del", service.PlatformAnthropic, service.AccountTypeOAuth},
		&accountID,
	))
	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", accountID, g.ID, 1)
	s.Require().NoError(err)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err, "DeleteAccountGroupsByGroupID")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")

	count, _, err := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(0), count, "expected 0 account groups")
}

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID_MultipleAccounts() {
	g := &service.Group{
		Name:             "g-multi",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, g))

	insertAccount := func(name string) int64 {
		var id int64
		s.Require().NoError(scanSingleRow(
			s.ctx,
			s.tx,
			"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
			[]any{name, service.PlatformAnthropic, service.AccountTypeOAuth},
			&id,
		))
		return id
	}
	a1 := insertAccount("a1")
	a2 := insertAccount("a2")
	a3 := insertAccount("a3")
	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a1, g.ID, 1)
	s.Require().NoError(err)
	_, err = s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a2, g.ID, 2)
	s.Require().NoError(err)
	_, err = s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a3, g.ID, 3)
	s.Require().NoError(err)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), affected)

	count, _, _ := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().Zero(count)
}

// --- 软删除过滤测试 ---

func (s *GroupRepoSuite) TestDelete_SoftDelete_NotVisibleInList() {
	group := &service.Group{
		Name:             "to-soft-delete",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	// 获取删除前的列表数量
	listBefore, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 100})
	s.Require().NoError(err)
	beforeCount := len(listBefore)

	// 软删除
	err = s.repo.Delete(s.ctx, group.ID)
	s.Require().NoError(err, "Delete (soft delete)")

	// 验证列表中不再包含软删除的 group
	listAfter, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 100})
	s.Require().NoError(err)
	s.Require().Len(listAfter, beforeCount-1, "soft deleted group should not appear in list")

	// 验证 GetByID 也无法找到
	_, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().Error(err)
	s.Require().ErrorIs(err, service.ErrGroupNotFound)
}

func (s *GroupRepoSuite) TestDelete_SoftDeletedGroup_lockForUpdate() {
	group := &service.Group{
		Name:             "lock-soft-delete",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	// 软删除
	err := s.repo.Delete(s.ctx, group.ID)
	s.Require().NoError(err)

	// 验证软删除的 group 在 GetByID 时返回 ErrGroupNotFound
	// 这证明 lockForUpdate 的 deleted_at IS NULL 过滤正在工作
	_, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().Error(err, "should fail to get soft-deleted group")
	s.Require().ErrorIs(err, service.ErrGroupNotFound)
}
