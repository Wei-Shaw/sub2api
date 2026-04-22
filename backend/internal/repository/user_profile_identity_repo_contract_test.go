//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type UserProfileIdentityRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *userRepository
}

func TestUserProfileIdentityRepoSuite(t *testing.T) {
	suite.Run(t, new(UserProfileIdentityRepoSuite))
}

func (s *UserProfileIdentityRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = testEntClient(s.T())
	s.repo = newUserRepositoryWithSQL(s.client, integrationDB)

	_, err := integrationDB.ExecContext(s.ctx, `
TRUNCATE TABLE
	identity_adoption_decisions,
	auth_identity_channels,
	auth_identities,
	pending_auth_sessions,
	user_provider_default_grants,
	user_avatars
RESTART IDENTITY`)
	s.Require().NoError(err)
}

func (s *UserProfileIdentityRepoSuite) mustCreateUser(label string) *dbent.User {
	s.T().Helper()

	user, err := s.client.User.Create().
		SetEmail(fmt.Sprintf("%s-%d@example.com", label, time.Now().UnixNano())).
		SetPasswordHash("test-password-hash").
		SetRole("user").
		SetStatus("active").
		Save(s.ctx)
	s.Require().NoError(err)
	return user
}

func (s *UserProfileIdentityRepoSuite) mustCreatePendingAuthSession(key AuthIdentityKey) *dbent.PendingAuthSession {
	s.T().Helper()

	session, err := s.client.PendingAuthSession.Create().
		SetSessionToken(fmt.Sprintf("pending-%d", time.Now().UnixNano())).
		SetIntent("bind_current_user").
		SetProviderType(key.ProviderType).
		SetProviderKey(key.ProviderKey).
		SetProviderSubject(key.ProviderSubject).
		SetExpiresAt(time.Now().UTC().Add(15 * time.Minute)).
		SetUpstreamIdentityClaims(map[string]any{"provider_subject": key.ProviderSubject}).
		SetLocalFlowState(map[string]any{"step": "pending"}).
		Save(s.ctx)
	s.Require().NoError(err)
	return session
}

func (s *UserProfileIdentityRepoSuite) TestCreateAndLookupCanonicalAndChannelIdentity() {
	user := s.mustCreateUser("canonical-channel")

	verifiedAt := time.Now().UTC().Truncate(time.Second)
	created, err := s.repo.CreateAuthIdentity(s.ctx, CreateAuthIdentityInput{
		UserID: user.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-123",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "wechat",
			ProviderKey:    "wechat-open",
			Channel:        "mp",
			ChannelAppID:   "wx-app",
			ChannelSubject: "openid-123",
		},
		Issuer:          stringPtr("https://issuer.example"),
		VerifiedAt:      &verifiedAt,
		Metadata:        map[string]any{"unionid": "union-123"},
		ChannelMetadata: map[string]any{"openid": "openid-123"},
	})
	s.Require().NoError(err)
	s.Require().NotNil(created.Identity)
	s.Require().NotNil(created.Channel)

	canonical, err := s.repo.GetUserByCanonicalIdentity(s.ctx, created.IdentityRef())
	s.Require().NoError(err)
	s.Require().Equal(user.ID, canonical.User.ID)
	s.Require().Equal(created.Identity.ID, canonical.Identity.ID)
	s.Require().Equal("union-123", canonical.Identity.ProviderSubject)

	channel, err := s.repo.GetUserByChannelIdentity(s.ctx, *created.ChannelRef())
	s.Require().NoError(err)
	s.Require().Equal(user.ID, channel.User.ID)
	s.Require().Equal(created.Identity.ID, channel.Identity.ID)
	s.Require().Equal(created.Channel.ID, channel.Channel.ID)
}

func (s *UserProfileIdentityRepoSuite) TestBindAuthIdentityToUser_IsIdempotentAndRejectsOtherOwners() {
	owner := s.mustCreateUser("owner")
	other := s.mustCreateUser("other")

	first, err := s.repo.BindAuthIdentityToUser(s.ctx, BindAuthIdentityInput{
		UserID: owner.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo-main",
			ProviderSubject: "subject-1",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "linuxdo",
			ProviderKey:    "linuxdo-main",
			Channel:        "oauth",
			ChannelAppID:   "linuxdo-web",
			ChannelSubject: "subject-1",
		},
		Metadata:        map[string]any{"username": "first"},
		ChannelMetadata: map[string]any{"scope": "read"},
	})
	s.Require().NoError(err)

	second, err := s.repo.BindAuthIdentityToUser(s.ctx, BindAuthIdentityInput{
		UserID: owner.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo-main",
			ProviderSubject: "subject-1",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "linuxdo",
			ProviderKey:    "linuxdo-main",
			Channel:        "oauth",
			ChannelAppID:   "linuxdo-web",
			ChannelSubject: "subject-1",
		},
		Metadata:        map[string]any{"username": "second"},
		ChannelMetadata: map[string]any{"scope": "write"},
	})
	s.Require().NoError(err)
	s.Require().Equal(first.Identity.ID, second.Identity.ID)
	s.Require().Equal(first.Channel.ID, second.Channel.ID)
	s.Require().Equal("second", second.Identity.Metadata["username"])
	s.Require().Equal("write", second.Channel.Metadata["scope"])

	_, err = s.repo.BindAuthIdentityToUser(s.ctx, BindAuthIdentityInput{
		UserID: other.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo-main",
			ProviderSubject: "subject-1",
		},
	})
	s.Require().ErrorIs(err, ErrAuthIdentityOwnershipConflict)

	_, err = s.repo.BindAuthIdentityToUser(s.ctx, BindAuthIdentityInput{
		UserID: other.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo-main",
			ProviderSubject: "subject-2",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "linuxdo",
			ProviderKey:    "linuxdo-main",
			Channel:        "oauth",
			ChannelAppID:   "linuxdo-web",
			ChannelSubject: "subject-1",
		},
	})
	s.Require().ErrorIs(err, ErrAuthIdentityChannelOwnershipConflict)
}

func (s *UserProfileIdentityRepoSuite) TestBindAuthIdentityToUser_ReusesLegacyWeChatAliasRecords() {
	user := s.mustCreateUser("wechat-legacy-alias")

	legacyIdentity, err := s.client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetProviderSubject("union-legacy-123").
		SetMetadata(map[string]any{"source": "legacy-alias"}).
		Save(s.ctx)
	s.Require().NoError(err)

	legacyChannel, err := s.client.AuthIdentityChannel.Create().
		SetIdentityID(legacyIdentity.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetChannel("oa").
		SetChannelAppID("wx-app-legacy").
		SetChannelSubject("openid-legacy-123").
		SetMetadata(map[string]any{"scene": "legacy-alias"}).
		Save(s.ctx)
	s.Require().NoError(err)

	bound, err := s.repo.BindAuthIdentityToUser(s.ctx, BindAuthIdentityInput{
		UserID: user.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-main",
			ProviderSubject: "union-legacy-123",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "wechat",
			ProviderKey:    "wechat-main",
			Channel:        "oa",
			ChannelAppID:   "wx-app-legacy",
			ChannelSubject: "openid-legacy-123",
		},
		Metadata:        map[string]any{"source": "canonical-bind"},
		ChannelMetadata: map[string]any{"scene": "canonical-bind"},
	})
	s.Require().NoError(err)
	s.Require().NotNil(bound)
	s.Require().NotNil(bound.Identity)
	s.Require().NotNil(bound.Channel)
	s.Require().Equal(legacyIdentity.ID, bound.Identity.ID)
	s.Require().Equal(legacyChannel.ID, bound.Channel.ID)
	s.Require().Equal("wechat-main", bound.Identity.ProviderKey)
	s.Require().Equal("wechat-main", bound.Channel.ProviderKey)
	s.Require().Equal("canonical-bind", bound.Identity.Metadata["source"])
	s.Require().Equal("canonical-bind", bound.Channel.Metadata["scene"])

	identityCount, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderSubjectEQ("union-legacy-123"),
		).
		Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(1, identityCount)

	channelCount, err := s.client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ("wechat"),
			authidentitychannel.ChannelEQ("oa"),
			authidentitychannel.ChannelAppIDEQ("wx-app-legacy"),
			authidentitychannel.ChannelSubjectEQ("openid-legacy-123"),
		).
		Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(1, channelCount)
}

func (s *UserProfileIdentityRepoSuite) TestCreateAuthIdentity_RejectsChannelProviderMismatch() {
	user := s.mustCreateUser("provider-mismatch-create")

	_, err := s.repo.CreateAuthIdentity(s.ctx, CreateAuthIdentityInput{
		UserID: user.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-main",
			ProviderSubject: "union-create-mismatch",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "linuxdo",
			ProviderKey:    "linuxdo-main",
			Channel:        "oauth",
			ChannelAppID:   "app-mismatch",
			ChannelSubject: "openid-create-mismatch",
		},
	})
	s.Require().ErrorIs(err, ErrAuthIdentityChannelProviderMismatch)
}

func (s *UserProfileIdentityRepoSuite) TestBindAuthIdentityToUser_RejectsChannelProviderMismatch() {
	user := s.mustCreateUser("provider-mismatch-bind")

	_, err := s.repo.BindAuthIdentityToUser(s.ctx, BindAuthIdentityInput{
		UserID: user.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-main",
			ProviderSubject: "union-bind-mismatch",
		},
		Channel: &AuthIdentityChannelKey{
			ProviderType:   "wechat",
			ProviderKey:    "wechat-legacy",
			Channel:        "oa",
			ChannelAppID:   "wx-app-bind-mismatch",
			ChannelSubject: "openid-bind-mismatch",
		},
	})
	s.Require().ErrorIs(err, ErrAuthIdentityChannelProviderMismatch)
}

func (s *UserProfileIdentityRepoSuite) TestWithUserProfileIdentityTx_RollsBackIdentityAndGrantOnError() {
	user := s.mustCreateUser("tx-rollback")
	expectedErr := errors.New("rollback")

	err := s.repo.WithUserProfileIdentityTx(s.ctx, func(txCtx context.Context) error {
		_, err := s.repo.CreateAuthIdentity(txCtx, CreateAuthIdentityInput{
			UserID: user.ID,
			Canonical: AuthIdentityKey{
				ProviderType:    "oidc",
				ProviderKey:     "https://issuer.example",
				ProviderSubject: "subject-rollback",
			},
		})
		s.Require().NoError(err)

		inserted, err := s.repo.RecordProviderGrant(txCtx, ProviderGrantRecordInput{
			UserID:       user.ID,
			ProviderType: "oidc",
			GrantReason:  ProviderGrantReasonFirstBind,
		})
		s.Require().NoError(err)
		s.Require().True(inserted)
		return expectedErr
	})
	s.Require().ErrorIs(err, expectedErr)

	_, err = s.repo.GetUserByCanonicalIdentity(s.ctx, AuthIdentityKey{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-rollback",
	})
	s.Require().True(dbent.IsNotFound(err))

	var count int
	s.Require().NoError(integrationDB.QueryRowContext(s.ctx, `
SELECT COUNT(*)
FROM user_provider_default_grants
WHERE user_id = $1 AND provider_type = $2 AND grant_reason = $3`,
		user.ID,
		"oidc",
		string(ProviderGrantReasonFirstBind),
	).Scan(&count))
	s.Require().Zero(count)
}

func (s *UserProfileIdentityRepoSuite) TestRecordProviderGrant_IsIdempotentPerReason() {
	user := s.mustCreateUser("grant")

	inserted, err := s.repo.RecordProviderGrant(s.ctx, ProviderGrantRecordInput{
		UserID:       user.ID,
		ProviderType: "wechat",
		GrantReason:  ProviderGrantReasonFirstBind,
	})
	s.Require().NoError(err)
	s.Require().True(inserted)

	inserted, err = s.repo.RecordProviderGrant(s.ctx, ProviderGrantRecordInput{
		UserID:       user.ID,
		ProviderType: "wechat",
		GrantReason:  ProviderGrantReasonFirstBind,
	})
	s.Require().NoError(err)
	s.Require().False(inserted)

	inserted, err = s.repo.RecordProviderGrant(s.ctx, ProviderGrantRecordInput{
		UserID:       user.ID,
		ProviderType: "wechat",
		GrantReason:  ProviderGrantReasonSignup,
	})
	s.Require().NoError(err)
	s.Require().True(inserted)

	var count int
	s.Require().NoError(integrationDB.QueryRowContext(s.ctx, `
SELECT COUNT(*)
FROM user_provider_default_grants
WHERE user_id = $1 AND provider_type = $2`,
		user.ID,
		"wechat",
	).Scan(&count))
	s.Require().Equal(2, count)
}

func (s *UserProfileIdentityRepoSuite) TestUpsertIdentityAdoptionDecision_PersistsAndLinksIdentity() {
	user := s.mustCreateUser("adoption")
	identity, err := s.repo.CreateAuthIdentity(s.ctx, CreateAuthIdentityInput{
		UserID: user.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-adoption",
		},
	})
	s.Require().NoError(err)

	session := s.mustCreatePendingAuthSession(identity.IdentityRef())

	first, err := s.repo.UpsertIdentityAdoptionDecision(s.ctx, IdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          false,
	})
	s.Require().NoError(err)
	s.Require().True(first.AdoptDisplayName)
	s.Require().False(first.AdoptAvatar)
	s.Require().Nil(first.IdentityID)

	second, err := s.repo.UpsertIdentityAdoptionDecision(s.ctx, IdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.Identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          true,
	})
	s.Require().NoError(err)
	s.Require().Equal(first.ID, second.ID)
	s.Require().NotNil(second.IdentityID)
	s.Require().Equal(identity.Identity.ID, *second.IdentityID)
	s.Require().True(second.AdoptAvatar)

	loaded, err := s.repo.GetIdentityAdoptionDecisionByPendingAuthSessionID(s.ctx, session.ID)
	s.Require().NoError(err)
	s.Require().Equal(second.ID, loaded.ID)
	s.Require().Equal(identity.Identity.ID, *loaded.IdentityID)
}

func (s *UserProfileIdentityRepoSuite) TestUpsertIdentityAdoptionDecision_ReassignsExistingIdentityReference() {
	user := s.mustCreateUser("adoption-reassign")
	identity, err := s.repo.CreateAuthIdentity(s.ctx, CreateAuthIdentityInput{
		UserID: user.ID,
		Canonical: AuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-adoption-reassign",
		},
	})
	s.Require().NoError(err)

	firstSession := s.mustCreatePendingAuthSession(identity.IdentityRef())
	firstDecision, err := s.repo.UpsertIdentityAdoptionDecision(s.ctx, IdentityAdoptionDecisionInput{
		PendingAuthSessionID: firstSession.ID,
		IdentityID:           &identity.Identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          false,
	})
	s.Require().NoError(err)
	s.Require().NotNil(firstDecision.IdentityID)
	s.Require().Equal(identity.Identity.ID, *firstDecision.IdentityID)

	secondSession := s.mustCreatePendingAuthSession(identity.IdentityRef())
	secondDecision, err := s.repo.UpsertIdentityAdoptionDecision(s.ctx, IdentityAdoptionDecisionInput{
		PendingAuthSessionID: secondSession.ID,
		IdentityID:           &identity.Identity.ID,
		AdoptDisplayName:     false,
		AdoptAvatar:          true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(secondDecision.IdentityID)
	s.Require().Equal(identity.Identity.ID, *secondDecision.IdentityID)

	reloadedFirst, err := s.repo.GetIdentityAdoptionDecisionByPendingAuthSessionID(s.ctx, firstSession.ID)
	s.Require().NoError(err)
	s.Require().Nil(reloadedFirst.IdentityID)
}

func (s *UserProfileIdentityRepoSuite) TestWithUserProfileIdentityTx_AllowsAvatarOnlyProfileUpdate() {
	user := s.mustCreateUser("avatar-only-update")

	model, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().NotNil(model)

	err = s.repo.WithUserProfileIdentityTx(s.ctx, func(txCtx context.Context) error {
		_, err := s.repo.UpsertUserAvatar(txCtx, user.ID, service.UpsertUserAvatarInput{
			StorageProvider: "remote_url",
			URL:             "https://cdn.example.com/avatar.png",
		})
		if err != nil {
			return err
		}
		return s.repo.Update(txCtx, model)
	})
	s.Require().NoError(err)

	avatar, err := s.repo.GetUserAvatar(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().NotNil(avatar)
	s.Require().Equal("https://cdn.example.com/avatar.png", avatar.URL)
}

func (s *UserProfileIdentityRepoSuite) TestUserAvatarCRUDAndUserLookup() {
	user := s.mustCreateUser("avatar")

	inlineAvatar, err := s.repo.UpsertUserAvatar(s.ctx, user.ID, service.UpsertUserAvatarInput{
		StorageProvider: "inline",
		URL:             "data:image/png;base64,QUJD",
		ContentType:     "image/png",
		ByteSize:        3,
		SHA256:          "902fbdd2b1df0c4f70b4a5d23525e932",
	})
	s.Require().NoError(err)
	s.Require().Equal("inline", inlineAvatar.StorageProvider)
	s.Require().Equal("data:image/png;base64,QUJD", inlineAvatar.URL)

	loadedAvatar, err := s.repo.GetUserAvatar(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().NotNil(loadedAvatar)
	s.Require().Equal("image/png", loadedAvatar.ContentType)
	s.Require().Equal(3, loadedAvatar.ByteSize)

	_, err = s.repo.UpsertUserAvatar(s.ctx, user.ID, service.UpsertUserAvatarInput{
		StorageProvider: "remote_url",
		URL:             "https://cdn.example.com/avatar.png",
	})
	s.Require().NoError(err)

	loadedAvatar, err = s.repo.GetUserAvatar(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().NotNil(loadedAvatar)
	s.Require().Equal("remote_url", loadedAvatar.StorageProvider)
	s.Require().Equal("https://cdn.example.com/avatar.png", loadedAvatar.URL)
	s.Require().Zero(loadedAvatar.ByteSize)

	s.Require().NoError(s.repo.DeleteUserAvatar(s.ctx, user.ID))
	loadedAvatar, err = s.repo.GetUserAvatar(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().Nil(loadedAvatar)
}

func (s *UserProfileIdentityRepoSuite) TestUpdateUserLastLoginAndActiveAt_UsesDedicatedColumns() {
	user := s.mustCreateUser("activity")
	loginAt := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	activeAt := loginAt.Add(5 * time.Minute)

	s.Require().NoError(s.repo.UpdateUserLastLoginAt(s.ctx, user.ID, loginAt))
	s.Require().NoError(s.repo.UpdateUserLastActiveAt(s.ctx, user.ID, activeAt))

	var storedLoginAt sqlNullTime
	var storedActiveAt sqlNullTime
	s.Require().NoError(integrationDB.QueryRowContext(s.ctx, `
SELECT last_login_at, last_active_at
FROM users
WHERE id = $1`,
		user.ID,
	).Scan(&storedLoginAt, &storedActiveAt))
	s.Require().True(storedLoginAt.Valid)
	s.Require().True(storedActiveAt.Valid)
	s.Require().True(storedLoginAt.Time.Equal(loginAt))
	s.Require().True(storedActiveAt.Time.Equal(activeAt))
}

type sqlNullTime struct {
	Time  time.Time
	Valid bool
}

func (t *sqlNullTime) Scan(value any) error {
	switch v := value.(type) {
	case time.Time:
		t.Time = v
		t.Valid = true
		return nil
	case nil:
		t.Time = time.Time{}
		t.Valid = false
		return nil
	default:
		return fmt.Errorf("unsupported scan type %T", value)
	}
}

func stringPtr(v string) *string {
	return &v
}
