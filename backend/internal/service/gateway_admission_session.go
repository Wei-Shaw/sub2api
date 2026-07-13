package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	gatewayAdmissionPollInterval  = 20 * time.Millisecond
	gatewayAdmissionRenewInterval = 5 * time.Minute
	gatewayAdmissionRenewTimeout  = 2 * time.Second
)

type gatewayAdmissionLeaseTTLProvider interface {
	GatewayAdmissionLeaseTTL() time.Duration
}

type GatewayAdmission struct {
	store                 GatewayAdmissionStore
	gatewayService        *GatewayService
	capacitySource        AdmissionCapacitySource
	runtimeSettingsSource ExtraConcurrencyRuntimeSettingsSource
	renewInterval         time.Duration
}

type ExtraConcurrencyRuntimeSettingsSource interface {
	GetExtraConcurrencyRuntimeSettings(ctx context.Context) ExtraConcurrencyRuntimeSettings
}

type GatewayAdmissionRequest struct {
	UserID        int64
	StandardLimit int
	ExtraLimit    int
	Settings      ExtraConcurrencyRuntimeSettings
}

type GatewayAdmissionSession struct {
	admission        *GatewayAdmission
	store            GatewayAdmissionStore
	request          GatewayAdmissionRequest
	requestID        string
	userWaitDeadline time.Time
	waited           atomic.Bool
	standardOnly     atomic.Bool
	closeOnce        sync.Once

	mu            sync.Mutex
	class         AdmissionClass
	unlimited     bool
	closed        bool
	targetRelease func()
}

type GatewayTargetRequest struct {
	GroupID            *int64
	SessionKey         string
	Model              string
	MetadataUserID     string
	ExcludedAccountIDs map[int64]struct{}
	Selector           GatewayTargetSelector
}

type GatewayTargetSelector interface {
	Select(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error)
}

type GatewayTargetSelectorFunc func(context.Context, TargetClaimer) (*AccountSelectionResult, error)

func (f GatewayTargetSelectorFunc) Select(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
	return f(ctx, claimer)
}

type AdmittedTarget struct {
	Account    *Account
	Class      AdmissionClass
	session    *GatewayAdmissionSession
	request    GatewayTargetRequest
	settings   ExtraConcurrencyRuntimeSettings
	dispatched atomic.Bool
}

// GatewayTargetPreparation contains target-bound state created before the
// atomic dispatch boundary. Cleanup must be idempotent. Recheck indicates that
// preparation waited outside gateway admission and eligibility must be checked
// again. Handled indicates that the request completed locally and must not be
// sent upstream.
type GatewayTargetPreparation struct {
	Cleanup func()
	Recheck bool
	Handled bool
}

type GatewayTargetPrepareFunc func(context.Context, *Account) (GatewayTargetPreparation, error)

func NewGatewayAdmission(store GatewayAdmissionStore, gatewayService *GatewayService, capacitySource AdmissionCapacitySource) *GatewayAdmission {
	if capacitySource == nil && gatewayService != nil {
		capacitySource = gatewayService
	}
	return &GatewayAdmission{
		store:          store,
		gatewayService: gatewayService,
		capacitySource: capacitySource,
		renewInterval:  gatewayAdmissionRenewIntervalForStore(store),
	}
}

func (a *GatewayAdmission) SetExtraConcurrencyRuntimeSettingsSource(source ExtraConcurrencyRuntimeSettingsSource) {
	if a == nil {
		return
	}
	a.runtimeSettingsSource = source
}

func gatewayAdmissionRenewIntervalForStore(store GatewayAdmissionStore) time.Duration {
	provider, ok := store.(gatewayAdmissionLeaseTTLProvider)
	if !ok {
		return gatewayAdmissionRenewInterval
	}
	interval := provider.GatewayAdmissionLeaseTTL() / 3
	if interval <= 0 {
		return gatewayAdmissionRenewInterval
	}
	return interval
}

func (a *GatewayAdmission) Begin(ctx context.Context, request GatewayAdmissionRequest) (*GatewayAdmissionSession, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("gateway admission is unavailable")
	}
	if request.UserID <= 0 {
		return nil, fmt.Errorf("invalid gateway admission user")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	requestID := generateRequestID()
	waitTimeout := time.Duration(request.Settings.WaitTimeoutSeconds) * time.Second
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	userWaitDeadline := time.Now().Add(waitTimeout)
	waitCtx, cancel := context.WithDeadline(ctx, userWaitDeadline)
	defer cancel()

	waited := false
	for {
		result, err := a.store.TryAcquireUserLease(waitCtx, UserLeaseRequest{
			RequestID:     requestID,
			UserID:        request.UserID,
			StandardLimit: request.StandardLimit,
			ExtraLimit:    request.ExtraLimit,
			MaxWaiting:    gatewayAdmissionMaxWaiting(request),
			WaitTimeout:   waitTimeout,
		})
		if err != nil {
			a.releaseUserState(request.UserID, requestID)
			return nil, err
		}
		if result.Draining {
			a.releaseUserState(request.UserID, requestID)
			return nil, ErrGatewayAdmissionDraining
		}
		if result.Expired {
			a.releaseUserState(request.UserID, requestID)
			class := AdmissionClassStandard
			if request.ExtraLimit > 0 {
				class = AdmissionClassExtra
			}
			return nil, gatewayAdmissionWaitTimeoutError(class, "user")
		}
		if result.QueueFull {
			a.releaseUserState(request.UserID, requestID)
			return nil, &GatewayAdmissionQueueFullError{}
		}
		if result.Acquired {
			session := &GatewayAdmissionSession{
				admission:        a,
				store:            a.store,
				request:          request,
				requestID:        requestID,
				userWaitDeadline: userWaitDeadline,
				class:            result.Class,
				unlimited:        result.Unlimited,
			}
			session.waited.Store(waited)
			context.AfterFunc(ctx, session.Close)
			return session, nil
		}

		waited = true
		timer := time.NewTimer(gatewayAdmissionPollInterval)
		select {
		case <-waitCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			a.releaseUserState(request.UserID, requestID)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			class := AdmissionClassStandard
			if request.ExtraLimit > 0 {
				class = AdmissionClassExtra
			}
			return nil, gatewayAdmissionWaitTimeoutError(class, "user")
		case <-timer.C:
		}
	}
}

func (a *GatewayAdmission) releaseUserState(userID int64, requestID string) {
	if a == nil || a.store == nil {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.store.ReleaseUserLease(releaseCtx, userID, requestID)
}

func (s *GatewayAdmissionSession) Class() AdmissionClass {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.class
}

func (s *GatewayAdmissionSession) Waited() bool {
	return s != nil && s.waited.Load()
}

func (s *GatewayAdmissionSession) NextTarget(ctx context.Context, request GatewayTargetRequest) (*AdmittedTarget, error) {
	return s.nextTarget(ctx, request, s.Waited())
}

func (s *GatewayAdmissionSession) nextTarget(
	ctx context.Context,
	request GatewayTargetRequest,
	refreshUser bool,
) (*AdmittedTarget, error) {
	if s == nil || s.admission == nil || (request.Selector == nil && s.admission.gatewayService == nil) {
		return nil, fmt.Errorf("gateway admission target selection is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	initialSettings := s.currentExtraConcurrencyRuntimeSettings(ctx)
	waitTimeout := time.Duration(initialSettings.WaitTimeoutSeconds) * time.Second
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	s.ReleaseTarget()
	claimer := &gatewayAdmissionTargetClaimer{
		store:          s.store,
		capacitySource: s.admission.capacitySource,
		requestID:      s.requestID,
		settings:       initialSettings,
	}
	targetAcquired := false
	defer func() {
		if !targetAcquired {
			claimer.ReleasePending()
		}
	}()
	class := s.Class()

	for {
		if s.isClosed() {
			return nil, context.Canceled
		}
		var err error
		var settings ExtraConcurrencyRuntimeSettings
		class, settings, err = s.fallbackDisabledExtraToStandard(waitCtx, class, nil)
		if err != nil {
			return nil, err
		}
		if refreshUser {
			class, err = s.refreshUserLease(waitCtx)
			if err != nil {
				if waitCtx.Err() != nil {
					return nil, gatewayAdmissionTargetWaitError(ctx, class)
				}
				return nil, err
			}
		}
		claimer.class = class
		claimer.settings = settings
		var selection *AccountSelectionResult
		if request.Selector != nil {
			selection, err = request.Selector.Select(waitCtx, claimer)
		} else {
			selection, err = s.admission.gatewayService.selectAccountWithTargetClaimer(
				waitCtx,
				request.GroupID,
				request.SessionKey,
				request.Model,
				request.ExcludedAccountIDs,
				request.MetadataUserID,
				s.request.UserID,
				claimer,
			)
		}
		if err != nil && waitCtx.Err() != nil {
			return nil, gatewayAdmissionTargetWaitError(ctx, class)
		}
		if claimErr := claimer.Err(); claimErr != nil {
			if waitCtx.Err() != nil {
				return nil, gatewayAdmissionTargetWaitError(ctx, class)
			}
			if errors.Is(claimErr, ErrGatewayAdmissionDraining) && class == AdmissionClassExtra {
				claimer.ReleasePending()
				class, settings, err = s.fallbackExtraToStandard(waitCtx, class, nil, true)
				if err != nil {
					return nil, err
				}
				claimer = &gatewayAdmissionTargetClaimer{
					store:          s.store,
					capacitySource: s.admission.capacitySource,
					requestID:      s.requestID,
					settings:       settings,
				}
				refreshUser = false
				continue
			}
			return nil, claimErr
		}
		if err != nil {
			return nil, err
		}
		if selection == nil || selection.Account == nil {
			return nil, fmt.Errorf("gateway admission selected no target")
		}
		if selection.Acquired {
			targetAcquired = true
			if !s.setTargetRelease(selection.ReleaseFunc) {
				return nil, context.Canceled
			}
			return &AdmittedTarget{
				Account:  selection.Account,
				Class:    class,
				session:  s,
				request:  request,
				settings: settings,
			}, nil
		}

		s.waited.Store(true)
		timer := time.NewTimer(gatewayAdmissionPollInterval)
		select {
		case <-waitCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, gatewayAdmissionTargetWaitError(ctx, class)
		case <-timer.C:
			refreshUser = true
		}
	}
}

func (s *GatewayAdmissionSession) currentExtraConcurrencyRuntimeSettings(ctx context.Context) ExtraConcurrencyRuntimeSettings {
	if s == nil {
		return ExtraConcurrencyRuntimeSettings{}
	}
	settings := s.request.Settings
	if s.admission != nil && s.admission.runtimeSettingsSource != nil {
		settings = s.admission.runtimeSettingsSource.GetExtraConcurrencyRuntimeSettings(ctx)
	}
	return settings
}

func (s *GatewayAdmissionSession) fallbackDisabledExtraToStandard(
	ctx context.Context,
	class AdmissionClass,
	beforeWait func(),
) (AdmissionClass, ExtraConcurrencyRuntimeSettings, error) {
	return s.fallbackExtraToStandard(ctx, class, beforeWait, false)
}

func (s *GatewayAdmissionSession) fallbackExtraToStandard(
	ctx context.Context,
	class AdmissionClass,
	beforeWait func(),
	force bool,
) (AdmissionClass, ExtraConcurrencyRuntimeSettings, error) {
	settings := s.currentExtraConcurrencyRuntimeSettings(ctx)
	if class != AdmissionClassExtra || s == nil || s.admission == nil {
		return class, settings, nil
	}
	if !force && (s.admission.runtimeSettingsSource == nil || settings.Enabled) {
		return class, settings, nil
	}
	s.standardOnly.Store(true)
	if beforeWait != nil {
		beforeWait()
	}

	waitCtx := ctx
	cancel := func() {}
	if !s.userWaitDeadline.IsZero() {
		waitCtx, cancel = context.WithDeadline(ctx, s.userWaitDeadline)
	}
	defer cancel()

	request := UserLeaseRequest{
		RequestID:     s.requestID,
		UserID:        s.request.UserID,
		StandardLimit: s.request.StandardLimit,
		ExtraLimit:    0,
		MaxWaiting:    gatewayAdmissionMaxWaiting(GatewayAdmissionRequest{StandardLimit: s.request.StandardLimit}),
		WaitTimeout:   time.Until(s.userWaitDeadline),
	}
	for {
		result, err := s.store.TryAcquireUserLease(waitCtx, request)
		if err != nil {
			return class, settings, err
		}
		if result.QueueFull {
			return class, settings, &GatewayAdmissionQueueFullError{}
		}
		if result.Expired {
			return class, settings, gatewayAdmissionWaitTimeoutError(AdmissionClassStandard, "user")
		}
		if result.Acquired {
			if result.Class != AdmissionClassStandard {
				return class, settings, fmt.Errorf("gateway admission extra fallback did not acquire standard concurrency")
			}
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				s.admission.releaseUserState(s.request.UserID, s.requestID)
				return class, settings, context.Canceled
			}
			s.class = AdmissionClassStandard
			s.unlimited = result.Unlimited
			s.mu.Unlock()
			s.waited.Store(true)
			return AdmissionClassStandard, settings, nil
		}

		timer := time.NewTimer(gatewayAdmissionPollInterval)
		select {
		case <-waitCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if err := ctx.Err(); err != nil {
				return class, settings, err
			}
			return class, settings, gatewayAdmissionWaitTimeoutError(AdmissionClassStandard, "user")
		case <-timer.C:
		}
	}
}

func extraConcurrencyAdmissionPolicyEqual(a, b ExtraConcurrencyRuntimeSettings) bool {
	if a.Enabled != b.Enabled ||
		a.WaitTimeoutSeconds != b.WaitTimeoutSeconds ||
		a.ReservePercent != b.ReservePercent ||
		a.MinReservedSlots != b.MinReservedSlots ||
		len(a.PlatformReserves) != len(b.PlatformReserves) {
		return false
	}
	for platform, left := range a.PlatformReserves {
		right, ok := b.PlatformReserves[platform]
		if !ok ||
			!optionalFloat64Equal(left.ReservePercent, right.ReservePercent) ||
			!optionalIntEqual(left.MinReservedSlots, right.MinReservedSlots) {
			return false
		}
	}
	return true
}

func optionalFloat64Equal(a, b *float64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func optionalIntEqual(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func gatewayAdmissionTargetWaitError(parent context.Context, class AdmissionClass) error {
	if parent != nil {
		if err := parent.Err(); err != nil {
			return err
		}
	}
	return gatewayAdmissionWaitTimeoutError(class, "account")
}

func (t *AdmittedTarget) Dispatch(
	ctx context.Context,
	recheck func(context.Context) error,
	upstream func(context.Context, *Account) error,
) error {
	_, err := t.DispatchPrepared(ctx, nil, recheck, upstream)
	return err
}

// DispatchPrepared prepares target-bound handler state before the atomic
// dispatch boundary. If drain or policy changes retarget the request, the old
// preparation is cleaned and the replacement target is prepared again.
func (t *AdmittedTarget) DispatchPrepared(
	ctx context.Context,
	prepare GatewayTargetPrepareFunc,
	recheck func(context.Context) error,
	upstream func(context.Context, *Account) error,
) (bool, error) {
	if t == nil || t.Account == nil || t.session == nil {
		return false, fmt.Errorf("gateway admission target is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer t.session.ReleaseTarget()
	finish, handled, err := t.beginAttempt(ctx, prepare, recheck)
	if err != nil {
		return false, err
	}
	if handled {
		return true, nil
	}
	defer finish()

	if upstream == nil {
		return false, fmt.Errorf("gateway admission upstream dispatch is unavailable")
	}
	return false, upstream(ctx, t.Account)
}

// BeginAttempt starts one target-bound upstream attempt. If an undispatched
// extra request must fall back after emergency disable, it releases and
// reacquires the target under standard admission before the attempt starts.
// The returned function stops lease renewal and is idempotent.
func (t *AdmittedTarget) BeginAttempt(
	ctx context.Context,
	recheck func(context.Context) error,
) (func(), error) {
	finish, _, err := t.beginAttempt(ctx, nil, recheck)
	return finish, err
}

func (t *AdmittedTarget) beginAttempt(
	ctx context.Context,
	prepare GatewayTargetPrepareFunc,
	recheck func(context.Context) error,
) (func(), bool, error) {
	if t == nil || t.Account == nil || t.session == nil {
		return nil, false, fmt.Errorf("gateway admission target is unavailable")
	}
	if !t.dispatched.CompareAndSwap(false, true) {
		return nil, false, fmt.Errorf("gateway admission target was already dispatched")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	class := t.Class
	cleanup := func() {}
	cleanupOwned := false
	defer func() {
		if cleanupOwned {
			cleanup()
		}
	}()
	releaseCleanup := func() {
		if !cleanupOwned {
			return
		}
		cleanup()
		cleanupOwned = false
	}
	recheckRequired := false
	for {
		preparation := GatewayTargetPreparation{}
		if prepare != nil {
			var err error
			preparation, err = prepare(ctx, t.Account)
			cleanup = idempotentGatewayTargetCleanup(preparation.Cleanup)
			cleanupOwned = true
			if err != nil {
				releaseCleanup()
				return nil, false, err
			}
			if preparation.Handled {
				releaseCleanup()
				return nil, true, nil
			}
			recheckRequired = recheckRequired || preparation.Recheck
		} else {
			cleanup = func() {}
			cleanupOwned = true
		}
		if (t.session.Waited() || recheckRequired) && recheck != nil {
			if err := recheck(ctx); err != nil {
				releaseCleanup()
				return nil, false, err
			}
		}

		forceStandard := false
		dispatch, err := t.session.store.BeginTargetDispatch(ctx, TargetDispatchRequest{
			RequestID: t.session.requestID,
			Platform:  t.Account.Platform,
			AccountID: t.Account.ID,
			Class:     class,
			Unlimited: t.Account.Concurrency <= 0,
		})
		if err != nil {
			releaseCleanup()
			return nil, false, err
		}
		if !dispatch.Started && !dispatch.Draining {
			releaseCleanup()
			return nil, false, fmt.Errorf("gateway admission target lease was lost before dispatch")
		}
		forceStandard = class == AdmissionClassExtra && dispatch.Draining

		retarget := false
		latestClass, latestSettings, err := t.session.fallbackExtraToStandard(ctx, class, func() {
			releaseCleanup()
			t.session.ReleaseTarget()
			retarget = true
		}, forceStandard)
		if err != nil {
			releaseCleanup()
			return nil, false, err
		}
		class = latestClass
		if !retarget && class == AdmissionClassExtra && !extraConcurrencyAdmissionPolicyEqual(t.settings, latestSettings) {
			releaseCleanup()
			t.session.ReleaseTarget()
			retarget = true
		}
		if !retarget {
			t.Class = class
			break
		}

		replacement, err := t.session.nextTarget(ctx, t.request, false)
		if err != nil {
			return nil, false, err
		}
		t.Account = replacement.Account
		class = replacement.Class
		t.Class = class
		t.settings = replacement.settings
	}

	stopRenewal := t.session.startRenewal(ctx, t.Account)
	attemptCleanup := cleanup
	cleanupOwned = false
	var once sync.Once
	return func() {
		once.Do(func() {
			stopRenewal()
			attemptCleanup()
		})
	}, false, nil
}

func idempotentGatewayTargetCleanup(cleanup func()) func() {
	if cleanup == nil {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(cleanup)
	}
}

func (s *GatewayAdmissionSession) startRenewal(ctx context.Context, account *Account) func() {
	if s == nil || s.admission == nil || s.store == nil || account == nil || s.admission.renewInterval <= 0 {
		return func() {}
	}
	renewCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.admission.renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				s.renewHeldLeases(renewCtx, account)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func (s *GatewayAdmissionSession) renewHeldLeases(ctx context.Context, account *Account) {
	renewCtx, cancel := context.WithTimeout(ctx, gatewayAdmissionRenewTimeout)
	defer cancel()

	s.mu.Lock()
	class := s.class
	unlimited := s.unlimited
	s.mu.Unlock()
	if !unlimited {
		_, _ = s.store.RenewUserLease(renewCtx, s.request.UserID, s.requestID, class)
	}
	_, _ = s.store.RenewTargetLease(renewCtx, account.Platform, account.ID, s.requestID)
}

func (s *GatewayAdmissionSession) refreshUserLease(ctx context.Context) (AdmissionClass, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", context.Canceled
	}
	class := s.class
	if s.unlimited {
		s.mu.Unlock()
		return class, nil
	}
	s.mu.Unlock()

	extraLimit := s.request.ExtraLimit
	if s.standardOnly.Load() {
		extraLimit = 0
	}
	result, err := s.store.TryAcquireUserLease(ctx, UserLeaseRequest{
		RequestID:     s.requestID,
		UserID:        s.request.UserID,
		StandardLimit: s.request.StandardLimit,
		ExtraLimit:    extraLimit,
		MaxWaiting: gatewayAdmissionMaxWaiting(GatewayAdmissionRequest{
			StandardLimit: s.request.StandardLimit,
			ExtraLimit:    extraLimit,
		}),
		WaitTimeout: time.Duration(s.request.Settings.WaitTimeoutSeconds) * time.Second,
	})
	if err != nil {
		return "", err
	}
	if result.Expired {
		return "", gatewayAdmissionWaitTimeoutError(class, "user")
	}
	if !result.Acquired {
		if class == AdmissionClassExtra {
			fallbackClass, _, fallbackErr := s.fallbackExtraToStandard(ctx, class, nil, true)
			return fallbackClass, fallbackErr
		}
		return "", fmt.Errorf("gateway admission user lease was lost")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.admission.releaseUserState(s.request.UserID, s.requestID)
		return "", context.Canceled
	}
	s.class = result.Class
	s.unlimited = result.Unlimited
	class = s.class
	s.mu.Unlock()
	return class, nil
}

func gatewayAdmissionMaxWaiting(request GatewayAdmissionRequest) int {
	totalLimit := max(request.StandardLimit, 0) + max(request.ExtraLimit, 0)
	if totalLimit <= 0 {
		return 0
	}
	return max(CalculateMaxWait(totalLimit)-totalLimit, 1)
}

func (s *GatewayAdmissionSession) isClosed() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *GatewayAdmissionSession) setTargetRelease(release func()) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		if release != nil {
			release()
		}
		return false
	}
	s.targetRelease = release
	s.mu.Unlock()
	return true
}

func (s *GatewayAdmissionSession) ReleaseTarget() {
	if s == nil {
		return
	}
	s.mu.Lock()
	release := s.targetRelease
	s.targetRelease = nil
	s.mu.Unlock()
	if release != nil {
		release()
	}
}

func (s *GatewayAdmissionSession) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		release := s.targetRelease
		s.targetRelease = nil
		unlimited := s.unlimited
		s.mu.Unlock()
		if release != nil {
			release()
		}
		if unlimited || s.store == nil {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.store.ReleaseUserLease(releaseCtx, s.request.UserID, s.requestID)
	})
}
