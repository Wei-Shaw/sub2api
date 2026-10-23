package httputil

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"sync"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// decompressZstdWithStreamingReader is the streaming decode that requests
// without a usable declared size still take, kept as the reference.
func decompressZstdWithStreamingReader(raw []byte) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return io.ReadAll(io.LimitReader(dec, maxDecompressedBodySize))
}

func zstdStreamFrame(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestZstdEncoder(t *testing.T, opts ...zstd.EOption) *zstd.Encoder {
	t.Helper()
	enc, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = enc.Close() })
	return enc
}

func TestDecompressZstdRequestBodyMatchesStreamingReader(t *testing.T) {
	enc := newTestZstdEncoder(t)
	// Single-segment frames always declare their size, also for small and
	// empty input; the default encoder leaves it out for very small input.
	sized := newTestZstdEncoder(t, zstd.WithSingleSegment(true), zstd.WithZeroFrames(true))
	small := []byte(samplePayload)
	image := zstdImageRequestBody(3 << 20)
	frame := enc.EncodeAll(image, nil)
	repetitive := bytes.Repeat([]byte(`{"type":"input_text","text":"same"},`), 1<<16)
	oversized := bytes.Repeat([]byte{'A'}, maxDecompressedBodySize+1)
	noChecksum := newTestZstdEncoder(t, zstd.WithEncoderCRC(false))
	// A skippable frame: magic 0x184D2A50, little-endian payload size, payload.
	skippable := append([]byte{0x50, 0x2a, 0x4d, 0x18, 3, 0, 0, 0}, "pad"...)
	// A single-segment frame declaring 100 bytes in one RLE block of 'A',
	// without checksum.
	rleFrame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x20, 100, 0x23, 0x03, 0x00, 'A'}

	cases := []struct {
		name string
		raw  []byte
		fast bool // decoded by decodeZstdRequestBodyOfDeclaredSize
	}{
		{"declared_small", sized.EncodeAll(small, nil), true},
		{"declared_image", frame, true},
		{"declared_large", enc.EncodeAll(zstdImageRequestBody(24<<20), nil), true},
		{"declared_without_checksum", noChecksum.EncodeAll(image, nil), true},
		{"declared_rle_block", rleFrame, true},
		{"small_without_size", enc.EncodeAll(small, nil), false},
		{"streamed_without_size", zstdStreamFrame(t, image), false},
		{"two_frames", append(bytes.Clone(frame), sized.EncodeAll(small, nil)...), false},
		{"skippable_frame_after", append(bytes.Clone(frame), skippable...), false},
		{"skippable_frame_first", append(bytes.Clone(skippable), frame...), false},
		{"above_expansion", enc.EncodeAll(repetitive, nil), false},
		{"above_limit", enc.EncodeAll(oversized, nil), false},
		{"truncated", frame[:len(frame)/2], false},
		{"trailing_garbage", append(bytes.Clone(frame), 'x', 'y'), false},
		{"empty_frame", sized.EncodeAll(nil, nil), false},
		{"empty_body", []byte{}, false},
		{"not_zstd", []byte("not actually zstd"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, fast := decodeZstdRequestBodyOfDeclaredSize(tc.raw); fast != tc.fast {
				t.Fatalf("declared-size path taken=%v, want %v", fast, tc.fast)
			}
			want, wantErr := decompressZstdWithStreamingReader(tc.raw)
			got, err := decompressRequestBody("zstd", tc.raw)
			if (wantErr == nil) != (err == nil) {
				t.Fatalf("error changed: streaming=%v got=%v", wantErr, err)
			}
			if err == nil && !bytes.Equal(want, got) {
				t.Fatalf("decoded body changed: streaming len=%d got len=%d", len(want), len(got))
			}
		})
	}
	var header zstd.Header
	if err := header.Decode(sized.EncodeAll(nil, nil)); err != nil || !header.HasFCS || header.FrameContentSize != 0 {
		t.Fatalf("empty_frame does not declare a zero size: %+v err=%v", header, err)
	}
}

//go:noinline
func decodeZstdAndDrop(raw []byte) {
	_, _ = decompressRequestBody("zstd", raw)
}

func TestDecompressZstdRequestBodyKeepsNoReferenceToBodies(t *testing.T) {
	enc := newTestZstdEncoder(t)
	block := make([]byte, 2<<20)
	rng := rand.New(rand.NewSource(1))
	for i := range block {
		block[i] = byte(rng.Intn(256))
	}
	// Repeating a random block compresses well enough to take the declared-size
	// path for a 24 MiB body.
	raw := enc.EncodeAll(bytes.Repeat(block, 12), nil)
	if _, ok := zstdSingleFrameContentSize(raw); !ok {
		t.Fatal("repeated body does not qualify for the declared-size path")
	}
	corrupt := bytes.Clone(raw)
	corrupt[len(corrupt)-1] ^= 0xff // frame checksum
	// Decode small frames first, so a decoder state kept from an earlier run
	// (or from another test) does not already hold a large body when the
	// baseline is taken.
	smallFrame := enc.EncodeAll(zstdImageRequestBody(4<<10), nil)
	for i := 0; i < 2*runtime.GOMAXPROCS(0); i++ {
		decodeZstdAndDrop(smallFrame)
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decodeZstdAndDrop(raw)
			decodeZstdAndDrop(corrupt)
		}()
	}
	wg.Wait()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(raw)
	runtime.KeepAlive(corrupt)
	runtime.KeepAlive(enc)
	if after.HeapAlloc > before.HeapAlloc+8<<20 {
		t.Fatalf("decoded bodies stayed reachable: heap %d -> %d bytes", before.HeapAlloc, after.HeapAlloc)
	}
}

// minAllocatedBytes returns the fewest bytes fn allocated over a few runs, so
// allocations by unrelated goroutines do not make the check flaky.
func minAllocatedBytes(fn func()) uint64 {
	var best uint64
	for i := 0; i < 5; i++ {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		fn()
		runtime.ReadMemStats(&after)
		if allocated := after.TotalAlloc - before.TotalAlloc; i == 0 || allocated < best {
			best = allocated
		}
	}
	return best
}

func TestDecompressZstdRequestBodyOfDeclaredSizeAllocatesResultOnce(t *testing.T) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	for _, size := range []int{256 << 10, 3 << 20} {
		body := zstdImageRequestBody(size)
		raw := enc.EncodeAll(body, nil)
		t.Run(fmt.Sprintf("bytes_%d", len(body)), func(t *testing.T) {
			allocated := minAllocatedBytes(func() {
				got, err := decompressRequestBody("zstd", raw)
				if err != nil || !bytes.Equal(got, body) {
					t.Fatalf("decoded body changed: err=%v", err)
				}
			})
			if limit := uint64(len(body)) * 9 / 8; allocated > limit {
				t.Fatalf("allocated %d bytes to decode %d bytes", allocated, len(body))
			}
		})
	}
}
