package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	billingReservationReaperDefaultInterval = 60 * time.Second
	billingReservationReaperDefaultBatch    = 100
)

type BillingReservationReaper struct {
	repo     BillingReservationReaperRepository
	enabled  bool
	interval time.Duration
	batch    int
	now      func() time.Time
}

func NewBillingReservationReaper(repo BillingReservationReaperRepository, cfg *config.Config) *BillingReservationReaper {
	interval := billingReservationReaperDefaultInterval
	enabled := false
	if cfg != nil {
		enabled = cfg.ReliabilityCore.VideoEnabled
		if cfg.ReliabilityCore.ReservationReapIntervalSeconds > 0 {
			interval = time.Duration(cfg.ReliabilityCore.ReservationReapIntervalSeconds) * time.Second
		}
	}
	return &BillingReservationReaper{
		repo:     repo,
		enabled:  enabled,
		interval: interval,
		batch:    billingReservationReaperDefaultBatch,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (r *BillingReservationReaper) RunOnce(ctx context.Context) ([]BillingReservationReapResult, error) {
	if r == nil || !r.enabled {
		return nil, nil
	}
	if r.repo == nil {
		return nil, fmt.Errorf("billing reservation reaper repository is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results, err := r.repo.ReapExpiredVideoReservations(ctx, r.now(), r.batch)
	if err != nil {
		return nil, fmt.Errorf("reap expired video reservations: %w", err)
	}
	return results, nil
}
