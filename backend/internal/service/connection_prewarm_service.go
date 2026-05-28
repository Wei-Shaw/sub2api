package service

import (
	"context"
	"log/slog"
	"net/url"
	"sync"
	"time"
)

// ConnectionPrewarmService keeps upstream connections warm by periodically
// sending lightweight HEAD requests, avoiding cold TCP+TLS handshake latency
// when real requests arrive.
type ConnectionPrewarmService struct {
	httpUpstream HTTPUpstream
	opsService   *OpsService
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewConnectionPrewarmService creates a new prewarm service.
func NewConnectionPrewarmService(httpUpstream HTTPUpstream, opsService *OpsService) *ConnectionPrewarmService {
	return &ConnectionPrewarmService{
		httpUpstream: httpUpstream,
		opsService:   opsService,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the background pre-warming loop.
func (s *ConnectionPrewarmService) Start() {
	s.wg.Add(1)
	go s.loop()
	slog.Info("connection_prewarm_service.started")
}

// Stop gracefully stops the pre-warming loop.
func (s *ConnectionPrewarmService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	slog.Info("connection_prewarm_service.stopped")
}

func (s *ConnectionPrewarmService) loop() {
	defer s.wg.Done()

	settings := s.getSettings()
	if !settings.Enabled {
		slog.Info("connection_prewarm_service.disabled")
		return
	}

	interval := time.Duration(settings.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	s.prewarm(settings)

	for {
		select {
		case <-ticker.C:
			// Re-read settings in case they changed
			newSettings := s.getSettings()
			if !newSettings.Enabled {
				continue
			}
			// Adjust interval if changed
			newInterval := time.Duration(newSettings.IntervalSeconds) * time.Second
			if newInterval != interval && newInterval >= 10*time.Second {
				interval = newInterval
				ticker.Reset(interval)
			}
			s.prewarm(newSettings)
		case <-s.stopCh:
			return
		}
	}
}

func (s *ConnectionPrewarmService) prewarm(settings ConnectionPrewarmSettings) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sem := make(chan struct{}, settings.MaxConcurrentProbes)

	var wg sync.WaitGroup
	for _, targetURL := range settings.TargetURLs {
		if !isValidPrewarmURL(targetURL) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(target string) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			err := s.httpUpstream.Prewarm(ctx, target, "", 0, 1, nil)
			elapsed := time.Since(start)

			if err != nil {
				slog.Debug("connection_prewarm.failed",
					"target", target,
					"elapsed_ms", elapsed.Milliseconds(),
					"error", err)
			} else {
				slog.Debug("connection_prewarm.success",
					"target", target,
					"elapsed_ms", elapsed.Milliseconds())
			}
		}(targetURL)
	}
	wg.Wait()
}

func (s *ConnectionPrewarmService) getSettings() ConnectionPrewarmSettings {
	if s.opsService == nil {
		return defaultPerformanceSettings().ConnectionPrewarm
	}
	cfg, err := s.opsService.GetPerformanceSettings(context.Background())
	if err != nil {
		return defaultPerformanceSettings().ConnectionPrewarm
	}
	return cfg.ConnectionPrewarm
}

func isValidPrewarmURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Host != ""
}
