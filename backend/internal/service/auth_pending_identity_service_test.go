//go:build unit

package service

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAuthPendingIdentityServiceTestClient(t *testing.T) (*AuthPendingIdentityService, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth_pending_identity_service?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewAuthPendingIdentityService(client), client
}

func TestAuthPendingIdentityService_CreatePendingSessionStoresSeparatedState(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	targetUser, err := client.User.Create().
		SetEmail("pending-target@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-123",
		},
		TargetUserID:           &targetUser.ID,
		RedirectTo:             "/profile",
		ResolvedEmail:          "user@example.com",
		BrowserSessionKey:      "browser-1",
		UpstreamIdentityClaims: map[string]any{"nickname": "wx-user", "avatar_url": "https://cdn.example/avatar.png"},
		LocalFlowState:         map[string]any{"step": "email_required"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, session.SessionToken)
	require.Equal(t, "bind_current_user", session.Intent)
	require.Equal(t, "wechat", session.ProviderType)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, targetUser.ID, *session.TargetUserID)
	require.Equal(t, "wx-user", session.UpstreamIdentityClaims["nickname"])
	require.Equal(t, "email_required", session.LocalFlowState["step"])
}

func TestAuthPendingIdentityService_CompletionCodeIsBrowserBoundAndOneTime(t *testing.T) {
	svc, _ := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo-main",
			ProviderSubject: "subject-1",
		},
		BrowserSessionKey:      "browser-expected",
		UpstreamIdentityClaims: map[string]any{"nickname": "linux-user"},
		LocalFlowState:         map[string]any{"step": "pending"},
	})
	require.NoError(t, err)

	issued, err := svc.IssueCompletionCode(ctx, IssuePendingAuthCompletionCodeInput{
		PendingAuthSessionID: session.ID,
		BrowserSessionKey:    "browser-expected",
	})
	require.NoError(t, err)
	require.NotEmpty(t, issued.Code)

	_, err = svc.ConsumeCompletionCode(ctx, issued.Code, "browser-other")
	require.ErrorIs(t, err, ErrPendingAuthBrowserMismatch)

	consumed, err := svc.ConsumeCompletionCode(ctx, issued.Code, "browser-expected")
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
	require.Empty(t, consumed.CompletionCodeHash)
	require.Nil(t, consumed.CompletionCodeExpiresAt)

	_, err = svc.ConsumeCompletionCode(ctx, issued.Code, "browser-expected")
	require.ErrorIs(t, err, ErrPendingAuthCodeInvalid)
}

func TestAuthPendingIdentityService_CompletionCodeExpires(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "oidc",
			ProviderKey:     "https://issuer.example",
			ProviderSubject: "subject-1",
		},
		BrowserSessionKey: "browser-expired",
	})
	require.NoError(t, err)

	issued, err := svc.IssueCompletionCode(ctx, IssuePendingAuthCompletionCodeInput{
		PendingAuthSessionID: session.ID,
		BrowserSessionKey:    "browser-expired",
		TTL:                  time.Second,
	})
	require.NoError(t, err)

	_, err = client.PendingAuthSession.UpdateOneID(session.ID).
		SetCompletionCodeExpiresAt(time.Now().UTC().Add(-time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.ConsumeCompletionCode(ctx, issued.Code, "browser-expired")
	require.ErrorIs(t, err, ErrPendingAuthCodeExpired)
}

func TestAuthPendingIdentityService_UpsertAdoptionDecision(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("adoption@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-open").
		SetProviderSubject("union-adoption").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-adoption",
		},
	})
	require.NoError(t, err)

	first, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          false,
	})
	require.NoError(t, err)
	require.True(t, first.AdoptDisplayName)
	require.False(t, first.AdoptAvatar)
	require.Nil(t, first.IdentityID)

	second, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          true,
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.NotNil(t, second.IdentityID)
	require.Equal(t, identity.ID, *second.IdentityID)
	require.True(t, second.AdoptAvatar)
}

func TestAuthPendingIdentityService_UpsertAdoptionDecision_ReassignsExistingIdentityReference(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("adoption-reassign@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-open").
		SetProviderSubject("union-reassign").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	firstSession, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-reassign",
		},
	})
	require.NoError(t, err)

	firstDecision, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: firstSession.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          false,
	})
	require.NoError(t, err)
	require.NotNil(t, firstDecision.IdentityID)
	require.Equal(t, identity.ID, *firstDecision.IdentityID)

	secondSession, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-reassign",
		},
	})
	require.NoError(t, err)

	secondDecision, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: secondSession.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     false,
		AdoptAvatar:          true,
	})
	require.NoError(t, err)
	require.NotNil(t, secondDecision.IdentityID)
	require.Equal(t, identity.ID, *secondDecision.IdentityID)

	reloadedFirst, err := client.IdentityAdoptionDecision.Get(ctx, firstDecision.ID)
	require.NoError(t, err)
	require.Nil(t, reloadedFirst.IdentityID)
}

func TestAuthPendingIdentityService_UpsertAdoptionDecision_IsIdempotentUnderConcurrency(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("adoption-concurrent@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-main").
		SetProviderSubject("union-concurrent").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-main",
			ProviderSubject: "union-concurrent",
		},
	})
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

	input := PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          true,
	}

	results := make(chan adoptionResult, 2)
	go func() {
		decision, err := svc.UpsertAdoptionDecision(ctx, input)
		results <- adoptionResult{decision: decision, err: err}
	}()

	<-firstCreateStarted

	go func() {
		decision, err := svc.UpsertAdoptionDecision(ctx, input)
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
}

func TestAuthPendingIdentityService_UpsertAdoptionDecision_ClearsLegacyNullSessionReference(t *testing.T) {
	t.Skip("legacy NULL pending_auth_session_id rows only exist in production PostgreSQL history; sqlite unit schema rejects NULL")

	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("legacy-null-session@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-main").
		SetProviderSubject("legacy-null-session").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ExecContext(
		ctx,
		`INSERT INTO identity_adoption_decisions
			(identity_id, adopt_display_name, adopt_avatar, decided_at, created_at, updated_at, pending_auth_session_id)
		VALUES (?, ?, ?, ?, ?, ?, NULL)`,
		identity.ID,
		true,
		false,
		time.Now().UTC(),
		time.Now().UTC(),
		time.Now().UTC(),
	)
	require.NoError(t, err)
	legacyDecision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.IdentityIDEQ(identity.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, legacyDecision.IdentityID)

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-main",
			ProviderSubject: "legacy-null-session",
		},
	})
	require.NoError(t, err)

	decision, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     false,
		AdoptAvatar:          true,
	})
	require.NoError(t, err)
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)

	reloadedLegacy, err := client.IdentityAdoptionDecision.Get(ctx, legacyDecision.ID)
	require.NoError(t, err)
	require.Nil(t, reloadedLegacy.IdentityID)
}

func TestAuthPendingIdentityService_ConsumeBrowserSession(t *testing.T) {
	svc, _ := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo",
			ProviderSubject: "subject-session-token",
		},
		BrowserSessionKey: "browser-session",
		LocalFlowState: map[string]any{
			"completion_response": map[string]any{
				"access_token": "token",
			},
		},
	})
	require.NoError(t, err)

	_, err = svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-other")
	require.ErrorIs(t, err, ErrPendingAuthBrowserMismatch)

	consumed, err := svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-session")
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)

	_, err = svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-session")
	require.ErrorIs(t, err, ErrPendingAuthSessionConsumed)
}

func TestAuthPendingIdentityService_ConsumeBrowserSessionRejectsStaleLoadedSessionReplay(t *testing.T) {
	svc, _ := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo",
			ProviderSubject: "stale-replay-subject",
		},
		BrowserSessionKey: "browser-session",
	})
	require.NoError(t, err)

	loaded, err := svc.getBrowserSession(ctx, session.SessionToken)
	require.NoError(t, err)

	consumed, err := svc.consumeSession(ctx, loaded, "browser-session", ErrPendingAuthSessionExpired, ErrPendingAuthSessionConsumed)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)

	_, err = svc.consumeSession(ctx, loaded, "browser-session", ErrPendingAuthSessionExpired, ErrPendingAuthSessionConsumed)
	require.ErrorIs(t, err, ErrPendingAuthSessionConsumed)
}

func TestAuthPendingIdentityService_ConsumeBrowserSessionScrubsLegacyCompletionTokens(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo",
			ProviderSubject: "legacy-token-subject",
		},
		BrowserSessionKey: "browser-session",
		LocalFlowState: map[string]any{
			"completion_response": map[string]any{
				"access_token":  "legacy-access-token",
				"refresh_token": "legacy-refresh-token",
				"expires_in":    float64(3600),
				"token_type":    "Bearer",
				"redirect":      "/dashboard",
			},
		},
	})
	require.NoError(t, err)

	consumed, err := svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-session")
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)

	stored, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)

	completion, ok := stored.LocalFlowState["completion_response"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, completion, "access_token")
	require.NotContains(t, completion, "refresh_token")
	require.NotContains(t, completion, "expires_in")
	require.NotContains(t, completion, "token_type")
	require.Equal(t, "/dashboard", completion["redirect"])
}
