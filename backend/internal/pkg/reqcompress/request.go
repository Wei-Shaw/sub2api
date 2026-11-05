package reqcompress

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

const (
	// MinCompressibleBodyBytes skips bodies that are too small to save bytes
	// after coding overhead. Below this threshold the request is forwarded
	// unchanged and does not consume a negotiation attempt.
	MinCompressibleBodyBytes int64 = 1024
	// MaxCompressibleBodyBytes bounds the memory one compressed attempt may
	// allocate. Larger bodies (for example huge inline media) are forwarded
	// unchanged rather than duplicated in memory.
	MaxCompressibleBodyBytes int64 = 16 << 20
)

// ErrBodyTooLarge reports a body that exceeds MaxCompressibleBodyBytes. It is a
// normal fallback to identity, not a request failure.
var ErrBodyTooLarge = errors.New("reqcompress: request body too large to compress")

// Eligible reports whether req may carry a compressed request body.
//
// Only re-creatable, bounded, non-signed POST/PUT/PATCH entities are eligible:
// negotiating a coding requires replaying the body, and a body that is itself
// signed (AWS SigV4) or already coded must be forwarded untouched.
func Eligible(req *http.Request) bool {
	if req == nil || req.URL == nil || req.Body == nil || req.Body == http.NoBody {
		return false
	}
	if req.GetBody == nil {
		return false
	}
	switch req.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return false
	}
	if req.ContentLength < MinCompressibleBodyBytes || req.ContentLength > MaxCompressibleBodyBytes {
		return false
	}
	if headerValue(req.Header, "Content-Encoding") != "" {
		return false
	}
	if headerValue(req.Header, "Range") != "" {
		return false
	}
	// WebSocket and other protocol upgrades are not HTTP entities.
	if headerValue(req.Header, "Upgrade") != "" || strings.Contains(strings.ToLower(headerValue(req.Header, "Connection")), "upgrade") {
		return false
	}
	return !isSignedRequest(req.Header)
}

// isSignedRequest detects payload signatures that a new Content-Encoding (and a
// different body) would invalidate.
func isSignedRequest(header http.Header) bool {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(headerValue(header, "Authorization"))), "AWS4-HMAC-SHA256") {
		return true
	}
	return headerValue(header, "X-Amz-Content-Sha256") != ""
}

// encodeRequestWithLevels returns a replayable clone of req whose body is
// compressed with algorithm. The original request is left untouched so the
// caller can fall back to identity after a rejected coding.
func encodeRequestWithLevels(req *http.Request, algorithm Algorithm, levels compresslevel.Settings) (*http.Request, error) {
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	// Closing the replay reader is best-effort; it must not change the
	// compression result or the original request used for fallback.
	defer func() { _ = body.Close() }()

	original, err := io.ReadAll(io.LimitReader(body, MaxCompressibleBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(original)) > MaxCompressibleBodyBytes {
		return nil, ErrBodyTooLarge
	}
	encoded, err := encodeBodyWithLevels(algorithm, original, levels)
	if err != nil {
		return nil, err
	}

	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(encoded))
	clone.ContentLength = int64(len(encoded))
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encoded)), nil
	}
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	deleteHeaderCaseInsensitive(clone.Header, "Content-Encoding")
	clone.Header.Set("Content-Encoding", string(algorithm))
	deleteHeaderCaseInsensitive(clone.Header, "Content-Length")

	// The uncompressed size is only knowable here. Annotating the request lets
	// the traffic collector publish a real original/actual ratio instead of an
	// unpaired compressed body.
	clone = clone.WithContext(traffic.WithOriginalRequestBodyBytes(clone.Context(), int64(len(original))))
	return clone, nil
}

// headerValue performs a case-insensitive lookup because the gateway writes
// several headers with their raw wire casing, bypassing net/http
// canonicalization.
func headerValue(header http.Header, name string) string {
	if header == nil {
		return ""
	}
	if values, ok := header[name]; ok {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
		return ""
	}
	for key, values := range header {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
		return ""
	}
	return ""
}

func deleteHeaderCaseInsensitive(header http.Header, name string) {
	for key := range header {
		if strings.EqualFold(key, name) {
			delete(header, key)
		}
	}
}
