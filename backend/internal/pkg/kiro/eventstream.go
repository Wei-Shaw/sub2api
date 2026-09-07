// Package kiro 实现 Kiro（Amazon Q Developer / AWS CodeWhisperer 后端）的
// 线协议：AWS event-stream 解码，以及 Anthropic ⇄ Kiro 的双向转换。
//
// 本包是纯函数层：不触碰数据库、缓存或 HTTP 客户端，全部逻辑可单测。
package kiro

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

const (
	// preludeSize 是帧前导的固定长度：TotalLen + HeaderLen + PreludeCRC。
	preludeSize = 12
	// minMessageSize 是一个合法帧的最小长度：前导 + 消息 CRC。
	minMessageSize = preludeSize + 4
	// maxMessageSize 与上游一致，防止畸形 TotalLen 触发巨额分配。
	maxMessageSize = 16 * 1024 * 1024
)

var (
	// ErrFrameTooLarge 表示 TotalLen 超过 maxMessageSize。
	ErrFrameTooLarge = errors.New("kiro: event-stream frame exceeds size limit")
	// ErrCRCMismatch 表示前导或整帧的 CRC32 校验失败。
	ErrCRCMismatch = errors.New("kiro: event-stream CRC mismatch")
	// ErrMalformedFrame 表示帧结构不自洽（长度越界、header 截断等）。
	ErrMalformedFrame = errors.New("kiro: malformed event-stream frame")
)

// Header 值类型标识，取值见 AWS event-stream 规范。
const (
	hdrBoolTrue  uint8 = 0
	hdrBoolFalse uint8 = 1
	hdrByte      uint8 = 2
	hdrShort     uint8 = 3
	hdrInteger   uint8 = 4
	hdrLong      uint8 = 5
	hdrByteArray uint8 = 6
	hdrString    uint8 = 7
	hdrTimestamp uint8 = 8
	hdrUUID      uint8 = 9
)

// HeaderValue 是一条 event-stream header 的值。
// Type 决定哪个字段有效，未使用的字段保持零值。
type HeaderValue struct {
	Type  uint8
	Str   string
	Bytes []byte
	Int   int64
	Bool  bool
	UUID  [16]byte
}

// Frame 是一个解码后的 event-stream 帧。
type Frame struct {
	Headers map[string]HeaderValue
	Payload []byte
}

// MessageType 返回 :message-type header（通常是 "event" 或 "exception"）。
func (f *Frame) MessageType() string { return f.Headers[":message-type"].Str }

// EventType 返回 :event-type header（如 "assistantResponseEvent"）。
func (f *Frame) EventType() string { return f.Headers[":event-type"].Str }

// ExceptionType 返回 :exception-type header，仅异常帧携带。
func (f *Frame) ExceptionType() string { return f.Headers[":exception-type"].Str }

// ParseFrame 尝试从 buf 头部解析出一个完整帧。
//
// 返回值语义：
//   - (frame, n, nil)  成功，消费了 n 字节
//   - (nil, 0, nil)    数据不足，调用方应等待更多字节
//   - (nil, 0, err)    帧损坏，调用方应终止该流
func ParseFrame(buf []byte) (*Frame, int, error) {
	if len(buf) < preludeSize {
		return nil, 0, nil
	}

	total := binary.BigEndian.Uint32(buf[0:4])
	headerLen := binary.BigEndian.Uint32(buf[4:8])
	preludeCRC := binary.BigEndian.Uint32(buf[8:12])

	if crc32.ChecksumIEEE(buf[0:8]) != preludeCRC {
		return nil, 0, fmt.Errorf("%w: prelude", ErrCRCMismatch)
	}
	if total > maxMessageSize {
		return nil, 0, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, total)
	}
	if total < minMessageSize {
		return nil, 0, fmt.Errorf("%w: total length %d below minimum", ErrMalformedFrame, total)
	}
	if uint64(headerLen)+uint64(minMessageSize) > uint64(total) {
		return nil, 0, fmt.Errorf("%w: header length %d overflows frame", ErrMalformedFrame, headerLen)
	}
	if uint64(len(buf)) < uint64(total) {
		return nil, 0, nil
	}

	msg := buf[:total]
	wantCRC := binary.BigEndian.Uint32(msg[total-4:])
	if crc32.ChecksumIEEE(msg[:total-4]) != wantCRC {
		return nil, 0, fmt.Errorf("%w: message", ErrCRCMismatch)
	}

	headers, err := parseHeaders(msg[preludeSize : preludeSize+headerLen])
	if err != nil {
		return nil, 0, err
	}

	payload := make([]byte, total-4-preludeSize-headerLen)
	copy(payload, msg[preludeSize+headerLen:total-4])

	return &Frame{Headers: headers, Payload: payload}, int(total), nil
}

// parseHeaders 解码 header 区。每条 header 的编码为
// [nameLen u8][name][valueType u8][value]。
func parseHeaders(buf []byte) (map[string]HeaderValue, error) {
	headers := make(map[string]HeaderValue)
	pos := 0

	need := func(n int) error {
		if pos+n > len(buf) {
			return fmt.Errorf("%w: header truncated", ErrMalformedFrame)
		}
		return nil
	}

	for pos < len(buf) {
		if err := need(1); err != nil {
			return nil, err
		}
		nameLen := int(buf[pos])
		pos++

		if err := need(nameLen); err != nil {
			return nil, err
		}
		name := string(buf[pos : pos+nameLen])
		pos += nameLen

		if err := need(1); err != nil {
			return nil, err
		}
		valType := buf[pos]
		pos++

		val := HeaderValue{Type: valType}
		switch valType {
		case hdrBoolTrue:
			val.Bool = true
		case hdrBoolFalse:
			val.Bool = false
		case hdrByte:
			if err := need(1); err != nil {
				return nil, err
			}
			val.Int = int64(int8(buf[pos]))
			pos++
		case hdrShort:
			if err := need(2); err != nil {
				return nil, err
			}
			val.Int = int64(int16(binary.BigEndian.Uint16(buf[pos : pos+2])))
			pos += 2
		case hdrInteger:
			if err := need(4); err != nil {
				return nil, err
			}
			val.Int = int64(int32(binary.BigEndian.Uint32(buf[pos : pos+4])))
			pos += 4
		case hdrLong, hdrTimestamp:
			if err := need(8); err != nil {
				return nil, err
			}
			val.Int = int64(binary.BigEndian.Uint64(buf[pos : pos+8]))
			pos += 8
		case hdrByteArray, hdrString:
			if err := need(2); err != nil {
				return nil, err
			}
			n := int(binary.BigEndian.Uint16(buf[pos : pos+2]))
			pos += 2
			if err := need(n); err != nil {
				return nil, err
			}
			if valType == hdrString {
				val.Str = string(buf[pos : pos+n])
			} else {
				val.Bytes = append([]byte(nil), buf[pos:pos+n]...)
			}
			pos += n
		case hdrUUID:
			if err := need(16); err != nil {
				return nil, err
			}
			copy(val.UUID[:], buf[pos:pos+16])
			pos += 16
		default:
			return nil, fmt.Errorf("%w: unknown header value type %d", ErrMalformedFrame, valType)
		}

		headers[name] = val
	}

	return headers, nil
}

// Decoder 是有状态的流式解码器：把任意切分的字节块累积成完整帧。
// 非并发安全，每个上游响应一个实例。
type Decoder struct {
	buf []byte
}

// NewDecoder 创建一个解码器。
func NewDecoder() *Decoder { return &Decoder{} }

// Feed 追加一段字节并返回其中已完整的帧。
// 返回错误后该 Decoder 不应继续使用。
func (d *Decoder) Feed(chunk []byte) ([]*Frame, error) {
	d.buf = append(d.buf, chunk...)

	var frames []*Frame
	for {
		frame, n, err := ParseFrame(d.buf)
		if err != nil {
			return frames, err
		}
		if frame == nil {
			return frames, nil
		}
		d.buf = d.buf[n:]
		frames = append(frames, frame)
	}
}
