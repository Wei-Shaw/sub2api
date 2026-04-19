//go:build unit

package signature

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// waitPool polls the fakePool until the expected count is reached or timeout.
func waitPool(pool *fakePool, bucket string, want int, timeout time.Duration) []string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pool.mu.Lock()
		got := len(pool.entries[bucket])
		pool.mu.Unlock()
		if got >= want {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	result := make([]string, len(pool.entries[bucket]))
	copy(result, pool.entries[bucket])
	return result
}

// waitPoolEmpty waits briefly and asserts the pool bucket remains empty.
func waitPoolEmpty(t *testing.T, pool *fakePool, bucket string) {
	t.Helper()
	// Give the goroutine time to process, then check.
	time.Sleep(200 * time.Millisecond)
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.entries[bucket]) != 0 {
		t.Errorf("expected pool %q to be empty, got %v", bucket, pool.entries[bucket])
	}
}

func TestHarvester_SSE_ExtractsContentBlockStartSignatures(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	body := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":"SIG-A"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":"","signature":"SIG-B"}}`,
		``,
	}, "\n") + "\n"

	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = rc.Close()

	got := waitPool(pool, "oauth", 2, 2*time.Second)
	if len(got) != 2 {
		t.Fatalf("expected 2 signatures, got %d: %v", len(got), got)
	}
	if got[0] != "SIG-B" || got[1] != "SIG-A" {
		t.Errorf("ordering: got %v, want [SIG-B, SIG-A]", got)
	}
}

func TestHarvester_SSE_ExtractsSignatureDelta(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	// Real-world SSE shape: content_block_start has empty signature,
	// the real signature arrives as a signature_delta event.
	body := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"EqACCkg..."}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
	}, "\n") + "\n"

	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	got := waitPool(pool, "oauth", 1, 2*time.Second)
	if len(got) != 1 || got[0] != "EqACCkg..." {
		t.Errorf("expected [EqACCkg...], got %v", got)
	}
}

func TestHarvester_SSE_ExtractsBothStartAndDelta(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	body := strings.Join([]string{
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","signature":"FROM-START"}}`,
		``,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"FROM-DELTA"}}`,
		``,
	}, "\n") + "\n"

	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	got := waitPool(pool, "oauth", 2, 2*time.Second)
	if len(got) != 2 {
		t.Fatalf("expected 2 signatures (start + delta), got %d: %v", len(got), got)
	}
}

func TestHarvester_SSE_IgnoresEmptySignatureInStart(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	body := `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}` + "\n\n"
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	waitPoolEmpty(t, pool, "oauth")
}

func TestHarvester_SSE_DeduplicatesWithinSameResponse(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := strings.Repeat(
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"signature_delta\",\"signature\":\"DUP\"}}\n\n",
		5,
	)
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	got := waitPool(pool, "oauth", 1, 2*time.Second)
	if len(got) != 1 {
		t.Errorf("expected dedup to 1, got %d: %v", len(got), got)
	}
}

func TestHarvester_SkipCallbackBlocksIngestion(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := `data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"NO"}}` + "\n\n"
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
		Skip:      func() bool { return true },
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	waitPoolEmpty(t, pool, "oauth")
}

func TestHarvester_NonStreaming_ParsesOnClose(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := `{"id":"msg_1","content":[
		{"type":"thinking","thinking":"x","signature":"NS-A"},
		{"type":"text","text":"hi"},
		{"type":"thinking","thinking":"y","signature":"NS-B"}
	]}`
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: false,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	got := waitPool(pool, "oauth", 2, 2*time.Second)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after Close, got %d: %v", len(got), got)
	}
}

func TestHarvester_EmptyBucketDisablesWrap(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	inner := io.NopCloser(strings.NewReader("irrelevant"))
	out := h.Wrap(context.Background(), inner, HarvestOptions{Bucket: ""})
	if out != inner {
		t.Errorf("empty bucket must return original body")
	}
}

func TestHarvester_ZeroCapacityDisablesWrap(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 0)
	inner := io.NopCloser(strings.NewReader("irrelevant"))
	out := h.Wrap(context.Background(), inner, HarvestOptions{Bucket: "oauth"})
	if out != inner {
		t.Errorf("zero capacity must return original body")
	}
}

type splitReader struct {
	data []byte
	pos  int
	size int
}

func (r *splitReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	end := r.pos + r.size
	if end > len(r.data) {
		end = len(r.data)
	}
	n := copy(p, r.data[r.pos:end])
	r.pos += n
	return n, nil
}

func TestHarvester_SSE_SignatureDeltaSplitAcrossReads(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	line := `data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"SPLIT-SIG"}}` + "\n\n"
	rc := h.Wrap(context.Background(), io.NopCloser(&splitReader{data: []byte(line), size: 7}), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = rc.Close()

	got := waitPool(pool, "oauth", 1, 2*time.Second)
	if len(got) != 1 || got[0] != "SPLIT-SIG" {
		t.Errorf("got %v, want [SPLIT-SIG]", got)
	}
}

func TestHarvester_SSE_LineBufCapTruncatesHugeLine(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	huge := strings.Repeat("x", lineBufCap+1000)
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(huge)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	waitPoolEmpty(t, pool, "oauth")
}

func TestHarvester_ReadSemanticPreserved(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	original := "hello world this is the response body"
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(original)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = rc.Close()
	if string(data) != original {
		t.Errorf("read semantics broken: got %q, want %q", string(data), original)
	}
}

func TestHarvester_PanicInPoolDoesNotAffectRead(t *testing.T) {
	pool := &panicPool{}
	h := NewHarvester(pool, 10)
	body := `data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"BOOM"}}` + "\n\n"
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read should succeed even if pool panics: %v", err)
	}
	_ = rc.Close()
	if string(data) != body {
		t.Errorf("read data corrupted")
	}
}

func TestHarvester_SSE_TextDeltaContainingSignatureWordIgnored(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	body := strings.Join([]string{
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"please check your signature here"}}`,
		``,
	}, "\n") + "\n"

	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	waitPoolEmpty(t, pool, "oauth")
}

func TestHarvester_SSE_NoThinkingBlocksProducesNothing(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	body := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"model":"claude-sonnet-4-6"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello!"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n") + "\n"

	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	waitPoolEmpty(t, pool, "oauth")
}

func TestHarvester_ContextCancellationStopsGoroutine(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	ctx, cancel := context.WithCancel(context.Background())

	body := `data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"CTX-SIG"}}` + "\n\n"
	rc := h.Wrap(ctx, io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})

	_, _ = io.ReadAll(rc)
	// Cancel context before Close — goroutine should exit via ctx.Done()
	cancel()
	time.Sleep(50 * time.Millisecond)
	_ = rc.Close()
}

func TestHarvester_DoubleCloseNoPanic(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := `data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"X"}}` + "\n\n"
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()
	_ = rc.Close() // second close must not panic
}

type panicPool struct{}

func (p *panicPool) Add(_ context.Context, _, _ string, _ time.Time, _ int) error {
	panic("pool exploded")
}
func (p *panicPool) TopN(_ context.Context, _ string, _ int) ([]string, error) { return nil, nil }
func (p *panicPool) Size(_ context.Context, _ string) (int64, error)           { return 0, nil }
