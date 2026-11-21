//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerCacheAccountOmitsExpandedGroups(t *testing.T) {
	for _, groupCount := range []int{1, 10, 100, 241} {
		t.Run(strconv.Itoa(groupCount), func(t *testing.T) {
			ctx := context.Background()
			cache := newSchedulerCacheUnit(t)
			now := time.Date(2026, time.November, 16, 12, 0, 0, 0, time.UTC)
			resetAt := now.Add(time.Hour)
			rate := 0.0
			proxyID := int64(5)
			expected := service.Account{
				ID: 8842, Name: "multi-group", Platform: service.PlatformOpenAI,
				Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{
					"access_token": "fixture-access", "refresh_token": "fixture-refresh",
					"openai_capabilities": []any{"responses"},
					"model_mapping":       map[string]any{"public-model": "upstream-model"},
				},
				Extra:   map[string]any{"custom_account_setting": "keep", "openai_oauth_passthrough": true},
				ProxyID: &proxyID, Proxy: &service.Proxy{ID: proxyID, Protocol: "http", Host: "proxy.example", Port: 8080},
				Concurrency: 3, Priority: 7, RateMultiplier: &rate,
				RateLimitResetAt: &resetAt, LastUsedAt: &now,
				CreatedAt: now, UpdatedAt: now,
			}
			for i := 1; i <= groupCount; i++ {
				expected.GroupIDs = append(expected.GroupIDs, int64(i))
				expected.AccountGroups = append(expected.AccountGroups, service.AccountGroup{
					AccountID: expected.ID, GroupID: int64(i), Priority: i, CreatedAt: now,
				})
			}
			account := expected
			account.AccountGroups = append([]service.AccountGroup(nil), expected.AccountGroups...)
			for i := range account.AccountGroups {
				group := &service.Group{
					ID: int64(i + 1), Name: fmt.Sprintf("group-%d", i+1),
					Description: strings.Repeat("unused group configuration ", 128),
				}
				account.Groups = append(account.Groups, group)
				account.AccountGroups[i].Group = group
				account.AccountGroups[i].Account = &service.Account{ID: account.ID, Name: "unused account edge"}
			}
			original, err := json.Marshal(account)
			require.NoError(t, err)

			// Both account updates and bucket rebuilds write the same global record.
			for _, write := range []string{"account", "snapshot"} {
				t.Run(write, func(t *testing.T) {
					if write == "account" {
						require.NoError(t, cache.SetAccount(ctx, &account))
					} else {
						bucket := service.SchedulerBucket{GroupID: 1, Platform: account.Platform, Mode: service.SchedulerModeSingle}
						token, err := cache.CaptureBucketWriteToken(ctx, bucket)
						require.NoError(t, err)
						require.NoError(t, cache.SetSnapshot(ctx, bucket, token, []service.Account{account}))
						snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
						require.NoError(t, err)
						require.True(t, hit)
						require.Len(t, snapshot, 1)
						require.Equal(t, expected.GroupIDs, snapshot[0].GroupIDs)
						require.Equal(t, expected.AccountGroups, snapshot[0].AccountGroups)
					}
					full, err := cache.GetAccount(ctx, account.ID)
					require.NoError(t, err)
					require.NotNil(t, full)
					require.Zero(t, len(full.Groups), "full cache must omit expanded groups")
					for _, membership := range full.AccountGroups {
						require.True(t, membership.Group == nil && membership.Account == nil, "full cache must omit relationship objects")
					}
					require.Equal(t, &expected, full, "hydration must retain account configuration and membership")
					payload, err := cache.rdb.Get(ctx, schedulerAccountKey(strconv.FormatInt(account.ID, 10))).Bytes()
					require.NoError(t, err)
					require.NotContains(t, string(payload), "unused group configuration")
					require.NotContains(t, string(payload), "unused account edge")
					require.Less(t, len(payload)*4, len(original))
					t.Logf("groups=%d expanded_bytes=%d cached_bytes=%d", groupCount, len(original), len(payload))
				})
			}

			// Serialization must not detach relationships from the repository's object.
			after, err := json.Marshal(account)
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
}

func TestSchedulerCacheReadsLegacyExpandedGroups(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	account := service.Account{
		ID: 8843, GroupIDs: []int64{2}, Groups: []*service.Group{{ID: 2, Name: "legacy"}},
		AccountGroups: []service.AccountGroup{{AccountID: 8843, GroupID: 2, Priority: 4, Group: &service.Group{ID: 2}}},
	}
	payload, err := json.Marshal(account)
	require.NoError(t, err)
	require.NoError(t, cache.rdb.Set(ctx, schedulerAccountKey("8843"), payload, 0).Err())
	full, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, &account, full, "rolling upgrades can still read records written by old instances")
}
