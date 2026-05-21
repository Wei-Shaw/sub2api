package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	SettingTelegramNotifyProxyExpiringThresholdDays = "telegram_notify_proxy_expiring_threshold_days"
	defaultProxyExpiringThresholdDays               = 3
)

// ProxyExpiryService periodically processes expired proxy fallback/reroute and sends Telegram warnings.
type ProxyExpiryService struct {
	proxyRepo         ProxyRepository
	settingRepo       SettingRepository
	interval          time.Duration
	stopCh            chan struct{}
	stopOnce          sync.Once
	wg                sync.WaitGroup
	telegramNotifySvc *TelegramNotifyService
}

func NewProxyExpiryService(proxyRepo ProxyRepository, settingRepo SettingRepository, interval time.Duration) *ProxyExpiryService {
	return &ProxyExpiryService{
		proxyRepo:   proxyRepo,
		settingRepo: settingRepo,
		interval:    interval,
		stopCh:      make(chan struct{}),
	}
}

func (s *ProxyExpiryService) SetTelegramNotifyService(svc *TelegramNotifyService) {
	s.telegramNotifySvc = svc
}

func (s *ProxyExpiryService) Start() {
	if s == nil || s.proxyRepo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// Initial delay to avoid a notification/rebuild burst during process startup.
		time.Sleep(30 * time.Second)
		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *ProxyExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *ProxyExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Step 1: run fork fallback semantics for expired proxies. This marks proxies
	// expired and re-routes bound accounts according to fallback_mode/backup_proxy_id.
	changed, err := s.proxyRepo.SweepExpiredProxies(ctx, time.Now())
	if err != nil {
		slog.Error("[ProxyExpiry] sweep expired proxies failed", "error", err)
		return
	}
	if changed > 0 {
		slog.Info("[ProxyExpiry] re-routed accounts off expired proxies", "count", changed)
	}

	// Step 2: warn about proxies nearing expiration (only if Telegram is configured).
	if s.telegramNotifySvc == nil {
		return
	}
	if !s.telegramNotifySvc.isEnabled(context.Background(), SettingTelegramNotifyProxyExpired) {
		return
	}

	thresholdDays := s.getThresholdDays(ctx)
	deadline := time.Now().Add(time.Duration(thresholdDays) * 24 * time.Hour)

	proxies, err := s.proxyRepo.ListExpiringBefore(ctx, deadline)
	if err != nil {
		slog.Error("[ProxyExpiry] failed to list expiring proxies", "error", err)
		return
	}
	if len(proxies) == 0 {
		return
	}

	go s.telegramNotifySvc.NotifyProxyExpiring(context.Background(), proxies, thresholdDays)
}

func (s *ProxyExpiryService) getThresholdDays(ctx context.Context) int {
	if s.settingRepo == nil {
		return defaultProxyExpiringThresholdDays
	}
	val, err := s.settingRepo.GetValue(ctx, SettingTelegramNotifyProxyExpiringThresholdDays)
	if err != nil || val == "" {
		return defaultProxyExpiringThresholdDays
	}
	days := 0
	for _, c := range val {
		if c >= '0' && c <= '9' {
			days = days*10 + int(c-'0')
		} else {
			break
		}
	}
	if days <= 0 {
		return defaultProxyExpiringThresholdDays
	}
	return days
}
