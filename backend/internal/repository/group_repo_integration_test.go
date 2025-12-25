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

type GroupRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *GroupRepository
REDACTED

func (s *GroupRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewGroupRepository(s.db)
REDACTED

func TestGroupRepoSuite(t *testing.T) {
	suite.Run(t, new(GroupRepoSuite))
REDACTED

// --- Create / GetByID / Update / Delete ---

func (s *GroupRepoSuite) TestCreate() {
	group := &model.Group{
		Name:     "test-create",
		Platform: model.PlatformAnthropic,
		Status:   model.StatusActive,
REDACTED

	err := s.repo.Create(s.ctx, group)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(group.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
REDACTED

func (s *GroupRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
REDACTED

func (s *GroupRepoSuite) TestUpdate() {
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "original"REDACTED)

	group.Name = "updated"
	err := s.repo.Update(s.ctx, group)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
REDACTED

func (s *GroupRepoSuite) TestDelete() {
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "to-delete"REDACTED)

	err := s.repo.Delete(s.ctx, group.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().Error(err, "expected error after delete")
REDACTED

// --- List / ListWithFilters ---

func (s *GroupRepoSuite) TestList() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g1"REDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g2"REDACTED)

	groups, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "List")
	s.Require().Len(groups, 2)
	s.Require().Equal(int64(2), page.Total)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_Platform() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g1", Platform: model.PlatformAnthropicREDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g2", Platform: model.PlatformOpenAIREDACTED)

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, model.PlatformOpenAI, "", nil)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().Equal(model.PlatformOpenAI, groups[0].Platform)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_Status() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g1", Status: model.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g2", Status: model.StatusDisabledREDACTED)

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", model.StatusDisabled, nil)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().Equal(model.StatusDisabled, groups[0].Status)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_IsExclusive() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g1", IsExclusive: falseREDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g2", IsExclusive: trueREDACTED)

	isExclusive := true
	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", "", &isExclusive)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().True(groups[0].IsExclusive)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_AccountCount() {
	g1 := mustCreateGroup(s.T(), s.db, &model.Group{
		Name:     "g1",
		Platform: model.PlatformAnthropic,
		Status:   model.StatusActive,
REDACTED)
	g2 := mustCreateGroup(s.T(), s.db, &model.Group{
		Name:        "g2",
		Platform:    model.PlatformAnthropic,
		Status:      model.StatusActive,
		IsExclusive: true,
REDACTED)

	a := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc1"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a.ID, g1.ID, 1)
	mustBindAccountToGroup(s.T(), s.db, a.ID, g2.ID, 1)

	isExclusive := true
	groups, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, model.PlatformAnthropic, model.StatusActive, &isExclusive)
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(groups, 1)
	s.Require().Equal(g2.ID, groups[0].ID, "ListWithFilters returned wrong group")
	s.Require().Equal(int64(1), groups[0].AccountCount, "AccountCount mismatch")
REDACTED

// --- ListActive / ListActiveByPlatform ---

func (s *GroupRepoSuite) TestListActive() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "active1", Status: model.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "inactive1", Status: model.StatusDisabledREDACTED)

	groups, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(groups, 1)
	s.Require().Equal("active1", groups[0].Name)
REDACTED

func (s *GroupRepoSuite) TestListActiveByPlatform() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g1", Platform: model.PlatformAnthropic, Status: model.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g2", Platform: model.PlatformOpenAI, Status: model.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "g3", Platform: model.PlatformAnthropic, Status: model.StatusDisabledREDACTED)

	groups, err := s.repo.ListActiveByPlatform(s.ctx, model.PlatformAnthropic)
	s.Require().NoError(err, "ListActiveByPlatform")
	s.Require().Len(groups, 1)
	s.Require().Equal("g1", groups[0].Name)
REDACTED

// --- ExistsByName ---

func (s *GroupRepoSuite) TestExistsByName() {
	mustCreateGroup(s.T(), s.db, &model.Group{Name: "existing-group"REDACTED)

	exists, err := s.repo.ExistsByName(s.ctx, "existing-group")
	s.Require().NoError(err, "ExistsByName")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByName(s.ctx, "non-existing")
	s.Require().NoError(err)
	s.Require().False(notExists)
REDACTED

// --- GetAccountCount ---

func (s *GroupRepoSuite) TestGetAccountCount() {
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-count"REDACTED)
	a1 := mustCreateAccount(s.T(), s.db, &model.Account{Name: "a1"REDACTED)
	a2 := mustCreateAccount(s.T(), s.db, &model.Account{Name: "a2"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a1.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.db, a2.ID, group.ID, 2)

	count, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(2), count)
REDACTED

func (s *GroupRepoSuite) TestGetAccountCount_Empty() {
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-empty"REDACTED)

	count, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Zero(count)
REDACTED

// --- DeleteAccountGroupsByGroupID ---

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID() {
	g := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-del"REDACTED)
	a := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-del"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a.ID, g.ID, 1)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err, "DeleteAccountGroupsByGroupID")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")

	count, err := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(0), count, "expected 0 account groups")
REDACTED

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID_MultipleAccounts() {
	g := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-multi"REDACTED)
	a1 := mustCreateAccount(s.T(), s.db, &model.Account{Name: "a1"REDACTED)
	a2 := mustCreateAccount(s.T(), s.db, &model.Account{Name: "a2"REDACTED)
	a3 := mustCreateAccount(s.T(), s.db, &model.Account{Name: "a3"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a1.ID, g.ID, 1)
	mustBindAccountToGroup(s.T(), s.db, a2.ID, g.ID, 2)
	mustBindAccountToGroup(s.T(), s.db, a3.ID, g.ID, 3)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), affected)

	count, _ := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().Zero(count)
REDACTED

// --- DB ---

func (s *GroupRepoSuite) TestDB() {
	db := s.repo.DB()
	s.Require().NotNil(db, "DB should return non-nil")
	s.Require().Equal(s.db, db, "DB should return the underlying gorm.DB")
REDACTED
