package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfluxRPCPoolFailsOverAndSkipsUnhealthyEndpoint(t *testing.T) {
	failing := &confluxRPCCallerStub{call: func(context.Context, any, string, ...any) error {
		return errors.New("unavailable")
	}}
	healthy := successfulConfluxRPCCaller("0x406")
	pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
		"https://rpc-1.example": failing,
		"https://rpc-2.example": healthy,
	}, []string{"https://rpc-1.example", "https://rpc-2.example"}, ConfluxRPCPoolOptions{
		RequestTimeout:  time.Second,
		FailureCooldown: time.Minute,
	})

	for range 3 {
		var chainID string
		require.NoError(t, pool.CallContext(context.Background(), &chainID, "eth_chainId"))
		require.Equal(t, "0x406", chainID)
	}

	require.Equal(t, int32(1), failing.calls.Load())
	require.Equal(t, int32(3), healthy.calls.Load())
	states := pool.EndpointStates()
	require.False(t, states[0].Healthy)
	require.True(t, states[1].Healthy)
}

func TestConfluxRPCPoolAppliesPerEndpointDeadline(t *testing.T) {
	slow := &confluxRPCCallerStub{call: func(ctx context.Context, _ any, _ string, _ ...any) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	healthy := successfulConfluxRPCCaller("0x406")
	pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
		"https://slow.example":    slow,
		"https://healthy.example": healthy,
	}, []string{"https://slow.example", "https://healthy.example"}, ConfluxRPCPoolOptions{
		RequestTimeout:  20 * time.Millisecond,
		FailureCooldown: time.Minute,
	})

	started := time.Now()
	var chainID string
	require.NoError(t, pool.CallContext(context.Background(), &chainID, "eth_chainId"))
	require.Equal(t, "0x406", chainID)
	require.GreaterOrEqual(t, time.Since(started), 20*time.Millisecond)
	require.Less(t, time.Since(started), 100*time.Millisecond)
}

func TestConfluxRPCPoolHonorsCallerCancellation(t *testing.T) {
	caller := successfulConfluxRPCCaller("0x406")
	pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
		"https://rpc.example": caller,
	}, []string{"https://rpc.example"}, ConfluxRPCPoolOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var chainID string
	err := pool.CallContext(ctx, &chainID, "eth_chainId")
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, caller.calls.Load())
}

func TestConfluxRPCPoolErrorsDoNotExposeEndpointURLs(t *testing.T) {
	secretURL := "https://rpc.example/provider/super-secret-token"
	caller := &confluxRPCCallerStub{call: func(context.Context, any, string, ...any) error {
		return errors.New("request failed for " + secretURL)
	}}
	pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
		secretURL: caller,
	}, []string{secretURL}, ConfluxRPCPoolOptions{})

	var result string
	err := pool.CallContext(context.Background(), &result, "eth_chainId")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrConfluxRPCUnavailable)
	require.NotContains(t, err.Error(), secretURL)
	require.NotContains(t, err.Error(), "super-secret-token")
	require.Contains(t, err.Error(), "endpoint_1")

	states := fmt.Sprint(pool.EndpointStates())
	require.False(t, strings.Contains(states, secretURL))
}

func TestConfluxRPCPoolFailsOverJSONRPCError(t *testing.T) {
	failing := &confluxRPCCallerStub{call: func(context.Context, any, string, ...any) error {
		return errors.New("json-rpc error -32000")
	}}
	healthy := successfulConfluxRPCCaller("0x406")
	pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
		"https://rpc-1.example": failing,
		"https://rpc-2.example": healthy,
	}, []string{"https://rpc-1.example", "https://rpc-2.example"}, ConfluxRPCPoolOptions{})

	var chainID string
	require.NoError(t, pool.CallContext(context.Background(), &chainID, "eth_chainId"))
	require.Equal(t, "0x406", chainID)
}

func TestNewConfluxRPCPoolRequiresEndpoints(t *testing.T) {
	pool, err := NewConfluxRPCPool(context.Background(), nil, ConfluxRPCPoolOptions{})
	require.Nil(t, pool)
	require.ErrorIs(t, err, ErrConfluxRPCUnavailable)
}

func TestNewConfluxRPCPoolIsolatesDialFailure(t *testing.T) {
	healthy := successfulConfluxRPCCaller("0x406")
	pool, err := NewConfluxRPCPool(context.Background(), []string{"https://bad.example", "https://good.example"}, ConfluxRPCPoolOptions{
		dial: func(_ context.Context, rawURL string, _ *http.Client) (confluxRPCCaller, error) {
			if rawURL == "https://bad.example" {
				return nil, errors.New("dial failed for secret endpoint")
			}
			return healthy, nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var chainID string
	require.NoError(t, pool.CallContext(context.Background(), &chainID, "eth_chainId"))
	require.Equal(t, "0x406", chainID)
	require.False(t, pool.EndpointStates()[0].Healthy)
	require.True(t, pool.EndpointStates()[1].Healthy)
}

func TestConfluxRPCRangeTooLargeClassification(t *testing.T) {
	require.True(t, isConfluxRPCRangeTooLargeError(confluxRPCErrorStub{code: -32005, message: "request rejected"}))
	require.True(t, isConfluxRPCRangeTooLargeError(errors.New("query returned more than 10000 results")))
	require.True(t, isConfluxRPCRangeTooLargeError(errors.New("block range is too wide")))
	require.False(t, isConfluxRPCRangeTooLargeError(confluxRPCErrorStub{code: -32000, message: "execution failed"}))
}

type confluxRPCCallerStub struct {
	calls  atomic.Int32
	call   func(context.Context, any, string, ...any) error
	closed atomic.Bool
}

type confluxRPCErrorStub struct {
	code    int
	message string
}

func (e confluxRPCErrorStub) Error() string  { return e.message }
func (e confluxRPCErrorStub) ErrorCode() int { return e.code }

func (s *confluxRPCCallerStub) CallContext(ctx context.Context, result any, method string, args ...any) error {
	s.calls.Add(1)
	return s.call(ctx, result, method, args...)
}

func (s *confluxRPCCallerStub) Close() {
	s.closed.Store(true)
}

func successfulConfluxRPCCaller(value string) *confluxRPCCallerStub {
	return &confluxRPCCallerStub{call: func(_ context.Context, result any, _ string, _ ...any) error {
		resultPointer, ok := result.(*string)
		if !ok {
			return errors.New("unexpected result type")
		}
		*resultPointer = value
		return nil
	}}
}

func newConfluxRPCPoolForTest(
	t *testing.T,
	callers map[string]confluxRPCCaller,
	rawURLs []string,
	options ConfluxRPCPoolOptions,
) *ConfluxRPCPool {
	t.Helper()
	options.HTTPClient = &http.Client{}
	options.dial = func(_ context.Context, rawURL string, _ *http.Client) (confluxRPCCaller, error) {
		caller, ok := callers[rawURL]
		if !ok {
			return nil, errors.New("missing caller")
		}
		return caller, nil
	}
	pool, err := NewConfluxRPCPool(context.Background(), rawURLs, options)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}
