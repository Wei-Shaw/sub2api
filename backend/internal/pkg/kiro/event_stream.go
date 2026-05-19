package kiro

import (
	"encoding/json"
	"fmt"
	"io"
)

// Event represents one decoded event from Kiro's AWS Event Stream
// response. Type is the value of the :event-type header; Payload is the
// JSON-decoded message body.
type Event struct {
	Type    string
	Payload map[string]any
}

// DecodeEventStream reads an AWS Event Stream from r and invokes onEvent
// for each successfully decoded message. Returns the first I/O error, or
// nil on a clean EOF. JSON-decode failures for individual messages are
// silently skipped — matches Kiro-Go's tolerant behaviour for malformed
// frames mid-stream.
//
// Frame format:
//
//	[12-byte prelude: total_len(4) | headers_len(4) | crc(4)]
//	[headers_len bytes of headers, encoded per AWS spec]
//	[payload bytes = total_len - 12 - headers_len - 4]
//	[4-byte trailing CRC]
//
// We don't verify CRCs — the underlying TLS connection already guarantees
// integrity for the streaming use case.
func DecodeEventStream(r io.Reader, onEvent func(Event)) error {
	for {
		prelude := make([]byte, 12)
		if _, err := io.ReadFull(r, prelude); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}

		totalLen := int(prelude[0])<<24 | int(prelude[1])<<16 | int(prelude[2])<<8 | int(prelude[3])
		headersLen := int(prelude[4])<<24 | int(prelude[5])<<16 | int(prelude[6])<<8 | int(prelude[7])

		if totalLen < 16 {
			// Too small to be a valid frame; bail.
			return fmt.Errorf("kiro event stream: invalid frame total_len=%d", totalLen)
		}

		remaining := totalLen - 12
		msgBuf := make([]byte, remaining)
		if _, err := io.ReadFull(r, msgBuf); err != nil {
			return err
		}

		if headersLen > len(msgBuf)-4 {
			// Malformed header section; skip this frame entirely.
			continue
		}

		eventType := extractEventType(msgBuf[:headersLen])
		payloadBytes := msgBuf[headersLen : len(msgBuf)-4]
		if len(payloadBytes) == 0 {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			// Malformed payload — skip, matching Kiro-Go behaviour.
			continue
		}

		onEvent(Event{Type: eventType, Payload: payload})
	}
}

// extractEventType walks the AWS Event Stream header block to find the
// :event-type entry. Returns "" if not present.
//
// The header format is a sequence of [name_len(1)][name][value_type(1)][value]
// records. value_type=7 means string with a 2-byte length prefix; other
// value types have fixed widths (skipped here).
func extractEventType(headers []byte) string {
	offset := 0
	for offset < len(headers) {
		if offset >= len(headers) {
			break
		}
		nameLen := int(headers[offset])
		offset++
		if offset+nameLen > len(headers) {
			break
		}
		name := string(headers[offset : offset+nameLen])
		offset += nameLen
		if offset >= len(headers) {
			break
		}
		valueType := headers[offset]
		offset++

		if valueType == 7 { // string with 2-byte length prefix
			if offset+2 > len(headers) {
				break
			}
			valueLen := int(headers[offset])<<8 | int(headers[offset+1])
			offset += 2
			if offset+valueLen > len(headers) {
				break
			}
			value := string(headers[offset : offset+valueLen])
			offset += valueLen
			if name == ":event-type" {
				return value
			}
			continue
		}

		// Skip other value types by their fixed byte widths.
		switch valueType {
		case 0, 1: // bool true / false (no payload)
			// no bytes
		case 2: // byte
			offset += 1
		case 3: // int16
			offset += 2
		case 4: // int32
			offset += 4
		case 5: // int64
			offset += 8
		case 6: // byte array, 2-byte length prefix
			if offset+2 > len(headers) {
				return ""
			}
			l := int(headers[offset])<<8 | int(headers[offset+1])
			offset += 2 + l
		case 8: // timestamp (int64 millis)
			offset += 8
		case 9: // uuid
			offset += 16
		default:
			// Unknown type — bail.
			return ""
		}
	}
	return ""
}

// EncodeEventStream is a minimal AWS Event Stream encoder used only by
// tests. Each message is one event with a :event-type header and a JSON
// payload. The CRC fields are written as zeros (decoders we control
// don't verify them).
func EncodeEventStream(w io.Writer, events []Event) error {
	for _, ev := range events {
		header := encodeHeader(":event-type", ev.Type)
		payload, err := json.Marshal(ev.Payload)
		if err != nil {
			return err
		}
		headersLen := len(header)
		totalLen := 12 + headersLen + len(payload) + 4

		prelude := make([]byte, 12)
		prelude[0] = byte(totalLen >> 24)
		prelude[1] = byte(totalLen >> 16)
		prelude[2] = byte(totalLen >> 8)
		prelude[3] = byte(totalLen)
		prelude[4] = byte(headersLen >> 24)
		prelude[5] = byte(headersLen >> 16)
		prelude[6] = byte(headersLen >> 8)
		prelude[7] = byte(headersLen)
		// prelude CRC (offset 8..11) left as zero.

		if _, err := w.Write(prelude); err != nil {
			return err
		}
		if _, err := w.Write(header); err != nil {
			return err
		}
		if _, err := w.Write(payload); err != nil {
			return err
		}
		// Trailing CRC, left as zero.
		if _, err := w.Write([]byte{0, 0, 0, 0}); err != nil {
			return err
		}
	}
	return nil
}

func encodeHeader(name, value string) []byte {
	nameBytes := []byte(name)
	valueBytes := []byte(value)
	out := make([]byte, 0, 1+len(nameBytes)+1+2+len(valueBytes))
	out = append(out, byte(len(nameBytes)))
	out = append(out, nameBytes...)
	out = append(out, 7) // string value type
	out = append(out, byte(len(valueBytes)>>8), byte(len(valueBytes)))
	out = append(out, valueBytes...)
	return out
}
