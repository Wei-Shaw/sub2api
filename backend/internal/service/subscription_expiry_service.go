package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// SubscriptionExpiryService periodically updates expired subscription status.
type SubscriptionExpiryService struct {
	userSubRepo UserSubscriptionRepository
	interval    time.Duration
	stopCh      chan struct{REDACTED
	stopOnce    sync.Once
	wg          sync.WaitGroup
REDACTED

func NewSubscriptionExpiryService(userSubRepo UserSubscriptionRepository, interval time.Duration) *SubscriptionExpiryService {
	return &SubscriptionExpiryService{
		userSubRepo: userSubRepo,
		interval:    interval,
		stopCh:      make(chan struct{REDACTED),
REDACTED
REDACTED

func (s *SubscriptionExpiryService) Start() {
	if s == nil || s.userSubRepo == nil || s.interval <= 0 {
		return
REDACTED
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
		REDACTED
	REDACTED
REDACTED()
REDACTED

func (s *SubscriptionExpiryService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
	s.wg.Wait()
REDACTED

func (s *SubscriptionExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updated, err := s.userSubRepo.BatchUpdateExpiredStatus(ctx)
	if err != nil {
		log.Printf("[SubscriptionExpiry] Update expired subscriptions failed: %v", err)
		return
REDACTED
	if updated > 0 {
		log.Printf("[SubscriptionExpiry] Updated %d expired subscriptions", updated)
REDACTED
REDACTED
