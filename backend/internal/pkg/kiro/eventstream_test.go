package kiro

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildFrame 按 AWS event-stream 格式组装一个测试帧。
// headers 为有序的 (name, stringValue) 对。
func buildFrame(t *testing.T, headers [][2]string, payload []byte) []byte {
	t.Helper()

	var hdr []byte
	for _, kv := range headers {
		name := kv[0]
		val := kv[1]
		hdr = append(hdr, byte(len(name)))
		hdr = append(hdr, name...)
		hdr = append(hdr, 7) // string
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(val)))
		hdr = append(hdr, l[:]...)
		hdr = append(hdr, val...)
	}

	total := uint32(12 + len(hdr) + len(payload) + 4)
	buf := make([]byte, 0, total)
	var u32 [4]byte

	binary.BigEndian.PutUint32(u32[:], total)
	buf = append(buf, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], uint32(len(hdr)))
	buf = append(buf, u32[:]...)

	preludeCRC := crc32.ChecksumIEEE(buf[:8])
	binary.BigEndian.PutUint32(u32[:], preludeCRC)
	buf = append(buf, u32[:]...)

	buf = append(buf, hdr...)
	buf = append(buf, payload...)

	msgCRC := crc32.ChecksumIEEE(buf)
	binary.BigEndian.PutUint32(u32[:], msgCRC)
	buf = append(buf, u32[:]...)

	return buf
}

func TestParseFrameSingle(t *testing.T) {
	t.Parallel()

	raw := buildFrame(t, [][2]string{
		{":message-type", "event"},
		{":event-type", "assistantResponseEvent"},
	}, []byte(`{"content":"hi"}`))

	frame, consumed, err := ParseFrame(raw)
	require.NoError(t, err)
	require.NotNil(t, frame)
	require.Equal(t, len(raw), consumed)
	require.Equal(t, "event", frame.MessageType())
	require.Equal(t, "assistantResponseEvent", frame.EventType())
	require.JSONEq(t, `{"content":"hi"}`, string(frame.Payload))
}

func TestParseFrameIncompleteReturnsNil(t *testing.T) {
	t.Parallel()

	raw := buildFrame(t, [][2]string{{":event-type", "assistantResponseEvent"}}, []byte(`{"content":"x"}`))

	// prelude 都不完整
	frame, consumed, err := ParseFrame(raw[:5])
	require.NoError(t, err)
	require.Nil(t, frame)
	require.Zero(t, consumed)

	// prelude 完整但 body 不完整
	frame, consumed, err = ParseFrame(raw[:len(raw)-3])
	require.NoError(t, err)
	require.Nil(t, frame)
	require.Zero(t, consumed)
}

func TestParseFrameCRCMismatch(t *testing.T) {
	t.Parallel()

	raw := buildFrame(t, [][2]string{{":event-type", "assistantResponseEvent"}}, []byte(`{"content":"x"}`))
	raw[len(raw)-1] ^= 0xFF // 破坏 message CRC

	_, _, err := ParseFrame(raw)
	require.ErrorIs(t, err, ErrCRCMismatch)
}

func TestParseFrameTooLarge(t *testing.T) {
	t.Parallel()

	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[0:4], maxMessageSize+1)
	binary.BigEndian.PutUint32(buf[4:8], 0)
	binary.BigEndian.PutUint32(buf[8:12], crc32.ChecksumIEEE(buf[:8]))

	_, _, err := ParseFrame(buf)
	require.ErrorIs(t, err, ErrFrameTooLarge)
}

func TestDecoderSplitAcrossChunks(t *testing.T) {
	t.Parallel()

	f1 := buildFrame(t, [][2]string{{":event-type", "assistantResponseEvent"}}, []byte(`{"content":"a"}`))
	f2 := buildFrame(t, [][2]string{{":event-type", "assistantResponseEvent"}}, []byte(`{"content":"b"}`))
	stream := append(append([]byte{}, f1...), f2...)

	d := NewDecoder()
	var got []*Frame
	// 逐字节喂入，最严苛的切分
	for i := 0; i < len(stream); i++ {
		frames, err := d.Feed(stream[i : i+1])
		require.NoError(t, err)
		got = append(got, frames...)
	}

	require.Len(t, got, 2)
	require.JSONEq(t, `{"content":"a"}`, string(got[0].Payload))
	require.JSONEq(t, `{"content":"b"}`, string(got[1].Payload))
}

func TestDecoderPayloadContainingFrameLikeLiteral(t *testing.T) {
	t.Parallel()

	// 回归测试：payload 内含 `{"content":` 字面量。
	// 字符串扫描式的解析器（kiro-go-proxy 的做法）会在这里错切。
	payload := []byte(`{"content":"the literal {\"content\": is inside"}`)
	raw := buildFrame(t, [][2]string{{":event-type", "assistantResponseEvent"}}, payload)

	d := NewDecoder()
	frames, err := d.Feed(raw)
	require.NoError(t, err)
	require.Len(t, frames, 1)
	require.Equal(t, payload, frames[0].Payload)
}
