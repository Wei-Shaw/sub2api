//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshotConfiguredMembership(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformAnthropic, service.PlatformGemini} {
		t.Run(platform, func(t *testing.T) {
			ctx := context.Background()
			tx := testEntTx(t)
			client := tx.Client()
			repo := newAccountRepositoryWithSQL(client, tx, nil)
			group := mustCreateGroup(t, client, &service.Group{Name: "snapshot-pool", Platform: platform})
			other := mustCreateGroup(t, client, &service.Group{Name: "snapshot-other", Platform: platform})
			future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
			ids := make(map[int64]map[string]int64)
			for _, gid := range []int64{0, group.ID, other.ID} {
				ids[gid] = make(map[string]int64)
				for _, kind := range []string{"ready", "cooling", "expired", "manual", "disabled", "deleted", "other_platform", "mixed_on", "mixed_off"} {
					accountPlatform := platform
					if kind == "mixed_on" || kind == "mixed_off" {
						accountPlatform = service.PlatformAntigravity
					}
					if kind == "other_platform" {
						accountPlatform = service.PlatformGrok
						if platform == accountPlatform {
							accountPlatform = service.PlatformOpenAI
						}
					}
					create := client.Account.Create().SetName(fmt.Sprintf("%s-%d", kind, gid)).
						SetPlatform(accountPlatform).SetType(service.AccountTypeAPIKey).
						SetCredentials(map[string]any{}).SetExtra(map[string]any{"mixed_scheduling": kind == "mixed_on"}).
						SetStatus(service.StatusActive).SetSchedulable(kind != "manual")
					switch kind {
					case "cooling":
						create.SetRateLimitResetAt(future).SetOverloadUntil(future).SetTempUnschedulableUntil(future)
					case "expired":
						create.SetExpiresAt(past).SetAutoPauseOnExpired(true)
					case "disabled":
						create.SetStatus(service.StatusDisabled)
					case "deleted":
						create.SetDeletedAt(past)
					}
					account, err := create.Save(ctx)
					require.NoError(t, err)
					ids[gid][kind] = account.ID
					if gid > 0 {
						_, err = client.AccountGroup.Create().SetAccountID(account.ID).SetGroupID(gid).Save(ctx)
						require.NoError(t, err)
					}
				}
			}

			for _, scope := range []string{"simple", "group", "ungrouped"} {
				for _, forced := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/forced=%t", scope, forced), func(t *testing.T) {
						cfg := &config.Config{RunMode: config.RunModeStandard}
						cfg.Gateway.Scheduling.DbFallbackEnabled = true
						var groupID *int64
						groups := []int64{0}
						if scope == "group" {
							groupID, groups = &group.ID, []int64{group.ID}
						}
						if scope == "simple" {
							cfg.RunMode = config.RunModeSimple
							groupID, groups = &group.ID, []int64{0, group.ID, other.ID}
						}
						mixed := platform != service.PlatformOpenAI && !forced
						var wantReady, wantMembers []int64
						for _, gid := range groups {
							wantReady = append(wantReady, ids[gid]["ready"])
							wantMembers = append(wantMembers, ids[gid]["ready"], ids[gid]["cooling"], ids[gid]["expired"])
							if mixed {
								wantReady = append(wantReady, ids[gid]["mixed_on"])
								wantMembers = append(wantMembers, ids[gid]["mixed_on"])
							}
						}
						cache := NewSchedulerCache(testRedis(t))
						svc := service.NewSchedulerSnapshotService(cache, nil, repo, nil, cfg)
						accountIDs := func(accounts []service.Account) []int64 {
							out := make([]int64, len(accounts))
							for i := range accounts {
								out[i] = accounts[i].ID
							}
							return out
						}
						for range 2 {
							accounts, useMixed, err := svc.ListSchedulableAccounts(ctx, groupID, platform, forced)
							require.NoError(t, err)
							require.Equal(t, mixed, useMixed)
							require.ElementsMatch(t, wantReady, accountIDs(accounts), "both DB fallback and cache hit enforce current eligibility")
						}
						bucket := service.SchedulerBucket{Platform: platform, Mode: service.SchedulerModeSingle}
						if scope == "group" {
							bucket.GroupID = group.ID
						}
						if forced {
							bucket.Mode = service.SchedulerModeForced
						} else if mixed {
							bucket.Mode = service.SchedulerModeMixed
						}
						members, hit, err := cache.GetSnapshot(ctx, bucket)
						require.NoError(t, err)
						require.True(t, hit)
						memberIDs := make([]int64, len(members))
						for i := range members {
							memberIDs[i] = members[i].ID
						}
						require.ElementsMatch(t, wantMembers, memberIDs, "the stored pool retains timed blocks without widening platform/group or manual eligibility")
					})
				}
			}
		})
	}
}
