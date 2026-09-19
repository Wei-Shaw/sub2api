package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// APIKeyQueuePolicy is the immutable, process-local policy for the key-level
// wait queue. MaxWaiting == 0 disables queueing while concurrency limits remain.
type APIKeyQueuePolicy struct {
	MaxWaiting int
	Timeout    time.Duration
}

// APIKeyQueueOutcome is the Redis queue state machine result. The numeric
// values are the Lua return codes so the repository can pass them through.
type APIKeyQueueOutcome int

const (
	APIKeyQueueOutcomeAcquired      APIKeyQueueOutcome = 1
	APIKeyQueueOutcomeWaiting       APIKeyQueueOutcome = 2
	APIKeyQueueOutcomeTimeout       APIKeyQueueOutcome = 3
	APIKeyQueueOutcomeAborted       APIKeyQueueOutcome = 4
	APIKeyQueueOutcomeTicketLost    APIKeyQueueOutcome = 5
	APIKeyQueueOutcomePolicyChanged APIKeyQueueOutcome = 6
	APIKeyQueueOutcomeQueueFull     APIKeyQueueOutcome = 7
	APIKeyQueueOutcomeLimitReached  APIKeyQueueOutcome = 8
)

// APIKeyQueueMode selects the queue Lua branch: ENTER is used only before a
// ticket is confirmed, POLL only after a confirmed WAITING reply.
type APIKeyQueueMode int

const (
	APIKeyQueueModeEnter APIKeyQueueMode = 1
	APIKeyQueueModePoll  APIKeyQueueMode = 2
)

// APIKeySlotQueueCache owns the atomic admission/wait state machine.
type APIKeySlotQueueCache interface {
	APIKeySlotLeaseCache
	// AcquireAPIKeySlotWithTime attempts one immediate admission. On BUSY it
	// returns the Redis server time in milliseconds so the caller can derive a
	// conservative, fixed queue deadline without an extra round trip.
	AcquireAPIKeySlotWithTime(ctx context.Context, apiKeyID int64, maxConcurrency int, requestID string) (bool, int64, error)
	// AdvanceAPIKeyQueue performs one atomic ENTER or POLL step.
	AdvanceAPIKeyQueue(ctx context.Context, apiKeyID int64, requestID string, mode APIKeyQueueMode, deadlineMs int64, keyLimit int, maxWaiting int) (APIKeyQueueOutcome, error)
	// AbortAPIKeyQueueAttempt fences late ENTER/POLL commands and removes this
	// exact attempt from the wait set, plus the regular set when removeSlot is
	// true. A confirmed ACQUIRED must pass false so the new reservation survives.
	AbortAPIKeyQueueAttempt(ctx context.Context, apiKeyID int64, requestID string, deadlineMs int64, removeSlot bool) error
	// GetAPIKeyQueueStats returns active and waiting counts in one atomic read.
	GetAPIKeyQueueStats(ctx context.Context, apiKeyID int64) (active int, waiting int, err error)
}

// ErrAPIKeyReservationLost means a Live handoff could not find the regular key
// reservation that was supposed to be transferred. The caller must not proceed.
var ErrAPIKeyReservationLost = errors.New("API key reservation lost before Live handoff")

// APIKeyQueueErrorKind classifies admission failures for protocol mapping.
type APIKeyQueueErrorKind int

const (
	APIKeyQueueErrorFull APIKeyQueueErrorKind = iota + 1
	APIKeyQueueErrorTimeout
	APIKeyQueueErrorUnavailable
	APIKeyQueueErrorPolicyChanged
	// APIKeyQueueErrorAuthRejected means the queued request's authentication
	// snapshot became invalid while it waited. The cause carries the same
	// application error the authentication middleware would return.
	APIKeyQueueErrorAuthRejected
)

type APIKeyQueueError struct {
	Kind  APIKeyQueueErrorKind
	Cause error
}

func (e *APIKeyQueueError) Error() string {
	switch e.Kind {
	case APIKeyQueueErrorFull:
		return "API key wait queue is full"
	case APIKeyQueueErrorTimeout:
		return "timed out waiting for API key concurrency slot"
	case APIKeyQueueErrorPolicyChanged:
		return "API key concurrency policy changed while waiting"
	case APIKeyQueueErrorAuthRejected:
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "API key authentication changed while waiting"
	default:
		return "API key wait queue is temporarily unavailable"
	}
}

// APIKeyQueueAuthRejectedError marks a revalidation result as a final
// authentication rejection: waiting stops and the caller surfaces Err like the
// initial authentication would. It distinguishes a definite rejection from a
// transient revalidation failure (which maps to a service error instead).
type APIKeyQueueAuthRejectedError struct {
	Err error
}

// NewAPIKeyQueueAuthRejected wraps a definite authentication/authorization
// failure for the queue revalidator.
func NewAPIKeyQueueAuthRejected(err error) *APIKeyQueueAuthRejectedError {
	return &APIKeyQueueAuthRejectedError{Err: err}
}

func (e *APIKeyQueueAuthRejectedError) Error() string {
	if e == nil || e.Err == nil {
		return "API key authentication changed while waiting"
	}
	return e.Err.Error()
}

func (e *APIKeyQueueAuthRejectedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// APIKeyQueueAuthRevalidator re-reads the request's live authentication state
// (disabled/deleted/expired/group/limit) through the existing L1/L2 auth cache
// and returns the current concurrency limit. It is installed by API-key auth
// middleware and must not log the credential it closes over.
type APIKeyQueueAuthRevalidator func(ctx context.Context) (int, error)

type apiKeyQueueAuthRevalidatorKey struct{}

// WithAPIKeyQueueAuthRevalidator installs the bounded revalidation callback on
// the request context. A nil callback leaves waiting without revalidation.
func WithAPIKeyQueueAuthRevalidator(ctx context.Context, revalidator APIKeyQueueAuthRevalidator) context.Context {
	if ctx == nil || revalidator == nil {
		return ctx
	}
	return context.WithValue(ctx, apiKeyQueueAuthRevalidatorKey{}, revalidator)
}

func apiKeyQueueAuthRevalidatorFromContext(ctx context.Context) APIKeyQueueAuthRevalidator {
	if ctx == nil {
		return nil
	}
	revalidator, _ := ctx.Value(apiKeyQueueAuthRevalidatorKey{}).(APIKeyQueueAuthRevalidator)
	return revalidator
}

// HasAPIKeyQueueAuthRevalidator reports whether the request carries the
// per-request revalidator. WS turns use it to keep the handshake allowlist as
// defense-in-depth only when no fresh per-turn gate was installed.
func HasAPIKeyQueueAuthRevalidator(ctx context.Context) bool {
	return apiKeyQueueAuthRevalidatorFromContext(ctx) != nil
}

// APIKeyQueueCapability identifies one group-gated capability that the current
// request actually uses. The queue revalidator only re-checks capabilities a
// request exercised, so unrelated group permission changes never interrupt a
// wait.
type APIKeyQueueCapability uint8

const (
	// APIKeyQueueCapabilityImageGeneration marks an explicit image-generation
	// request (image endpoints or a native image_generation tool).
	APIKeyQueueCapabilityImageGeneration APIKeyQueueCapability = 1 << iota
	// APIKeyQueueCapabilityLive marks a Live creation request.
	APIKeyQueueCapabilityLive
	// APIKeyQueueCapabilityMessagesDispatch marks an OpenAI-compatible
	// /v1/messages dispatch that was governed by the group switch (Grok/CN and
	// composite-to-Grok/CN are exempt and must not record it).
	APIKeyQueueCapabilityMessagesDispatch
	// APIKeyQueueCapabilityForbiddenWhenClaudeCodeOnly marks an endpoint that a
	// claude_code_only group rejects (/v1/chat/completions, /v1/responses).
	APIKeyQueueCapabilityForbiddenWhenClaudeCodeOnly
)

// APIKeyQueueRequestPermissions is the immutable per-request authorization
// requirement captured before admission: the client-written model candidates
// (duplicate keys and case variants included) and the capabilities this request
// actually uses. Middleware and the WS handler install it on the request/turn
// context before waiting, so the revalidator never reads mutable Gin state.
type APIKeyQueueRequestPermissions struct {
	Models       []string
	Capabilities APIKeyQueueCapability
}

type apiKeyQueueRequestPermissionsKey struct{}

// WithAPIKeyQueueRequestPermissions installs or replaces the immutable
// per-request permissions. The model slice is copied so callers cannot mutate
// an admitted wait's snapshot afterwards.
func WithAPIKeyQueueRequestPermissions(ctx context.Context, permissions APIKeyQueueRequestPermissions) context.Context {
	if ctx == nil {
		return ctx
	}
	if len(permissions.Models) > 0 {
		permissions.Models = append([]string(nil), permissions.Models...)
	}
	return context.WithValue(ctx, apiKeyQueueRequestPermissionsKey{}, permissions)
}

// APIKeyQueueRequestPermissionsFromContext returns the permissions captured for
// this request; the zero value means "no request-level checks".
func APIKeyQueueRequestPermissionsFromContext(ctx context.Context) APIKeyQueueRequestPermissions {
	if ctx == nil {
		return APIKeyQueueRequestPermissions{}
	}
	permissions, _ := ctx.Value(apiKeyQueueRequestPermissionsKey{}).(APIKeyQueueRequestPermissions)
	return permissions
}

// WithAPIKeyQueueCapability records that the current request actually uses one
// more group-gated capability, preserving permissions already captured for the
// same request.
func WithAPIKeyQueueCapability(ctx context.Context, capability APIKeyQueueCapability) context.Context {
	if ctx == nil || capability == 0 {
		return ctx
	}
	permissions := APIKeyQueueRequestPermissionsFromContext(ctx)
	permissions.Capabilities |= capability
	return context.WithValue(ctx, apiKeyQueueRequestPermissionsKey{}, permissions)
}

// APIKeyQueueImagePermission is a tiny per-request holder the queue revalidator
// updates with the freshest group image-generation permission it observed.
// Forwarding gates read it so a permission changed after handshake (relaxed or
// revoked) is judged from the latest revalidation instead of the handshake
// Group snapshot. It deliberately stores one fact, not a Group replacement.
type APIKeyQueueImagePermission struct {
	mu      sync.Mutex
	allowed bool
	known   bool
}

// NewAPIKeyQueueImagePermission creates an unresolved permission holder.
func NewAPIKeyQueueImagePermission() *APIKeyQueueImagePermission {
	return &APIKeyQueueImagePermission{}
}

// Set records the latest observed image-generation permission.
func (p *APIKeyQueueImagePermission) Set(allowed bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.allowed = allowed
	p.known = true
	p.mu.Unlock()
}

// Allowed returns the latest observed permission and whether one was recorded.
func (p *APIKeyQueueImagePermission) Allowed() (bool, bool) {
	if p == nil {
		return false, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.allowed, p.known
}

type apiKeyQueueImagePermissionKey struct{}

// WithAPIKeyQueueImagePermission installs the per-request permission holder.
func WithAPIKeyQueueImagePermission(ctx context.Context, permission *APIKeyQueueImagePermission) context.Context {
	if ctx == nil || permission == nil {
		return ctx
	}
	return context.WithValue(ctx, apiKeyQueueImagePermissionKey{}, permission)
}

// APIKeyQueueImagePermissionFromContext returns the installed holder, if any.
func APIKeyQueueImagePermissionFromContext(ctx context.Context) *APIKeyQueueImagePermission {
	if ctx == nil {
		return nil
	}
	permission, _ := ctx.Value(apiKeyQueueImagePermissionKey{}).(*APIKeyQueueImagePermission)
	return permission
}

// GroupAllowsImageGenerationLatest prefers the latest revalidated image
// permission and falls back to the handshake group snapshot when no
// revalidation has run for this request.
func GroupAllowsImageGenerationLatest(ctx context.Context, group *Group) bool {
	if permission := APIKeyQueueImagePermissionFromContext(ctx); permission != nil {
		if allowed, known := permission.Allowed(); known {
			return allowed
		}
	}
	return GroupAllowsImageGeneration(group)
}

// RevalidateAPIKeyQueueTurn runs the request's installed revalidator once under
// the same bounded context the wait loop uses. ok is false when no revalidator
// is installed. WS turns call this before any capacity decision so unlimited or
// queue-disabled keys still see fresh permissions.
func (s *ConcurrencyService) RevalidateAPIKeyQueueTurn(ctx context.Context) (limit int, ok bool, err error) {
	revalidator := apiKeyQueueAuthRevalidatorFromContext(ctx)
	if revalidator == nil {
		return 0, false, nil
	}
	limit, err = s.revalidateAPIKeyQueueAuth(ctx, revalidator)
	if err != nil {
		return 0, true, err
	}
	return limit, true, nil
}

func (e *APIKeyQueueError) Unwrap() error { return e.Cause }

func apiKeyQueueError(kind APIKeyQueueErrorKind, cause error) *APIKeyQueueError {
	return &APIKeyQueueError{Kind: kind, Cause: cause}
}

// IsAPIKeyQueueErrorKind matches a classified queue failure.
func IsAPIKeyQueueErrorKind(err error, kind APIKeyQueueErrorKind) bool {
	var queueErr *APIKeyQueueError
	return errors.As(err, &queueErr) && queueErr.Kind == kind
}

const (
	apiKeyQueueInitialBackoff = 100 * time.Millisecond
	apiKeyQueueMaxBackoff     = 2 * time.Second
	apiKeyQueueBackoffFactor  = 1.5
	apiKeyQueueAbortTimeout   = 5 * time.Second
	apiKeyQueueAbortAttempts  = 3
)

// apiKeyQueueStopState is shared by every wait loop so shutdown can cancel them.
// The context lets an in-flight Redis operation be canceled at stop, not only
// the backoff select.
type apiKeyQueueStopState struct {
	once   atomic.Bool
	ch     chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

func newAPIKeyQueueStopState() *apiKeyQueueStopState {
	ctx, cancel := context.WithCancel(context.Background())
	return &apiKeyQueueStopState{ch: make(chan struct{}), ctx: ctx, cancel: cancel}
}

// APIKeySlotReservation is an owned regular key slot that can be released, or
// transferred atomically into a Live lease by the Live owner.
type APIKeySlotReservation struct {
	apiKeyID     int64
	requestID    string
	limit        int
	release      func()
	pauseRenewal func()
	consumed     atomic.Bool
}

// APIKeyID reports the owned key.
func (r *APIKeySlotReservation) APIKeyID() int64 {
	if r == nil {
		return 0
	}
	return r.apiKeyID
}

// EffectiveLimit is the limit actually used to admit this reservation. Zero
// means the reservation is stats-only: it still has an exact member identity
// that a Live transfer can move atomically, but no key capacity was enforced.
func (r *APIKeySlotReservation) EffectiveLimit() int {
	if r == nil {
		return 0
	}
	return r.limit
}

// StatsOnly reports whether the reservation only tracks statistics (the key's
// effective limit is 0) and must never be treated as an enforced slot.
func (r *APIKeySlotReservation) StatsOnly() bool {
	return r.EffectiveLimit() == 0
}

// RequestID exposes the exact member identity for Live transfer. Empty means
// the reservation has already been consumed.
func (r *APIKeySlotReservation) RequestID() string {
	if r == nil || r.consumed.Load() {
		return ""
	}
	return r.requestID
}

// Release stops renewal and removes the slot after any upstream work stopped.
// A consumed reservation belongs to the Live transfer owner and is only
// stopped, never deleted.
func (r *APIKeySlotReservation) Release() {
	if r == nil {
		return
	}
	if r.consumed.Load() {
		r.pause()
		return
	}
	if r.release != nil {
		r.release()
	}
}

// PauseRenewal stops local renewal while leaving the member in place for the
// Live transfer. The caller must then either Consume the reservation or abort
// the transfer with the exact member identity.
func (r *APIKeySlotReservation) PauseRenewal() {
	if r == nil || r.consumed.Load() {
		return
	}
	r.pause()
}

func (r *APIKeySlotReservation) pause() {
	if r == nil || r.pauseRenewal == nil {
		return
	}
	r.pauseRenewal()
}

// Consume marks the reservation as owned by a transfer successor so the
// original release path never deletes the successor's member. Renewal is
// stopped: the member is either the Live member already or cleaned by the
// transfer abort.
func (r *APIKeySlotReservation) Consume() {
	if r == nil {
		return
	}
	if r.consumed.CompareAndSwap(false, true) {
		r.pause()
	}
}

// SetAPIKeyQueuePolicy installs the immutable process policy. It is called once
// during wiring and is safe to read concurrently afterwards.
func (s *ConcurrencyService) SetAPIKeyQueuePolicy(policy APIKeyQueuePolicy) {
	if s == nil {
		return
	}
	if policy.Timeout <= 0 {
		policy.Timeout = 30 * time.Second
	}
	s.apiKeyQueuePolicy.Store(&policy)
}

// APIKeyQueuePolicy returns the policy loaded by this process.
func (s *ConcurrencyService) APIKeyQueuePolicy() APIKeyQueuePolicy {
	if s == nil {
		return APIKeyQueuePolicy{}
	}
	policy := s.apiKeyQueuePolicy.Load()
	if policy == nil {
		return APIKeyQueuePolicy{}
	}
	return *policy
}

// APIKeyQueueCounts is one read-only snapshot: active execution members plus
// confirmed waiters.
type APIKeyQueueCounts struct {
	Active  int
	Waiting int
}

// GetAPIKeyQueueStatsBatch reads per-key active and waiting counts. Any Redis
// failure fails the whole batch so callers cannot display a fake zero.
func (s *ConcurrencyService) GetAPIKeyQueueStatsBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]APIKeyQueueCounts, error) {
	result := make(map[int64]APIKeyQueueCounts, len(apiKeyIDs))
	if len(apiKeyIDs) == 0 {
		return result, nil
	}
	if s == nil || s.cache == nil {
		return nil, errors.New("API key queue statistics unavailable")
	}
	cache, ok := s.cache.(APIKeySlotQueueCache)
	if !ok {
		return nil, errors.New("API key queue statistics unsupported")
	}
	redisCtx, cancel := context.WithTimeout(context.Background(), apiKeyConcurrencyFetchTimeout)
	defer cancel()
	for _, apiKeyID := range apiKeyIDs {
		active, waiting, err := cache.GetAPIKeyQueueStats(redisCtx, apiKeyID)
		if err != nil {
			return nil, fmt.Errorf("read api key %d queue statistics: %w", apiKeyID, err)
		}
		result[apiKeyID] = APIKeyQueueCounts{Active: active, Waiting: waiting}
	}
	return result, nil
}

// StopAPIKeyQueue cancels wait loops and in-flight admission operations.
// Running slots are owned by their forwarding owners and are not released here.
func (s *ConcurrencyService) StopAPIKeyQueue() {
	if s == nil || s.apiKeyQueueStop == nil {
		return
	}
	if s.apiKeyQueueStop.once.CompareAndSwap(false, true) {
		s.apiKeyQueueStop.cancel()
		close(s.apiKeyQueueStop.ch)
	}
}

func (s *ConcurrencyService) apiKeyQueueStopped() bool {
	if s == nil || s.apiKeyQueueStop == nil {
		return false
	}
	return s.apiKeyQueueStop.ctx.Err() != nil
}

func (s *ConcurrencyService) apiKeyQueueStopContext() context.Context {
	if s == nil || s.apiKeyQueueStop == nil {
		return nil
	}
	return s.apiKeyQueueStop.ctx
}

// apiKeyQueueOperationContext bounds one Redis call by the remaining queue
// budget and cancels it when the service stops accepting waiters.
func (s *ConcurrencyService) apiKeyQueueOperationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	opCtx, cancel := context.WithTimeout(parent, timeout)
	stopCtx := s.apiKeyQueueStopContext()
	if stopCtx == nil {
		return opCtx, cancel
	}
	stopCancel := context.AfterFunc(stopCtx, cancel)
	return opCtx, func() {
		stopCancel()
		cancel()
	}
}

// AcquireAPIKeySlotWithWait acquires a key slot, waiting when the queue is
// enabled. It returns Acquired=false (no error) only for the legacy immediate
// rejection path; queue failures are typed errors.
func (s *ConcurrencyService) AcquireAPIKeySlotWithWait(ctx context.Context, apiKeyID int64, maxConcurrency int) (*AcquireResult, error) {
	reservation, err := s.ReserveAPIKeySlotWithWait(ctx, apiKeyID, maxConcurrency)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return &AcquireResult{Acquired: false}, nil
	}
	return &AcquireResult{Acquired: true, ReleaseFunc: reservation.Release}, nil
}

// ReserveAPIKeySlotWithWait returns an owned reservation for forwarding paths
// that need the slot identity (Live transfer). A nil reservation with nil error
// means "capacity full and queueing disabled" so callers keep the existing 429.
//
// When the request context carries an APIKeyQueueAuthRevalidator (installed by
// API-key auth middleware), the current authentication snapshot is re-read
// through the existing auth cache before the first attempt, before every queue
// retry and again before the confirmed slot is handed over. A key whose limit
// became 0 is cleaned from the queue and switched to the unlimited tracking
// path; a disabled/deleted/expired/changed key stops waiting with the same
// authentication error.
func (s *ConcurrencyService) ReserveAPIKeySlotWithWait(ctx context.Context, apiKeyID int64, maxConcurrency int) (*APIKeySlotReservation, error) {
	if maxConcurrency == 0 {
		return s.trackAPIKeyReservation(ctx, apiKeyID), nil
	}
	if s == nil || s.cache == nil || apiKeyID <= 0 || maxConcurrency < 0 {
		return nil, fmt.Errorf("API key concurrency admission unavailable")
	}
	if s.apiKeyQueueStopped() {
		return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("service is shutting down"))
	}
	cache, ok := s.cache.(APIKeySlotAdmissionCache)
	if !ok {
		return nil, fmt.Errorf("API key concurrency admission unavailable")
	}
	policy := s.APIKeyQueuePolicy()
	queueCache, queueOK := s.cache.(APIKeySlotQueueCache)
	if !queueOK || policy.MaxWaiting == 0 {
		return s.reserveAPIKeySlotImmediate(ctx, cache, apiKeyID, maxConcurrency)
	}

	owner, ownerOK := apiKeyAdmissionOwnerFromContext(ctx)
	if !ownerOK || queueCache.APIKeySlotTTL() <= 2*time.Second || queueCache.APIKeySlotRefreshInterval() <= 0 {
		return nil, fmt.Errorf("API key concurrency requires a renewable lease and cancellation owner")
	}

	revalidator := apiKeyQueueAuthRevalidatorFromContext(ctx)
	keyLimit := maxConcurrency
	if revalidator != nil {
		limit, err := s.revalidateAPIKeyQueueAuth(ctx, revalidator)
		if err != nil {
			return nil, err
		}
		if limit == 0 {
			// No queue is needed for a key that is now unlimited.
			return s.trackAPIKeyReservation(ctx, apiKeyID), nil
		}
		keyLimit = limit
	}

	requestID := generateRequestID()
	started := time.Now()
	firstCtx, cancelFirst := s.apiKeyQueueOperationContext(ctx, apiKeySlotTrackTimeout)
	acquired, redisNowMs, err := queueCache.AcquireAPIKeySlotWithTime(firstCtx, apiKeyID, keyLimit, requestID)
	cancelFirst()
	if err != nil {
		// The write may have succeeded even if its response was lost. Do not
		// continue into the queue with an unknown immediate admission.
		releaseAPIKeySlot(queueCache, apiKeyID, requestID)
		return nil, fmt.Errorf("acquire API key %d concurrency slot: %w", apiKeyID, err)
	}
	if acquired {
		return s.adoptAPIKeyGrantOrRelease(ctx, queueCache, owner, apiKeyID, requestID, keyLimit, started)
	}

	localDeadline := started.Add(policy.Timeout)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(localDeadline) {
		localDeadline = deadline
	}
	remaining := time.Until(localDeadline)
	if remaining <= 0 {
		s.abortAPIKeyQueueAttempt(queueCache, apiKeyID, requestID, 0, true)
		return nil, apiKeyQueueError(APIKeyQueueErrorTimeout, nil)
	}
	deadlineMs := redisNowMs + remaining.Milliseconds()
	if deadlineMs <= redisNowMs {
		deadlineMs = redisNowMs + 1
	}
	return s.waitAPIKeyQueue(ctx, queueCache, owner, apiKeyID, requestID, keyLimit, policy, localDeadline, deadlineMs, revalidator)
}

// trackAPIKeyReservation returns the stats-only handle used by the existing
// unlimited path so every caller keeps one release shape. It exposes the exact
// member identity and stop-only pause so a Live transfer can consume it
// atomically without leaking the renewal worker.
func (s *ConcurrencyService) trackAPIKeyReservation(ctx context.Context, apiKeyID int64) *APIKeySlotReservation {
	requestID, release, pause := s.TrackAPIKeySlotOwned(ctx, apiKeyID)
	return &APIKeySlotReservation{
		apiKeyID:     apiKeyID,
		requestID:    requestID,
		limit:        0,
		release:      release,
		pauseRenewal: pause,
	}
}

// revalidateAPIKeyQueueAuth calls the middleware-installed callback with a
// bounded context and classifies its failure. A parent cancellation keeps its
// typed cause; a definite rejection keeps the original authentication error;
// anything else fails closed as unavailable.
func (s *ConcurrencyService) revalidateAPIKeyQueueAuth(ctx context.Context, revalidator APIKeyQueueAuthRevalidator) (int, error) {
	revalidateCtx, cancel := context.WithTimeout(ctx, apiKeySlotTrackTimeout)
	limit, err := revalidator(revalidateCtx)
	cancel()
	if ctx != nil && ctx.Err() != nil {
		return 0, apiKeyQueueContextCause(ctx)
	}
	if err != nil {
		var rejected *APIKeyQueueAuthRejectedError
		if errors.As(err, &rejected) {
			return 0, apiKeyQueueError(APIKeyQueueErrorAuthRejected, rejected.Err)
		}
		return 0, apiKeyQueueError(APIKeyQueueErrorUnavailable, err)
	}
	if limit < 0 {
		return 0, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("API key queue revalidation returned an invalid limit"))
	}
	return limit, nil
}

func (s *ConcurrencyService) reserveAPIKeySlotImmediate(ctx context.Context, cache APIKeySlotAdmissionCache, apiKeyID int64, maxConcurrency int) (*APIKeySlotReservation, error) {
	owner, ownerOK := apiKeyAdmissionOwnerFromContext(ctx)
	leaseCache, leaseOK := cache.(APIKeySlotLeaseCache)
	if !leaseOK || leaseCache.APIKeySlotTTL() <= 2*time.Second || leaseCache.APIKeySlotRefreshInterval() <= 0 || !ownerOK {
		return nil, fmt.Errorf("API key concurrency requires a renewable lease and cancellation owner")
	}
	if s.apiKeyQueueStopped() {
		return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("service is shutting down"))
	}
	requestID := generateRequestID()
	started := time.Now()
	acquireCtx, cancel := s.apiKeyQueueOperationContext(ctx, apiKeySlotTrackTimeout)
	acquired, err := cache.AcquireAPIKeySlot(acquireCtx, apiKeyID, maxConcurrency, requestID)
	cancel()
	if err != nil {
		releaseAPIKeySlot(cache, apiKeyID, requestID)
		return nil, fmt.Errorf("acquire API key %d concurrency slot: %w", apiKeyID, err)
	}
	if !acquired {
		return nil, nil
	}
	return s.adoptAPIKeyGrantOrRelease(ctx, leaseCache, owner, apiKeyID, requestID, maxConcurrency, started)
}

// adoptAPIKeyGrantOrRelease never hands a confirmed slot to forwarding after
// cancellation; it releases with the exact same member instead. limit is the
// effective limit used for the admission and is exposed for Live transfer.
func (s *ConcurrencyService) adoptAPIKeyGrantOrRelease(ctx context.Context, cache APIKeySlotLeaseCache, owner *apiKeyAdmissionOwner, apiKeyID int64, requestID string, limit int, started time.Time) (*APIKeySlotReservation, error) {
	release, pauseRenewal := keepEnforcedAPIKeySlotState(owner, cache, apiKeyID, requestID, started)
	if ctx != nil && ctx.Err() != nil {
		release()
		return nil, apiKeyQueueContextCause(ctx)
	}
	// A grant confirmed after shutdown starts must not be handed over, and a
	// lease whose safe validity already elapsed must not start forwarding: its
	// watchdog would cancel the owner immediately.
	if s.apiKeyQueueStopped() {
		release()
		return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("service is shutting down"))
	}
	if !apiKeyQueueSafeLeaseRemaining(cache, started, time.Now()) {
		release()
		return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, ErrAPIKeySlotLeaseLost)
	}
	return &APIKeySlotReservation{
		apiKeyID:     apiKeyID,
		requestID:    requestID,
		limit:        limit,
		release:      release,
		pauseRenewal: pauseRenewal,
	}, nil
}

// apiKeyQueueSafeLeaseRemaining mirrors the watchdog calculation: the lease is
// valid for TTL minus the Redis TIME rounding and transport margin measured
// from the START of the acknowledged operation.
func apiKeyQueueSafeLeaseRemaining(cache APIKeySlotLeaseCache, acknowledgedAt, now time.Time) bool {
	if cache == nil {
		return false
	}
	ttl := cache.APIKeySlotTTL()
	validFor := ttl - (ttl/3 + time.Second)
	if validFor <= 0 {
		return false
	}
	return now.Before(acknowledgedAt.Add(validFor))
}

// switchAPIKeyQueueToUnlimited closes the queue attempt before starting the
// stats-only path. A failed close is a service error: the two states must not
// run in parallel.
func (s *ConcurrencyService) switchAPIKeyQueueToUnlimited(ctx context.Context, cache APIKeySlotQueueCache, apiKeyID int64, requestID string, deadlineMs int64, removeRegularOnExit *bool) (*APIKeySlotReservation, error) {
	if err := s.abortAPIKeyQueueAttemptErr(cache, apiKeyID, requestID, deadlineMs, true); err != nil {
		return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, fmt.Errorf("close API key queue attempt before switching to unlimited: %w", err))
	}
	*removeRegularOnExit = false
	return s.trackAPIKeyReservation(ctx, apiKeyID), nil
}

func (s *ConcurrencyService) waitAPIKeyQueue(ctx context.Context, cache APIKeySlotQueueCache, owner *apiKeyAdmissionOwner, apiKeyID int64, requestID string, keyLimit int, policy APIKeyQueuePolicy, localDeadline time.Time, deadlineMs int64, revalidator APIKeyQueueAuthRevalidator) (*APIKeySlotReservation, error) {
	// Leaving the queue always fences late commands. On a confirmed ACQUIRED the
	// regular member is the new reservation, so it must not be removed here.
	removeRegularOnExit := true
	defer func() {
		s.abortAPIKeyQueueAttempt(cache, apiKeyID, requestID, deadlineMs, removeRegularOnExit)
	}()

	limit := keyLimit
	mode := APIKeyQueueModeEnter
	backoff := apiKeyQueueInitialBackoff
	for {
		if s.apiKeyQueueStopped() {
			return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("service is shutting down"))
		}
		if err := s.apiKeyQueueWaitError(ctx, localDeadline); err != nil {
			return nil, err
		}
		// Revalidate through the existing auth cache before every retry so a
		// queued request never forwards under a revoked authorization.
		if revalidator != nil {
			refreshed, err := s.revalidateAPIKeyQueueAuth(ctx, revalidator)
			if err != nil {
				return nil, err
			}
			if refreshed == 0 {
				return s.switchAPIKeyQueueToUnlimited(ctx, cache, apiKeyID, requestID, deadlineMs, &removeRegularOnExit)
			}
			limit = refreshed
		}
		opStarted := time.Now()
		opCtx, cancel := s.apiKeyQueueOperationContext(ctx, apiKeyQueueOperationTimeout(localDeadline))
		outcome, err := cache.AdvanceAPIKeyQueue(opCtx, apiKeyID, requestID, mode, deadlineMs, limit, policy.MaxWaiting)
		cancel()
		if err != nil {
			return nil, s.apiKeyQueueRedisFailure(ctx, err)
		}
		if s.apiKeyQueueStopped() {
			return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("service is shutting down"))
		}
		switch outcome {
		case APIKeyQueueOutcomeAcquired:
			// The waiting stage is closed; the regular member is the new
			// reservation and must survive the deferred fence.
			removeRegularOnExit = false
			if err := s.apiKeyQueueWaitError(ctx, localDeadline); err != nil {
				releaseAPIKeySlot(cache, apiKeyID, requestID)
				return nil, err
			}
			if revalidator != nil {
				// Final snapshot check before ownership is handed over: a
				// lease/watchdog must not start for a key that just changed.
				refreshed, err := s.revalidateAPIKeyQueueAuth(ctx, revalidator)
				if err != nil {
					releaseAPIKeySlot(cache, apiKeyID, requestID)
					return nil, err
				}
				if refreshed == 0 {
					releaseAPIKeySlot(cache, apiKeyID, requestID)
					return s.trackAPIKeyReservation(ctx, apiKeyID), nil
				}
				if err := s.apiKeyQueueWaitError(ctx, localDeadline); err != nil {
					releaseAPIKeySlot(cache, apiKeyID, requestID)
					return nil, err
				}
			}
			return s.adoptAPIKeyGrantOrRelease(ctx, cache, owner, apiKeyID, requestID, limit, opStarted)
		case APIKeyQueueOutcomeWaiting:
			mode = APIKeyQueueModePoll
		case APIKeyQueueOutcomeTimeout:
			return nil, apiKeyQueueError(APIKeyQueueErrorTimeout, nil)
		case APIKeyQueueOutcomeAborted:
			if ctx.Err() != nil {
				return nil, apiKeyQueueContextCause(ctx)
			}
			return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("queue attempt was already closed"))
		case APIKeyQueueOutcomeTicketLost:
			return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("queue ticket lost"))
		case APIKeyQueueOutcomePolicyChanged:
			// The Lua saw a non-positive limit. With revalidation available this
			// is the real policy-change path: re-read the auth snapshot and
			// either switch to the unlimited tracking path or retry with the
			// refreshed positive limit.
			if revalidator == nil {
				return nil, apiKeyQueueError(APIKeyQueueErrorPolicyChanged, nil)
			}
			refreshed, err := s.revalidateAPIKeyQueueAuth(ctx, revalidator)
			if err != nil {
				return nil, err
			}
			if refreshed == 0 {
				return s.switchAPIKeyQueueToUnlimited(ctx, cache, apiKeyID, requestID, deadlineMs, &removeRegularOnExit)
			}
			limit = refreshed
		case APIKeyQueueOutcomeQueueFull:
			return nil, apiKeyQueueError(APIKeyQueueErrorFull, nil)
		case APIKeyQueueOutcomeLimitReached:
			return nil, nil
		default:
			return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, fmt.Errorf("unexpected queue outcome %d", outcome))
		}

		wait := apiKeyQueueNextBackoff(backoff)
		backoff = wait
		if remaining := time.Until(localDeadline); remaining < wait {
			wait = remaining
		}
		if wait <= 0 {
			return nil, apiKeyQueueError(APIKeyQueueErrorTimeout, nil)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, apiKeyQueueContextCause(ctx)
		case <-s.apiKeyQueueStopChannel():
			timer.Stop()
			return nil, apiKeyQueueError(APIKeyQueueErrorUnavailable, errors.New("service is shutting down"))
		case <-timer.C:
		}
	}
}

func (s *ConcurrencyService) apiKeyQueueStopChannel() <-chan struct{} {
	if s == nil || s.apiKeyQueueStop == nil {
		return nil
	}
	return s.apiKeyQueueStop.ch
}

// apiKeyQueueContextCause returns the typed cancellation cause (e.g. a WS
// peer-gone or session-preempted error) instead of flattening it to
// context.Canceled. Callers resolve the final protocol error from it.
func apiKeyQueueContextCause(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return context.Canceled
}

// apiKeyQueueWaitError distinguishes the caller's own cancellation from the
// queue budget expiring. Queue timeout must not be reported as Redis failure.
func (s *ConcurrencyService) apiKeyQueueWaitError(ctx context.Context, localDeadline time.Time) error {
	if ctx != nil && ctx.Err() != nil {
		return apiKeyQueueContextCause(ctx)
	}
	if time.Now().After(localDeadline) {
		return apiKeyQueueError(APIKeyQueueErrorTimeout, nil)
	}
	return nil
}

func (s *ConcurrencyService) apiKeyQueueRedisFailure(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return apiKeyQueueContextCause(ctx)
	}
	return apiKeyQueueError(APIKeyQueueErrorUnavailable, err)
}

func apiKeyQueueOperationTimeout(localDeadline time.Time) time.Duration {
	timeout := apiKeySlotTrackTimeout
	if remaining := time.Until(localDeadline); remaining < timeout {
		timeout = remaining
	}
	if timeout <= 0 {
		timeout = time.Millisecond
	}
	return timeout
}

func apiKeyQueueNextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * apiKeyQueueBackoffFactor)
	if next > apiKeyQueueMaxBackoff {
		next = apiKeyQueueMaxBackoff
	}
	jitter := 0.8 + rand.Float64()*0.4
	jittered := time.Duration(float64(next) * jitter)
	if jittered < apiKeyQueueInitialBackoff {
		return apiKeyQueueInitialBackoff
	}
	if jittered > apiKeyQueueMaxBackoff {
		return apiKeyQueueMaxBackoff
	}
	return jittered
}

// abortAPIKeyQueueAttempt performs bounded, detached compensation with the same
// attempt ID. Failure is logged as a bounded residue, never silently claimed.
func (s *ConcurrencyService) abortAPIKeyQueueAttempt(cache APIKeySlotQueueCache, apiKeyID int64, requestID string, deadlineMs int64, removeSlot bool) {
	if err := s.abortAPIKeyQueueAttemptErr(cache, apiKeyID, requestID, deadlineMs, removeSlot); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: failed to abort api key queue attempt for %d (req=%s) after bounded retries: %v", apiKeyID, requestID, err)
	}
}

// abortAPIKeyQueueAttemptErr is the synchronous variant used before switching a
// queued attempt to the unlimited path: the caller must not start new work
// while the old ticket may still exist.
func (s *ConcurrencyService) abortAPIKeyQueueAttemptErr(cache APIKeySlotQueueCache, apiKeyID int64, requestID string, deadlineMs int64, removeSlot bool) error {
	if cache == nil || requestID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiKeyQueueAbortTimeout)
	defer cancel()
	var err error
	for attempt := 0; attempt < apiKeyQueueAbortAttempts; attempt++ {
		opCtx, stop := context.WithTimeout(ctx, time.Second)
		err = cache.AbortAPIKeyQueueAttempt(opCtx, apiKeyID, requestID, deadlineMs, removeSlot)
		stop()
		if err == nil {
			return nil
		}
		if attempt < apiKeyQueueAbortAttempts-1 {
			timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	return err
}
