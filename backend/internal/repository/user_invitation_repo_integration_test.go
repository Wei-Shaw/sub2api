//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type UserInvitationRepoSuite struct {
	suite.Suite
	ctx        context.Context
	client     *dbent.Client
	repo       *userInvitationRepository
	redeemRepo *redeemCodeRepository
}

func (s *UserInvitationRepoSuite) SetupTest() {
	tx := testEntTx(s.T())
	s.ctx = dbent.NewTxContext(context.Background(), tx)
	s.client = tx.Client()
	s.repo = NewUserInvitationRepository(s.client).(*userInvitationRepository)
	s.redeemRepo = NewRedeemCodeRepository(s.client).(*redeemCodeRepository)
}

func TestUserInvitationRepoSuite(t *testing.T) {
	suite.Run(t, new(UserInvitationRepoSuite))
}

func (s *UserInvitationRepoSuite) createUser(email string) *dbent.User {
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return u
}

func (s *UserInvitationRepoSuite) newCode(code string, creatorID int64, expiresAt *time.Time) *service.RedeemCode {
	return &service.RedeemCode{
		Code:      code,
		Type:      service.RedeemTypeInvitation,
		Status:    service.StatusUnused,
		CreatedBy: &creatorID,
		ExpiresAt: expiresAt,
	}
}

func (s *UserInvitationRepoSuite) TestCreateEnforcesLimitAndPersistsCreator() {
	inviter := s.createUser("inviter@example.com")
	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)

	first := s.newCode("USER-INV-1", inviter.ID, &expiresAt)
	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, first, 2, now))
	s.Require().NotZero(first.ID)
	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, s.newCode("USER-INV-2", inviter.ID, &expiresAt), 2, now))

	err := s.repo.CreateInvitationForUser(s.ctx, s.newCode("USER-INV-3", inviter.ID, &expiresAt), 2, now)
	s.Require().ErrorIs(err, service.ErrUserInvitationLimitReached)

	stored, err := s.redeemRepo.GetByCode(s.ctx, "USER-INV-1")
	s.Require().NoError(err)
	s.Require().NotNil(stored.CreatedBy)
	s.Require().Equal(inviter.ID, *stored.CreatedBy)
	s.Require().Equal(service.RedeemTypeInvitation, stored.Type)

	// maxCodes <= 0 表示不限
	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, s.newCode("USER-INV-4", inviter.ID, &expiresAt), 0, now))
}

func (s *UserInvitationRepoSuite) TestCountExcludesExpiredUnusedButKeepsUsed() {
	inviter := s.createUser("counter@example.com")
	invitee := s.createUser("invitee@example.com")
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, s.newCode("CNT-ACTIVE", inviter.ID, &future), 0, now))
	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, s.newCode("CNT-NOEXP", inviter.ID, nil), 0, now))
	expired := s.newCode("CNT-EXPIRED", inviter.ID, &past)
	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, expired, 0, now))
	usedThenExpired := s.newCode("CNT-USED", inviter.ID, &past)
	s.Require().NoError(s.repo.CreateInvitationForUser(s.ctx, usedThenExpired, 0, now))
	s.Require().NoError(s.redeemRepo.Use(s.ctx, usedThenExpired.ID, invitee.ID))

	// 管理员生成的邀请码（created_by 为空）不计入任何用户名额
	s.Require().NoError(s.redeemRepo.Create(s.ctx, &service.RedeemCode{
		Code: "CNT-ADMIN", Type: service.RedeemTypeInvitation, Status: service.StatusUnused,
	}))

	count, err := s.repo.CountActiveInvitationsByCreator(s.ctx, inviter.ID, now)
	s.Require().NoError(err)
	s.Require().Equal(3, count, "active + never-expiring + used; expired-unused frees its slot")

	codes, err := s.repo.ListInvitationsByCreator(s.ctx, inviter.ID, 100)
	s.Require().NoError(err)
	s.Require().Len(codes, 4)
	var used *service.RedeemCode
	for i := range codes {
		if codes[i].Code == "CNT-USED" {
			used = &codes[i]
		}
	}
	s.Require().NotNil(used)
	s.Require().NotNil(used.User, "used code should load invitee")
	s.Require().Equal("invitee@example.com", used.User.Email)
}

func (s *UserInvitationRepoSuite) TestCreateRejectsUnknownCreator() {
	err := s.repo.CreateInvitationForUser(s.ctx, s.newCode("NO-USER", 99999999, nil), 5, time.Now())
	s.Require().ErrorIs(err, service.ErrUserNotFound)
}
