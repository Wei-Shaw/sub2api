//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyFailureCounterRaisesEachLevelOnce(t *testing.T) {
	counter := requestBodyFailureCounter{entries: make(map[string]requestBodyFailureWindowEntry)}
	now := time.Now()

	for i := 1; i <= 9; i++ {
		count, score, level := counter.record("user:1", 1, now)
		require.Equal(t, i, count)
		require.Equal(t, i, score)
		require.Zero(t, level)
	}
	_, _, level := counter.record("user:1", 1, now)
	require.Equal(t, 1, level)
	_, _, level = counter.record("user:1", 1, now)
	require.Zero(t, level)

	for i := 12; i <= 30; i++ {
		_, _, level = counter.record("user:1", 1, now)
	}
	require.Equal(t, 2, level)
}

func TestRequestBodyFailureCounterResetsExpiredWindow(t *testing.T) {
	counter := requestBodyFailureCounter{entries: make(map[string]requestBodyFailureWindowEntry)}
	now := time.Now()
	counter.record("user:1", 5, now)

	count, score, level := counter.record("user:1", 1, now.Add(requestBodyFailureWindow))
	require.Equal(t, 1, count)
	require.Equal(t, 1, score)
	require.Zero(t, level)
}

func TestRequestBodyFailureWeight(t *testing.T) {
	require.Zero(t, requestBodyFailureWeight(pkghttputil.RequestBodyErrorReadCanceled))
	require.Equal(t, 1, requestBodyFailureWeight(pkghttputil.RequestBodyErrorUnexpectedEOF))
	require.Equal(t, 3, requestBodyFailureWeight(pkghttputil.RequestBodyErrorInvalidCompression))
	require.Equal(t, 5, requestBodyFailureWeight(pkghttputil.RequestBodyErrorTooLarge))
}

func TestFormatRequestBodyDiagnosticContainsMetadataOnly(t *testing.T) {
	diagnostic := requestBodyDiagnostic{
		Kind:             pkghttputil.RequestBodyErrorUnexpectedEOF,
		Encoding:         "gzip",
		BytesRead:        128,
		ContentLength:    256,
		TransferEncoding: "chunked",
		WindowCount:      4,
		WindowScore:      4,
	}

	got := formatRequestBodyDiagnostic(diagnostic)
	require.Contains(t, got, "request_body_error=unexpected_eof")
	require.Contains(t, got, "bytes_read=128")
	require.NotContains(t, got, "request_body=")
}

func TestRequestBodyDiagnosticsClassifiesUnknownError(t *testing.T) {
	diagnostic := pkghttputil.RequestBodyDiagnostics(context.DeadlineExceeded)
	require.Equal(t, pkghttputil.RequestBodyErrorReadFailed, diagnostic.Kind)
}

func TestLogRequestBodyReadFailureClassifiesWithoutPayload(t *testing.T) {
	log, logs := newObservedLogger(t)
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("secret-payload-marker"))
	require.NoError(t, err)
	req.Header.Set("Content-Encoding", "gzip")
	req.ContentLength = 21

	logRequestBodyReadFailure(log, req, errors.New(`decode Content-Encoding "gzip": unexpected EOF secret-payload-marker`))

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "read request body failed", entries[0].Message)
	fields := entries[0].ContextMap()
	require.Equal(t, "decode_content_encoding", fields["error_kind"])
	require.Equal(t, "gzip", fields["content_encoding"])
	require.EqualValues(t, 21, fields["content_length"])
	require.NotContains(t, entries[0].Message+fmt.Sprint(fields), "secret-payload-marker")
}

func TestRequestBodyReadErrorKind(t *testing.T) {
	require.Equal(t, "unsupported_content_encoding", requestBodyReadErrorKind(errors.New(`decode Content-Encoding "br": unsupported Content-Encoding`)))
	require.Equal(t, "truncated_body", requestBodyReadErrorKind(io.ErrUnexpectedEOF))
	require.Equal(t, "max_bytes", requestBodyReadErrorKind(&http.MaxBytesError{Limit: 10}))
	require.Equal(t, "other", requestContentEncodingCategory("private-payload-marker"))
}
