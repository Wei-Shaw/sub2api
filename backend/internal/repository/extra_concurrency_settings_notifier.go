package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	extraConcurrencySettingsInvalidationChannel = "settings:extra_concurrency:invalidate"
	extraConcurrencyAdmissionDrainKey           = "settings:{extra_concurrency}:admission:drain"
	extraConcurrencyAdmissionEpochKey           = "settings:{extra_concurrency}:admission:epoch"
	extraConcurrencySettingsUpdateLockKey       = "settings:{extra_concurrency}:update:lock"
	// The drain only bridges the replica-local 10 second runtime cache window.
	// A finite TTL prevents a missed re-enable publish from wedging admission.
	extraConcurrencyAdmissionDrainTTL          = 15 * time.Second
	extraConcurrencySettingsUpdateLockTTL      = 5 * time.Second
	extraConcurrencySettingsUpdateLockRetry    = 25 * time.Millisecond
	extraConcurrencySettingsUpdateFinalTimeout = 2 * time.Second
)

var errExtraConcurrencySettingsUpdateLockLost = errors.New("extra concurrency settings update lock lost")

var renewExtraConcurrencySettingsUpdateLockScript = redis.NewScript(`
	if redis.call('GET', KEYS[1]) ~= ARGV[1] then
		return 0
	end
	return redis.call('PEXPIRE', KEYS[1], ARGV[2])
`)

var releaseExtraConcurrencySettingsUpdateLockScript = redis.NewScript(`
	if redis.call('GET', KEYS[1]) ~= ARGV[1] then
		return 0
	end
	return redis.call('DEL', KEYS[1])
`)

var finalizeExtraConcurrencySettingsUpdateScript = redis.NewScript(`
	if redis.call('GET', KEYS[1]) ~= ARGV[1] then
		return 0
	end
	local epoch = redis.call('INCR', KEYS[3])
	if ARGV[2] == '1' then
		redis.call('DEL', KEYS[2])
	else
		redis.call('SET', KEYS[2], epoch, 'PX', ARGV[4])
	end
	redis.call('PUBLISH', ARGV[3], tostring(epoch))
	redis.call('DEL', KEYS[1])
	return epoch
`)

type extraConcurrencySettingsNotifier struct {
	rdb                     *redis.Client
	drainTTL                time.Duration
	updateLockTTL           time.Duration
	updateLockRetryInterval time.Duration
}

func NewExtraConcurrencySettingsNotifier(rdb *redis.Client) service.ExtraConcurrencySettingsNotifier {
	return &extraConcurrencySettingsNotifier{
		rdb:                     rdb,
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           extraConcurrencySettingsUpdateLockTTL,
		updateLockRetryInterval: extraConcurrencySettingsUpdateLockRetry,
	}
}

func (n *extraConcurrencySettingsNotifier) PublishExtraConcurrencySettingsState(ctx context.Context, enabled bool) error {
	return n.serializeExtraConcurrencySettingsUpdate(ctx, enabled, nil, nil)
}

func (n *extraConcurrencySettingsNotifier) SerializeExtraConcurrencySettingsUpdate(
	ctx context.Context,
	enabled bool,
	reserveFence func(context.Context) (int64, error),
	update func(context.Context, int64) error,
) (err error) {
	if reserveFence == nil {
		return errors.New("serialize extra concurrency settings update: reserve fence callback is nil")
	}
	if update == nil {
		return errors.New("serialize extra concurrency settings update: update callback is nil")
	}
	return n.serializeExtraConcurrencySettingsUpdate(ctx, enabled, reserveFence, update)
}

func (n *extraConcurrencySettingsNotifier) serializeExtraConcurrencySettingsUpdate(
	ctx context.Context,
	enabled bool,
	reserveFence func(context.Context) (int64, error),
	update func(context.Context, int64) error,
) (err error) {
	if n == nil || n.rdb == nil {
		return errors.New("serialize extra concurrency settings update: redis client is unavailable")
	}

	lockTTL := n.updateLockTTL
	if lockTTL <= 0 {
		lockTTL = extraConcurrencySettingsUpdateLockTTL
	}
	retryInterval := n.updateLockRetryInterval
	if retryInterval <= 0 {
		retryInterval = extraConcurrencySettingsUpdateLockRetry
	}
	token := uuid.NewString()
	if err := n.acquireExtraConcurrencySettingsUpdateLock(ctx, token, lockTTL, retryInterval); err != nil {
		return err
	}

	updateCtx, cancelUpdate := context.WithCancel(ctx)
	stopRenewal := make(chan struct{})
	renewalResult := make(chan error, 1)
	go n.renewExtraConcurrencySettingsUpdateLock(cancelUpdate, token, lockTTL, stopRenewal, renewalResult)
	var stopRenewalOnce sync.Once
	var renewalErr error
	renewalErrReturned := false
	stopAndWaitForRenewal := func() error {
		stopRenewalOnce.Do(func() {
			close(stopRenewal)
			renewalErr = <-renewalResult
		})
		return renewalErr
	}
	lockReleased := false
	defer func() {
		cleanupRenewalErr := stopAndWaitForRenewal()
		if renewalErrReturned {
			cleanupRenewalErr = nil
		}
		cancelUpdate()
		var releaseErr error
		if !lockReleased {
			releaseErr = n.releaseExtraConcurrencySettingsUpdateLock(token)
		}
		err = errors.Join(err, cleanupRenewalErr, releaseErr)
	}()

	if reserveFence != nil {
		for {
			fence, reserveErr := reserveFence(updateCtx)
			if reserveErr != nil {
				return fmt.Errorf("reserve settings update fence: %w", reserveErr)
			}
			if fence <= 0 {
				return fmt.Errorf("reserve settings update fence: invalid fence %d", fence)
			}
			if verifyErr := n.verifyExtraConcurrencySettingsUpdateLock(updateCtx, token, lockTTL); verifyErr != nil {
				return verifyErr
			}
			updateErr := update(updateCtx, fence)
			if updateErr == nil {
				break
			}
			if !errors.Is(updateErr, service.ErrStaleSettingUpdateFence) {
				return updateErr
			}
			if verifyErr := n.verifyExtraConcurrencySettingsUpdateLock(updateCtx, token, lockTTL); verifyErr != nil {
				return errors.Join(updateErr, verifyErr)
			}
		}
	}

	renewalErr = stopAndWaitForRenewal()
	cancelUpdate()
	if renewalErr != nil {
		renewalErrReturned = true
		return renewalErr
	}
	if err := n.finalizeExtraConcurrencySettingsUpdate(token, enabled); err != nil {
		return err
	}
	lockReleased = true
	return nil
}

func (n *extraConcurrencySettingsNotifier) acquireExtraConcurrencySettingsUpdateLock(
	ctx context.Context,
	token string,
	lockTTL time.Duration,
	retryInterval time.Duration,
) error {
	for {
		acquired, err := n.rdb.SetNX(ctx, extraConcurrencySettingsUpdateLockKey, token, lockTTL).Result()
		if err != nil {
			return fmt.Errorf("acquire extra concurrency settings update lock: %w", err)
		}
		if acquired {
			return nil
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("acquire extra concurrency settings update lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func (n *extraConcurrencySettingsNotifier) verifyExtraConcurrencySettingsUpdateLock(
	ctx context.Context,
	token string,
	lockTTL time.Duration,
) error {
	owned, err := renewExtraConcurrencySettingsUpdateLockScript.Run(
		ctx,
		n.rdb,
		[]string{extraConcurrencySettingsUpdateLockKey},
		token,
		max(lockTTL.Milliseconds(), int64(1)),
	).Int64()
	if err != nil {
		return fmt.Errorf("verify extra concurrency settings update lock: %w", err)
	}
	if owned != 1 {
		return errExtraConcurrencySettingsUpdateLockLost
	}
	return nil
}

func (n *extraConcurrencySettingsNotifier) renewExtraConcurrencySettingsUpdateLock(
	cancelUpdate context.CancelFunc,
	token string,
	lockTTL time.Duration,
	stop <-chan struct{},
	result chan<- error,
) {
	interval := lockTTL / 3
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			result <- nil
			return
		case <-ticker.C:
			renewTimeout := interval
			if renewTimeout > time.Second {
				renewTimeout = time.Second
			}
			renewCtx, cancelRenew := context.WithTimeout(context.Background(), renewTimeout)
			renewed, err := renewExtraConcurrencySettingsUpdateLockScript.Run(
				renewCtx,
				n.rdb,
				[]string{extraConcurrencySettingsUpdateLockKey},
				token,
				max(lockTTL.Milliseconds(), int64(1)),
			).Int64()
			cancelRenew()
			if err != nil {
				cancelUpdate()
				result <- fmt.Errorf("renew extra concurrency settings update lock: %w", err)
				return
			}
			if renewed != 1 {
				cancelUpdate()
				result <- errExtraConcurrencySettingsUpdateLockLost
				return
			}
		}
	}
}

func (n *extraConcurrencySettingsNotifier) releaseExtraConcurrencySettingsUpdateLock(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), extraConcurrencySettingsUpdateFinalTimeout)
	defer cancel()
	released, err := releaseExtraConcurrencySettingsUpdateLockScript.Run(
		ctx,
		n.rdb,
		[]string{extraConcurrencySettingsUpdateLockKey},
		token,
	).Int64()
	if err != nil {
		return fmt.Errorf("release extra concurrency settings update lock: %w", err)
	}
	if released != 1 {
		return errExtraConcurrencySettingsUpdateLockLost
	}
	return nil
}

func (n *extraConcurrencySettingsNotifier) finalizeExtraConcurrencySettingsUpdate(token string, enabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), extraConcurrencySettingsUpdateFinalTimeout)
	defer cancel()
	enabledFlag := "0"
	if enabled {
		enabledFlag = "1"
	}
	drainTTL := n.drainTTL
	if drainTTL <= 0 {
		drainTTL = extraConcurrencyAdmissionDrainTTL
	}
	epoch, err := finalizeExtraConcurrencySettingsUpdateScript.Run(
		ctx,
		n.rdb,
		[]string{
			extraConcurrencySettingsUpdateLockKey,
			extraConcurrencyAdmissionDrainKey,
			extraConcurrencyAdmissionEpochKey,
		},
		token,
		enabledFlag,
		extraConcurrencySettingsInvalidationChannel,
		max(drainTTL.Milliseconds(), int64(1)),
	).Int64()
	if err != nil {
		return fmt.Errorf("finalize extra concurrency settings update: %w", err)
	}
	if epoch == 0 {
		return errExtraConcurrencySettingsUpdateLockLost
	}
	return nil
}

func (n *extraConcurrencySettingsNotifier) SubscribeExtraConcurrencySettingsInvalidation(ctx context.Context, handler func()) error {
	if n == nil || n.rdb == nil {
		return nil
	}
	pubsub := n.rdb.Subscribe(ctx, extraConcurrencySettingsInvalidationChannel)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return fmt.Errorf("subscribe to extra concurrency settings invalidation: %w", err)
	}

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("Warning: failed to close extra concurrency settings pubsub: %v", err)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-pubsub.Channel():
				if !ok {
					return
				}
				if handler != nil {
					handler()
				}
			}
		}
	}()
	return nil
}
