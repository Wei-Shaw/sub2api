package repository

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBillingCacheDeductUserBalanceReportsMissingKey(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewBillingCache(client)

	err := cache.DeductUserBalance(context.Background(), 7, 0.25)

	require.Error(t, err, "余额缓存键不存在时不能把未扣减误报为成功")
}
