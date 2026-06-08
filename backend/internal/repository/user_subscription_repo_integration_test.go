//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type UserSubscriptionRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *userSubscriptionRepository
}

func (s *UserSubscriptionRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = NewUserSubscriptionRepository(s.client).(*userSubscriptionRepository)
}

func TestUserSubscriptionRepoSuite(t *testing.T) {
	suite.Run(t, new(UserSubscriptionRepoSuite))
}

func (s *UserSubscriptionRepoSuite) mustCreateUser(email string, role string) *service.User {
	s.T().Helper()

	if role == "" {
		role = service.RoleUser
	}

	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(role).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return userEntityToService(u)
}

func (s *UserSubscriptionRepoSuite) mustCreateGroup(name string) *service.Group {
	s.T().Helper()

	g, err := s.client.Group.Create().
		SetName(name).
		SetStatus(service.StatusActive).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return groupEntityToService(g)
}

func (s *UserSubscriptionRepoSuite) mustCreateSubscription(userID, groupID int64, mutate func(*dbent.UserSubscriptionCreate)) *dbent.UserSubscription {
	s.T().Helper()

	now := time.Now()
	create := s.client.UserSubscription.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		SetStartsAt(now.Add(-1 * time.Hour)).
		SetExpiresAt(now.Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(now).
		SetNotes("")

	if mutate != nil {
		mutate(create)
	}

	sub, err := create.Save(s.ctx)
	s.Require().NoError(err, "create user subscription")
	return sub
}

// --- Create / GetByID / Update / Delete ---

func (s *UserSubscriptionRepoSuite) TestCreate() {
	user := s.mustCreateUser("sub-create@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-create")

	sub := &service.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    service.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := s.repo.Create(s.ctx, sub)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(sub.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(sub.UserID, got.UserID)
	s.Require().Equal(sub.GroupID, got.GroupID)
}

func (s *UserSubscriptionRepoSuite) TestGetByID_WithPreloads() {
	user := s.mustCreateUser("preload@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-preload")
	admin := s.mustCreateUser("admin@test.com", service.RoleAdmin)

	sub := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetAssignedBy(admin.ID)
	})

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().NotNil(got.User, "expected User preload")
	s.Require().NotNil(got.Group, "expected Group preload")
	s.Require().NotNil(got.AssignedByUser, "expected AssignedByUser preload")
	s.Require().Equal(user.ID, got.User.ID)
	s.Require().Equal(group.ID, got.Group.ID)
	s.Require().Equal(admin.ID, got.AssignedByUser.ID)
}

func (s *UserSubscriptionRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *UserSubscriptionRepoSuite) TestUpdate() {
	user := s.mustCreateUser("update@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-update")
	created := s.mustCreateSubscription(user.ID, group.ID, nil)

	sub, err := s.repo.GetByID(s.ctx, created.ID)
	s.Require().NoError(err, "GetByID")

	sub.Notes = "updated notes"
	s.Require().NoError(s.repo.Update(s.ctx, sub), "Update")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated notes", got.Notes)
}

func (s *UserSubscriptionRepoSuite) TestDelete() {
	user := s.mustCreateUser("delete@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-delete")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	err := s.repo.Delete(s.ctx, sub.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, sub.ID)
	s.Require().Error(err, "expected error after delete")
}

func (s *UserSubscriptionRepoSuite) TestDelete_Idempotent() {
	s.Require().NoError(s.repo.Delete(s.ctx, 42424242), "Delete should be idempotent")
}

// --- GetByUserIDAndGroupID / GetActiveByUserIDAndGroupID ---

func (s *UserSubscriptionRepoSuite) TestGetByUserIDAndGroupID() {
	user := s.mustCreateUser("byuser@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-byuser")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	got, err := s.repo.GetByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "GetByUserIDAndGroupID")
	s.Require().Equal(sub.ID, got.ID)
	s.Require().NotNil(got.Group, "expected Group preload")
}

func (s *UserSubscriptionRepoSuite) TestGetByUserIDAndGroupID_NotFound() {
	_, err := s.repo.GetByUserIDAndGroupID(s.ctx, 999999, 999999)
	s.Require().Error(err, "expected error for non-existent pair")
}

func (s *UserSubscriptionRepoSuite) TestGetActiveByUserIDAndGroupID() {
	user := s.mustCreateUser("active@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-active")

	active := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(2 * time.Hour))
	})

	got, err := s.repo.GetActiveByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "GetActiveByUserIDAndGroupID")
	s.Require().Equal(active.ID, got.ID)
}

func (s *UserSubscriptionRepoSuite) TestGetActiveByUserIDAndGroupID_ExpiredIgnored() {
	user := s.mustCreateUser("expired@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-expired")

	s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(-2 * time.Hour))
	})

	_, err := s.repo.GetActiveByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().Error(err, "expected error for expired subscription")
}

// --- ListByUserID / ListActiveByUserID ---

func (s *UserSubscriptionRepoSuite) TestListByUserID() {
	user := s.mustCreateUser("listby@test.com", service.RoleUser)
	g1 := s.mustCreateGroup("g-list1")
	g2 := s.mustCreateGroup("g-list2")

	s.mustCreateSubscription(user.ID, g1.ID, nil)
	s.mustCreateSubscription(user.ID, g2.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetStatus(service.SubscriptionStatusExpired)
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour))
	})

	subs, err := s.repo.ListByUserID(s.ctx, user.ID)
	s.Require().NoError(err, "ListByUserID")
	s.Require().Len(subs, 2)
	for _, sub := range subs {
		s.Require().NotNil(sub.Group, "expected Group preload")
	}
}

func (s *UserSubscriptionRepoSuite) TestListActiveByUserID() {
	user := s.mustCreateUser("listactive@test.com", service.RoleUser)
	g1 := s.mustCreateGroup("g-act1")
	g2 := s.mustCreateGroup("g-act2")

	s.mustCreateSubscription(user.ID, g1.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(24 * time.Hour))
	})
	s.mustCreateSubscription(user.ID, g2.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetStatus(service.SubscriptionStatusExpired)
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour))
	})

	subs, err := s.repo.ListActiveByUserID(s.ctx, user.ID)
	s.Require().NoError(err, "ListActiveByUserID")
	s.Require().Len(subs, 1)
	s.Require().Equal(service.SubscriptionStatusActive, subs[0].Status)
}

// --- ListByGroupID ---

func (s *UserSubscriptionRepoSuite) TestListByGroupID() {
	user1 := s.mustCreateUser("u1@test.com", service.RoleUser)
	user2 := s.mustCreateUser("u2@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-listgrp")

	s.mustCreateSubscription(user1.ID, group.ID, nil)
	s.mustCreateSubscription(user2.ID, group.ID, nil)

	subs, page, err := s.repo.ListByGroupID(s.ctx, group.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByGroupID")
	s.Require().Len(subs, 2)
	s.Require().Equal(int64(2), page.Total)
	for _, sub := range subs {
		s.Require().NotNil(sub.User, "expected User preload")
		s.Require().NotNil(sub.Group, "expected Group preload")
	}
}

// --- List with filters ---

func (s *UserSubscriptionRepoSuite) TestList_NoFilters() {
	user := s.mustCreateUser("list@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-list")
	s.mustCreateSubscription(user.ID, group.ID, nil)

	subs, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, nil, nil, "", "", "", "")
	s.Require().NoError(err, "List")
	s.Require().Len(subs, 1)
	s.Require().Equal(int64(1), page.Total)
}

func (s *UserSubscriptionRepoSuite) TestList_FilterByUserID() {
	user1 := s.mustCreateUser("filter1@test.com", service.RoleUser)
	user2 := s.mustCreateUser("filter2@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-filter")

	s.mustCreateSubscription(user1.ID, group.ID, nil)
	s.mustCreateSubscription(user2.ID, group.ID, nil)

	subs, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, &user1.ID, nil, "", "", "", "")
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	s.Require().Equal(user1.ID, subs[0].UserID)
}

func (s *UserSubscriptionRepoSuite) TestList_FilterByGroupID() {
	user := s.mustCreateUser("grpfilter@test.com", service.RoleUser)
	g1 := s.mustCreateGroup("g-f1")
	g2 := s.mustCreateGroup("g-f2")

	s.mustCreateSubscription(user.ID, g1.ID, nil)
	s.mustCreateSubscription(user.ID, g2.ID, nil)

	subs, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, nil, &g1.ID, "", "", "", "")
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	s.Require().Equal(g1.ID, subs[0].GroupID)
}

func (s *UserSubscriptionRepoSuite) TestList_FilterByStatus() {
	user1 := s.mustCreateUser("statfilter1@test.com", service.RoleUser)
	user2 := s.mustCreateUser("statfilter2@test.com", service.RoleUser)
	group1 := s.mustCreateGroup("g-stat-1")
	group2 := s.mustCreateGroup("g-stat-2")

	s.mustCreateSubscription(user1.ID, group1.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetStatus(service.SubscriptionStatusActive)
		c.SetExpiresAt(time.Now().Add(24 * time.Hour))
	})
	s.mustCreateSubscription(user2.ID, group2.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetStatus(service.SubscriptionStatusExpired)
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour))
	})

	subs, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, nil, nil, service.SubscriptionStatusExpired, "", "", "")
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	s.Require().Equal(service.SubscriptionStatusExpired, subs[0].Status)
}

// --- Usage tracking ---

func (s *UserSubscriptionRepoSuite) TestIncrementUsage() {
	user := s.mustCreateUser("usage@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-usage")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	err := s.repo.IncrementUsage(s.ctx, sub.ID, 1.25)
	s.Require().NoError(err, "IncrementUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().InDelta(1.25, got.DailyUsageUSD, 1e-6)
	s.Require().InDelta(1.25, got.WeeklyUsageUSD, 1e-6)
	s.Require().InDelta(1.25, got.MonthlyUsageUSD, 1e-6)
}

func (s *UserSubscriptionRepoSuite) TestIncrementUsage_Accumulates() {
	user := s.mustCreateUser("accum@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-accum")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	s.Require().NoError(s.repo.IncrementUsage(s.ctx, sub.ID, 1.0))
	s.Require().NoError(s.repo.IncrementUsage(s.ctx, sub.ID, 2.5))

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().InDelta(3.5, got.DailyUsageUSD, 1e-6)
}

func (s *UserSubscriptionRepoSuite) TestActivateWindows() {
	user := s.mustCreateUser("activate@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-activate")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	activateAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	err := s.repo.ActivateWindows(s.ctx, sub.ID, activateAt)
	s.Require().NoError(err, "ActivateWindows")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.DailyWindowStart)
	s.Require().NotNil(got.WeeklyWindowStart)
	s.Require().NotNil(got.MonthlyWindowStart)
	s.Require().WithinDuration(activateAt, *got.DailyWindowStart, time.Microsecond)
}

func (s *UserSubscriptionRepoSuite) TestResetDailyUsage() {
	user := s.mustCreateUser("resetd@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-resetd")
	sub := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetDailyUsageUsd(10.0)
		c.SetWeeklyUsageUsd(20.0)
	})

	resetAt := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	err := s.repo.ResetDailyUsage(s.ctx, sub.ID, resetAt)
	s.Require().NoError(err, "ResetDailyUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().InDelta(0.0, got.DailyUsageUSD, 1e-6)
	s.Require().InDelta(20.0, got.WeeklyUsageUSD, 1e-6)
	s.Require().NotNil(got.DailyWindowStart)
	s.Require().WithinDuration(resetAt, *got.DailyWindowStart, time.Microsecond)
}

func (s *UserSubscriptionRepoSuite) TestResetWeeklyUsage() {
	user := s.mustCreateUser("resetw@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-resetw")
	sub := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetWeeklyUsageUsd(15.0)
		c.SetMonthlyUsageUsd(30.0)
	})

	resetAt := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	err := s.repo.ResetWeeklyUsage(s.ctx, sub.ID, resetAt)
	s.Require().NoError(err, "ResetWeeklyUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().InDelta(0.0, got.WeeklyUsageUSD, 1e-6)
	s.Require().InDelta(30.0, got.MonthlyUsageUSD, 1e-6)
	s.Require().NotNil(got.WeeklyWindowStart)
	s.Require().WithinDuration(resetAt, *got.WeeklyWindowStart, time.Microsecond)
}

func (s *UserSubscriptionRepoSuite) TestResetMonthlyUsage() {
	user := s.mustCreateUser("resetm@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-resetm")
	sub := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetMonthlyUsageUsd(25.0)
	})

	resetAt := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	err := s.repo.ResetMonthlyUsage(s.ctx, sub.ID, resetAt)
	s.Require().NoError(err, "ResetMonthlyUsage")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().InDelta(0.0, got.MonthlyUsageUSD, 1e-6)
	s.Require().NotNil(got.MonthlyWindowStart)
	s.Require().WithinDuration(resetAt, *got.MonthlyWindowStart, time.Microsecond)
}

// --- UpdateStatus / ExtendExpiry / UpdateNotes ---

func (s *UserSubscriptionRepoSuite) TestUpdateStatus() {
	user := s.mustCreateUser("status@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-status")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	err := s.repo.UpdateStatus(s.ctx, sub.ID, service.SubscriptionStatusExpired)
	s.Require().NoError(err, "UpdateStatus")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.SubscriptionStatusExpired, got.Status)
}

func (s *UserSubscriptionRepoSuite) TestExtendExpiry() {
	user := s.mustCreateUser("extend@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-extend")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	newExpiry := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err := s.repo.ExtendExpiry(s.ctx, sub.ID, newExpiry)
	s.Require().NoError(err, "ExtendExpiry")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().WithinDuration(newExpiry, got.ExpiresAt, time.Microsecond)
}

func (s *UserSubscriptionRepoSuite) TestUpdateNotes() {
	user := s.mustCreateUser("notes@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-notes")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	err := s.repo.UpdateNotes(s.ctx, sub.ID, "VIP user")
	s.Require().NoError(err, "UpdateNotes")

	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal("VIP user", got.Notes)
}

// --- ListExpired / BatchUpdateExpiredStatus ---

func (s *UserSubscriptionRepoSuite) TestListExpired() {
	user := s.mustCreateUser("listexp@test.com", service.RoleUser)
	groupActive := s.mustCreateGroup("g-listexp-active")
	groupExpired := s.mustCreateGroup("g-listexp-expired")

	s.mustCreateSubscription(user.ID, groupActive.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(24 * time.Hour))
	})
	s.mustCreateSubscription(user.ID, groupExpired.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour))
	})

	expired, err := s.repo.ListExpired(s.ctx)
	s.Require().NoError(err, "ListExpired")
	s.Require().Len(expired, 1)
}

func (s *UserSubscriptionRepoSuite) TestBatchUpdateExpiredStatus() {
	user := s.mustCreateUser("batch@test.com", service.RoleUser)
	groupFuture := s.mustCreateGroup("g-batch-future")
	groupPast := s.mustCreateGroup("g-batch-past")

	active := s.mustCreateSubscription(user.ID, groupFuture.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(24 * time.Hour))
	})
	expiredActive := s.mustCreateSubscription(user.ID, groupPast.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour))
	})

	affected, err := s.repo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err, "BatchUpdateExpiredStatus")
	s.Require().Equal(int64(1), affected)

	gotActive, _ := s.repo.GetByID(s.ctx, active.ID)
	s.Require().Equal(service.SubscriptionStatusActive, gotActive.Status)

	gotExpired, _ := s.repo.GetByID(s.ctx, expiredActive.ID)
	s.Require().Equal(service.SubscriptionStatusExpired, gotExpired.Status)
}

// --- ExistsByUserIDAndGroupID ---

func (s *UserSubscriptionRepoSuite) TestExistsByUserIDAndGroupID() {
	user := s.mustCreateUser("exists@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-exists")

	s.mustCreateSubscription(user.ID, group.ID, nil)

	exists, err := s.repo.ExistsByUserIDAndGroupID(s.ctx, user.ID, group.ID)
	s.Require().NoError(err, "ExistsByUserIDAndGroupID")
	s.Require().True(exists)

	notExists, err := s.repo.ExistsByUserIDAndGroupID(s.ctx, user.ID, 999999)
	s.Require().NoError(err)
	s.Require().False(notExists)
}

// --- CountByGroupID / CountActiveByGroupID ---

func (s *UserSubscriptionRepoSuite) TestCountByGroupID() {
	user1 := s.mustCreateUser("cnt1@test.com", service.RoleUser)
	user2 := s.mustCreateUser("cnt2@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-count")

	s.mustCreateSubscription(user1.ID, group.ID, nil)
	s.mustCreateSubscription(user2.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetStatus(service.SubscriptionStatusExpired)
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour))
	})

	count, err := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountByGroupID")
	s.Require().Equal(int64(2), count)
}

func (s *UserSubscriptionRepoSuite) TestCountActiveByGroupID() {
	user1 := s.mustCreateUser("cntact1@test.com", service.RoleUser)
	user2 := s.mustCreateUser("cntact2@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-cntact")

	s.mustCreateSubscription(user1.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(24 * time.Hour))
	})
	s.mustCreateSubscription(user2.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(-24 * time.Hour)) // expired by time
	})

	count, err := s.repo.CountActiveByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "CountActiveByGroupID")
	s.Require().Equal(int64(1), count, "only future expiry counts as active")
}

// --- DeleteByGroupID ---

func (s *UserSubscriptionRepoSuite) TestDeleteByGroupID() {
	user1 := s.mustCreateUser("delgrp1@test.com", service.RoleUser)
	user2 := s.mustCreateUser("delgrp2@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-delgrp")

	s.mustCreateSubscription(user1.ID, group.ID, nil)
	s.mustCreateSubscription(user2.ID, group.ID, nil)

	affected, err := s.repo.DeleteByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "DeleteByGroupID")
	s.Require().Equal(int64(2), affected)

	count, _ := s.repo.CountByGroupID(s.ctx, group.ID)
	s.Require().Zero(count)
}

// --- Combined scenario ---

func (s *UserSubscriptionRepoSuite) TestActiveExpiredBoundaries_UsageAndReset_BatchUpdateExpiredStatus() {
	user := s.mustCreateUser("subr@example.com", service.RoleUser)
	groupActive := s.mustCreateGroup("g-subr-active")
	groupExpired := s.mustCreateGroup("g-subr-expired")

	active := s.mustCreateSubscription(user.ID, groupActive.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(2 * time.Hour))
	})
	expiredActive := s.mustCreateSubscription(user.ID, groupExpired.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetExpiresAt(time.Now().Add(-2 * time.Hour))
	})

	got, err := s.repo.GetActiveByUserIDAndGroupID(s.ctx, user.ID, groupActive.ID)
	s.Require().NoError(err, "GetActiveByUserIDAndGroupID")
	s.Require().Equal(active.ID, got.ID, "expected active subscription")

	activateAt := time.Now().Add(-25 * time.Hour)
	s.Require().NoError(s.repo.ActivateWindows(s.ctx, active.ID, activateAt), "ActivateWindows")
	s.Require().NoError(s.repo.IncrementUsage(s.ctx, active.ID, 1.25), "IncrementUsage")

	after, err := s.repo.GetByID(s.ctx, active.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().InDelta(1.25, after.DailyUsageUSD, 1e-6)
	s.Require().InDelta(1.25, after.WeeklyUsageUSD, 1e-6)
	s.Require().InDelta(1.25, after.MonthlyUsageUSD, 1e-6)
	s.Require().NotNil(after.DailyWindowStart, "expected DailyWindowStart activated")
	s.Require().NotNil(after.WeeklyWindowStart, "expected WeeklyWindowStart activated")
	s.Require().NotNil(after.MonthlyWindowStart, "expected MonthlyWindowStart activated")

	resetAt := time.Now().Truncate(time.Microsecond) // truncate to microsecond for DB precision
	s.Require().NoError(s.repo.ResetDailyUsage(s.ctx, active.ID, resetAt), "ResetDailyUsage")
	afterReset, err := s.repo.GetByID(s.ctx, active.ID)
	s.Require().NoError(err, "GetByID after reset")
	s.Require().InDelta(0.0, afterReset.DailyUsageUSD, 1e-6)
	s.Require().NotNil(afterReset.DailyWindowStart)
	s.Require().WithinDuration(resetAt, *afterReset.DailyWindowStart, time.Microsecond)

	affected, err := s.repo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err, "BatchUpdateExpiredStatus")
	s.Require().Equal(int64(1), affected, "expected 1 affected row")

	updated, err := s.repo.GetByID(s.ctx, expiredActive.ID)
	s.Require().NoError(err, "GetByID expired")
	s.Require().Equal(service.SubscriptionStatusExpired, updated.Status, "expected status expired")
}

// --- 软删除过滤测试 ---

func (s *UserSubscriptionRepoSuite) TestIncrementUsage_SoftDeletedGroup() {
	user := s.mustCreateUser("softdeleted@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-softdeleted")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	// 软删除分组
	_, err := s.client.Group.UpdateOneID(group.ID).SetDeletedAt(time.Now()).Save(s.ctx)
	s.Require().NoError(err, "soft delete group")

	// IncrementUsage 应该失败，因为分组已软删除
	err = s.repo.IncrementUsage(s.ctx, sub.ID, 1.0)
	s.Require().Error(err, "should fail for soft-deleted group")
	s.Require().ErrorIs(err, service.ErrSubscriptionNotFound)
}

func (s *UserSubscriptionRepoSuite) TestIncrementUsage_NotFound() {
	err := s.repo.IncrementUsage(s.ctx, 999999, 1.0)
	s.Require().Error(err, "should fail for non-existent subscription")
	s.Require().ErrorIs(err, service.ErrSubscriptionNotFound)
}

// --- nil 入参测试 ---

func (s *UserSubscriptionRepoSuite) TestCreate_NilInput() {
	err := s.repo.Create(s.ctx, nil)
	s.Require().Error(err, "Create should fail with nil input")
	s.Require().ErrorIs(err, service.ErrSubscriptionNilInput)
}

func (s *UserSubscriptionRepoSuite) TestUpdate_NilInput() {
	err := s.repo.Update(s.ctx, nil)
	s.Require().Error(err, "Update should fail with nil input")
	s.Require().ErrorIs(err, service.ErrSubscriptionNilInput)
}

// --- 并发用量更新测试 ---

func (s *UserSubscriptionRepoSuite) TestIncrementUsage_Concurrent() {
	user := s.mustCreateUser("concurrent@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-concurrent")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)

	const numGoroutines = 10
	const incrementPerGoroutine = 1.5

	// 启动多个 goroutine 并发调用 IncrementUsage
	errCh := make(chan error, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			errCh <- s.repo.IncrementUsage(s.ctx, sub.ID, incrementPerGoroutine)
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		err := <-errCh
		s.Require().NoError(err, "IncrementUsage should succeed")
	}

	// 验证累加结果正确
	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	expectedUsage := float64(numGoroutines) * incrementPerGoroutine
	s.Require().InDelta(expectedUsage, got.DailyUsageUSD, 1e-6, "daily usage should be correctly accumulated")
	s.Require().InDelta(expectedUsage, got.WeeklyUsageUSD, 1e-6, "weekly usage should be correctly accumulated")
	s.Require().InDelta(expectedUsage, got.MonthlyUsageUSD, 1e-6, "monthly usage should be correctly accumulated")
}

func (s *UserSubscriptionRepoSuite) TestTxContext_RollbackIsolation() {
	baseClient := testEntClient(s.T())
	tx, err := baseClient.Tx(context.Background())
	s.Require().NoError(err, "begin tx")
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	txCtx := dbent.NewTxContext(context.Background(), tx)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	userEnt, err := tx.Client().User.Create().
		SetEmail("tx-user-" + suffix + "@example.com").
		SetPasswordHash("test").
		Save(txCtx)
	s.Require().NoError(err, "create user in tx")

	groupEnt, err := tx.Client().Group.Create().
		SetName("tx-group-" + suffix).
		Save(txCtx)
	s.Require().NoError(err, "create group in tx")

	repo := NewUserSubscriptionRepository(baseClient)
	sub := &service.UserSubscription{
		UserID:     userEnt.ID,
		GroupID:    groupEnt.ID,
		ExpiresAt:  time.Now().AddDate(0, 0, 30),
		Status:     service.SubscriptionStatusActive,
		AssignedAt: time.Now(),
		Notes:      "tx",
	}
	s.Require().NoError(repo.Create(txCtx, sub), "create subscription in tx")
	s.Require().NoError(repo.UpdateNotes(txCtx, sub.ID, "tx-note"), "update subscription in tx")

	s.Require().NoError(tx.Rollback(), "rollback tx")
	tx = nil

	_, err = repo.GetByID(context.Background(), sub.ID)
	s.Require().ErrorIs(err, service.ErrSubscriptionNotFound)
}
