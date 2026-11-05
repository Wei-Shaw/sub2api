package httputil

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/require"
)

// Keep docs/TRAFFIC_STATS.md's pre-normalization measurement contract while
// adding Brotli to the current branch's bounded ingress reader.
func TestBrotliRequestTrafficPreservesIngressMemoryBudget(t *testing.T) {
	payload := append([]byte(`{"input":"`), bytes.Repeat([]byte{'x'}, 4096)...)
	payload = append(payload, 0)
	payload = append(payload, []byte(`"}`)...)
	var encoded bytes.Buffer
	encoder := brotli.NewWriter(&encoded)
	_, err := encoder.Write(payload)
	require.NoError(t, err)
	require.NoError(t, encoder.Close())

	for _, tc := range []struct {
		name   string
		budget int64
		fail   bool
	}{
		{name: "decoded_and_normalized", budget: 64 << 10},
		{name: "decoded_output_exhausts_budget", budget: 2048, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := NewBodyMemoryBudget(tc.budget)
			collector := traffic.NewCollector()
			ctx, release := WithRequestBodyMemoryBudget(traffic.WithCollector(context.Background(), collector), budget)
			defer release()
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(encoded.Bytes())).WithContext(ctx)
			req.Header.Set("Content-Encoding", "br")
			counter := collector.Begin(traffic.DownstreamRequest, true)
			req.Body = traffic.ObserveBody(req.Body, counter, false, req.ContentLength)

			body, err := ReadLenientJSONRequestBodyWithPrealloc(req, 16<<10)
			if tc.fail {
				require.ErrorIs(t, err, ErrRequestBodyMemoryBudget)
				require.Nil(t, body)
				require.Zero(t, requestBodyBudgetUsed(budget))
			} else {
				require.NoError(t, err)
				require.Equal(t, bytes.ReplaceAll(payload, []byte{0}, []byte(`\u0000`)), body)
				require.Equal(t, int64(cap(body)), requestBodyBudgetUsed(budget))
			}

			collector.Finalize()
			usage := collector.Snapshot()
			require.NotNil(t, usage.Downstream)
			measurement := usage.Downstream.Request
			require.NotNil(t, measurement)
			require.True(t, measurement.Compressed)
			require.NotNil(t, measurement.ActualBytes)
			require.Equal(t, int64(encoded.Len()), *measurement.ActualBytes)
			if tc.fail {
				require.Nil(t, measurement.OriginalBytes)
			} else {
				require.NotNil(t, measurement.OriginalBytes)
				require.Equal(t, int64(len(payload)), *measurement.OriginalBytes)
			}
			release()
			require.Zero(t, requestBodyBudgetUsed(budget))
		})
	}
}

// A Brotli expansion past the shared 64 MiB ceiling must fail like every other
// coding (a size error, not transient memory overload) and must refund every
// temporary reservation. BestSpeed plus a reused write chunk keeps the coded
// fixture small instead of materializing the whole source in the test process.
func TestBrotliRequestDecompressionOverLimitReleasesBudget(t *testing.T) {
	expansion := int64(maxDecompressedBodySize) + 1024

	var encoded bytes.Buffer
	encoder := brotli.NewWriterLevel(&encoded, brotli.BestSpeed)
	chunk := bytes.Repeat([]byte{'a'}, 64<<10)
	for remaining := expansion; remaining > 0; {
		n := int64(len(chunk))
		if remaining < n {
			n = remaining
		}
		_, err := encoder.Write(chunk[:int(n)])
		require.NoError(t, err)
		remaining -= n
	}
	require.NoError(t, encoder.Close())
	require.Less(t, encoded.Len(), 4<<20, "coded fixture must stay small")

	// The chunk list and its contiguous copy can be charged at the same time,
	// so the budget must be large enough to reach the size check rather than
	// fail as transient overload.
	budget := NewBodyMemoryBudget(192 << 20)
	collector := traffic.NewCollector()
	ctx, release := WithRequestBodyMemoryBudget(traffic.WithCollector(context.Background(), collector), budget)
	defer release()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(encoded.Bytes())).WithContext(ctx)
	req.Header.Set("Content-Encoding", "br")
	counter := collector.Begin(traffic.DownstreamRequest, true)
	req.Body = traffic.ObserveBody(req.Body, counter, false, req.ContentLength)

	body, err := ReadRequestBodyWithPrealloc(req)
	var maxErr *http.MaxBytesError
	require.ErrorAs(t, err, &maxErr)
	require.Equal(t, int64(maxDecompressedBodySize), maxErr.Limit)
	require.NotErrorIs(t, err, ErrRequestBodyMemoryBudget)
	require.Nil(t, body)
	require.Zero(t, requestBodyBudgetUsed(budget))

	collector.Finalize()
	usage := collector.Snapshot()
	require.NotNil(t, usage)
	require.NotNil(t, usage.Downstream)
	measurement := usage.Downstream.Request
	require.NotNil(t, measurement)
	require.True(t, measurement.Compressed)
	// The failed decode leaves the original representation unknown, never zero.
	require.Nil(t, measurement.OriginalBytes)

	release()
	require.Zero(t, requestBodyBudgetUsed(budget))
}
