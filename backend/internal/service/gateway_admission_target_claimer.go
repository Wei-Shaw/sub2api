package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

type gatewayAdmissionTargetClaimer struct {
	store          GatewayAdmissionStore
	capacitySource AdmissionCapacitySource
	requestID      string
	class          AdmissionClass
	settings       ExtraConcurrencyRuntimeSettings

	errMu       sync.Mutex
	terminalErr error
	pendingMu   sync.Mutex
	pending     TargetClaimRequest
}

func (c *gatewayAdmissionTargetClaimer) TryClaim(ctx context.Context, target TargetClaimRequest) (func(), bool, error) {
	if c == nil || c.store == nil {
		return nil, false, fmt.Errorf("gateway admission target claimer is unavailable")
	}
	if target.Platform == "" || target.AccountID <= 0 {
		return nil, false, fmt.Errorf("invalid gateway admission target")
	}
	c.releasePendingOnPlatformSwitch(target.Platform)

	unlimited := target.AccountConcurrency <= 0
	platformCapacity := 0
	accountLimit := 0
	if !unlimited && c.capacitySource != nil {
		snapshot, err := c.capacitySource.AdmissionCapacity(ctx, target.Platform)
		if err != nil {
			if c.class == AdmissionClassExtra {
				return nil, false, nil
			}
		} else {
			platformCapacity = snapshot.TotalConcurrency
			accountLimit = snapshot.AccountConcurrency[target.AccountID]
		}
	}

	if !unlimited && c.class == AdmissionClassStandard && (platformCapacity <= 0 || accountLimit <= 0) {
		platformCapacity = math.MaxInt
		accountLimit = target.AccountConcurrency
	}
	if !unlimited && (platformCapacity <= 0 || accountLimit <= 0) {
		return nil, false, nil
	}
	c.recordPending(target)

	waitTimeout := time.Duration(c.settings.WaitTimeoutSeconds) * time.Second
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	result, err := c.store.TryAcquireTargetLease(ctx, TargetLeaseRequest{
		RequestID:        c.requestID,
		Platform:         target.Platform,
		AccountID:        target.AccountID,
		AccountLimit:     accountLimit,
		PlatformCapacity: platformCapacity,
		ReservedSlots:    gatewayAdmissionReservedSlots(c.settings, target.Platform, platformCapacity),
		Class:            c.class,
		WaitTimeout:      waitTimeout,
		Unlimited:        unlimited,
	})
	if err != nil {
		c.setTerminalError(err)
		return nil, false, nil
	}
	if result.Expired {
		c.setTerminalError(gatewayAdmissionWaitTimeoutError(c.class, "account"))
		return nil, false, nil
	}
	if result.Draining {
		c.setTerminalError(ErrGatewayAdmissionDraining)
		return nil, false, nil
	}
	if !result.Acquired {
		return nil, false, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.store.ReleaseTargetLease(releaseCtx, target.Platform, target.AccountID, c.requestID)
		})
	}
	return release, true, nil
}

func (c *gatewayAdmissionTargetClaimer) Err() error {
	if c == nil {
		return nil
	}
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.terminalErr
}

func (c *gatewayAdmissionTargetClaimer) setTerminalError(err error) {
	if c == nil || err == nil {
		return
	}
	c.errMu.Lock()
	if c.terminalErr == nil {
		c.terminalErr = err
	}
	c.errMu.Unlock()
}

func (c *gatewayAdmissionTargetClaimer) recordPending(target TargetClaimRequest) {
	c.pendingMu.Lock()
	previous := c.pending
	if previous.Platform == target.Platform {
		c.pending = target
		c.pendingMu.Unlock()
		return
	}
	c.pending = target
	c.pendingMu.Unlock()
	c.releasePendingTarget(previous)
}

func (c *gatewayAdmissionTargetClaimer) releasePendingOnPlatformSwitch(platform string) {
	c.pendingMu.Lock()
	previous := c.pending
	if previous.Platform == "" || previous.Platform == platform {
		c.pendingMu.Unlock()
		return
	}
	c.pending = TargetClaimRequest{}
	c.pendingMu.Unlock()
	c.releasePendingTarget(previous)
}

func (c *gatewayAdmissionTargetClaimer) ReleasePending() {
	if c == nil || c.store == nil {
		return
	}
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = TargetClaimRequest{}
	c.pendingMu.Unlock()
	c.releasePendingTarget(pending)
}

func (c *gatewayAdmissionTargetClaimer) releasePendingTarget(pending TargetClaimRequest) {
	if pending.Platform == "" || pending.AccountID <= 0 {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.store.ReleaseTargetLease(releaseCtx, pending.Platform, pending.AccountID, c.requestID)
}

func gatewayAdmissionReservedSlots(settings ExtraConcurrencyRuntimeSettings, platform string, totalCapacity int) int {
	if totalCapacity <= 0 {
		return 0
	}
	reservePercent := settings.ReservePercent
	minReservedSlots := settings.MinReservedSlots
	if override, ok := settings.PlatformReserves[platform]; ok {
		if override.ReservePercent != nil {
			reservePercent = *override.ReservePercent
		}
		if override.MinReservedSlots != nil {
			minReservedSlots = *override.MinReservedSlots
		}
	}

	reservedSlots := int(math.Ceil(float64(totalCapacity) * reservePercent / 100))
	reservedSlots = max(reservedSlots, minReservedSlots)
	return min(max(reservedSlots, 0), totalCapacity)
}
