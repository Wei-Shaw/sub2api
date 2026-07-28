//go:build integration

package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *UserRepoSuite) TestCreate_CreatesEmailAuthIdentityForNormalEmail() {
	user := &service.User{
		Email:        "repo-create@example.com",
		PasswordHash: "test-password-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  2,
REDACTED

	s.Require().NoError(s.repo.Create(s.ctx, user))

	identity, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("repo-create@example.com"),
		).
		Only(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(user.ID, identity.UserID)
REDACTED

func (s *UserRepoSuite) TestCreate_SkipsEmailAuthIdentityForSyntheticLinuxDoEmail() {
	user := &service.User{
		Email:        "linuxdo-legacy-user@linuxdo-connect.invalid",
		PasswordHash: "test-password-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  2,
REDACTED

	s.Require().NoError(s.repo.Create(s.ctx, user))

	count, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
		).
		Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Zero(count)
REDACTED

func (s *UserRepoSuite) TestUpdate_ReplacesEmailAuthIdentityWhenEmailChanges() {
	user := s.mustCreateUser(&service.User{
		Email: "before-update@example.com",
REDACTED)

	user.Email = "after-update@example.com"
	s.Require().NoError(s.repo.Update(s.ctx, user, service.UserUpdateFields{Email: trueREDACTED))

	newIdentity, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("after-update@example.com"),
		).
		Only(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(user.ID, newIdentity.UserID)

	oldCount, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("before-update@example.com"),
		).
		Count(context.Background())
	s.Require().NoError(err)
	s.Require().Zero(oldCount)
REDACTED
