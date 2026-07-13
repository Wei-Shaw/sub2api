package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayAdmissionBeginOwnsUserLeaseUntilClose(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq: []bool{true},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 77})
	streamStarted := false

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
		Stream:         false,
		StreamStarted:  &streamStarted,
	})
	require.NoError(t, err)
	require.NotNil(t, admission)
	require.Equal(t, 1, cache.userAcquireCalls)
	require.Equal(t, 1, cache.apiKeyTrackCalls)

	admission.Close()
	admission.Close()

	require.Equal(t, 1, cache.userReleaseCalls)
	require.Equal(t, 1, cache.apiKeyReleaseCalls)
}

func TestGatewayAdmissionAdmitAccountWaitsAndOwnsLease(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:    []bool{true},
		accountSeq: []bool{true},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	streamStarted := false

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
		StreamStarted:  &streamStarted,
	})
	require.NoError(t, err)

	err = admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account:  &service.Account{ID: 303},
			Acquired: false,
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 2,
				Timeout:        time.Second,
				MaxWaiting:     9,
			},
		},
		WaitPolicy: AccountWaitPolicyTracked,
	})
	require.NoError(t, err)
	require.Equal(t, 1, cache.accountWaitIncrementCalls)
	require.Equal(t, 9, cache.accountWaitMaxWait)
	require.Equal(t, 1, cache.accountAcquireCalls)
	require.Equal(t, 1, cache.accountWaitDecrementCalls)

	admission.Close()

	require.Equal(t, 1, cache.accountReleaseCalls)
	require.Equal(t, 1, cache.userReleaseCalls)
}

func TestGatewayAdmissionAdmitAccountCanPreserveUntrackedWait(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:    []bool{true},
		accountSeq: []bool{true},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/responses")

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
	})
	require.NoError(t, err)
	defer admission.Close()

	err = admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account:  &service.Account{ID: 303},
			Acquired: false,
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 2,
				Timeout:        time.Second,
				MaxWaiting:     9,
			},
		},
		WaitPolicy: AccountWaitPolicyUntracked,
	})
	require.NoError(t, err)
	require.Equal(t, 0, cache.accountWaitIncrementCalls)
	require.Equal(t, 0, cache.accountWaitDecrementCalls)
}

func TestGatewayAdmissionFailFastNeverEntersWaitQueues(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:    []bool{true},
		accountSeq: []bool{false},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodGet, "/v1/responses")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 77})

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeFailFast,
	})
	require.NoError(t, err)
	defer admission.Close()

	err = admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account: &service.Account{ID: 303},
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 2,
				Timeout:        time.Second,
				MaxWaiting:     9,
			},
		},
		WaitPolicy: AccountWaitPolicyRetryThenTracked,
	})
	var unavailableErr *AccountUnavailableError
	require.ErrorAs(t, err, &unavailableErr)
	require.Equal(t, 0, cache.waitIncrementCalls)
	require.Equal(t, 0, cache.accountWaitIncrementCalls)
}

func TestGatewayAdmissionRetryThenTrackedAcquiresBeforeQueueing(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:    []bool{true},
		accountSeq: []bool{true},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/responses")

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
	})
	require.NoError(t, err)
	defer admission.Close()

	err = admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account: &service.Account{ID: 303},
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 2,
				Timeout:        time.Second,
				MaxWaiting:     9,
			},
		},
		WaitPolicy: AccountWaitPolicyRetryThenTracked,
	})
	require.NoError(t, err)
	require.Equal(t, 1, cache.accountAcquireCalls)
	require.Equal(t, 0, cache.accountWaitIncrementCalls)
}

func TestGatewayAdmissionReleasesOnlyAccountAcrossFailover(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:    []bool{true},
		accountSeq: []bool{true, true},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
	})
	require.NoError(t, err)

	admit := func(accountID int64) {
		t.Helper()
		err := admission.AdmitAccount(AccountAdmissionRequest{
			Selection: &service.AccountSelectionResult{
				Account: &service.Account{ID: accountID},
				WaitPlan: &service.AccountWaitPlan{
					AccountID:      accountID,
					MaxConcurrency: 1,
					Timeout:        time.Second,
				},
			},
			WaitPolicy: AccountWaitPolicyUntracked,
		})
		require.NoError(t, err)
	}

	admit(301)
	admission.ReleaseAccount()
	require.Equal(t, 1, cache.accountReleaseCalls)
	require.Equal(t, 0, cache.userReleaseCalls)

	admit(302)
	admission.Close()
	require.Equal(t, 2, cache.accountReleaseCalls)
	require.Equal(t, 1, cache.userReleaseCalls)
}

func TestGatewayAdmissionContextCancellationReleasesEveryLeaseOnce(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:    []bool{true},
		accountSeq: []bool{true},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	requestCtx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestCtx)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 77})

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
	})
	require.NoError(t, err)
	require.NoError(t, admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account: &service.Account{ID: 303},
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 1,
				Timeout:        time.Second,
			},
		},
		WaitPolicy: AccountWaitPolicyUntracked,
	}))

	cancel()
	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.userReleaseCalls == 1 && cache.accountReleaseCalls == 1 && cache.apiKeyReleaseCalls == 1
	}, time.Second, 10*time.Millisecond)

	admission.Close()
	require.Equal(t, 1, cache.userReleaseCalls)
	require.Equal(t, 1, cache.accountReleaseCalls)
	require.Equal(t, 1, cache.apiKeyReleaseCalls)
}

func TestGatewayAdmissionAccountQueueFullPreservesTypedError(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:           []bool{true},
		accountWaitDenied: true,
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
	})
	require.NoError(t, err)
	defer admission.Close()

	err = admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account: &service.Account{ID: 303},
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 1,
				Timeout:        time.Second,
				MaxWaiting:     9,
			},
		},
		WaitPolicy: AccountWaitPolicyTracked,
	})
	var queueFullErr *WaitQueueFullError
	require.ErrorAs(t, err, &queueFullErr)
	require.Equal(t, "account", queueFullErr.SlotType)
	require.Equal(t, 1, cache.accountWaitIncrementCalls)
	require.Equal(t, 0, cache.accountWaitDecrementCalls)
	require.Equal(t, 0, cache.accountAcquireCalls)
}

func TestGatewayAdmissionTrackedWaitCounterStorageFailureStillAttemptsAcquisition(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		userSeq:                 []bool{true},
		accountSeq:              []bool{true},
		accountWaitIncrementErr: context.DeadlineExceeded,
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")

	admission, err := helper.Begin(c, UserAdmissionRequest{
		UserID:         202,
		MaxConcurrency: 3,
		Mode:           AdmissionModeWait,
	})
	require.NoError(t, err)
	defer admission.Close()

	err = admission.AdmitAccount(AccountAdmissionRequest{
		Selection: &service.AccountSelectionResult{
			Account: &service.Account{ID: 303},
			WaitPlan: &service.AccountWaitPlan{
				AccountID:      303,
				MaxConcurrency: 1,
				Timeout:        time.Second,
				MaxWaiting:     9,
			},
		},
		WaitPolicy: AccountWaitPolicyTracked,
	})

	require.NoError(t, err)
	require.Equal(t, 1, cache.accountWaitIncrementCalls)
	require.Equal(t, 1, cache.accountAcquireCalls)
	require.Equal(t, 1, cache.accountWaitDecrementCalls)
}

func TestWrapReleaseOnDoneAlreadyCanceledContextReleasesOnce(t *testing.T) {
	for range 1000 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		released := make(chan struct{})
		release := wrapReleaseOnDone(ctx, func() {
			close(released)
		})

		release()
		<-released
		release()
	}
}
