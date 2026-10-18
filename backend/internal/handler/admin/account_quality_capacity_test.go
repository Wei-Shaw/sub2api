package admin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type qualityCapacityAdminStub struct {
	service.AdminService
	accounts []*service.Account
	ids      []int64
	err      error
}

func (s *qualityCapacityAdminStub) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	s.ids = append([]int64(nil), ids...)
	return s.accounts, s.err
}

type qualityCapacityConcurrencyStub struct {
	service.ConcurrencyCache
	calls int
	err   error
}

func (s *qualityCapacityConcurrencyStub) GetAccountConcurrencyBatch(_ context.Context, ids []int64) (map[int64]int, error) {
	s.calls++
	return map[int64]int{7: 3, 8: 0}, s.err
}

func TestAccountQualityCapacityPreservesQuotaSemanticsAndExcludesSecrets(t *testing.T) {
	account := &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 10,
		Credentials: map[string]any{"api_key": "secret-must-not-appear"},
		Extra:       map[string]any{"quota_limit": 100.0, "quota_used": 4.0, "quota_daily_limit": 20.0, "quota_daily_used": 8.0, "quota_daily_start": time.Now().Add(-48 * time.Hour).Format(time.RFC3339)},
	}
	capacity := qualityCapacityFromAccount(account)
	require.Equal(t, 10, capacity.Concurrency)
	require.Equal(t, 100.0, *capacity.QuotaLimit)
	require.Equal(t, 4.0, *capacity.QuotaUsed)
	require.Equal(t, 0.0, *capacity.QuotaDailyUsed, "Expired daily usage must use the same reset semantics as the original list")
	data, err := json.Marshal(capacity)
	require.NoError(t, err)
	require.NotContains(t, string(data), "secret-must-not-appear")
	require.NotContains(t, string(data), "credentials")
	require.Contains(t, string(data), "\"current_concurrency\":null")
}

func TestAccountQualityCapacityBatchesOnlyPageMembersAndKeepsMissingDataUnknown(t *testing.T) {
	admin := &qualityCapacityAdminStub{accounts: []*service.Account{
		{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 10},
		{ID: 8, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 4},
	}}
	cache := &qualityCapacityConcurrencyStub{}
	h := NewAccountQualityHandler(nil)
	h.SetAccountHandler(&AccountHandler{adminService: admin, concurrencyService: service.NewConcurrencyService(cache)})
	page := service.AccountQualityPage{Items: []service.AccountQualityRow{{ID: 7, Name: "same-account"}, {ID: 8}, {ID: 9}}, Total: 100, Page: 2}
	result := h.pageWithCapacity(context.Background(), page)
	require.Equal(t, []int64{7, 8, 9}, admin.ids)
	require.Equal(t, 1, cache.calls)
	require.Equal(t, 3, *result.Items[0].Capacity.CurrentConcurrency)
	require.Equal(t, 0, *result.Items[1].Capacity.CurrentConcurrency)
	require.Nil(t, result.Items[2].Capacity)
	require.Equal(t, 100, result.Total)
	require.Equal(t, 2, result.Page)
	data, err := json.Marshal(result)
	require.NoError(t, err)
	var wire struct {
		Items []struct {
			ID       int64
			Capacity *accountQualityCapacity
		}
	}
	require.NoError(t, json.Unmarshal(data, &wire))
	require.Len(t, wire.Items, 3)
	require.Equal(t, int64(7), wire.Items[0].ID)
	require.Equal(t, 3, *wire.Items[0].Capacity.CurrentConcurrency)
	cache.err = errors.New("cache unavailable")
	result = h.pageWithCapacity(context.Background(), page)
	require.Nil(t, result.Items[0].Capacity.CurrentConcurrency, "Cache errors must not fabricate zero usage")
	admin.err = errors.New("database unavailable")
	result = h.pageWithCapacity(context.Background(), page)
	require.Len(t, result.Items, 3)
	require.Nil(t, result.Items[0].Capacity)
	require.Equal(t, "same-account", result.Items[0].Name)
}
