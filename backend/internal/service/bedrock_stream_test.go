package service

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExtractBedrockChunkData(t *testing.T) {
	t.Run("valid base64 payload", func(t *testing.T) {
		original := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`
		b64 := base64.StdEncoding.EncodeToString([]byte(original))
		payload := []byte(`{"bytes":"` + b64 + `"}`)

		result := extractBedrockChunkData(payload)
		require.NotNil(t, result)
		assert.JSONEq(t, original, string(result))
	})

	t.Run("empty bytes field", func(t *testing.T) {
		result := extractBedrockChunkData([]byte(`{"bytes":""}`))
		assert.Nil(t, result)
	})

	t.Run("no bytes field", func(t *testing.T) {
		result := extractBedrockChunkData([]byte(`{"other":"value"}`))
		assert.Nil(t, result)
	})

	t.Run("invalid base64", func(t *testing.T) {
		result := extractBedrockChunkData([]byte(`{"bytes":"not-valid-base64!!!"}`))
		assert.Nil(t, result)
	})
}

func TestTransformBedrockInvocationMetrics(t *testing.T) {
	t.Run("converts metrics to usage", func(t *testing.T) {
		input := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"amazon-bedrock-invocationMetrics":{"inputTokenCount":150,"outputTokenCount":42}}`
		result := transformBedrockInvocationMetrics([]byte(input))

		// amazon-bedrock-invocationMetrics should be removed
		assert.False(t, gjson.GetBytes(result, "amazon-bedrock-invocationMetrics").Exists())
		// usage should be set
		assert.Equal(t, int64(150), gjson.GetBytes(result, "usage.input_tokens").Int())
		assert.Equal(t, int64(42), gjson.GetBytes(result, "usage.output_tokens").Int())
		// original fields preserved
		assert.Equal(t, "message_delta", gjson.GetBytes(result, "type").String())
		assert.Equal(t, "end_turn", gjson.GetBytes(result, "delta.stop_reason").String())
	})

	t.Run("no metrics present", func(t *testing.T) {
		input := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`
		result := transformBedrockInvocationMetrics([]byte(input))
		assert.JSONEq(t, input, string(result))
	})

	t.Run("does not overwrite existing usage", func(t *testing.T) {
		input := `{"type":"message_delta","usage":{"output_tokens":100},"amazon-bedrock-invocationMetrics":{"inputTokenCount":150,"outputTokenCount":42}}`
		result := transformBedrockInvocationMetrics([]byte(input))

		// metrics removed but existing usage preserved
		assert.False(t, gjson.GetBytes(result, "amazon-bedrock-invocationMetrics").Exists())
		assert.Equal(t, int64(100), gjson.GetBytes(result, "usage.output_tokens").Int())
	})
}

func TestExtractEventStreamHeaderValue(t *testing.T) {
	// Build a header with :event-type = "chunk" (string type = 7)
	buildStringHeader := func(name, value string) []byte {
		var buf bytes.Buffer
		// name length (1 byte)
		_ = buf.WriteByte(byte(len(name)))
		// name
		_, _ = buf.WriteString(name)
		// value type (7 = string)
		_ = buf.WriteByte(7)
		// value length (2 bytes, big-endian)
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(value)))
		// value
		_, _ = buf.WriteString(value)
		return buf.Bytes()
	}

	t.Run("find string header", func(t *testing.T) {
		headers := buildStringHeader(":event-type", "chunk")
		assert.Equal(t, "chunk", extractEventStreamHeaderValue(headers, ":event-type"))
	})

	t.Run("header not found", func(t *testing.T) {
		headers := buildStringHeader(":event-type", "chunk")
		assert.Equal(t, "", extractEventStreamHeaderValue(headers, ":message-type"))
	})

	t.Run("multiple headers", func(t *testing.T) {
		var buf bytes.Buffer
		_, _ = buf.Write(buildStringHeader(":content-type", "application/json"))
		_, _ = buf.Write(buildStringHeader(":event-type", "chunk"))
		_, _ = buf.Write(buildStringHeader(":message-type", "event"))

		headers := buf.Bytes()
		assert.Equal(t, "chunk", extractEventStreamHeaderValue(headers, ":event-type"))
		assert.Equal(t, "application/json", extractEventStreamHeaderValue(headers, ":content-type"))
		assert.Equal(t, "event", extractEventStreamHeaderValue(headers, ":message-type"))
	})

	t.Run("empty headers", func(t *testing.T) {
		assert.Equal(t, "", extractEventStreamHeaderValue([]byte{}, ":event-type"))
	})
}

func TestBedrockEventStreamDecoder(t *testing.T) {
	crc32IeeeTab := crc32.MakeTable(crc32.IEEE)

	// Build a valid EventStream frame with correct CRC32/IEEE checksums.
	buildFrame := func(eventType string, payload []byte) []byte {
		// Build headers
		var headersBuf bytes.Buffer
		// :event-type header
		_ = headersBuf.WriteByte(byte(len(":event-type")))
		_, _ = headersBuf.WriteString(":event-type")
		_ = headersBuf.WriteByte(7) // string type
		_ = binary.Write(&headersBuf, binary.BigEndian, uint16(len(eventType)))
		_, _ = headersBuf.WriteString(eventType)
		// :message-type header
		_ = headersBuf.WriteByte(byte(len(":message-type")))
		_, _ = headersBuf.WriteString(":message-type")
		_ = headersBuf.WriteByte(7)
		_ = binary.Write(&headersBuf, binary.BigEndian, uint16(len("event")))
		_, _ = headersBuf.WriteString("event")

		headers := headersBuf.Bytes()
		headersLen := uint32(len(headers))
		// total = 12 (prelude) + headers + payload + 4 (message_crc)
		totalLen := uint32(12 + len(headers) + len(payload) + 4)

		// Prelude: total_length(4) + headers_length(4)
		var preludeBuf bytes.Buffer
		_ = binary.Write(&preludeBuf, binary.BigEndian, totalLen)
		_ = binary.Write(&preludeBuf, binary.BigEndian, headersLen)
		preludeBytes := preludeBuf.Bytes()
		preludeCRC := crc32.Checksum(preludeBytes, crc32IeeeTab)

		// Build frame: prelude + prelude_crc + headers + payload
		var frame bytes.Buffer
		_, _ = frame.Write(preludeBytes)
		_ = binary.Write(&frame, binary.BigEndian, preludeCRC)
		_, _ = frame.Write(headers)
		_, _ = frame.Write(payload)

		// Message CRC covers everything before itself
		messageCRC := crc32.Checksum(frame.Bytes(), crc32IeeeTab)
		_ = binary.Write(&frame, binary.BigEndian, messageCRC)
		return frame.Bytes()
	}

	t.Run("decode chunk event", func(t *testing.T) {
		payload := []byte(`{"bytes":"dGVzdA=="}`) // base64("test")
		frame := buildFrame("chunk", payload)

		decoder := newBedrockEventStreamDecoder(bytes.NewReader(frame))
		result, err := decoder.Decode()
		require.NoError(t, err)
		assert.Equal(t, payload, result)
	})

	t.Run("skip non-chunk events", func(t *testing.T) {
		// Write initial-response followed by chunk
		var buf bytes.Buffer
		_, _ = buf.Write(buildFrame("initial-response", []byte(`{}`)))
		chunkPayload := []byte(`{"bytes":"aGVsbG8="}`)
		_, _ = buf.Write(buildFrame("chunk", chunkPayload))

		decoder := newBedrockEventStreamDecoder(&buf)
		result, err := decoder.Decode()
		require.NoError(t, err)
		assert.Equal(t, chunkPayload, result)
	})

	t.Run("EOF on empty input", func(t *testing.T) {
		decoder := newBedrockEventStreamDecoder(bytes.NewReader(nil))
		_, err := decoder.Decode()
		assert.Equal(t, io.EOF, err)
	})

	t.Run("corrupted prelude CRC", func(t *testing.T) {
		frame := buildFrame("chunk", []byte(`{"bytes":"dGVzdA=="}`))
		// Corrupt the prelude CRC (bytes 8-11)
		frame[8] ^= 0xFF
		decoder := newBedrockEventStreamDecoder(bytes.NewReader(frame))
		_, err := decoder.Decode()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prelude CRC mismatch")
	})

	t.Run("corrupted message CRC", func(t *testing.T) {
		frame := buildFrame("chunk", []byte(`{"bytes":"dGVzdA=="}`))
		// Corrupt the message CRC (last 4 bytes)
		frame[len(frame)-1] ^= 0xFF
		decoder := newBedrockEventStreamDecoder(bytes.NewReader(frame))
		_, err := decoder.Decode()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "message CRC mismatch")
	})

	t.Run("castagnoli encoded frame is rejected", func(t *testing.T) {
		castagnoliTab := crc32.MakeTable(crc32.Castagnoli)
		payload := []byte(`{"bytes":"dGVzdA=="}`)

		var headersBuf bytes.Buffer
		_ = headersBuf.WriteByte(byte(len(":event-type")))
		_, _ = headersBuf.WriteString(":event-type")
		_ = headersBuf.WriteByte(7)
		_ = binary.Write(&headersBuf, binary.BigEndian, uint16(len("chunk")))
		_, _ = headersBuf.WriteString("chunk")

		headers := headersBuf.Bytes()
		headersLen := uint32(len(headers))
		totalLen := uint32(12 + len(headers) + len(payload) + 4)

		var preludeBuf bytes.Buffer
		_ = binary.Write(&preludeBuf, binary.BigEndian, totalLen)
		_ = binary.Write(&preludeBuf, binary.BigEndian, headersLen)
		preludeBytes := preludeBuf.Bytes()

		var frame bytes.Buffer
		_, _ = frame.Write(preludeBytes)
		_ = binary.Write(&frame, binary.BigEndian, crc32.Checksum(preludeBytes, castagnoliTab))
		_, _ = frame.Write(headers)
		_, _ = frame.Write(payload)
		_ = binary.Write(&frame, binary.BigEndian, crc32.Checksum(frame.Bytes(), castagnoliTab))

		decoder := newBedrockEventStreamDecoder(bytes.NewReader(frame.Bytes()))
		_, err := decoder.Decode()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prelude CRC mismatch")
	})
}

func TestBuildBedrockURL(t *testing.T) {
	t.Run("stream URL with colon in model ID", func(t *testing.T) {
		url := BuildBedrockURL("us-east-1", "us.anthropic.claude-opus-4-5-20251101-v1:0", true)
		assert.Equal(t, "https://bedrock-runtime.us-east-1.amazonaws.com/model/us.anthropic.claude-opus-4-5-20251101-v1%3A0/invoke-with-response-stream", url)
	})

	t.Run("non-stream URL with colon in model ID", func(t *testing.T) {
		url := BuildBedrockURL("eu-west-1", "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", false)
		assert.Equal(t, "https://bedrock-runtime.eu-west-1.amazonaws.com/model/eu.anthropic.claude-sonnet-4-5-20250929-v1%3A0/invoke", url)
	})

	t.Run("model ID without colon", func(t *testing.T) {
		url := BuildBedrockURL("us-east-1", "us.anthropic.claude-sonnet-4-6", true)
		assert.Equal(t, "https://bedrock-runtime.us-east-1.amazonaws.com/model/us.anthropic.claude-sonnet-4-6/invoke-with-response-stream", url)
	})
}

// TestBedrockEventStreamDecoderHostileLengths 覆盖"CRC 自洽但长度字段恶意"的帧。
// CRC32 只能证明数据没有被传输损坏，无法证明 total_length / headers_length 合法，
// 因此这些帧会正常通过两道 CRC 校验并直达切片逻辑。修复前 data[:headersLength]
// 与 data[headersLength:len(data)-4] 会触发 slice bounds out of range panic；
// 该 panic 发生在解码协程中，不受 gin Recovery() 保护，会直接终止整个进程。
func TestBedrockEventStreamDecoderHostileLengths(t *testing.T) {
	tab := crc32.MakeTable(crc32.IEEE)

	// buildHostileFrame 构造 prelude CRC 与 message CRC 均正确、
	// 但 total_length / headers_length 由调用方任意指定的帧。
	// bodyLen 为 prelude 之后、message_crc 之前的字节数；
	// 合法帧应满足 bodyLen == total_length - 16。
	buildHostileFrame := func(totalLen, headersLen uint32, bodyLen int) []byte {
		prelude := make([]byte, 8)
		binary.BigEndian.PutUint32(prelude[0:4], totalLen)
		binary.BigEndian.PutUint32(prelude[4:8], headersLen)

		var frame bytes.Buffer
		_, _ = frame.Write(prelude)
		_ = binary.Write(&frame, binary.BigEndian, crc32.Checksum(prelude, tab))
		if bodyLen > 0 {
			_, _ = frame.Write(bytes.Repeat([]byte{0x41}, bodyLen))
		}
		// message CRC 覆盖它之前的全部字节（prelude + prelude_crc + body）
		_ = binary.Write(&frame, binary.BigEndian, crc32.Checksum(frame.Bytes(), tab))
		return frame.Bytes()
	}

	// 自检：辅助函数生成的合法帧确实能通过两道 CRC 校验，
	// 否则下面的用例会在 CRC 处提前返回，根本触及不到越界代码。
	t.Run("helper frames pass both CRC checks", func(t *testing.T) {
		// total_length=40 => body 24 字节 + 4 字节 message_crc
		frame := buildHostileFrame(40, 8, 24)
		decoder := newBedrockEventStreamDecoder(bytes.NewReader(frame))
		_, err := decoder.Decode()
		// headers/payload 是无意义填充，不是 chunk 事件，会被跳过并读到流尾
		require.ErrorIs(t, err, io.EOF)
		assert.NotContains(t, err.Error(), "CRC mismatch")
	})

	tests := []struct {
		name      string
		frame     []byte
		wantErrIs error
		wantErrIn string
	}{
		{
			// headers_length 超出帧体：total_length=40 时上限为 24
			name:      "headers_length exceeds frame body",
			frame:     buildHostileFrame(40, 100, 24),
			wantErrIn: "headers_length=100 exceeds total_length=40",
		},
		{
			name:      "headers_length is MaxUint32",
			frame:     buildHostileFrame(40, math.MaxUint32, 24),
			wantErrIn: "headers_length=4294967295 exceeds total_length=40",
		},
		{
			// 恰好越界一个字节：payload 切片下界会大于上界
			name:      "headers_length off by one past payload boundary",
			frame:     buildHostileFrame(40, 25, 24),
			wantErrIn: "headers_length=25 exceeds total_length=40",
		},
		{
			// headers_length == total_length-16 是合法的（payload 为空）
			name:      "headers_length exactly fills frame body",
			frame:     buildHostileFrame(40, 24, 24),
			wantErrIs: io.EOF,
		},
		{
			// 最小合法帧：仅 prelude + message_crc，headers/payload 均为空
			name:      "minimum valid frame with empty headers and payload",
			frame:     buildHostileFrame(16, 0, 0),
			wantErrIs: io.EOF,
		},
		{
			name:      "total_length below the 16 byte minimum",
			frame:     buildHostileFrame(15, 0, 0),
			wantErrIn: "total_length=15",
		},
		{
			name:      "total_length zero",
			frame:     buildHostileFrame(0, 0, 0),
			wantErrIn: "total_length=0",
		},
		{
			// total_length=16 时 headers_length 只能为 0
			name:      "headers_length nonzero on minimum frame",
			frame:     buildHostileFrame(16, 1, 0),
			wantErrIn: "headers_length=1 exceeds total_length=16",
		},
		{
			// 超大 total_length 会触发巨量分配，必须在 make 之前拒绝
			name:      "total_length exceeds max frame size",
			frame:     buildHostileFrame(bedrockFrameMaxLength+1, 0, 0),
			wantErrIn: "exceeds max",
		},
		{
			name:      "total_length MaxUint32",
			frame:     buildHostileFrame(math.MaxUint32, 0, 0),
			wantErrIn: "exceeds max",
		},
		{
			// 长度字段合法但实际数据被截断
			name:      "truncated frame body",
			frame:     buildHostileFrame(1000, 10, 20),
			wantErrIs: io.ErrUnexpectedEOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := newBedrockEventStreamDecoder(bytes.NewReader(tt.frame))
			var (
				err error
				out []byte
			)
			require.NotPanics(t, func() {
				out, err = decoder.Decode()
			})
			require.Error(t, err)
			assert.Nil(t, out)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantErrIn != "" {
				assert.Contains(t, err.Error(), tt.wantErrIn)
			}
		})
	}
}
