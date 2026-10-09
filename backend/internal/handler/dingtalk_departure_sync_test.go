package handler

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/require"
)

type dingTalkKeyInvalidationCache struct {
	service.APIKeyCache
	deleted, published int
}

func (c *dingTalkKeyInvalidationCache) DeleteAuthCache(context.Context, string) error {
	c.deleted++
	return nil
}
func (c *dingTalkKeyInvalidationCache) PublishAuthCacheInvalidation(context.Context, string) error {
	c.published++
	return nil
}

func TestDingTalkDepartureDisablesAllAPIKeys(t *testing.T) {
	for _, app := range []string{"default", "extra"} {
		t.Run(app, func(t *testing.T) {
			ctx := context.Background()
			directoryError := atomic.Value{}
			directoryError.Store("")
			h, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
			if app == "extra" {
				h.cfg.DingTalk.Apps = []config.DingTalkAppConfig{{ID: app, Name: app, Enabled: true, ClientID: app, ClientSecret: "secret", RedirectURL: h.cfg.DingTalk.RedirectURL}}
			}
			callDingTalkCallback(t, h, app)
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
			require.NoError(t, err)
			userID := identity.UserID
			keys := []string{"departure-active", "departure-expired", "departure-quota", "departure-disabled"}
			for i, status := range []string{service.StatusAPIKeyActive, service.StatusAPIKeyExpired, service.StatusAPIKeyQuotaExhausted, service.StatusAPIKeyDisabled} {
				_, err := client.APIKey.Create().SetUserID(userID).SetKey(keys[i]).SetName(keys[i]).SetStatus(status).SetQuotaUsed(12).Save(ctx)
				require.NoError(t, err)
			}
			deleted, err := client.APIKey.Create().SetUserID(userID).SetKey("deleted-key").SetName("deleted").SetDeletedAt(time.Now()).Save(ctx)
			require.NoError(t, err)
			other, err := client.User.Create().SetEmail("other@example.com").SetPasswordHash("hash").Save(ctx)
			require.NoError(t, err)
			otherApp := "other-app"
			_, err = client.AuthIdentity.Create().SetUserID(other.ID).SetProviderType("dingtalk").SetProviderKey(service.DingTalkProviderKey(otherApp)).SetProviderSubject(identity.ProviderSubject).Save(ctx)
			require.NoError(t, err)
			otherKey, err := client.APIKey.Create().SetUserID(other.ID).SetKey("other-user-key").SetName("other").Save(ctx)
			require.NoError(t, err)
			cache := &dingTalkKeyInvalidationCache{}
			apiKeys := service.NewAPIKeyService(repository.NewAPIKeyRepository(client, nil), nil, nil, nil, nil, cache, nil)
			organization := NewDingTalkOrganizationHandler(nil, h.settingSvc, h.userService, h, apiKeys)
			_, err = apiKeys.GetByKey(ctx, keys[0])
			require.NoError(t, err)
			directoryError.Store(`{"errcode":60121,"errmsg":"not in directory"}`)

			// Gateway authentication must not query DingTalk, even after departure.
			beforeSync, err := apiKeys.GetByKey(ctx, keys[0])
			require.NoError(t, err)
			require.Equal(t, service.StatusAPIKeyActive, beforeSync.Status)
			require.NoError(t, organization.reconcileDepartedMembers(ctx, app, nil))

			stored, err := client.APIKey.Query().Where(apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).All(ctx)
			require.NoError(t, err)
			require.Len(t, stored, 4)
			disabledUser, err := client.User.Get(ctx, userID)
			require.NoError(t, err)
			require.Equal(t, service.StatusDisabled, disabledUser.Status)
			for _, key := range stored {
				require.Equal(t, service.StatusAPIKeyDisabled, key.Status)
				require.Equal(t, float64(12), key.QuotaUsed)
			}
			require.Equal(t, 4, cache.deleted)
			require.Equal(t, 4, cache.published)
			for _, id := range []int64{deleted.ID, otherKey.ID} {
				key, err := client.APIKey.Get(mixins.SkipSoftDelete(ctx), id)
				require.NoError(t, err)
				require.Equal(t, service.StatusAPIKeyActive, key.Status)
			}
			directoryError.Store("")
			require.NoError(t, organization.reconcileDepartedMembers(ctx, app, []service.DingTalkDirectoryMember{{UnionID: identity.ProviderSubject}}))
			key, err := apiKeys.GetByKey(ctx, keys[0])
			require.NoError(t, err)
			require.Equal(t, service.StatusAPIKeyDisabled, key.Status, "membership recovery must not reactivate disabled keys")
		})
	}
}

func TestDingTalkTemporaryFailureDoesNotDisableAPIKeys(t *testing.T) {
	for _, upstreamError := range []string{`{"errcode":88,"errmsg":"rate limited"}`, `{"errcode":60011,"errmsg":"permission denied"}`, `invalid json`} {
		t.Run(upstreamError, func(t *testing.T) {
			ctx := context.Background()
			directoryError := atomic.Value{}
			directoryError.Store("")
			h, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
			callDingTalkCallback(t, h)
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
			require.NoError(t, err)
			key, err := client.APIKey.Create().SetUserID(identity.UserID).SetKey("temporary-error-key").SetName("key").Save(ctx)
			require.NoError(t, err)
			directoryError.Store(upstreamError)
			organization := NewDingTalkOrganizationHandler(nil, h.settingSvc, h.userService, h, nil)
			require.Error(t, organization.reconcileDepartedMembers(ctx, "default", nil))
			stored, err := client.APIKey.Get(ctx, key.ID)
			require.NoError(t, err)
			require.Equal(t, service.StatusAPIKeyActive, stored.Status)
			directoryError.Store("")
			require.NoError(t, h.validateDingTalkUserMembership(ctx, identity.UserID))
		})
	}
}

func TestDingTalkDailySyncSchedule(t *testing.T) {
	schedule, err := cron.ParseStandard(dingTalkDailySyncSchedule)
	require.NoError(t, err)
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	before := time.Date(2026, 10, 9, 3, 22, 59, 0, loc)
	first := time.Date(2026, 10, 9, 3, 23, 0, 0, loc)
	require.Equal(t, first, schedule.Next(before))
	require.Equal(t, first.AddDate(0, 0, 1), schedule.Next(first))
}

func TestDingTalkCurrentMembersKeepAPIKeys(t *testing.T) {
	for _, inSnapshot := range []bool{true, false} {
		t.Run(fmt.Sprint(inSnapshot), func(t *testing.T) {
			ctx := context.Background()
			directoryError := atomic.Value{}
			directoryError.Store("")
			auth, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
			callDingTalkCallback(t, auth)
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
			require.NoError(t, err)
			key, err := client.APIKey.Create().SetUserID(identity.UserID).SetKey("current-member-key").SetName("key").Save(ctx)
			require.NoError(t, err)
			var members []service.DingTalkDirectoryMember
			if inSnapshot {
				members = []service.DingTalkDirectoryMember{{UnionID: identity.ProviderSubject}}
				directoryError.Store(`{"errcode":88,"errmsg":"should not query an existing member"}`)
			}
			organization := NewDingTalkOrganizationHandler(nil, auth.settingSvc, auth.userService, auth, nil)
			require.NoError(t, organization.reconcileDepartedMembers(ctx, "default", members))
			stored, err := client.APIKey.Get(ctx, key.ID)
			require.NoError(t, err)
			require.Equal(t, service.StatusAPIKeyActive, stored.Status)
		})
	}
}

func TestDingTalkDailySyncStartsOnlyEnabledApps(t *testing.T) {
	directoryError := atomic.Value{}
	directoryError.Store("")
	auth, _ := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
	auth.cfg.DingTalk.Apps = []config.DingTalkAppConfig{
		{ID: "enabled", Name: "Enabled", Enabled: true, ClientID: "enabled", ClientSecret: "secret", RedirectURL: auth.cfg.DingTalk.RedirectURL},
		{ID: "disabled", Name: "Disabled", Enabled: false, ClientID: "disabled", ClientSecret: "secret", RedirectURL: auth.cfg.DingTalk.RedirectURL},
	}
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	for _, app := range []string{"default", "enabled"} {
		// An existing lease avoids starting background network calls in this test.
		mock.ExpectQuery("INSERT INTO dingtalk_sync_jobs").WithArgs(app, sqlmock.AnyArg()).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("SELECT job_id").WithArgs(app).WillReturnRows(sqlmock.NewRows([]string{"job_id", "status", "started_at", "finished_at", "error", "departments", "members"}).AddRow("existing", "running", time.Now(), nil, "", 0, 0))
	}
	h := NewDingTalkOrganizationHandler(db, auth.settingSvc, auth.userService, auth, nil)
	h.startDailyOrganizationSync(context.Background())
	require.NoError(t, mock.ExpectationsWereMet())
}
