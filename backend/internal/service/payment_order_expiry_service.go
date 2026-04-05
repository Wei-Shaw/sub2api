package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// PaymentOrderExpiryService periodically expires timed-out payment orders.
type PaymentOrderExpiryService struct {
	paymentSvc *PaymentService
	interval   time.Duration
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

func NewPaymentOrderExpiryService(paymentSvc *PaymentService, interval time.Duration) *PaymentOrderExpiryService {
	return &PaymentOrderExpiryService{
		paymentSvc: paymentSvc,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

func (s *PaymentOrderExpiryService) Start() {
	if s == nil || s.paymentSvc == nil || s.interval <= 0 {
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

func (s *PaymentOrderExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *PaymentOrderExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	expired, err := s.paymentSvc.ExpireTimedOutOrders(ctx)
	if err != nil {
		log.Printf("[PaymentOrderExpiry] failed to expire orders: %v", err)
		return
	}
	if expired > 0 {
		log.Printf("[PaymentOrderExpiry] expired %d timed-out orders", expired)
	}
}
