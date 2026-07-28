//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func seedGrokMediaCleanupRecords(ownerRepo *grokVideoOwnerRepoStub, imageRepo *grokImageCreateRepoStub, expired, live int) {
	now := time.Now()
	if ownerRepo.owners == nil {
		ownerRepo.owners = make(map[string]GrokMediaVideoRequestOwner)
	}
	if ownerRepo.creates == nil {
		ownerRepo.creates = make(map[string]GrokMediaVideoCreateRecord)
	}
	if imageRepo.creates == nil {
		imageRepo.creates = make(map[string]GrokMediaImageCreateRecord)
	}
	for i := 0; i < expired+live; i++ {
		expiresAt := now.Add(-time.Hour)
		if i >= expired {
			expiresAt = now.Add(time.Hour)
		}
		owner := GrokMediaVideoRequestOwner{
			RequestID: fmt.Sprintf("video-%d", i), UserID: int64(i + 1), APIKeyID: 51,
			GroupID: 7, AccountID: 63, ExpiresAt: expiresAt,
		}
		ownerRepo.owners[grokVideoOwnerRepoKey(owner.RequestID, owner.UserID, owner.APIKeyID, owner.GroupID)] = owner
		videoCreate := GrokMediaVideoCreateRecord{
			UserID: int64(i + 1), APIKeyID: 51, GroupID: 7,
			Endpoint: GrokMediaEndpointVideosGenerations, IdempotencyKeyHash: fmt.Sprintf("%064x", i+1),
			ExpiresAt: expiresAt,
		}
		ownerRepo.creates[grokVideoCreateRepoKey(videoCreate)] = videoCreate
		imageCreate := GrokMediaImageCreateRecord{
			UserID: int64(i + 1), APIKeyID: 51, GroupID: 7,
			Endpoint: GrokMediaEndpointImagesGenerations, IdempotencyKeyHash: fmt.Sprintf("%064x", i+1),
			ExpiresAt: expiresAt,
		}
		imageRepo.creates[grokImageCreateRepoKey(imageCreate)] = imageCreate
	}
}

func TestCleanupGrokMediaExpiredRecordsBatchesAllTypesAndPreservesLiveRows(t *testing.T) {
	ownerRepo := &grokVideoOwnerRepoStub{}
	imageRepo := &grokImageCreateRepoStub{}
	seedGrokMediaCleanupRecords(ownerRepo, imageRepo, 237, 3)
	svc := &OpenAIGatewayService{
		grokMediaVideoOwnerRepo:  ownerRepo,
		grokMediaImageCreateRepo: imageRepo,
	}

	for _, expected := range []int64{100, 100, 37, 0} {
		stats := svc.CleanupGrokMediaExpiredRecords(context.Background())
		require.Equal(t, expected, stats.OwnerDeleted)
		require.Equal(t, expected, stats.VideoCreateDeleted)
		require.Equal(t, expected, stats.ImageCreateDeleted)
		require.GreaterOrEqual(t, stats.Duration, time.Duration(0))
	}
	require.Len(t, ownerRepo.owners, 3, "unexpired owners must remain")
	require.Len(t, ownerRepo.creates, 3, "unexpired video creates must remain")
	require.Len(t, imageRepo.creates, 3, "unexpired image creates must remain")
	for _, owner := range ownerRepo.owners {
		require.True(t, owner.ExpiresAt.After(time.Now()))
	}
	for _, record := range ownerRepo.creates {
		require.True(t, record.ExpiresAt.After(time.Now()))
	}
	for _, record := range imageRepo.creates {
		require.True(t, record.ExpiresAt.After(time.Now()))
	}
	require.Equal(t, 4, ownerRepo.ownerCleanupCalls)
	require.Equal(t, 4, ownerRepo.videoCleanupCalls)
	require.Equal(t, 4, imageRepo.cleanupCalls)
}

func TestCleanupGrokMediaExpiredRecordsTwoWorkersDoNotDuplicateOrBlock(t *testing.T) {
	ownerRepo := &grokVideoOwnerRepoStub{}
	imageRepo := &grokImageCreateRepoStub{}
	seedGrokMediaCleanupRecords(ownerRepo, imageRepo, 150, 0)
	svc := &OpenAIGatewayService{
		grokMediaVideoOwnerRepo:  ownerRepo,
		grokMediaImageCreateRepo: imageRepo,
	}
	start := make(chan struct{})
	results := make(chan GrokMediaExpiredCleanupStats, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- svc.CleanupGrokMediaExpiredRecords(context.Background())
		}()
	}
	close(start)
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("two cleanup workers blocked")
	}
	close(results)
	var ownerDeleted, videoDeleted, imageDeleted int64
	for stats := range results {
		ownerDeleted += stats.OwnerDeleted
		videoDeleted += stats.VideoCreateDeleted
		imageDeleted += stats.ImageCreateDeleted
	}
	require.Equal(t, int64(150), ownerDeleted)
	require.Equal(t, int64(150), videoDeleted)
	require.Equal(t, int64(150), imageDeleted)
	require.Empty(t, ownerRepo.owners)
	require.Empty(t, ownerRepo.creates)
	require.Empty(t, imageRepo.creates)
}

func TestCleanupGrokMediaExpiredRecordsFailureOnlyWarnsWithoutResponseEcho(t *testing.T) {
	const sensitive = `response_body={"secret":"must-not-log"}`
	ownerRepo := &grokVideoOwnerRepoStub{
		ownerCleanupErr: errors.New(sensitive),
		videoCleanupErr: errors.New(sensitive),
	}
	imageRepo := &grokImageCreateRepoStub{cleanupErr: errors.New(sensitive)}
	svc := &OpenAIGatewayService{
		grokMediaVideoOwnerRepo:  ownerRepo,
		grokMediaImageCreateRepo: imageRepo,
	}
	core, observed := observer.New(zap.DebugLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))

	stats := svc.CleanupGrokMediaExpiredRecords(ctx)
	require.Zero(t, stats.OwnerDeleted)
	require.Zero(t, stats.VideoCreateDeleted)
	require.Zero(t, stats.ImageCreateDeleted)
	require.Equal(t, 3, observed.FilterMessage("grok_media.expired_cleanup_failed").Len())
	require.Equal(t, 1, observed.FilterMessage("grok_media.expired_cleanup_completed").Len())
	for _, entry := range observed.All() {
		serialized := entry.Message + fmt.Sprint(entry.ContextMap())
		require.NotContains(t, serialized, sensitive)
		require.NotContains(t, serialized, "response_body")
	}
}
