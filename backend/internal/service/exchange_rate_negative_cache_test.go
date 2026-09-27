//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exchangeRateFailingTransport fails every request after a short delay.
type exchangeRateFailingTransport struct {
	calls atomic.Int64
}

func (tr *exchangeRateFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	tr.calls.Add(1)
	time.Sleep(50 * time.Millisecond)
	return nil, errors.New("provider unreachable")
}

func newFailingExchangeRateService(t *testing.T) (*ExchangeRateService, *exchangeRateFailingTransport) {
	t.Helper()
	stored, err := json.Marshal(&ExchangeRateSnapshot{
		RateText: "26000", FetchedAt: time.Now().Add(-2 * time.Hour), Source: "vietcombank",
	})
	require.NoError(t, err)
	svc := NewExchangeRateService(&paymentFulfillmentSettingRepoStub{
		values: map[string]string{SettingExchangeRateCache: string(stored)},
	})
	transport := &exchangeRateFailingTransport{}
	svc.httpClient.Transport = transport
	return svc, transport
}

func TestExchangeRateFailureIsSharedAndNegativeCached(t *testing.T) {
	svc, transport := newFailingExchangeRateService(t)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rate, _, err := svc.EffectiveRate(context.Background())
			// Every caller falls back to the stored snapshot (2h old, under max age).
			assert.NoError(t, err)
			assert.True(t, rate.Equal(decimal.RequireFromString("26000")), "got %s", rate)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, transport.calls.Load(), "concurrent callers must share one upstream fetch")

	// Within the backoff the provider is not asked again.
	_, _, err := svc.EffectiveRate(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, transport.calls.Load())

	// Once the backoff has passed, the next call retries the provider.
	svc.mu.Lock()
	svc.lastFetchFailure = time.Now().Add(-exchangeRateFailureBackoff)
	svc.mu.Unlock()
	_, _, err = svc.EffectiveRate(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 2, transport.calls.Load())
}

// exchangeRateOKTransport answers with a valid Vietcombank price list.
type exchangeRateOKTransport struct{}

func (exchangeRateOKTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	body := `<ExrateList><Exrate CurrencyCode="USD" Sell="26,260.00" /></ExrateList>`
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func TestExchangeRateRefreshIgnoresCallerCancellation(t *testing.T) {
	svc, _ := newFailingExchangeRateService(t)
	svc.httpClient.Transport = exchangeRateOKTransport{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The shared fetch must not inherit one caller's cancellation, otherwise a
	// disconnecting client would count as a provider failure for everyone.
	rate, _, err := svc.EffectiveRate(ctx)
	require.NoError(t, err)
	require.True(t, rate.Equal(decimal.RequireFromString("26260")), "got %s", rate)
}
