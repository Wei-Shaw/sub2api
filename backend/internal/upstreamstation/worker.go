package upstreamstation

import (
	"context"
	"log/slog"
	"time"
)

const (
	automaticSyncInitialDelay = 30 * time.Second
	automaticSyncInterval     = 5 * time.Minute
	automaticSyncTimeout      = 2 * time.Minute
)

func (s *Service) Start() {
	if s == nil || s.repository == nil || s.syncer == nil || s.workerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.workerCancel = cancel
	s.workerWG.Add(1)
	go s.runAutomaticSync(ctx)
}

func (s *Service) Stop() {
	if s == nil || s.workerCancel == nil {
		return
	}
	s.workerCancel()
	s.workerWG.Wait()
	s.workerCancel = nil
}

func (s *Service) runAutomaticSync(ctx context.Context) {
	defer s.workerWG.Done()
	initial := time.NewTimer(automaticSyncInitialDelay)
	defer initial.Stop()
	select {
	case <-ctx.Done():
		return
	case <-initial.C:
		s.syncAutomaticStations(ctx)
	}

	ticker := time.NewTicker(automaticSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncAutomaticStations(ctx)
		}
	}
}

func (s *Service) syncAutomaticStations(parent context.Context) {
	stations, err := s.repository.ListStations(parent)
	if err != nil {
		slog.Error("upstream station auto sync list failed", "error", err)
		return
	}
	for _, station := range stations {
		if !station.Enabled || !station.AutoSync {
			continue
		}
		ctx, cancel := context.WithTimeout(parent, automaticSyncTimeout)
		_, syncErr := s.syncer.SyncStation(ctx, station.ID)
		cancel()
		if syncErr != nil {
			slog.Warn("upstream station auto sync failed", "station_id", station.ID, "error", syncErr)
		}
	}
}
