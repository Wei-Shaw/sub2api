package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// SubscriptionExpiryService periodically updates expired subscription status.
type SubscriptionExpiryService struct {
	userSubRepo              UserSubscriptionRepository
	notificationEmailService *NotificationEmailService
	interval                 time.Duration
	stopCh                   chan struct{REDACTED
	stopOnce                 sync.Once
	wg                       sync.WaitGroup
REDACTED

func NewSubscriptionExpiryService(userSubRepo UserSubscriptionRepository, interval time.Duration) *SubscriptionExpiryService {
	return &SubscriptionExpiryService{
		userSubRepo: userSubRepo,
		interval:    interval,
		stopCh:      make(chan struct{REDACTED),
REDACTED
REDACTED

func (s *SubscriptionExpiryService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
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
	s.sendExpiryReminders(ctx)
REDACTED

func (s *SubscriptionExpiryService) sendExpiryReminders(ctx context.Context) {
	if s == nil || s.userSubRepo == nil || s.notificationEmailService == nil {
		return
REDACTED
	for page := 1; ; page++ {
		subs, pag, err := s.userSubRepo.List(ctx, pagination.PaginationParams{Page: page, PageSize: 200REDACTED, nil, nil, SubscriptionStatusActive, "", "expires_at", "asc")
		if err != nil {
			log.Printf("[SubscriptionExpiry] List active subscriptions for reminder failed: %v", err)
			return
	REDACTED
		for i := range subs {
			s.sendExpiryReminderIfDue(ctx, &subs[i])
	REDACTED
		if pag == nil || page >= pag.Pages || len(subs) == 0 {
			return
	REDACTED
REDACTED
REDACTED

func (s *SubscriptionExpiryService) sendExpiryReminderIfDue(ctx context.Context, sub *UserSubscription) {
	if sub == nil || sub.User == nil || sub.Group == nil || sub.User.Email == "" {
		return
REDACTED
	daysRemaining := sub.DaysRemaining()
	if daysRemaining != 7 && daysRemaining != 3 && daysRemaining != 1 {
		return
REDACTED
	if err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionExpiryReminder,
		RecipientEmail: sub.User.Email,
		RecipientName:  firstNonEmpty(sub.User.Username, sub.User.Email),
		UserID:         sub.UserID,
		SourceType:     "user_subscription",
		SourceID:       strconv.FormatInt(sub.ID, 10),
		ReminderKey:    fmt.Sprintf("%dd", daysRemaining),
		Variables: map[string]string{
			"subscription_group": sub.Group.Name,
			"expiry_time":        sub.ExpiresAt.Format("2006-01-02 15:04"),
			"days_remaining":     strconv.Itoa(daysRemaining),
	REDACTED,
REDACTED); err != nil {
		log.Printf("[SubscriptionExpiry] Send expiry reminder failed: subscription=%d user=%d err=%v", sub.ID, sub.UserID, err)
REDACTED
REDACTED
