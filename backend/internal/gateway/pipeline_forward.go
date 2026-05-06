package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (p *GatewayPipeline) selectAndForward(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	excludedIDs := make(map[int64]struct{})

	for i := 0; i < p.maxFailovers; i++ {
		result, done, err := p.tryOneAccount(ctx, w, req, excludedIDs)
		if done {
			return result, err
		}
		req.SwitchCount++
	}
	return nil, errors.New("gateway: failover limit reached, no account succeeded")
}

func (p *GatewayPipeline) tryOneAccount(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
) (*ForwardResult, bool, error) {
	account, releaseFunc, err := p.selectAccount(ctx, req, excludedIDs)
	if err != nil {
		return nil, true, fmt.Errorf("gateway: select account: %w", err)
	}
	req.Account = account
	defer func() {
		if releaseFunc != nil {
			releaseFunc()
		}
	}()

	if err := p.consumeBilling(ctx, req); err != nil {
		return nil, true, err
	}
	result, err := p.forwardToProvider(ctx, w, req)
	if err != nil {
		return p.handleForwardError(ctx, req, excludedIDs, err)
	}
	return result, true, nil
}

func (p *GatewayPipeline) selectAccount(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
) (*service.Account, func(), error) {
	selection, err := p.gatewayService.SelectAccountWithLoadAwareness(
		ctx, req.GroupID, req.SessionHash, req.Model,
		excludedIDs, req.MetadataUserID, req.UserID,
	)
	if err != nil {
		return nil, nil, err
	}
	account := selection.Account
	releaseFunc := selection.ReleaseFunc
	if !selection.Acquired && selection.WaitPlan != nil {
		slot, err := p.concurrency.AcquireAccountSlot(
			ctx, account.ID, selection.WaitPlan.MaxConcurrency,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("account slot: %w", err)
		}
		releaseFunc = slot.ReleaseFunc
		_ = p.gatewayService.BindStickySession(ctx, req.GroupID, req.SessionHash, account.ID)
	}
	return account, releaseFunc, nil
}

func (p *GatewayPipeline) consumeBilling(ctx context.Context, req *ForwardRequest) error {
	if req.BillingTicket == nil {
		return nil
	}
	channelID := int64(0)
	if req.ChannelMapping != nil {
		channelID = req.ChannelMapping.ChannelID
	}
	if err := req.BillingTicket.Consume(ctx, channelID, req.Account.ID); err != nil {
		return fmt.Errorf("gateway: billing consume: %w", err)
	}
	return nil
}

func (p *GatewayPipeline) forwardToProvider(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	provider, ok := p.registry.Get(req.Account.Platform)
	if !ok {
		return nil, fmt.Errorf("gateway: no provider for platform %q", req.Account.Platform)
	}
	return provider.Forward(ctx, w, req)
}

func (p *GatewayPipeline) handleForwardError(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
	err error,
) (*ForwardResult, bool, error) {
	if req.UpstreamAccepted {
		return nil, true, fmt.Errorf("gateway: forward failed after upstream accepted: %w", err)
	}
	provider, ok := p.registry.Get(req.Account.Platform)
	if !ok {
		return nil, true, err
	}
	if provider.ShouldFailover(ctx, req, err) {
		excludedIDs[req.Account.ID] = struct{}{}
		slog.Info("pipeline.failover",
			"account_id", req.Account.ID,
			"platform", req.Account.Platform,
			"switch_count", req.SwitchCount+1,
			"error", err,
		)
		return nil, false, nil
	}
	return nil, true, err
}

func (p *GatewayPipeline) recordUsage(
	ctx context.Context,
	req *ForwardRequest,
	result *ForwardResult,
	record RecordUsageFunc,
) error {
	if record == nil || result == nil || req.Account == nil {
		return nil
	}
	return record(ctx, req.Account, result)
}
