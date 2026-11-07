package service

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// compressOpenAIOAuthCodexRequestBody encodes requests to the official
// ChatGPT Codex backend. API-key accounts may target arbitrary compatible
// upstreams, so their bodies must remain uncompressed.
func compressOpenAIOAuthCodexRequestBody(account *Account, body []byte) ([]byte, bool, error) {
	if account == nil || account.Type != AccountTypeOAuth {
		return body, false, nil
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
	if err != nil {
		return nil, false, fmt.Errorf("create zstd encoder: %w", err)
	}
	defer encoder.Close()

	return encoder.EncodeAll(body, nil), true, nil
}
