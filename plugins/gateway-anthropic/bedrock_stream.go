package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// --- AWS EventStream binary decoder ---

// bedrockEventStreamDecoder decodes AWS EventStream binary frames.
// Frame layout:
//
//	[total_byte_length: 4 bytes]
//	[headers_byte_length: 4 bytes]
//	[prelude_crc: 4 bytes]
//	[headers: variable]
//	[payload: variable]
//	[message_crc: 4 bytes]
type bedrockEventStreamDecoder struct {
	reader *bufio.Reader
}

func newBedrockEventStreamDecoder(r io.Reader) *bedrockEventStreamDecoder {
	return &bedrockEventStreamDecoder{
		reader: bufio.NewReaderSize(r, 64*1024),
	}
}

// crc32IEEETable is the CRC32/IEEE table used by AWS EventStream.
var crc32IEEETable = crc32.MakeTable(crc32.IEEE)

// decode reads the next EventStream frame and returns the payload of
// "chunk" type events. Non-chunk events are skipped; exceptions cause errors.
func (d *bedrockEventStreamDecoder) decode() ([]byte, error) {
	for {
		prelude := make([]byte, 12)
		if _, err := io.ReadFull(d.reader, prelude); err != nil {
			return nil, err
		}

		preludeCRC := readUint32(prelude[8:12])
		if crc32.Checksum(prelude[0:8], crc32IEEETable) != preludeCRC {
			return nil, fmt.Errorf("eventstream prelude CRC mismatch")
		}

		totalLength := readUint32(prelude[0:4])
		headersLength := readUint32(prelude[4:8])

		if totalLength < 16 {
			return nil, fmt.Errorf("invalid eventstream frame: total_length=%d", totalLength)
		}

		remaining := int(totalLength) - 12
		if remaining <= 0 {
			continue
		}
		data := make([]byte, remaining)
		if _, err := io.ReadFull(d.reader, data); err != nil {
			return nil, err
		}

		// Verify message CRC.
		messageCRC := readUint32(data[len(data)-4:])
		h := crc32.New(crc32IEEETable)
		_, _ = h.Write(prelude)
		_, _ = h.Write(data[:len(data)-4])
		if h.Sum32() != messageCRC {
			return nil, fmt.Errorf("eventstream message CRC mismatch")
		}

		headers := data[:headersLength]
		payload := data[headersLength : len(data)-4]

		eventType := extractEventStreamHeaderValue(headers, ":event-type")
		if eventType == "chunk" {
			return payload, nil
		}

		exceptionType := extractEventStreamHeaderValue(headers, ":exception-type")
		if exceptionType != "" {
			return nil, fmt.Errorf("bedrock exception: %s: %s", exceptionType, string(payload))
		}
		messageType := extractEventStreamHeaderValue(headers, ":message-type")
		if messageType == "exception" || messageType == "error" {
			return nil, fmt.Errorf("bedrock error: %s", string(payload))
		}
		// Skip other event types (e.g., initial-response).
	}
}

// extractEventStreamHeaderValue reads a string-typed header from EventStream
// binary header data.
func extractEventStreamHeaderValue(headers []byte, targetName string) string {
	pos := 0
	for pos < len(headers) {
		nameLen := int(headers[pos])
		pos++
		if pos+nameLen > len(headers) {
			break
		}
		name := string(headers[pos : pos+nameLen])
		pos += nameLen

		if pos >= len(headers) {
			break
		}
		valueType := headers[pos]
		pos++

		switch valueType {
		case 7: // string
			if pos+2 > len(headers) {
				return ""
			}
			valueLen := int(readUint16(headers[pos : pos+2]))
			pos += 2
			if pos+valueLen > len(headers) {
				return ""
			}
			value := string(headers[pos : pos+valueLen])
			pos += valueLen
			if name == targetName {
				return value
			}
		case 0, 1: // bool true / false
			if name == targetName {
				if valueType == 0 {
					return "true"
				}
				return "false"
			}
		case 2: // byte
			pos++
		case 3: // short
			pos += 2
		case 4: // int
			pos += 4
		case 5: // long
			pos += 8
		case 6: // bytes
			if pos+2 > len(headers) {
				return ""
			}
			valueLen := int(readUint16(headers[pos : pos+2]))
			pos += 2 + valueLen
		case 8: // timestamp
			pos += 8
		case 9: // uuid
			pos += 16
		default:
			return ""
		}
	}
	return ""
}

func readUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func readUint16(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

// --- Bedrock EventStream chunk data extraction ---

// extractBedrockChunkData extracts the Claude SSE JSON from a Bedrock
// EventStream chunk payload. The payload format is:
//
//	{"bytes":"<base64-encoded-json>"}
func extractBedrockChunkData(payload []byte) []byte {
	var envelope struct {
		Bytes string `json:"bytes"`
	}
	if json.Unmarshal(payload, &envelope) != nil || envelope.Bytes == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(envelope.Bytes)
	if err != nil {
		return nil
	}
	return decoded
}

// --- Bedrock invocation metrics transform ---

// transformBedrockInvocationMetrics converts Bedrock's proprietary
// amazon-bedrock-invocationMetrics to standard Anthropic usage format.
func transformBedrockInvocationMetrics(data []byte) []byte {
	var parsed map[string]json.RawMessage
	if json.Unmarshal(data, &parsed) != nil {
		return data
	}

	metricsRaw, ok := parsed["amazon-bedrock-invocationMetrics"]
	if !ok {
		return data
	}
	delete(parsed, "amazon-bedrock-invocationMetrics")

	// If standard usage already exists, just remove the metrics field.
	if _, hasUsage := parsed["usage"]; hasUsage {
		result, err := json.Marshal(parsed)
		if err != nil {
			return data
		}
		return result
	}

	// Convert camelCase metric fields to snake_case usage.
	var metrics struct {
		InputTokenCount  int64 `json:"inputTokenCount"`
		OutputTokenCount int64 `json:"outputTokenCount"`
	}
	if json.Unmarshal(metricsRaw, &metrics) != nil {
		result, _ := json.Marshal(parsed)
		return result
	}

	usage := map[string]int64{}
	if metrics.InputTokenCount > 0 {
		usage["input_tokens"] = metrics.InputTokenCount
	}
	if metrics.OutputTokenCount > 0 {
		usage["output_tokens"] = metrics.OutputTokenCount
	}
	if len(usage) > 0 {
		usageBytes, _ := json.Marshal(usage)
		parsed["usage"] = usageBytes
	}

	result, err := json.Marshal(parsed)
	if err != nil {
		return data
	}
	return result
}

// --- Bedrock stream processing for Forward ---

// processBedrockStream reads the Bedrock EventStream response, decodes each
// chunk from the binary frame format, extracts usage, and streams SSE events
// back to the host via gRPC chunks.
func processBedrockStream(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	body io.Reader,
	startTime time.Time,
	originalModel, mappedModel string,
) (*streamResult, error) {
	decoder := newBedrockEventStreamDecoder(body)
	result := &streamResult{}
	needModelReplace := originalModel != mappedModel
	firstTokenSent := false

	for {
		payload, err := decoder.decode()
		if err != nil {
			if err == io.EOF {
				return result, nil
			}
			if result.sawTerminalEvent {
				return result, nil
			}
			return result, fmt.Errorf("bedrock stream decode: %w", err)
		}

		sseData := extractBedrockChunkData(payload)
		if sseData == nil {
			continue
		}

		// Record first token timing.
		if !firstTokenSent {
			firstTokenSent = true
			result.firstTokenMs = int32(time.Since(startTime).Milliseconds())
		}

		// Transform Bedrock metrics to standard usage.
		sseData = transformBedrockInvocationMetrics(sseData)

		// Extract usage from the SSE event.
		var event map[string]any
		if json.Unmarshal(sseData, &event) == nil {
			extractUsageFromEvent(event, result)

			eventType, _ := event["type"].(string)
			if eventType == "message_stop" {
				result.sawTerminalEvent = true
			}
		}

		// Replace model in response if mapping was applied.
		if needModelReplace {
			sseData = replaceModelInBedrockSSE(sseData, mappedModel, originalModel)
		}

		// Determine SSE event type for framing.
		eventType := ""
		if event != nil {
			eventType, _ = event["type"].(string)
		}

		// Build standard SSE frame and send as body chunk.
		var sseFrame string
		if eventType != "" {
			sseFrame = fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, sseData)
		} else {
			sseFrame = fmt.Sprintf("data: %s\n\n", sseData)
		}

		if !result.clientDisconnect {
			if err := gatewayutil.SendBodyChunk(stream, []byte(sseFrame)); err != nil {
				result.clientDisconnect = true
				// Continue draining to capture usage.
			}
		}
	}
}

// processBedrockNonStreamResponse reads a non-streaming Bedrock response,
// transforms metrics, extracts usage, and sends it as a body chunk.
func processBedrockNonStreamResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	body io.Reader,
	originalModel, mappedModel string,
) (*streamResult, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBodySize))
	if err != nil {
		return nil, err
	}

	// Transform Bedrock invocation metrics.
	data = transformBedrockInvocationMetrics(data)

	result := &streamResult{}
	extractNonStreamUsage(data, result)

	if originalModel != mappedModel {
		data = replaceModelInResponseJSON(data, mappedModel, originalModel)
	}

	return result, gatewayutil.SendBodyChunk(stream, data)
}

// replaceModelInBedrockSSE replaces the model in a Bedrock SSE event JSON.
func replaceModelInBedrockSSE(data []byte, from, to string) []byte {
	var event map[string]any
	if json.Unmarshal(data, &event) != nil {
		return data
	}
	if replaceModelInEvent(event, from, to) {
		if newData, err := json.Marshal(event); err == nil {
			return newData
		}
	}
	return data
}

// --- Bedrock streaming response header builder ---

// bedrockResponseContentType returns the expected content type for Bedrock
// responses. Streaming uses application/vnd.amazon.eventstream.
func bedrockResponseContentType(isStream bool) string {
	if isStream {
		return "application/vnd.amazon.eventstream"
	}
	return "application/json"
}

// mapBedrockResponseHeaders maps Bedrock-specific response headers to
// standard headers. Specifically, x-amzn-requestid -> x-request-id.
func mapBedrockResponseHeaders(headers map[string]string) {
	if reqID, ok := headers["X-Amzn-Requestid"]; ok {
		if _, exists := headers["X-Request-Id"]; !exists {
			headers["X-Request-Id"] = reqID
		}
	}
	// Also check lowercase variants.
	if reqID, ok := headers["x-amzn-requestid"]; ok {
		if _, exists := headers["x-request-id"]; !exists {
			headers["x-request-id"] = reqID
		}
	}
}

// bedrockRequestID extracts the request ID from Bedrock response headers,
// checking both standard and Bedrock-specific header names.
func bedrockRequestID(headers map[string]string) string {
	for _, key := range []string{
		"x-amzn-requestid", "X-Amzn-Requestid",
		"x-request-id", "X-Request-Id",
	} {
		if v := headers[key]; v != "" {
			return v
		}
	}
	return ""
}

// getClientBetaHeader extracts the anthropic-beta header from client headers.
func getClientBetaHeader(req *pb.GatewayForwardRequest) string {
	for _, headerMap := range []map[string]string{
		req.GetClientHeaders(),
		req.GetHeaders(),
	} {
		for key, val := range headerMap {
			if strings.EqualFold(key, "anthropic-beta") {
				return val
			}
		}
	}
	return ""
}
