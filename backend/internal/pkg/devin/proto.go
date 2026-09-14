// proto.go 提供 Devin Connect wire 类型所需的最小 protobuf 编解码。
//
// 字段编号与编码形态逐字节对齐 devin-connect 插件的 proto.ts（后者按
// devin 3000.10.21 真实抓包验证），不引入 protoc 生成代码——上游没有
// 公开的 .proto schema，手写编解码即权威实现。
package devin

import (
	"encoding/binary"
	"errors"
	"math"
)

// Protobuf wire types.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

// appendVarint 追加无符号 varint 编码。
func appendVarint(dst []byte, value uint64) []byte {
	for value > 0x7f {
		dst = append(dst, byte(value&0x7f)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

// appendTag 追加 field tag。
func appendTag(dst []byte, field int, wire int) []byte {
	return appendVarint(dst, uint64(field<<3|wire))
}

// appendString 追加 string 字段（tag + len + utf8 字节）。
func appendString(dst []byte, field int, value string) []byte {
	dst = appendTag(dst, field, wireBytes)
	dst = appendVarint(dst, uint64(len(value)))
	return append(dst, value...)
}

// appendBytes 追加 bytes 字段。
func appendBytes(dst []byte, field int, value []byte) []byte {
	dst = appendTag(dst, field, wireBytes)
	dst = appendVarint(dst, uint64(len(value)))
	return append(dst, value...)
}

// appendMessage 追加嵌套 message 字段（与 bytes 同形）。
func appendMessage(dst []byte, field int, body []byte) []byte {
	return appendBytes(dst, field, body)
}

// appendVarintField 追加 varint 字段（int*/uint*/enum）。
func appendVarintField(dst []byte, field int, value uint64) []byte {
	dst = appendTag(dst, field, wireVarint)
	return appendVarint(dst, value)
}

// appendBool 追加 bool 字段。
func appendBool(dst []byte, field int, value bool) []byte {
	if value {
		return appendVarintField(dst, field, 1)
	}
	return appendVarintField(dst, field, 0)
}

// appendDouble 追加 double（fixed64）字段。
func appendDouble(dst []byte, field int, value float64) []byte {
	dst = appendTag(dst, field, wireFixed64)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
	return append(dst, buf[:]...)
}

// AppendTestString 暴露 string 字段编码，仅供外部包测试构造桩响应。
func AppendTestString(dst []byte, field int, value string) []byte {
	return appendString(dst, field, value)
}

// protoField 是解码迭代产生的一个字段。
type protoField struct {
	num    int
	wire   int
	vuint  uint64 // wire==0
	vbytes []byte // wire==1/2/5
}

// decodeVarint 解码一个 varint，返回数值与消费后的偏移。
func decodeVarint(buf []byte, offset int) (uint64, int, error) {
	var result uint64
	var shift uint
	for i := offset; i < len(buf); i++ {
		b := buf[i]
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, i + 1, nil
		}
		shift += 7
		if shift > 63 {
			return 0, 0, errors.New("varint overflow")
		}
	}
	return 0, 0, errors.New("truncated varint")
}

// iterFields 逐字段迭代 protobuf 消息体。遇到损坏字段即停止
// （与插件 iterFields 一致：宽容截断，不报错）。
func iterFields(buf []byte, yield func(protoField) bool) {
	i := 0
	for i < len(buf) {
		tag, next, err := decodeVarint(buf, i)
		if err != nil {
			return
		}
		i = next
		num := int(tag >> 3)
		wire := int(tag & 0x7)
		switch wire {
		case wireVarint:
			v, after, err := decodeVarint(buf, i)
			if err != nil {
				return
			}
			i = after
			if !yield(protoField{num: num, wire: wire, vuint: v}) {
				return
			}
		case wireFixed64:
			if i+8 > len(buf) {
				return
			}
			if !yield(protoField{num: num, wire: wire, vbytes: buf[i : i+8]}) {
				return
			}
			i += 8
		case wireBytes:
			length, after, err := decodeVarint(buf, i)
			if err != nil {
				return
			}
			i = after
			end := i + int(length)
			if end > len(buf) {
				return
			}
			if !yield(protoField{num: num, wire: wire, vbytes: buf[i:end]}) {
				return
			}
			i = end
		case wireFixed32:
			if i+4 > len(buf) {
				return
			}
			if !yield(protoField{num: num, wire: wire, vbytes: buf[i : i+4]}) {
				return
			}
			i += 4
		default:
			return
		}
	}
}

// collectFields 把整个消息体收进切片（测试与短消息解析用）。
func collectFields(buf []byte) []protoField {
	var out []protoField
	iterFields(buf, func(f protoField) bool {
		out = append(out, f)
		return true
	})
	return out
}

func (f protoField) string() string {
	if f.wire == wireBytes {
		return string(f.vbytes)
	}
	return ""
}

func (f protoField) bool() bool {
	return f.wire == wireVarint && f.vuint != 0
}

func (f protoField) int() int64 {
	if f.wire == wireVarint {
		return int64(f.vuint)
	}
	return 0
}

// signedInt 把 int32/int64 字段按二进制补码解码——上游 -1（unlimited
// 哨兵）在 wire 上是 2^64-1，直接 uint64 会溢出成正数。
func (f protoField) signedInt() (int64, bool) {
	if f.wire != wireVarint {
		return 0, false
	}
	return int64(f.vuint), true
}

// double 解码 fixed64 字段为 float64。
func (f protoField) double() (float64, bool) {
	if f.wire == wireFixed64 && len(f.vbytes) == 8 {
		return math.Float64frombits(binary.LittleEndian.Uint64(f.vbytes)), true
	}
	return 0, false
}

func (f protoField) bytes() []byte {
	if f.wire == wireBytes {
		return f.vbytes
	}
	return nil
}

// fieldString 在顶层迭代中按字段号取第一个 string 值。
func fieldString(buf []byte, num int) string {
	var out string
	iterFields(buf, func(f protoField) bool {
		if f.num == num {
			out = f.string()
			return false
		}
		return true
	})
	return out
}

// marshalStringFields 是测试辅助：把 field->string 序列化为消息体。
func marshalStringFields(fields map[int]string) []byte {
	var buf []byte
	for num, value := range fields {
		buf = appendString(buf, num, value)
	}
	return buf
}
