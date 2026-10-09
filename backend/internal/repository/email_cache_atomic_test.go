package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newMiniredisEmailCache(t *testing.T) (service.EmailCache, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewEmailCache(rdb), mr, rdb
}

func TestEmailCache_ConcurrentWrongCodesCannotExceedAttemptCap(t *testing.T) {
	cache, _, _ := newMiniredisEmailCache(t)
	ctx := context.Background()
	email := "user@example.com"

	svc := service.NewEmailService(nil, cache)
	require.NoError(t, cache.SetVerificationCode(ctx, email, &service.VerificationCodeData{
		Code:      "123456",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}, 15*time.Minute))

	const workers = 50
	var invalid, maxed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.VerifyCode(ctx, email, "000000")
			switch {
			case errors.Is(err, service.ErrInvalidVerifyCode):
				invalid.Add(1)
			case errors.Is(err, service.ErrVerifyCodeMaxAttempts):
				maxed.Add(1)
			default:
				t.Errorf("unexpected result: %v", err)
			}
		}()
	}
	wg.Wait()

	// Only attempts 1..4 may return "invalid"; every other guess is rejected by the cap.
	require.LessOrEqual(t, int(invalid.Load()), 4)
	require.Equal(t, workers, int(invalid.Load()+maxed.Load()))

	// Even the correct code is now rejected.
	require.ErrorIs(t, svc.VerifyCode(ctx, email, "123456"), service.ErrVerifyCodeMaxAttempts)

	data, err := cache.GetVerificationCode(ctx, email)
	require.NoError(t, err)
	require.GreaterOrEqual(t, data.Attempts, 5)
}

func TestEmailCache_AttemptsResetOnNewCodeAndTTLFollowsCode(t *testing.T) {
	cache, mr, _ := newMiniredisEmailCache(t)
	ctx := context.Background()
	email := "User@Example.com"

	require.NoError(t, cache.SetVerificationCode(ctx, email, &service.VerificationCodeData{Code: "1"}, time.Minute))
	n, err := cache.IncrVerificationCodeAttempts(ctx, email)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Greater(t, mr.TTL(verifyCodeKey(email)+attemptsKeySuffix), time.Duration(0))

	require.NoError(t, cache.SetVerificationCode(ctx, email, &service.VerificationCodeData{Code: "2"}, time.Minute))
	data, err := cache.GetVerificationCode(ctx, email)
	require.NoError(t, err)
	require.Equal(t, 0, data.Attempts)

	require.NoError(t, cache.DeleteVerificationCode(ctx, email))
	_, err = cache.IncrVerificationCodeAttempts(ctx, email)
	require.Error(t, err)
	require.False(t, mr.Exists(verifyCodeKey(email)+attemptsKeySuffix))
}

func TestEmailCache_PasswordResetTokenHashedAndSingleUse(t *testing.T) {
	cache, mr, _ := newMiniredisEmailCache(t)
	ctx := context.Background()
	email := "reset@example.com"

	svc := service.NewEmailService(nil, cache)

	// Seed the token the same way SendPasswordResetEmail does (hash only).
	token, err := svc.GeneratePasswordResetToken()
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(token))
	require.NoError(t, cache.SetPasswordResetToken(ctx, email, &service.PasswordResetTokenData{
		Token: hex.EncodeToString(sum[:]), CreatedAt: time.Now(),
	}, 30*time.Minute))

	raw, err := mr.Get(passwordResetKey(email))
	require.NoError(t, err)
	require.False(t, strings.Contains(raw, token), "plaintext token must not be stored")

	require.NoError(t, svc.VerifyPasswordResetToken(ctx, email, token))
	require.ErrorIs(t, svc.ConsumePasswordResetToken(ctx, email, "wrong"), service.ErrInvalidResetToken)

	const workers = 30
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.ConsumePasswordResetToken(ctx, email, token) == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), ok.Load())
	require.False(t, mr.Exists(passwordResetKey(email)))
}

func TestEmailCache_ConsumePasswordResetTokenMismatchKeepsToken(t *testing.T) {
	cache, mr, _ := newMiniredisEmailCache(t)
	ctx := context.Background()
	email := "keep@example.com"
	require.NoError(t, cache.SetPasswordResetToken(ctx, email, &service.PasswordResetTokenData{Token: "abc"}, time.Minute))

	ok, err := cache.ConsumePasswordResetToken(ctx, email, "xyz")
	require.NoError(t, err)
	require.False(t, ok)
	require.True(t, mr.Exists(passwordResetKey(email)))

	ok, err = cache.ConsumePasswordResetToken(ctx, email, "abc")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.ConsumePasswordResetToken(ctx, email, "abc")
	require.NoError(t, err)
	require.False(t, ok)
}

// Upgrade compatibility: old nodes store Attempts only in the JSON document.
func TestEmailCacheLegacyAttemptCountAndRemainingTTL(t *testing.T) {
	for _, notify := range []bool{false, true} {
		name := "verification"
		if notify {
			name = "notification"
		}
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name                   string
				legacy, separate, want int
			}{
				{"fresh", 0, 0, 1}, {"four_legacy_failures", 4, 0, 5}, {"exhausted_legacy", 5, 0, 6},
				{"separate_counter_ahead", 4, 7, 8}, {"legacy_counter_ahead", 4, 2, 5},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cache, mr, rdb := newMiniredisEmailCache(t)
					ctx := context.Background()
					email := "legacy-count@example.com"
					key := verifyCodeKey(email)
					incr := cache.IncrVerificationCodeAttempts
					if notify {
						key = notifyVerifyKey(email)
						incr = cache.IncrNotifyVerifyCodeAttempts
					}
					raw, err := json.Marshal(service.VerificationCodeData{Code: "123456", Attempts: tc.legacy, ExpiresAt: time.Now().Add(time.Minute)})
					require.NoError(t, err)
					require.NoError(t, rdb.Set(ctx, key, raw, time.Minute).Err())
					if tc.separate > 0 {
						require.NoError(t, rdb.Set(ctx, key+attemptsKeySuffix, tc.separate, 2*time.Minute).Err())
					}
					mr.FastForward(23 * time.Second)
					remaining := mr.TTL(key)
					n, err := incr(ctx, email)
					require.NoError(t, err)
					require.Equal(t, tc.want, n)
					require.Equal(t, remaining, mr.TTL(key))
					require.Equal(t, remaining, mr.TTL(key+attemptsKeySuffix))
					stored, err := rdb.Get(ctx, key).Result()
					require.NoError(t, err)
					require.Equal(t, string(raw), stored)
					mr.FastForward(remaining)
					_, err = incr(ctx, email)
					require.Error(t, err)
					require.False(t, mr.Exists(key+attemptsKeySuffix))
				})
			}
		})
	}
}

func TestEmailCacheLegacyCodeStillStopsAtFifthWrongAttempt(t *testing.T) {
	for _, notify := range []bool{false, true} {
		name := "verification"
		if notify {
			name = "notification"
		}
		t.Run(name, func(t *testing.T) {
			cache, mr, rdb := newMiniredisEmailCache(t)
			ctx := context.Background()
			email := "legacy-flow@example.com"
			key := verifyCodeKey(email)
			if notify {
				key = notifyVerifyKey(email)
			}
			raw, err := json.Marshal(service.VerificationCodeData{Code: "123456", Attempts: 4, ExpiresAt: time.Now().Add(time.Minute)})
			require.NoError(t, err)
			require.NoError(t, rdb.Set(ctx, key, raw, time.Minute).Err())
			check := func(code string) error { return service.NewEmailService(nil, cache).VerifyCode(ctx, email, code) }
			if notify {
				check = func(code string) error {
					return (&service.UserService{}).VerifyAndAddNotifyEmail(ctx, 1, email, code, cache)
				}
			}
			require.ErrorIs(t, check("000000"), service.ErrVerifyCodeMaxAttempts)
			require.ErrorIs(t, check("123456"), service.ErrVerifyCodeMaxAttempts)
			require.True(t, mr.Exists(key))
		})
	}
}

func TestEmailCacheLegacyAttemptsConcurrentAndResend(t *testing.T) {
	cache, mr, rdb := newMiniredisEmailCache(t)
	ctx := context.Background()
	email := "concurrent-legacy@example.com"
	raw, err := json.Marshal(service.VerificationCodeData{Code: "123456", Attempts: 4, ExpiresAt: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	require.NoError(t, rdb.Set(ctx, notifyVerifyKey(email), raw, time.Minute).Err())
	const workers = 30
	var wg sync.WaitGroup
	counts := make(chan int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, e := cache.IncrNotifyVerifyCodeAttempts(ctx, email)
			if e != nil {
				t.Error(e)
				return
			}
			counts <- n
		}()
	}
	wg.Wait()
	close(counts)
	seen := map[int]bool{}
	for n := range counts {
		require.GreaterOrEqual(t, n, 5)
		require.LessOrEqual(t, n, 4+workers)
		require.False(t, seen[n])
		seen[n] = true
	}
	require.Len(t, seen, workers)
	require.Equal(t, mr.TTL(notifyVerifyKey(email)), mr.TTL(notifyVerifyKey(email)+attemptsKeySuffix))
	require.NoError(t, cache.SetNotifyVerifyCode(ctx, email, &service.VerificationCodeData{Code: "654321"}, time.Minute))
	n, err := cache.IncrNotifyVerifyCodeAttempts(ctx, email)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}
