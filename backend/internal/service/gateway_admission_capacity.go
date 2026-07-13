package service

import (
	"context"
	"fmt"
	"time"
)

type AdmissionCapacitySnapshot struct {
	TotalConcurrency   int
	AccountConcurrency map[int64]int
	BuiltAt            time.Time
	ValidUntil         *time.Time
}

type AdmissionCapacitySource interface {
	AdmissionCapacity(ctx context.Context, platform string) (AdmissionCapacitySnapshot, error)
}

func (s *GatewayService) AdmissionCapacity(ctx context.Context, platform string) (AdmissionCapacitySnapshot, error) {
	if s != nil && s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.AdmissionCapacity(ctx, platform)
	}
	if s == nil || s.accountRepo == nil {
		return AdmissionCapacitySnapshot{}, fmt.Errorf("gateway admission capacity source is unavailable")
	}

	accounts, err := s.accountRepo.ListSchedulableByPlatform(ctx, platform)
	if err != nil {
		return AdmissionCapacitySnapshot{}, fmt.Errorf("list gateway admission capacity for %s: %w", platform, err)
	}

	return buildAdmissionCapacitySnapshot(platform, accounts), nil
}

func buildAdmissionCapacitySnapshot(platform string, accounts []Account) AdmissionCapacitySnapshot {
	snapshot := AdmissionCapacitySnapshot{
		AccountConcurrency: make(map[int64]int, len(accounts)),
		BuiltAt:            time.Now().UTC(),
	}
	for i := range accounts {
		account := &accounts[i]
		if account.ID <= 0 || account.Platform != platform || !account.IsSchedulable() || account.Concurrency <= 0 {
			continue
		}
		if _, exists := snapshot.AccountConcurrency[account.ID]; exists {
			continue
		}
		snapshot.AccountConcurrency[account.ID] = account.Concurrency
		snapshot.TotalConcurrency += account.Concurrency
		if account.AutoPauseOnExpired && account.ExpiresAt != nil &&
			(snapshot.ValidUntil == nil || account.ExpiresAt.Before(*snapshot.ValidUntil)) {
			validUntil := account.ExpiresAt.UTC()
			snapshot.ValidUntil = &validUntil
		}
	}
	return snapshot
}
