package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryBindAuthIdentityToUserCanonicalizesLegacyWeChatAlias(t *testing.T) {
	repo, client := newUserEntRepo(t)
	ctx := context.Background()

	user := &service.User{
		Email:        "wechat-legacy@example.com",
		Username:     "wechat-legacy",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, user))

	legacyIdentity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetProviderSubject("union-legacy-123").
		SetMetadata(map[string]any{"source": "legacy-alias"}).
		Save(ctx)
	require.NoError(t, err)

	legacyChannel, err := client.AuthIdentityChannel.Create().
		SetIdentityID(legacyIdentity.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetChannel("oa").
		SetChannelAppID("wx-app-legacy").
		SetChannelSubject("openid-legacy-123").
		SetMetadata(map[string]any{"scene": "legacy-alias"}).
		Save(ctx)
	require.NoError(t, err)

	bound, err := repo.BindAuthIdentityToUser(ctx, BindAuthIdentityInput{
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
	require.NoError(t, err)
	require.NotNil(t, bound)
	require.NotNil(t, bound.Identity)
	require.NotNil(t, bound.Channel)
	require.Equal(t, legacyIdentity.ID, bound.Identity.ID)
	require.Equal(t, legacyChannel.ID, bound.Channel.ID)
	require.Equal(t, "wechat-main", bound.Identity.ProviderKey)
	require.Equal(t, "wechat-main", bound.Channel.ProviderKey)

	reloadedIdentity, err := client.AuthIdentity.Get(ctx, legacyIdentity.ID)
	require.NoError(t, err)
	require.Equal(t, "wechat-main", reloadedIdentity.ProviderKey)
	require.Equal(t, "canonical-bind", reloadedIdentity.Metadata["source"])

	reloadedChannel, err := client.AuthIdentityChannel.Get(ctx, legacyChannel.ID)
	require.NoError(t, err)
	require.Equal(t, "wechat-main", reloadedChannel.ProviderKey)
	require.Equal(t, "canonical-bind", reloadedChannel.Metadata["scene"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderSubjectEQ("union-legacy-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, identityCount)

	channelCount, err := client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ("wechat"),
			authidentitychannel.ChannelEQ("oa"),
			authidentitychannel.ChannelAppIDEQ("wx-app-legacy"),
			authidentitychannel.ChannelSubjectEQ("openid-legacy-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, channelCount)
}

func TestUserRepositoryUpsertIdentityAdoptionDecisionIsIdempotentUnderConcurrency(t *testing.T) {
	repo, client := newUserEntRepo(t)
	ctx := context.Background()

	user := &service.User{
		Email:        "repo-adoption@example.com",
		Username:     "repo-adoption",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, user))

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-main").
		SetProviderSubject("union-repo-adoption").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("pending-repo-adoption").
		SetIntent("bind_current_user").
		SetProviderType("wechat").
		SetProviderKey("wechat-main").
		SetProviderSubject("union-repo-adoption").
		SetExpiresAt(time.Now().UTC().Add(15 * time.Minute)).
		SetUpstreamIdentityClaims(map[string]any{"provider_subject": "union-repo-adoption"}).
		SetLocalFlowState(map[string]any{"step": "pending"}).
		Save(ctx)
	require.NoError(t, err)

	firstCreateStarted := make(chan struct{})
	releaseFirstCreate := make(chan struct{})
	var firstCreate sync.Once
	client.IdentityAdoptionDecision.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			blocked := false
			if m.Op().Is(dbent.OpCreate) {
				firstCreate.Do(func() {
					blocked = true
					close(firstCreateStarted)
				})
			}
			if blocked {
				<-releaseFirstCreate
			}
			return next.Mutate(ctx, m)
		})
	})

	type adoptionResult struct {
		decision *dbent.IdentityAdoptionDecision
		err      error
	}

	input := IdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          true,
	}

	results := make(chan adoptionResult, 2)
	go func() {
		decision, err := repo.UpsertIdentityAdoptionDecision(ctx, input)
		results <- adoptionResult{decision: decision, err: err}
	}()

	<-firstCreateStarted

	go func() {
		decision, err := repo.UpsertIdentityAdoptionDecision(ctx, input)
		results <- adoptionResult{decision: decision, err: err}
	}()

	time.Sleep(100 * time.Millisecond)
	close(releaseFirstCreate)

	first := <-results
	second := <-results

	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.decision)
	require.NotNil(t, second.decision)
	require.Equal(t, first.decision.ID, second.decision.ID)

	count, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	loaded, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, loaded.IdentityID)
	require.Equal(t, identity.ID, *loaded.IdentityID)
	require.True(t, loaded.AdoptDisplayName)
	require.True(t, loaded.AdoptAvatar)
}
