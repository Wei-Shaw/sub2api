package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cacheWriteBlockingSettingRepo struct {
	SettingRepository
	entered chan struct{}
	release chan struct{}
	values  map[string]string
	err     error
}

func (r *cacheWriteBlockingSettingRepo) GetMultiple(ctx context.Context, _ []string) (map[string]string, error) {
	close(r.entered)
	select {
	case <-r.release:
		return r.values, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestOpenAICacheWriteSettingRace_InflightReadCannotUndoAdminRefresh(t *testing.T) {
	for _, staleReadError := range []bool{false, true} {
		name := "stale_true_cannot_undo_disable"
		if staleReadError {
			name = "stale_error_cannot_undo_enable"
		}
		t.Run(name, func(t *testing.T) {
			resetOpenAICacheWriteInferenceSwitchForTest(t)
			repo := &cacheWriteBlockingSettingRepo{
				entered: make(chan struct{}), release: make(chan struct{}),
				values: map[string]string{SettingKeyOpenAICacheWriteInferenceEnabled: "true"},
			}
			if staleReadError {
				repo.err = errors.New("simulated old read failure")
			}
			svc := &SettingService{settingRepo: repo}
			returned := make(chan bool, 1)
			go func() { returned <- svc.IsOpenAICacheWriteInferenceEnabled(context.Background()) }()
			select {
			case <-repo.entered:
			case <-time.After(time.Second):
				t.Fatal("settings read did not start")
			}
			// singleflight.Forget cannot cancel the read already in progress.
			// Its later result must lose to this completed admin publication.
			svc.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: staleReadError})
			publishedEpoch := openAICacheWriteInferenceSettingEpoch.Load()
			publishedGeneration := gatewayForwardingCacheGeneration.Load()
			close(repo.release)
			select {
			case enabled := <-returned:
				require.Equal(t, staleReadError, enabled, "the waiting getter must return the current publication")
			case <-time.After(time.Second):
				t.Fatal("settings read did not finish")
			}
			require.Equal(t, publishedEpoch, openAICacheWriteInferenceSettingEpoch.Load())
			require.Equal(t, publishedGeneration, gatewayForwardingCacheGeneration.Load(), "rejected stale reads cannot publish")
			require.Equal(t, staleReadError, svc.IsOpenAICacheWriteInferenceEnabled(context.Background()))
		})
	}
}

type cacheWriteBlockingAdjustmentRepo struct {
	UsageBillingRepository
	delegate UsageBillingRepository
	entered  chan struct{}
	release  chan struct{}
}

func (r *cacheWriteBlockingAdjustmentRepo) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if cmd.CacheWriteCorrection != nil {
		close(r.entered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.delegate.Apply(ctx, cmd)
}

func TestOpenAICacheWriteSettingRace_DisableWaitsForStartedAdjustment(t *testing.T) {
	f := newCacheWriteSimulationFixture(t)
	require.NoError(t, f.svc.RecordUsage(context.Background(), f.input))
	blocking := &cacheWriteBlockingAdjustmentRepo{
		delegate: f.billingRepo, entered: make(chan struct{}), release: make(chan struct{}),
	}
	f.svc.usageBillingRepo = blocking
	reconciled := make(chan struct{})
	go func() {
		defer close(reconciled)
		f.reconcile900Tokens()
	}()
	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("adjustment did not reach the billing repository")
	}
	if gatewayForwardingPublicationMu.TryLock() {
		gatewayForwardingPublicationMu.Unlock()
		close(blocking.release)
		t.Fatal("adjustment released the publication gate before its transaction completed")
	}
	updated := make(chan struct{})
	go func() {
		f.svc.settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: false})
		close(updated)
	}()
	// The write gate cannot publish a disable until this already-started
	// adjustment finishes. Once refresh returns, no later adjustment may start.
	close(blocking.release)
	select {
	case <-reconciled:
	case <-time.After(time.Second):
		t.Fatal("adjustment did not complete")
	}
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Fatal("disable refresh did not complete after the adjustment")
	}
	require.False(t, f.svc.settingService.IsOpenAICacheWriteInferenceEnabled(context.Background()))
	require.Len(t, f.billingRepo.cmds, 2)
	f.reconcile900Tokens()
	require.Len(t, f.billingRepo.cmds, 2, "no stale queued adjustment may start after disable acknowledgment")
}
