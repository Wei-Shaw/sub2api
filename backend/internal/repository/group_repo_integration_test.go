//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type GroupRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *groupRepository
REDACTED

func (s *GroupRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewGroupRepository(s.db).(*groupRepository)
REDACTED

func TestGroupRepoSuite(t *testing.T) {
	suite.Run(t, new(GroupRepoSuite))
REDACTED

// --- Create / GetByID / Update / Delete ---

func (s *GroupRepoSuite) TestCreate() {
	group := &service.Group{
		Name:     "test-create",
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
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
	group := groupModelToService(mustCreateGroup(s.T(), s.db, &groupModel{Name: "original"REDACTED))

	group.Name = "updated"
	err := s.repo.Update(s.ctx, group)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
REDACTED

func (s *GroupRepoSuite) TestDelete() {
	group := mustCreateGroup(s.T(), s.db, &groupModel{Name: "to-delete"REDACTED)

	err := s.repo.Delete(s.ctx, group.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().Error(err, "expected error after delete")
REDACTED

// --- List / ListWithFilters ---

func (s *GroupRepoSuite) TestList() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g1"REDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g2"REDACTED)

	groups, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "List")
	// 3 default groups + 2 test groups = 5 total
	s.Require().Len(groups, 5)
	s.Require().Equal(int64(5), page.Total)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_Platform() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g1", Platform: service.PlatformAnthropicREDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g2", Platform: service.PlatformOpenAIREDACTED)

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, service.PlatformOpenAI, "", nil)
	s.Require().NoError(err)
	// 1 default openai group + 1 test openai group = 2 total
	s.Require().Len(groups, 2)
	// Verify all groups are OpenAI platform
	for _, g := range groups {
		s.Require().Equal(service.PlatformOpenAI, g.Platform)
REDACTED
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_Status() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g1", Status: service.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g2", Status: service.StatusDisabledREDACTED)

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", service.StatusDisabled, nil)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().Equal(service.StatusDisabled, groups[0].Status)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_IsExclusive() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g1", IsExclusive: falseREDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g2", IsExclusive: trueREDACTED)

	isExclusive := true
	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", "", &isExclusive)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().True(groups[0].IsExclusive)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_AccountCount() {
	g1 := mustCreateGroup(s.T(), s.db, &groupModel{
		Name:     "g1",
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
REDACTED)
	g2 := mustCreateGroup(s.T(), s.db, &groupModel{
		Name:        "g2",
		Platform:    service.PlatformAnthropic,
		Status:      service.StatusActive,
		IsExclusive: true,
REDACTED)

	a := mustCreateAccount(s.T(), s.db, &accountModel{Name: "acc1"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a.ID, g1.ID, 1)
	mustBindAccountToGroup(s.T(), s.db, a.ID, g2.ID, 1)

	isExclusive := true
	groups, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, service.PlatformAnthropic, service.StatusActive, &isExclusive)
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(groups, 1)
	s.Require().Equal(g2.ID, groups[0].ID, "ListWithFilters returned wrong group")
	s.Require().Equal(int64(1), groups[0].AccountCount, "AccountCount mismatch")
REDACTED

// --- ListActive / ListActiveByPlatform ---

func (s *GroupRepoSuite) TestListActive() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "active1", Status: service.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "inactive1", Status: service.StatusDisabledREDACTED)

	groups, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	// 3 default groups (all active) + 1 test active group = 4 total
	s.Require().Len(groups, 4)
	// Verify our test group is in the results
	var found bool
	for _, g := range groups {
		if g.Name == "active1" {
			found = true
			break
	REDACTED
REDACTED
	s.Require().True(found, "active1 group should be in results")
REDACTED

func (s *GroupRepoSuite) TestListActiveByPlatform() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g1", Platform: service.PlatformAnthropic, Status: service.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g2", Platform: service.PlatformOpenAI, Status: service.StatusActiveREDACTED)
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "g3", Platform: service.PlatformAnthropic, Status: service.StatusDisabledREDACTED)

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
	REDACTED
REDACTED
	s.Require().True(found, "g1 group should be in results")
REDACTED

// --- ExistsByName ---

func (s *GroupRepoSuite) TestExistsByName() {
	mustCreateGroup(s.T(), s.db, &groupModel{Name: "existing-group"REDACTED)

	exists, err := s.repo.ExistsByName(s.ctx, "existing-group")
	s.Require().NoError(err, "ExistsByName")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByName(s.ctx, "non-existing")
	s.Require().NoError(err)
	s.Require().False(notExists)
REDACTED

// --- GetAccountCount ---

func (s *GroupRepoSuite) TestGetAccountCount() {
	group := mustCreateGroup(s.T(), s.db, &groupModel{Name: "g-count"REDACTED)
	a1 := mustCreateAccount(s.T(), s.db, &accountModel{Name: "a1"REDACTED)
	a2 := mustCreateAccount(s.T(), s.db, &accountModel{Name: "a2"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a1.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.db, a2.ID, group.ID, 2)

	count, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(2), count)
REDACTED

func (s *GroupRepoSuite) TestGetAccountCount_Empty() {
	group := mustCreateGroup(s.T(), s.db, &groupModel{Name: "g-empty"REDACTED)

	count, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Zero(count)
REDACTED

// --- DeleteAccountGroupsByGroupID ---

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID() {
	g := mustCreateGroup(s.T(), s.db, &groupModel{Name: "g-del"REDACTED)
	a := mustCreateAccount(s.T(), s.db, &accountModel{Name: "acc-del"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a.ID, g.ID, 1)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err, "DeleteAccountGroupsByGroupID")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")

	count, err := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(0), count, "expected 0 account groups")
REDACTED

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID_MultipleAccounts() {
	g := mustCreateGroup(s.T(), s.db, &groupModel{Name: "g-multi"REDACTED)
	a1 := mustCreateAccount(s.T(), s.db, &accountModel{Name: "a1"REDACTED)
	a2 := mustCreateAccount(s.T(), s.db, &accountModel{Name: "a2"REDACTED)
	a3 := mustCreateAccount(s.T(), s.db, &accountModel{Name: "a3"REDACTED)
	mustBindAccountToGroup(s.T(), s.db, a1.ID, g.ID, 1)
	mustBindAccountToGroup(s.T(), s.db, a2.ID, g.ID, 2)
	mustBindAccountToGroup(s.T(), s.db, a3.ID, g.ID, 3)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), affected)

	count, _ := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().Zero(count)
REDACTED
