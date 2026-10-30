//go:build unit

package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
