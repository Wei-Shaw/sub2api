// Adapted from Oh My Pi (MIT), commit 4049c62fbb0a7f5068954d01f11a73112c942a11,
// packages/catalog/src/discovery/devin-proto.ts and packages/catalog/src/wire/devin*.ts.
// See the repository root THIRD_PARTY_NOTICES.md for license and attribution.
package devin

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"runtime"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

const maxPayload = 16 * 1024 * 1024

// A byte limit alone does not bound decoded memory: two-byte repeated scalar
// fields expand into much larger Go structs/slices. No supported Devin message
// needs this many fields at one nesting level (tool calls are separately capped).
const maxProtoFields = 64 * 1024

type protoField struct {
	typ protowire.Type
	b   []byte
	n   uint64
}
type protoMessage map[protowire.Number][]protoField

func parseProto(b []byte) (protoMessage, error) {
	m := make(protoMessage)
	fields := 0
	for len(b) > 0 {
		if fields >= maxProtoFields {
			return nil, protocolError()
		}
		fields++
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 || num < 1 {
			return nil, protocolError()
		}
		b = b[n:]
		f := protoField{typ: typ}
		switch typ {
		case protowire.BytesType:
			f.b, n = protowire.ConsumeBytes(b)
		case protowire.VarintType:
			f.n, n = protowire.ConsumeVarint(b)
		case protowire.Fixed32Type:
			var v uint32
			v, n = protowire.ConsumeFixed32(b)
			f.n = uint64(v)
		case protowire.Fixed64Type:
			f.n, n = protowire.ConsumeFixed64(b)
		default:
			return nil, protocolError()
		}
		if n < 0 {
			return nil, protocolError()
		}
		b = b[n:]
		m[num] = append(m[num], f)
	}
	return m, nil
}
func (m protoMessage) data(n protowire.Number) []byte {
	fs := m[n]
	if len(fs) == 0 {
		return nil
	}
	return fs[len(fs)-1].b
}
func (m protoMessage) text(n protowire.Number) string { return string(m.data(n)) }
func (m protoMessage) num(n protowire.Number) uint64 {
	fs := m[n]
	if len(fs) == 0 {
		return 0
	}
	return fs[len(fs)-1].n
}
func (m protoMessage) check(bytesFields, intFields []int32) error {
	for _, n := range bytesFields {
		for _, f := range m[protowire.Number(n)] {
			if f.typ != protowire.BytesType {
				return protocolError()
			}
		}
	}
	for _, n := range intFields {
		for _, f := range m[protowire.Number(n)] {
			if f.typ != protowire.VarintType {
				return protocolError()
			}
		}
	}
	return nil
}
func pb(b []byte, n protowire.Number, v []byte) []byte {
	b = protowire.AppendTag(b, n, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}
func ps(b []byte, n protowire.Number, v string) []byte {
	if v == "" {
		return b
	}
	return pb(b, n, []byte(v))
}
func pv(b []byte, n protowire.Number, v uint64) []byte {
	if v == 0 {
		return b
	}
	b = protowire.AppendTag(b, n, protowire.VarintType)
	return protowire.AppendVarint(b, v)
}
func pd(b []byte, n protowire.Number, v float64) []byte {
	if v == 0 {
		return b
	}
	b = protowire.AppendTag(b, n, protowire.Fixed64Type)
	return protowire.AppendFixed64(b, math.Float64bits(v))
}
func metadata(token, jwt string, discovery bool) []byte {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "devin-session-token$") {
		token = "devin-session-token$" + token
	}
	name, version := "devin-cli", "3000.6.2"
	if discovery {
		name, version = "chisel", "0.0.0-dev"
	}
	var b []byte
	b = ps(b, 1, name)
	b = ps(b, 7, version)
	b = ps(b, 12, "chisel")
	b = ps(b, 2, version)
	b = ps(b, 3, token)
	b = ps(b, 4, "en")
	os := runtime.GOOS
	if os != "darwin" && os != "windows" {
		os = "linux"
	}
	b = ps(b, 5, os)
	if !discovery {
		b = ps(b, 28, "chisel")
		b = ps(b, 21, jwt)
	} else {
		// Repeated enum values are packed, as in the native protobuf encoder.
		b = pb(b, 30, []byte{3, 4, 6, 7, 8})
	}
	return b
}
func boundedRead(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxPayload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxPayload {
		return nil, protocolError()
	}
	return b, nil
}
func inflate(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, protocolError()
	}
	defer r.Close()
	out, err := boundedRead(r)
	if err != nil {
		return nil, protocolError()
	}
	return out, nil
}
func decodeUnary(b []byte) (protoMessage, error) {
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		var err error
		b, err = inflate(b)
		if err != nil {
			return nil, err
		}
	}
	return parseProto(b)
}
func gzipFrame(b []byte) ([]byte, error) {
	if len(b) > maxPayload {
		return nil, inputError("Request exceeds Devin frame limit")
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if compressed.Len() > maxPayload {
		return nil, inputError("Request exceeds Devin frame limit")
	}
	return connectFrame(1, compressed.Bytes()), nil
}
func connectFrame(flag byte, b []byte) []byte {
	out := make([]byte, 5+len(b))
	out[0] = flag
	binary.BigEndian.PutUint32(out[1:5], uint32(len(b)))
	copy(out[5:], b)
	return out
}

type connectReader struct {
	r        io.Reader
	terminal bool
}

// next requires an explicit, valid successful terminal trailer. EOF is never success.
func (r *connectReader) next() (protoMessage, error) {
	if r.terminal {
		return nil, io.EOF
	}
	var header [5]byte
	if _, err := io.ReadFull(r.r, header[:]); err != nil {
		return nil, protocolError()
	}
	flag := header[0]
	size := binary.BigEndian.Uint32(header[1:])
	if flag & ^byte(3) != 0 || size > maxPayload {
		return nil, protocolError()
	}
	b := make([]byte, int(size))
	if _, err := io.ReadFull(r.r, b); err != nil {
		return nil, protocolError()
	}
	if flag&1 != 0 {
		var err error
		b, err = inflate(b)
		if err != nil {
			return nil, err
		}
	}
	if flag&2 != 0 {
		var trailer struct {
			Error    json.RawMessage `json:"error"`
			Metadata json.RawMessage `json:"metadata"`
		}
		if len(bytes.TrimSpace(b)) == 0 || bytes.TrimSpace(b)[0] != '{' || json.Unmarshal(b, &trailer) != nil {
			return nil, protocolError()
		}
		if len(trailer.Error) > 0 && string(trailer.Error) != "null" {
			var e struct {
				Code string `json:"code"`
			}
			if json.Unmarshal(trailer.Error, &e) != nil {
				return nil, protocolError()
			}
			return nil, connectError(e.Code)
		}
		// The trailer must actually be terminal. Reject extra frames/garbage
		// instead of reporting success for a truncated suffix.
		var extra [1]byte
		if n, err := io.ReadFull(r.r, extra[:]); n != 0 || err != io.EOF {
			return nil, protocolError()
		}
		r.terminal = true
		return nil, io.EOF
	}
	return parseProto(b)
}
