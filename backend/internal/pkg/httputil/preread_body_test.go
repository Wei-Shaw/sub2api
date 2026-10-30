package httputil

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPrereadBodyRoundtrip(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4"}`)
	preread := NewPrereadBody(body)

	got, err := io.ReadAll(preread)
	if err != nil {
		t.Fatalf("read preread body: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body mismatch: got %q want %q", got, body)
	}
	if err := preread.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !bytes.Equal(preread.Bytes(), body) {
		t.Fatalf("Bytes() mismatch after close: %q", preread.Bytes())
	}
}

func TestPrereadBodyReadableTwiceFromFreshWrapper(t *testing.T) {
	// ResetRequestBody 每次都回填新的 PrereadBody；同一实例只被顺序消费一次。
	body := []byte("payload")
	first := NewPrereadBody(body)
	got1, _ := io.ReadAll(first)
	if !bytes.Equal(got1, body) {
		t.Fatalf("first read mismatch: %q", got1)
	}

	second := NewPrereadBody(first.Bytes())
	got2, _ := io.ReadAll(second)
	if !bytes.Equal(got2, body) {
		t.Fatalf("second read mismatch: %q", got2)
	}
}

func TestReadRequestBodyWithPreallocReturnsPrereadBodySliceWithoutCopy(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4.5"}`)
	req, err := http.NewRequest(http.MethodPost, "/v1/messages", NewPrereadBody(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Length", "0") // 故意与实际不符，验证不依赖 ContentLength

	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("ReadRequestBodyWithPrealloc: %v", err)
	}
	if &got[0] != &body[0] {
		t.Fatalf("expected zero-copy return of the same slice")
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body mismatch: got %q want %q", got, body)
	}
}

func TestReadRequestBodyWithPreallocHandlesNilPrereadBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/messages", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil || got != nil {
		t.Fatalf("expected nil body, got (%#v, %v)", got, err)
	}
}

func TestPrereadBodyPartialConsumeThenPreallocReturnsFullBody(t *testing.T) {
	body := []byte(strings.Repeat("x", 64))
	preread := NewPrereadBody(body)

	head := make([]byte, 16)
	if _, err := io.ReadFull(preread, head); err != nil {
		t.Fatalf("partial read: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "/v1/responses", preread)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("ReadRequestBodyWithPrealloc: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("expected full body after partial consume, got %d bytes", len(got))
	}
}
