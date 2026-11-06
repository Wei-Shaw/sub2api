package traffic

import (
	"io"
	"net/http"
	"strings"
	"sync"
)

// IsCompressed reports a content coding other than identity. Unknown codings
// are still compressed representations, but their original length is unknown.
func IsCompressed(encoding string) bool {
	encoding = strings.TrimSpace(encoding)
	return encoding != "" && !strings.EqualFold(encoding, "identity")
}

// ObserveBody counts actual reads, never Content-Length alone. A matching known
// length can establish completion without requiring a speculative extra Read.
// identity means the same bytes also measure the original representation.
func ObserveBody(body io.ReadCloser, counter *Counter, identity bool, expected int64) io.ReadCloser {
	if counter == nil || body == nil {
		return body
	}
	return &observedBody{ReadCloser: body, counter: counter, identity: identity, expected: expected}
}

// ObserveResponseBody measures bytes actually consumed from an upstream body,
// including a stream closed at its protocol terminal event before HTTP EOF.
// Unlike an outgoing request reader (whose bytes may not reach the socket), a
// successful response Read is already an observation. Preserve those bytes on
// early Close/read failure without draining the body or guessing Content-Length.
// For encoded responses only actual bytes become available this way: decoder
// read-ahead means a partial decoded body cannot supply a trustworthy ratio.
func ObserveResponseBody(body io.ReadCloser, counter *Counter, identity bool, expected int64) io.ReadCloser {
	if counter == nil || body == nil {
		return body
	}
	if expected == 0 && body != http.NoBody {
		// Custom transports commonly leave ContentLength at its zero value.
		// An unread nonempty body must not become an observed zero-byte body.
		expected = -1
	}
	return &observedBody{ReadCloser: body, counter: counter, identity: identity, expected: expected, keepConsumed: true}
}

type observedBody struct {
	io.ReadCloser
	counter      *Counter
	identity     bool
	expected     int64
	keepConsumed bool
	mu           sync.Mutex
	read         int64
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.read += int64(n)
	b.counter.AddActual(n)
	if b.identity {
		b.counter.AddOriginal(n)
	}
	if b.keepConsumed && n > 0 {
		b.counter.CompleteActual()
		if b.identity {
			b.counter.CompleteOriginal()
		}
	}
	if err != nil && err != io.EOF {
		if !b.keepConsumed {
			b.counter.FailActual()
			if b.identity {
				b.counter.FailOriginal()
			}
		}
	} else if err == io.EOF || (b.expected >= 0 && b.read == b.expected) {
		b.counter.CompleteActual()
		if b.identity {
			b.counter.CompleteOriginal()
		}
	}
	return n, err
}

// ObserveOriginal is placed after a content decoder, before protocol transforms.
func ObserveOriginal(body io.ReadCloser, counter *Counter) io.ReadCloser {
	if body == nil || counter == nil {
		return body
	}
	return &originalBody{ReadCloser: body, counter: counter}
}

type originalBody struct {
	io.ReadCloser
	counter *Counter
}

func (b *originalBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.counter.AddOriginal(n)
	if err == io.EOF {
		b.counter.CompleteOriginal()
	} else if err != nil {
		b.counter.FailOriginal()
	}
	return n, err
}
