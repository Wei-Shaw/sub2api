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

type UserSubscriptionRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *UserSubscriptionRepository
REDACTED

func (s *UserSubscriptionRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewUserSubscriptionRepository(s.db)
REDACTED

func TestUserSubscriptionRepoSuite(t *testing.T) {
	suite.Run(t, new(UserSubscriptionRepoSuite))
REDACTED

// --- Create / GetByID / Update / Delete ---

func (s *UserSubscriptionRepoSuite) TestCreate() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "sub-create@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-create"REDACTED)

	sub := &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED

	err := s.repo.Create(s.ctx, sub)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(sub.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(sub.UserID, got.UserID)
	s.Require().Equal(sub.GroupID, got.GroupID)
REDACTED

func (s *UserSubscriptionRepoSuite) TestGetByID_WithPreloads() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "preload@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-preload"REDACTED)
	admin := mustCreateUser(s.T(), s.db, &model.User{Email: "admin@test.com", Role: model.RoleAdminREDACTED)

	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:     user.ID,
		GroupID:    group.ID,
		Status:     model.SubscriptionStatusActive,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		AssignedBy: &admin.ID,
REDACTED)

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().NotNil(got.User, "expected User preload")
	s.Require().NotNil(got.Group, "expected Group preload")
	s.Require().NotNil(got.AssignedByUser, "expected AssignedByUser preload")
	s.Require().Equal(user.ID, got.User.ID)
	s.Require().Equal(group.ID, got.Group.ID)
	s.Require().Equal(admin.ID, got.AssignedByUser.ID)
REDACTED

func (s *UserSubscriptionRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
REDACTED

func (s *UserSubscriptionRepoSuite) TestUpdate() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "update@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-update"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	sub.Notes = "updated notes"
	err := s.repo.Update(s.ctx, sub)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated notes", got.Notes)
REDACTED

func (s *UserSubscriptionRepoSuite) TestDelete() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "delete@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-delete"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	err := s.repo.Delete(s.ctx, sub.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, sub.ID)
	s.Require().Error(err, "expected error after delete")
REDACTED

// --- GetByUserIDAndGroupID / GetActiveByUserIDAndGroupID ---

func (s *UserSubscriptionRepoSuite) TestGetByUserIDAndGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "byuser@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-byuser"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	got, err := s.repo.GetByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "GetByUserIDAndGroupID")
	s.Require().Equal(sub.ID, got.ID)
	s.Require().NotNil(got.Group, "expected Group preload")
REDACTED

func (s *UserSubscriptionRepoSuite) TestGetByUserIDAndGroupID_NotFound() {
	_, err := s.repo.GetByUserIDAndGroupID(s.ctx, 999999, 999999)
	s.Require().Error(err, "expected error for non-existent pair")
REDACTED

func (s *UserSubscriptionRepoSuite) TestGetActiveByUserIDAndGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "active@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-active"REDACTED)

	// Create active subscription (future expiry)
	active := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(2 * time.Hour),
REDACTED)

	got, err := s.repo.GetActiveByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "GetActiveByUserIDAndGroupID")
	s.Require().Equal(active.ID, got.ID)
REDACTED

func (s *UserSubscriptionRepoSuite) TestGetActiveByUserIDAndGroupID_ExpiredIgnored() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "expired@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-expired"REDACTED)

	// Create expired subscription (past expiry but active status)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(-2 * time.Hour),
REDACTED)

	_, err := s.repo.GetActiveByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().Error(err, "expected error for expired subscription")
REDACTED

// --- ListByUserID / ListActiveByUserID ---

func (s *UserSubscriptionRepoSuite) TestListByUserID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listby@test.com"REDACTED)
	g1 := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-list1"REDACTED)
	g2 := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-list2"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   g1.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   g2.ID,
		Status:    model.SubscriptionStatusExpired,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	subs, err := s.repo.ListByUserID(s.ctx, user.ID)
	s.Require().NoError(err, "ListByUserID")
	s.Require().Len(subs, 2)
	for _, sub := range subs {
		s.Require().NotNil(sub.Group, "expected Group preload")
REDACTED
REDACTED

func (s *UserSubscriptionRepoSuite) TestListActiveByUserID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listactive@test.com"REDACTED)
	g1 := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-act1"REDACTED)
	g2 := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-act2"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   g1.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   g2.ID,
		Status:    model.SubscriptionStatusExpired,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	subs, err := s.repo.ListActiveByUserID(s.ctx, user.ID)
	s.Require().NoError(err, "ListActiveByUserID")
	s.Require().Len(subs, 1)
	s.Require().Equal(model.SubscriptionStatusActive, subs[0].Status)
REDACTED

// --- ListByGroupID ---

func (s *UserSubscriptionRepoSuite) TestListByGroupID() {
	user1 := mustCreateUser(s.T(), s.db, &model.User{Email: "u1@test.com"REDACTED)
	user2 := mustCreateUser(s.T(), s.db, &model.User{Email: "u2@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-listgrp"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user1.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user2.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	subs, page, err := s.repo.ListByGroupID(s.ctx, group.ID, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "ListByGroupID")
	s.Require().Len(subs, 2)
	s.Require().Equal(int64(2), page.Total)
	for _, sub := range subs {
		s.Require().NotNil(sub.User, "expected User preload")
		s.Require().NotNil(sub.Group, "expected Group preload")
REDACTED
REDACTED

// --- List with filters ---

func (s *UserSubscriptionRepoSuite) TestList_NoFilters() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "list@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-list"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	subs, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, nil, nil, "")
	s.Require().NoError(err, "List")
	s.Require().Len(subs, 1)
	s.Require().Equal(int64(1), page.Total)
REDACTED

func (s *UserSubscriptionRepoSuite) TestList_FilterByUserID() {
	user1 := mustCreateUser(s.T(), s.db, &model.User{Email: "filter1@test.com"REDACTED)
	user2 := mustCreateUser(s.T(), s.db, &model.User{Email: "filter2@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-filter"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user1.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user2.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	subs, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, &user1.ID, nil, "")
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	s.Require().Equal(user1.ID, subs[0].UserID)
REDACTED

func (s *UserSubscriptionRepoSuite) TestList_FilterByGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "grpfilter@test.com"REDACTED)
	g1 := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-f1"REDACTED)
	g2 := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-f2"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   g1.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   g2.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	subs, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, nil, &g1.ID, "")
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	s.Require().Equal(g1.ID, subs[0].GroupID)
REDACTED

func (s *UserSubscriptionRepoSuite) TestList_FilterByStatus() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "statfilter@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-stat"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusExpired,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	subs, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, nil, nil, model.SubscriptionStatusExpired)
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	s.Require().Equal(model.SubscriptionStatusExpired, subs[0].Status)
REDACTED

// --- Usage tracking ---

func (s *UserSubscriptionRepoSuite) TestIncrementUsage() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "usage@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-usage"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	err := s.repo.IncrementUsage(s.ctx, sub.ID, 1.25)
	s.Require().NoError(err, "IncrementUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal(1.25, got.DailyUsageUSD)
	s.Require().Equal(1.25, got.WeeklyUsageUSD)
	s.Require().Equal(1.25, got.MonthlyUsageUSD)
REDACTED

func (s *UserSubscriptionRepoSuite) TestIncrementUsage_Accumulates() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "accum@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-accum"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	s.Require().NoError(s.repo.IncrementUsage(s.ctx, sub.ID, 1.0))
	s.Require().NoError(s.repo.IncrementUsage(s.ctx, sub.ID, 2.5))

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal(3.5, got.DailyUsageUSD)
REDACTED

func (s *UserSubscriptionRepoSuite) TestActivateWindows() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "activate@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-activate"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	activateAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	err := s.repo.ActivateWindows(s.ctx, sub.ID, activateAt)
	s.Require().NoError(err, "ActivateWindows")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.DailyWindowStart)
	s.Require().NotNil(got.WeeklyWindowStart)
	s.Require().NotNil(got.MonthlyWindowStart)
	s.Require().True(got.DailyWindowStart.Equal(activateAt))
REDACTED

func (s *UserSubscriptionRepoSuite) TestResetDailyUsage() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "resetd@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-resetd"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:         user.ID,
		GroupID:        group.ID,
		Status:         model.SubscriptionStatusActive,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		DailyUsageUSD:  10.0,
		WeeklyUsageUSD: 20.0,
REDACTED)

	resetAt := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	err := s.repo.ResetDailyUsage(s.ctx, sub.ID, resetAt)
	s.Require().NoError(err, "ResetDailyUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Zero(got.DailyUsageUSD)
	s.Require().Equal(20.0, got.WeeklyUsageUSD, "weekly should remain unchanged")
	s.Require().True(got.DailyWindowStart.Equal(resetAt))
REDACTED

func (s *UserSubscriptionRepoSuite) TestResetWeeklyUsage() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "resetw@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-resetw"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:          user.ID,
		GroupID:         group.ID,
		Status:          model.SubscriptionStatusActive,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		WeeklyUsageUSD:  15.0,
		MonthlyUsageUSD: 30.0,
REDACTED)

	resetAt := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	err := s.repo.ResetWeeklyUsage(s.ctx, sub.ID, resetAt)
	s.Require().NoError(err, "ResetWeeklyUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Zero(got.WeeklyUsageUSD)
	s.Require().Equal(30.0, got.MonthlyUsageUSD, "monthly should remain unchanged")
	s.Require().True(got.WeeklyWindowStart.Equal(resetAt))
REDACTED

func (s *UserSubscriptionRepoSuite) TestResetMonthlyUsage() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "resetm@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-resetm"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:          user.ID,
		GroupID:         group.ID,
		Status:          model.SubscriptionStatusActive,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		MonthlyUsageUSD: 100.0,
REDACTED)

	resetAt := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	err := s.repo.ResetMonthlyUsage(s.ctx, sub.ID, resetAt)
	s.Require().NoError(err, "ResetMonthlyUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Zero(got.MonthlyUsageUSD)
	s.Require().True(got.MonthlyWindowStart.Equal(resetAt))
REDACTED

// --- UpdateStatus / ExtendExpiry / UpdateNotes ---

func (s *UserSubscriptionRepoSuite) TestUpdateStatus() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "status@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-status"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	err := s.repo.UpdateStatus(s.ctx, sub.ID, model.SubscriptionStatusExpired)
	s.Require().NoError(err, "UpdateStatus")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal(model.SubscriptionStatusExpired, got.Status)
REDACTED

func (s *UserSubscriptionRepoSuite) TestExtendExpiry() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "extend@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-extend"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	newExpiry := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err := s.repo.ExtendExpiry(s.ctx, sub.ID, newExpiry)
	s.Require().NoError(err, "ExtendExpiry")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().True(got.ExpiresAt.Equal(newExpiry))
REDACTED

func (s *UserSubscriptionRepoSuite) TestUpdateNotes() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "notes@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-notes"REDACTED)
	sub := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	err := s.repo.UpdateNotes(s.ctx, sub.ID, "VIP user")
	s.Require().NoError(err, "UpdateNotes")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal("VIP user", got.Notes)
REDACTED

// --- ListExpired / BatchUpdateExpiredStatus ---

func (s *UserSubscriptionRepoSuite) TestListExpired() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listexp@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-listexp"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	expired, err := s.repo.ListExpired(s.ctx)
	s.Require().NoError(err, "ListExpired")
	s.Require().Len(expired, 1)
REDACTED

func (s *UserSubscriptionRepoSuite) TestBatchUpdateExpiredStatus() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "batch@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-batch"REDACTED)

	active := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	expiredActive := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	affected, err := s.repo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err, "BatchUpdateExpiredStatus")
	s.Require().Equal(int64(1), affected)

	gotActive, _ := s.repo.GetByID(s.ctx, active.ID)
	s.Require().Equal(model.SubscriptionStatusActive, gotActive.Status)

	gotExpired, _ := s.repo.GetByID(s.ctx, expiredActive.ID)
	s.Require().Equal(model.SubscriptionStatusExpired, gotExpired.Status)
REDACTED

// --- ExistsByUserIDAndGroupID ---

func (s *UserSubscriptionRepoSuite) TestExistsByUserIDAndGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "exists@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-exists"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)

	exists, err := s.repo.ExistsByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "ExistsByUserIDAndGroupID")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByUserIDAndGroupID(s.ctx, user.ID, 999999)
	s.Require().NoError(err)
	s.Require().False(notExists)
REDACTED

// --- CountByGroupID / CountActiveByGroupID ---

func (s *UserSubscriptionRepoSuite) TestCountByGroupID() {
	user1 := mustCreateUser(s.T(), s.db, &model.User{Email: "cnt1@test.com"REDACTED)
	user2 := mustCreateUser(s.T(), s.db, &model.User{Email: "cnt2@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-count"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user1.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user2.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusExpired,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	count, err := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountByGroupID")
	s.Require().Equal(int64(2), count)
REDACTED

func (s *UserSubscriptionRepoSuite) TestCountActiveByGroupID() {
	user1 := mustCreateUser(s.T(), s.db, &model.User{Email: "cntact1@test.com"REDACTED)
	user2 := mustCreateUser(s.T(), s.db, &model.User{Email: "cntact2@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-cntact"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user1.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user2.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(-24 * time.Hour), // expired by time
REDACTED)

	count, err := s.repo.CountActiveByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountActiveByGroupID")
	s.Require().Equal(int64(1), count, "only future expiry counts as active")
REDACTED

// --- DeleteByGroupID ---

func (s *UserSubscriptionRepoSuite) TestDeleteByGroupID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "delgrp@test.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-delgrp"REDACTED)

	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
REDACTED)
	mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusExpired,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
REDACTED)

	affected, err := s.repo.DeleteByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "DeleteByGroupID")
	s.Require().Equal(int64(2), affected)

	count, _ := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().Zero(count)
REDACTED

// --- Combined original test ---

func (s *UserSubscriptionRepoSuite) TestActiveExpiredBoundaries_UsageAndReset_BatchUpdateExpiredStatus() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "subr@example.com"REDACTED)
	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-subr"REDACTED)

	active := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(2 * time.Hour),
REDACTED)
	expiredActive := mustCreateSubscription(s.T(), s.db, &model.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    model.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(-2 * time.Hour),
REDACTED)

	got, err := s.repo.GetActiveByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "GetActiveByUserIDAndGroupID")
	s.Require().Equal(active.ID, got.ID, "expected active subscription")

	activateAt := time.Now().Add(-25 * time.Hour)
	s.Require().NoError(s.repo.ActivateWindows(s.ctx, active.ID, activateAt), "ActivateWindows")
	s.Require().NoError(s.repo.IncrementUsage(s.ctx, active.ID, 1.25), "IncrementUsage")

	after, err := s.repo.GetByID(s.ctx, active.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(1.25, after.DailyUsageUSD, "DailyUsageUSD mismatch")
	s.Require().Equal(1.25, after.WeeklyUsageUSD, "WeeklyUsageUSD mismatch")
	s.Require().Equal(1.25, after.MonthlyUsageUSD, "MonthlyUsageUSD mismatch")
	s.Require().NotNil(after.DailyWindowStart, "expected DailyWindowStart activated")
	s.Require().NotNil(after.WeeklyWindowStart, "expected WeeklyWindowStart activated")
	s.Require().NotNil(after.MonthlyWindowStart, "expected MonthlyWindowStart activated")

	resetAt := time.Now().Truncate(time.Microsecond) // truncate to microsecond for DB precision
	s.Require().NoError(s.repo.ResetDailyUsage(s.ctx, active.ID, resetAt), "ResetDailyUsage")
	afterReset, err := s.repo.GetByID(s.ctx, active.ID)
	s.Require().NoError(err, "GetByID after reset")
	s.Require().Equal(0.0, afterReset.DailyUsageUSD, "expected daily usage reset to 0")
	s.Require().NotNil(afterReset.DailyWindowStart, "expected DailyWindowStart not nil")
	s.Require().True(afterReset.DailyWindowStart.Equal(resetAt), "expected daily window start updated")

	affected, err := s.repo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err, "BatchUpdateExpiredStatus")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")
	updated, err := s.repo.GetByID(s.ctx, expiredActive.ID)
	s.Require().NoError(err, "GetByID expired")
	s.Require().Equal(model.SubscriptionStatusExpired, updated.Status, "expected status expired")
REDACTED
