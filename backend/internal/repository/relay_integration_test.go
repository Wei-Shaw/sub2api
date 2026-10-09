//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func relayFixture(t *testing.T) (consumer, supplier *service.User, public, private *service.Group, account *service.Account, key *service.APIKey) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	consumer = mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@relay.test", PasswordHash: "hash", Balance: 100})
	supplier = mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@relay.test", PasswordHash: "hash", Balance: 0})
	groupRepo := NewGroupRepository(client, integrationDB)
	public = &service.Group{Name: "public-" + uuid.NewString(), Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, SupplierRate: 0.9, RateMultiplier: 1, SupportedModelScopes: []string{}}
	require.NoError(t, groupRepo.Create(ctx, public))
	private = &service.Group{Name: "private-" + uuid.NewString(), OwnerUserID: &supplier.ID, Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, IsExclusive: true, SupportedModelScopes: []string{}}
	require.NoError(t, groupRepo.Create(ctx, private))
	accountRepo := NewAccountRepository(client, integrationDB, nil)
	account = &service.Account{Name: uuid.NewString(), OwnerUserID: &supplier.ID, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Status: service.StatusActive, Schedulable: true, Concurrency: 3}
	require.NoError(t, accountRepo.Create(ctx, account))
	require.NoError(t, accountRepo.BindGroups(ctx, account.ID, []int64{public.ID, private.ID}))
	key = mustCreateApiKey(t, client, &service.APIKey{UserID: consumer.ID, Key: "sk-" + uuid.NewString(), Name: "relay", GroupID: &public.ID})
	return
}

func TestRelaySettlementCreditsSupplierExactlyOnce(t *testing.T) {
	consumer, supplier, public, _, account, key := relayFixture(t)
	ctx := context.Background()
	repo := NewUsageBillingRepository(testEntClient(t), integrationDB)
	cmd := &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: consumer.ID,
		AccountID: account.ID, AccountType: account.Type, GroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9, BalanceCost: 1}
	first, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.InDelta(t, 0.9, first.SupplierAmount, 1e-8)
	again, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, again.Applied)
	var buyerBalance, supplierBalance float64
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", consumer.ID).Scan(&buyerBalance))
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", supplier.ID).Scan(&supplierBalance))
	require.Equal(t, 99.0, buyerBalance)
	require.Equal(t, 0.9, supplierBalance)
	earnings, err := NewRelayRepository(testEntClient(t), integrationDB).Earnings(ctx, supplier.ID, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), earnings.Total)
	require.False(t, earnings.Items[0].SelfUse)
}

func TestRelayPublicSelfUseCreditsOwnWallet(t *testing.T) {
	_, supplier, public, _, account, key := relayFixture(t)
	ctx := context.Background()
	_, err := integrationDB.Exec("UPDATE users SET balance=10 WHERE id=$1", supplier.ID)
	require.NoError(t, err)
	cmd := &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: supplier.ID,
		AccountID: account.ID, AccountType: account.Type, GroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9, BalanceCost: 1}
	result, err := NewUsageBillingRepository(testEntClient(t), integrationDB).Apply(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, 0.9, result.SupplierAmount)
	require.InDelta(t, 9.9, *result.NewBalance, 1e-8)
	var balance float64
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", supplier.ID).Scan(&balance))
	require.InDelta(t, 9.9, balance, 1e-8)
	var self bool
	require.NoError(t, integrationDB.QueryRow("SELECT consumer_user_id=supplier_user_id FROM relay_earnings WHERE request_id=$1", cmd.RequestID).Scan(&self))
	require.True(t, self)
}

func TestRelayDatabasePreventsCrossOwnerBindingsAndPublicFallbackLeak(t *testing.T) {
	consumer, supplier, public, private, account, _ := relayFixture(t)
	ctx := context.Background()
	client := testEntClient(t)
	other := mustCreateAccount(t, client, &service.Account{Name: uuid.NewString(), Type: service.AccountTypeAPIKey, Platform: service.PlatformOpenAI})
	_, err := integrationDB.Exec("INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)", other.ID, private.ID)
	require.Error(t, err)
	_, err = integrationDB.Exec("UPDATE accounts SET owner_user_id=$1 WHERE id=$2", consumer.ID, account.ID)
	require.Error(t, err)
	_, err = integrationDB.Exec("UPDATE groups SET rate_multiplier=1 WHERE id=$1", private.ID)
	require.Error(t, err)
	_, err = integrationDB.Exec("UPDATE groups SET fallback_group_id=$1 WHERE id=$2", public.ID, private.ID)
	require.Error(t, err)
	accounts := NewAccountRepository(client, integrationDB, nil)
	loaded, err := accounts.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, supplier.ID, *loaded.OwnerUserID)
	groupLoaded, err := NewGroupRepository(client, integrationDB).GetByID(ctx, private.ID)
	require.NoError(t, err)
	require.True(t, groupLoaded.CanUse(supplier))
	require.False(t, groupLoaded.CanUse(consumer))
	_, err = integrationDB.Exec("DELETE FROM account_groups WHERE account_id=$1", account.ID)
	require.NoError(t, err)
	ungrouped, err := accounts.ListSchedulableUngroupedByPlatform(ctx, service.PlatformOpenAI)
	require.NoError(t, err)
	for _, a := range ungrouped {
		require.NotEqual(t, account.ID, a.ID)
	}
}

func TestRelayConcurrentTradesDoNotDeadlockOrDoubleCredit(t *testing.T) {
	consumer, supplier, public, _, account, key := relayFixture(t)
	ctx := context.Background()
	_, err := integrationDB.Exec("UPDATE users SET balance=100 WHERE id=$1", supplier.ID)
	require.NoError(t, err)
	client := testEntClient(t)
	other := &service.Account{Name: uuid.NewString(), OwnerUserID: &consumer.ID, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Credentials: map[string]any{}}
	require.NoError(t, NewAccountRepository(client, integrationDB, nil).Create(ctx, other))
	repo := NewUsageBillingRepository(client, integrationDB)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			buyer, seller, used := consumer.ID, supplier.ID, account.ID
			if i%2 == 1 {
				buyer, seller, used = supplier.ID, consumer.ID, other.ID
			}
			_, err := repo.Apply(ctx, &service.UsageBillingCommand{RequestID: fmt.Sprintf("relay-%s-%d", uuid.NewString(), i), APIKeyID: key.ID, UserID: buyer, AccountID: used, GroupID: &public.ID, SupplierUserID: &seller, SupplierRate: 0.9, BalanceCost: 1})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for _, id := range []int64{consumer.ID, supplier.ID} {
		var balance float64
		require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", id).Scan(&balance))
		require.Equal(t, 99.0, balance)
	}
}

func TestRelaySettlementRollsBackDebitWhenEarningInsertFails(t *testing.T) {
	consumer, supplier, public, _, account, key := relayFixture(t)
	ctx := context.Background()
	request := uuid.NewString()
	_, err := integrationDB.Exec(`INSERT INTO relay_earnings(request_id,api_key_id,consumer_user_id,supplier_user_id,account_id,group_id,charged_amount,supplier_rate,amount) VALUES($1,$2,$3,$4,$5,$6,1,0.9,0.9)`, request, key.ID, consumer.ID, supplier.ID, account.ID, public.ID)
	require.NoError(t, err)
	_, err = NewUsageBillingRepository(testEntClient(t), integrationDB).Apply(ctx, &service.UsageBillingCommand{RequestID: request, APIKeyID: key.ID, UserID: consumer.ID, AccountID: account.ID, GroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9, BalanceCost: 1})
	require.Error(t, err)
	var balance float64
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", consumer.ID).Scan(&balance))
	require.Equal(t, 100.0, balance)
	var count int
	require.NoError(t, integrationDB.QueryRow("SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id=$1", request).Scan(&count))
	require.Zero(t, count)
}

func TestRelayOverdraftEarningsRemainPendingUntilDebtIsFunded(t *testing.T) {
	consumer, supplier, public, _, account, key := relayFixture(t)
	ctx := context.Background()
	cmd := &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: consumer.ID, AccountID: account.ID,
		GroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9, BalanceCost: 101}
	result, err := NewUsageBillingRepository(testEntClient(t), integrationDB).Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.BalanceOverdrafted)
	require.Nil(t, result.SupplierUserID)
	r := NewRelayRepository(testEntClient(t), integrationDB)
	earnings, err := r.Earnings(ctx, supplier.ID, 1, 20)
	require.NoError(t, err)
	require.Zero(t, earnings.TotalEarned)
	require.InDelta(t, 90.9, earnings.PendingEarned, 1e-8)
	var balance float64
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", supplier.ID).Scan(&balance))
	require.Zero(t, balance)
	_, err = integrationDB.Exec("UPDATE users SET balance=0 WHERE id=$1", consumer.ID)
	require.NoError(t, err)
	earnings, err = r.Earnings(ctx, supplier.ID, 1, 20)
	require.NoError(t, err)
	require.True(t, earnings.BalanceChanged)
	require.Zero(t, earnings.PendingEarned)
	require.InDelta(t, 90.9, earnings.TotalEarned, 1e-8)
	earnings, err = r.Earnings(ctx, supplier.ID, 1, 20)
	require.NoError(t, err)
	require.False(t, earnings.BalanceChanged)
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", supplier.ID).Scan(&balance))
	require.InDelta(t, 90.9, balance, 1e-8)
}

func TestRelayBatchCapturePaysSupplierOnceFromHeldBalance(t *testing.T) {
	consumer, supplier, public, _, account, key := relayFixture(t)
	ctx := context.Background()
	client := testEntClient(t)
	batchID := "batch_" + uuid.NewString()
	hold := 2.0
	job, err := NewBatchImageRepository(integrationDB).CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID: batchID, UserID: consumer.ID, APIKeyID: &key.ID, AccountID: &account.ID, Provider: service.BatchImageProviderGeminiAPI, Model: "image-model", TaskName: "relay test", Status: service.BatchImageJobStatusCreated,
		ItemCount: 1, EstimatedCost: 2, HoldAmount: &hold, Currency: "USD", RelayGroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9})
	require.NoError(t, err)
	require.Equal(t, public.ID, *job.RelayGroupID)
	require.Equal(t, supplier.ID, *job.SupplierUserID)
	require.Equal(t, 0.9, job.SupplierRate)
	repo := NewUsageBillingRepository(client, integrationDB)
	_, err = repo.ReserveBatchImageBalance(ctx, &service.BatchImageBalanceHoldCommand{RequestID: service.BatchImageHoldRequestID(batchID), APIKeyID: key.ID, UserID: consumer.ID, BatchID: batchID, HoldAmount: 2})
	require.NoError(t, err)
	cmd := &service.BatchImageBalanceHoldCommand{RequestID: service.BatchImageCaptureRequestID(batchID), APIKeyID: key.ID, UserID: consumer.ID, BatchID: batchID, HoldAmount: 2, ActualAmount: 1,
		AccountID: &account.ID, GroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9}
	first, err := repo.CaptureBatchImageBalance(ctx, cmd)
	require.NoError(t, err)
	require.True(t, first.Applied)
	again, err := repo.CaptureBatchImageBalance(ctx, cmd)
	require.NoError(t, err)
	require.False(t, again.Applied)
	var buyerBalance, supplierBalance, frozen float64
	require.NoError(t, integrationDB.QueryRow("SELECT balance,frozen_balance FROM users WHERE id=$1", consumer.ID).Scan(&buyerBalance, &frozen))
	require.Equal(t, 99.0, buyerBalance)
	require.Zero(t, frozen)
	require.NoError(t, integrationDB.QueryRow("SELECT balance FROM users WHERE id=$1", supplier.ID).Scan(&supplierBalance))
	require.Equal(t, 0.9, supplierBalance)
}

func TestRelayDeletingOwnerRetiresTheirResourcesAndPreservesEarnings(t *testing.T) {
	consumer, supplier, public, private, account, key := relayFixture(t)
	ctx := context.Background()
	client := testEntClient(t)
	_, err := NewUsageBillingRepository(client, integrationDB).Apply(ctx, &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: consumer.ID,
		AccountID: account.ID, GroupID: &public.ID, SupplierUserID: &supplier.ID, SupplierRate: 0.9, BalanceCost: 1})
	require.NoError(t, err)
	_, err = integrationDB.Exec("UPDATE users SET deleted_at=NOW() WHERE id=$1", supplier.ID)
	require.NoError(t, err)
	var retired bool
	require.NoError(t, integrationDB.QueryRow("SELECT deleted_at IS NOT NULL AND NOT schedulable FROM accounts WHERE id=$1", account.ID).Scan(&retired))
	require.True(t, retired)
	require.NoError(t, integrationDB.QueryRow("SELECT deleted_at IS NOT NULL FROM groups WHERE id=$1", private.ID).Scan(&retired))
	require.True(t, retired)
	var earnings int
	require.NoError(t, integrationDB.QueryRow("SELECT COUNT(*) FROM relay_earnings WHERE supplier_user_id=$1", supplier.ID).Scan(&earnings))
	require.Equal(t, 1, earnings)
}
