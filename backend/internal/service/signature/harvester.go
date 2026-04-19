package signature

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

// Harvester ingests thinking signatures from upstream responses into a
// SignaturePool. It is implemented as an io.ReadCloser decorator so the
// surrounding gateway code does not need to know about signature extraction.
//
// Design: fully decoupled from the main read path.
//   - Read() copies each chunk to a buffered channel (non-blocking).
//   - A background goroutine drains the channel and parses for signatures.
//   - Panics in the goroutine are recovered and logged, never affecting callers.
//   - The goroutine also watches ctx.Done() to avoid leaking when Close() is
//     never called (e.g., abandoned responses on context cancellation).
//
// Two SSE event types carry signatures:
//   - content_block_start with a non-empty content_block.signature (legacy).
//   - content_block_delta with delta.type == "signature_delta" (current API).
//
// Non-streaming JSON responses are accumulated and parsed once on Close.
type Harvester struct {
	pool     SignaturePool
	capacity int
}

// NewHarvester builds a Harvester bound to the given pool and capacity.
func NewHarvester(pool SignaturePool, capacity int) *Harvester {
	return &Harvester{pool: pool, capacity: capacity}
}

// HarvestOptions configures a single Wrap call.
type HarvestOptions struct {
	Bucket    string
	Streaming bool
	Skip      func() bool
}

// Wrap returns a reader that transparently forwards body contents and, as a
// side effect, extracts any thinking signatures into the pool via a background
// goroutine. The returned reader must be closed.
func (h *Harvester) Wrap(ctx context.Context, body io.ReadCloser, opts HarvestOptions) io.ReadCloser {
	if h == nil || h.pool == nil || h.capacity <= 0 || opts.Bucket == "" {
		return body
	}
	chunks := make(chan []byte, harvestChanCap)
	r := &harvestReader{
		src:    body,
		chunks: chunks,
		skip:   opts.Skip,
	}
	go processChunks(ctx, chunks, h.pool, opts.Bucket, h.capacity, opts.Streaming)
	return r
}

const (
	harvestChanCap = 64
	bodyBufCap     = 2 * 1024 * 1024
	lineBufCap     = 256 * 1024
)

// harvestReader is the io.ReadCloser decorator. Its Read/Close methods are
// pure pass-throughs with a non-blocking channel send — zero parsing, zero
// Redis I/O, zero panic risk on the caller's goroutine.
type harvestReader struct {
	src       io.ReadCloser
	chunks    chan []byte
	skip      func() bool
	closeOnce sync.Once
}

func (r *harvestReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 && !r.skipNow() {
		chunk := make([]byte, n)
		copy(chunk, p[:n])
		// Non-blocking send. Recover protects against the rare case where
		// Close() races with Read() and the channel is already closed.
		func() {
			defer func() { _ = recover() }()
			select {
			case r.chunks <- chunk:
			default:
			}
		}()
	}
	return n, err
}

func (r *harvestReader) Close() error {
	r.closeOnce.Do(func() { close(r.chunks) })
	return r.src.Close()
}

func (r *harvestReader) skipNow() bool {
	return r.skip != nil && r.skip()
}

// processChunks runs in a background goroutine. It drains the chunks channel
// and parses for signatures. Exits when the channel is closed OR the context
// is cancelled (preventing goroutine leaks on abandoned responses).
func processChunks(ctx context.Context, chunks <-chan []byte, pool SignaturePool, bucket string, capacity int, streaming bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("signature_harvester_panic", "error", r, "bucket", bucket)
		}
	}()

	state := &parseState{
		ctx:    ctx,
		pool:   pool,
		bucket: bucket,
		cap:    capacity,
		stream: streaming,
	}
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				state.flush()
				return
			}
			state.observe(chunk)
		case <-ctx.Done():
			return
		}
	}
}

// parseState holds the goroutine-private parsing state.
type parseState struct {
	ctx    context.Context
	pool   SignaturePool
	bucket string
	cap    int
	stream bool

	lineBuf []byte
	bodyBuf []byte
	seen    map[string]struct{}
}

func (s *parseState) observe(chunk []byte) {
	if s.stream {
		s.observeSSE(chunk)
		return
	}
	remaining := bodyBufCap - len(s.bodyBuf)
	if remaining <= 0 {
		return
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	s.bodyBuf = append(s.bodyBuf, chunk...)
}

func (s *parseState) observeSSE(chunk []byte) {
	s.lineBuf = append(s.lineBuf, chunk...)
	for {
		idx := bytes.IndexByte(s.lineBuf, '\n')
		if idx < 0 {
			if len(s.lineBuf) > lineBufCap {
				s.lineBuf = nil
			}
			return
		}
		line := s.lineBuf[:idx]
		s.lineBuf = s.lineBuf[idx+1:]
		s.parseLine(line)
	}
}

var sseDataPrefix = []byte("data:")

// parseLine extracts a signature from a single SSE data line.
//
// Two shapes carry signatures:
//  1. content_block_start with a non-empty content_block.signature (legacy).
//  2. content_block_delta with delta.type == "signature_delta" (current API).
func (s *parseState) parseLine(line []byte) {
	line = bytes.TrimRight(line, "\r")
	if len(line) == 0 || !bytes.HasPrefix(line, sseDataPrefix) {
		return
	}
	payload := bytes.TrimSpace(line[len(sseDataPrefix):])
	if len(payload) == 0 || payload[0] != '{' {
		return
	}
	if !bytes.Contains(payload, []byte(`"signature"`)) {
		return
	}
	evType := gjson.GetBytes(payload, "type").String()
	switch evType {
	case "content_block_start":
		if sig := gjson.GetBytes(payload, "content_block.signature").String(); sig != "" {
			s.emit(sig)
		}
	case "content_block_delta":
		if gjson.GetBytes(payload, "delta.type").String() == "signature_delta" {
			if sig := gjson.GetBytes(payload, "delta.signature").String(); sig != "" {
				s.emit(sig)
			}
		}
	}
}

func (s *parseState) flush() {
	if s.stream {
		if len(s.lineBuf) > 0 {
			s.parseLine(s.lineBuf)
			s.lineBuf = nil
		}
		return
	}
	if len(s.bodyBuf) == 0 || !bytes.Contains(s.bodyBuf, []byte(`"signature"`)) {
		return
	}
	gjson.GetBytes(s.bodyBuf, "content.#.signature").ForEach(func(_, v gjson.Result) bool {
		if sig := v.String(); sig != "" {
			s.emit(sig)
		}
		return true
	})
}

func (s *parseState) emit(sig string) {
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	if _, dup := s.seen[sig]; dup {
		return
	}
	s.seen[sig] = struct{}{}
	_ = s.pool.Add(s.ctx, s.bucket, sig, time.Now(), s.cap)
}
