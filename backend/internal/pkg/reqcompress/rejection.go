package reqcompress

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// maxRejectionBodyBytes bounds how much of a suspicious error body is inspected
// before deciding whether the upstream rejected the content coding.
const maxRejectionBodyBytes int64 = 64 << 10

// rejectionStatuses are the statuses an upstream may use when it cannot decode
// a request entity. These need an encoding keyword in the body; 415 alone is
// authoritative. Other statuses (including 401/429/529) prove nothing about the
// coding.
var rejectionStatuses = map[int]struct{}{
	http.StatusBadRequest:          {},
	http.StatusUnprocessableEntity: {},
	http.StatusInternalServerError: {},
	http.StatusNotImplemented:      {},
}

// rejectionKeywords describe the failure as a content-coding problem. A status
// alone is never enough: providers also return 400 for unrelated invalid
// requests, and treating those as "coding rejected" would retry a request the
// provider may have already understood.
var rejectionKeywords = []string{
	"content-encoding",
	"content encoding",
	"unsupported encoding",
	"unsupported_encoding",
	"unsupported content coding",
	"invalid encoding",
	"invalid_encoding",
	"cannot decode",
	"could not decode",
	"unable to decode",
	"failed to decode",
	"error decoding",
	"decompress",
	"brotli",
	"zstd",
	"deflate",
}

// shouldRetryIdentity reports whether a non-success response is ambiguous enough
// that an identity attempt should be tried. Clear auth/routing/rate signals are
// excluded: identity cannot change them and a second attempt only adds load.
// 403 stays eligible because WAF/CDN layers commonly answer a body they cannot
// inspect with a generic "rejected request".
func shouldRetryIdentity(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < 400 {
		return false
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusProxyAuthRequired,
		http.StatusRequestTimeout,
		http.StatusTooManyRequests:
		return false
	case 529: // Anthropic "overloaded"
		return false
	}
	return true
}

// isEncodingRejection consumes and inspects a small error body and reports
// whether the upstream rejected the request because of its Content-Encoding.
// The body is always restored for the caller; a rejected attempt is closed by
// the caller instead of being returned.
func isEncodingRejection(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	// 415 is explicitly about an entity representation the server refuses and
	// needs no diagnostic body to be conclusive.
	if resp.StatusCode == http.StatusUnsupportedMediaType {
		return true
	}
	if resp.Body == nil || resp.Body == http.NoBody {
		return false
	}
	if _, ok := rejectionStatuses[resp.StatusCode]; !ok {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRejectionBodyBytes+1))
	if err != nil {
		resp.Body = &replayBody{reader: bytes.NewReader(body), closer: resp.Body}
		return false
	}
	if int64(len(body)) > maxRejectionBodyBytes {
		// Too large to be an encoding diagnostics message. Restore the full
		// stream, including the unread remainder.
		resp.Body = &replayBody{reader: io.MultiReader(bytes.NewReader(body), resp.Body), closer: resp.Body}
		return false
	}
	resp.Body = &replayBody{reader: bytes.NewReader(body), closer: resp.Body}
	return mentionsEncodingRejection(body)
}

func mentionsEncodingRejection(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	lower := strings.ToLower(string(body))
	for _, keyword := range rejectionKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// replayBody serves an already-consumed prefix while keeping the original Close
// (which cancels the detached upstream context and releases the connection).
type replayBody struct {
	reader io.Reader
	closer io.Closer
}

func (b *replayBody) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

func (b *replayBody) Close() error {
	return b.closer.Close()
}
