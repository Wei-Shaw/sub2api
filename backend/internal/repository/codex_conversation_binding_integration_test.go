//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestCodexDeviceConversationCapacityOnRedis(t *testing.T) {
	cache := NewConcurrencyCache(integrationRedis, 15, 900).(service.CodexDeviceConversationCapacityCache)
	ctx := context.Background()
	slotKey := fmt.Sprintf("integration-slot-%d", time.Now().UnixNano())
	const workers = 15
	results := make(chan bool, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, slotKey, fmt.Sprint(i), 3)
			results <- ok
			errs <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	admitted := 0
	for ok := range results {
		if ok {
			admitted++
		}
	}
	require.Equal(t, 3, admitted)
	for i := range workers {
		require.NoError(t, cache.ReleaseCodexDeviceConversationLease(ctx, slotKey, fmt.Sprint(i)))
	}
	for i := range workers {
		ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, slotKey, fmt.Sprint(i), 0)
		require.NoError(t, err)
		require.True(t, ok)
	}
	for i := range workers {
		ok, err := cache.RefreshCodexDeviceConversationLease(ctx, slotKey, fmt.Sprint(i))
		require.NoError(t, err)
		require.True(t, ok)
	}
	for i := range workers {
		require.NoError(t, cache.ReleaseCodexDeviceConversationLease(ctx, slotKey, fmt.Sprint(i)))
	}
}

func TestCodexConversationBindingAffinityOnPostgres(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	suffix := time.Now().UnixNano()
	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("slot-affinity-%d@example.com", suffix)})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-slot-affinity-%d", suffix)})
	account := &service.Account{Name: fmt.Sprintf("slot-affinity-%d", suffix), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{}, Status: service.StatusActive, Schedulable: true, Concurrency: 5, Priority: 50}
	policy := service.CodexIdentityPolicySpec{Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		SessionPolicy: service.CodexSessionPolicySpec{Mode: service.CodexSessionDeviceShared, MaxActiveConversationsPerSlot: 5, DisableCrossKeyContinuation: true},
		Profiles:      []service.CodexOSProfilePolicy{{OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 3}}}
	require.NoError(t, repo.ProvisionAccount(ctx, &service.AccountProvisioningSpec{Account: account, Identity: &policy, FinalStatus: service.StatusActive, Schedulable: true, ProvisioningState: service.AccountProvisioningActive}))
	t.Cleanup(func() {
		_ = client.Account.DeleteOneID(account.ID).Exec(context.Background())
		_ = client.APIKey.DeleteOneID(key.ID).Exec(context.Background())
		_ = client.User.DeleteOneID(user.ID).Exec(context.Background())
	})
	for _, limit := range []int{0, 1, 5, 1000} {
		_, err := integrationDB.ExecContext(ctx, "UPDATE account_codex_identity_policies SET session_policy=jsonb_set(session_policy,'{max_active_conversations_per_slot}',to_jsonb($2::integer)) WHERE account_id=$1", account.ID, limit)
		require.NoError(t, err)
	}
	for _, limit := range []int{-1, 1001} {
		_, err := integrationDB.ExecContext(ctx, "UPDATE account_codex_identity_policies SET session_policy=jsonb_set(session_policy,'{max_active_conversations_per_slot}',to_jsonb($2::integer)) WHERE account_id=$1", account.ID, limit)
		var pgErr *pq.Error
		require.ErrorAs(t, err, &pgErr)
		require.Equal(t, pq.ErrorCode("23514"), pgErr.Code)
	}
	original, err := repo.ResolveCodexDeviceBinding(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	require.NoError(t, err)
	slots, err := repo.ListCodexDeviceSlots(ctx, account.ID, service.CodexOSLinux, service.CodexSurfaceCLI, false)
	require.NoError(t, err)
	require.Len(t, slots, 3)
	alternative := slots[0]
	if alternative.SlotID == original.SlotID {
		alternative = slots[1]
	}
	hash := strings.Repeat("a", 64)
	pin, err := repo.BindCodexConversationSlot(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI, hash, alternative.SlotID)
	require.NoError(t, err)
	require.Equal(t, alternative.SlotID, pin.SlotID)
	again, err := repo.BindCodexConversationSlot(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI, hash, original.SlotID)
	require.NoError(t, err)
	require.Equal(t, pin.BindingID, again.BindingID)
	require.Equal(t, pin.SlotID, again.SlotID)
	preferred, err := repo.ResolveCodexDeviceBinding(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	require.NoError(t, err)
	require.Equal(t, original.SlotID, preferred.SlotID)
	found, err := repo.FindCodexConversationBinding(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI, hash)
	require.NoError(t, err)
	require.Equal(t, pin.SlotID, found.SlotID)
	_, err = repo.BindCodexConversationSlot(ctx, account.ID, key.ID, service.CodexOSWindows, service.CodexSurfaceDesktop, hash, alternative.SlotID)
	require.ErrorIs(t, err, service.ErrDeviceProfileUnsupported)
	// Concurrent first writers with different candidates must converge.
	const workers = 9
	outcomes := make(chan *service.CodexResolvedDeviceSlot, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := repo.BindCodexConversationSlot(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI, strings.Repeat("b", 64), slots[i%len(slots)].SlotID)
			outcomes <- result
			errs <- err
		}(i)
	}
	wg.Wait()
	close(outcomes)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var winningID int64
	for result := range outcomes {
		require.NotNil(t, result)
		if winningID == 0 {
			winningID = result.SlotID
		}
		require.Equal(t, winningID, result.SlotID)
	}
	// Existing drain cleanup sees conversation bindings as well as key preferences.
	_, err = integrationDB.ExecContext(ctx, "UPDATE account_codex_device_slots SET state='draining' WHERE id=$1", pin.SlotID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE account_codex_device_bindings SET updated_at=NOW()-INTERVAL '2 days' WHERE slot_id=$1", pin.SlotID)
	require.NoError(t, err)
	require.NoError(t, repo.RefreshCodexConversationBinding(ctx, pin.BindingID))
	_, err = repo.FinalizeDrainedCodexDeviceSlots(ctx, account.ID)
	require.NoError(t, err)
	found, err = repo.FindCodexConversationBinding(ctx, account.ID, key.ID, service.CodexOSLinux, service.CodexSurfaceCLI, hash)
	require.NoError(t, err)
	require.NotNil(t, found)
	_, err = integrationDB.ExecContext(ctx, "UPDATE account_codex_device_bindings SET updated_at=NOW()-INTERVAL '2 days' WHERE slot_id=$1", pin.SlotID)
	require.NoError(t, err)
	deleted, err := repo.FinalizeDrainedCodexDeviceSlots(ctx, account.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, deleted, int64(1))
	require.Error(t, repo.RefreshCodexConversationBinding(ctx, pin.BindingID))
}
