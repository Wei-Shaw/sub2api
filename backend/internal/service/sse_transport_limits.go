package service

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultSSEQueueMaxBytes = 8 << 20
	defaultSSEEventMaxLines = 4096
)

// sseQueueBudget bounds queued payload bytes, in addition to the channel's
// event count. A single line larger than the budget reserves the whole budget:
// legitimate image frames still pass, but cannot fill all 16 queue slots. The
// consumer's current line and the producer's next line/scanner buffer are not
// queue storage and remain subject to the line/event limits.
//
// There is one producer and one consumer. Release on dequeue, not on shutdown,
// so cancellation never depends on draining an abandoned queue.
type sseQueueBudget struct {
	limit     int64
	used      atomic.Int64
	available chan struct{}
}

func newSSEQueueBudget(cfg *config.Config) *sseQueueBudget {
	limit := int64(defaultSSEQueueMaxBytes)
	if cfg != nil && cfg.Gateway.SSEQueueMaxBytes > 0 {
		limit = cfg.Gateway.SSEQueueMaxBytes
	}
	return &sseQueueBudget{limit: limit, available: make(chan struct{}, 1)}
}

func (b *sseQueueBudget) weight(size int) int64 {
	return min(int64(size), b.limit)
}

func (b *sseQueueBudget) acquire(size int, done <-chan struct{}) bool {
	weight := b.weight(size)
	for {
		select {
		case <-done:
			return false
		default:
		}
		used := b.used.Load()
		if weight <= b.limit-used && b.used.CompareAndSwap(used, used+weight) {
			return true
		}
		select {
		case <-done:
			return false
		case <-b.available:
		}
	}
}

func (b *sseQueueBudget) release(size int) {
	if size == 0 {
		return
	}
	b.used.Add(-b.weight(size))
	select {
	case b.available <- struct{}{}:
	default:
	}
}

var errSSETransportLimit = errors.New("upstream SSE transport limit exceeded")

type sseEventLimits struct {
	maxBytes int64
	maxLines int
	bytes    int64
	lines    int
}

func newSSEEventLimits(cfg *config.Config, maxLineSize int) *sseEventLimits {
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
	}
	limits := &sseEventLimits{maxBytes: int64(maxLineSize), maxLines: defaultSSEEventMaxLines}
	if cfg != nil {
		if cfg.Gateway.SSEEventMaxBytes > 0 {
			limits.maxBytes = cfg.Gateway.SSEEventMaxBytes
		}
		if cfg.Gateway.SSEEventMaxLines > 0 {
			limits.maxLines = cfg.Gateway.SSEEventMaxLines
		}
	}
	return limits
}

func (l *sseEventLimits) observe(line []byte, wireBytes int) error {
	if len(bytes.TrimRight(line, "\r\n")) == 0 {
		l.bytes, l.lines = 0, 0
		return nil
	}
	if int64(wireBytes) > l.maxBytes-l.bytes || l.lines >= l.maxLines {
		return l.exceeded()
	}
	l.bytes += int64(wireBytes)
	l.lines++
	return nil
}

func (l *sseEventLimits) exceeded() error {
	// Preserve the existing upstream response_too_large error paths. This is
	// an SSE framing limit, independent of JSON/protocol translation.
	return fmt.Errorf("%w: %w: SSE event exceeds %d bytes or %d lines", errSSETransportLimit, bufio.ErrTooLong, l.maxBytes, l.maxLines)
}

// newBoundedSSESplit limits complete events as well as incomplete lines. Without
// this, an upstream can bypass a single-line cap by never sending a blank line.
// next may preserve line endings or enforce the OpenAI pre-output guard.
func newBoundedSSESplit(cfg *config.Config, maxLineSize int, next bufio.SplitFunc) bufio.SplitFunc {
	if next == nil {
		next = bufio.ScanLines
	}
	limits := newSSEEventLimits(cfg, maxLineSize)
	return func(data []byte, atEOF bool) (int, []byte, error) {
		advance, token, err := next(data, atEOF)
		if err != nil {
			return advance, token, err
		}
		if token != nil {
			if err := limits.observe(token, advance); err != nil {
				return 0, nil, err
			}
		} else if int64(len(data)) > limits.maxBytes-limits.bytes {
			return 0, nil, limits.exceeded()
		}
		return advance, token, nil
	}
}

// boundedSSELineReader preserves the original bytes (including CRLF and the
// final unterminated line) used by image passthrough. ReadSlice bounds individual
// allocations; oversized/incomplete lines are rejected without forwarding a
// truncated prefix as a successful SSE event.
type boundedSSELineReader struct {
	reader      *bufio.Reader
	maxLineSize int
	limits      *sseEventLimits
}

func newBoundedSSELineReader(reader io.Reader, cfg *config.Config) *boundedSSELineReader {
	maxLineSize := defaultMaxLineSize
	if cfg != nil && cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = cfg.Gateway.MaxLineSize
	}
	return &boundedSSELineReader{reader: bufio.NewReader(reader), maxLineSize: maxLineSize, limits: newSSEEventLimits(cfg, maxLineSize)}
}

func (r *boundedSSELineReader) ReadLine() ([]byte, error) {
	var line []byte
	for {
		part, err := r.reader.ReadSlice('\n')
		if len(part) > r.maxLineSize-len(line) || (len(bytes.TrimRight(part, "\r\n")) != 0 || len(line) != 0) && int64(len(line)+len(part)) > r.limits.maxBytes-r.limits.bytes {
			return nil, r.limits.exceeded()
		}
		line = append(line, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if len(line) > 0 {
			if limitErr := r.limits.observe(line, len(line)); limitErr != nil {
				return nil, limitErr
			}
		}
		return line, err
	}
}

func (s *AntigravityGatewayService) antigravitySSEConfig() *config.Config {
	if s == nil || s.settingService == nil {
		return nil
	}
	return s.settingService.cfg
}
