package reqcompress

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// Algorithm is an HTTP content-coding token used for a compressed request body.
type Algorithm string

const (
	// AlgorithmNone is the settled state "this account falls back to identity".
	AlgorithmNone Algorithm = ""
	// AlgorithmBrotli is the HTTP "br" content coding.
	AlgorithmBrotli Algorithm = "br"
	// AlgorithmZstd is the HTTP "zstd" content coding.
	AlgorithmZstd Algorithm = "zstd"
	// AlgorithmDeflate is the HTTP "deflate" content coding (zlib wrapper).
	AlgorithmDeflate Algorithm = "deflate"
)

// TrialOrder is the fixed preference order used while negotiating. Brotli
// usually saves the most bytes; zstd/deflate cover providers that only accept
// the simpler codings.
var TrialOrder = []Algorithm{AlgorithmBrotli, AlgorithmZstd, AlgorithmDeflate}

type zstdEncoderFactory func() (*zstd.Encoder, error)

// sharedZstdEncoders amortizes each encoder mode's large window allocation
// across requests. EncodeAll is documented as safe for concurrent use.
var sharedZstdEncoders sync.Map // map[zstd.EncoderLevel]zstdEncoderFactory

func sharedZstdEncoder(level int) zstdEncoderFactory {
	encoderLevel := zstd.EncoderLevelFromZstd(level)
	factory, _ := sharedZstdEncoders.LoadOrStore(encoderLevel, zstdEncoderFactory(sync.OnceValues(func() (*zstd.Encoder, error) {
		return zstd.NewWriter(nil,
			zstd.WithEncoderConcurrency(1),
			zstd.WithEncoderLevel(encoderLevel),
		)
	})))
	return factory.(zstdEncoderFactory)
}

// encodeBody compresses body with the default levels.
func encodeBody(algorithm Algorithm, body []byte) ([]byte, error) {
	return encodeBodyWithLevels(algorithm, body, compresslevel.Default())
}

// encodeBodyWithLevels compresses body with the given coding. HTTP "deflate"
// is the zlib wrapped format (RFC 9110 references RFC 1950), not raw DEFLATE.
func encodeBodyWithLevels(algorithm Algorithm, body []byte, levels compresslevel.Settings) ([]byte, error) {
	var buf bytes.Buffer
	switch algorithm {
	case AlgorithmBrotli:
		writer := brotli.NewWriterLevel(&buf, levels.Brotli)
		if _, err := writer.Write(body); err != nil {
			return nil, fmt.Errorf("reqcompress: brotli encode: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("reqcompress: brotli close: %w", err)
		}
	case AlgorithmZstd:
		encoder, err := sharedZstdEncoder(levels.Zstd)()
		if err != nil {
			return nil, fmt.Errorf("reqcompress: zstd writer: %w", err)
		}
		return encoder.EncodeAll(body, nil), nil
	case AlgorithmDeflate:
		writer, err := zlib.NewWriterLevel(&buf, levels.Deflate)
		if err != nil {
			return nil, fmt.Errorf("reqcompress: deflate writer: %w", err)
		}
		if _, err := writer.Write(body); err != nil {
			return nil, fmt.Errorf("reqcompress: deflate encode: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("reqcompress: deflate close: %w", err)
		}
	default:
		return nil, fmt.Errorf("reqcompress: unsupported algorithm %q", algorithm)
	}
	return buf.Bytes(), nil
}
