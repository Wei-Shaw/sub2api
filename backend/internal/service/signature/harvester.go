package signature

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/tidwall/gjson"
)

// Harvester ingests thinking signatures from upstream responses into a
// SignaturePool. It is implemented as an io.ReadCloser decorator so the
// surrounding gateway code does not need to know about signature extraction.
//
// Two response shapes are handled:
//   - SSE streams (content-type text/event-stream): line-based parsing, only
//     data lines carrying `content_block_start` events with a thinking block
//     are inspected.
//   - Non-streaming JSON: the entire body is accumulated and parsed once in
//     Close().
//
// Ingestion is best-effort — errors writing to Redis are swallowed. Read
// semantics of the underlying body are preserved exactly.
type Harvester struct {
	pool     SignaturePool
	capacity int // pool capacity (RectifierSettings.SignaturePoolSize)
}

// NewHarvester builds a Harvester bound to the given pool and capacity.
func NewHarvester(pool SignaturePool, capacity int) *Harvester {
	return &Harvester{pool: pool, capacity: capacity}
}

// HarvestOptions configures a single Wrap call.
type HarvestOptions struct {
	// Bucket is the pool bucket to write into; see BucketFor.
	// An empty bucket disables harvesting for this response.
	Bucket string
	// Streaming=true selects SSE line-based parsing; false accumulates the
	// body and parses once on Close.
	Streaming bool
	// Skip, when non-nil and returns true at read time, short-circuits
	// harvesting for this call. Typical use: check
	// ctxkey.IsSignatureRectifyRetry on the request context to avoid
	// re-ingesting signatures we ourselves injected.
	Skip func() bool
}

// Wrap returns a reader that transparently forwards body contents and, as a
// side effect, extracts any thinking signatures it finds into the pool. The
// returned reader must be closed; Close() flushes non-streaming extraction.
func (h *Harvester) Wrap(ctx context.Context, body io.ReadCloser, opts HarvestOptions) io.ReadCloser {
	if h == nil || h.pool == nil || h.capacity <= 0 || opts.Bucket == "" {
		return body
	}
	return &harvestReader{
		ctx:    ctx,
		src:    body,
		pool:   h.pool,
		bucket: opts.Bucket,
		cap:    h.capacity,
		skip:   opts.Skip,
		stream: opts.Streaming,
	}
}

// harvestReader is the io.ReadCloser decorator.
type harvestReader struct {
	ctx    context.Context
	src    io.ReadCloser
	pool   SignaturePool
	bucket string
	cap    int
	skip   func() bool

	stream  bool
	lineBuf []byte   // SSE line accumulator
	bodyBuf []byte   // non-streaming body accumulator (bounded)
	seen    [][]byte // de-dupe within a single response (avoid N writes for repeated sig events)
	closed  bool
}

// bodyBufCap bounds the accumulation buffer for non-streaming responses so a
// malicious / buggy upstream cannot blow memory. 2 MiB is well above any
// real Claude JSON response size.
const bodyBufCap = 2 * 1024 * 1024

func (r *harvestReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 && !r.skipNow() {
		r.observe(p[:n])
	}
	return n, err
}

func (r *harvestReader) Close() error {
	if !r.closed && !r.skipNow() {
		r.closed = true
		r.flush()
	}
	return r.src.Close()
}

func (r *harvestReader) skipNow() bool {
	if r.skip == nil {
		return false
	}
	return r.skip()
}

// observe processes a chunk of freshly-read bytes.
func (r *harvestReader) observe(chunk []byte) {
	if r.stream {
		r.observeSSE(chunk)
		return
	}
	// Non-streaming: accumulate, parse on Close.
	remaining := bodyBufCap - len(r.bodyBuf)
	if remaining <= 0 {
		return
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	r.bodyBuf = append(r.bodyBuf, chunk...)
}

// observeSSE accumulates a line at a time and parses each completed line.
func (r *harvestReader) observeSSE(chunk []byte) {
	r.lineBuf = append(r.lineBuf, chunk...)
	for {
		idx := bytes.IndexByte(r.lineBuf, '\n')
		if idx < 0 {
			return
		}
		line := r.lineBuf[:idx]
		r.lineBuf = r.lineBuf[idx+1:]
		r.parseLine(line)
	}
}

// parseLine extracts a signature from a single SSE line of form
// `data: {"type":"content_block_start",...,"content_block":{...,"signature":"..."}}`.
// Other lines are ignored cheaply.
var sseDataPrefix = []byte("data:")

func (r *harvestReader) parseLine(line []byte) {
	line = bytes.TrimRight(line, "\r")
	if len(line) == 0 {
		return
	}
	if !bytes.HasPrefix(line, sseDataPrefix) {
		return
	}
	payload := bytes.TrimSpace(line[len(sseDataPrefix):])
	if len(payload) == 0 || payload[0] != '{' {
		return
	}
	if !bytes.Contains(payload, []byte(`"signature"`)) {
		return
	}
	// Only content_block_start events carry new thinking signatures we care about.
	evType := gjson.GetBytes(payload, "type").String()
	if evType != "content_block_start" {
		return
	}
	sig := gjson.GetBytes(payload, "content_block.signature").String()
	if sig == "" {
		return
	}
	r.emit(sig)
}

// flush parses an accumulated non-streaming body at Close time.
func (r *harvestReader) flush() {
	if r.stream {
		// Any trailing line without newline — try it.
		if len(r.lineBuf) > 0 {
			r.parseLine(r.lineBuf)
			r.lineBuf = nil
		}
		return
	}
	if len(r.bodyBuf) == 0 || !bytes.Contains(r.bodyBuf, []byte(`"signature"`)) {
		return
	}
	// Claude non-streaming: top-level content[].signature
	gjson.GetBytes(r.bodyBuf, "content.#.signature").ForEach(func(_, v gjson.Result) bool {
		if sig := v.String(); sig != "" {
			r.emit(sig)
		}
		return true
	})
}

// emit writes a signature to the pool, de-duplicating within the same response.
func (r *harvestReader) emit(sig string) {
	b := []byte(sig)
	for _, prev := range r.seen {
		if bytes.Equal(prev, b) {
			return
		}
	}
	r.seen = append(r.seen, b)
	// Best-effort: ignore errors so harvesting never affects the response.
	_ = r.pool.Add(r.ctx, r.bucket, sig, time.Now(), r.cap)
}
