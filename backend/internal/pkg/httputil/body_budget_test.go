package httputil

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestRequestBodyMemoryBudgetSharedUntilRelease(t *testing.T) {
	budget := NewBodyMemoryBudget(1024)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	req := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 700))).WithContext(ctx)
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil || len(got) != 700 {
		t.Fatalf("read: %d %v", len(got), err)
	}
	ctx2, release2 := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release2()
	req2 := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 700))).WithContext(ctx2)
	if _, err := ReadRequestBodyWithPrealloc(req2); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("want capacity error, got %v", err)
	}
	release()
	release() // Idempotent cleanup cannot refund another request's allocation.
	if _, err := ReadRequestBodyWithPrealloc(req2); err != nil {
		t.Fatal(err)
	}
}

func TestRequestBodyMemoryBudgetUnknownLengthAndConcurrency(t *testing.T) {
	budget := NewBodyMemoryBudget(4096)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	req := httptest.NewRequest("POST", "/", io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, 8192)))).WithContext(ctx)
	req.ContentLength = -1
	if _, err := ReadRequestBodyWithPrealloc(req); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("unknown length bypassed budget: %v", err)
	}
	release()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
			defer release()
			_ = reserveRequestBodyMemory(ctx, 1024)
		})
	}
	wg.Wait()
	if budget.used != 0 {
		t.Fatalf("leaked reservations: %d", budget.used)
	}
}

func TestDecompressRequestBodyRejectsExcessiveZstdWindow(t *testing.T) {
	for _, exponent := range []byte{20, 24, 29} {
		raw := []byte{0x28, 0xb5, 0x2f, 0xfd, 0, (exponent - 10) << 3, 9, 0, 0, 'x'}
		decoded, err := decompressRequestBody("zstd", raw)
		if exponent == 20 {
			if err != nil || string(decoded) != "x" {
				t.Fatalf("small window: %q %v", decoded, err)
			}
		} else if err == nil {
			t.Fatalf("accepted window 2^%d", exponent)
		}
	}
}

func TestDecompressRequestBodyRejectsOversizeInsteadOfTruncating(t *testing.T) {
	var compressed bytes.Buffer
	encoder := gzip.NewWriter(&compressed)
	chunk := bytes.Repeat([]byte{' '}, 1<<20)
	for range maxDecompressedBodySize / len(chunk) {
		_, _ = encoder.Write(chunk)
	}
	_, _ = encoder.Write([]byte{'x'})
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	budget := NewBodyMemoryBudget(256 << 20)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	req := newRequestWithBody(t, compressed.Bytes(), "gzip").WithContext(ctx)
	decoded, err := ReadRequestBodyWithPrealloc(req)
	var maxErr *http.MaxBytesError
	if decoded != nil || !errors.As(err, &maxErr) || maxErr.Limit != maxDecompressedBodySize {
		t.Fatalf("silently truncated: len=%d err=%v", len(decoded), err)
	}
	if used := requestBodyBudgetUsed(budget); used != 0 {
		t.Fatalf("oversize decoded body retained %d bytes", used)
	}
}

func TestDecodedRequestBodySharesMemoryBudget(t *testing.T) {
	var compressed bytes.Buffer
	encoder := gzip.NewWriter(&compressed)
	_, _ = encoder.Write(bytes.Repeat([]byte{'x'}, 4096))
	_ = encoder.Close()
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), NewBodyMemoryBudget(2048))
	defer release()
	req := httptest.NewRequest("POST", "/", bytes.NewReader(compressed.Bytes())).WithContext(ctx)
	req.Header.Set("Content-Encoding", "gzip")
	if _, err := ReadRequestBodyWithPrealloc(req); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("decode bypassed budget: %v", err)
	}
}

func TestNormalizedRequestBodySharesMemoryBudget(t *testing.T) {
	body := []byte(`{"input":"` + strings.Repeat("\x00", 100) + `"}`)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), NewBodyMemoryBudget(600))
	defer release()
	req := httptest.NewRequest("POST", "/", bytes.NewReader(body)).WithContext(ctx)
	if _, err := ReadLenientJSONRequestBodyWithPrealloc(req, 4096); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("normalization bypassed budget: %v", err)
	}
}

func requestBodyBudgetUsed(budget *BodyMemoryBudget) int64 {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.used
}

func TestRequestBodyMemoryBudgetRetainsOnlyFinalAllocation(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip", "zstd"} {
		for _, normalize := range []bool{false, true} {
			name := encoding
			if normalize {
				name += "/normalized"
			}
			t.Run(name, func(t *testing.T) {
				text := strings.Repeat("x", requestBodyReadMaxInitCap+64)
				if normalize {
					text += "\x00"
				}
				payload := []byte(`{"input":"` + text + `"}`)
				raw := payload
				switch encoding {
				case "gzip":
					var compressed bytes.Buffer
					encoder := gzip.NewWriter(&compressed)
					if _, err := encoder.Write(payload); err != nil {
						t.Fatal(err)
					}
					if err := encoder.Close(); err != nil {
						t.Fatal(err)
					}
					raw = compressed.Bytes()
				case "zstd":
					encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
					if err != nil {
						t.Fatal(err)
					}
					raw = encoder.EncodeAll(payload, nil)
					if err := encoder.Close(); err != nil {
						t.Fatal(err)
					}
				}
				budget := NewBodyMemoryBudget(32 << 20)
				ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
				defer release()
				req := newRequestWithBody(t, raw, encoding).WithContext(ctx)
				// Unknown length exercises chunk growth and its extra EOF probe.
				req.ContentLength = -1
				got, err := ReadLenientJSONRequestBodyWithPrealloc(req, 4<<20)
				if err != nil {
					t.Fatal(err)
				}
				want := bytes.ReplaceAll(payload, []byte{0}, []byte(`\u0000`))
				if !bytes.Equal(got, want) {
					t.Fatal("body contents changed")
				}
				if used := requestBodyBudgetUsed(budget); used != int64(cap(got)) {
					t.Fatalf("retained %d bytes, final allocation is %d", used, cap(got))
				}
				release()
				if used := requestBodyBudgetUsed(budget); used != 0 {
					t.Fatalf("handler cleanup retained %d bytes", used)
				}
			})
		}
	}
}

func TestRequestBodyMemoryBudgetReusesTransientCapacityWhileHandlersWait(t *testing.T) {
	const bodySize = requestBodyReadMaxInitCap + 512
	// Enough for two retained bodies plus the third body before concatenation.
	// A third simultaneous final copy must still be rejected.
	budget := NewBodyMemoryBudget(3*bodySize + 1)
	payload := bytes.Repeat([]byte{'x'}, bodySize)
	readDone := make(chan error, 2)
	finish := make(chan struct{})
	var handlers sync.WaitGroup
	defer func() {
		close(finish)
		handlers.Wait()
		if used := requestBodyBudgetUsed(budget); used != 0 {
			t.Errorf("completed handlers retained %d bytes", used)
		}
	}()
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, release := WithRequestBodyMemoryBudget(req.Context(), budget)
		defer release()
		body, err := ReadRequestBodyWithPrealloc(req.WithContext(ctx))
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		readDone <- nil
		<-finish
		// Keep the final body live for the entire waiting handler.
		_, _ = w.Write(body[:1])
	})
	for range 2 {
		handlers.Go(func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload)))
			if recorder.Code != http.StatusOK {
				readDone <- errors.New("temporary allocations blocked a second held request")
			}
		})
		select {
		case err := <-readDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("request did not finish reading")
		}
	}
	if used := requestBodyBudgetUsed(budget); used != 2*bodySize {
		t.Fatalf("waiting handlers retain %d bytes, want %d", used, 2*bodySize)
	}
	thirdDone := make(chan *httptest.ResponseRecorder, 1)
	handlers.Go(func() {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload)))
		thirdDone <- recorder
	})
	select {
	case recorder := <-thirdDone:
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("third request escaped the transient peak budget: %d", recorder.Code)
		}
	case <-readDone:
		t.Fatal("third request escaped the transient peak budget")
	case <-time.After(5 * time.Second):
		t.Fatal("third request did not reject overload")
	}
	if used := requestBodyBudgetUsed(budget); used != 2*bodySize {
		t.Fatalf("failed request retained temporary storage: %d", used)
	}
}

type requestBodyBudgetReaderFunc func([]byte) (int, error)

func (read requestBodyBudgetReaderFunc) Read(p []byte) (int, error) { return read(p) }

func TestRequestBodyMemoryBudgetRefundsFailedOwnedBuffers(t *testing.T) {
	for _, name := range []string{"unknown_length_overload", "concat_overload", "normalization_overload", "normalization_size", "invalid_gzip", "invalid_zstd", "read_error", "cancellation"} {
		t.Run(name, func(t *testing.T) {
			budget := NewBodyMemoryBudget(16 << 20)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx, release := WithRequestBodyMemoryBudget(parent, budget)
			defer release()
			// A previous returned/borrowed allocation must survive this failure.
			if err := reserveRequestBodyMemory(ctx, 100); err != nil {
				t.Fatal(err)
			}
			payload := []byte(`{"input":"` + strings.Repeat("x", 100) + `"}`)
			maxNormalized := int64(4 << 20)
			encoding := ""
			var wantErr error
			switch name {
			case "unknown_length_overload":
				budget.limit = 4096
				payload = bytes.Repeat([]byte{'x'}, 8192)
				wantErr = ErrRequestBodyMemoryBudget
			case "concat_overload":
				budget.limit = 20000
				payload = bytes.Repeat([]byte{'x'}, 8192)
				wantErr = ErrRequestBodyMemoryBudget
			case "normalization_overload":
				budget.limit = 600
				payload = []byte(`{"input":"` + strings.Repeat("\x00", 100) + `"}`)
				wantErr = ErrRequestBodyMemoryBudget
			case "normalization_size":
				maxNormalized = 64
			case "invalid_gzip":
				encoding = "gzip"
			case "invalid_zstd":
				encoding = "zstd"
			case "read_error":
				wantErr = io.ErrUnexpectedEOF
			case "cancellation":
				wantErr = context.Canceled
			}
			req := newRequestWithBody(t, payload, encoding).WithContext(ctx)
			req.ContentLength = -1
			if name == "normalization_overload" {
				req.ContentLength = int64(len(payload))
			}
			if name == "read_error" {
				req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(payload), requestBodyBudgetReaderFunc(func([]byte) (int, error) {
					return 0, io.ErrUnexpectedEOF
				})))
			}
			if name == "cancellation" {
				reader := bytes.NewReader(payload)
				req.Body = io.NopCloser(requestBodyBudgetReaderFunc(func(p []byte) (int, error) {
					n, err := reader.Read(p)
					cancel()
					return n, err
				}))
			}
			got, err := ReadLenientJSONRequestBodyWithPrealloc(req, maxNormalized)
			if got != nil || err == nil || wantErr != nil && !errors.Is(err, wantErr) {
				t.Fatalf("expected failed read (%v), got len=%d err=%v", wantErr, len(got), err)
			}
			if used := requestBodyBudgetUsed(budget); used != 100 {
				t.Fatalf("failed read retained temporary storage or refunded another owner: %d", used)
			}
		})
	}
}

func TestRequestBodyMemoryBudgetPreservesBorrowedPrereadAllocation(t *testing.T) {
	budget := NewBodyMemoryBudget(4096)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	payload := []byte(`{"input":"hello` + "\x00" + `"}`)
	req := newRequestWithBody(t, payload, "").WithContext(ctx)
	original, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Body = NewPrereadBody(original)
	if _, err := ReadLenientJSONRequestBodyWithPrealloc(req, 1); err == nil {
		t.Fatal("oversize preread body was accepted")
	}
	if used := requestBodyBudgetUsed(budget); used != int64(cap(original)) {
		t.Fatalf("failed normalization refunded borrowed storage: %d", used)
	}
	normalized, err := ReadLenientJSONRequestBodyWithPrealloc(req, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if used := requestBodyBudgetUsed(budget); used != int64(cap(original)+cap(normalized)) {
		t.Fatalf("original and normalized bodies remain live but only %d bytes are charged", used)
	}
	again, err := ReadRequestBodyWithPrealloc(req)
	if err != nil || !bytes.Equal(again, payload) || &again[0] != &original[0] {
		t.Fatal("normalization changed the borrowed preread body")
	}
	if used := requestBodyBudgetUsed(budget); used != int64(cap(original)+cap(normalized)) {
		t.Fatalf("preread fast path changed the budget: %d", used)
	}
}
