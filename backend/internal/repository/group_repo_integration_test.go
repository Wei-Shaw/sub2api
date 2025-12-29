//go:build integration

package repository

import (
	"context"
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
REDACTED

func (s *GroupRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.repo = newGroupRepositoryWithSQL(tx.Client(), tx)
REDACTED

func TestGroupRepoSuite(t *testing.T) {
	suite.Run(t, new(GroupRepoSuite))
REDACTED

// --- Create / GetByID / Update / Delete ---

func (s *GroupRepoSuite) TestCreate() {
	group := &service.Group{
		Name:             "test-create",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
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
	s.Require().ErrorIs(err, service.ErrGroupNotFound)
REDACTED

func (s *GroupRepoSuite) TestUpdate() {
	group := &service.Group{
		Name:             "original",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, group))

	group.Name = "updated"
	err := s.repo.Update(s.ctx, group)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
REDACTED

func (s *GroupRepoSuite) TestDelete() {
	group := &service.Group{
		Name:             "to-delete",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, group))

	err := s.repo.Delete(s.ctx, group.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().Error(err, "expected error after delete")
	s.Require().ErrorIs(err, service.ErrGroupNotFound)
REDACTED

// --- List / ListWithFilters ---

func (s *GroupRepoSuite) TestList() {
	baseGroups, basePage, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "List base")

	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

	groups, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "List")
	s.Require().Len(groups, len(baseGroups)+2)
	s.Require().Equal(basePage.Total+2, page.Total)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_Platform() {
	baseGroups, _, err := s.repo.ListWithFilters(
		s.ctx,
		pagination.PaginationParams{Page: 1, PageSize: 10REDACTED,
		service.PlatformOpenAI,
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
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformOpenAI,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, service.PlatformOpenAI, "", nil)
	s.Require().NoError(err)
	s.Require().Len(groups, len(baseGroups)+1)
	// Verify all groups are OpenAI platform
	for _, g := range groups {
		s.Require().Equal(service.PlatformOpenAI, g.Platform)
REDACTED
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_Status() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusDisabled,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", service.StatusDisabled, nil)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().Equal(service.StatusDisabled, groups[0].Status)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_IsExclusive() {
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      true,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

	isExclusive := true
	groups, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", "", &isExclusive)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().True(groups[0].IsExclusive)
REDACTED

func (s *GroupRepoSuite) TestListWithFilters_AccountCount() {
	g1 := &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	g2 := &service.Group{
		Name:             "g2",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      true,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, g1))
	s.Require().NoError(s.repo.Create(s.ctx, g2))

	var accountID int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"acc1", service.PlatformAnthropic, service.AccountTypeOAuthREDACTED,
		&accountID,
	))
	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", accountID, g1.ID, 1)
	s.Require().NoError(err)
	_, err = s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", accountID, g2.ID, 1)
	s.Require().NoError(err)

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
	baseGroups, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive base")

	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "active1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "inactive1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusDisabled,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

	groups, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(groups, len(baseGroups)+1)
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
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g1",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g2",
		Platform:         service.PlatformOpenAI,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "g3",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusDisabled,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

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
	s.Require().NoError(s.repo.Create(s.ctx, &service.Group{
		Name:             "existing-group",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED))

	exists, err := s.repo.ExistsByName(s.ctx, "existing-group")
	s.Require().NoError(err, "ExistsByName")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByName(s.ctx, "non-existing")
	s.Require().NoError(err)
	s.Require().False(notExists)
REDACTED

// --- GetAccountCount ---

func (s *GroupRepoSuite) TestGetAccountCount() {
	group := &service.Group{
		Name:             "g-count",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, group))

	var a1 int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"a1", service.PlatformAnthropic, service.AccountTypeOAuthREDACTED,
		&a1,
	))
	var a2 int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"a2", service.PlatformAnthropic, service.AccountTypeOAuthREDACTED,
		&a2,
	))

	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a1, group.ID, 1)
	s.Require().NoError(err)
	_, err = s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", a2, group.ID, 2)
	s.Require().NoError(err)

	count, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(2), count)
REDACTED

func (s *GroupRepoSuite) TestGetAccountCount_Empty() {
	group := &service.Group{
		Name:             "g-empty",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, group))

	count, err := s.repo.GetAccountCount(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Zero(count)
REDACTED

// --- DeleteAccountGroupsByGroupID ---

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID() {
	g := &service.Group{
		Name:             "g-del",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, g))
	var accountID int64
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.tx,
		"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
		[]any{"acc-del", service.PlatformAnthropic, service.AccountTypeOAuthREDACTED,
		&accountID,
	))
	_, err := s.tx.ExecContext(s.ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, $3, NOW())", accountID, g.ID, 1)
	s.Require().NoError(err)

	affected, err := s.repo.DeleteAccountGroupsByGroupID(s.ctx, g.ID)
	s.Require().NoError(err, "DeleteAccountGroupsByGroupID")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")

	count, err := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().NoError(err, "GetAccountCount")
	s.Require().Equal(int64(0), count, "expected 0 account groups")
REDACTED

func (s *GroupRepoSuite) TestDeleteAccountGroupsByGroupID_MultipleAccounts() {
	g := &service.Group{
		Name:             "g-multi",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1.0,
		IsExclusive:      false,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
REDACTED
	s.Require().NoError(s.repo.Create(s.ctx, g))

	insertAccount := func(name string) int64 {
		var id int64
		s.Require().NoError(scanSingleRow(
			s.ctx,
			s.tx,
			"INSERT INTO accounts (name, platform, type) VALUES ($1, $2, $3) RETURNING id",
			[]any{name, service.PlatformAnthropic, service.AccountTypeOAuthREDACTED,
			&id,
		))
		return id
REDACTED
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

	count, _ := s.repo.GetAccountCount(s.ctx, g.ID)
	s.Require().Zero(count)
REDACTED
