// proto_test.go 验证手写 protobuf 编码器的字节形态。
package devin

import (
	"bytes"
	"testing"
)

func TestAppendVarint(t *testing.T) {
	cases := []struct {
		v    uint64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x01}},
		{300, []byte{0xac, 0x02}},
		{1 << 63, []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}},
	}
	for _, c := range cases {
		if got := appendVarint(nil, c.v); !bytes.Equal(got, c.want) {
			t.Fatalf("appendVarint(%d) = %x, want %x", c.v, got, c.want)
		}
	}
}

func TestTagAndStringGolden(t *testing.T) {
	// field 1 varint 5 → 0x08 0x05
	got := appendVarintField(nil, 1, 5)
	if want := []byte{0x08, 0x05}; !bytes.Equal(got, want) {
		t.Fatalf("field(1,varint,5) = %x, want %x", got, want)
	}
	// field 2 len-delimited "hi" → 0x12 0x02 'h' 'i'
	got = appendString(nil, 2, "hi")
	if want := []byte{0x12, 0x02, 'h', 'i'}; !bytes.Equal(got, want) {
		t.Fatalf("field(2,\"hi\") = %x, want %x", got, want)
	}
	// field 15 message {1:"x"} → 0x7a len 0x0a 0x01 'x'
	got = appendMessage(nil, 15, []byte{0x0a, 0x01, 'x'})
	if want := []byte{0x7a, 0x03, 0x0a, 0x01, 'x'}; !bytes.Equal(got, want) {
		t.Fatalf("field(15,msg) = %x, want %x", got, want)
	}
}

func TestIterFieldsRoundtrip(t *testing.T) {
	var msg []byte
	msg = appendVarintField(msg, 1, 42)
	msg = appendString(msg, 3, "hello")
	msg = appendMessage(msg, 15, appendVarintField(nil, 2, 7))
	var gotV uint64
	var gotS, gotInner string
	var innerV uint64
	n := 0
	iterFields(msg, func(f protoField) bool {
		n++
		switch f.num {
		case 1:
			gotV = f.vuint
		case 3:
			gotS = f.string()
		case 15:
			iterFields(f.bytes(), func(inner protoField) bool {
				if inner.num == 2 {
					innerV = inner.vuint
					gotInner = "seen"
				}
				return true
			})
		}
		return true
	})
	if n != 3 || gotV != 42 || gotS != "hello" || innerV != 7 || gotInner != "seen" {
		t.Fatalf("roundtrip mismatch: n=%d v=%d s=%q inner=%d %q", n, gotV, gotS, innerV, gotInner)
	}
}

func TestDoubleField(t *testing.T) {
	got := appendDouble(nil, 5, 1.0)
	// field 5 fixed64: tag = 5<<3|1 = 0x29, payload 1.0 little-endian
	want := append([]byte{0x29}, []byte{0, 0, 0, 0, 0, 0, 0xf0, 0x3f}...)
	if !bytes.Equal(got, want) {
		t.Fatalf("double(5,1.0) = %x, want %x", got, want)
	}
}
