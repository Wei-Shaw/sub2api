package connectrpc

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	payload := []byte("hello connect")
	frame, rest, err := Decode(Encode(payload, 0))
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Equal(t, byte(0), frame.Flags)
	require.Equal(t, payload, frame.Payload)
}

func TestEncodeDecodeGzip(t *testing.T) {
	payload := bytes.Repeat([]byte("gzip-payload-"), 32)
	encoded, err := EncodeGzip(payload)
	require.NoError(t, err)
	frame, rest, err := Decode(encoded)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Equal(t, byte(FlagCompressed), frame.Flags)
	require.Equal(t, payload, frame.Payload)
}

func TestReadFramesEndStreamTrailer(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Encode([]byte("one"), 0))
	buf.Write(Encode([]byte(`{"code":"internal","message":"boom"}`), FlagEndStream))

	var payloads []string
	err := ReadFrames(&buf, func(frame Frame) error {
		payloads = append(payloads, string(frame.Payload))
		return nil
	})
	require.EqualError(t, err, "connect internal: boom")
	require.Equal(t, []string{"one", `{"code":"internal","message":"boom"}`}, payloads)
}

func TestDecodeRejectsOversizedFrame(t *testing.T) {
	header := Encode(nil, 0)
	header[1] = 0x02
	header[2] = 0x00
	header[3] = 0x00
	header[4] = 0x01
	_, _, err := Decode(header)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}

func TestDecodeUnexpectedEOF(t *testing.T) {
	_, _, err := Decode([]byte{0x00, 0x00})
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}
