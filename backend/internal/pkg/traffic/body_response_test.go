package traffic

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type responseReadProbe struct {
	reader io.Reader
	reads  int
	closed bool
}

func (p *responseReadProbe) Read(dst []byte) (int, error) {
	p.reads++
	return p.reader.Read(dst)
}
func (p *responseReadProbe) Close() error { p.closed = true; return nil }

type responseErrorReader struct {
	payload string
	err     error
}

func (r *responseErrorReader) Read(dst []byte) (int, error) {
	n := copy(dst, r.payload)
	r.payload = r.payload[n:]
	return n, r.err
}

func TestObserveResponseBodyRetainsConsumedPrefixWithoutDraining(t *testing.T) {
	for _, identity := range []bool{false, true} {
		t.Run(map[bool]string{false: "encoded", true: "identity"}[identity], func(t *testing.T) {
			collector := NewCollector()
			counter := collector.Begin(UpstreamResponse, !identity)
			raw := &responseReadProbe{reader: strings.NewReader("prefix-unread-tail")}
			body := ObserveResponseBody(raw, counter, identity, 18)
			n, err := body.Read(make([]byte, 6))
			require.NoError(t, err)
			require.Equal(t, 6, n)
			require.NoError(t, body.Close())
			require.Equal(t, 1, raw.reads, "Close must not consume any additional upstream data")
			require.True(t, raw.closed)
			collector.Finalize()
			measured := collector.Snapshot().Upstream.Response
			require.NotNil(t, measured.ActualBytes)
			require.EqualValues(t, n, *measured.ActualBytes, "never substitute advertised body size for consumed bytes")
			if identity {
				require.NotNil(t, measured.OriginalBytes)
				require.EqualValues(t, n, *measured.OriginalBytes)
			} else {
				require.Nil(t, measured.OriginalBytes, "encoded and decoded prefixes may have different read-ahead")
				require.Nil(t, measured.Ratio())
			}
		})
	}
}

func TestObserveResponseBodyReadErrorPreservesDeliveredBytes(t *testing.T) {
	for _, payload := range []string{"", "received"} {
		for _, identity := range []bool{false, true} {
			collector := NewCollector()
			counter := collector.Begin(UpstreamResponse, !identity)
			body := ObserveResponseBody(&responseReadProbe{reader: &responseErrorReader{payload: payload, err: io.ErrUnexpectedEOF}}, counter, identity, -1)
			n, err := body.Read(make([]byte, 32))
			require.ErrorIs(t, err, io.ErrUnexpectedEOF, "accounting must not alter stream failure semantics")
			require.Equal(t, len(payload), n)
			require.NoError(t, body.Close())
			measured := collector.Snapshot().Upstream.Response
			if payload == "" {
				require.Nil(t, measured.ActualBytes, "an error without any observed bytes is not measured zero")
				require.Nil(t, measured.OriginalBytes)
			} else {
				require.NotNil(t, measured.ActualBytes)
				require.EqualValues(t, len(payload), *measured.ActualBytes)
				if identity {
					require.NotNil(t, measured.OriginalBytes)
					require.EqualValues(t, len(payload), *measured.OriginalBytes)
				} else {
					require.Nil(t, measured.OriginalBytes)
				}
			}
		}
	}
}

func TestObserveResponseBodyUnreadAndMeasuredEmptyStayDistinct(t *testing.T) {
	for _, length := range []int64{-1, 0, 7} {
		collector := NewCollector()
		counter := collector.Begin(UpstreamResponse, false)
		raw := &responseReadProbe{reader: strings.NewReader("payload")}
		body := ObserveResponseBody(raw, counter, true, length)
		_, err := body.Read(nil)
		require.NoError(t, err)
		require.NoError(t, body.Close())
		require.Equal(t, 1, raw.reads)
		measured := collector.Snapshot().Upstream.Response
		require.Nil(t, measured.ActualBytes)
		require.Nil(t, measured.OriginalBytes)
	}

	collector := NewCollector()
	counter := collector.Begin(UpstreamResponse, false)
	body := ObserveResponseBody(io.NopCloser(strings.NewReader("")), counter, true, -1)
	_, err := body.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	measured := collector.Snapshot().Upstream.Response
	require.NotNil(t, measured.ActualBytes)
	require.NotNil(t, measured.OriginalBytes)
	require.Zero(t, *measured.ActualBytes)
	require.Zero(t, *measured.OriginalBytes)
}

func TestObserveResponseBodyCombinesConsumedRetryPrefixes(t *testing.T) {
	collector := NewCollector()
	for _, prefix := range []string{"first", "second"} {
		counter := collector.Begin(UpstreamResponse, false)
		body := ObserveResponseBody(io.NopCloser(strings.NewReader(prefix+"-tail")), counter, true, -1)
		_, err := io.ReadFull(body, make([]byte, len(prefix)))
		require.NoError(t, err)
		require.NoError(t, body.Close())
	}
	measured := collector.Snapshot().Upstream.Response
	require.EqualValues(t, len("firstsecond"), *measured.ActualBytes)
	require.EqualValues(t, len("firstsecond"), *measured.OriginalBytes)
}

func TestObserveResponseBodyDoesNotRelaxOutgoingRequestAccounting(t *testing.T) {
	collector := NewCollector()
	counter := collector.Begin(UpstreamRequest, false)
	failure := errors.New("request body reader failure")
	body := ObserveBody(&responseReadProbe{reader: &responseErrorReader{payload: "not proven sent", err: failure}}, counter, true, -1)
	_, err := body.Read(make([]byte, 32))
	require.ErrorIs(t, err, failure)
	counter.CompleteActual()
	counter.CompleteOriginal()
	measured := collector.Snapshot().Upstream.Request
	require.Nil(t, measured.ActualBytes)
	require.Nil(t, measured.OriginalBytes)
}
