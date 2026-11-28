//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type pendingMuseStore struct {
	*museStoreFixture
	deferred int
}

func (s *pendingMuseStore) Settle(context.Context, string) (*MuseSettlement, error) {
	return nil, errors.New("temporary log insertion failure")
}
func (s *pendingMuseStore) DeferSettlement(context.Context, string) error { s.deferred++; return nil }

type museHoldCache struct {
	held     float64
	releases int
}

func (c *museHoldCache) ReserveInflightBalance(_ context.Context, _ int64, _ string, amount, balance float64, _ time.Duration) (bool, float64, error) {
	before := c.held
	if before > 0 && balance-before < amount {
		return false, before, nil
	}
	c.held += amount
	return true, before, nil
}
func (c *museHoldCache) ReleaseInflightBalance(context.Context, int64, string) error {
	c.held = 0
	c.releases++
	return nil
}
func TestPendingMuseSettlementTransfersToDurableHold(t *testing.T) {
	g, core, provider, store, runtime, account, key := newMuseCoreFixture()
	g.cfg = &config.Config{}
	g.resolver = NewModelPricingResolver(nil, nil)
	price := 0.03
	key.Group.RateMultiplier = 1
	key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"muse/assistant"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}}
	key.GroupID = &key.Group.ID
	pending := &pendingMuseStore{museStoreFixture: store}
	core.store = pending
	holdCache := &museHoldCache{held: price}
	hold := &InflightReservation{cache: holdCache, userID: key.UserID, requestID: "original-hold", amount: price, ttl: time.Minute}
	hold.refs.Store(1)
	ctx := WithInflightReservation(context.Background(), hold)
	result, turn, err := core.Execute(ctx, key, account, &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, muse.Completed, turn.State)
	require.Equal(t, muse.Completed, runtime.state)
	require.Equal(t, 1, provider.calls)
	require.InDelta(t, price, store.command.BalanceCost, 1e-8)
	require.Equal(t, 1, pending.deferred)
	require.EqualValues(t, 0, hold.refs.Load(), "the durable reservation replaces the temporary Redis hold")
	require.InDelta(t, price, runtime.balanceHold, 1e-8)
	hold.HandlerDone()
	require.Equal(t, 1, holdCache.releases)
	// The original completion is returned exactly once. Eligibility uses the
	// durable turn's pending amount even after the HTTP/Redis hold has ended.
	cache := &balanceEligibilityCacheStub{balance: price}
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	billing := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	defer billing.Stop()
	billing.museBalances = &pendingBalanceFixture{balance: price, amount: runtime.balanceHold}
	require.ErrorIs(t, billing.checkBalanceEligibility(context.Background(), key.UserID), ErrInsufficientBalance)
	require.Equal(t, 1, provider.calls)
}

type pendingBalanceFixture struct {
	balance float64
	amount  float64
	err     error
}

func (f *pendingBalanceFixture) AvailableBalance(context.Context, int64) (float64, error) {
	return f.balance - f.amount, f.err
}
func TestPendingMuseBalanceLookupFailsClosed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	cache := &memInflightCache{balance: 1}
	billing := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	defer billing.Stop()
	billing.museBalances = &pendingBalanceFixture{err: errors.New("database unavailable")}
	require.Error(t, billing.checkBalanceEligibility(context.Background(), 1))
	_, err := billing.ReserveInflight(context.Background(), &User{ID: 1}, nil, nil, 0.03)
	require.Error(t, err)
}

func TestNativeAdmissionDoesNotUseStaleBalanceAfterSettlement(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	cache := newMemInflightCache(0.03) // gross cached balance predates committed debit
	billing := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	defer billing.Stop()
	billing.museBalances = &pendingBalanceFixture{balance: 0, amount: 0}
	require.ErrorIs(t, billing.checkBalanceEligibility(context.Background(), 1), ErrInsufficientBalance)
	cfg.Billing.InflightReservation.Enabled = false
	// No new database read on the default path, including database outages.
	billing.museBalances = &pendingBalanceFixture{err: errors.New("must not query")}
	require.NoError(t, billing.checkBalanceEligibility(context.Background(), 1))
}
