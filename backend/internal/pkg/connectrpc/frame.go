package connectrpc

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	FlagCompressed = 0x01
	FlagEndStream  = 0x02
	MaxFrameBytes  = 16 * 1024 * 1024
)

type Frame struct {
	Flags   byte
	Payload []byte
}

func Encode(payload []byte, flags byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = flags
	binary.BigEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:], payload)
	return out
}

func EncodeGzip(payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(payload); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return Encode(buf.Bytes(), FlagCompressed), nil
}

func Decode(src []byte) (frame Frame, rest []byte, err error) {
	if len(src) < 5 {
		return Frame{}, src, io.ErrUnexpectedEOF
	}
	length := binary.BigEndian.Uint32(src[1:5])
	if length > MaxFrameBytes {
		return Frame{}, src, fmt.Errorf("connect frame length %d exceeds %d", length, MaxFrameBytes)
	}
	if uint32(len(src)-5) < length {
		return Frame{}, src, io.ErrUnexpectedEOF
	}
	payload := src[5 : 5+length]
	flags := src[0]
	if flags&FlagCompressed != 0 {
		zr, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return Frame{}, nil, err
		}
		defer zr.Close()
		decoded, err := io.ReadAll(io.LimitReader(zr, MaxFrameBytes+1))
		if err != nil {
			return Frame{}, nil, err
		}
		if len(decoded) > MaxFrameBytes {
			return Frame{}, nil, fmt.Errorf("decompressed connect frame exceeds %d", MaxFrameBytes)
		}
		payload = decoded
	}
	return Frame{Flags: flags, Payload: payload}, src[5+length:], nil
}

func ReadFrames(r io.Reader, fn func(Frame) error) error {
	header := make([]byte, 5)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		length := binary.BigEndian.Uint32(header[1:5])
		if length > MaxFrameBytes {
			return fmt.Errorf("connect frame length %d exceeds %d", length, MaxFrameBytes)
		}
		payload := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(r, payload); err != nil {
				return err
			}
		}
		frame, _, err := Decode(append(append([]byte{}, header...), payload...))
		if err != nil {
			return err
		}
		if err := fn(frame); err != nil {
			return err
		}
		if frame.Flags&FlagEndStream != 0 {
			if err := TrailerError(frame.Payload); err != nil {
				return err
			}
			return nil
		}
	}
}

func TrailerError(payload []byte) error {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return nil
	}
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("connect trailer: %s", text)
	}
	code, _ := envelope["code"].(string)
	message, _ := envelope["message"].(string)
	if code == "" && message == "" {
		return nil
	}
	if message == "" {
		message = text
	}
	return fmt.Errorf("connect %s: %s", strings.TrimSpace(code), message)
}
