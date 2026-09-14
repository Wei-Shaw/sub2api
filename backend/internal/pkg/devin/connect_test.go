// connect_test.go 验证 Connect envelope 帧、帧读取器与错误解析。
package devin

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestEncodeEnvelopeGolden(t *testing.T) {
	got := encodeEnvelope([]byte("ab"), 0x00)
	want := []byte{0x00, 0x00, 0x00, 0x00, 0x02, 'a', 'b'}
	if !bytes.Equal(got, want) {
		t.Fatalf("envelope = %x, want %x", got, want)
	}
}

func TestConnectFrameReader(t *testing.T) {
	var stream []byte
	stream = append(stream, encodeEnvelope([]byte("frame1"), 0x00)...)
	stream = append(stream, encodeEnvelope([]byte(`{"error":{"code":"unavailable","message":"nope"}}`), 0x02)...)
	r := NewConnectFrameReader(bytes.NewReader(stream))
	var got []StreamFrame
	for r.Next() {
		got = append(got, r.Frame())
	}
	if err := r.Err(); err != nil {
		t.Fatalf("reader err = %v", err)
	}
	if len(got) != 2 || string(got[0].Payload) != "frame1" || !got[1].End {
		t.Fatalf("frames = %+v", got)
	}
	err := ParseEndTrailer(got[1].Payload)
	var connectErr *ConnectError
	if err == nil || !errors.As(err, &connectErr) || connectErr.Code != "unavailable" {
		t.Fatalf("end trailer err = %v", err)
	}
}

func TestConnectFrameReaderTruncated(t *testing.T) {
	r := NewConnectFrameReader(bytes.NewReader([]byte{0x00, 0x00, 0x00}))
	if r.Next() {
		t.Fatalf("expected no frame")
	}
	if r.Err() == nil {
		t.Fatalf("expected truncation error")
	}
}

func TestReadUnaryResponseError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 401,
		Body:       io.NopCloser(strings.NewReader(`{"code":"unauthenticated","message":"bad key"}`)),
	}
	_, err := ReadUnaryResponse(resp)
	if !IsUnauthenticated(err) {
		t.Fatalf("err = %v, want unauthenticated", err)
	}
}

func TestIsTransientTransportError(t *testing.T) {
	if IsTransientTransportError(&ConnectError{Code: "unavailable"}) {
		t.Fatalf("connect error must not be transient")
	}
	if !IsTransientTransportError(io.ErrUnexpectedEOF) {
		t.Fatalf("EOF should be transient")
	}
}

func TestBuildMetadataShape(t *testing.T) {
	md := BuildMetadata("tok", "3000.2.17", "mac", false)
	if fieldStr(md, 1) != ClientName || fieldStr(md, 3) != "tok" || fieldStr(md, 5) != "mac" {
		t.Fatalf("metadata fields wrong")
	}
	if fieldStr(md, 2) != "3000.2.17" || fieldStr(md, 7) != "3000.2.17" {
		t.Fatalf("version fields wrong")
	}
	// 非 catalog 请求不带 f30 displays；f31 是 366 字节随机 hex。
	if hasField(md, 30) {
		t.Fatalf("chat metadata must not carry displays field")
	}
	if f := fieldStr(md, 31); len(f) != 732 {
		t.Fatalf("fingerprint len = %d, want 732", len(f))
	}
	md = BuildMetadata("tok", "3000.2.17", "mac", true)
	if !hasField(md, 30) {
		t.Fatalf("catalog metadata must carry displays field")
	}
}

func TestNormalizeToken(t *testing.T) {
	if got := NormalizeToken("eyJabc.def.ghi"); !strings.HasPrefix(got, "devin-session-token$") {
		t.Fatalf("JWT must gain devin-session-token$ prefix, got %q", got)
	}
	if got := NormalizeToken("devin-session-token$eyJx"); got != "devin-session-token$eyJx" {
		t.Fatalf("already-prefixed token must pass through")
	}
	if got := NormalizeToken("  \n tok \n"); got != "tok" {
		t.Fatalf("whitespace must be trimmed, got %q", got)
	}
}
