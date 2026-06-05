package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

const expiryCheckTimeout = 30 * time.Second

const (
	// paymentOrderExpiryLeaderLockKey gates the periodic reconcile + expiry sweep so
	// that only one instance issues the upstream payment-provider calls per cycle.
	paymentOrderExpiryLeaderLockKey = "payment:order:expiry:leader"
	// paymentOrderExpiryLeaderLockTTL must exceed the combined reconcile + expiry
	// timeouts (2 * expiryCheckTimeout) so the lock never expires mid-run.
	paymentOrderExpiryLeaderLockTTL = 3 * time.Minute
)

// PaymentOrderExpiryService periodically expires timed-out payment orders.
type PaymentOrderExpiryService struct {
	paymentSvc *PaymentService
	interval   time.Duration
	stopCh     chan struct{REDACTED
	stopOnce   sync.Once
	wg         sync.WaitGroup

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
REDACTED

func NewPaymentOrderExpiryService(paymentSvc *PaymentService, interval time.Duration) *PaymentOrderExpiryService {
	return &PaymentOrderExpiryService{
		paymentSvc: paymentSvc,
		interval:   interval,
		stopCh:     make(chan struct{REDACTED),
		instanceID: uuid.NewString(),
REDACTED
REDACTED

// SetLeaderLock injects the leader-lock cache and DB used to elect a single
// instance for the periodic reconcile/expiry sweep. When both are nil the job
// runs ungated (single-instance / test behavior).
func (s *PaymentOrderExpiryService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
REDACTED
	s.lockCache = lockCache
	s.db = db
REDACTED

func (s *PaymentOrderExpiryService) Start() {
	if s == nil || s.paymentSvc == nil || s.interval <= 0 {
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

func (s *PaymentOrderExpiryService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
	s.wg.Wait()
REDACTED

func (s *PaymentOrderExpiryService) runOnce() {
	// Multi-instance guard: only the leader reconciles/expires orders per cycle,
	// avoiding N× upstream payment-provider API calls and update races.
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 2*time.Second)
	release, ok := tryAcquireSingletonLeaderLock(lockCtx, s.lockCache, s.db, paymentOrderExpiryLeaderLockKey, s.instanceID, paymentOrderExpiryLeaderLockTTL)
	lockCancel()
	if !ok {
		return
REDACTED
	defer release()

	reconcileCtx, cancel := context.WithTimeout(context.Background(), expiryCheckTimeout)
	recovered, err := s.paymentSvc.ReconcilePendingWxpayOrders(reconcileCtx)
	cancel()
	if err != nil {
		slog.Warn("[PaymentOrderExpiry] failed to reconcile pending wxpay orders", "error", err)
REDACTED else if recovered > 0 {
		slog.Info("[PaymentOrderExpiry] reconciled paid wxpay orders", "count", recovered)
REDACTED

	expireCtx, cancel := context.WithTimeout(context.Background(), expiryCheckTimeout)
	defer cancel()
	expired, err := s.paymentSvc.ExpireTimedOutOrders(expireCtx)
	if err != nil {
		slog.Error("[PaymentOrderExpiry] failed to expire orders", "error", err)
		return
REDACTED
	if expired > 0 {
		slog.Info("[PaymentOrderExpiry] expired timed-out orders", "count", expired)
REDACTED
REDACTED
