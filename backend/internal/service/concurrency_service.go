package service

import (
	"context"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

const (
	// Default extra wait slots beyond concurrency limit
	defaultExtraWaitSlots = 20
)

// ConcurrencyService manages concurrent request limiting for accounts and users
type ConcurrencyService struct {
	cache ports.ConcurrencyCache
REDACTED

// NewConcurrencyService creates a new ConcurrencyService
func NewConcurrencyService(cache ports.ConcurrencyCache) *ConcurrencyService {
	return &ConcurrencyService{cache: cacheREDACTED
REDACTED

// AcquireResult represents the result of acquiring a concurrency slot
type AcquireResult struct {
	Acquired    bool
	ReleaseFunc func() // Must be called when done (typically via defer)
REDACTED

// AcquireAccountSlot attempts to acquire a concurrency slot for an account.
// If the account is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {REDACTED, // no-op
	REDACTED, nil
REDACTED

	acquired, err := s.cache.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, err
REDACTED

	if acquired {
		return &AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cache.ReleaseAccountSlot(bgCtx, accountID); err != nil {
					log.Printf("Warning: failed to release account slot for %d: %v", accountID, err)
			REDACTED
		REDACTED,
	REDACTED, nil
REDACTED

	return &AcquireResult{
		Acquired:    false,
		ReleaseFunc: nil,
REDACTED, nil
REDACTED

// AcquireUserSlot attempts to acquire a concurrency slot for a user.
// If the user is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {REDACTED, // no-op
	REDACTED, nil
REDACTED

	acquired, err := s.cache.AcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, err
REDACTED

	if acquired {
		return &AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cache.ReleaseUserSlot(bgCtx, userID); err != nil {
					log.Printf("Warning: failed to release user slot for %d: %v", userID, err)
			REDACTED
		REDACTED,
	REDACTED, nil
REDACTED

	return &AcquireResult{
		Acquired:    false,
		ReleaseFunc: nil,
REDACTED, nil
REDACTED

// ============================================
// Wait Queue Count Methods
// ============================================

// IncrementWaitCount attempts to increment the wait queue counter for a user.
// Returns true if successful, false if the wait queue is full.
// maxWait should be user.Concurrency + defaultExtraWaitSlots
func (s *ConcurrencyService) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	if s.cache == nil {
		// Redis not available, allow request
		return true, nil
REDACTED

	result, err := s.cache.IncrementWaitCount(ctx, userID, maxWait)
	if err != nil {
		// On error, allow the request to proceed (fail open)
		log.Printf("Warning: increment wait count failed for user %d: %v", userID, err)
		return true, nil
REDACTED
	return result, nil
REDACTED

// DecrementWaitCount decrements the wait queue counter for a user.
// Should be called when a request completes or exits the wait queue.
func (s *ConcurrencyService) DecrementWaitCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
REDACTED

	// Use background context to ensure decrement even if original context is cancelled
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.cache.DecrementWaitCount(bgCtx, userID); err != nil {
		log.Printf("Warning: decrement wait count failed for user %d: %v", userID, err)
REDACTED
REDACTED

// CalculateMaxWait calculates the maximum wait queue size for a user
// maxWait = userConcurrency + defaultExtraWaitSlots
func CalculateMaxWait(userConcurrency int) int {
	if userConcurrency <= 0 {
		userConcurrency = 1
REDACTED
	return userConcurrency + defaultExtraWaitSlots
REDACTED

// GetAccountConcurrencyBatch gets current concurrency counts for multiple accounts
// Returns a map of accountID -> current concurrency count
func (s *ConcurrencyService) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int)

	for _, accountID := range accountIDs {
		count, err := s.cache.GetAccountConcurrency(ctx, accountID)
		if err != nil {
			// If key doesn't exist in Redis, count is 0
			count = 0
	REDACTED
		result[accountID] = count
REDACTED

	return result, nil
REDACTED
