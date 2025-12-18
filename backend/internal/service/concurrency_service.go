package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes
	accountConcurrencyKey = "concurrency:account:"
	userConcurrencyKey    = "concurrency:user:"
	userWaitCountKey      = "concurrency:wait:"

	// TTL for concurrency keys (auto-release safety net)
	concurrencyKeyTTL = 10 * time.Minute

	// Wait polling interval
	waitPollInterval = 100 * time.Millisecond

	// Default max wait time
	defaultMaxWait = 60 * time.Second

	// Default extra wait slots beyond concurrency limit
	defaultExtraWaitSlots = 20
)

// Pre-compiled Lua scripts for better performance
var (
	// acquireScript: increment counter if below max, return 1 if successful
	acquireScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		if current < tonumber(ARGV[1]) then
			redis.call('INCR', KEYS[1])
			redis.call('EXPIRE', KEYS[1], ARGV[2])
			return 1
		end
		return 0
	`)

	// releaseScript: decrement counter, but don't go below 0
	releaseScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current ~= false and tonumber(current) > 0 then
			redis.call('DECR', KEYS[1])
		end
		return 1
	`)

	// incrementWaitScript: increment wait counter if below max, return 1 if successful
	incrementWaitScript = redis.NewScript(`
		local waitKey = KEYS[1]
		local maxWait = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])
		local current = redis.call('GET', waitKey)
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		if current >= maxWait then
			return 0
		end
		redis.call('INCR', waitKey)
		redis.call('EXPIRE', waitKey, ttl)
		return 1
	`)

	// decrementWaitScript: decrement wait counter, but don't go below 0
	decrementWaitScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current ~= false and tonumber(current) > 0 then
			redis.call('DECR', KEYS[1])
		end
		return 1
	`)
)

// ConcurrencyService manages concurrent request limiting for accounts and users
type ConcurrencyService struct {
	rdb *redis.Client
REDACTED

// NewConcurrencyService creates a new ConcurrencyService
func NewConcurrencyService(rdb *redis.Client) *ConcurrencyService {
	return &ConcurrencyService{rdb: rdbREDACTED
REDACTED

// AcquireResult represents the result of acquiring a concurrency slot
type AcquireResult struct {
	Acquired   bool
	ReleaseFunc func() // Must be called when done (typically via defer)
REDACTED

// AcquireAccountSlot attempts to acquire a concurrency slot for an account.
// If the account is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	key := fmt.Sprintf("%s%d", accountConcurrencyKey, accountID)
	return s.acquireSlot(ctx, key, maxConcurrency)
REDACTED

// AcquireUserSlot attempts to acquire a concurrency slot for a user.
// If the user is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (*AcquireResult, error) {
	key := fmt.Sprintf("%s%d", userConcurrencyKey, userID)
	return s.acquireSlot(ctx, key, maxConcurrency)
REDACTED

// acquireSlot is the core implementation for acquiring a concurrency slot
func (s *ConcurrencyService) acquireSlot(ctx context.Context, key string, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {REDACTED, // no-op
	REDACTED, nil
REDACTED

	// Try to acquire immediately
	acquired, err := s.tryAcquire(ctx, key, maxConcurrency)
	if err != nil {
		return nil, err
REDACTED

	if acquired {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: s.makeReleaseFunc(key),
	REDACTED, nil
REDACTED

	// Not acquired, return with Acquired=false
	// The caller (gateway handler) will handle waiting with ping support
	return &AcquireResult{
		Acquired:    false,
		ReleaseFunc: nil,
REDACTED, nil
REDACTED

// tryAcquire attempts to increment the counter if below max
// Uses pre-compiled Lua script for atomicity and performance
func (s *ConcurrencyService) tryAcquire(ctx context.Context, key string, maxConcurrency int) (bool, error) {
	result, err := acquireScript.Run(ctx, s.rdb, []string{keyREDACTED, maxConcurrency, int(concurrencyKeyTTL.Seconds())).Int()
	if err != nil {
		return false, fmt.Errorf("acquire slot failed: %w", err)
REDACTED
	return result == 1, nil
REDACTED

// makeReleaseFunc creates a function to release a concurrency slot
func (s *ConcurrencyService) makeReleaseFunc(key string) func() {
	return func() {
		// Use background context to ensure release even if original context is cancelled
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := releaseScript.Run(ctx, s.rdb, []string{keyREDACTED).Err(); err != nil {
			// Log error but don't panic - TTL will eventually clean up
			log.Printf("Warning: failed to release concurrency slot for %s: %v", key, err)
	REDACTED
REDACTED
REDACTED

// GetCurrentCount returns the current concurrency count for debugging/monitoring
func (s *ConcurrencyService) GetCurrentCount(ctx context.Context, key string) (int, error) {
	val, err := s.rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
REDACTED
	if err != nil {
		return 0, err
REDACTED
	return val, nil
REDACTED

// GetAccountCurrentCount returns current concurrency count for an account
func (s *ConcurrencyService) GetAccountCurrentCount(ctx context.Context, accountID int64) (int, error) {
	key := fmt.Sprintf("%s%d", accountConcurrencyKey, accountID)
	return s.GetCurrentCount(ctx, key)
REDACTED

// GetUserCurrentCount returns current concurrency count for a user
func (s *ConcurrencyService) GetUserCurrentCount(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", userConcurrencyKey, userID)
	return s.GetCurrentCount(ctx, key)
REDACTED

// ============================================
// Wait Queue Count Methods
// ============================================

// IncrementWaitCount attempts to increment the wait queue counter for a user.
// Returns true if successful, false if the wait queue is full.
// maxWait should be user.Concurrency + defaultExtraWaitSlots
func (s *ConcurrencyService) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	if s.rdb == nil {
		// Redis not available, allow request
		return true, nil
REDACTED

	key := fmt.Sprintf("%s%d", userWaitCountKey, userID)
	result, err := incrementWaitScript.Run(ctx, s.rdb, []string{keyREDACTED, maxWait, int(concurrencyKeyTTL.Seconds())).Int()
	if err != nil {
		// On error, allow the request to proceed (fail open)
		log.Printf("Warning: increment wait count failed for user %d: %v", userID, err)
		return true, nil
REDACTED
	return result == 1, nil
REDACTED

// DecrementWaitCount decrements the wait queue counter for a user.
// Should be called when a request completes or exits the wait queue.
func (s *ConcurrencyService) DecrementWaitCount(ctx context.Context, userID int64) {
	if s.rdb == nil {
		return
REDACTED

	key := fmt.Sprintf("%s%d", userWaitCountKey, userID)
	// Use background context to ensure decrement even if original context is cancelled
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := decrementWaitScript.Run(bgCtx, s.rdb, []string{keyREDACTED).Err(); err != nil {
		log.Printf("Warning: decrement wait count failed for user %d: %v", userID, err)
REDACTED
REDACTED

// GetUserWaitCount returns current wait queue count for a user
func (s *ConcurrencyService) GetUserWaitCount(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", userWaitCountKey, userID)
	return s.GetCurrentCount(ctx, key)
REDACTED

// CalculateMaxWait calculates the maximum wait queue size for a user
// maxWait = userConcurrency + defaultExtraWaitSlots
func CalculateMaxWait(userConcurrency int) int {
	if userConcurrency <= 0 {
		userConcurrency = 1
REDACTED
	return userConcurrency + defaultExtraWaitSlots
REDACTED
