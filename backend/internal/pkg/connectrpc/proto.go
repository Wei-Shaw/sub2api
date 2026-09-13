package connectrpc

import (
	"google.golang.org/protobuf/encoding/protowire"
)

func AppendString(buf []byte, num protowire.Number, value string) []byte {
	if value == "" {
		return buf
	}
	buf = protowire.AppendTag(buf, num, protowire.BytesType)
	return protowire.AppendString(buf, value)
}

func AppendBytes(buf []byte, num protowire.Number, value []byte) []byte {
	if len(value) == 0 {
		return buf
	}
	buf = protowire.AppendTag(buf, num, protowire.BytesType)
	return protowire.AppendBytes(buf, value)
}

func AppendVarint(buf []byte, num protowire.Number, value uint64) []byte {
	if value == 0 {
		return buf
	}
	buf = protowire.AppendTag(buf, num, protowire.VarintType)
	return protowire.AppendVarint(buf, value)
}

func AppendBool(buf []byte, num protowire.Number, value bool) []byte {
	if !value {
		return buf
	}
	return AppendVarint(buf, num, 1)
}

func AppendMessage(buf []byte, num protowire.Number, message []byte) []byte {
	return AppendBytes(buf, num, message)
}

func Consume(buf []byte) (num protowire.Number, typ protowire.Type, value []byte, rest []byte, ok bool) {
	num, typ, n := protowire.ConsumeTag(buf)
	if n < 0 {
		return 0, 0, nil, buf, false
	}
	buf = buf[n:]
	switch typ {
	case protowire.VarintType:
		v, n := protowire.ConsumeVarint(buf)
		if n < 0 {
			return 0, 0, nil, buf, false
		}
		return num, typ, protowire.AppendVarint(nil, v), buf[n:], true
	case protowire.BytesType:
		v, n := protowire.ConsumeBytes(buf)
		if n < 0 {
			return 0, 0, nil, buf, false
		}
		return num, typ, v, buf[n:], true
	case protowire.Fixed32Type:
		v, n := protowire.ConsumeFixed32(buf)
		if n < 0 {
			return 0, 0, nil, buf, false
		}
		out := make([]byte, 4)
		protowire.AppendFixed32(out[:0], v)
		return num, typ, out, buf[n:], true
	case protowire.Fixed64Type:
		v, n := protowire.ConsumeFixed64(buf)
		if n < 0 {
			return 0, 0, nil, buf, false
		}
		out := make([]byte, 8)
		protowire.AppendFixed64(out[:0], v)
		return num, typ, out, buf[n:], true
	default:
		n := protowire.ConsumeFieldValue(num, typ, buf)
		if n < 0 {
			return 0, 0, nil, buf, false
		}
		return num, typ, buf[:n], buf[n:], true
	}
}

func ConsumeString(value []byte) string {
	return string(value)
}

func ConsumeVarint(value []byte) uint64 {
	v, _ := protowire.ConsumeVarint(value)
	return v
}
