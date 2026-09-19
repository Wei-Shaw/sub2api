// Package cursor implements the Connect-RPC protocol codec and protobuf
// encoding/decoding needed to communicate with Cursor's backend API.
package cursor

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Connect-RPC envelope frame flags.
const (
	FrameFlagUncompressed byte = 0x00
	FrameFlagGzip         byte = 0x01
	FrameFlagEndStream    byte = 0x02
)

// EncodeFrame wraps a protobuf payload in a Connect-RPC envelope.
// Format: [1-byte flags][4-byte big-endian length][payload].
// If compress is true the payload is gzip-compressed and the flag is set.
func EncodeFrame(payload []byte, compress bool) ([]byte, error) {
	flag := FrameFlagUncompressed
	data := payload
	if compress {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(payload); err != nil {
			return nil, fmt.Errorf("cursor: gzip write: %w", err)
		}
		if err := gz.Close(); err != nil {
			return nil, fmt.Errorf("cursor: gzip close: %w", err)
		}
		data = buf.Bytes()
		flag = FrameFlagGzip
	}

	out := make([]byte, 5+len(data))
	out[0] = flag
	binary.BigEndian.PutUint32(out[1:5], uint32(len(data)))
	copy(out[5:], data)
	return out, nil
}

// Frame is a single decoded Connect-RPC envelope frame.
type Frame struct {
	Flags   byte
	Payload []byte
}

// DecodeFrame reads exactly one Connect-RPC frame from r.
// Returns io.EOF when there is no more data.
func DecodeFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	flags := header[0]
	length := binary.BigEndian.Uint32(header[1:5])
	if length > 16<<20 { // 16 MiB safety limit
		return nil, fmt.Errorf("cursor: frame too large: %d bytes", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("cursor: short frame payload: %w", err)
	}

	if flags&FrameFlagGzip != 0 {
		gr, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("cursor: gzip open: %w", err)
		}
		decompressed, err := io.ReadAll(gr)
		gr.Close()
		if err != nil {
			return nil, fmt.Errorf("cursor: gzip decompress: %w", err)
		}
		payload = decompressed
	}

	return &Frame{Flags: flags, Payload: payload}, nil
}

// ReadRawFrame reads one Connect-RPC envelope and returns both the original
// on-the-wire bytes and the decoded (possibly gunzipped) payload.
func ReadRawFrame(r io.Reader) (raw []byte, frame *Frame, err error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, nil, err
	}
	flags := header[0]
	length := binary.BigEndian.Uint32(header[1:5])
	if length > 16<<20 {
		return nil, nil, fmt.Errorf("cursor: frame too large: %d bytes", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, nil, fmt.Errorf("cursor: short frame payload: %w", err)
	}
	raw = make([]byte, 5+len(payload))
	copy(raw, header)
	copy(raw[5:], payload)

	decoded := payload
	if flags&FrameFlagGzip != 0 {
		gr, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, nil, fmt.Errorf("cursor: gzip open: %w", err)
		}
		decompressed, err := io.ReadAll(gr)
		gr.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("cursor: gzip decompress: %w", err)
		}
		decoded = decompressed
	}
	return raw, &Frame{Flags: flags, Payload: decoded}, nil
}

// ConnectErrorJSON extracts a Connect end-stream JSON error, if present.
func ConnectErrorJSON(frame *Frame) string {
	if frame == nil || frame.Flags&FrameFlagEndStream == 0 {
		return ""
	}
	if len(frame.Payload) == 0 || frame.Payload[0] != '{' {
		return ""
	}
	return string(frame.Payload)
}

// ConnectError is a Connect end-stream JSON error.
type ConnectError struct {
	Code    string
	Message string
}

// ParseConnectError decodes a Connect end-stream JSON object.
func ParseConnectError(raw string) (ConnectError, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return ConnectError{}, false
	}
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return ConnectError{Message: raw}, true
	}
	out := ConnectError{Code: envelope.Code, Message: envelope.Message}
	if envelope.Error != nil {
		if envelope.Error.Code != "" {
			out.Code = envelope.Error.Code
		}
		if envelope.Error.Message != "" {
			out.Message = envelope.Error.Message
		}
	}
	if out.Code == "" && out.Message == "" {
		return ConnectError{Message: raw}, true
	}
	return out, true
}

// IsBadModelName reports whether a Connect error is Cursor ERROR_BAD_MODEL_NAME.
func (e ConnectError) IsBadModelName() bool {
	blob := strings.ToLower(e.Code + " " + e.Message)
	return strings.Contains(blob, "bad_model_name") || strings.Contains(blob, "model name is not valid")
}
