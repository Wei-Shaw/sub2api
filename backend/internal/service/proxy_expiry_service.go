package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// ProxyExpiryService 周期扫描到期代理并把绑定账号改投备用/直连。
type ProxyExpiryService struct {
	proxyRepo ProxyRepository
	interval  time.Duration
	stopCh    chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

func NewProxyExpiryService(proxyRepo ProxyRepository, interval time.Duration) *ProxyExpiryService {
	return &ProxyExpiryService{proxyRepo: proxyRepo, interval: interval, stopCh: make(chan struct{})}
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
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *ProxyExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	changed, err := s.proxyRepo.SweepExpiredProxies(ctx, time.Now())
	if err != nil {
		log.Printf("[ProxyExpiry] sweep expired proxies failed: %v", err)
		return
	}
	if changed > 0 {
		log.Printf("[ProxyExpiry] re-routed %d accounts off expired proxies", changed)
	}
}
