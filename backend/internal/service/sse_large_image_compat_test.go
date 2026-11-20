package service

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type largeImageBase64Reader struct{}

func (largeImageBase64Reader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'A'
	}
	return len(p), nil
}

func TestDefaultSSELimitPreservesImageFrameAbove40MiB(t *testing.T) {
	// High-resolution image frames can carry more than 40 MiB of base64 data.
	// Generate the input lazily so this test retains only the reader's result.
	const imageBytes = 41 << 20
	const prefix = "data: {\"b64_json\":\""
	const suffix = "\"}\r\n"
	wire := io.MultiReader(
		strings.NewReader(prefix),
		io.LimitReader(largeImageBase64Reader{}, imageBytes),
		strings.NewReader(suffix+"\r\ndata: [DONE]"),
	)
	reader := newBoundedSSELineReader(wire, nil)
	line, err := reader.ReadLine()
	require.NoError(t, err)
	require.Len(t, line, len(prefix)+imageBytes+len(suffix))
	require.True(t, bytes.HasPrefix(line, []byte(prefix)))
	require.True(t, bytes.HasSuffix(line, []byte(suffix)))
	require.Equal(t, imageBytes, bytes.Count(line, []byte{'A'}))
	line, err = reader.ReadLine()
	require.NoError(t, err)
	require.Equal(t, "\r\n", string(line))
	line, err = reader.ReadLine()
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, "data: [DONE]", string(line))
}
