package repository

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type reviewSettings struct {
	service.SettingRepository
	ttl string
}

func (s *reviewSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyCyberSessionBlockEnabled {
		return "true", nil
	}
	if key == service.SettingKeyCyberSessionBlockTTLSeconds {
		if s.ttl != "" {
			return s.ttl, nil
		}
		return "60", nil
	}
	return "", service.ErrSettingNotFound
}

func TestReviewCyberConfiguredTTLAndLookup(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	cache := NewGatewayCache(client)
	settings := service.NewSettingService(&reviewSettings{ttl: "120"}, nil)
	svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, cache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, settings, nil)
	enabled, ttl := svc.CyberSessionBlockRuntime(context.Background())
	require.True(t, enabled)
	require.Equal(t, 120*time.Second, ttl)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set("session_id", "explicit-ttl")
	body := []byte(`{"input":"trigger"}`)
	service.BindCyberSessionIdentity(c, svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, c, body, "ip", "ua"))
	service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{Message: "blocked"})
	key := service.CyberSessionExplicitBlockKey(11, c, body)
	require.InDelta(t, float64(120*time.Second), float64(server.TTL(cyberSessionBlockPrefix+key)), float64(time.Second))
	server.FastForward(100 * time.Second)
	require.NotEmpty(t, svc.FindCyberSessionBlockedForRequest(context.Background(), 11, c, body, "ip", "ua"))
	require.InDelta(t, float64(20*time.Second), float64(server.TTL(cyberSessionBlockPrefix+key)), float64(time.Second), "blocked request lookup must not reset the block TTL")
	server.FastForward(21 * time.Second)
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(context.Background(), 11, c, body, "ip", "ua"))
}

func TestReviewCyberFallbackExtendsFirstHitDeadline(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	cache := NewGatewayCache(client)
	svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, cache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, service.NewSettingService(&reviewSettings{ttl: "120"}, nil), nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	body := []byte(`{"input":"trigger"}`)
	service.BindCyberSessionIdentity(c, svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, c, body, "ip", "ua"))
	service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{Message: "blocked"})
	scope, keys := service.CyberSessionIdentityBlockWritePlan(c)
	server.FastForward(20 * time.Second)
	svc.MarkCyberSessionBlocked(context.Background(), scope, keys)
	require.InDelta(t, float64(100*time.Second), float64(server.TTL(cyberSessionBlockPrefix+keys[0])), float64(time.Second), "post-forward fallback must preserve first-hit deadline")
}
func TestReviewCyberLineageExpiresBeforeBlock(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	cache := NewGatewayCache(client)
	svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, cache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, service.NewSettingService(&reviewSettings{}, nil), nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	identity := svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, c, []byte(`{"input":"trigger"}`), "ip", "ua")
	service.BindCyberSessionIdentity(c, identity)
	service.ObserveCyberSessionResponseID(c, "resp_1")
	server.FastForward(30 * time.Second)
	service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{Message: "blocked", UpstreamStatus: 200})
	_, keys := service.CyberSessionIdentityBlockWritePlan(c)
	server.FastForward(31 * time.Second)
	blockStore, ok := cache.(service.CyberSessionBlockStore)
	require.True(t, ok)
	matched, err := blockStore.FindCyberSessionBlocked(context.Background(), keys)
	require.NoError(t, err)
	require.NotEmpty(t, matched, "the cyber block still has 29 seconds remaining")
	next, _ := gin.CreateTestContext(httptest.NewRecorder())
	next.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	body := []byte(`{"previous_response_id":"resp_1","input":"continue"}`)
	service.BindCyberSessionIdentity(next, svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, next, body, "ip", "ua"))
	require.NotEmpty(t, svc.FindCyberSessionBlockedForRequest(context.Background(), 11, next, body, "ip", "ua"), "continuation must remain blocked for the full cooldown TTL")
}

func TestCyberLineageRefreshPreservesEarlierTurnsAndTenantIsolation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	store, ok := NewGatewayCache(client).(*gatewayCache)
	require.True(t, ok)
	ctx := context.Background()
	require.NoError(t, store.BindCyberSessionRoot(ctx, 7, 11, "resp_1", "root", time.Minute))
	server.FastForward(40 * time.Second)
	require.NoError(t, store.BindCyberSessionRoot(ctx, 7, 11, "resp_2", "root", time.Minute))
	server.FastForward(30 * time.Second)
	// The root is still active through resp_2. Restore the earlier alias for
	// the block's full TTL without touching a different API key's mapping.
	require.NoError(t, store.BindCyberSessionRoot(ctx, 7, 12, "resp_1", "other", time.Minute))
	require.NoError(t, store.RefreshCyberSessionLineage(ctx, 7, 11, "root", time.Minute))
	root, found, err := store.GetCyberSessionRoot(ctx, 7, 11, "resp_1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "root", root)
	root, found, err = store.GetCyberSessionRoot(ctx, 7, 12, "resp_1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "other", root)
	server.FastForward(61 * time.Second)
	_, found, err = store.GetCyberSessionRoot(ctx, 7, 11, "resp_1")
	require.NoError(t, err)
	require.False(t, found)
}

func TestCyberLineageShorterSettingDoesNotShortenActiveAliases(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	store, ok := NewGatewayCache(client).(*gatewayCache)
	require.True(t, ok)
	ctx := context.Background()
	require.NoError(t, store.BindCyberSessionRoot(ctx, 7, 11, "resp_1", "root", 120*time.Second))
	server.FastForward(20 * time.Second)
	require.NoError(t, store.BindCyberSessionRoot(ctx, 7, 11, "resp_1", "root", 10*time.Second))
	require.NoError(t, store.RefreshCyberSessionLineage(ctx, 7, 11, "root", 10*time.Second))
	server.FastForward(11 * time.Second)
	_, found, err := store.GetCyberSessionRoot(ctx, 7, 11, "resp_1")
	require.NoError(t, err)
	require.True(t, found, "a shorter new setting must not remove an alias of an existing longer block")
}
