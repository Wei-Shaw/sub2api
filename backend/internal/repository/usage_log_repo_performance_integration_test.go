//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountPerformanceRepositoryRollingHour(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "account-performance@test.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-account-performance", Name: "performance"})
	weighted := mustCreateAccount(t, client, &service.Account{Name: "performance-weighted"})
	other := mustCreateAccount(t, client, &service.Account{Name: "performance-other"})
	media := mustCreateAccount(t, client, &service.Account{Name: "performance-media"})
	invalid := mustCreateAccount(t, client, &service.Account{Name: "performance-invalid"})
	empty := mustCreateAccount(t, client, &service.Account{Name: "performance-empty"})
	end := time.Date(2026, 9, 16, 11, 37, 23, 123456000, time.UTC)
	start := end.Add(-time.Hour)
	middle := end.Add(-30 * time.Minute)
	intPointer := func(v int) *int { return &v }
	stringPointer := func(v string) *string { return &v }
	count := 0
	insert := func(accountID int64, at time.Time, change func(*service.UsageLog)) {
		t.Helper()
		count++
		log := &service.UsageLog{
			UserID: user.ID, APIKeyID: key.ID, AccountID: accountID,
			RequestID: fmt.Sprintf("performance-%d", count), Model: "text-model",
			RequestType: service.RequestTypeStream, Stream: true,
			InputTokens: 100, OutputTokens: 10, FirstTokenMs: intPointer(10), DurationMs: intPointer(110),
			CreatedAt: at,
		}
		if change != nil {
			change(log)
		}
		inserted, err := repo.Create(ctx, log)
		require.NoError(t, err)
		require.True(t, inserted)
	}
	insert(weighted.ID, start, nil)
	insert(weighted.ID, end.Add(-time.Microsecond), func(log *service.UsageLog) {
		log.InputTokens, log.CacheCreationTokens, log.CacheReadTokens = 10, 30, 60
		log.OutputTokens, log.DurationMs = 90, intPointer(310)
	})
	insert(weighted.ID, middle, func(log *service.UsageLog) {
		log.RequestType, log.Stream = service.RequestTypeSync, false
		log.InputTokens, log.CacheReadTokens, log.OutputTokens = 50, 150, 1000
		log.FirstTokenMs, log.DurationMs = intPointer(5), intPointer(500)
	})
	insert(weighted.ID, middle, func(log *service.UsageLog) {
		log.InputTokens, log.OutputTokens = 0, 100
		log.FirstTokenMs, log.DurationMs = intPointer(0), intPointer(100)
	})
	for _, at := range []time.Time{start.Add(-time.Microsecond), start.Add(-48 * time.Hour), end, end.Add(time.Microsecond)} {
		insert(weighted.ID, at, nil)
	}
	insert(other.ID, middle, func(log *service.UsageLog) {
		log.InputTokens, log.CacheReadTokens, log.OutputTokens = 10, 90, 20
		log.FirstTokenMs, log.DurationMs = intPointer(0), intPointer(100)
	})
	mediaCases := []func(*service.UsageLog){
		func(log *service.UsageLog) { log.ImageCount = 1 },
		func(log *service.UsageLog) { log.VideoCount = 1 },
		func(log *service.UsageLog) { log.ImageOutputTokens = 20 },
		func(log *service.UsageLog) { log.NativeCompactionV2 = true },
		func(log *service.UsageLog) { log.RequestType = service.RequestTypeCyberBlocked },
		func(log *service.UsageLog) { log.RequestType = service.RequestTypeLive },
		func(log *service.UsageLog) { log.BillingMode = stringPointer("image") },
		func(log *service.UsageLog) { log.InboundEndpoint = stringPointer("/v1/images/generations") },
		func(log *service.UsageLog) { log.UpstreamEndpoint = stringPointer("/v1/videos") },
		func(log *service.UsageLog) { log.InboundEndpoint = stringPointer("/v1/audio/transcriptions") },
		func(log *service.UsageLog) { log.UpstreamEndpoint = stringPointer("/v1/realtime") },
		func(log *service.UsageLog) { log.InboundEndpoint = stringPointer("/v1/live") },
		func(log *service.UsageLog) { log.UpstreamEndpoint = stringPointer("/v1/responses/compact") },
		func(log *service.UsageLog) {
			log.UpstreamEndpoint = stringPointer("/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent")
		},
	}
	for _, change := range mediaCases {
		insert(media.ID, middle, change)
	}
	invalidCases := []func(*service.UsageLog){
		func(log *service.UsageLog) { log.FirstTokenMs = nil },
		func(log *service.UsageLog) { log.FirstTokenMs = intPointer(-1) },
		func(log *service.UsageLog) { log.DurationMs = nil },
		func(log *service.UsageLog) { log.DurationMs = intPointer(-1) },
		func(log *service.UsageLog) { log.DurationMs = intPointer(0) },
		func(log *service.UsageLog) { log.DurationMs = intPointer(9) },
		func(log *service.UsageLog) { log.DurationMs = intPointer(10) },
		func(log *service.UsageLog) { log.OutputTokens = 0 },
		func(log *service.UsageLog) { log.InputTokens = -1 },
		func(log *service.UsageLog) { log.OutputTokens = -1 },
		func(log *service.UsageLog) { log.CacheReadTokens = -1 },
		func(log *service.UsageLog) { log.CacheCreationTokens = -1 },
	}
	for _, change := range invalidCases {
		insert(invalid.ID, middle, func(log *service.UsageLog) {
			log.InputTokens = 0
			change(log)
		})
	}
	stats, err := repo.GetAccountPerformanceStatsBatch(ctx,
		[]int64{weighted.ID, other.ID, media.ID, invalid.ID, empty.ID}, start, end)
	require.NoError(t, err)
	require.Len(t, stats, 5)
	require.Equal(t, int64(4), stats[weighted.ID].RequestCount)
	require.InDelta(t, 20.0/3, *stats[weighted.ID].TTFTMs, 1e-12)
	require.InDelta(t, 400, *stats[weighted.ID].TPS, 1e-12)
	require.InDelta(t, 0.525, *stats[weighted.ID].CacheRate, 1e-12)
	require.Equal(t, int64(3), stats[weighted.ID].TTFTSamples)
	require.Equal(t, int64(3), stats[weighted.ID].TPSSamples)
	require.Equal(t, int64(3), stats[weighted.ID].CacheSamples)
	require.True(t, end.Add(-time.Microsecond).Equal(*stats[weighted.ID].LastRequestAt))
	require.Equal(t, int64(1), stats[other.ID].RequestCount)
	require.Equal(t, float64(0), *stats[other.ID].TTFTMs)
	require.InDelta(t, 200, *stats[other.ID].TPS, 1e-12)
	require.InDelta(t, 0.9, *stats[other.ID].CacheRate, 1e-12)
	require.Equal(t, int64(len(mediaCases)), stats[media.ID].RequestCount)
	require.Nil(t, stats[media.ID].TTFTMs)
	require.Nil(t, stats[media.ID].TPS)
	require.Nil(t, stats[media.ID].CacheRate)
	require.True(t, middle.Equal(*stats[media.ID].LastRequestAt))
	require.Equal(t, int64(len(invalidCases)), stats[invalid.ID].RequestCount)
	require.Equal(t, int64(1), stats[invalid.ID].TTFTSamples)
	require.Equal(t, float64(10), *stats[invalid.ID].TTFTMs)
	require.Equal(t, int64(0), stats[invalid.ID].TPSSamples)
	require.Nil(t, stats[invalid.ID].TPS)
	require.Nil(t, stats[invalid.ID].CacheRate)
	require.Equal(t, &service.AccountPerformanceStats{}, stats[empty.ID])
}
