//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type RedeemCodeRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *RedeemCodeRepository
}

func (s *RedeemCodeRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewRedeemCodeRepository(s.db)
}

func TestRedeemCodeRepoSuite(t *testing.T) {
	suite.Run(t, new(RedeemCodeRepoSuite))
}

// --- Create / CreateBatch / GetByID / GetByCode ---

func (s *RedeemCodeRepoSuite) TestCreate() {
	code := &model.RedeemCode{
		Code:   "TEST-CREATE",
		Type:   model.RedeemTypeBalance,
		Value:  100,
		Status: model.StatusUnused,
	}

	err := s.repo.Create(s.ctx, code)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(code.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, code.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("TEST-CREATE", got.Code)
}

func (s *RedeemCodeRepoSuite) TestCreateBatch() {
	codes := []model.RedeemCode{
		{Code: "BATCH-1", Type: model.RedeemTypeBalance, Value: 10, Status: model.StatusUnused},
		{Code: "BATCH-2", Type: model.RedeemTypeBalance, Value: 20, Status: model.StatusUnused},
	}

	err := s.repo.CreateBatch(s.ctx, codes)
	s.Require().NoError(err, "CreateBatch")

	got1, err := s.repo.GetByCode(s.ctx, "BATCH-1")
	s.Require().NoError(err)
	s.Require().Equal(float64(10), got1.Value)

	got2, err := s.repo.GetByCode(s.ctx, "BATCH-2")
	s.Require().NoError(err)
	s.Require().Equal(float64(20), got2.Value)
}

func (s *RedeemCodeRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *RedeemCodeRepoSuite) TestGetByCode() {
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "GET-BY-CODE", Type: model.RedeemTypeBalance})

	got, err := s.repo.GetByCode(s.ctx, "GET-BY-CODE")
	s.Require().NoError(err, "GetByCode")
	s.Require().Equal("GET-BY-CODE", got.Code)
}

func (s *RedeemCodeRepoSuite) TestGetByCode_NotFound() {
	_, err := s.repo.GetByCode(s.ctx, "NON-EXISTENT")
	s.Require().Error(err, "expected error for non-existent code")
}

// --- Delete ---

func (s *RedeemCodeRepoSuite) TestDelete() {
	code := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "TO-DELETE", Type: model.RedeemTypeBalance})

	err := s.repo.Delete(s.ctx, code.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, code.ID)
	s.Require().Error(err, "expected error after delete")
}

// --- List / ListWithFilters ---

func (s *RedeemCodeRepoSuite) TestList() {
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "LIST-1", Type: model.RedeemTypeBalance})
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "LIST-2", Type: model.RedeemTypeBalance})

	codes, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(codes, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_Type() {
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "TYPE-BAL", Type: model.RedeemTypeBalance})
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "TYPE-SUB", Type: model.RedeemTypeSubscription})

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, model.RedeemTypeSubscription, "", "")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().Equal(model.RedeemTypeSubscription, codes[0].Type)
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_Status() {
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "STAT-UNUSED", Type: model.RedeemTypeBalance, Status: model.StatusUnused})
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "STAT-USED", Type: model.RedeemTypeBalance, Status: model.StatusUsed})

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", model.StatusUsed, "")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().Equal(model.StatusUsed, codes[0].Status)
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_Search() {
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "ALPHA-CODE", Type: model.RedeemTypeBalance})
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "BETA-CODE", Type: model.RedeemTypeBalance})

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "alpha")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().Contains(codes[0].Code, "ALPHA")
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_GroupPreload() {
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-preload"})
	mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{
		Code:    "WITH-GROUP",
		Type:    model.RedeemTypeSubscription,
		GroupID: &group.ID,
	})

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().NotNil(codes[0].Group, "expected Group preload")
	s.Require().Equal(group.ID, codes[0].Group.ID)
}

// --- Update ---

func (s *RedeemCodeRepoSuite) TestUpdate() {
	code := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "UPDATE-ME", Type: model.RedeemTypeBalance, Value: 10})

	code.Value = 50
	err := s.repo.Update(s.ctx, code)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, code.ID)
	s.Require().NoError(err)
	s.Require().Equal(float64(50), got.Value)
}

// --- Use ---

func (s *RedeemCodeRepoSuite) TestUse() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "use@test.com"})
	code := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "USE-ME", Type: model.RedeemTypeBalance, Status: model.StatusUnused})

	err := s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().NoError(err, "Use")

	got, err := s.repo.GetByID(s.ctx, code.ID)
	s.Require().NoError(err)
	s.Require().Equal(model.StatusUsed, got.Status)
	s.Require().NotNil(got.UsedBy)
	s.Require().Equal(user.ID, *got.UsedBy)
	s.Require().NotNil(got.UsedAt)
}

func (s *RedeemCodeRepoSuite) TestUse_Idempotency() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "idem@test.com"})
	code := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "IDEM-CODE", Type: model.RedeemTypeBalance, Status: model.StatusUnused})

	err := s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().NoError(err, "Use first time")

	// Second use should fail
	err = s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().Error(err, "Use expected error on second call")
	s.Require().ErrorIs(err, gorm.ErrRecordNotFound)
}

func (s *RedeemCodeRepoSuite) TestUse_AlreadyUsed() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "already@test.com"})
	code := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{Code: "ALREADY-USED", Type: model.RedeemTypeBalance, Status: model.StatusUsed})

	err := s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().Error(err, "expected error for already used code")
	s.Require().ErrorIs(err, gorm.ErrRecordNotFound)
}

// --- ListByUser ---

func (s *RedeemCodeRepoSuite) TestListByUser() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listby@test.com"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create codes with explicit used_at for ordering
	c1 := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{
		Code:   "USER-1",
		Type:   model.RedeemTypeBalance,
		Status: model.StatusUsed,
		UsedBy: &user.ID,
	})
	s.db.Model(c1).Update("used_at", base)

	c2 := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{
		Code:   "USER-2",
		Type:   model.RedeemTypeBalance,
		Status: model.StatusUsed,
		UsedBy: &user.ID,
	})
	s.db.Model(c2).Update("used_at", base.Add(1*time.Hour))

	codes, err := s.repo.ListByUser(s.ctx, user.ID, 10)
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(codes, 2)
	// Ordered by used_at DESC, so USER-2 first
	s.Require().Equal("USER-2", codes[0].Code)
	s.Require().Equal("USER-1", codes[1].Code)
}

func (s *RedeemCodeRepoSuite) TestListByUser_WithGroupPreload() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "grp@test.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-listby"})

	c := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{
		Code:    "WITH-GRP",
		Type:    model.RedeemTypeSubscription,
		Status:  model.StatusUsed,
		UsedBy:  &user.ID,
		GroupID: &group.ID,
	})
	s.db.Model(c).Update("used_at", time.Now())

	codes, err := s.repo.ListByUser(s.ctx, user.ID, 10)
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().NotNil(codes[0].Group)
	s.Require().Equal(group.ID, codes[0].Group.ID)
}

func (s *RedeemCodeRepoSuite) TestListByUser_DefaultLimit() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "deflimit@test.com"})
	c := mustCreateRedeemCode(s.T(), s.db, &model.RedeemCode{
		Code:   "DEF-LIM",
		Type:   model.RedeemTypeBalance,
		Status: model.StatusUsed,
		UsedBy: &user.ID,
	})
	s.db.Model(c).Update("used_at", time.Now())

	// limit <= 0 should default to 10
	codes, err := s.repo.ListByUser(s.ctx, user.ID, 0)
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
}

// --- Combined original test ---

func (s *RedeemCodeRepoSuite) TestCreateBatch_Filters_Use_Idempotency_ListByUser() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "rc@example.com"})
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-rc"})

	codes := []model.RedeemCode{
		{Code: "CODEA", Type: model.RedeemTypeBalance, Value: 1, Status: model.StatusUnused, CreatedAt: time.Now()},
		{Code: "CODEB", Type: model.RedeemTypeSubscription, Value: 0, Status: model.StatusUnused, GroupID: &group.ID, ValidityDays: 7, CreatedAt: time.Now()},
	}
	s.Require().NoError(s.repo.CreateBatch(s.ctx, codes), "CreateBatch")

	list, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, model.RedeemTypeSubscription, model.StatusUnused, "code")
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(list, 1)
	s.Require().NotNil(list[0].Group, "expected Group preload")
	s.Require().Equal(group.ID, list[0].Group.ID)

	codeB, err := s.repo.GetByCode(s.ctx, "CODEB")
	s.Require().NoError(err, "GetByCode")
	s.Require().NoError(s.repo.Use(s.ctx, codeB.ID, user.ID), "Use")
	err = s.repo.Use(s.ctx, codeB.ID, user.ID)
	s.Require().Error(err, "Use expected error on second call")
	s.Require().ErrorIs(err, gorm.ErrRecordNotFound)

	codeA, err := s.repo.GetByCode(s.ctx, "CODEA")
	s.Require().NoError(err, "GetByCode")

	// Use fixed time instead of time.Sleep for deterministic ordering
	s.db.Model(&model.RedeemCode{}).Where("id = ?", codeB.ID).Update("used_at", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	s.Require().NoError(s.repo.Use(s.ctx, codeA.ID, user.ID), "Use codeA")
	s.db.Model(&model.RedeemCode{}).Where("id = ?", codeA.ID).Update("used_at", time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC))

	used, err := s.repo.ListByUser(s.ctx, user.ID, 10)
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(used, 2, "expected 2 used codes")
	s.Require().Equal("CODEA", used[0].Code, "expected newest used code first")
}
