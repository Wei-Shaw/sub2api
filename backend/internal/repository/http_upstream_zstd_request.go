package repository

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/klauspost/compress/zstd"
)

const (
	upstreamZstdMinBodyBytes = 16 << 10
	upstreamZstdMaxBodyBytes = 64 << 20
	upstreamZstdConcurrency  = 2
)

var upstreamZstdSlots = make(chan struct{}, upstreamZstdConcurrency)
var sharedUpstreamZstdEncoder = sync.OnceValues(func() (*zstd.Encoder, error) {
	return zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderConcurrency(upstreamZstdConcurrency))
})

// Encode per destination so redirects and provider fallbacks can replay the original JSON.
type upstreamRequestZstdTransport struct {
	base      http.RoundTripper
	upstream  *httpUpstreamService
	hostname  string
	accountID int64
}

func (t *upstreamRequestZstdTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	accountID := t.accountID
	if !strings.EqualFold(t.hostname, req.URL.Hostname()) {
		accountID = 0 // a different host needs its own explicit opt-in
	}
	return t.base.RoundTrip(t.upstream.compressUpstreamRequestBody(req, accountID))
}

// Use the final, replayable JSON that is about to be sent. Never consume the
// original body on a skip, so it remains available to the normal transport.
func (s *httpUpstreamService) compressUpstreamRequestBody(req *http.Request, accountID int64) *http.Request {
	if req == nil || req.URL == nil || req.Body == nil || req.GetBody == nil || req.ContentLength <= 0 || req.ContentLength > upstreamZstdMaxBodyBytes {
		return req
	}
	probe := service.UpstreamRequestZstdProbeFromContext(req.Context())
	if probe != nil {
		if !probe.Compress {
			return req
		}
	} else if req.ContentLength < upstreamZstdMinBodyBytes || !s.wantsZstdRequestBody(req.URL.Hostname(), accountID) {
		return req
	}
	if !compressibleUpstreamJSON(req.Header) {
		return req
	}
	// Bound temporary copies as well as encoder workers. Production traffic may
	// continue uncompressed if both slots are busy; an explicit probe waits.
	if probe == nil {
		select {
		case upstreamZstdSlots <- struct{}{}:
		default:
			return req
		}
	} else {
		select {
		case upstreamZstdSlots <- struct{}{}:
		case <-req.Context().Done():
			return req
		}
	}
	defer func() { <-upstreamZstdSlots }()

	encoder, err := sharedUpstreamZstdEncoder()
	if err != nil {
		return req
	}
	body, err := req.GetBody()
	if err != nil {
		return req
	}
	// Read one extra byte to detect a stale ContentLength without buffering an unbounded reader.
	raw := make([]byte, req.ContentLength+1)
	n, err := io.ReadFull(body, raw)
	_ = body.Close()
	if err != io.ErrUnexpectedEOF || int64(n) != req.ContentLength {
		return req
	}
	raw = raw[:n]
	compressed := encoder.EncodeAll(raw, nil)
	if probe == nil && len(compressed) > len(raw)*9/10 {
		return req
	}

	out := req.Clone(req.Context())
	// Some forwarding paths preserve the caller's header casing in the map.
	for key := range out.Header {
		if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Content-Encoding") {
			delete(out.Header, key)
		}
	}
	out.Header.Set("Content-Encoding", "zstd")
	out.ContentLength = int64(len(compressed))
	out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(compressed)), nil }
	out.Body = io.NopCloser(bytes.NewReader(compressed))
	_ = req.Body.Close() // the HTTP client will own and close the replacement
	if probe != nil {
		probe.Record(service.UpstreamRequestZstdAttempt{Host: req.URL.Host, RawBytes: n, WireBytes: len(compressed)})
	}
	return out
}

func (s *httpUpstreamService) wantsZstdRequestBody(hostname string, accountID int64) bool {
	return (s.cfg != nil && slices.ContainsFunc(s.cfg.Gateway.UpstreamZstdRequestHosts, func(host string) bool {
		return strings.EqualFold(host, hostname)
	})) || s.zstdAccounts.enabled(accountID)
}

func compressibleUpstreamJSON(headers http.Header) bool {
	var isJSON bool
	for name, values := range headers {
		switch strings.ToLower(name) {
		case "content-type":
			if len(values) != 1 || isJSON {
				return false
			}
			mediaType, _, err := mime.ParseMediaType(values[0])
			isJSON = err == nil && (mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"))
			if !isJSON {
				return false
			}
		case "content-encoding":
			for _, value := range values {
				if value != "" && !strings.EqualFold(strings.TrimSpace(value), "identity") {
					return false
				}
			}
		case "content-md5", "digest", "content-digest", "signature", "signature-input", "x-amz-content-sha256":
			return false // preserve the bytes used for an integrity check or signature
		case "authorization":
			for _, value := range values {
				if strings.HasPrefix(value, "AWS4-") {
					return false
				}
			}
		}
	}
	return isJSON
}
