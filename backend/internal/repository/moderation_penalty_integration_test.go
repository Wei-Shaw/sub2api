//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type penaltySettings struct {
	service.SettingRepository
	values map[string]string
}

func (s penaltySettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return s.values, nil
}

func penaltyUser(t *testing.T, role string) *service.User {
	t.Helper()
	u := &service.User{Role: role, Email: uuid.NewString() + "@example.test", Balance: 100}
	require.NoError(t, u.SetPassword("fixture-password"))
	return mustCreateUser(t, integrationEntClient, u)
}
func penaltyKey(t *testing.T, u *service.User) *service.APIKey {
	t.Helper()
	return mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: u.ID, Key: "fixture-" + uuid.NewString()})
}
func setPenaltyScope(ctx context.Context, u *service.User, scope string, actorID int64) error {
	patch := service.User{ID: u.ID, ModerationPenaltyScope: scope}
	return NewUserRepository(integrationEntClient, integrationDB).Update(ctx, &patch, service.UserUpdateFields{ModerationPenaltyScope: true, ModerationScopeActorID: actorID})
}
func setPenaltyDuration(ctx context.Context, u *service.User, minutes int, actorID int64) error {
	patch := service.User{ID: u.ID, ModerationKeyLockMinutes: minutes}
	return NewUserRepository(integrationEntClient, integrationDB).Update(ctx, &patch, service.UserUpdateFields{ModerationKeyLockMinutes: true, ModerationScopeActorID: actorID})
}

// Advance only the fixture lock. Authentication and all regular key limits still use the real database.
func makePenaltyDue(t *testing.T, keyID int64) {
	t.Helper()
	result, err := integrationDB.Exec(`UPDATE api_keys SET moderation_locked_at=clock_timestamp()-interval '1 hour',
		moderation_locked_until=clock_timestamp()-interval '1 millisecond' WHERE id=$1 AND moderation_locked_until IS NOT NULL`, keyID)
	require.NoError(t, err)
	n, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func penaltyHit(u *service.User, k *service.APIKey) service.ContentModerationLog {
	return service.ContentModerationLog{UserID: &u.ID, APIKeyID: &k.ID, APIKeyName: k.Name, EventID: uuid.NewString(), OccurredAt: time.Now().UTC(), Mode: service.ContentModerationModePreBlock, Action: service.ContentModerationActionBlock, Flagged: true}
}
func recordPenaltyHit(t *testing.T, log service.ContentModerationLog, rules service.ModerationPenaltyRules) service.ContentModerationLog {
	t.Helper()
	result, err := NewContentModerationRepository(integrationDB).RecordHit(context.Background(), &log, rules)
	require.NoError(t, err)
	require.True(t, result.Inserted)
	return log
}
func penaltyInt(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRow(query, args...).Scan(&n))
	return n
}

func TestModerationPenaltyGatewaysAndSessions(t *testing.T) {
	ctx := context.Background()
	a, b, operator := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	a1, a2, b1, b2 := penaltyKey(t, a), penaltyKey(t, a), penaltyKey(t, b), penaltyKey(t, b)
	users := NewUserRepository(integrationEntClient, integrationDB)
	keys := NewAPIKeyRepository(integrationEntClient, integrationDB)
	cache := NewAPIKeyCache(testRedis(t))
	cfg := &config.Config{RunMode: config.RunModeSimple, JWT: config.JWTConfig{Secret: uuid.NewString(), ExpireHour: 1}, APIKeyAuth: config.APIKeyAuthCacheConfig{L1Size: 100, L1TTLSeconds: 180, L2TTLSeconds: 180}}
	first := service.NewAPIKeyService(keys, users, nil, nil, nil, cache, cfg)
	// The second instance has its own L1 and deliberately never subscribes to invalidations.
	second := service.NewAPIKeyService(keys, users, nil, nil, nil, cache, cfg)
	auth := service.NewAuthService(integrationEntClient, users, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	login := func(u *service.User) string {
		t.Helper()
		token, _, err := auth.Login(ctx, u.Email, "fixture-password")
		require.NoError(t, err)
		return token
	}
	aJWT, bJWT, adminJWT := login(a), login(b), login(operator)
	settings := penaltySettings{values: map[string]string{service.SettingKeyRiskControlEnabled: "true", service.SettingKeyContentModerationConfig: `{"enabled":true,"mode":"pre_block","all_groups":true,"ban_threshold":2,"auto_ban_enabled":true,"violation_window_hours":24,"keyword_blocking_mode":"keyword_only","blocked_keywords":["fixtureword"]}`}}
	repo := NewContentModerationRepository(integrationDB)
	mod := service.NewContentModerationService(settings, repo, nil, nil, users, nil, first, nil)
	usr := service.NewUserService(users, nil, first, nil)
	adminSvc := service.NewAdminService(cfg, users, nil, nil, nil, keys, nil, nil, nil, nil, nil, nil, first, integrationEntClient, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	router.GET("/api", gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(second, nil, cfg)), ok)
	router.GET("/google", middleware.APIKeyAuthGoogle(second, cfg), ok)
	jwt := gin.HandlerFunc(middleware.NewJWTAuthMiddleware(auth, usr, nil, nil))
	router.GET("/session", jwt, ok)
	router.PUT("/profile", jwt, handler.NewUserHandler(usr, auth, nil, nil, nil, nil).UpdateProfile)
	management := router.Group("/admin", gin.HandlerFunc(middleware.NewAdminAuthMiddleware(auth, usr, nil, nil)))
	management.PUT("/users/:id", admin.NewUserHandler(adminSvc, nil, nil, nil, nil, usr, nil).Update)
	management.POST("/risk-control/users/:user_id/api-keys/:key_id/unlock", admin.NewContentModerationHandler(mod).UnlockKey)
	check := func(method, path, token, body string, code int, contains string) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if path == "/google" {
			r.Header.Set("x-goog-api-key", token)
		} else {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, code, w.Code, "%s %s: %s", method, path, w.Body.String())
		if contains != "" {
			require.Contains(t, w.Body.String(), contains)
		}
	}
	userPath := fmt.Sprintf("/admin/users/%d", a.ID)
	unlockPath := fmt.Sprintf("/admin/risk-control/users/%d/api-keys/%d/unlock", a.ID, a1.ID)
	const apiKeyScope = `{"moderation_penalty_scope":"api_key"}`
	const tenMinutes = `{"moderation_key_lock_minutes":10}`
	check("PUT", "/profile", aJWT, tenMinutes, 403, "")
	check("PUT", userPath, aJWT, tenMinutes, 403, "")
	check("PUT", fmt.Sprintf("/admin/users/%d", operator.ID), adminJWT, tenMinutes, 400, "")
	check("PUT", userPath, adminJWT, tenMinutes, 400, "") // account scope
	check("PUT", "/profile", aJWT, apiKeyScope, 403, "")
	check("PUT", userPath, aJWT, apiKeyScope, 403, "")
	check("POST", unlockPath, aJWT, "", 403, "")
	check("PUT", fmt.Sprintf("/admin/users/%d", operator.ID), adminJWT, apiKeyScope, 400, "")
	check("PUT", userPath, adminJWT, `{"moderation_penalty_scope":"other"}`, 400, "")
	check("PUT", userPath, adminJWT, apiKeyScope, 200, `"moderation_penalty_scope":"api_key"`)
	check("PUT", userPath, adminJWT, `{"notes":"profile only"}`, 200, `"moderation_penalty_scope":"api_key"`)
	for _, u := range []*service.User{a, b} {
		_, err := integrationDB.Exec(`INSERT INTO mcp_oauth_grants(id,client_id,client_name,user_id,token_version,resource,scope,created_at,expires_at) VALUES($1,'fixture','fixture',$2,0,'test','user',now(),now()+interval '1 day')`, uuid.NewString(), u.ID)
		require.NoError(t, err)
	}
	// L2 really contains the previously authorized keys; a stale instance must still reject after commit.
	for _, k := range []*service.APIKey{a1, a2, b1, b2} {
		_, err := first.GetByKey(ctx, k.Key)
		require.NoError(t, err)
		entry, err := cache.GetAuthCache(ctx, fmt.Sprintf("%x", sha256.Sum256([]byte(k.Key))))
		require.NoError(t, err)
		require.Equal(t, k.ID, entry.Snapshot.APIKeyID)
		check("GET", "/api", k.Key, "", 204, "")
	}
	ordinary := func(k *service.APIKey) {
		t.Helper()
		requestID := uuid.NewString()
		dec, err := mod.Check(ctx, service.ContentModerationCheckInput{UserID: k.UserID, APIKeyID: k.ID, RequestID: requestID, Protocol: service.ContentModerationProtocolOpenAIResponses, Body: []byte(`{"input":"fixtureword"}`)})
		require.NoError(t, err)
		require.True(t, dec.Blocked)
		require.False(t, dec.Allowed)
		require.Eventually(t, func() bool {
			return penaltyInt(t, `SELECT COUNT(*) FROM content_moderation_logs WHERE request_id=$1 AND flagged`, requestID) == 1
		}, 4*time.Second, 10*time.Millisecond)
	}
	cyber := func(k *service.APIKey) {
		mod.RecordCyberPolicyEvent(ctx, service.CyberPolicyRecordInput{UserID: k.UserID, APIKeyID: k.ID, EventID: uuid.NewString(), OccurredAt: time.Now().UTC(), Model: "fixture"})
	}
	ordinary(a1)
	cyber(a2)
	for _, k := range []*service.APIKey{a1, a2} {
		check("GET", "/api", k.Key, "", 204, "")
	}
	cyber(a1)
	check("GET", "/api", a1.Key, "", 401, "API_KEY_DISABLED")
	check("GET", "/google", a1.Key, "", 401, "API key is disabled")
	check("GET", "/google", a2.Key, "", 204, "")
	check("GET", "/session", aJWT, "", 204, "")
	_ = login(a)
	ordinary(b1)
	cyber(b2)
	for _, k := range []*service.APIKey{b1, b2} {
		check("GET", "/api", k.Key, "", 401, "USER_INACTIVE")
		check("GET", "/google", k.Key, "", 401, "User account is not active")
	}
	check("GET", "/session", bJWT, "", 401, "USER_INACTIVE")
	_, _, err := auth.Login(ctx, b.Email, "fixture-password")
	require.ErrorIs(t, err, service.ErrUserNotActive)
	check("GET", "/api", a2.Key, "", 204, "")
	require.Equal(t, 0, penaltyInt(t, `SELECT count(*) FROM mcp_oauth_grants WHERE user_id=$1 AND revoked_at IS NOT NULL`, a.ID))
	require.Equal(t, 1, penaltyInt(t, `SELECT count(*) FROM mcp_oauth_grants WHERE user_id=$1 AND revoked_at IS NOT NULL`, b.ID))
	require.ErrorIs(t, first.Delete(ctx, a1.ID, a.ID), service.ErrAPIKeyModerationLocked)
	active := service.StatusActive
	_, err = first.Update(ctx, a1.ID, a.ID, service.UpdateAPIKeyRequest{Status: &active})
	require.ErrorIs(t, err, service.ErrAPIKeyModerationLocked)
	check("POST", fmt.Sprintf("/admin/risk-control/users/%d/api-keys/%d/unlock", a.ID, b1.ID), adminJWT, "", 404, "")
	check("POST", unlockPath, adminJWT, "", 200, `"unlocked":true`)
	check("POST", unlockPath, adminJWT, "", 200, `"unlocked":false`)
	check("GET", "/api", a1.Key, "", 204, "")
	for _, invalid := range []string{"-1", "1.5", "2147483648"} {
		check("PUT", userPath, adminJWT, `{"moderation_key_lock_minutes":`+invalid+`}`, 400, "")
	}
	check("PUT", userPath, adminJWT, tenMinutes, 200, `"moderation_key_lock_minutes":10`)
	check("PUT", userPath, adminJWT, `{"notes":"unrelated"}`, 200, `"moderation_key_lock_minutes":10`)
	cyber(a2)
	check("GET", "/api", a2.Key, "", 401, "API_KEY_DISABLED")
	check("GET", "/google", a2.Key, "", 401, "API key is disabled")
	makePenaltyDue(t, a2.ID)
	check("GET", "/google", a2.Key, "", 204, "")
	check("GET", "/api", a2.Key, "", 204, "")
	check("GET", "/session", aJWT, "", 204, "")
	cyber(a1)
	check("GET", "/api", a1.Key, "", 204, "")
	require.Equal(t, 3, penaltyInt(t, `SELECT count(*) FROM moderation_admin_actions WHERE actor_id=$1 AND user_id=$2 AND ((action='change_scope' AND old_value='user' AND new_value='api_key') OR (action='unlock_key' AND api_key_id=$3 AND new_value='unlocked'))`, operator.ID, a.ID, a1.ID))
}

func TestModerationPenaltyConcurrentHitsAndBilling(t *testing.T) {
	ctx := context.Background()
	u, operator := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	k := penaltyKey(t, u)
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 2, Window: 24 * time.Hour}
	hits := []service.ContentModerationLog{penaltyHit(u, k), penaltyHit(u, k)}
	type outcome struct {
		log   service.ContentModerationLog
		write service.ModerationWriteResult
		err   error
	}
	start := make(chan struct{})
	results := make(chan outcome, 4)
	// Two independent repository instances each receive both events. They use
	// the same DB as the billing writer and don't share any in-memory locks.
	for range 2 {
		repo := NewContentModerationRepository(integrationDB)
		for _, hit := range hits {
			go func(event service.ContentModerationLog) {
				<-start
				result, err := repo.RecordHit(ctx, &event, rules)
				results <- outcome{event, result, err}
			}(hit)
		}
	}
	billed := make(chan error, 1)
	go func() {
		<-start
		_, err := NewAPIKeyRepository(integrationEntClient, integrationDB).IncrementQuotaUsed(ctx, k.ID, 7)
		billed <- err
	}()
	close(start)
	counts := []int{}
	penalties := 0
	for range 4 {
		out := <-results
		require.NoError(t, out.err)
		if out.write.Inserted {
			counts = append(counts, out.log.ViolationCount)
		}
		if out.log.PenaltyAction == service.ModerationPenaltyKeyLocked {
			penalties++
			require.True(t, out.write.Inserted)
			require.Equal(t, k.Key, out.write.InvalidationKey)
		} else {
			require.Empty(t, out.write.InvalidationKey)
		}
	}
	require.NoError(t, <-billed)
	require.ElementsMatch(t, []int{1, 2}, counts)
	require.Equal(t, 1, penalties)
	require.Equal(t, 2, penaltyInt(t, `SELECT count(*) FROM content_moderation_logs WHERE user_id=$1`, u.ID))
	require.Equal(t, 1, penaltyInt(t, `SELECT count(*) FROM api_keys WHERE id=$1 AND moderation_locked_at IS NOT NULL AND quota_used=7 AND status='active'`, k.ID))
}

func TestModerationPenaltyCountingBoundaries(t *testing.T) {
	ctx := context.Background()
	u, operator := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	k1, k2 := penaltyKey(t, u), penaltyKey(t, u)
	repo := NewContentModerationRepository(integrationDB)
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 2, Window: 24 * time.Hour}
	boundary := func(query string, id int64) time.Time {
		t.Helper()
		var timestamp time.Time
		require.NoError(t, integrationDB.QueryRow(query, id).Scan(&timestamp))
		return timestamp
	}
	const scopeTime = `SELECT moderation_scope_changed_at FROM users WHERE id=$1`
	const unlockTime = `SELECT moderation_last_unlocked_at FROM api_keys WHERE id=$1`
	auditOnly := func(k *service.APIKey, at time.Time) {
		t.Helper()
		event := penaltyHit(u, k)
		event.OccurredAt = at
		saved := recordPenaltyHit(t, event, rules)
		require.Empty(t, saved.PenaltyScope)
		require.Empty(t, saved.PenaltyAction)
		require.Zero(t, saved.ViolationCount)
	}
	hit := func(k *service.APIKey, count int, action string) {
		t.Helper()
		saved := recordPenaltyHit(t, penaltyHit(u, k), rules)
		require.Equal(t, count, saved.ViolationCount)
		require.Equal(t, action, saved.PenaltyAction)
	}
	hit(k1, 1, "")
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	start := boundary(scopeTime, u.ID)
	auditOnly(k1, start.Add(-time.Second))
	hit(k1, 1, "")
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	require.True(t, start.Equal(boundary(scopeTime, u.ID)))
	hit(k1, 2, service.ModerationPenaltyKeyLocked)
	hit(k2, 1, "")
	key, err := repo.UnlockModerationKey(ctx, u.ID, k1.ID, operator.ID)
	require.NoError(t, err)
	require.Equal(t, k1.Key, key)
	unlocked := boundary(unlockTime, k1.ID)
	key, err = repo.UnlockModerationKey(ctx, u.ID, k1.ID, operator.ID)
	require.NoError(t, err)
	require.Empty(t, key)
	require.True(t, unlocked.Equal(boundary(unlockTime, k1.ID)))
	auditOnly(k1, unlocked)
	hit(k1, 1, "")
	hit(k2, 2, service.ModerationPenaltyKeyLocked)
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeUser, operator.ID))
	require.Equal(t, 1, penaltyInt(t, `SELECT count(*) FROM api_keys WHERE id=$1 AND moderation_locked_at IS NOT NULL`, k2.ID))
	auditOnly(k2, boundary(scopeTime, u.ID).Add(-time.Second))
	hit(k1, 1, "")
	hit(k1, 2, service.ModerationPenaltyUserDisabled)
	require.Equal(t, 2, penaltyInt(t, `SELECT count(*) FROM moderation_admin_actions WHERE user_id=$1 AND action='change_scope'`, u.ID))
	require.Zero(t, penaltyInt(t, `SELECT count(*) FROM moderation_admin_actions a WHERE user_id=$1 AND (strpos(to_jsonb(a)::text,$2)>0 OR strpos(to_jsonb(a)::text,$3)>0)`, u.ID, k1.Key, k2.Key))
}

// Race an already captured event with the administrative write on independent
// connections. The outcome must match one of the two legal commit orders.
func TestModerationPenaltyConcurrentScopeChangeAndUnlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	operator := penaltyUser(t, service.RoleAdmin)
	repo := NewContentModerationRepository(integrationDB)
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 2, Window: 24 * time.Hour}
	race := func(t *testing.T, hit service.ContentModerationLog, adminWrite func() error) service.ContentModerationLog {
		t.Helper()
		start := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			<-start
			done <- adminWrite()
		}()
		close(start)
		result, hitErr := repo.RecordHit(ctx, &hit, rules)
		require.NoError(t, <-done)
		require.NoError(t, hitErr)
		require.True(t, result.Inserted)
		return hit
	}
	t.Run("switch from user to key", func(t *testing.T) {
		u := penaltyUser(t, service.RoleUser)
		k1, k2 := penaltyKey(t, u), penaltyKey(t, u)
		recordPenaltyHit(t, penaltyHit(u, k1), rules)
		old := penaltyHit(u, k2)
		old.OccurredAt = old.OccurredAt.Add(-time.Second) // definitely before the scope boundary
		saved := race(t, old, func() error { return setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID) })
		user, err := NewUserRepository(integrationEntClient, integrationDB).GetByID(ctx, u.ID)
		require.NoError(t, err)
		require.Equal(t, service.ModerationScopeAPIKey, user.ModerationPenaltyScope)
		switch saved.PenaltyScope {
		case service.ModerationScopeUser: // the original scope reached its threshold first
			require.Equal(t, service.ModerationPenaltyUserDisabled, saved.PenaltyAction)
			require.Equal(t, service.StatusDisabled, user.Status)
		case "": // the committed switch made the old event audit-only
			require.Zero(t, saved.ViolationCount)
			require.Empty(t, saved.PenaltyAction)
			require.Equal(t, service.StatusActive, user.Status)
		default:
			t.Fatalf("pre-switch event counted in scope %q", saved.PenaltyScope)
		}
		require.Zero(t, penaltyInt(t, `SELECT count(*) FROM api_keys WHERE user_id=$1 AND moderation_locked_at IS NOT NULL`, u.ID))
	})
	t.Run("unlock and delayed hit", func(t *testing.T) {
		u := penaltyUser(t, service.RoleUser)
		k := penaltyKey(t, u)
		require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
		recordPenaltyHit(t, penaltyHit(u, k), rules)
		locked := recordPenaltyHit(t, penaltyHit(u, k), rules)
		require.Equal(t, service.ModerationPenaltyKeyLocked, locked.PenaltyAction)
		saved := race(t, penaltyHit(u, k), func() error {
			_, err := repo.UnlockModerationKey(ctx, u.ID, k.ID, operator.ID)
			return err
		})
		require.Empty(t, saved.PenaltyAction)
		fresh := recordPenaltyHit(t, penaltyHit(u, k), rules)
		require.Equal(t, 1, fresh.ViolationCount)
		require.Empty(t, fresh.PenaltyAction)
		require.Equal(t, 1, penaltyInt(t, `SELECT count(*) FROM api_keys WHERE id=$1 AND moderation_locked_at IS NULL AND moderation_last_unlocked_at IS NOT NULL`, k.ID))
	})
}

func TestModerationPenaltyEligibility(t *testing.T) {
	ctx := context.Background()
	u, outsider := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	k, deleted, foreign := penaltyKey(t, u), penaltyKey(t, u), penaltyKey(t, outsider)
	keys := NewAPIKeyRepository(integrationEntClient, integrationDB)
	require.NoError(t, keys.Delete(ctx, deleted.ID))
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 1, Window: 24 * time.Hour, ExcludeCyber: true}
	for _, scope := range []string{service.ModerationScopeUser, service.ModerationScopeAPIKey} {
		t.Run(scope, func(t *testing.T) {
			if scope == service.ModerationScopeAPIKey {
				require.NoError(t, setPenaltyScope(ctx, u, scope, outsider.ID))
			}
			cases := []struct {
				name   string
				adjust func(*service.ContentModerationLog)
			}{
				{"missing user", func(l *service.ContentModerationLog) { l.UserID = nil }},
				{"missing key", func(l *service.ContentModerationLog) { l.APIKeyID = nil }},
				{"foreign key", func(l *service.ContentModerationLog) { l.APIKeyID = &foreign.ID }},
				{"deleted key", func(l *service.ContentModerationLog) { l.APIKeyID = &deleted.ID }},
				{"outside window", func(l *service.ContentModerationLog) { l.OccurredAt = time.Now().Add(-25 * time.Hour) }},
				{"hash block", func(l *service.ContentModerationLog) { l.Action = service.ContentModerationActionHashBlock }},
				{"trusted upstream audit", func(l *service.ContentModerationLog) { l.Mode = service.ContentModerationModeCyberLogOnly }},
				{"trusted local audit", func(l *service.ContentModerationLog) { l.Mode = service.ContentModerationModeRiskControlLogOnly }},
				{"excluded upstream event", func(l *service.ContentModerationLog) { l.Action = service.ContentModerationActionCyberPolicy }},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					hit := penaltyHit(u, k)
					tc.adjust(&hit)
					saved := recordPenaltyHit(t, hit, rules)
					require.Zero(t, saved.ViolationCount)
					require.Empty(t, saved.PenaltyAction)
				})
			}
			disabled := rules
			disabled.AutoBan = false
			saved := recordPenaltyHit(t, penaltyHit(u, k), disabled)
			require.Equal(t, 1, saved.ViolationCount)
			require.Empty(t, saved.PenaltyAction)
			state, err := keys.GetModerationState(ctx, k.ID, u.ID)
			require.NoError(t, err)
			require.Equal(t, service.StatusActive, state.UserStatus)
			require.Nil(t, state.LockedAt)
		})
	}
	saved := recordPenaltyHit(t, penaltyHit(outsider, foreign), rules)
	require.Equal(t, 1, saved.ViolationCount)
	require.Empty(t, saved.PenaltyAction)
	state, err := keys.GetModerationState(ctx, foreign.ID, outsider.ID)
	require.NoError(t, err)
	require.Nil(t, state.LockedAt)
	require.Equal(t, service.StatusActive, state.UserStatus)
}

func TestModerationPenaltyLegacyHistoryAndLogView(t *testing.T) {
	ctx := context.Background()
	u, operator := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	k := penaltyKey(t, u)
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 2, Window: 24 * time.Hour}
	oldHit := func(banned bool, hours int) {
		t.Helper()
		_, err := integrationDB.Exec(`INSERT INTO content_moderation_logs(user_id, api_key_id, user_email, flagged, mode, action, auto_banned, violation_count, created_at) VALUES ($1,$2,$3,true,'pre_block','block',$4,1,$5)`, u.ID, k.ID, u.Email, banned, time.Now().Add(-time.Duration(hours)*time.Hour))
		require.NoError(t, err)
	}
	oldHit(false, 4)
	oldHit(true, 3)
	oldHit(false, 2)
	saved := recordPenaltyHit(t, penaltyHit(u, k), rules)
	require.Equal(t, 2, saved.ViolationCount)
	require.Equal(t, service.ModerationPenaltyUserDisabled, saved.PenaltyAction)
	require.True(t, saved.AutoBanned)
	repo := NewContentModerationRepository(integrationDB)
	logs, _, err := repo.ListLogs(ctx, service.ContentModerationLogFilter{Search: u.Email})
	require.NoError(t, err)
	require.Len(t, logs, 3) // Only legacy rows above carry this exact fixture email.
	for _, row := range logs {
		require.Equal(t, service.ModerationScopeUser, row.PenaltyScope)
		require.False(t, row.APIKeyLocked)
		if row.AutoBanned {
			require.Equal(t, service.ModerationPenaltyUserDisabled, row.PenaltyAction)
		} else {
			require.Empty(t, row.PenaltyAction)
		}
	}
	u = penaltyUser(t, service.RoleUser)
	k = penaltyKey(t, u)
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	rules.Threshold = 1
	event := penaltyHit(u, k)
	event.RequestID = uuid.NewString()
	recordPenaltyHit(t, event, rules)
	checkView := func(locked bool) {
		t.Helper()
		logs, _, err := repo.ListLogs(ctx, service.ContentModerationLogFilter{Search: event.RequestID})
		require.NoError(t, err)
		require.Len(t, logs, 1)
		row := logs[0]
		require.Equal(t, service.ModerationScopeAPIKey, row.PenaltyScope)
		require.Equal(t, service.ModerationPenaltyKeyLocked, row.PenaltyAction)
		require.False(t, row.AutoBanned)
		require.Equal(t, locked, row.APIKeyLocked)
	}
	checkView(true)
	_, err = repo.UnlockModerationKey(ctx, u.ID, k.ID, operator.ID)
	require.NoError(t, err)
	checkView(false)
}

func TestModerationPenaltyTimedKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	u, operator := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	key, sibling := penaltyKey(t, u), penaltyKey(t, u)
	repo := NewContentModerationRepository(integrationDB)
	keys := NewAPIKeyRepository(integrationEntClient, integrationDB)
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 2, Window: 24 * time.Hour}
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	require.Equal(t, 1, recordPenaltyHit(t, penaltyHit(u, key), rules).ViolationCount)
	require.NoError(t, setPenaltyDuration(ctx, u, 10, operator.ID))
	event := penaltyHit(u, key)
	event.RequestID = uuid.NewString()
	locked := recordPenaltyHit(t, event, rules)
	require.Equal(t, 2, locked.ViolationCount)
	require.Equal(t, service.ModerationPenaltyKeyLocked, locked.PenaltyAction)
	require.NotNil(t, locked.PenaltyLockedUntil)
	state, err := keys.GetModerationState(ctx, key.ID, u.ID)
	require.NoError(t, err)
	require.NotNil(t, state.LockedAt)
	require.Equal(t, 10*time.Minute, locked.PenaltyLockedUntil.Sub(*state.LockedAt))

	// Future policy edits and in-flight hits do not extend or shorten an active lock.
	require.NoError(t, setPenaltyDuration(ctx, u, 37, operator.ID))
	again := recordPenaltyHit(t, penaltyHit(u, key), rules)
	require.Empty(t, again.PenaltyAction)
	require.Nil(t, again.PenaltyLockedUntil)
	stored, err := keys.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.True(t, locked.PenaltyLockedUntil.Equal(*stored.ModerationLockedUntil))
	siblingState, err := keys.GetModerationState(ctx, sibling.ID, u.ID)
	require.NoError(t, err)
	require.Nil(t, siblingState.LockedAt)
	require.Equal(t, service.StatusActive, siblingState.UserStatus)

	// Bring the deadline near, then wait for database time to pass it without cleanup, a request or a worker.
	var due time.Time
	err = integrationDB.QueryRow(`UPDATE api_keys SET moderation_locked_at=clock_timestamp()-interval '1 minute',
		moderation_locked_until=clock_timestamp()+interval '300 milliseconds' WHERE id=$1 RETURNING moderation_locked_until`, key.ID).Scan(&due)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return penaltyInt(t, `SELECT count(*) FROM api_keys WHERE id=$1 AND moderation_locked_until <= clock_timestamp()`, key.ID) == 1
	}, 5*time.Second, 20*time.Millisecond)
	credential, err := repo.UnlockModerationKey(ctx, u.ID, key.ID, operator.ID)
	require.NoError(t, err)
	require.Empty(t, credential) // late admin click is idempotent and does not move the boundary
	state, err = keys.GetModerationState(ctx, key.ID, u.ID)
	require.NoError(t, err)
	require.Nil(t, state.LockedAt)
	require.NotNil(t, state.LastUnlockedAt)
	require.True(t, due.Equal(*state.LastUnlockedAt))
	logs, _, err := repo.ListLogs(ctx, service.ContentModerationLogFilter{Search: event.RequestID})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.False(t, logs[0].APIKeyLocked)
	require.True(t, locked.PenaltyLockedUntil.Equal(*logs[0].PenaltyLockedUntil)) // immutable history
	duplicate, err := repo.RecordHit(ctx, &event, rules)
	require.NoError(t, err)
	require.False(t, duplicate.Inserted)
	late := penaltyHit(u, key)
	late.OccurredAt = due
	require.Zero(t, recordPenaltyHit(t, late, rules).ViolationCount)
	first := penaltyHit(u, key)
	first.OccurredAt = due.Add(time.Microsecond)
	require.Equal(t, 1, recordPenaltyHit(t, first, rules).ViolationCount)
	second := penaltyHit(u, key)
	second.OccurredAt = due.Add(2 * time.Microsecond)
	relocked := recordPenaltyHit(t, second, rules)
	require.Equal(t, 2, relocked.ViolationCount)
	state, err = keys.GetModerationState(ctx, key.ID, u.ID)
	require.NoError(t, err)
	require.NotNil(t, state.LockedAt)
	require.NotNil(t, relocked.PenaltyLockedUntil)
	require.Equal(t, 37*time.Minute, relocked.PenaltyLockedUntil.Sub(*state.LockedAt))
	require.True(t, due.Equal(*state.LastUnlockedAt))
	credential, err = repo.UnlockModerationKey(ctx, u.ID, key.ID, operator.ID)
	require.NoError(t, err)
	require.Equal(t, key.Key, credential)
	stored, err = keys.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, stored.ModerationLockedUntil)
	require.Nil(t, stored.ModerationLockedAt)
	require.True(t, stored.ModerationLastUnlockedAt.Before(*relocked.PenaltyLockedUntil))

	// Leaving per-key mode resets the preference. The next lock in per-key mode defaults to indefinite.
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeUser, operator.ID))
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	rules.Threshold = 1
	indefinite := recordPenaltyHit(t, penaltyHit(u, sibling), rules)
	require.Equal(t, service.ModerationPenaltyKeyLocked, indefinite.PenaltyAction)
	require.Nil(t, indefinite.PenaltyLockedUntil)
	require.Equal(t, 1, penaltyInt(t, `SELECT COUNT(*) FROM moderation_admin_actions WHERE user_id=$1 AND actor_id=$2 AND action='change_lock_minutes' AND old_value='37' AND new_value='0'`, u.ID, operator.ID))

	// A member may resume regular management of a due key even before a gateway request materializes the unlock.
	require.NoError(t, setPenaltyDuration(ctx, u, 1, operator.ID))
	for _, audit := range []bool{false, true} {
		k := penaltyKey(t, u)
		recordPenaltyHit(t, penaltyHit(u, k), rules)
		require.ErrorIs(t, keys.Delete(ctx, k.ID), service.ErrAPIKeyModerationLocked)
		makePenaltyDue(t, k.ID)
		if audit {
			require.NoError(t, keys.DeleteWithAudit(ctx, k.ID))
		} else {
			require.NoError(t, keys.Delete(ctx, k.ID))
		}
	}
}

func TestModerationPenaltyUnlockPreservesKeyRestrictions(t *testing.T) {
	ctx := context.Background()
	u, operator := penaltyUser(t, service.RoleUser), penaltyUser(t, service.RoleAdmin)
	require.NoError(t, setPenaltyScope(ctx, u, service.ModerationScopeAPIKey, operator.ID))
	require.NoError(t, setPenaltyDuration(ctx, u, 10, operator.ID))
	repo := NewContentModerationRepository(integrationDB)
	rules := service.ModerationPenaltyRules{AutoBan: true, Threshold: 1, Window: 24 * time.Hour}
	keys := NewAPIKeyRepository(integrationEntClient, integrationDB)
	cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{L1Size: 100, L1TTLSeconds: 180, L2TTLSeconds: 180}}
	cache := NewAPIKeyCache(testRedis(t))
	users := NewUserRepository(integrationEntClient, integrationDB)
	first := service.NewAPIKeyService(keys, users, nil, nil, nil, cache, cfg)
	second := service.NewAPIKeyService(keys, users, nil, nil, nil, cache, cfg)
	router := gin.New()
	router.GET("/api", gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(second, nil, cfg)), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	past := time.Now().Add(-time.Hour)
	for _, tc := range []struct {
		name   string
		apply  func(*service.APIKey)
		status int
		reason string
	}{
		{"manual disable", func(k *service.APIKey) { k.Status = service.StatusDisabled }, 401, "API_KEY_DISABLED"},
		{"expired", func(k *service.APIKey) { k.ExpiresAt = &past }, 403, "API_KEY_EXPIRED"},
		{"quota exhausted", func(k *service.APIKey) { k.Quota, k.QuotaUsed = 5, 5 }, 429, "API_KEY_QUOTA_EXHAUSTED"},
	} {
		for _, mode := range []string{"manual unlock", "automatic expiry"} {
			for _, when := range []string{"before lock", "while locked"} {
				t.Run(tc.name+"/"+when+"/"+mode, func(t *testing.T) {
					k := penaltyKey(t, u)
					check := func(code int, reason string) {
						t.Helper()
						req := httptest.NewRequest("GET", "/api", nil)
						req.Header.Set("Authorization", "Bearer "+k.Key)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)
						require.Equal(t, code, w.Code, w.Body.String())
						if reason != "" {
							require.Contains(t, w.Body.String(), reason)
						}
					}
					restrict := func() {
						t.Helper()
						tc.apply(k)
						require.NoError(t, keys.Update(ctx, k, service.APIKeyUpdateFields{Status: true, ExpiresAt: true, Quota: true, QuotaUsed: true}))
					}
					_, err := first.GetByKey(ctx, k.Key)
					require.NoError(t, err)
					check(204, "") // second instance authenticates from the warm Redis snapshot
					if when == "before lock" {
						restrict()
					}
					recordPenaltyHit(t, penaltyHit(u, k), rules)
					if when == "while locked" {
						restrict()
					}
					check(401, "API_KEY_DISABLED")
					if mode == "manual unlock" {
						_, err = repo.UnlockModerationKey(ctx, u.ID, k.ID, operator.ID)
						require.NoError(t, err)
					} else {
						makePenaltyDue(t, k.ID)
					}
					// No eviction or pub/sub delivery. Prove the old, unrestricted snapshot
					// is still present immediately before the request arrives after unlock.
					cacheKey := fmt.Sprintf("%x", sha256.Sum256([]byte(k.Key)))
					stale, err := cache.GetAuthCache(ctx, cacheKey)
					require.NoError(t, err)
					require.Equal(t, service.StatusActive, stale.Snapshot.Status)
					require.Nil(t, stale.Snapshot.ExpiresAt)
					require.Zero(t, stale.Snapshot.Quota)
					require.Nil(t, stale.Snapshot.ModerationLastUnlockedAt)
					check(tc.status, tc.reason)
					fresh, err := cache.GetAuthCache(ctx, cacheKey)
					require.NoError(t, err)
					require.NotNil(t, fresh.Snapshot.ModerationLastUnlockedAt)
				})
			}
		}
	}
}
