package cursor

import (
	"encoding/binary"
	"fmt"
)

// Wire types for protobuf encoding.
const (
	WireVarint  = 0
	WireFixed64 = 1
	WireBytes   = 2
	WireFixed32 = 5
)

// ProtobufWriter accumulates hand-encoded protobuf fields.
type ProtobufWriter struct {
	buf []byte
}

func (w *ProtobufWriter) Result() []byte { return w.buf }

func (w *ProtobufWriter) appendVarint(v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	w.buf = append(w.buf, tmp[:n]...)
}

func (w *ProtobufWriter) appendTag(field uint32, wireType int) {
	w.appendVarint(uint64(field)<<3 | uint64(wireType))
}

// Varint writes field = varint value.
func (w *ProtobufWriter) Varint(field uint32, v int) {
	w.appendTag(field, WireVarint)
	w.appendVarint(uint64(v))
}

// Bool writes field = bool as varint.
func (w *ProtobufWriter) Bool(field uint32, v bool) {
	val := 0
	if v {
		val = 1
	}
	w.Varint(field, val)
}

// String writes field = length-delimited string.
func (w *ProtobufWriter) String(field uint32, v string) {
	w.appendTag(field, WireBytes)
	w.appendVarint(uint64(len(v)))
	w.buf = append(w.buf, v...)
}

// Bytes writes field = length-delimited bytes (or nested message).
func (w *ProtobufWriter) Bytes(field uint32, v []byte) {
	w.appendTag(field, WireBytes)
	w.appendVarint(uint64(len(v)))
	w.buf = append(w.buf, v...)
}

// ProtobufReader provides forward-only parsing of protobuf wire format.
type ProtobufReader struct {
	data []byte
	pos  int
}

func NewProtobufReader(data []byte) *ProtobufReader {
	return &ProtobufReader{data: data}
}

func (r *ProtobufReader) Done() bool { return r.pos >= len(r.data) }

// Field represents a decoded protobuf field.
type Field struct {
	Num      uint32
	WireType int
	Varint   uint64
	Data     []byte // for WireBytes
}

// Next reads the next field. Returns nil when done.
func (r *ProtobufReader) Next() (*Field, error) {
	if r.Done() {
		return nil, nil
	}
	tag, err := r.readVarint()
	if err != nil {
		return nil, err
	}
	f := &Field{
		Num:      uint32(tag >> 3),
		WireType: int(tag & 0x07),
	}
	switch f.WireType {
	case WireVarint:
		f.Varint, err = r.readVarint()
	case WireFixed64:
		if r.pos+8 > len(r.data) {
			return nil, fmt.Errorf("protobuf: short fixed64")
		}
		f.Varint = binary.LittleEndian.Uint64(r.data[r.pos : r.pos+8])
		r.pos += 8
	case WireBytes:
		length, err2 := r.readVarint()
		if err2 != nil {
			return nil, err2
		}
		if r.pos+int(length) > len(r.data) {
			return nil, fmt.Errorf("protobuf: short bytes field %d", f.Num)
		}
		f.Data = r.data[r.pos : r.pos+int(length)]
		r.pos += int(length)
	case WireFixed32:
		if r.pos+4 > len(r.data) {
			return nil, fmt.Errorf("protobuf: short fixed32")
		}
		f.Varint = uint64(binary.LittleEndian.Uint32(r.data[r.pos : r.pos+4]))
		r.pos += 4
	default:
		return nil, fmt.Errorf("protobuf: unknown wire type %d", f.WireType)
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *ProtobufReader) readVarint() (uint64, error) {
	var val uint64
	var shift uint
	for {
		if r.pos >= len(r.data) {
			return 0, fmt.Errorf("protobuf: unexpected EOF in varint")
		}
		b := r.data[r.pos]
		r.pos++
		val |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("protobuf: varint overflow")
		}
	}
	return val, nil
}

// GetString extracts the first string value for the given field number from raw protobuf bytes.
func GetString(data []byte, fieldNum uint32) string {
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			return ""
		}
		if f.Num == fieldNum && f.WireType == WireBytes {
			return string(f.Data)
		}
	}
}

// GetNested extracts the first nested message bytes for the given field number.
func GetNested(data []byte, fieldNum uint32) []byte {
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			return nil
		}
		if f.Num == fieldNum && f.WireType == WireBytes {
			return f.Data
		}
	}
}
