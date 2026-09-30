package service

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

var errOpenAISSEIdle = errors.New("upstream SSE data interval timeout")
var errOpenAISSEFirstOutput = errors.New("upstream SSE first output timeout")

// openAISSEReadPump owns the scanner and its buffer. Only its consumer writes
// downstream. Closing an HTTP response body must interrupt Read; shutdown joins
// the reader before returning its buffer to the pool.
type openAISSEReadPump struct {
	body      io.ReadCloser
	events    chan openAISSEReadEvent
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	lastRead  atomic.Int64
	text      string
	err       error
	// Consumer-owned semantic deadline; upstream heartbeats do not clear it.
	firstOutputDeadline time.Time
}

type openAISSEReadEvent struct {
	line string
	err  error
	eof  bool
}

type openAISSEActivityReader struct{ pump *openAISSEReadPump }

func (r openAISSEActivityReader) Read(b []byte) (int, error) {
	n, err := r.pump.body.Read(b)
	if n > 0 {
		// Count bytes, not complete JSON documents. A large fragmented event is
		// upstream activity even before the document scanner can emit it.
		r.pump.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

func newOpenAISSEReadPump(body io.ReadCloser, maxLineSize int) *openAISSEReadPump {
	p := &openAISSEReadPump{
		body: body, events: make(chan openAISSEReadEvent, 1),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	p.lastRead.Store(time.Now().UnixNano())
	go func() {
		defer close(p.done)
		buf := getSSEScannerBuf64K()
		defer putSSEScannerBuf64K(buf)
		scanner := bufio.NewScanner(openAISSEActivityReader{pump: p})
		scanner.Buffer(buf[:0], maxLineSize)
		documents := newOpenAISSEJSONDocumentScanner(scanner)
		send := func(e openAISSEReadEvent) bool {
			select {
			case p.events <- e:
				return true
			case <-p.stop:
				return false
			}
		}
		for documents.Scan() {
			if !send(openAISSEReadEvent{line: documents.Text()}) {
				return
			}
		}
		send(openAISSEReadEvent{err: documents.Err(), eof: true})
	}()
	return p
}

func (p *openAISSEReadPump) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		_ = p.body.Close()
		<-p.done
	})
}

// Next invokes heartbeat on the consumer goroutine. Downstream heartbeat bytes
// never refresh lastRead. A timeout is terminal, NOT proof of an unsent request.
func (p *openAISSEReadPump) Next(ctx context.Context, idle time.Duration, heartbeatCh <-chan time.Time, heartbeat func()) bool {
	if err := ctx.Err(); err != nil {
		p.err = err
		return false
	}
	if !p.firstOutputDeadline.IsZero() && !time.Now().Before(p.firstOutputDeadline) {
		p.err = errOpenAISSEFirstOutput
		return false
	}
	var timer *time.Timer
	var timeoutCh <-chan time.Time
	nextDeadline := func() time.Time {
		deadline := p.firstOutputDeadline
		if idle > 0 {
			idleDeadline := time.Unix(0, p.lastRead.Load()).Add(idle)
			if deadline.IsZero() || idleDeadline.Before(deadline) {
				deadline = idleDeadline
			}
		}
		return deadline
	}
	if deadline := nextDeadline(); !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		timer = time.NewTimer(remaining)
		timeoutCh = timer.C
		defer timer.Stop()
	}
	for {
		select {
		case e := <-p.events:
			p.text, p.err = e.line, e.err
			return !e.eof
		case <-ctx.Done():
			p.err = ctx.Err()
			return false
		case <-timeoutCh:
			// Give an already-read terminal/usage event precedence over timeout.
			select {
			case e := <-p.events:
				p.text, p.err = e.line, e.err
				return !e.eof
			default:
			}
			if !p.firstOutputDeadline.IsZero() && !time.Now().Before(p.firstOutputDeadline) {
				p.err = errOpenAISSEFirstOutput
				return false
			}
			remaining := time.Until(nextDeadline())
			if remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			p.err = errOpenAISSEIdle
			return false
		case <-heartbeatCh:
			if heartbeat != nil {
				heartbeat()
			}
		}
	}
}

func (p *openAISSEReadPump) Text() string { return p.text }
func (p *openAISSEReadPump) Err() error   { return p.err }
