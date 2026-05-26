//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type RedeemCodeRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *redeemCodeRepository
}

func (s *RedeemCodeRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = NewRedeemCodeRepository(s.client).(*redeemCodeRepository)
}

func TestRedeemCodeRepoSuite(t *testing.T) {
	suite.Run(t, new(RedeemCodeRepoSuite))
}

func (s *RedeemCodeRepoSuite) createUser(email string) *dbent.User {
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return u
}

func (s *RedeemCodeRepoSuite) createGroup(name string) *dbent.Group {
	g, err := s.client.Group.Create().
		SetName(name).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return g
}

// --- Create / CreateBatch / GetByID / GetByCode ---

func (s *RedeemCodeRepoSuite) TestCreate() {
	code := &service.RedeemCode{
		Code:   "TEST-CREATE",
		Type:   service.RedeemTypeBalance,
		Value:  100,
		Status: service.StatusUnused,
	}

	err := s.repo.Create(s.ctx, code)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(code.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, code.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("TEST-CREATE", got.Code)
}

func (s *RedeemCodeRepoSuite) TestCreateBatch() {
	codes := []service.RedeemCode{
		{Code: "BATCH-1", Type: service.RedeemTypeBalance, Value: 10, Status: service.StatusUnused},
		{Code: "BATCH-2", Type: service.RedeemTypeBalance, Value: 20, Status: service.StatusUnused},
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
	s.Require().ErrorIs(err, service.ErrRedeemCodeNotFound)
}

func (s *RedeemCodeRepoSuite) TestGetByCode() {
	_, err := s.client.RedeemCode.Create().
		SetCode("GET-BY-CODE").
		SetType(service.RedeemTypeBalance).
		SetStatus(service.StatusUnused).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		Save(s.ctx)
	s.Require().NoError(err, "seed redeem code")

	got, err := s.repo.GetByCode(s.ctx, "GET-BY-CODE")
	s.Require().NoError(err, "GetByCode")
	s.Require().Equal("GET-BY-CODE", got.Code)
}

func (s *RedeemCodeRepoSuite) TestGetByCode_NotFound() {
	_, err := s.repo.GetByCode(s.ctx, "NON-EXISTENT")
	s.Require().Error(err, "expected error for non-existent code")
	s.Require().ErrorIs(err, service.ErrRedeemCodeNotFound)
}

// --- Delete ---

func (s *RedeemCodeRepoSuite) TestDelete() {
	created, err := s.client.RedeemCode.Create().
		SetCode("TO-DELETE").
		SetType(service.RedeemTypeBalance).
		SetStatus(service.StatusUnused).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		Save(s.ctx)
	s.Require().NoError(err)

	err = s.repo.Delete(s.ctx, created.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, created.ID)
	s.Require().Error(err, "expected error after delete")
	s.Require().ErrorIs(err, service.ErrRedeemCodeNotFound)
}

// --- List / ListWithFilters ---

func (s *RedeemCodeRepoSuite) TestList() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "LIST-1", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "LIST-2", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}))

	codes, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(codes, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_Type() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "TYPE-BAL", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "TYPE-SUB", Type: service.RedeemTypeSubscription, Value: 0, Status: service.StatusUnused}))

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, service.RedeemTypeSubscription, "", "")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().Equal(service.RedeemTypeSubscription, codes[0].Type)
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_Status() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "STAT-UNUSED", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "STAT-USED", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUsed}))

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", service.StatusUsed, "")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().Equal(service.StatusUsed, codes[0].Status)
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_Search() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "ALPHA-CODE", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}))
	s.Require().NoError(s.repo.Create(s.ctx, &service.RedeemCode{Code: "BETA-CODE", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}))

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "alpha")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().Contains(codes[0].Code, "ALPHA")
}

func (s *RedeemCodeRepoSuite) TestListWithFilters_GroupPreload() {
	group := s.createGroup(uniqueTestValue(s.T(), "g-preload"))
	_, err := s.client.RedeemCode.Create().
		SetCode("WITH-GROUP").
		SetType(service.RedeemTypeSubscription).
		SetStatus(service.StatusUnused).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		SetGroupID(group.ID).
		Save(s.ctx)
	s.Require().NoError(err)

	codes, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().NotNil(codes[0].Group, "expected Group preload")
	s.Require().Equal(group.ID, codes[0].Group.ID)
}

// --- Update ---

func (s *RedeemCodeRepoSuite) TestUpdate() {
	code := &service.RedeemCode{
		Code:   "UPDATE-ME",
		Type:   service.RedeemTypeBalance,
		Value:  10,
		Status: service.StatusUnused,
	}
	s.Require().NoError(s.repo.Create(s.ctx, code))

	code.Value = 50
	err := s.repo.Update(s.ctx, code)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, code.ID)
	s.Require().NoError(err)
	s.Require().Equal(float64(50), got.Value)
}

// --- Use ---

func (s *RedeemCodeRepoSuite) TestUse() {
	user := s.createUser(uniqueTestValue(s.T(), "use") + "@example.com")
	code := &service.RedeemCode{Code: "USE-ME", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}
	s.Require().NoError(s.repo.Create(s.ctx, code))

	err := s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().NoError(err, "Use")

	got, err := s.repo.GetByID(s.ctx, code.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusUsed, got.Status)
	s.Require().NotNil(got.UsedBy)
	s.Require().Equal(user.ID, *got.UsedBy)
	s.Require().NotNil(got.UsedAt)
}

func (s *RedeemCodeRepoSuite) TestUse_Idempotency() {
	user := s.createUser(uniqueTestValue(s.T(), "idem") + "@example.com")
	code := &service.RedeemCode{Code: "IDEM-CODE", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUnused}
	s.Require().NoError(s.repo.Create(s.ctx, code))

	err := s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().NoError(err, "Use first time")

	// Second use should fail
	err = s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().Error(err, "Use expected error on second call")
	s.Require().ErrorIs(err, service.ErrRedeemCodeUsed)
}

func (s *RedeemCodeRepoSuite) TestUse_AlreadyUsed() {
	user := s.createUser(uniqueTestValue(s.T(), "already") + "@example.com")
	code := &service.RedeemCode{Code: "ALREADY-USED", Type: service.RedeemTypeBalance, Value: 0, Status: service.StatusUsed}
	s.Require().NoError(s.repo.Create(s.ctx, code))

	err := s.repo.Use(s.ctx, code.ID, user.ID)
	s.Require().Error(err, "expected error for already used code")
	s.Require().ErrorIs(err, service.ErrRedeemCodeUsed)
}

// --- ListByUser ---

func (s *RedeemCodeRepoSuite) TestListByUser() {
	user := s.createUser(uniqueTestValue(s.T(), "listby") + "@example.com")
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	usedAt1 := base
	_, err := s.client.RedeemCode.Create().
		SetCode("USER-1").
		SetType(service.RedeemTypeBalance).
		SetStatus(service.StatusUsed).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		SetUsedBy(user.ID).
		SetUsedAt(usedAt1).
		Save(s.ctx)
	s.Require().NoError(err)

	usedAt2 := base.Add(1 * time.Hour)
	_, err = s.client.RedeemCode.Create().
		SetCode("USER-2").
		SetType(service.RedeemTypeBalance).
		SetStatus(service.StatusUsed).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		SetUsedBy(user.ID).
		SetUsedAt(usedAt2).
		Save(s.ctx)
	s.Require().NoError(err)

	codes, err := s.repo.ListByUser(s.ctx, user.ID, 10)
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(codes, 2)
	// Ordered by used_at DESC, so USER-2 first
	s.Require().Equal("USER-2", codes[0].Code)
	s.Require().Equal("USER-1", codes[1].Code)
}

func (s *RedeemCodeRepoSuite) TestListByUser_WithGroupPreload() {
	user := s.createUser(uniqueTestValue(s.T(), "grp") + "@example.com")
	group := s.createGroup(uniqueTestValue(s.T(), "g-listby"))

	_, err := s.client.RedeemCode.Create().
		SetCode("WITH-GRP").
		SetType(service.RedeemTypeSubscription).
		SetStatus(service.StatusUsed).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		SetUsedBy(user.ID).
		SetUsedAt(time.Now()).
		SetGroupID(group.ID).
		Save(s.ctx)
	s.Require().NoError(err)

	codes, err := s.repo.ListByUser(s.ctx, user.ID, 10)
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
	s.Require().NotNil(codes[0].Group)
	s.Require().Equal(group.ID, codes[0].Group.ID)
}

func (s *RedeemCodeRepoSuite) TestListByUser_DefaultLimit() {
	user := s.createUser(uniqueTestValue(s.T(), "deflimit") + "@example.com")
	_, err := s.client.RedeemCode.Create().
		SetCode("DEF-LIM").
		SetType(service.RedeemTypeBalance).
		SetStatus(service.StatusUsed).
		SetValue(0).
		SetNotes("").
		SetValidityDays(30).
		SetUsedBy(user.ID).
		SetUsedAt(time.Now()).
		Save(s.ctx)
	s.Require().NoError(err)

	// limit <= 0 should default to 10
	codes, err := s.repo.ListByUser(s.ctx, user.ID, 0)
	s.Require().NoError(err)
	s.Require().Len(codes, 1)
}

// --- Combined original test ---

func (s *RedeemCodeRepoSuite) TestCreateBatch_Filters_Use_Idempotency_ListByUser() {
	user := s.createUser(uniqueTestValue(s.T(), "rc") + "@example.com")
	group := s.createGroup(uniqueTestValue(s.T(), "g-rc"))
	groupID := group.ID

	codes := []service.RedeemCode{
		{Code: "CODEA", Type: service.RedeemTypeBalance, Value: 1, Status: service.StatusUnused, Notes: ""},
		{Code: "CODEB", Type: service.RedeemTypeSubscription, Value: 0, Status: service.StatusUnused, Notes: "", GroupID: &groupID, ValidityDays: 7},
	}
	s.Require().NoError(s.repo.CreateBatch(s.ctx, codes), "CreateBatch")

	list, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, service.RedeemTypeSubscription, service.StatusUnused, "code")
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
	s.Require().ErrorIs(err, service.ErrRedeemCodeUsed)

	codeA, err := s.repo.GetByCode(s.ctx, "CODEA")
	s.Require().NoError(err, "GetByCode")

	// Use fixed time instead of time.Sleep for deterministic ordering.
	_, err = s.client.RedeemCode.UpdateOneID(codeB.ID).
		SetUsedAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)).
		Save(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.Use(s.ctx, codeA.ID, user.ID), "Use codeA")
	_, err = s.client.RedeemCode.UpdateOneID(codeA.ID).
		SetUsedAt(time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC)).
		Save(s.ctx)
	s.Require().NoError(err)

	used, err := s.repo.ListByUser(s.ctx, user.ID, 10)
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(used, 2, "expected 2 used codes")
	s.Require().Equal("CODEA", used[0].Code, "expected newest used code first")
}
